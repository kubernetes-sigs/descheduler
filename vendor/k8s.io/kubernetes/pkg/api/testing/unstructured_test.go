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

package testing

import (
	"math/rand"
	"reflect"
	"sort"
	"testing"

	fuzz "github.com/google/gofuzz"
	jsoniter "github.com/json-iterator/go"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metaunstruct "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/kubernetes/pkg/api/legacyscheme"
	"k8s.io/kubernetes/pkg/api/testapi"
	api "k8s.io/kubernetes/pkg/apis/core"
)

func doRoundTrip(t *testing.T, internalVersion schema.GroupVersion, externalVersion schema.GroupVersion, kind string) {
	// We do fuzzing on the internal version of the object, and only then
	// convert to the external version. This is because custom fuzzing
	// function are only supported for internal objects.
	internalObj, err := legacyscheme.Scheme.New(internalVersion.WithKind(kind))
	if err != nil {
		t.Fatalf("Couldn't create internal object %v: %v", kind, err)
	}
	seed := rand.Int63()
	fuzzer.FuzzerFor(FuzzerFuncs, rand.NewSource(seed), legacyscheme.Codecs).
		// We are explicitly overwriting custom fuzzing functions, to ensure
		// that InitContainers and their statuses are not generated. This is
		// because in thise test we are simply doing json operations, in which
		// those disappear.
		Funcs(
			func(s *api.PodSpec, c fuzz.Continue) {
				c.FuzzNoCustom(s)
				s.InitContainers = nil
			},
			func(s *api.PodStatus, c fuzz.Continue) {
				c.FuzzNoCustom(s)
				s.InitContainerStatuses = nil
			},
		).Fuzz(internalObj)

	item, err := legacyscheme.Scheme.New(externalVersion.WithKind(kind))
	if err != nil {
		t.Fatalf("Couldn't create external object %v: %v", kind, err)
	}
	if err := legacyscheme.Scheme.Convert(internalObj, item, nil); err != nil {
		t.Fatalf("Conversion for %v failed: %v", kind, err)
	}

	data, err := json.Marshal(item)
	if err != nil {
		t.Errorf("Error when marshaling object: %v", err)
		return
	}
	unstr := make(map[string]interface{})
	err = json.Unmarshal(data, &unstr)
	if err != nil {
		t.Errorf("Error when unmarshaling to unstructured: %v", err)
		return
	}

	data, err = json.Marshal(unstr)
	if err != nil {
		t.Errorf("Error when marshaling unstructured: %v", err)
		return
	}
	unmarshalledObj := reflect.New(reflect.TypeOf(item).Elem()).Interface()
	err = json.Unmarshal(data, &unmarshalledObj)
	if err != nil {
		t.Errorf("Error when unmarshaling to object: %v", err)
		return
	}
	if !apiequality.Semantic.DeepEqual(item, unmarshalledObj) {
		t.Errorf("Object changed during JSON operations, diff: %v", diff.ObjectReflectDiff(item, unmarshalledObj))
		return
	}

	newUnstr, err := runtime.DefaultUnstructuredConverter.ToUnstructured(item)
	if err != nil {
		t.Errorf("ToUnstructured failed: %v", err)
		return
	}

	newObj := reflect.New(reflect.TypeOf(item).Elem()).Interface().(runtime.Object)
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(newUnstr, newObj)
	if err != nil {
		t.Errorf("FromUnstructured failed: %v", err)
		return
	}

	if !apiequality.Semantic.DeepEqual(item, newObj) {
		t.Errorf("Object changed, diff: %v", diff.ObjectReflectDiff(item, newObj))
	}
}

func TestRoundTrip(t *testing.T) {
	for groupKey, group := range testapi.Groups {
		for kind := range legacyscheme.Scheme.KnownTypes(*group.GroupVersion()) {
			if nonRoundTrippableTypes.Has(kind) {
				continue
			}
			t.Logf("Testing: %v in %v", kind, groupKey)
			for i := 0; i < 50; i++ {
				doRoundTrip(t, schema.GroupVersion{Group: groupKey, Version: runtime.APIVersionInternal}, *group.GroupVersion(), kind)
				if t.Failed() {
					break
				}
			}
		}
	}
}

