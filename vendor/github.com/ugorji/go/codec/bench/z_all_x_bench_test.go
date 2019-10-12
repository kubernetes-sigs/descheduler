// +build alltests
// +build go1.7
// +build x
// +build !generated

// Copyright (c) 2012-2018 Ugorji Nwoke. All rights reserved.
// Use of this source code is governed by a MIT license found in the LICENSE file.

package codec

// see notes in z_all_bench_test.go

import "testing"

// Note: The following cannot parse TestStruc effectively,
// even with changes to remove arrays and minimize integer size to fit into int64 space.
//
// So we exclude them, listed below:
// encode: gcbor, xdr
// decode: gcbor, vmsgpack, xdr, sereal

func benchmarkXGroup(t *testing.B) {
	benchmarkDivider()
	t.Run("Benchmark__JsonIter___Encode", Benchmark__JsonIter___Encode)
	t.Run("Benchmark__Bson_______Encode", Benchmark__Bson_______Encode)
	t.Run("Benchmark__Mgobson____Encode", Benchmark__Mgobson____Encode)
	t.Run("Benchmark__VMsgpack___Encode", Benchmark__VMsgpack___Encode)
	// t.Run("Benchmark__Gcbor______Encode", Benchmark__Gcbor______Encode)
	// t.Run("Benchmark__Xdr________Encode", Benchmark__Xdr________Encode)
	t.Run("Benchmark__Sereal_____Encode", Benchmark__Sereal_____Encode)

	benchmarkDivider()
	t.Run("Benchmark__JsonIter___Decode", Benchmark__JsonIter___Decode)
	t.Run("Benchmark__Bson_______Decode", Benchmark__Bson_______Decode)
	t.Run("Benchmark__Mgobson____Decode", Benchmark__Mgobson____Decode)
	// t.Run("Benchmark__VMsgpack___Decode", Benchmark__VMsgpack___Decode)
	// t.Run("Benchmark__Gcbor______Decode", Benchmark__Gcbor______Decode)
	// t.Run("Benchmark__Xdr________Decode", Benchmark__Xdr________Decode)
	// t.Run("Benchmark__Sereal_____Decode", Benchmark__Sereal_____Decode)
}

func benchmarkCodecXGroup(t *testing.B) {
	benchmarkDivider()
	t.Run("Benchmark__Msgpack____Encode", Benchmark__Msgpack____Encode)
	t.Run("Benchmark__Binc_______Encode", Benchmark__Binc_______Encode)
	t.Run("Benchmark__Simple_____Encode", Benchmark__Simple_____Encode)
	t.Run("Benchmark__Cbor_______Encode", Benchmark__Cbor_______Encode)
	t.Run("Benchmark__Json_______Encode", Benchmark__Json_______Encode)
	t.Run("Benchmark__Std_Json___Encode", Benchmark__Std_Json___Encode)
	t.Run("Benchmark__Gob________Encode", Benchmark__Gob________Encode)
	// t.Run("Benchmark__Std_Xml____Encode", Benchmark__Std_Xml____Encode)
	t.Run("Benchmark__JsonIter___Encode", Benchmark__JsonIter___Encode)
	t.Run("Benchmark__Bson_______Encode", Benchmark__Bson_______Encode)
	t.Run("Benchmark__Mgobson____Encode", Benchmark__Mgobson____Encode)
	t.Run("Benchmark__VMsgpack___Encode", Benchmark__VMsgpack___Encode)
	// t.Run("Benchmark__Gcbor______Encode", Benchmark__Gcbor______Encode)
	// t.Run("Benchmark__Xdr________Encode", Benchmark__Xdr________Encode)
	t.Run("Benchmark__Sereal_____Encode", Benchmark__Sereal_____Encode)

	benchmarkDivider()
	t.Run("Benchmark__Msgpack____Decode", Benchmark__Msgpack____Decode)
	t.Run("Benchmark__Binc_______Decode", Benchmark__Binc_______Decode)
	t.Run("Benchmark__Simple_____Decode", Benchmark__Simple_____Decode)
	t.Run("Benchmark__Cbor_______Decode", Benchmark__Cbor_______Decode)
	t.Run("Benchmark__Json_______Decode", Benchmark__Json_______Decode)
	t.Run("Benchmark__Std_Json___Decode", Benchmark__Std_Json___Decode)
	t.Run("Benchmark__Gob________Decode", Benchmark__Gob________Decode)
	// t.Run("Benchmark__Std_Xml____Decode", Benchmark__Std_Xml____Decode)
	t.Run("Benchmark__JsonIter___Decode", Benchmark__JsonIter___Decode)
	t.Run("Benchmark__Bson_______Decode", Benchmark__Bson_______Decode)
	t.Run("Benchmark__Mgobson____Decode", Benchmark__Mgobson____Decode)
	// t.Run("Benchmark__VMsgpack___Decode", Benchmark__VMsgpack___Decode)
	// t.Run("Benchmark__Gcbor______Decode", Benchmark__Gcbor______Decode)
	// t.Run("Benchmark__Xdr________Decode", Benchmark__Xdr________Decode)
	// t.Run("Benchmark__Sereal_____Decode", Benchmark__Sereal_____Decode)
}

