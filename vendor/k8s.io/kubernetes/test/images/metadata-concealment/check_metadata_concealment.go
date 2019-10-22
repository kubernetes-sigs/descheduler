/*
Copyright 2017 The Kubernetes Authors.

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

package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
)

var (
	successEndpoints = []string{
		// Discovery
		"http://169.254.169.254",
		"http://metadata.google.internal",
		"http://169.254.169.254/",
		"http://metadata.google.internal/",
		"http://metadata.google.internal/0.1",
		"http://metadata.google.internal/0.1/",
		"http://metadata.google.internal/computeMetadata",
		"http://metadata.google.internal/computeMetadata/v1",
		// Allowed API versions.
		"http://metadata.google.internal/computeMetadata/v1/",
		// Service account token endpoints.
		"http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token",
		// Permitted recursive query to SA endpoint.
		"http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/?recursive=true",
		// Known query params.
		"http://metadata.google.internal/computeMetadata/v1/instance/tags?alt=text",
		"http://metadata.google.internal/computeMetadata/v1/instance/tags?wait_for_change=false",
		"http://metadata.google.internal/computeMetadata/v1/instance/tags?wait_for_change=true&timeout_sec=0",
		"http://metadata.google.internal/computeMetadata/v1/instance/tags?wait_for_change=true&last_etag=d34db33f",
	}
	legacySuccessEndpoints = []string{
		// Discovery
		"http://metadata.google.internal/0.1/meta-data",
		"http://metadata.google.internal/computeMetadata/v1beta1",
		// Allowed API versions.
		"http://metadata.google.internal/0.1/meta-data/",
		"http://metadata.google.internal/computeMetadata/v1beta1/",
		// Service account token endpoints.
		"http://metadata.google.internal/0.1/meta-data/service-accounts/default/acquire",
		"http://metadata.google.internal/computeMetadata/v1beta1/instance/service-accounts/default/token",
		// Known query params.
		"http://metadata.google.internal/0.1/meta-data/service-accounts/default/acquire?scopes",
	}
	noKubeEnvEndpoints = []string{
		// Check that these don't get a recursive result.
		"http://metadata.google.internal/computeMetadata/v1/instance/?recursive%3Dtrue",   // urlencoded
		"http://metadata.google.internal/computeMetadata/v1/instance/?re%08ecursive=true", // backspaced
	}
	failureEndpoints = []string{
		// Other API versions.
		"http://metadata.google.internal/0.2/",
		"http://metadata.google.internal/computeMetadata/v2/",
		// kube-env.
		"http://metadata.google.internal/0.1/meta-data/attributes/kube-env",
		"http://metadata.google.internal/computeMetadata/v1beta1/instance/attributes/kube-env",
		"http://metadata.google.internal/computeMetadata/v1/instance/attributes/kube-env",
		// VM identity.
		"http://metadata.google.internal/0.1/meta-data/service-accounts/default/identity",
		"http://metadata.google.internal/computeMetadata/v1beta1/instance/service-accounts/default/identity",
		"http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/identity",
		// Forbidden recursive queries.
		"http://metadata.google.internal/computeMetadata/v1/instance/?recursive=true",
		"http://metadata.google.internal/computeMetadata/v1/instance/?%72%65%63%75%72%73%69%76%65=true", // url-encoded
		// Unknown query param key.
		"http://metadata.google.internal/computeMetadata/v1/instance/?something=else",
		"http://metadata.google.internal/computeMetadata/v1/instance/?unknown",
		// Other.
		"http://metadata.google.internal/computeMetadata/v1/instance/attributes//kube-env",
		"http://metadata.google.internal/computeMetadata/v1/instance/attributes/../attributes/kube-env",
		"http://metadata.google.internal/computeMetadata/v1/instance/service-accounts//default/identity",
		"http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/../service-accounts/default/identity",
	}
)

func main() {
	success := 0
	h := map[string][]string{
		"Metadata-Flavor": {"Google"},
	}
	for _, e := range successEndpoints {
		if err := checkURL(e, h, 200, "", ""); err != nil {
			log.Printf("Wrong response for %v: %v", e, err)
			success = 1
		}
	}
	for _, e := range noKubeEnvEndpoints {
		if err := checkURL(e, h, 403, "", "kube-env"); err != nil {
			log.Printf("Wrong response for %v: %v", e, err)
			success = 1
		}
	}
	for _, e := range failureEndpoints {
		if err := checkURL(e, h, 403, "", ""); err != nil {
			log.Printf("Wrong response for %v: %v", e, err)
			success = 1
		}
	}

	legacyEndpointExpectedStatus := 200
	if err := checkURL("http://metadata.google.internal/computeMetadata/v1/instance/attributes/disable-legacy-endpoints", h, 200, "true", ""); err == nil {
		// If `disable-legacy-endpoints` is set to true, queries to unconcealed legacy endpoints will return a 403.
		legacyEndpointExpectedStatus = 403
	}
	for _, e := range legacySuccessEndpoints {
		if err := checkURL(e, h, legacyEndpointExpectedStatus, "", ""); err != nil {
			log.Printf("Wrong response for %v: %v", e, err)
			success = 1
		}
	}

	xForwardedForHeader := map[string][]string{
		"X-Forwarded-For": {"Somebody-somewhere"},
	}
	// Check that success endpoints fail if X-Forwarded-For is present.
	for _, e := range successEndpoints {
		if err := checkURL(e, xForwardedForHeader, 403, "", ""); err != nil {
			log.Printf("Wrong response for %v with X-Forwarded-For: %v", e, err)
			success = 1
		}
	}
	os.Exit(success)
}

// Checks that a URL with the given headers returns the right code.
// If expectedToContain is non-empty, checks that the body contains expectedToContain.
// Similarly, if expectedToNotContain is non-empty, checks that the body doesn't contain expectedToNotContain.
func checkURL(url string, header http.Header, expectedStatus int, expectedToContain, expectedToNotContain string) error {
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header = header
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != expectedStatus {
		return fmt.Errorf("unexpected response: got %d, want %d", resp.StatusCode, expectedStatus)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if expectedToContain != "" {
		matched, err := regexp.Match(expectedToContain, body)
		if err != nil {
			return err
		}
		if !matched {
			return fmt.Errorf("body didn't contain %q: got %v", expectedToContain, string(body))
		}
	}
	if expectedToNotContain != "" {
		matched, err := regexp.Match(expectedToNotContain, body)
		if err != nil {
			return err
		}
		if matched {
			return fmt.Errorf("body incorrectly contained %q: got %v", expectedToNotContain, string(body))
		}
	}
	return nil
}
