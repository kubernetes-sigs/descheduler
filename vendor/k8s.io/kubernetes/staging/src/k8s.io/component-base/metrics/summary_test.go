/*
Copyright 2019 The Kubernetes Authors.

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

package metrics

import (
	"testing"

	"github.com/blang/semver"
	"github.com/stretchr/testify/assert"

	apimachineryversion "k8s.io/apimachinery/pkg/version"
)

func TestSummary(t *testing.T) {
	v115 := semver.MustParse("1.15.0")
	var tests = []struct {
		desc string
		SummaryOpts
		registryVersion     *semver.Version
		expectedMetricCount int
		expectedHelp        string
	}{
		{
			desc: "Test non deprecated",
			SummaryOpts: SummaryOpts{
				Namespace:      "namespace",
				Name:           "metric_test_name",
				Subsystem:      "subsystem",
				Help:           "summary help message",
				StabilityLevel: ALPHA,
			},
			registryVersion:     &v115,
			expectedMetricCount: 1,
			expectedHelp:        "[ALPHA] summary help message",
		},
		{
			desc: "Test deprecated",
			SummaryOpts: SummaryOpts{
				Namespace:         "namespace",
				Name:              "metric_test_name",
				Subsystem:         "subsystem",
				Help:              "summary help message",
				DeprecatedVersion: "1.15.0",
				StabilityLevel:    ALPHA,
			},
			registryVersion:     &v115,
			expectedMetricCount: 1,
			expectedHelp:        "[ALPHA] (Deprecated since 1.15.0) summary help message",
		},
		{
			desc: "Test hidden",
			SummaryOpts: SummaryOpts{
				Namespace:         "namespace",
				Name:              "metric_test_name",
				Subsystem:         "subsystem",
				Help:              "summary help message",
				DeprecatedVersion: "1.14.0",
			},
			registryVersion:     &v115,
			expectedMetricCount: 0,
			expectedHelp:        "summary help message",
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			registry := newKubeRegistry(apimachineryversion.Info{
				Major:      "1",
				Minor:      "15",
				GitVersion: "v1.15.0-alpha-1.12345",
			})
			c := NewSummary(&test.SummaryOpts)
			registry.MustRegister(c)

			ms, err := registry.Gather()
			assert.Equalf(t, test.expectedMetricCount, len(ms), "Got %v metrics, Want: %v metrics", len(ms), test.expectedMetricCount)
			assert.Nil(t, err, "Gather failed %v", err)

			for _, metric := range ms {
				assert.Equalf(t, test.expectedHelp, metric.GetHelp(), "Got %s as help message, want %s", metric.GetHelp(), test.expectedHelp)
			}

			// let's increment the counter and verify that the metric still works
			c.Observe(1)
			c.Observe(2)
			c.Observe(3)
			c.Observe(1.5)
			expected := 4
			ms, err = registry.Gather()
			assert.Nil(t, err, "Gather failed %v", err)

			for _, mf := range ms {
				for _, m := range mf.GetMetric() {
					assert.Equalf(t, expected, int(m.GetSummary().GetSampleCount()), "Got %v, want %v as the sample count", m.GetHistogram().GetSampleCount(), expected)
				}
			}
		})
	}
}

func TestSummaryVec(t *testing.T) {
	v115 := semver.MustParse("1.15.0")
	var tests = []struct {
		desc string
		SummaryOpts
		labels              []string
		registryVersion     *semver.Version
		expectedMetricCount int
		expectedHelp        string
	}{
		{
			desc: "Test non deprecated",
			SummaryOpts: SummaryOpts{
				Namespace: "namespace",
				Name:      "metric_test_name",
				Subsystem: "subsystem",
				Help:      "summary help message",
			},
			labels:              []string{"label_a", "label_b"},
			registryVersion:     &v115,
			expectedMetricCount: 1,
			expectedHelp:        "[ALPHA] summary help message",
		},
		{
			desc: "Test deprecated",
			SummaryOpts: SummaryOpts{
				Namespace:         "namespace",
				Name:              "metric_test_name",
				Subsystem:         "subsystem",
				Help:              "summary help message",
				DeprecatedVersion: "1.15.0",
			},
			labels:              []string{"label_a", "label_b"},
			registryVersion:     &v115,
			expectedMetricCount: 1,
			expectedHelp:        "[ALPHA] (Deprecated since 1.15.0) summary help message",
		},
		{
			desc: "Test hidden",
			SummaryOpts: SummaryOpts{
				Namespace:         "namespace",
				Name:              "metric_test_name",
				Subsystem:         "subsystem",
				Help:              "summary help message",
				DeprecatedVersion: "1.14.0",
			},
			labels:              []string{"label_a", "label_b"},
			registryVersion:     &v115,
			expectedMetricCount: 0,
			expectedHelp:        "summary help message",
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			registry := newKubeRegistry(apimachineryversion.Info{
				Major:      "1",
				Minor:      "15",
				GitVersion: "v1.15.0-alpha-1.12345",
			})
			c := NewSummaryVec(&test.SummaryOpts, test.labels)
			registry.MustRegister(c)
			c.WithLabelValues("1", "2").Observe(1.0)
			ms, err := registry.Gather()
			assert.Equalf(t, test.expectedMetricCount, len(ms), "Got %v metrics, Want: %v metrics", len(ms), test.expectedMetricCount)
			assert.Nil(t, err, "Gather failed %v", err)

			for _, metric := range ms {
				assert.Equalf(t, test.expectedHelp, metric.GetHelp(), "Got %s as help message, want %s", metric.GetHelp(), test.expectedHelp)
			}

			// let's increment the counter and verify that the metric still works
			c.WithLabelValues("1", "3").Observe(1.0)
			c.WithLabelValues("2", "3").Observe(1.0)
			ms, err = registry.Gather()
			assert.Nil(t, err, "Gather failed %v", err)

			for _, mf := range ms {
				assert.Equalf(t, 3, len(mf.GetMetric()), "Got %v metrics, wanted 2 as the count", len(mf.GetMetric()))
				for _, m := range mf.GetMetric() {
					assert.Equalf(t, uint64(1), m.GetSummary().GetSampleCount(), "Got %v metrics, wanted 1 as the summary sample count", m.GetSummary().GetSampleCount())
				}
			}
		})
	}
}
