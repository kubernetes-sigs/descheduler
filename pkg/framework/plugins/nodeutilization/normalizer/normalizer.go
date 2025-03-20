/*
Copyright 2025 The Kubernetes Authors.
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

package normalizer

import (
	"math"

	"golang.org/x/exp/constraints"
)

// Normalizer is a function that receives two values of the same type and
// return an object of a different type. An usage case can be a function
// that converts a memory usage from mb to % (the first argument would be
// the memory usage in mb and the second argument would be the total memory
// available in mb).
type Normalizer[V, N any] func(V, V) N

// Values is a map of values indexed by a comparable key. An example of this
// can be a list of resources indexed by a node name.
type Values[K comparable, V any] map[K]V

// Number is an interface that represents a number. Represents things we
// can do math operations on.
type Number interface {
	constraints.Integer | constraints.Float
}

// Normalize uses a Normalizer function to normalize a set of values. For
// example one may want to convert a set of memory usages from mb to %.
// This function receives a set of usages, a set of totals, and a Normalizer
// function. The function will return a map with the normalized values.
func Normalize[K comparable, V, N any](usages, totals Values[K, V], fn Normalizer[V, N]) map[K]N {
	result := Values[K, N]{}
	for key, value := range usages {
		total, ok := totals[key]
		if !ok {
			continue
		}
		result[key] = fn(value, total)
	}
	return result
}

// Replicate replicates the provide value for each key in the provided slice.
// Returns a map with the keys and the provided value.
func Replicate[K comparable, V any](keys []K, value V) map[K]V {
	result := map[K]V{}
	for _, key := range keys {
		result[key] = value
	}
	return result
}

// Clamp imposes minimum and maximum limits on a set of values. The function
// will return a set of values where each value is between the minimum and
// maximum values (included). Values below minimum are rounded up to the
// minimum value, and values above maximum are rounded down to the maximum
// value.
func Clamp[K comparable, N Number, V ~map[K]N](values V, minimum, maximum N) V {
	result := V{}
	for key := range values {
		value := values[key]
		value = N(math.Max(float64(value), float64(minimum)))
		value = N(math.Min(float64(value), float64(maximum)))
		result[key] = value
	}
	return result
}

// Map applies a function to each element of a map of values. Returns a new
// slice with the results of applying the function to each element.
func Map[K comparable, N Number, V ~map[K]N](items []V, fn func(V) V) []V {
	result := []V{}
	for _, item := range items {
		result = append(result, fn(item))
	}
	return result
}

// Negate converts the values of a map to their negated values.
func Negate[K comparable, N Number, V ~map[K]N](values V) V {
	result := V{}
	for key, value := range values {
		result[key] = -value
	}
	return result
}

// Round rounds the values of a map to the nearest integer. Calls math.Round on
// each value of the map.
func Round[K comparable, N Number, V ~map[K]N](values V) V {
	result := V{}
	for key, value := range values {
		result[key] = N(math.Round(float64(value)))
	}
	return result
}

// Sum sums up the values of two maps. Values are expected to be of Number
// type. Original values are preserved. If a key is present in one map but
// not in the other, the key is ignored.
func Sum[K comparable, N Number, V ~map[K]N](mapA, mapB V) V {
	result := V{}
	for name, value := range mapA {
		result[name] = value + mapB[name]
	}
	return result
}

// Average calculates the average of a set of values. This function receives
// a map of values and returns the average of all the values. Average expects
// the values to represent the same unit of measure. You can use this function
// after Normalizing the values.
func Average[J, K comparable, N Number, V ~map[J]N](values map[K]V) V {
	counter := map[J]int{}
	result := V{}
	for _, imap := range values {
		for name, value := range imap {
			result[name] += value
			counter[name]++
		}
	}

	for name := range result {
		result[name] /= N(counter[name])
	}

	return result
}
