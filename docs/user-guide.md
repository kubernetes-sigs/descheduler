# User Guide

Starting with descheduler release v0.10.0 container images are available in the official k8s container registry.

Descheduler Version | Container Image                            | Architectures           |
------------------- |--------------------------------------------|-------------------------|
v0.24.1             | k8s.gcr.io/descheduler/descheduler:v0.24.1 | AMD64<br>ARM64<br>ARMv7 |
v0.24.0             | k8s.gcr.io/descheduler/descheduler:v0.24.0 | AMD64<br>ARM64<br>ARMv7 |
v0.23.1             | k8s.gcr.io/descheduler/descheduler:v0.23.1 | AMD64<br>ARM64<br>ARMv7 |
v0.22.0             | k8s.gcr.io/descheduler/descheduler:v0.22.0 | AMD64<br>ARM64<br>ARMv7 |
v0.21.0             | k8s.gcr.io/descheduler/descheduler:v0.21.0 | AMD64<br>ARM64<br>ARMv7 |
v0.20.0             | k8s.gcr.io/descheduler/descheduler:v0.20.0 | AMD64<br>ARM64          |
v0.19.0             | k8s.gcr.io/descheduler/descheduler:v0.19.0 | AMD64                   |
v0.18.0             | k8s.gcr.io/descheduler/descheduler:v0.18.0 | AMD64                   |
v0.10.0             | k8s.gcr.io/descheduler/descheduler:v0.10.0 | AMD64                   |

