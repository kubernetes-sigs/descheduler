# PodLifeTime Plugin

## What It Does

The PodLifeTime plugin evicts pods based on their age, status phase, condition transitions, container states, exit codes, and owner kinds. It can be used for simple age-based eviction or for fine-grained cleanup of pods matching specific transition criteria.

## How It Works

The plugin builds a filter chain from the configured criteria. All non-empty filter categories are ANDed together (a pod must satisfy every specified filter to be evicted). Within each filter category, items are ORed (matching any one entry satisfies that filter).

Once pods are selected, they are sorted by the most relevant time (last transition time if any condition filter carries a `minTimeSinceLastTransitionSeconds`, otherwise creation time) with the oldest first, then evicted in order. Eviction stops when limits are reached (per-node limits, total limits, or Pod Disruption Budget constraints).

## Use Cases

- **Evict completed/succeeded pods that have been idle too long**:
  ```yaml
  args:
    states: [Succeeded]
    conditions:
      - reason: PodCompleted
        status: "True"
        minTimeSinceLastTransitionSeconds: 14400  # 4 hours
  ```

- **Evict failed pods**, excluding Job-owned pods:
  ```yaml
  args:
    states: [Failed]
    exitCodes: [1]
    ownerKinds:
      exclude: [Job]
    maxPodLifeTimeSeconds: 3600
    includingInitContainers: true
  ```

- **Resource Leakage Mitigation**: Restart long-running pods that may have accumulated memory leaks:
  ```yaml
  args:
    maxPodLifeTimeSeconds: 604800  # 7 days
    states: [Running]
  ```

- **Clean up stuck pods in CrashLoopBackOff**:
  ```yaml
  args:
    states: [CrashLoopBackOff, ImagePullBackOff]
  ```

- **Evict pods owned only by specific kinds**:
  ```yaml
  args:
    states: [Succeeded, Failed]
    ownerKinds:
      include: [Job]
    maxPodLifeTimeSeconds: 600
  ```

## Configuration

| Parameter | Description | Type | Required | Default |
|-----------|-------------|------|----------|---------|
| `maxPodLifeTimeSeconds` | Pods older than this many seconds are evicted | `uint` | No* | `nil` |
| `states` | Filter pods by phase, pod status reason, or container waiting/terminated reason. A pod matches if any of its state values appear in this list | `[]string` | No | `nil` |
| `conditions` | Only evict pods with matching status conditions (see PodConditionFilter) | `[]PodConditionFilter` | No | `nil` |
| `exitCodes` | Only evict pods with matching container terminated exit codes | `[]int32` | No | `nil` |
| `ownerKinds` | Include or exclude pods by owner reference kind | `OwnerKinds` | No | `nil` |
| `namespaces` | Limit eviction to specific namespaces (include or exclude) | `Namespaces` | No | `nil` |
| `labelSelector` | Only evict pods matching these labels | `metav1.LabelSelector` | No | `nil` |
| `includingInitContainers` | Extend state/exitCode filtering to init containers | `bool` | No | `false` |
| `includingEphemeralContainers` | Extend state filtering to ephemeral containers | `bool` | No | `false` |

*At least one filtering criterion must be specified (`maxPodLifeTimeSeconds`, `states`, `conditions`, or `exitCodes`).

### States

The `states` field matches pods using an OR across these categories:

| Category | Examples |
|----------|----------|
| Pod phase | `Running`, `Pending`, `Succeeded`, `Failed`, `Unknown` |
| Pod status reason | `NodeAffinity`, `NodeLost`, `Shutdown`, `UnexpectedAdmissionError` |
| Container waiting reason | `CrashLoopBackOff`, `ImagePullBackOff`, `ErrImagePull`, `CreateContainerConfigError`, `CreateContainerError`, `InvalidImageName`, `PodInitializing`, `ContainerCreating` |
| Container terminated reason | `OOMKilled`, `Error`, `Completed`, `DeadlineExceeded`, `Evicted`, `ContainerCannotRun`, `StartError` |

