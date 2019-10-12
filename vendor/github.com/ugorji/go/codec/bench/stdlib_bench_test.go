// +build !generated

// Copyright (c) 2012-2018 Ugorji Nwoke. All rights reserved.
// Use of this source code is governed by a MIT license found in the LICENSE file.

package codec

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"encoding/xml"
	"testing"
)

func init() {
	testPreInitFns = append(testPreInitFns, stdlibBenchPreInit)
	// testPostInitFns = append(testPostInitFns, codecbenchPostInit)
}

func stdlibBenchPreInit() {
	benchCheckers = append(benchCheckers,
		benchChecker{"std-json", fnStdJsonEncodeFn, fnStdJsonDecodeFn},
		benchChecker{"gob", fnGobEncodeFn, fnGobDecodeFn},
		benchChecker{"std-xml", fnStdXmlEncodeFn, fnStdXmlDecodeFn},
	)
}

// ------------ tests below

func fnGobEncodeFn(ts interface{}, bsIn []byte) ([]byte, error) {
	buf := fnBenchmarkByteBuf(bsIn)
	err := gob.NewEncoder(buf).Encode(ts)
	return buf.Bytes(), err
}

func fnGobDecodeFn(buf []byte, ts interface{}) error {
	return gob.NewDecoder(bytes.NewReader(buf)).Decode(ts)
}

func fnStdXmlEncodeFn(ts interface{}, bsIn []byte) ([]byte, error) {
	buf := fnBenchmarkByteBuf(bsIn)
	err := xml.NewEncoder(buf).Encode(ts)
	return buf.Bytes(), err
}

func fnStdXmlDecodeFn(buf []byte, ts interface{}) error {
	return xml.NewDecoder(bytes.NewReader(buf)).Decode(ts)
}

func fnStdJsonEncodeFn(ts interface{}, bsIn []byte) ([]byte, error) {
	if testUseIoEncDec >= 0 {
		buf := fnBenchmarkByteBuf(bsIn)
		err := json.NewEncoder(buf).Encode(ts)
		return buf.Bytes(), err
	}
	return json.Marshal(ts)
}

func fnStdJsonDecodeFn(buf []byte, ts interface{}) error {
	if testUseIoEncDec >= 0 {
		return json.NewDecoder(bytes.NewReader(buf)).Decode(ts)
	}
	return json.Unmarshal(buf, ts)
}

// ----------- ENCODE ------------------

func Benchmark__Std_Json___Encode(b *testing.B) {
	fnBenchmarkEncode(b, "std-json", benchTs, fnStdJsonEncodeFn)
}

func Benchmark__Gob________Encode(b *testing.B) {
	fnBenchmarkEncode(b, "gob", benchTs, fnGobEncodeFn)
}

func Benchmark__Std_Xml____Encode(b *testing.B) {
	fnBenchmarkEncode(b, "std-xml", benchTs, fnStdXmlEncodeFn)
}

// ----------- DECODE ------------------

func Benchmark__Std_Json___Decode(b *testing.B) {
	fnBenchmarkDecode(b, "std-json", benchTs, fnStdJsonEncodeFn, fnStdJsonDecodeFn, fnBenchNewTs)
}

func Benchmark__Gob________Decode(b *testing.B) {
	fnBenchmarkDecode(b, "gob", benchTs, fnGobEncodeFn, fnGobDecodeFn, fnBenchNewTs)
}

func Benchmark__Std_Xml____Decode(b *testing.B) {
	fnBenchmarkDecode(b, "std-xml", benchTs, fnStdXmlEncodeFn, fnStdXmlDecodeFn, fnBenchNewTs)
}