func TestRoundTripWithEmptyCreationTimestamp(t *testing.T) {
	for groupKey, group := range testapi.Groups {
		for kind := range legacyscheme.Scheme.KnownTypes(*group.GroupVersion()) {
			if nonRoundTrippableTypes.Has(kind) {
				continue
			}
			item, err := legacyscheme.Scheme.New(group.GroupVersion().WithKind(kind))
			if err != nil {
				t.Fatalf("Couldn't create external object %v: %v", kind, err)
			}
			t.Logf("Testing: %v in %v", kind, groupKey)

			unstrBody, err := runtime.DefaultUnstructuredConverter.ToUnstructured(item)
			if err != nil {
				t.Fatalf("ToUnstructured failed: %v", err)
			}

			unstructObj := &metaunstruct.Unstructured{}
			unstructObj.Object = unstrBody

			if meta, err := meta.Accessor(unstructObj); err == nil {
				meta.SetCreationTimestamp(metav1.Time{})
			} else {
				t.Fatalf("Unable to set creation timestamp: %v", err)
			}

			// attempt to re-convert unstructured object - conversion should not fail
			// based on empty metadata fields, such as creationTimestamp
			newObj := reflect.New(reflect.TypeOf(item).Elem()).Interface().(runtime.Object)
			err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructObj.Object, newObj)
			if err != nil {
				t.Fatalf("FromUnstructured failed: %v", err)
			}
		}
	}
}

func BenchmarkToUnstructured(b *testing.B) {
	items := benchmarkItems(b)
	size := len(items)
	convertor := runtime.DefaultUnstructuredConverter
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		unstr, err := convertor.ToUnstructured(&items[i%size])
		if err != nil || unstr == nil {
			b.Fatalf("unexpected error: %v", err)
		}
	}
	b.StopTimer()
}

func BenchmarkFromUnstructured(b *testing.B) {
	items := benchmarkItems(b)
	convertor := runtime.DefaultUnstructuredConverter
	var unstr []map[string]interface{}
	for i := range items {
		item, err := convertor.ToUnstructured(&items[i])
		if err != nil || item == nil {
			b.Fatalf("unexpected error: %v", err)
		}
		unstr = append(unstr, item)
	}
	size := len(items)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		obj := v1.Pod{}
		if err := convertor.FromUnstructured(unstr[i%size], &obj); err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
	}
	b.StopTimer()
}

func BenchmarkToUnstructuredViaJSON(b *testing.B) {
	items := benchmarkItems(b)
	var data [][]byte
	for i := range items {
		item, err := json.Marshal(&items[i])
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
		data = append(data, item)
	}
	size := len(items)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		unstr := map[string]interface{}{}
		if err := json.Unmarshal(data[i%size], &unstr); err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
	}
	b.StopTimer()
}

func BenchmarkFromUnstructuredViaJSON(b *testing.B) {
	items := benchmarkItems(b)
	var unstr []map[string]interface{}
	for i := range items {
		data, err := json.Marshal(&items[i])
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
		item := map[string]interface{}{}
		if err := json.Unmarshal(data, &item); err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
		unstr = append(unstr, item)
	}
	size := len(items)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		item, err := json.Marshal(unstr[i%size])
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
		obj := v1.Pod{}
		if err := json.Unmarshal(item, &obj); err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
	}
	b.StopTimer()
}

func BenchmarkToUnstructuredViaJSONIter(b *testing.B) {
	items := benchmarkItems(b)
	size := len(items)
	var keys []string
	for k := range jsonIterConfig {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, name := range keys {
		c := jsonIterConfig[name]
		b.Run(name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				data, err := c.Marshal(&items[i%size])
				if err != nil {
					b.Fatalf("unexpected error: %v", err)
				}
				unstr := map[string]interface{}{}
				if err := c.Unmarshal(data, &unstr); err != nil {
					b.Fatalf("unexpected error: %v", err)
				}
			}
			b.StopTimer()
		})
	}
}

var jsonIterConfig = map[string]jsoniter.API{
	"default": jsoniter.ConfigDefault,
	"fastest": jsoniter.ConfigFastest,
	"compat":  jsoniter.ConfigCompatibleWithStandardLibrary,
}

func BenchmarkFromUnstructuredViaJSONIter(b *testing.B) {
	items := benchmarkItems(b)
	var unstr []map[string]interface{}
	for i := range items {
		data, err := jsoniter.Marshal(&items[i])
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
		item := map[string]interface{}{}
		if err := json.Unmarshal(data, &item); err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
		unstr = append(unstr, item)
	}
	size := len(items)
	var keys []string
	for k := range jsonIterConfig {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, name := range keys {
		c := jsonIterConfig[name]
		b.Run(name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				item, err := c.Marshal(unstr[i%size])
				if err != nil {
					b.Fatalf("unexpected error: %v", err)
				}
				obj := v1.Pod{}
				if err := c.Unmarshal(item, &obj); err != nil {
					b.Fatalf("unexpected error: %v", err)
				}
			}
			b.StopTimer()
		})
	}
}
