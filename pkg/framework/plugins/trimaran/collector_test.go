/*
Copyright 2023 The Kubernetes Authors.

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

package trimaran

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/paypal/load-watcher/pkg/watcher"
)

var (
	defaultURL = "http://deadbeef:2020"

	defaultTrimaranSpec = &TrimaranSpec{WatcherAddress: &defaultURL}

	defaultWatcherResponse = watcher.WatcherMetrics{
		Window: watcher.Window{},
		Data: watcher.Data{
			NodeMetricsMap: map[string]watcher.NodeMetrics{
				"node-1": {
					Metrics: []watcher.Metric{
						{
							Type:     watcher.CPU,
							Operator: watcher.Average,
							Value:    80,
						},
						{
							Type:     watcher.CPU,
							Operator: watcher.Std,
							Value:    16,
						},
						{
							Type:     watcher.Memory,
							Operator: watcher.Average,
							Value:    25,
						},
						{
							Type:     watcher.Memory,
							Operator: watcher.Std,
							Value:    6.25,
						},
					},
				},
			},
		},
	}
)

func Test_collector_GetNodeMetrics(t *testing.T) {
	type args struct {
		nodeName string
	}
	tests := []struct {
		name        string
		nodeName    string
		response    watcher.WatcherMetrics
		wantMetrics []watcher.Metric
		wantWindow  *watcher.Window
	}{
		{
			name:        "default",
			nodeName:    "node-1",
			response:    defaultWatcherResponse,
			wantMetrics: defaultWatcherResponse.Data.NodeMetricsMap["node-1"].Metrics,
			wantWindow:  &watcher.Window{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(resp http.ResponseWriter, _ *http.Request) {
				bytes, _ := json.Marshal(tt.response)
				resp.Write(bytes)
			}))
			defer server.Close()

			c := NewCollector(&TrimaranSpec{WatcherAddress: &server.URL})
			metrics, window := c.GetNodeMetrics(tt.nodeName)
			if diff := cmp.Diff(tt.wantMetrics, metrics); len(diff) > 0 {
				t.Errorf("collector.GetNodeMetrics() failed, mismatch:\n%s", diff)
			}
			if diff := cmp.Diff(tt.wantWindow, window); len(diff) > 0 {
				t.Errorf("collector.GetNodeMetrics() failed, mismatch:\n%s", diff)
			}
		})
	}
}

func Test_collector_updateMetrics(t *testing.T) {
	tests := []struct {
		name     string
		response watcher.WatcherMetrics
		want     watcher.WatcherMetrics
		wantErr  error
	}{
		{
			name:     "default",
			response: defaultWatcherResponse,
			want:     defaultWatcherResponse,
			wantErr:  nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(resp http.ResponseWriter, _ *http.Request) {
				bytes, _ := json.Marshal(tt.response)
				resp.Write(bytes)
			}))
			defer server.Close()

			c := NewCollector(&TrimaranSpec{WatcherAddress: &server.URL})
			err := c.(*collector).updateMetrics()
			if diff := cmp.Diff(tt.wantErr, err); len(diff) > 0 {
				t.Errorf("collector.updateMetrics() failed, mismatch:\n%s", diff)
			}
			if diff := cmp.Diff(tt.want, c.(*collector).metrics); len(diff) > 0 {
				t.Errorf("collector.updateMetrics() failed, mismatch:\n%s", diff)
			}
		})
	}
}
