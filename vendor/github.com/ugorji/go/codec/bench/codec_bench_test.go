// Copyright (c) 2012-2018 Ugorji Nwoke. All rights reserved.
// Use of this source code is governed by a MIT license found in the LICENSE file.

package codec

import (
	"testing"
)

func init() {
	testPreInitFns = append(testPreInitFns, codecBenchPreInit)
	// testPostInitFns = append(testPostInitFns, codecbenchPostInit)
}

func codecBenchPreInit() {
	benchCheckers = append(benchCheckers,
		benchChecker{"msgpack", fnMsgpackEncodeFn, fnMsgpackDecodeFn},
		benchChecker{"binc", fnBincEncodeFn, fnBincDecodeFn},
		benchChecker{"simple", fnSimpleEncodeFn, fnSimpleDecodeFn},
		benchChecker{"cbor", fnCborEncodeFn, fnCborDecodeFn},
		benchChecker{"json", fnJsonEncodeFn, fnJsonDecodeFn},
	)
}

// ------------ tests below

func fnMsgpackEncodeFn(ts interface{}, bsIn []byte) (bs []byte, err error) {
	return sTestCodecEncode(ts, bsIn, fnBenchmarkByteBuf, testMsgpackH, &testMsgpackH.BasicHandle)
}

func fnMsgpackDecodeFn(buf []byte, ts interface{}) error {
	return sTestCodecDecode(buf, ts, testMsgpackH, &testMsgpackH.BasicHandle)
}

func fnBincEncodeFn(ts interface{}, bsIn []byte) (bs []byte, err error) {
	return sTestCodecEncode(ts, bsIn, fnBenchmarkByteBuf, testBincH, &testBincH.BasicHandle)
}

func fnBincDecodeFn(buf []byte, ts interface{}) error {
	return sTestCodecDecode(buf, ts, testBincH, &testBincH.BasicHandle)
}

func fnSimpleEncodeFn(ts interface{}, bsIn []byte) (bs []byte, err error) {
	return sTestCodecEncode(ts, bsIn, fnBenchmarkByteBuf, testSimpleH, &testSimpleH.BasicHandle)
}

func fnSimpleDecodeFn(buf []byte, ts interface{}) error {
	return sTestCodecDecode(buf, ts, testSimpleH, &testSimpleH.BasicHandle)
}

func fnCborEncodeFn(ts interface{}, bsIn []byte) (bs []byte, err error) {
	return sTestCodecEncode(ts, bsIn, fnBenchmarkByteBuf, testCborH, &testCborH.BasicHandle)
}

func fnCborDecodeFn(buf []byte, ts interface{}) error {
	return sTestCodecDecode(buf, ts, testCborH, &testCborH.BasicHandle)
}

func fnJsonEncodeFn(ts interface{}, bsIn []byte) (bs []byte, err error) {
	return sTestCodecEncode(ts, bsIn, fnBenchmarkByteBuf, testJsonH, &testJsonH.BasicHandle)
}

func fnJsonDecodeFn(buf []byte, ts interface{}) error {
	return sTestCodecDecode(buf, ts, testJsonH, &testJsonH.BasicHandle)
}

// ----------- ENCODE ------------------

func Benchmark__Msgpack____Encode(b *testing.B) {
	fnBenchmarkEncode(b, "msgpack", benchTs, fnMsgpackEncodeFn)
}

func Benchmark__Binc_______Encode(b *testing.B) {
	fnBenchmarkEncode(b, "binc", benchTs, fnBincEncodeFn)
}

func Benchmark__Simple_____Encode(b *testing.B) {
	fnBenchmarkEncode(b, "simple", benchTs, fnSimpleEncodeFn)
}

func Benchmark__Cbor_______Encode(b *testing.B) {
	fnBenchmarkEncode(b, "cbor", benchTs, fnCborEncodeFn)
}

func Benchmark__Json_______Encode(b *testing.B) {
	fnBenchmarkEncode(b, "json", benchTs, fnJsonEncodeFn)
}

// ----------- DECODE ------------------

func Benchmark__Msgpack____Decode(b *testing.B) {
	fnBenchmarkDecode(b, "msgpack", benchTs, fnMsgpackEncodeFn, fnMsgpackDecodeFn, fnBenchNewTs)
}

func Benchmark__Binc_______Decode(b *testing.B) {
	fnBenchmarkDecode(b, "binc", benchTs, fnBincEncodeFn, fnBincDecodeFn, fnBenchNewTs)
}

func Benchmark__Simple_____Decode(b *testing.B) {
	fnBenchmarkDecode(b, "simple", benchTs, fnSimpleEncodeFn, fnSimpleDecodeFn, fnBenchNewTs)
}

func Benchmark__Cbor_______Decode(b *testing.B) {
	fnBenchmarkDecode(b, "cbor", benchTs, fnCborEncodeFn, fnCborDecodeFn, fnBenchNewTs)
}

func Benchmark__Json_______Decode(b *testing.B) {
	fnBenchmarkDecode(b, "json", benchTs, fnJsonEncodeFn, fnJsonDecodeFn, fnBenchNewTs)
}
