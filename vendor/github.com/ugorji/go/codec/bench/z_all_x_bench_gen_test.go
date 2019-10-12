// +build alltests
// +build go1.7
// +build x
// +build generated

// Copyright (c) 2012-2018 Ugorji Nwoke. All rights reserved.
// Use of this source code is governed by a MIT license found in the LICENSE file.

package codec

// see notes in z_all_bench_test.go

import "testing"

func benchmarkCodecXGenGroup(t *testing.B) {
	benchmarkDivider()
	t.Run("Benchmark__Msgpack____Encode", Benchmark__Msgpack____Encode)
	t.Run("Benchmark__Binc_______Encode", Benchmark__Binc_______Encode)
	t.Run("Benchmark__Simple_____Encode", Benchmark__Simple_____Encode)
	t.Run("Benchmark__Cbor_______Encode", Benchmark__Cbor_______Encode)
	t.Run("Benchmark__Json_______Encode", Benchmark__Json_______Encode)
	t.Run("Benchmark__Msgp_______Encode", Benchmark__Msgp_______Encode)
	t.Run("Benchmark__Easyjson___Encode", Benchmark__Easyjson___Encode)
	t.Run("Benchmark__Ffjson_____Encode", Benchmark__Ffjson_____Encode)

	benchmarkDivider()
	t.Run("Benchmark__Msgpack____Decode", Benchmark__Msgpack____Decode)
	t.Run("Benchmark__Binc_______Decode", Benchmark__Binc_______Decode)
	t.Run("Benchmark__Simple_____Decode", Benchmark__Simple_____Decode)
	t.Run("Benchmark__Cbor_______Decode", Benchmark__Cbor_______Decode)
	t.Run("Benchmark__Json_______Decode", Benchmark__Json_______Decode)
	t.Run("Benchmark__Msgp_______Decode", Benchmark__Msgp_______Decode)
	t.Run("Benchmark__Easyjson___Decode", Benchmark__Easyjson___Decode)
	t.Run("Benchmark__Ffjson_____Decode", Benchmark__Ffjson_____Decode)
}

func BenchmarkCodecXGenSuite(t *testing.B) { benchmarkSuite(t, benchmarkCodecXGenGroup) }
