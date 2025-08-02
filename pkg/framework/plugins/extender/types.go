package extender

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ExternalDecisionArgs holds the arguments used to configure the ExternalDecision plugin.
// It defines how to connect to an external extender service and control plugin behavior.
type ExternalDecisionArgs struct {
	metav1.TypeMeta `json:",inline"`

	// Extender defines the external HTTP service that makes eviction decisions.
	Extender *ExternalExtender `json:"extender,omitempty"`
}

// +k8s:deepcopy-gen=true

// ExternalExtender contains configuration for calling an external HTTP service
// to decide whether a pod should be evicted.
type ExternalExtender struct {
	// URL prefix of the external service (e.g., "https://example.com/api/v1")
	URLPrefix string `json:"urlPrefix,omitempty"`

	// The HTTP verb (path) to append to URLPrefix for eviction decision (e.g., "evict")
	// The full URL will be: POST <urlPrefix>/<decisionVerb>
	DecisionVerb string `json:"decisionVerb,omitempty"`

	// If true, failures (timeout, network error, non-200 response) are ignored,
	// and the plugin proceeds as if eviction is allowed.
	// If false, failure behavior is determined by FailPolicy.
	Ignorable bool `json:"ignorable"`

	// Timeout for the HTTP request to the external service (e.g., "5s").
	// Required field.
	HTTPTimeout metav1.Duration `json:"httpTimeout"`

	// If true, enables HTTPS for communication with the external service.
	// Defaults to false.
	EnableHTTPS bool `json:"enableHTTPS,omitempty"`

	// TLS configuration for verifying the server's certificate.
	TLSConfig *ExtenderTLSConfig `json:"tlsConfig,omitempty"`

	// Defines the behavior when the extender call fails and ignorable=false.
	// Allowed values:
	// - "Allow": Proceed with eviction
	// - "Ignore": Skip this pod
	// - "Deny": Block eviction (default)
	// Optional; defaults to "Deny" if not specified.
	FailPolicy string `json:"failPolicy,omitempty"`
}

// +k8s:deepcopy-gen=true

// ExtenderTLSConfig holds the TLS settings for connecting to the extender service.
type ExtenderTLSConfig struct {
	// Path to the CA certificate file used to verify the server's certificate.
	// Optional.
	CACertFile string `json:"caCertFile,omitempty"`
}