var benchmarkXSkipMsg = `>>>> Skipping - these cannot (en|de)code TestStruc - encode (gcbor, xdr, xml), decode (gcbor, vmsgpack, xdr, sereal, xml)`

func BenchmarkXSuite(t *testing.B) {
	println(benchmarkXSkipMsg)
	benchmarkSuite(t, benchmarkXGroup)
}

func BenchmarkCodecXSuite(t *testing.B) {
	println(benchmarkXSkipMsg)
	benchmarkSuite(t, benchmarkCodecXGroup)
}

func benchmarkAllJsonEncodeGroup(t *testing.B) {
	benchmarkDivider()
	t.Run("Benchmark__Json_______Encode", Benchmark__Json_______Encode)
	t.Run("Benchmark__Std_Json___Encode", Benchmark__Std_Json___Encode)
	t.Run("Benchmark__JsonIter___Encode", Benchmark__JsonIter___Encode)
}

func benchmarkAllJsonDecodeGroup(t *testing.B) {
	benchmarkDivider()
	t.Run("Benchmark__Json_______Decode", Benchmark__Json_______Decode)
	t.Run("Benchmark__Std_Json___Decode", Benchmark__Std_Json___Decode)
	t.Run("Benchmark__JsonIter___Decode", Benchmark__JsonIter___Decode)
}

func BenchmarkCodecVeryQuickAllJsonSuite(t *testing.B) {
	benchmarkVeryQuickSuite(t, "json-all", benchmarkAllJsonEncodeGroup, benchmarkAllJsonDecodeGroup)
}

func BenchmarkCodecQuickAllJsonSuite(t *testing.B) {
	benchmarkQuickSuite(t, "json-all", benchmarkAllJsonEncodeGroup, benchmarkAllJsonDecodeGroup)
	// benchmarkQuickSuite(t, "json-all", benchmarkAllJsonEncodeGroup)
	// benchmarkQuickSuite(t, "json-all", benchmarkAllJsonDecodeGroup)

	// depths := [...]int{1, 4}
	// for _, d := range depths {
	// 	benchmarkQuickSuite(t, d, benchmarkAllJsonEncodeGroup)
	// 	benchmarkQuickSuite(t, d, benchmarkAllJsonDecodeGroup)
	// }

	// benchmarkQuickSuite(t, 1, benchmarkAllJsonEncodeGroup)
	// benchmarkQuickSuite(t, 4, benchmarkAllJsonEncodeGroup)
	// benchmarkQuickSuite(t, 1, benchmarkAllJsonDecodeGroup)
	// benchmarkQuickSuite(t, 4, benchmarkAllJsonDecodeGroup)

	// benchmarkQuickSuite(t, 1, benchmarkAllJsonEncodeGroup, benchmarkAllJsonDecodeGroup)
	// benchmarkQuickSuite(t, 4, benchmarkAllJsonEncodeGroup, benchmarkAllJsonDecodeGroup)
	// benchmarkQuickSuite(t, benchmarkAllJsonEncodeGroup)
	// benchmarkQuickSuite(t, benchmarkAllJsonDecodeGroup)
}
