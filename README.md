# Rescheduler for Kubernetes

## Introduction

Scheduling in Kubernetes is the process of binding pending pods to nodes, and is performed by
a component of Kubernetes called kube-scheduler. The scheduler's decisions, whether or where a
pod can or can not be scheduled, are guided by its configurable policy which comprises of set of
rules, called predicates and priorities. The scheduler's decisions are influenced by its view of
a Kubernetes cluster at that point of time when a new pod appears first time for scheduling.
As Kubernetes clusters are very dynamic and their state change over time, there may be desired
to move already running pods to some other nodes for various reasons:

* Some nodes are under or over utilized.
* The original scheduling decision does not hold true any more, as taints or labels are added to
or removed from nodes, pod/node affinity requirements are not satisfied any more.
* Some nodes failed and their pods moved to other nodes.
* New nodes are added to clusters.

Consequently, there might be several pods scheduled on less desired nodes in a cluster.
Rescheduler, based on its policy, finds pods that can be moved and evicts them. Please
note, in current implementation, rescheduler does not schedule replacement of evicted pods
but relies on the default scheduler for that.

## Build and Run

Build rescheduler:

```sh
$ make
```

and run rescheduler:

```sh
$ ./_output/bin/rescheduler --kubeconfig <path to kubeconfig> --policy-config-file <path-to-policy-file>
```

For more information about available options run:
```
$ ./_output/bin/rescheduler --help
```

## Policy and Strategies
 
Rescheduler's policy is configurable and includes strategies to be enabled or disabled.
Two strategies, `RemoveDuplicates` and `LowNodeUtilization` are currently implemented.
As part of the policy, the parameters associated with the strategies can be configured too.
By default, all strategies are enabled.

### RemoveDuplicates

This strategy makes sure that there is only one pod associated with a Replica Set (RS),
Replication Controller (RC), Deployment, or Job running on same node. If there are more,
those duplicate pods are evicted for better spreading of pods in a cluster. This issue could happen
if some nodes went down due to whatever reasons, and pods on them were moved to other nodes leading to
more than one pod associated with RS or RC, for example, running on same node. Once the failed nodes
are ready again, this strategy could be enabled to evict those duplicate pods. Currently, there are no
parameters associated with this strategy. To disable this strategy, the policy would look like:

```
apiVersion: "rescheduler/v1alpha1"
kind: "ReschedulerPolicy"
strategies:
  "RemoveDuplicates":
     enabled: false
```

### LowNodeUtilization

This strategy finds nodes that are under utilized and evicts pods, if possible, from other nodes
in the hope that recreation of evicted pods will be scheduled on these underutilized nodes. The
parameters of this strategy are configured under `nodeResourceUtilizationThresholds`.

The under utilization of nodes is determined by a configurable threshold, `thresholds`. The threshold
`thresholds` can be configured for cpu, memory, and number of pods in terms of percentage. If a node's
usage is below threshold for all (cpu, memory, and number of pods), the node is considered underutilized.
Currently, pods' request resource requirements are considered for computing node resource utilization.

There is another configurable threshold, `targetThresholds`, that is used to compute those potential nodes
from where pods could be evicted. Any node, between the thresholds, `thresholds` and `targetThresholds` is
considered appropriately utilized and is not considered for eviction. The threshold, `targetThresholds`,
can be configured for cpu, memory, and number of pods too in terms of percentage.

These thresholds, `thresholds` and `targetThresholds`, could be tuned as per your cluster requirements.
An example of the policy for this strategy would look like:

```
apiVersion: "rescheduler/v1alpha1"
kind: "ReschedulerPolicy"
strategies:
  "LowNodeUtilization":
     enabled: true
     params:
       nodeResourceUtilizationThresholds:
         thresholds:
           "cpu" : 20
           "memory": 20
           "pods": 20
         targetThresholds:
           "cpu" : 50
           "memory": 50
           "pods": 50
```

There is another parameter associated with `LowNodeUtilization` strategy, called `numberOfNodes`.
This parameter can be configured to activate the strategy only when number of under utilized nodes
are above the configured value. This could be helpful in large clusters where a few nodes could go
under utilized frequently or for a short period of time. By default, `numberOfNodes` is set to zero.

## Pod Evictions

When the rescheduler decides to evict pods from a node, it employs following general mechanism:

* Critical pods (with annotations scheduler.alpha.kubernetes.io/critical-pod) are never evicted. 
* Pods (static or mirrored pods or stand alone pods) not part of an RC, RS, Deployment or Jobs are
never evicted because these pods won't be recreated.
* Pods associated with DaemonSets are never evicted.
* Pods with local storage are never evicted.
* Best efforts pods are evicted before Burstable and Guaranteed pods. 

### Pod disruption Budget (PDB)
Pods subject to Pod Disruption Budget (PDB) are not evicted if rescheduling violates its pod
disruption budget (PDB). The pods are evicted by using eviction subresource to handle PDB.

## Roadmap

This roadmap is not in any particular order.

* Addition of test cases (unit and end-to-end)
* Ability to run inside a pod as a job
* Strategy to consider taints and tolerations
* Consideration of pod affinity and anti-affinity
* Strategy to consider pod life time
* Strategy to consider number of pending pods
* Integration with cluster autoscaler
* Integration with metrics providers for obtaining real load metrics
* Consideration of Kubernetes's scheduler's predicates


## Note

This project is under active development, and is not intended for production use.
Any api could be changed any time with out any notice. That said, your feedback is
very important and appreciated to make this project more stable and useful.
