# User Guide

Starting with descheduler release v0.10.0 container images are available in the official k8s container registry.

Descheduler Version | Container Image                                 | Architectures           |
------------------- |-------------------------------------------------|-------------------------|
v0.30.2             | registry.k8s.io/descheduler/descheduler:v0.30.2 | AMD64<br>ARM64<br>ARMv7 |
v0.30.1             | registry.k8s.io/descheduler/descheduler:v0.30.1 | AMD64<br>ARM64<br>ARMv7 |
v0.30.0             | registry.k8s.io/descheduler/descheduler:v0.30.0 | AMD64<br>ARM64<br>ARMv7 |
v0.29.0             | registry.k8s.io/descheduler/descheduler:v0.29.0 | AMD64<br>ARM64<br>ARMv7 |
v0.28.1             | registry.k8s.io/descheduler/descheduler:v0.28.1 | AMD64<br>ARM64<br>ARMv7 |
v0.28.0             | registry.k8s.io/descheduler/descheduler:v0.28.0 | AMD64<br>ARM64<br>ARMv7 |
v0.27.1             | registry.k8s.io/descheduler/descheduler:v0.27.1 | AMD64<br>ARM64<br>ARMv7 |
v0.27.0             | registry.k8s.io/descheduler/descheduler:v0.27.0 | AMD64<br>ARM64<br>ARMv7 |
v0.26.1             | registry.k8s.io/descheduler/descheduler:v0.26.1 | AMD64<br>ARM64<br>ARMv7 |
v0.26.0             | registry.k8s.io/descheduler/descheduler:v0.26.0 | AMD64<br>ARM64<br>ARMv7 |
v0.25.1             | registry.k8s.io/descheduler/descheduler:v0.25.1 | AMD64<br>ARM64<br>ARMv7 |
v0.25.0             | registry.k8s.io/descheduler/descheduler:v0.25.0 | AMD64<br>ARM64<br>ARMv7 |
v0.24.1             | registry.k8s.io/descheduler/descheduler:v0.24.1 | AMD64<br>ARM64<br>ARMv7 |
v0.24.0             | registry.k8s.io/descheduler/descheduler:v0.24.0 | AMD64<br>ARM64<br>ARMv7 |
v0.23.1             | registry.k8s.io/descheduler/descheduler:v0.23.1 | AMD64<br>ARM64<br>ARMv7 |
v0.22.0             | registry.k8s.io/descheduler/descheduler:v0.22.0 | AMD64<br>ARM64<br>ARMv7 |
v0.21.0             | registry.k8s.io/descheduler/descheduler:v0.21.0 | AMD64<br>ARM64<br>ARMv7 |
v0.20.0             | registry.k8s.io/descheduler/descheduler:v0.20.0 | AMD64<br>ARM64          |
v0.19.0             | registry.k8s.io/descheduler/descheduler:v0.19.0 | AMD64                   |
v0.18.0             | registry.k8s.io/descheduler/descheduler:v0.18.0 | AMD64                   |
v0.10.0             | registry.k8s.io/descheduler/descheduler:v0.10.0 | AMD64                   |

Note that multi-arch container images cannot be pulled by [kind](https://kind.sigs.k8s.io) from a registry. Therefore
starting with descheduler release v0.20.0 use the below process to download the official descheduler
image into a kind cluster.
```
kind create cluster
docker pull registry.k8s.io/descheduler/descheduler:v0.20.0
kind load docker-image registry.k8s.io/descheduler/descheduler:v0.20.0
```

## Policy Configuration Examples
The [examples](https://github.com/kubernetes-sigs/descheduler/tree/master/examples) directory has descheduler policy configuration examples.

## CLI Options
The descheduler has many CLI options that can be used to override its default behavior. Please check the [CLI Options](./cli/descheduler.md) documentation for details

## Production Use Cases
This section contains descriptions of real world production use cases.

### Balance Cluster By Pod Age
When initially migrating applications from a static virtual machine infrastructure to a cloud native k8s
infrastructure there can be a tendency to treat application pods like static virtual machines. One approach
to help prevent developers and operators from treating pods like virtual machines is to ensure that pods
only run for a fixed amount
of time.

The `PodLifeTime` strategy can be used to ensure that old pods are evicted. It is recommended to create a
[pod disruption budget](https://kubernetes.io/docs/tasks/run-application/configure-pdb/) for each
application to ensure application availability.
```
descheduler -v=3 --evict-local-storage-pods --policy-config-file=pod-life-time.yml
```

This policy configuration file ensures that pods created more than 7 days ago are evicted.
```
---
apiVersion: "descheduler/v1alpha2"
kind: "DeschedulerPolicy"
profiles:
  - name: ProfileName
    pluginConfig:
    - name: "PodLifeTime"
      args:
        maxPodLifeTimeSeconds: 604800
    plugins:
      deschedule:
        enabled:
          - "PodLifeTime"
```

### Balance Cluster By Node Memory Utilization
If your cluster has been running for a long period of time, you may find that the resource utilization is not very
balanced. The following two strategies can be used to rebalance your cluster based on `cpu`, `memory`
or `number of pods`.

#### Balance high utilization nodes
Using `LowNodeUtilization`, descheduler will rebalance the cluster based on memory by evicting pods
from nodes with memory utilization over 70% to nodes with memory utilization below 20%.

```
apiVersion: "descheduler/v1alpha2"
kind: "DeschedulerPolicy"
profiles:
  - name: ProfileName
    pluginConfig:
    - name: "LowNodeUtilization"
      args:
        thresholds:
          "memory": 20
        targetThresholds:
          "memory": 70
    plugins:
      balance:
        enabled:
          - "LowNodeUtilization"
```

#### Balance low utilization nodes
Using `HighNodeUtilization`, descheduler will rebalance the cluster based on memory by evicting pods
from nodes with memory utilization lower than 20%. This should be use `NodeResourcesFit` with the `MostAllocated` scoring strategy based on these [doc](https://kubernetes.io/docs/reference/scheduling/config/#scheduling-plugins).
The evicted pods will be compacted into minimal set of nodes.

```
apiVersion: "descheduler/v1alpha2"
kind: "DeschedulerPolicy"
profiles:
  - name: ProfileName
    pluginConfig:
    - name: "HighNodeUtilization"
      args:
        thresholds:
          "memory": 20
    plugins:
      balance:
        enabled:
          - "HighNodeUtilization"
```

### Autoheal Node Problems

Descheduler's `RemovePodsViolatingNodeTaints` strategy can be combined with
[Node Problem Detector](https://github.com/kubernetes/node-problem-detector/) and
[Cluster Autoscaler](https://github.com/kubernetes/autoscaler/tree/master/cluster-autoscaler) to automatically remove
Nodes which have problems. Node Problem Detector can detect specific Node problems and report them to the API server.
There is a feature called TaintNodeByCondition of the node controller that takes some conditions and turns them into taints. Currently, this only works for the default node conditions: PIDPressure, MemoryPressure, DiskPressure, Ready, and some cloud provider specific conditions.
The Descheduler will then deschedule workloads from those Nodes. Finally, if the descheduled Node's resource
allocation falls below the Cluster Autoscaler's scale down threshold, the Node will become a scale down candidate
and can be removed by Cluster Autoscaler. These three components form an autohealing cycle for Node problems.

---
**NOTE**

Once [kubernetes/node-problem-detector#565](https://github.com/kubernetes/node-problem-detector/pull/565) is available in NPD, we need to update this section.

---
