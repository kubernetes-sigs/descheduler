/*
Copyright 2020 PayPal

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

package metricsprovider

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/paypal/load-watcher/pkg/watcher"
	log "github.com/sirupsen/logrus"
)

const (
	// SignalFX Request Params
	DefaultSignalFxAddress    = "https://api.signalfx.com"
	signalFxMetricsAPI        = "/v1/timeserieswindow"
	signalFxMetdataAPI        = "/v2/metrictimeseries"
	signalFxHostFilter        = "host:"
	signalFxClusterFilter     = "cluster:"
	signalFxHostNameSuffixKey = "SIGNALFX_HOST_NAME_SUFFIX"
	signalFxClusterName       = "SIGNALFX_CLUSTER_NAME"
	// SignalFX Query Params
	oneMinuteResolutionMs   = 60000
	cpuUtilizationMetric    = `sf_metric:"cpu.utilization"`
	memoryUtilizationMetric = `sf_metric:"memory.utilization"`
	AND                     = "AND"
	resultSetLimit          = "10000"

	// Miscellaneous
	httpClientTimeout = 55 * time.Second
)

type signalFxClient struct {
	client          http.Client
	authToken       string
	signalFxAddress string
	hostNameSuffix  string
	clusterName     string
}

func NewSignalFxClient(opts watcher.MetricsProviderOpts) (watcher.MetricsProviderClient, error) {
	if opts.Name != watcher.SignalFxClientName {
		return nil, fmt.Errorf("metric provider name should be %v, found %v", watcher.SignalFxClientName, opts.Name)
	}
	tlsConfig := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: opts.InsecureSkipVerify}, // TODO(aqadeer): Figure out a secure way to let users add SSL certs
	}
	hostNameSuffix, _ := os.LookupEnv(signalFxHostNameSuffixKey)
	clusterName, _ := os.LookupEnv(signalFxClusterName)
	var signalFxAddress, signalFxAuthToken = DefaultSignalFxAddress, ""
	if opts.Address != "" {
		signalFxAddress = opts.Address
	}
	if opts.AuthToken != "" {
		signalFxAuthToken = opts.AuthToken
	}
	if signalFxAuthToken == "" {
		log.Fatalf("No auth token found to connect with SignalFx server")
	}
	return signalFxClient{client: http.Client{
		Timeout:   httpClientTimeout,
		Transport: tlsConfig},
		authToken:       signalFxAuthToken,
		signalFxAddress: signalFxAddress,
		hostNameSuffix:  hostNameSuffix,
		clusterName:     clusterName}, nil
}

func (s signalFxClient) Name() string {
	return watcher.SignalFxClientName
}

func (s signalFxClient) FetchHostMetrics(host string, window *watcher.Window) ([]watcher.Metric, error) {
	log.Debugf("fetching metrics for host %v", host)
	var metrics []watcher.Metric
	hostFilter := signalFxHostFilter + host + s.hostNameSuffix
	clusterFilter := signalFxClusterFilter + s.clusterName
	for _, metric := range []string{cpuUtilizationMetric, memoryUtilizationMetric} {
		uri, err := s.buildMetricURL(hostFilter, clusterFilter, metric, window)
		if err != nil {
			return metrics, fmt.Errorf("received error when building metric URL: %v", err)
		}
		req, _ := http.NewRequest(http.MethodGet, uri.String(), nil)
		req.Header.Set("X-SF-Token", s.authToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := s.client.Do(req)
		if err != nil {
			return metrics, fmt.Errorf("received error in metric API call: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return metrics, fmt.Errorf("received status code: %v", resp.StatusCode)
		}
		var res interface{}
		err = json.NewDecoder(resp.Body).Decode(&res)
		if err != nil {
			return metrics, fmt.Errorf("received error in decoding resp: %v", err)
		}

		var fetchedMetric watcher.Metric
		addMetadata(&fetchedMetric, metric)
		fetchedMetric.Value, err = decodeMetricsPayload(res)
		if err != nil {
			return metrics, err
		}
		metrics = append(metrics, fetchedMetric)
	}
	return metrics, nil
}

func (s signalFxClient) FetchAllHostsMetrics(window *watcher.Window) (map[string][]watcher.Metric, error) {
	hostFilter := signalFxHostFilter + "*" + s.hostNameSuffix
	clusterFilter := signalFxClusterFilter + s.clusterName
	metrics := make(map[string][]watcher.Metric)
	for _, metric := range []string{cpuUtilizationMetric, memoryUtilizationMetric} {
		uri, err := s.buildMetricURL(hostFilter, clusterFilter, metric, window)
		if err != nil {
			return metrics, fmt.Errorf("received error when building metric URL: %v", err)
		}
		req := s.requestWithAuthToken(uri.String())
		metricResp, err := s.client.Do(req)
		if err != nil {
			return metrics, fmt.Errorf("received error in metric API call: %v", err)
		}
		defer metricResp.Body.Close()
		if metricResp.StatusCode != http.StatusOK {
			return metrics, fmt.Errorf("received status code for metric resp: %v", metricResp.StatusCode)
		}
		var metricPayload interface{}
		err = json.NewDecoder(metricResp.Body).Decode(&metricPayload)
		if err != nil {
			return metrics, fmt.Errorf("received error in decoding resp: %v", err)
		}

		uri, err = s.buildMetadataURL(hostFilter, clusterFilter, metric)
		if err != nil {
			return metrics, fmt.Errorf("received error when building metadata URL: %v", err)
		}
		req = s.requestWithAuthToken(uri.String())
		metadataResp, err := s.client.Do(req)
		if err != nil {
			return metrics, fmt.Errorf("received error in metadata API call: %v", err)
		}
		defer metadataResp.Body.Close()
		if metadataResp.StatusCode != http.StatusOK {
			return metrics, fmt.Errorf("received status code for metadata resp: %v", metadataResp.StatusCode)
		}
		var metadataPayload interface{}
		err = json.NewDecoder(metadataResp.Body).Decode(&metadataPayload)
		if err != nil {
			return metrics, fmt.Errorf("received error in decoding metadata payload: %v", err)
		}
		mappedMetrics, err := getMetricsFromPayloads(metricPayload, metadataPayload)
		if err != nil {
			return metrics, fmt.Errorf("received error in getting metrics from payload: %v", err)
		}
		for k, v := range mappedMetrics {
			addMetadata(&v, metric)
			metrics[k] = append(metrics[k], v)
		}
	}
	return metrics, nil
}

func (s signalFxClient) Health() (int, error) {
	return Ping(s.client, s.signalFxAddress)
}

func (s signalFxClient) requestWithAuthToken(uri string) *http.Request {
	req, _ := http.NewRequest(http.MethodGet, uri, nil)
	req.Header.Set("X-SF-Token", s.authToken)
	req.Header.Set("Content-Type", "application/json")
	return req
}

// Simple ping utility to a given URL
// Returns -1 if unhealthy, 0 if healthy along with error if any
func Ping(client http.Client, url string) (int, error) {
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return -1, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return -1, err
	}
	if resp.StatusCode != http.StatusOK {
		return -1, fmt.Errorf("received response code: %v", resp.StatusCode)
	}
	return 0, nil
}

func addMetadata(metric *watcher.Metric, metricType string) {
	metric.Operator = watcher.Average
	if metricType == cpuUtilizationMetric {
		metric.Name = cpuUtilizationMetric
		metric.Type = watcher.CPU
	} else {
		metric.Name = memoryUtilizationMetric
		metric.Type = watcher.Memory
	}
}

func (s signalFxClient) buildMetricURL(hostFilter string, clusterFilter string, metric string, window *watcher.Window) (uri *url.URL, err error) {
	uri, err = url.Parse(s.signalFxAddress + signalFxMetricsAPI)
	if err != nil {
		return nil, err
	}
	q := uri.Query()

	builder := strings.Builder{}
	builder.WriteString(hostFilter)
	builder.WriteString(fmt.Sprintf(" %v ", AND))
	builder.WriteString(clusterFilter)
	builder.WriteString(fmt.Sprintf(" %v ", AND))
	builder.WriteString(metric)
	q.Set("query", builder.String())
	q.Set("startMs", strconv.FormatInt(window.Start*1000, 10))
	q.Set("endMs", strconv.FormatInt(window.End*1000, 10))
	q.Set("resolution", strconv.Itoa(oneMinuteResolutionMs))
	uri.RawQuery = q.Encode()
	return
}

func (s signalFxClient) buildMetadataURL(host string, clusterFilter string, metric string) (uri *url.URL, err error) {
	uri, err = url.Parse(s.signalFxAddress + signalFxMetdataAPI)
	if err != nil {
		return nil, err
	}
	q := uri.Query()

	builder := strings.Builder{}
	builder.WriteString(host)
	builder.WriteString(fmt.Sprintf(" %v ", AND))
	builder.WriteString(clusterFilter)
	builder.WriteString(fmt.Sprintf(" %v ", AND))
	builder.WriteString(metric)
	q.Set("query", builder.String())
	q.Set("limit", resultSetLimit)
	uri.RawQuery = q.Encode()
	return
}

/**
Sample payload:
{
  "data": {
    "Ehql_bxBgAc": [
      [
        1600213380000,
        84.64246793530153
      ]
    ]
  },
  "errors": []
}
*/
func decodeMetricsPayload(payload interface{}) (float64, error) {
	var data interface{}
	data = payload.(map[string]interface{})["data"]
	if data == nil {
		return -1, errors.New("unexpected payload: missing data field")
	}
	keyMap, ok := data.(map[string]interface{})
	if !ok {
		return -1, errors.New("unable to deserialise data field")
	}

	var values []interface{}
	if len(keyMap) == 0 {
		return -1, errors.New("no values found")
	}
	for _, v := range keyMap {
		values, ok = v.([]interface{})
		if !ok {
			return -1, errors.New("unable to deserialise values")
		}
		break
	}
	if len(values) == 0 {
		return -1, errors.New("no metric value array could be decoded")
	}

	var timestampUtilisation []interface{}
	// Choose the latest window out of multiple values returned
	timestampUtilisation, ok = values[len(values)-1].([]interface{})
	if !ok {
		return -1, errors.New("unable to deserialise metric values")
	}
	return timestampUtilisation[1].(float64), nil
}

