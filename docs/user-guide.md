# User Guide

Starting with descheduler release v0.10.0 container images are available in the official k8s container registry.
* `k8s.gcr.io/descheduler/descheduler`

## Policy Configuration Examples
The [examples](https://github.com/kubernetes-sigs/descheduler/tree/master/examples) directory has descheduler policy configuration examples.

## CLI Options
The descheduler has many CLI options that can be used to override its default behavior.
```
descheduler --help
The descheduler evicts pods which may be bound to less desired nodes

Usage:
  descheduler [flags]
  descheduler [command]

Available Commands:
  help        Help about any command
  version     Version of descheduler

Flags:
      --add-dir-header                   If true, adds the file directory to the header of the log messages
      --alsologtostderr                  log to standard error as well as files
      --descheduling-interval duration   Time interval between two consecutive descheduler executions. Setting this value instructs the descheduler to run in a continuous loop at the interval specified.
      --dry-run                          execute descheduler in dry run mode.
      --evict-local-storage-pods         DEPRECATED: enables evicting pods using local storage by descheduler
  -h, --help                             help for descheduler
      --kubeconfig string                File with  kube configuration.
      --log-backtrace-at traceLocation   when logging hits line file:N, emit a stack trace (default :0)
      --log-dir string                   If non-empty, write log files in this directory
      --log-file string                  If non-empty, use this log file
      --log-file-max-size uint           Defines the maximum size a log file can grow to. Unit is megabytes. If the value is 0, the maximum file size is unlimited. (default 1800)
      --log-flush-frequency duration     Maximum number of seconds between log flushes (default 5s)
      --logtostderr                      log to standard error instead of files (default true)
      --max-pods-to-evict-per-node int   DEPRECATED: limits the maximum number of pods to be evicted per node by descheduler
      --node-selector string             DEPRECATED: selector (label query) to filter on, supports '=', '==', and '!='.(e.g. -l key1=value1,key2=value2)
      --policy-config-file string        File with descheduler policy configuration.
      --skip-headers                     If true, avoid header prefixes in the log messages
      --skip-log-headers                 If true, avoid headers when opening log files
      --stderrthreshold severity         logs at or above this threshold go to stderr (default 2)
  -v, --v Level                          number for the log level verbosity
      --vmodule moduleSpec               comma-separated list of pattern=N settings for file-filtered logging

Use "descheduler [command] --help" for more information about a command.
```

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
apiVersion: "descheduler/v1alpha1"
kind: "DeschedulerPolicy"
strategies:
  "PodLifeTime":
    enabled: true
    params:
      maxPodLifeTimeSeconds: 604800 # pods run for a maximum of 7 days
```

### Autoheal Node Problems
Descheduler's `RemovePodsViolatingNodeTaints` strategy can be combined with
[Node Problem Detector](https://github.com/kubernetes/node-problem-detector/) and
[Cluster Autoscaler](https://github.com/kubernetes/autoscaler/tree/master/cluster-autoscaler) to automatically remove
Nodes which have problems. Node Problem Detector can detect specific Node problems and taint any Nodes which have those
problems. The Descheduler will then deschedule workloads from those Nodes. Finally, if the descheduled Node's resource
allocation falls below the Cluster Autoscaler's scale down threshold, the Node will become a scale down candidate
and can be removed by Cluster Autoscaler. These three components form an autohealing cycle for Node problems.
