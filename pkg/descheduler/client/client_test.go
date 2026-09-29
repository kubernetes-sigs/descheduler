package client

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prometheus/common/config"
)

func TestCreatePrometheusClientWithTLSConfig(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dir := t.TempDir()
	caFile := filepath.Join(dir, "ca.crt")
	if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}

	prometheusClient, transport, err := CreatePrometheusClient(server.URL, "", &config.TLSConfig{CAFile: caFile})
	if err != nil {
		t.Fatalf("CreatePrometheusClient() error = %v", err)
	}

	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if transport.TLSClientConfig.ServerName != u.Hostname() {
		t.Errorf("TLS ServerName = %q, want %q", transport.TLSClientConfig.ServerName, u.Hostname())
	}
	if strings.Contains(transport.TLSClientConfig.ServerName, ":") {
		t.Errorf("TLS ServerName = %q, must not contain a port", transport.TLSClientConfig.ServerName)
	}

	request, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, _, err := prometheusClient.Do(context.Background(), request)
	if err != nil {
		t.Fatalf("request using Prometheus client failed: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Errorf("response status = %d, want %d", response.StatusCode, http.StatusOK)
	}
}