/**
Sample metricData payload:
{
  "data": {
    "Ehql_bxBgAc": [
      [
        1600213380000,
        84.64246793530153
      ]
    ],
    "EuXgJm7BkAA": [
	  [
		1614634260000,
		5.450946379084264
     ]
    ],
    ....
    ....
  },
  "errors": []
}

https://dev.splunk.com/observability/reference/api/metrics_metadata/latest#endpoint-retrieve-metric-timeseries-metadata
Sample metaData payload:
{
    "count": 5,
    "partialCount": false,
    "results": [
        {
            "active": true,
            "created": 1614534848000,
            "creator": null,
            "dimensions": {
                "host": "test.dev.com",
                "sf_metric": null
            },
        "id": "EvVH6P7BgAA",
		"lastUpdated": 0,
		"lastUpdatedBy": null,
		"metric": "cpu.utilization"
       },
       ....
       ....
	]
}
*/
func getMetricsFromPayloads(metricData interface{}, metadata interface{}) (map[string]watcher.Metric, error) {
	keyHostMap := make(map[string]string)
	hostMetricMap := make(map[string]watcher.Metric)
	if _, ok := metadata.(map[string]interface{}); !ok {
		return hostMetricMap, fmt.Errorf("type conversion failed, found %T", metadata)
	}
	results := metadata.(map[string]interface{})["results"]
	if results == nil {
		return hostMetricMap, errors.New("unexpected payload: missing results field")
	}

	for _, v := range results.([]interface{}) {
		_, ok := v.(map[string]interface{})
		if !ok {
			log.Errorf("type conversion failed, found %T", v)
			continue
		}
		id := v.(map[string]interface{})["id"]
		if id == nil {
			log.Errorf("id not found in %v", v)
			continue
		}
		_, ok = id.(string)
		if !ok {
			log.Errorf("id not expected type string, found %T", id)
			continue
		}
		dimensions := v.(map[string]interface{})["dimensions"]
		if dimensions == nil {
			log.Errorf("no dimensions found in %v", v)
			continue
		}
		_, ok = dimensions.(map[string]interface{})
		if !ok {
			log.Errorf("type conversion failed, found %T", dimensions)
			continue
		}
		host := dimensions.(map[string]interface{})["host"]
		if host == nil {
			log.Errorf("no host found in %v", dimensions)
			continue
		}
		if _, ok := host.(string); !ok {
			log.Errorf("host not expected type string, found %T", host)
		}

		keyHostMap[id.(string)] = extractHostName(host.(string))
	}

	var data interface{}
	data = metricData.(map[string]interface{})["data"]
	if data == nil {
		return hostMetricMap, errors.New("unexpected payload: missing data field")
	}
	keyMetricMap, ok := data.(map[string]interface{})
	if !ok {
		return hostMetricMap, errors.New("unable to deserialise data field")
	}
	for key, metric := range keyMetricMap {
		if _, ok := keyHostMap[key]; !ok {
			log.Errorf("no metadata found for key %v", key)
			continue
		}
		values, ok := metric.([]interface{})
		if !ok {
			log.Errorf("unable to deserialise values for key %v", key)
			continue
		}
		if len(values) == 0 {
			log.Errorf("no metric value array could be decoded for key %v", key)
			continue
		}
		// Find the average across returned values per 1 minute resolution
		var sum float64
		var count float64
		for _, value := range values {
			var timestampUtilisation []interface{}
			timestampUtilisation, ok = value.([]interface{})
			if !ok || len(timestampUtilisation) < 2 {
				log.Errorf("unable to deserialise metric values for key %v", key)
				continue
			}
			if _, ok := timestampUtilisation[1].(float64); !ok {
				log.Errorf("unable to typecast value to float64: %v of type %T", timestampUtilisation, timestampUtilisation)
			}
			sum += timestampUtilisation[1].(float64)
			count += 1
		}

		fetchedMetric := watcher.Metric{Value: sum / count}
		hostMetricMap[keyHostMap[key]] = fetchedMetric
	}

	return hostMetricMap, nil
}

// This function checks and extracts node name from its FQDN if present
// It assumes that node names themselves don't contain "."
// Example: alpha.dev.k8s.com is returned as alpha
func extractHostName(fqdn string) string {
	index := strings.Index(fqdn, ".")
	if index != -1 {
		return fqdn[:index]
	}
	return fqdn
}
