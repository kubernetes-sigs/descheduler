/*
Copyright 2015 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package kubeletconfig

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HairpinMode denotes how the kubelet should configure networking to handle
// hairpin packets.
type HairpinMode string

// Enum settings for different ways to handle hairpin packets.
const (
	// Set the hairpin flag on the veth of containers in the respective
	// container runtime.
	HairpinVeth = "hairpin-veth"
	// Make the container bridge promiscuous. This will force it to accept
	// hairpin packets, even if the flag isn't set on ports of the bridge.
	PromiscuousBridge = "promiscuous-bridge"
	// Neither of the above. If the kubelet is started in this hairpin mode
	// and kube-proxy is running in iptables mode, hairpin packets will be
	// dropped by the container bridge.
	HairpinNone = "none"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// A configuration field should go in KubeletFlags instead of KubeletConfiguration if
// its value cannot be safely shared between nodes at the same time (e.g. a hostname)
// In general, please try to avoid adding flags or configuration fields,
// we already have a confusingly large amount of them.
type KubeletConfiguration struct {
	metav1.TypeMeta

	// Only used for dynamic configuration.
	// The length of the trial period for this configuration. This configuration will become the last-known-good after this duration.
	ConfigTrialDuration *metav1.Duration
	// podManifestPath is the path to the directory containing pod manifests to
	// run, or the path to a single manifest file
	PodManifestPath string
	// syncFrequency is the max period between synchronizing running
	// containers and config
	SyncFrequency metav1.Duration
	// fileCheckFrequency is the duration between checking config files for
	// new data
	FileCheckFrequency metav1.Duration
	// httpCheckFrequency is the duration between checking http for new data
	HTTPCheckFrequency metav1.Duration
	// manifestURL is the URL for accessing the container manifest
	ManifestURL string
	// manifestURLHeader is the HTTP header to use when accessing the manifest
	// URL, with the key separated from the value with a ':', as in 'key:value'
	ManifestURLHeader map[string][]string
	// enableServer enables the Kubelet's server
	EnableServer bool
	// address is the IP address for the Kubelet to serve on (set to 0.0.0.0
	// for all interfaces)
	Address string
	// port is the port for the Kubelet to serve on.
	Port int32
	// readOnlyPort is the read-only port for the Kubelet to serve on with
	// no authentication/authorization (set to 0 to disable)
	ReadOnlyPort int32
	// tlsCertFile is the file containing x509 Certificate for HTTPS.  (CA cert,
	// if any, concatenated after server cert). If tlsCertFile and
	// tlsPrivateKeyFile are not provided, a self-signed certificate
	// and key are generated for the public address and saved to the directory
	// passed to certDir.
	TLSCertFile string
	// tlsPrivateKeyFile is the ile containing x509 private key matching
	// tlsCertFile.
	TLSPrivateKeyFile string
	// authentication specifies how requests to the Kubelet's server are authenticated
	Authentication KubeletAuthentication
	// authorization specifies how requests to the Kubelet's server are authorized
	Authorization KubeletAuthorization
	// allowPrivileged enables containers to request privileged mode.
	// Defaults to false.
	AllowPrivileged bool
	// hostNetworkSources is a comma-separated list of sources from which the
	// Kubelet allows pods to use of host network. Defaults to "*". Valid
	// options are "file", "http", "api", and "*" (all sources).
	HostNetworkSources []string
	// hostPIDSources is a comma-separated list of sources from which the
	// Kubelet allows pods to use the host pid namespace. Defaults to "*".
	HostPIDSources []string
	// hostIPCSources is a comma-separated list of sources from which the
	// Kubelet allows pods to use the host ipc namespace. Defaults to "*".
	HostIPCSources []string
	// registryPullQPS is the limit of registry pulls per second. If 0,
	// unlimited. Set to 0 for no limit. Defaults to 5.0.
	RegistryPullQPS int32
	// registryBurst is the maximum size of a bursty pulls, temporarily allows
	// pulls to burst to this number, while still not exceeding registryQps.
	// Only used if registryQPS > 0.
	RegistryBurst int32
	// eventRecordQPS is the maximum event creations per second. If 0, there
	// is no limit enforced.
	EventRecordQPS int32
	// eventBurst is the maximum size of a bursty event records, temporarily
	// allows event records to burst to this number, while still not exceeding
	// event-qps. Only used if eventQps > 0
	EventBurst int32
	// enableDebuggingHandlers enables server endpoints for log collection
	// and local running of containers and commands
	EnableDebuggingHandlers bool
	// enableContentionProfiling enables lock contention profiling, if enableDebuggingHandlers is true.
	EnableContentionProfiling bool
	// cAdvisorPort is the port of the localhost cAdvisor endpoint (set to 0 to disable)
	CAdvisorPort int32
	// healthzPort is the port of the localhost healthz endpoint (set to 0 to disable)
	HealthzPort int32
	// healthzBindAddress is the IP address for the healthz server to serve
	// on.
	HealthzBindAddress string
	// oomScoreAdj is The oom-score-adj value for kubelet process. Values
	// must be within the range [-1000, 1000].
	OOMScoreAdj int32
	// clusterDomain is the DNS domain for this cluster. If set, kubelet will
	// configure all containers to search this domain in addition to the
	// host's search domains.
	ClusterDomain string
	// clusterDNS is a list of IP address for a cluster DNS server.  If set,
	// kubelet will configure all containers to use this for DNS resolution
	// instead of the host's DNS servers
	ClusterDNS []string
	// streamingConnectionIdleTimeout is the maximum time a streaming connection
	// can be idle before the connection is automatically closed.
	StreamingConnectionIdleTimeout metav1.Duration
	// nodeStatusUpdateFrequency is the frequency that kubelet posts node
	// status to master. Note: be cautious when changing the constant, it
	// must work with nodeMonitorGracePeriod in nodecontroller.
	NodeStatusUpdateFrequency metav1.Duration
	// imageMinimumGCAge is the minimum age for an unused image before it is
	// garbage collected.
	ImageMinimumGCAge metav1.Duration
	// imageGCHighThresholdPercent is the percent of disk usage after which
	// image garbage collection is always run.
	ImageGCHighThresholdPercent int32
	// imageGCLowThresholdPercent is the percent of disk usage before which
	// image garbage collection is never run. Lowest disk usage to garbage
	// collect to.
	ImageGCLowThresholdPercent int32
	// How frequently to calculate and cache volume disk usage for all pods
	VolumeStatsAggPeriod metav1.Duration
	// KubeletCgroups is the absolute name of cgroups to isolate the kubelet in.
	// +optional
	KubeletCgroups string
	// Enable QoS based Cgroup hierarchy: top level cgroups for QoS Classes
	// And all Burstable and BestEffort pods are brought up under their
	// specific top level QoS cgroup.
	// +optional
	CgroupsPerQOS bool
	// driver that the kubelet uses to manipulate cgroups on the host (cgroupfs or systemd)
	// +optional
	CgroupDriver string
	// SystemCgroups is absolute name of cgroups in which to place
	// all non-kernel processes that are not already in a container. Empty
	// for no container. Rolling back the flag requires a reboot.
	// +optional
	SystemCgroups string
	// CgroupRoot is the root cgroup to use for pods.
	// If CgroupsPerQOS is enabled, this is the root of the QoS cgroup hierarchy.
	// +optional
	CgroupRoot string
	// CPUManagerPolicy is the name of the policy to use.
	CPUManagerPolicy string
	// CPU Manager reconciliation period.
	CPUManagerReconcilePeriod metav1.Duration
	// runtimeRequestTimeout is the timeout for all runtime requests except long running
	// requests - pull, logs, exec and attach.
	// +optional
	RuntimeRequestTimeout metav1.Duration
	// How should the kubelet configure the container bridge for hairpin packets.
	// Setting this flag allows endpoints in a Service to loadbalance back to
	// themselves if they should try to access their own Service. Values:
	//   "promiscuous-bridge": make the container bridge promiscuous.
	//   "hairpin-veth":       set the hairpin flag on container veth interfaces.
	//   "none":               do nothing.
	// Generally, one must set --hairpin-mode=veth-flag to achieve hairpin NAT,
	// because promiscous-bridge assumes the existence of a container bridge named cbr0.
	HairpinMode string
	// maxPods is the number of pods that can run on this Kubelet.
	MaxPods int32
	// The CIDR to use for pod IP addresses, only used in standalone mode.
	// In cluster mode, this is obtained from the master.
	PodCIDR string
	// ResolverConfig is the resolver configuration file used as the basis
	// for the container DNS resolution configuration.
	ResolverConfig string
	// cpuCFSQuota is Enable CPU CFS quota enforcement for containers that
	// specify CPU limits
	CPUCFSQuota bool
	// maxOpenFiles is Number of files that can be opened by Kubelet process.
	MaxOpenFiles int64
	// contentType is contentType of requests sent to apiserver.
	ContentType string
	// kubeAPIQPS is the QPS to use while talking with kubernetes apiserver
	KubeAPIQPS int32
	// kubeAPIBurst is the burst to allow while talking with kubernetes
	// apiserver
	KubeAPIBurst int32
	// serializeImagePulls when enabled, tells the Kubelet to pull images one
	// at a time. We recommend *not* changing the default value on nodes that
	// run docker daemon with version  < 1.9 or an Aufs storage backend.
	// Issue #10959 has more details.
	SerializeImagePulls bool
	// Map of signal names to quantities that defines hard eviction thresholds. For example: {"memory.available": "300Mi"}.
	// +optional
	EvictionHard map[string]string
	// Map of signal names to quantities that defines soft eviction thresholds.  For example: {"memory.available": "300Mi"}.
	// +optional
	EvictionSoft map[string]string
	// Map of signal names to quantities that defines grace periods for each soft eviction signal. For example: {"memory.available": "30s"}.
	// +optional
	EvictionSoftGracePeriod map[string]string
	// Duration for which the kubelet has to wait before transitioning out of an eviction pressure condition.
	// +optional
	EvictionPressureTransitionPeriod metav1.Duration
	// Maximum allowed grace period (in seconds) to use when terminating pods in response to a soft eviction threshold being met.
	// +optional
	EvictionMaxPodGracePeriod int32
	// Map of signal names to quantities that defines minimum reclaims, which describe the minimum
	// amount of a given resource the kubelet will reclaim when performing a pod eviction while
	// that resource is under pressure. For example: {"imagefs.available": "2Gi"}
	// +optional
	EvictionMinimumReclaim map[string]string
	// Maximum number of pods per core. Cannot exceed MaxPods
	PodsPerCore int32
	// enableControllerAttachDetach enables the Attach/Detach controller to
	// manage attachment/detachment of volumes scheduled to this node, and
	// disables kubelet from executing any attach/detach operations
	EnableControllerAttachDetach bool
	// Default behaviour for kernel tuning
	ProtectKernelDefaults bool
	// If true, Kubelet ensures a set of iptables rules are present on host.
	// These rules will serve as utility for various components, e.g. kube-proxy.
	// The rules will be created based on IPTablesMasqueradeBit and IPTablesDropBit.
	MakeIPTablesUtilChains bool
	// iptablesMasqueradeBit is the bit of the iptables fwmark space to use for SNAT
	// Values must be within the range [0, 31].
	// Warning: Please match the value of corresponding parameter in kube-proxy
	// TODO: clean up IPTablesMasqueradeBit in kube-proxy
	IPTablesMasqueradeBit int32
	// iptablesDropBit is the bit of the iptables fwmark space to use for dropping packets. Kubelet will ensure iptables mark and drop rules.
	// Values must be within the range [0, 31]. Must be different from IPTablesMasqueradeBit
	IPTablesDropBit int32
	// featureGates is a map of feature names to bools that enable or disable alpha/experimental features.
	FeatureGates map[string]bool
	// Tells the Kubelet to fail to start if swap is enabled on the node.
	FailSwapOn bool

	/* following flags are meant for Node Allocatable */

	// A set of ResourceName=ResourceQuantity (e.g. cpu=200m,memory=150G) pairs
	// that describe resources reserved for non-kubernetes components.
	// Currently only cpu and memory are supported. [default=none]
	// See http://kubernetes.io/docs/user-guide/compute-resources for more detail.
	SystemReserved map[string]string
	// A set of ResourceName=ResourceQuantity (e.g. cpu=200m,memory=150G) pairs
	// that describe resources reserved for kubernetes system components.
	// Currently cpu, memory and local ephemeral storage for root file system are supported. [default=none]
	// See http://kubernetes.io/docs/user-guide/compute-resources for more detail.
	KubeReserved map[string]string
	// This flag helps kubelet identify absolute name of top level cgroup used to enforce `SystemReserved` compute resource reservation for OS system daemons.
	// Refer to [Node Allocatable](https://git.k8s.io/community/contributors/design-proposals/node/node-allocatable.md) doc for more information.
	SystemReservedCgroup string
	// This flag helps kubelet identify absolute name of top level cgroup used to enforce `KubeReserved` compute resource reservation for Kubernetes node system daemons.
	// Refer to [Node Allocatable](https://git.k8s.io/community/contributors/design-proposals/node/node-allocatable.md) doc for more information.
	KubeReservedCgroup string
	// This flag specifies the various Node Allocatable enforcements that Kubelet needs to perform.
	// This flag accepts a list of options. Acceptable options are `pods`, `system-reserved` & `kube-reserved`.
	// Refer to [Node Allocatable](https://git.k8s.io/community/contributors/design-proposals/node/node-allocatable.md) doc for more information.
	EnforceNodeAllocatable []string
}

