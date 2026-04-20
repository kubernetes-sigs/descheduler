## What type of PR is this?

/kind bug

## What this PR does / why we need it

When using metrics-based node utilization (`KubernetesMetrics` or `Prometheus`), the `LowNodeUtilization` plugin fails entirely if **any** Ready node is missing from the collected metrics. This can happen when a node is Ready but unreachable by metrics-server (e.g. a roaming laptop node connected via WireGuard with intermittent connectivity — metrics-server reports "no route to host").

The metrics collector (`metricscollector.go`) already handles missing node metrics gracefully — it logs an error and continues to the next node. However, the downstream `actualUsageClient.sync()` and `prometheusUsageClient.sync()` methods treat a missing node as a **hard error**, returning `fmt.Errorf(...)` which aborts the entire balance cycle for **all** nodes.

This means a single unreachable node prevents the descheduler from performing any load balancing, even among the remaining healthy nodes with valid metrics.

Note: `HighNodeUtilization` currently only uses `requestedUsageClient` (which doesn't depend on metrics), so it is not affected by this bug today. However, the `usageClient` interface is shared, so this PR updates the interface and both callers for consistency.

## How does this PR fix it

- Change the `usageClient.sync()` interface to return `([]*v1.Node, error)` — the returned slice is the subset of input nodes for which usage data is available.
- `actualUsageClient.sync()` and `prometheusUsageClient.sync()` now log a warning at V(1) and skip nodes without metrics instead of returning a fatal error.
- `requestedUsageClient.sync()` returns the full input list (it doesn't depend on metrics).
- Both `LowNodeUtilization.Balance()` and `HighNodeUtilization.Balance()` now operate on the filtered node list returned by `sync()`. (`HighNodeUtilization` currently only uses `requestedUsageClient`, which always returns all nodes, but is updated for interface consistency.)

## Which issue(s) this PR fixes

None filed yet — discovered while running a mixed cluster with VPS + roaming nodes.

## Special notes for your reviewer

The `usageClient` interface is package-private, so the signature change only affects code within `nodeutilization/`.

## Does this PR introduce a user-facing change?

```release-note
nodeutilization plugins (LowNodeUtilization, HighNodeUtilization) now skip nodes without available metrics instead of failing the entire balance cycle. This allows the descheduler to continue operating when some Ready nodes are temporarily unreachable by metrics-server or Prometheus.
```
