[![Build Status](https://travis-ci.org/kubernetes-sigs/descheduler.svg?branch=master)](https://travis-ci.org/kubernetes-sigs/descheduler)
[![Go Report Card](https://goreportcard.com/badge/kubernetes-sigs/descheduler)](https://goreportcard.com/report/sigs.k8s.io/descheduler)

# Descheduler for Kubernetes

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
Descheduler, based on its policy, finds pods that can be moved and evicts them. Please
note, in current implementation, descheduler does not schedule replacement of evicted pods
but relies on the default scheduler for that.

## Build and Run

- Checkout the repo into your $GOPATH directory under src/sigs.k8s.io/descheduler

Build descheduler:

```sh
$ make
```

and run descheduler:

```sh
$ ./_output/bin/descheduler --kubeconfig <path to kubeconfig> --policy-config-file <path-to-policy-file>
```

For more information about available options run:
```
$ ./_output/bin/descheduler --help
```

## Running Descheduler as a Job Inside of a Pod

Descheduler can be run as a job inside of a pod. It has the advantage of
being able to be run multiple times without needing user intervention.
Descheduler pod is run as a critical pod to avoid being evicted by itself,
or by kubelet due to an eviction event. Since critical pods are created in
`kube-system` namespace, descheduler job and its pod will also be created
in `kube-system` namespace.

###  Create a container image

First we create a simple Docker image utilizing the Dockerfile found in the root directory:

```
$ make dev-image
```

This creates an image based off the binary we've built before. To build both the
binary and image in one step you can run the following command:

```
$ make image
```

This eliminates the need to have Go installed locally and builds the binary
within it's own container.

### Create a cluster role

To give necessary permissions for the descheduler to work in a pod, create a cluster role:

```
$ cat << EOF| kubectl create -f -
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: descheduler-cluster-role
rules:
- apiGroups: [""]
  resources: ["nodes"]
  verbs: ["get", "watch", "list"]
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "watch", "list", "delete"]
- apiGroups: [""]
  resources: ["pods/eviction"]
  verbs: ["create"]
EOF
```

### Create the service account which will be used to run the job:

```
$ kubectl create sa descheduler-sa -n kube-system
```

### Bind the cluster role to the service account:

```
$ kubectl create clusterrolebinding descheduler-cluster-role-binding \
    --clusterrole=descheduler-cluster-role \
    --serviceaccount=kube-system:descheduler-sa
```
### Create a configmap to store descheduler policy

Descheduler policy is created as a ConfigMap in `kube-system` namespace
so that it can be mounted as a volume inside pod.

```
$ kubectl create configmap descheduler-policy-configmap \
     -n kube-system --from-file=<path-to-policy-dir/policy.yaml>
```
### Create the job specification (descheduler-job.yaml)

```
apiVersion: batch/v1
kind: Job
metadata:
  name: descheduler-job
  namespace: kube-system
spec:
  parallelism: 1
  completions: 1
  template:
    metadata:
      name: descheduler-pod
      annotations:
        scheduler.alpha.kubernetes.io/critical-pod: ""
    spec:
        containers:
        - name: descheduler
          image: descheduler
          volumeMounts:
          - mountPath: /policy-dir
            name: policy-volume
          command: ["/bin/descheduler",  "--policy-config-file", "/policy-dir/policy.yaml"]
        restartPolicy: "Never"
        serviceAccountName: descheduler-sa
        volumes:
        - name: policy-volume
          configMap:
            name: descheduler-policy-configmap
```

Please note that the pod template is configured with critical pod annotation, and
the policy `policy-file` is mounted as a volume from the config map.

### Run the descheduler as a job in a pod:
```
$ kubectl create -f descheduler-job.yaml
```

## Policy and Strategies

Descheduler's policy is configurable and includes strategies to be enabled or disabled.
Four strategies, `RemoveDuplicates`, `LowNodeUtilization`, `RemovePodsViolatingInterPodAntiAffinity`, `RemovePodsViolatingNodeAffinity` are currently implemented.
As part of the policy, the parameters associated with the strategies can be configured too.
By default, all strategies are enabled.

### RemoveDuplicates

This strategy makes sure that there is only one pod associated with a Replica Set (RS),
Replication Controller (RC), Deployment, or Job running on same node. If there are more,
those duplicate pods are evicted for better spreading of pods in a cluster. This issue could happen
if some nodes went down due to whatever reasons, and pods on them were moved to other nodes leading to
more than one pod associated with RS or RC, for example, running on same node. Once the failed nodes
are ready again, this strategy could be enabled to evict those duplicate pods. Currently, there are no
parameters associated with this strategy. To disable this strategy, the policy should look like:

```
apiVersion: "descheduler/v1alpha1"
kind: "DeschedulerPolicy"
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
apiVersion: "descheduler/v1alpha1"
kind: "DeschedulerPolicy"
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

### RemovePodsViolatingInterPodAntiAffinity

This strategy makes sure that pods violating interpod anti-affinity are removed from nodes. For example, if there is podA on node and podB and podC(running on same node) have antiaffinity rules which prohibit them to run on the same node, then podA will be evicted from the node so that podB and podC could run. This issue could happen, when the anti-affinity rules for pods B,C are created when they are already running on node. Currently, there are no parameters associated with this strategy. To disable this strategy, the policy should look like:

```
apiVersion: "descheduler/v1alpha1"
kind: "DeschedulerPolicy"
strategies:
  "RemovePodsViolatingInterPodAntiAffinity":
     enabled: false
```

### RemovePodsViolatingNodeAffinity

This strategy makes sure that pods violating node affinity are removed from nodes. For example, there is podA that was scheduled on nodeA which satisfied the node affinity rule `requiredDuringSchedulingIgnoredDuringExecution` at the time of scheduling, but over time nodeA no longer satisfies the rule, then if another node nodeB is available that satisfies the node affinity rule, then podA will be evicted from nodeA. The policy file should like this -

```
apiVersion: "descheduler/v1alpha1"
kind: "DeschedulerPolicy"
strategies:
  "RemovePodsViolatingNodeAffinity":
    enabled: true
    params:
      nodeAffinityType:
      - "requiredDuringSchedulingIgnoredDuringExecution"
```

## Pod Evictions

When the descheduler decides to evict pods from a node, it employs following general mechanism:

* Critical pods (with annotations scheduler.alpha.kubernetes.io/critical-pod) are never evicted.
* Pods (static or mirrored pods or stand alone pods) not part of an RC, RS, Deployment or Jobs are
never evicted because these pods won't be recreated.
* Pods associated with DaemonSets are never evicted.
* Pods with local storage are never evicted.
* Best efforts pods are evicted before Burstable and Guaranteed pods.

### Pod disruption Budget (PDB)
Pods subject to Pod Disruption Budget (PDB) are not evicted if descheduling violates its pod
disruption budget (PDB). The pods are evicted by using eviction subresource to handle PDB.

## Roadmap

This roadmap is not in any particular order.

* Strategy to consider taints and tolerations
* Consideration of pod affinity
* Strategy to consider pod life time
* Strategy to consider number of pending pods
* Integration with cluster autoscaler
* Integration with metrics providers for obtaining real load metrics
* Consideration of Kubernetes's scheduler's predicates


## Compatibility matrix

Descheduler  | supported Kubernetes version
-------------|-----------------------------
0.4+          | 1.9+
0.1-0.3      | 1.7-1.8

## Community, discussion, contribution, and support

Learn how to engage with the Kubernetes community on the [community page](http://kubernetes.io/community/).

You can reach the maintainers of this project at:

- [Slack channel](https://kubernetes.slack.com/messages/sig-scheduling)
- [Mailing list](https://groups.google.com/forum/#!forum/kubernetes-sig-scheduling)

### Code of conduct

Participation in the Kubernetes community is governed by the [Kubernetes Code of Conduct](code-of-conduct.md).
