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
	"testing"

	"github.com/bazelbuild/buildtools/build"
)

func TestAsExpr(t *testing.T) {
	var testCases = []struct {
		expr interface{}
		want string
	}{
		{42, "42"},
		{2.71828, "2.71828"},
		{2.718281828459045, "2.718281828459045"},
		{"a string", `"a string"`},
		// values should stay in specified order
		{[]int{4, 7, 2, 9, 21}, `[
    4,
    7,
    2,
    9,
    21,
]`},
		// keys should get sorted
		{map[int]string{1: "foo", 5: "baz", 3: "bar"}, `{
    1: "foo",
    3: "bar",
    5: "baz",
}`},
		// keys true and false should be sorted by their string representation
		{
			map[bool]map[string][]float64{
				true:  {"b": {2, 2.2}, "a": {1, 1.1, 1.11}},
				false: {"": {}},
			},
			`{
    false: {"": []},
    true: {
        "a": [
            1,
            1.1,
            1.11,
        ],
        "b": [
            2,
            2.2,
        ],
    },
}`},
	}

	for _, testCase := range testCases {
		result := build.FormatString(asExpr(testCase.expr))
		if result != testCase.want {
			t.Errorf("asExpr(%v) = %v; want %v", testCase.expr, result, testCase.want)
		}
	}
}