When `includingInitContainers` is true, init container states are also checked. When `includingEphemeralContainers` is true, ephemeral container states are also checked.

### PodConditionFilter

Each condition filter matches against `pod.status.conditions[]` entries. All specified field-level checks must match (AND). Unset fields are not checked. Multiple condition filters in the list are ORed.

| Field | Description |
|-------|-------------|
| `type` | Condition type (e.g., `Ready`, `Initialized`, `ContainersReady`) |
| `status` | Condition status (`True`, `False`, `Unknown`) |
| `reason` | Condition reason (e.g., `PodCompleted`) |
| `minTimeSinceLastTransitionSeconds` | Require the matching condition's `lastTransitionTime` to be at least this many seconds in the past |

At least one of these fields must be set per filter entry.

When `minTimeSinceLastTransitionSeconds` is set on a filter, a pod's condition must both match the type/status/reason fields AND have transitioned long enough ago. If the condition has no `lastTransitionTime`, it does not match.

### OwnerKinds

| Field | Description |
|-------|-------------|
| `include` | Only evict pods owned by these kinds |
| `exclude` | Do not evict pods owned by these kinds |

At most one of `include`/`exclude` may be set.

## Migrating from RemoveFailedPods

`RemoveFailedPods` delegates to `PodLifeTime` under the hood. You can use `PodLifeTime` directly for the same behavior with additional flexibility.

| RemoveFailedPods | PodLifeTime |
|---|---|
| *(implicit phase=Failed)* | `states: [Failed]` |
| `minPodLifetimeSeconds: 3600` | `maxPodLifeTimeSeconds: 3600` |
| `excludeOwnerKinds: [Job]` | `ownerKinds: { exclude: [Job] }` |
| `reasons: [NodeAffinity]` | `states: [Failed, NodeAffinity]` |
| `exitCodes: [1]` | `exitCodes: [1]` |
| `includingInitContainers: true` | `includingInitContainers: true` |

**Before (RemoveFailedPods):**
```yaml
- name: RemoveFailedPods
  args:
    reasons: [NodeAffinity]
    exitCodes: [1]
    excludeOwnerKinds: [Job]
    minPodLifetimeSeconds: 3600
    includingInitContainers: true
```

**After (PodLifeTime):**
```yaml
- name: PodLifeTime
  args:
    states: [Failed, NodeAffinity]
    exitCodes: [1]
    ownerKinds:
      exclude: [Job]
    maxPodLifeTimeSeconds: 3600
    includingInitContainers: true
```

> **Note:** When migrating `RemoveFailedPods` with `reasons`, the reasons are ORed with the `Failed` phase in `states`. This means a pod matching *any* of those values will pass the states filter. If you need strict AND logic (must be Failed AND have a specific reason), use `RemoveFailedPods` or apply additional filtering via `conditions`.

## Example

### Age-based eviction with state filter

```yaml
apiVersion: descheduler/v1alpha2
kind: DeschedulerPolicy
profiles:
  - name: default
    plugins:
      deschedule:
        enabled:
          - name: PodLifeTime
    pluginConfig:
      - name: PodLifeTime
        args:
          maxPodLifeTimeSeconds: 86400  # 1 day
          states:
            - Running
          namespaces:
            include:
              - default
```

### Transition-based eviction for completed pods

```yaml
apiVersion: descheduler/v1alpha2
kind: DeschedulerPolicy
profiles:
  - name: default
    plugins:
      deschedule:
        enabled:
          - name: PodLifeTime
    pluginConfig:
      - name: PodLifeTime
        args:
          states:
            - Succeeded
          conditions:
            - reason: PodCompleted
              status: "True"
              minTimeSinceLastTransitionSeconds: 14400
          namespaces:
            include:
              - default
```

This configuration evicts Succeeded pods in the `default` namespace that have a `PodCompleted` condition with status `True` and whose last matching transition happened more than 4 hours ago.