Note that multi-arch container images cannot be pulled by [kind](https://kind.sigs.k8s.io) from a registry. Therefore
starting with descheduler release v0.20.0 use the below process to download the official descheduler
image into a kind cluster.
```
kind create cluster
docker pull k8s.gcr.io/descheduler/descheduler:v0.20.0
kind load docker-image k8s.gcr.io/descheduler/descheduler:v0.20.0
```

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
  completion  generate the autocompletion script for the specified shell
  help        Help about any command
  version     Version of descheduler

Flags:
      --add-dir-header                           If true, adds the file directory to the header of the log messages (DEPRECATED: will be removed in a future release, see https://github.com/kubernetes/enhancements/tree/master/keps/sig-instrumentation/2845-deprecate-klog-specific-flags-in-k8s-components)
      --alsologtostderr                          log to standard error as well as files (DEPRECATED: will be removed in a future release, see https://github.com/kubernetes/enhancements/tree/master/keps/sig-instrumentation/2845-deprecate-klog-specific-flags-in-k8s-components)
      --bind-address ip                          The IP address on which to listen for the --secure-port port. The associated interface(s) must be reachable by the rest of the cluster, and by CLI/web clients. If blank or an unspecified address (0.0.0.0 or ::), all interfaces will be used. (default 0.0.0.0)
      --cert-dir string                          The directory where the TLS certs are located. If --tls-cert-file and --tls-private-key-file are provided, this flag will be ignored. (default "apiserver.local.config/certificates")
      --descheduling-interval duration           Time interval between two consecutive descheduler executions. Setting this value instructs the descheduler to run in a continuous loop at the interval specified.
      --disable-metrics                          Disables metrics. The metrics are by default served through https://localhost:10258/metrics. Secure address, resp. port can be changed through --bind-address, resp. --secure-port flags.
      --dry-run                                  execute descheduler in dry run mode.
  -h, --help                                     help for descheduler
      --http2-max-streams-per-connection int     The limit that the server gives to clients for the maximum number of streams in an HTTP/2 connection. Zero means to use golang's default.
      --kubeconfig string                        File with  kube configuration.
      --leader-elect                             Start a leader election client and gain leadership before executing the main loop. Enable this when running replicated components for high availability.
      --leader-elect-lease-duration duration     The duration that non-leader candidates will wait after observing a leadership renewal until attempting to acquire leadership of a led but unrenewed leader slot. This is effectively the maximum duration that a leader can be stopped before it is replaced by another candidate. This is only applicable if leader election is enabled. (default 15s)
      --leader-elect-renew-deadline duration     The interval between attempts by the acting master to renew a leadership slot before it stops leading. This must be less than or equal to the lease duration. This is only applicable if leader election is enabled. (default 10s)
      --leader-elect-resource-lock string        The type of resource object that is used for locking during leader election. Supported options are 'endpoints', 'configmaps', 'leases', 'endpointsleases' and 'configmapsleases'. (default "leases")
      --leader-elect-resource-name string        The name of resource object that is used for locking during leader election. (default "descheduler")
      --leader-elect-resource-namespace string   The namespace of resource object that is used for locking during leader election. (default "kube-system")
      --leader-elect-retry-period duration       The duration the clients should wait between attempting acquisition and renewal of a leadership. This is only applicable if leader election is enabled. (default 2s)
      --log-backtrace-at traceLocation           when logging hits line file:N, emit a stack trace (default :0) (DEPRECATED: will be removed in a future release, see https://github.com/kubernetes/enhancements/tree/master/keps/sig-instrumentation/2845-deprecate-klog-specific-flags-in-k8s-components)
      --log_dir string                           If non-empty, write log files in this directory (DEPRECATED: will be removed in a future release, see https://github.com/kubernetes/enhancements/tree/master/keps/sig-instrumentation/2845-deprecate-klog-specific-flags-in-k8s-components)
      --log_file string                          If non-empty, use this log file (DEPRECATED: will be removed in a future release, see https://github.com/kubernetes/enhancements/tree/master/keps/sig-instrumentation/2845-deprecate-klog-specific-flags-in-k8s-components)
      --log_file_max_size uint                   Defines the maximum size a log file can grow to. Unit is megabytes. If the value is 0, the maximum file size is unlimited. (default 1800) (DEPRECATED: will be removed in a future release, see https://github.com/kubernetes/enhancements/tree/master/keps/sig-instrumentation/2845-deprecate-klog-specific-flags-in-k8s-components)
      --log-flush-frequency duration             Maximum number of seconds between log flushes (default 5s)
      --logging-format string                    Sets the log format. Permitted formats: "text", "json". Non-default formats don't honor these flags: --add-dir-header, --alsologtostderr, --log-backtrace-at, --log_dir, --log_file, --log_file_max_size, --logtostderr, --skip-headers, --skip-log-headers, --stderrthreshold, --log-flush-frequency.\nNon-default choices are currently alpha and subject to change without warning. (default "text")
      --logtostderr                              log to standard error instead of files (default true) (DEPRECATED: will be removed in a future release, see https://github.com/kubernetes/enhancements/tree/master/keps/sig-instrumentation/2845-deprecate-klog-specific-flags-in-k8s-components)
      --one-output                               If true, only write logs to their native severity level (vs also writing to each lower severity level) (DEPRECATED: will be removed in a future release, see https://github.com/kubernetes/enhancements/tree/master/keps/sig-instrumentation/2845-deprecate-klog-specific-flags-in-k8s-components)
      --permit-address-sharing                   If true, SO_REUSEADDR will be used when binding the port. This allows binding to wildcard IPs like 0.0.0.0 and specific IPs in parallel, and it avoids waiting for the kernel to release sockets in TIME_WAIT state. [default=false]
      --permit-port-sharing                      If true, SO_REUSEPORT will be used when binding the port, which allows more than one instance to bind on the same address and port. [default=false]
      --policy-config-file string                File with descheduler policy configuration.
      --secure-port int                          The port on which to serve HTTPS with authentication and authorization. If 0, don't serve HTTPS at all. (default 10258)
      --skip-headers                             If true, avoid header prefixes in the log messages (DEPRECATED: will be removed in a future release, see https://github.com/kubernetes/enhancements/tree/master/keps/sig-instrumentation/2845-deprecate-klog-specific-flags-in-k8s-components)
      --skip-log-headers                         If true, avoid headers when opening log files (DEPRECATED: will be removed in a future release, see https://github.com/kubernetes/enhancements/tree/master/keps/sig-instrumentation/2845-deprecate-klog-specific-flags-in-k8s-components)
      --stderrthreshold severity                 logs at or above this threshold go to stderr (default 2) (DEPRECATED: will be removed in a future release, see https://github.com/kubernetes/enhancements/tree/master/keps/sig-instrumentation/2845-deprecate-klog-specific-flags-in-k8s-components)
      --tls-cert-file string                     File containing the default x509 Certificate for HTTPS. (CA cert, if any, concatenated after server cert). If HTTPS serving is enabled, and --tls-cert-file and --tls-private-key-file are not provided, a self-signed certificate and key are generated for the public address and saved to the directory specified by --cert-dir.
      --tls-cipher-suites strings                Comma-separated list of cipher suites for the server. If omitted, the default Go cipher suites will be used. 
                                                 Preferred values: TLS_AES_128_GCM_SHA256, TLS_AES_256_GCM_SHA384, TLS_CHACHA20_POLY1305_SHA256, TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA, TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA, TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384, TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305, TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256, TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA, TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA, TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384, TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305, TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256, TLS_RSA_WITH_AES_128_CBC_SHA, TLS_RSA_WITH_AES_128_GCM_SHA256, TLS_RSA_WITH_AES_256_CBC_SHA, TLS_RSA_WITH_AES_256_GCM_SHA384. 
                                                 Insecure values: TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256, TLS_ECDHE_ECDSA_WITH_RC4_128_SHA, TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA, TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256, TLS_ECDHE_RSA_WITH_RC4_128_SHA, TLS_RSA_WITH_3DES_EDE_CBC_SHA, TLS_RSA_WITH_AES_128_CBC_SHA256, TLS_RSA_WITH_RC4_128_SHA.
      --tls-min-version string                   Minimum TLS version supported. Possible values: VersionTLS10, VersionTLS11, VersionTLS12, VersionTLS13
      --tls-private-key-file string              File containing the default x509 private key matching --tls-cert-file.
      --tls-sni-cert-key namedCertKey            A pair of x509 certificate and private key file paths, optionally suffixed with a list of domain patterns which are fully qualified domain names, possibly with prefixed wildcard segments. The domain patterns also allow IP addresses, but IPs should only be used if the apiserver has visibility to the IP address requested by a client. If no domain patterns are provided, the names of the certificate are extracted. Non-wildcard matches trump over wildcard matches, explicit domain patterns trump over extracted names. For multiple key/certificate pairs, use the --tls-sni-cert-key multiple times. Examples: "example.crt,example.key" or "foo.crt,foo.key:*.foo.com,foo.com". (default [])
  -v, --v Level                                  number for the log level verbosity
      --vmodule moduleSpec                       comma-separated list of pattern=N settings for file-filtered logging

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
      podLifeTime:
        maxPodLifeTimeSeconds: 604800 # pods run for a maximum of 7 days
```

### Balance Cluster By Node Memory Utilization
If your cluster has been running for a long period of time, you may find that the resource utilization is not very
balanced. The following two strategies can be used to rebalance your cluster based on `cpu`, `memory` 
or `number of pods`.

#### Balance high utilization nodes
Using `LowNodeUtilization`, descheduler will rebalance the cluster based on memory by evicting pods
from nodes with memory utilization over 70% to nodes with memory utilization below 20%.

```
apiVersion: "descheduler/v1alpha1"
kind: "DeschedulerPolicy"
strategies:
  "LowNodeUtilization":
    enabled: true
    params:
      nodeResourceUtilizationThresholds:
        thresholds:
          "memory": 20
        targetThresholds:
          "memory": 70
```

#### Balance low utilization nodes
Using `HighNodeUtilization`, descheduler will rebalance the cluster based on memory by evicting pods
from nodes with memory utilization lower than 20%. This should be use `NodeResourcesFit` with the `MostAllocated` scoring strategy based on these [doc](https://kubernetes.io/docs/reference/scheduling/config/#scheduling-plugins).
The evicted pods will be compacted into minimal set of nodes.

```
apiVersion: "descheduler/v1alpha1"
kind: "DeschedulerPolicy"
strategies:
  "HighNodeUtilization":
    enabled: true
    params:
      nodeResourceUtilizationThresholds:
        thresholds:
          "memory": 20
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
