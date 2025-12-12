# PodLifeTime Plugin

## What It Does

The PodLifeTime plugin evicts pods that have been running for too long. You can configure a maximum age threshold, and the plugin evicts pods older than that threshold. The oldest pods are evicted first.

## How It Works

The plugin examines all pods across your nodes and selects those that exceed the configured age threshold. You can further narrow down which pods are considered by specifying:

- Which namespaces to include or exclude
- Which labels pods must have
- Which states pods must be in (e.g., Running, Pending, CrashLoopBackOff)

Once pods are selected, they are sorted by age (oldest first) and evicted in that order. Eviction stops when limits are reached (per-node limits, total limits, or Pod Disruption Budget constraints).

## Use Cases

- **Resource Leakage Mitigation**: Restart long-running pods that may have accumulated memory leaks, stale cache, or resource leaks
  ```yaml
  args:
    maxPodLifeTimeSeconds: 604800  # 7 days
    states: [Running]
  ```

- **Ephemeral Workload Cleanup**: Remove long-running batch jobs, test pods, or temporary workloads that have exceeded their expected lifetime
  ```yaml
  args:
    maxPodLifeTimeSeconds: 7200  # 2 hours
    states: [Succeeded, Failed]
  ```

- **Node Hygiene**: Remove forgotten or stuck pods that are consuming resources but not making progress
  ```yaml
  args:
    maxPodLifeTimeSeconds: 3600  # 1 hour
    states: [CrashLoopBackOff, ImagePullBackOff, ErrImagePull]
    includingInitContainers: true
  ```

- **Config/Secret Update Pickup**: Force pod restart to pick up updated ConfigMaps, Secrets, or environment variables
  ```yaml
  args:
    maxPodLifeTimeSeconds: 86400  # 1 day
    states: [Running]
    labelSelector:
      matchLabels:
        config-refresh: enabled
  ```

- **Security Rotation**: Periodically refresh pods to pick up new security tokens, certificates, or patched container images
  ```yaml
  args:
    maxPodLifeTimeSeconds: 259200  # 3 days
    states: [Running]
    namespaces:
      exclude: [kube-system]
  ```

- **Dev/Test Environment Cleanup**: Automatically clean up old pods in development or staging namespaces
  ```yaml
  args:
    maxPodLifeTimeSeconds: 86400  # 1 day
    namespaces:
      include: [dev, staging, test]
  ```

- **Cluster Health Freshness**: Ensure pods periodically restart to maintain cluster health and verify workloads can recover from restarts
  ```yaml
  args:
    maxPodLifeTimeSeconds: 604800  # 7 days
    states: [Running]
    namespaces:
      exclude: [kube-system, production]
  ```

- **Rebalancing Assistance**: Work alongside other descheduler strategies by removing old pods to allow better pod distribution
  ```yaml
  args:
    maxPodLifeTimeSeconds: 1209600  # 14 days
    states: [Running]
  ```

- **Non-Critical Stateful Refresh**: Occasionally reset tolerable stateful workloads that can handle data loss or have external backup mechanisms
  ```yaml
  args:
    maxPodLifeTimeSeconds: 2592000  # 30 days
    labelSelector:
      matchLabels:
        stateful-tier: cache
  ```

## Configuration

| Parameter | Description | Type | Required | Default |
|-----------|-------------|------|----------|---------|
| `maxPodLifeTimeSeconds` | Pods older than this many seconds are evicted | `uint` | Yes | - |
| `namespaces` | Limit eviction to specific namespaces (or exclude specific namespaces) | `Namespaces` | No | `nil` |
| `labelSelector` | Only evict pods matching these labels | `metav1.LabelSelector` | No | `nil` |
| `states` | Only evict pods in specific states (e.g., Running, CrashLoopBackOff) | `[]string` | No | `nil` |
| `includingInitContainers` | When checking states, also check init container states | `bool` | No | `false` |
| `includingEphemeralContainers` | When checking states, also check ephemeral container states | `bool` | No | `false` |

### Discovering states

Each pod is checked for the following locations to discover its relevant state:

1. **Pod Phase** - The overall pod lifecycle phase:
   - `Running` - Pod is running on a node
   - `Pending` - Pod has been accepted but containers are not yet running
   - `Succeeded` - All containers terminated successfully
   - `Failed` - All containers terminated, at least one failed
   - `Unknown` - Pod state cannot be determined

2. **Pod Status Reason** - Why the pod is in its current state:
   - `NodeAffinity` - Pod cannot be scheduled due to node affinity rules
   - `NodeLost` - Node hosting the pod is lost
   - `Shutdown` - Pod terminated due to node shutdown
   - `UnexpectedAdmissionError` - Pod admission failed unexpectedly

3. **Container Waiting Reason** - Why containers are waiting to start:
   - `PodInitializing` - Pod is still initializing
   - `ContainerCreating` - Container is being created
   - `ImagePullBackOff` - Image pull is failing and backing off
   - `CrashLoopBackOff` - Container is crashing repeatedly
   - `CreateContainerConfigError` - Container configuration is invalid
   - `ErrImagePull` - Image cannot be pulled
   - `CreateContainerError` - Container creation failed
   - `InvalidImageName` - Image name is invalid

By default, only regular containers are checked. Enable `includingInitContainers` or `includingEphemeralContainers` to also check those container types.

## Example

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
          namespaces:
            include:
              - default
          states:
            - Running
```

This configuration evicts Running pods in the `default` namespace that are older than 1 day.