type KubeletAuthorizationMode string

const (
	// KubeletAuthorizationModeAlwaysAllow authorizes all authenticated requests
	KubeletAuthorizationModeAlwaysAllow KubeletAuthorizationMode = "AlwaysAllow"
	// KubeletAuthorizationModeWebhook uses the SubjectAccessReview API to determine authorization
	KubeletAuthorizationModeWebhook KubeletAuthorizationMode = "Webhook"
)

type KubeletAuthorization struct {
	// mode is the authorization mode to apply to requests to the kubelet server.
	// Valid values are AlwaysAllow and Webhook.
	// Webhook mode uses the SubjectAccessReview API to determine authorization.
	Mode KubeletAuthorizationMode

	// webhook contains settings related to Webhook authorization.
	Webhook KubeletWebhookAuthorization
}

type KubeletWebhookAuthorization struct {
	// cacheAuthorizedTTL is the duration to cache 'authorized' responses from the webhook authorizer.
	CacheAuthorizedTTL metav1.Duration
	// cacheUnauthorizedTTL is the duration to cache 'unauthorized' responses from the webhook authorizer.
	CacheUnauthorizedTTL metav1.Duration
}

type KubeletAuthentication struct {
	// x509 contains settings related to x509 client certificate authentication
	X509 KubeletX509Authentication
	// webhook contains settings related to webhook bearer token authentication
	Webhook KubeletWebhookAuthentication
	// anonymous contains settings related to anonymous authentication
	Anonymous KubeletAnonymousAuthentication
}

type KubeletX509Authentication struct {
	// clientCAFile is the path to a PEM-encoded certificate bundle. If set, any request presenting a client certificate
	// signed by one of the authorities in the bundle is authenticated with a username corresponding to the CommonName,
	// and groups corresponding to the Organization in the client certificate.
	ClientCAFile string
}

type KubeletWebhookAuthentication struct {
	// enabled allows bearer token authentication backed by the tokenreviews.authentication.k8s.io API
	Enabled bool
	// cacheTTL enables caching of authentication results
	CacheTTL metav1.Duration
}

type KubeletAnonymousAuthentication struct {
	// enabled allows anonymous requests to the kubelet server.
	// Requests that are not rejected by another authentication method are treated as anonymous requests.
	// Anonymous requests have a username of system:anonymous, and a group name of system:unauthenticated.
	Enabled bool
}
