# RFC: ZoneAwareNodeFit for RemovePodsViolatingTopologySpreadConstraint

**Status:** Implementable  
**Author:** bruno.chauvet@rokt.com  
**Date:** 2026-04-15  
**Related issues:** kubernetes-sigs/descheduler#1534, kubernetes-sigs/descheduler#1067

---

## Problem

`RemovePodsViolatingTopologySpreadConstraint` evicts pods from over-loaded topology
zones. The existing `TopologyBalanceNodeFit` flag (default: `true`) gates eviction on
whether the pod can fit on *some* node across *all* under-loaded zones combined. This
aggregate check has a blind spot: if under-loaded zones each individually lack capacity,
but the check passes because a node from zone B and a node from zone C together appear
schedulable, the pod may be evicted and then reshuffled within the same over-loaded
cluster of zones—causing bounded eviction churn.

### Illustrative scenario

```
Zone A (over-loaded): 10 pods, nodes have 2 000 m CPU free per node
Zone B (under-loaded by pod count): 3 pods, nodes have only 50 m CPU free
Zone C (under-loaded by pod count): 3 pods, nodes have only 50 m CPU free
Pod to evict: requests 100 m CPU
```

With `TopologyBalanceNodeFit: true`, `nodesBelowIdealAvg` contains nodes from B and C
combined. `PodFitsAnyOtherNode(pod, B+C)` returns `false` because no individual node
has 100 m free — the check is correct here. But if any single node in B or C had
100 m free, the check would pass even if the ZONE-LEVEL capacity picture meant the
pod would be rescheduled back to A after eviction.

The subtler issue: `nodesBelowIdealAvg` is filtered by pod *count*, not by resource
headroom. A zone can appear under-loaded (fewer pods than average) while being
resource-saturated. Zone-level capacity validation requires per-zone independent checks.

## Proposed Change

Add a new opt-in field `ZoneAwareNodeFit *bool` to
`RemovePodsViolatingTopologySpreadConstraintArgs` (default: `false`).

When `ZoneAwareNodeFit: true`, `balanceDomains` additionally:

1. Groups eligible nodes from under-loaded zones by their topology key value (zone →
   nodes map).
2. For each candidate pod, checks whether at least one *specific* zone has a node that
   can fit the pod (resource requests, nodeSelector, taints).
3. Skips eviction of the pod if no single target zone individually has capacity.

This is **strictly more restrictive** than `TopologyBalanceNodeFit`'s existing check.
Enabling `ZoneAwareNodeFit` prevents some evictions that `TopologyBalanceNodeFit` alone
would permit: specifically, when no individual under-loaded zone has a node with enough
capacity to fit the pod, but the union of nodes across all under-loaded zones contains
a fitting node. In that case the aggregate check passes while the zone-aware check
blocks the eviction, avoiding churn from a pod being rescheduled to a zone that cannot
actually accommodate it independently.

## API

```yaml
apiVersion: "descheduler/v1alpha2"
kind: DeschedulerPolicy
profiles:
  - name: default
    plugins:
      balance:
        enabled:
          - name: RemovePodsViolatingTopologySpreadConstraint
    pluginConfig:
      - name: RemovePodsViolatingTopologySpreadConstraint
        args:
          topologyBalanceNodeFit: true   # unchanged — existing field
          zoneAwareNodeFit: true         # NEW — opt-in per-zone capacity check
```

### Field semantics

| Field | Default | Meaning |
|---|---|---|
| `topologyBalanceNodeFit` | `true` | Gate: pod must fit on *some* node across the union of all under-loaded zone nodes |
| `zoneAwareNodeFit` | `false` | Gate: pod must fit within at least one *specific* under-loaded zone |

Both gates are independent. Enabling both is valid (redundant for a single under-loaded
zone; stricter for multiple under-loaded zones with mixed capacity).

## Implementation

### New types (types.go)

Add `ZoneAwareNodeFit *bool` after `TopologyBalanceNodeFit`:

```go
ZoneAwareNodeFit *bool `json:"zoneAwareNodeFit,omitempty"`
```

### Default value (defaults.go)

`SetDefaults_RemovePodsViolatingTopologySpreadConstraintArgs` must initialise the new
field when it is nil:

```go
if args.ZoneAwareNodeFit == nil {
    args.ZoneAwareNodeFit = utilptr.To(false)
}
```

### Deep-copy generated file (zz_generated.deepcopy.go)

