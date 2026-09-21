# DeploymentEvictionLimit

An evictor plugin that caps how many pods of each listed Deployment the
descheduler may evict in a single descheduling cycle. It fills the gap between
the built-in `maxNoOfPodsToEvictPerNode` / `maxNoOfPodsToEvictPerNamespace` /
`maxNoOfPodsToEvictTotal` limits (which cannot see workloads) and
PodDisruptionBudgets (which bound concurrent unavailability, not per-cycle
churn).

## How it works

Strategies call the profile's `preevictionfilter` chain before evicting a pod.
This plugin resolves the pod's Deployment and clears the pod for eviction,
consuming one unit of that Deployment's budget the first time it sees the pod.
Once the budget is spent, further pods of that Deployment are rejected.

The Deployment is resolved from the pod alone. The Deployment controller names
every ReplicaSet `<deployment>-<pod-template-hash>` and stamps the same hash on
each pod as the `pod-template-hash` label, so stripping that suffix from the
owning ReplicaSet's name yields the Deployment name. No API calls, no extra
RBAC, and it works in `--dry-run` mode (whose sandbox client only mirrors a
fixed set of resources).

The plugin also registers a no-op `Deschedule` extension point whose only job
is to reset the per-cycle state. The framework runs all `deschedule` plugins of
a profile before any `balance` plugin, so listing this plugin **first** under
`deschedule.enabled` resets the budget at the start of every cycle. If it is
not listed there, the state never resets and the plugin logs a warning; it then
only gets stricter over time, never looser.

Budgets are per profile instance. Two profiles that both enable the plugin get
independent counters.

## Semantics worth knowing

- **Never over-evicts, may under-evict.** The budget counts pods *cleared* for
  eviction, not confirmed evictions. Two things spend budget without an
  eviction happening:
  - the API server rejects the eviction afterwards (for example a PDB returns
    429);
  - a strategy calls `PreEvictionFilter` while *listing* candidates and then
    decides not to evict that pod. `PodLifeTime`, `RemoveDuplicates`,
    `RemoveFailedPods`, `RemovePodsHavingTooManyRestarts`,
    `RemovePodsViolatingNodeAffinity`, `LowNodeUtilization` and
    `HighNodeUtilization` do this. `RemovePodsViolatingTopologySpreadConstraint`
    and `RemovePodsViolatingInterPodAntiAffinity` call it only right before
    evicting, so for those the cap is exact.
- Repeated `PreEvictionFilter` / `Filter` calls for the **same pod** are free;
  budget is counted per distinct pod UID.
- `maxPodsToEvict: 0` blocks eviction of that Deployment's pods entirely.
- Deployments not listed are not limited by this plugin at all.
- Only pods managed through an `apps/v1` ReplicaSet with a `pod-template-hash`
  label are attributed to a Deployment. StatefulSets, DaemonSets, hand-made
  ReplicaSets and bare pods pass through untouched.
- Deployment names longer than about 240 characters are truncated by the
  Deployment controller when it names the ReplicaSet, so such a Deployment
  would not match its configured name. This is not a practical concern.

## Policy example

```yaml
apiVersion: "descheduler/v1alpha2"
kind: "DeschedulerPolicy"
profiles:
- name: default
  pluginConfig:
  - name: DefaultEvictor
    args:
      nodeFit: true
  - name: DeploymentEvictionLimit
    args:
      deployments:
      - namespace: shop
        name: web
        maxPodsToEvict: 2
      - namespace: shop
        name: checkout
        maxPodsToEvict: 1
      - namespace: payments
        name: ledger
        maxPodsToEvict: 0   # never evict
  - name: RemovePodsViolatingTopologySpreadConstraint
    args:
      constraints:
      - DoNotSchedule
      - ScheduleAnyway
  plugins:
    deschedule:
      enabled:
      - DeploymentEvictionLimit          # must be first: resets counters each cycle
    balance:
      enabled:
      - RemovePodsViolatingTopologySpreadConstraint
    filter:
      enabled:
      - DefaultEvictor
      - DeploymentEvictionLimit
    preevictionfilter:                   # note: all-lowercase key (see "Gotcha" below)
      enabled:
      - DefaultEvictor
      - DeploymentEvictionLimit
```

## Gotcha: the extension point key is `preevictionfilter`

The `v1alpha2` policy type tags this field as `preevictionfilter` (all
lowercase), unlike the camelCase used everywhere else. The descheduler's policy
decoder is case-sensitive but not strict, so a `preEvictionFilter:` key is
silently ignored and the plugin never consumes budget (only `filter` would
still run, read-only). Always spell it `preevictionfilter:`.
