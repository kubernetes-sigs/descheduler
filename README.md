# Rescheduler for Kubernetes

## Introduction

Scheduling in Kubernetes is the process of binding pending pods to nodes, and is performed by a component of Kubernetes called kube-scheduler. The scheduler's decisions, whether or where a pod can or can not be scheduled, are guided by its configurable policy which comprises of set of rules, called predicates and priorities. The scheduler's decisions are influenced by its view of a Kubernetes cluster at that point of time when a new pod appears first time for scheduling. As a Kubernetes cluster is very dynamic and its state changes over time, the initial scheduling decision might turn out to be a sub-optimal one with respect to the cluster's ever changing state. Consequently, increasing population of pods scheduled on less desired nodes may lead to issues in clusters for example performance degradation. Due to this, it becomes important to continuously revisit initial scheduling decisions. This process of revisiting scheduling decisions and helping some pods reschedule on other nodes is defined as rescheduling, and its implementation here as rescheduler.

## Build and Run

Build rescheduler:

```sh
$ make build
```

and run rescheduler:

```sh
$ ./_output/bin/rescheduler --kubeconfig <path to kubeconfig> --policy-config-file <path-to-policy-file>
```

For more information about available options run:
```
$ ./_output/bin/rescheduler --help
```

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
