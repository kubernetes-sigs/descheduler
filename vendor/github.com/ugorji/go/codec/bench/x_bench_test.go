// +build x
// +build !generated

// Copyright (c) 2012-2018 Ugorji Nwoke. All rights reserved.
// Use of this source code is governed by a MIT license found in the LICENSE file.

package codec

import (
	"bytes"
	"testing"

	gcbor "bitbucket.org/bodhisnarkva/cbor/go" // gcbor "code.google.com/p/cbor/go"
	"github.com/Sereal/Sereal/Go/sereal"
	xdr "github.com/davecgh/go-xdr/xdr2"
	jsoniter "github.com/json-iterator/go"
	"go.mongodb.org/mongo-driver/bson"         // "github.com/mongodb/mongo-go-driver/bson"
	mgobson "gopkg.in/mgo.v2/bson"             //"labix.org/v2/mgo/bson"
	vmsgpack "gopkg.in/vmihailenco/msgpack.v2" //"github.com/vmihailenco/msgpack"
)

/*
 To update all these, use:
 go get -u github.com/tinylib/msgp/msgp github.com/tinylib/msgp \
           github.com/pquerna/ffjson/ffjson github.com/pquerna/ffjson \
           github.com/Sereal/Sereal/Go/sereal \
           bitbucket.org/bodhisnarkva/cbor/go \
           github.com/davecgh/go-xdr/xdr2 \
           gopkg.in/mgo.v2/bson \
           gopkg.in/vmihailenco/msgpack.v2 \
           github.com/json-iterator/go \
           github.com/mailru/easyjson/...

 Known Issues with external libraries:
 - msgp io.R/W support doesn't work. It throws error

*/

func init() {
	testPreInitFns = append(testPreInitFns, benchXPreInit)
	_ = bson.NewDecoder
}

func benchXPreInit() {
	benchCheckers = append(benchCheckers,
		benchChecker{"json-iter", fnJsonIterEncodeFn, fnJsonIterDecodeFn},
		benchChecker{"v-msgpack", fnVMsgpackEncodeFn, fnVMsgpackDecodeFn},
		benchChecker{"bson", fnBsonEncodeFn, fnBsonDecodeFn},
		benchChecker{"mgobson", fnMgobsonEncodeFn, fnMgobsonDecodeFn},
		// place codecs with issues at the end, so as not to make results too ugly
		benchChecker{"gcbor", fnGcborEncodeFn, fnGcborDecodeFn}, // this logs fat ugly message, but we log.SetOutput(ioutil.Discard)
		benchChecker{"xdr", fnXdrEncodeFn, fnXdrDecodeFn},
		benchChecker{"sereal", fnSerealEncodeFn, fnSerealDecodeFn},
	)
}

func fnVMsgpackEncodeFn(ts interface{}, bsIn []byte) ([]byte, error) {
	if testUseIoEncDec >= 0 {
		buf := bytes.NewBuffer(bsIn[:0]) // new(bytes.Buffer)
		err := vmsgpack.NewEncoder(buf).Encode(ts)
		return buf.Bytes(), err
	}
	return vmsgpack.Marshal(ts)
}

func fnVMsgpackDecodeFn(buf []byte, ts interface{}) error {
	if testUseIoEncDec >= 0 {
		return vmsgpack.NewDecoder(bytes.NewReader(buf)).Decode(ts)
	}
	return vmsgpack.Unmarshal(buf, ts)
}

func fnBsonEncodeFn(ts interface{}, bsIn []byte) ([]byte, error) {
	return bson.Marshal(ts)
}

func fnBsonDecodeFn(buf []byte, ts interface{}) error {
	return bson.Unmarshal(buf, ts)
}

func fnMgobsonEncodeFn(ts interface{}, bsIn []byte) ([]byte, error) {
	return mgobson.Marshal(ts)
}

func fnMgobsonDecodeFn(buf []byte, ts interface{}) error {
	return mgobson.Unmarshal(buf, ts)
}

func fnJsonIterEncodeFn(ts interface{}, bsIn []byte) ([]byte, error) {
	if testUseIoEncDec >= 0 {
		buf := bytes.NewBuffer(bsIn[:0]) // new(bytes.Buffer)
		err := jsoniter.NewEncoder(buf).Encode(ts)
		return buf.Bytes(), err
	}
	return jsoniter.Marshal(ts)
}

