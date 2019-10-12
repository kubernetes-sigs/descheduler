// +build alltests
// +build go1.7
// +build !generated

// Copyright (c) 2012-2018 Ugorji Nwoke. All rights reserved.
// Use of this source code is governed by a MIT license found in the LICENSE file.

package codec

import "testing"

func benchmarkStdlibGroup(t *testing.B) {
	benchmarkDivider()
	t.Run("Benchmark__Std_Json___Encode", Benchmark__Std_Json___Encode)
	t.Run("Benchmark__Gob________Encode", Benchmark__Gob________Encode)
	// t.Run("Benchmark__Std_Xml____Encode", Benchmark__Std_Xml____Encode)
	benchmarkDivider()
	t.Run("Benchmark__Std_Json___Decode", Benchmark__Std_Json___Decode)
	t.Run("Benchmark__Gob________Decode", Benchmark__Gob________Decode)
	// t.Run("Benchmark__Std_Xml____Decode", Benchmark__Std_Xml____Decode)
}

func BenchmarkStdlibSuite(t *testing.B) { benchmarkSuite(t, benchmarkStdlibGroup) }
