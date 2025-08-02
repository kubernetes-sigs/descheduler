package extender

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"net/http"
	"os"
)

// EvictionRequest represents the request body sent to the external extender.
type EvictionRequest struct {
	Pod  *v1.Pod  `json:"pod"`
	Node *v1.Node `json:"node"`
}

// EvictionResponse represents the response from the external extender.
type EvictionResponse struct {
	Allow  bool   `json:"allow"`
	Reason string `json:"reason,omitempty"`
	Error  string `json:"error,omitempty"`
}

// ExtenderClient
type ExtenderClient struct {
	baseURL    string // urlPrefix + decisionVerb
	httpClient *http.Client
	ignorable  bool
	failPolicy string
	logger     klog.Logger
}

// NewExtenderClient creates a new ExtenderClient from ExternalExtender config.
func NewExtenderClient(extenderConfig *ExternalExtender, logger klog.Logger) (*ExtenderClient, error) {
	if extenderConfig == nil {
		return nil, fmt.Errorf("extender config is nil")
	}
	if extenderConfig.URLPrefix == "" {
		return nil, fmt.Errorf("urlPrefix is required")
	}
	if extenderConfig.DecisionVerb == "" {
		return nil, fmt.Errorf("decisionVerb is required")
	}
	if extenderConfig.HTTPTimeout.Duration <= 0 {
		return nil, fmt.Errorf("httpTimeout must be > 0")
	}

	baseURL := fmt.Sprintf("%s/%s", trimSuffix(extenderConfig.URLPrefix, "/"), extenderConfig.DecisionVerb)

	var tlsConfig *tls.Config
	if extenderConfig.EnableHTTPS {
		var err error
		tlsConfig, err = buildTLSConfig(extenderConfig.TLSConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to build TLS config: %v", err)
		}
	}

	httpClient := &http.Client{
		Timeout: extenderConfig.HTTPTimeout.Duration,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	return &ExtenderClient{
		baseURL:    baseURL,
		httpClient: httpClient,
		ignorable:  extenderConfig.Ignorable,
		failPolicy: defaultIfEmpty(extenderConfig.FailPolicy, "Deny"),
		logger:     logger.WithValues("extender", baseURL),
	}, nil
}

// DecideEviction calls the external service to decide whether to evict the pod.
func (c *ExtenderClient) DecideEviction(ctx context.Context, pod *v1.Pod, node *v1.Node) (bool, string, error) {
	reqData := EvictionRequest{Pod: pod, Node: node}

	body, err := json.Marshal(reqData)
	if err != nil {
		return false, "", fmt.Errorf("failed to marshal request: %v", err)
	}

	httpRequest, err := http.NewRequestWithContext(ctx, "POST", c.baseURL, bytes.NewReader(body))
	if err != nil {
		return false, "", fmt.Errorf("failed to create request: %v", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")

	c.logger.V(4).Info("Calling external decision service", "url", c.baseURL, "pod", klog.KObj(pod))

	resp, err := c.httpClient.Do(httpRequest)
	if err != nil {
		errMsg := fmt.Sprintf("HTTP request failed: %v", err)
		c.logger.Error(err, "External decision service unreachable")
		if c.ignorable {
			c.logger.V(2).Info("ignorable=true, allowing eviction by default")
			return true, "extender unreachable (ignorable)", nil
		}
		return false, "", fmt.Errorf("%s", errMsg)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, "", fmt.Errorf("failed to read response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		errMsg := fmt.Sprintf("non-200 status code: %d, body: %s", resp.StatusCode, string(respBody))
		c.logger.Error(nil, "External decision service returned error", "statusCode", resp.StatusCode, "body", string(respBody))
		if c.ignorable {
			c.logger.V(2).Info("ignorable=true, allowing eviction by default")
			return true, "extender error (ignorable)", nil
		}
		return false, "", fmt.Errorf("%s", errMsg)
	}

	var extResp EvictionResponse
	if err := json.Unmarshal(respBody, &extResp); err != nil {
		return false, "", fmt.Errorf("failed to unmarshal response: %v, body: %s", err, string(respBody))
	}

	if extResp.Error != "" {
		c.logger.V(2).Info("External decision service returned logical error", "error", extResp.Error)
		if c.ignorable {
			return true, "extender logical error (ignorable)", nil
		}
		return false, extResp.Error, nil
	}

	c.logger.V(3).Info("External decision received", "allow", extResp.Allow, "reason", extResp.Reason)

	return extResp.Allow, extResp.Reason, nil
}

func trimSuffix(s, suffix string) string {
	if suffix == "" {
		return s
	}
	if s == suffix {
		return s
	}
	if len(s) > len(suffix) && s[len(s)-len(suffix):] == suffix {
		return s[:len(s)-len(suffix)]
	}
	return s
}

func defaultIfEmpty(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func buildTLSConfig(tlsConfig *ExtenderTLSConfig) (*tls.Config, error) {
	if tlsConfig == nil {
		return &tls.Config{InsecureSkipVerify: false}, nil
	}

	config := &tls.Config{InsecureSkipVerify: false}

	if tlsConfig.CACertFile != "" {
		caCert, err := os.ReadFile(tlsConfig.CACertFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA cert file: %v", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		config.RootCAs = caCertPool
	}

	return config, nil
}