Adding a `*bool` pointer field to a `+k8s:deepcopy-gen=true` struct requires
regenerating `zz_generated.deepcopy.go`. The `DeepCopyInto` function for
`RemovePodsViolatingTopologySpreadConstraintArgs` must include a nil-check block for
`ZoneAwareNodeFit`, mirroring the existing block for `TopologyBalanceNodeFit`:

```go
if in.ZoneAwareNodeFit != nil {
    in, out := &in.ZoneAwareNodeFit, &out.ZoneAwareNodeFit
    *out = new(bool)
    **out = **in
}
```

Run `hack/update-generated-deepcopy.sh` (or the equivalent `controller-gen` invocation)
to regenerate this file; do not edit it by hand.

### New helper functions (topologyspreadconstraint.go)

```go
// filterNodesByZoneBelowIdealAvg groups nodes from under-loaded topology domains
// by their domain value (e.g. zone label). Only domains with pod count strictly
// below idealAvg are included. Returns nil if no under-loaded domains exist.
func filterNodesByZoneBelowIdealAvg(
    nodes []*v1.Node,
    sortedDomains []topology,
    topologyKey string,
    idealAvg float64,
) map[string][]*v1.Node {
    topologyNodesMap := make(map[string][]*v1.Node, len(sortedDomains))
    for _, n := range nodes {
        if zone, ok := n.Labels[topologyKey]; ok {
            topologyNodesMap[zone] = append(topologyNodesMap[zone], n)
        }
    }
    result := make(map[string][]*v1.Node)
    for _, domain := range sortedDomains {
        if float64(len(domain.pods)) < idealAvg {
            result[domain.pair.value] = topologyNodesMap[domain.pair.value]
        }
    }
    return result
}

// podFitsAnyNodeInSomeZone returns true if the pod can be scheduled on at least
// one node within at least one zone from nodesByZone. Each zone is validated
// independently — the pod must fit entirely within a single zone.
func podFitsAnyNodeInSomeZone(
    nodeIndexer podutil.GetPodsAssignedToNodeFunc,
    pod *v1.Pod,
    nodesByZone map[string][]*v1.Node,
) bool {
    for _, zoneNodes := range nodesByZone {
        if node.PodFitsAnyOtherNode(nodeIndexer, pod, zoneNodes) {
            return true
        }
    }
    return false
}
```

### balanceDomains update (topologyspreadconstraint.go)

In `balanceDomains`, after the existing `topologyBalanceNodeFit` declaration:

```go
zoneAwareNodeFit := utilptr.Deref(d.args.ZoneAwareNodeFit, false)

var nodesByZoneBelowIdealAvg map[string][]*v1.Node
if zoneAwareNodeFit {
    nodesByZoneBelowIdealAvg = filterNodesByZoneBelowIdealAvg(
        eligibleNodes, sortedDomains, tsc.TopologyKey, idealAvg)
}
```

Inside the eviction gate loop, after the existing `topologyBalanceNodeFit` check:

```go
if zoneAwareNodeFit && !podFitsAnyNodeInSomeZone(
    getPodsAssignedToNode, aboveToEvict[k], nodesByZoneBelowIdealAvg) {
    d.logger.V(2).Info("ignoring pod for eviction: no target zone has sufficient capacity",
        "pod", klog.KObj(aboveToEvict[k]))
    continue
}
```

## Alternatives Considered

**Modify PreEvictionFilter in DefaultEvictor to be zone-aware**: requires threading zone
context through the `EvictorPlugin` interface (`PreEvictionFilter(pod *v1.Pod) bool` has
no zone parameter), which is a breaking API change affecting all plugins. Rejected in
favour of a TSC-local, opt-in gate.

**Change TopologyBalanceNodeFit semantics**: would be a silent behaviour change for
existing users. Rejected in favour of a new independent field.

## Test Plan

1. ZoneAwareNodeFit enabled, target zone has capacity → pod evicted
2. ZoneAwareNodeFit enabled, all target zones at CPU capacity → pod NOT evicted (no churn)
3. ZoneAwareNodeFit enabled, multiple under-loaded zones, mixed capacity → pod evicted
   (at least one zone qualifies)
4. ZoneAwareNodeFit disabled (default) → existing behaviour unchanged

## Risk

**Low.** Change is:
- Opt-in (`false` by default)
- Additive (existing `TopologyBalanceNodeFit` gate is unmodified)
- Localised to `balanceDomains` inside the TSC plugin
- No changes to framework types, eviction API, or other plugins