func fnJsonIterDecodeFn(buf []byte, ts interface{}) error {
	if testUseIoEncDec >= 0 {
		return jsoniter.NewDecoder(bytes.NewReader(buf)).Decode(ts)
	}
	return jsoniter.Unmarshal(buf, ts)
}

func fnXdrEncodeFn(ts interface{}, bsIn []byte) ([]byte, error) {
	buf := fnBenchmarkByteBuf(bsIn)
	i, err := xdr.Marshal(buf, ts)
	return buf.Bytes()[:i], err
}

func fnXdrDecodeFn(buf []byte, ts interface{}) error {
	_, err := xdr.Unmarshal(bytes.NewReader(buf), ts)
	return err
}

func fnSerealEncodeFn(ts interface{}, bsIn []byte) ([]byte, error) {
	return sereal.Marshal(ts)
}

func fnSerealDecodeFn(buf []byte, ts interface{}) error {
	return sereal.Unmarshal(buf, ts)
}

func fnGcborEncodeFn(ts interface{}, bsIn []byte) (bs []byte, err error) {
	buf := fnBenchmarkByteBuf(bsIn)
	err = gcbor.NewEncoder(buf).Encode(ts)
	return buf.Bytes(), err
}

func fnGcborDecodeFn(buf []byte, ts interface{}) error {
	return gcbor.NewDecoder(bytes.NewReader(buf)).Decode(ts)
}

func Benchmark__JsonIter___Encode(b *testing.B) {
	fnBenchmarkEncode(b, "jsoniter", benchTs, fnJsonIterEncodeFn)
}

func Benchmark__JsonIter___Decode(b *testing.B) {
	fnBenchmarkDecode(b, "jsoniter", benchTs, fnJsonIterEncodeFn, fnJsonIterDecodeFn, fnBenchNewTs)
}

// Place codecs with issues at the bottom, so as not to make results look too ugly.

func Benchmark__Mgobson____Encode(b *testing.B) {
	fnBenchmarkEncode(b, "mgobson", benchTs, fnMgobsonEncodeFn)
}

func Benchmark__Mgobson____Decode(b *testing.B) {
	fnBenchmarkDecode(b, "mgobson", benchTs, fnMgobsonEncodeFn, fnMgobsonDecodeFn, fnBenchNewTs)
}

func Benchmark__Bson_______Encode(b *testing.B) {
	fnBenchmarkEncode(b, "bson", benchTs, fnBsonEncodeFn)
}

func Benchmark__Bson_______Decode(b *testing.B) {
	fnBenchmarkDecode(b, "bson", benchTs, fnBsonEncodeFn, fnBsonDecodeFn, fnBenchNewTs)
}

func Benchmark__VMsgpack___Encode(b *testing.B) {
	fnBenchmarkEncode(b, "v-msgpack", benchTs, fnVMsgpackEncodeFn)
}

func Benchmark__VMsgpack___Decode(b *testing.B) {
	fnBenchmarkDecode(b, "v-msgpack", benchTs, fnVMsgpackEncodeFn, fnVMsgpackDecodeFn, fnBenchNewTs)
}

func Benchmark__Gcbor______Encode(b *testing.B) {
	fnBenchmarkEncode(b, "gcbor", benchTs, fnGcborEncodeFn)
}

func Benchmark__Gcbor______Decode(b *testing.B) {
	fnBenchmarkDecode(b, "gcbor", benchTs, fnGcborEncodeFn, fnGcborDecodeFn, fnBenchNewTs)
}

func Benchmark__Xdr________Encode(b *testing.B) {
	fnBenchmarkEncode(b, "xdr", benchTs, fnXdrEncodeFn)
}

func Benchmark__Xdr________Decode(b *testing.B) {
	fnBenchmarkDecode(b, "xdr", benchTs, fnXdrEncodeFn, fnXdrDecodeFn, fnBenchNewTs)
}

func Benchmark__Sereal_____Encode(b *testing.B) {
	fnBenchmarkEncode(b, "sereal", benchTs, fnSerealEncodeFn)
}

func Benchmark__Sereal_____Decode(b *testing.B) {
	fnBenchmarkDecode(b, "sereal", benchTs, fnSerealEncodeFn, fnSerealDecodeFn, fnBenchNewTs)
}
