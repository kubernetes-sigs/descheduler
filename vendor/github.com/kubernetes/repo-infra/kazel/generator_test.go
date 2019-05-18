/*
Copyright 2018 The Kubernetes Authors.

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
	"reflect"
	"testing"
)

func TestExtractTags(t *testing.T) {
	requestedTags := map[string]bool{
		"foo-gen":                    true,
		"baz-gen":                    true,
		"quux-gen:with-extra@things": true,
	}
	var testCases = []struct {
		src  string
		want map[string][]string
	}{
		{
			src:  "// +k8s:foo-gen=a,b\n",
			want: map[string][]string{"foo-gen": {"a", "b"}},
		},
		{
			src:  "// +k8s:bar-gen=a,b\n",
			want: map[string][]string{},
		},
		{
			src:  "// +k8s:quux-gen=true\n",
			want: map[string][]string{},
		},
		{
			src:  "// +k8s:quux-gen:with-extra@things=123\n",
			want: map[string][]string{"quux-gen:with-extra@things": {"123"}},
		},
		{
			src: `/*
This is a header.
*/
// +k8s:foo-gen=first
// +k8s:bar-gen=true
// +build linux

// +k8s:baz-gen=1,2,a
// +k8s:baz-gen=b

// k8s:foo-gen=not-this-one
// commenting out this one too  +k8s:foo-gen=disabled
// +k8s:foo-gen=ignore this one too

// Let's repeat one!
// +k8s:baz-gen=b
// +k8s:foo-gen=last

import "some package"
`,
			want: map[string][]string{
				"foo-gen": {"first", "last"},
				"baz-gen": {"1", "2", "a", "b", "b"},
			},
		},
	}

	for _, testCase := range testCases {
		result := extractTags([]byte(testCase.src), requestedTags)
		if !reflect.DeepEqual(result, testCase.want) {
			t.Errorf("extractTags(%v) = %v; want %v", testCase.src, result, testCase.want)
		}
	}
}

func TestFlattened(t *testing.T) {
	m := generatorTagsMap{
		"foo-gen": {
			"a": {
				"pkg/one": true,
				"pkg/two": true,
			},
		},
		"bar-gen": {
			"true": {
				"pkg/one":   true,
				"pkg/three": true,
				// also test sorting - this should end up at the front of the slice
				"a/pkg": true,
			},
			"false": {
				"pkg/one": true,
			},
		},
	}

	want := map[string]map[string][]string{
		"foo-gen": {
			"a": {"pkg/one", "pkg/two"},
		},
		"bar-gen": {
			"true":  {"a/pkg", "pkg/one", "pkg/three"},
			"false": {"pkg/one"},
		},
	}

	result := flattened(m)
	if !reflect.DeepEqual(result, want) {
		t.Errorf("flattened(%v) = %v; want %v", m, result, want)
	}

}
