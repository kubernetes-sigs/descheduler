// Copyright (c) 2012-2018 Ugorji Nwoke. All rights reserved.
// Use of this source code is governed by a MIT license found in the LICENSE file.

package codec

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"math"
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestCborIndefiniteLength(t *testing.T) {
	oldMapType := testCborH.MapType
	defer func() {
		testCborH.MapType = oldMapType
	}()
	testCborH.MapType = testMapStrIntfTyp
	// var (
	// 	M1 map[string][]byte
	// 	M2 map[uint64]bool
	// 	L1 []interface{}
	// 	S1 []string
	// 	B1 []byte
	// )
	var v, vv interface{}
	// define it (v), encode it using indefinite lengths, decode it (vv), compare v to vv
	v = map[string]interface{}{
		"one-byte-key":   []byte{1, 2, 3, 4, 5, 6},
		"two-string-key": "two-value",
		"three-list-key": []interface{}{true, false, uint64(1), int64(-1)},
	}
	var buf bytes.Buffer
	// buf.Reset()
	e := NewEncoder(&buf, testCborH)
	buf.WriteByte(cborBdIndefiniteMap)
	//----
	buf.WriteByte(cborBdIndefiniteString)
	e.MustEncode("one-")
	e.MustEncode("byte-")
	e.MustEncode("key")
	buf.WriteByte(cborBdBreak)

	buf.WriteByte(cborBdIndefiniteBytes)
	e.MustEncode([]byte{1, 2, 3})
	e.MustEncode([]byte{4, 5, 6})
	buf.WriteByte(cborBdBreak)

	//----
	buf.WriteByte(cborBdIndefiniteString)
	e.MustEncode("two-")
	e.MustEncode("string-")
	e.MustEncode("key")
	buf.WriteByte(cborBdBreak)

	buf.WriteByte(cborBdIndefiniteString)
	e.MustEncode([]byte("two-")) // encode as bytes, to check robustness of code
	e.MustEncode([]byte("value"))
	buf.WriteByte(cborBdBreak)

	//----
	buf.WriteByte(cborBdIndefiniteString)
	e.MustEncode("three-")
	e.MustEncode("list-")
	e.MustEncode("key")
	buf.WriteByte(cborBdBreak)

	buf.WriteByte(cborBdIndefiniteArray)
	e.MustEncode(true)
	e.MustEncode(false)
	e.MustEncode(uint64(1))
	e.MustEncode(int64(-1))
	buf.WriteByte(cborBdBreak)

	buf.WriteByte(cborBdBreak) // close map

	NewDecoderBytes(buf.Bytes(), testCborH).MustDecode(&vv)
	if err := deepEqual(v, vv); err != nil {
		t.Logf("-------- Before and After marshal do not match: Error: %v", err)
		if testVerbose {
			t.Logf("    ....... GOLDEN:  (%T) %#v", v, v)
			t.Logf("    ....... DECODED: (%T) %#v", vv, vv)
		}
		t.FailNow()
	}
}

type testCborGolden struct {
	Base64     string      `codec:"cbor"`
	Hex        string      `codec:"hex"`
	Roundtrip  bool        `codec:"roundtrip"`
	Decoded    interface{} `codec:"decoded"`
	Diagnostic string      `codec:"diagnostic"`
	Skip       bool        `codec:"skip"`
}

// Some tests are skipped because they include numbers outside the range of int64/uint64
func TestCborGoldens(t *testing.T) {
	oldMapType := testCborH.MapType
	defer func() {
		testCborH.MapType = oldMapType
	}()
	testCborH.MapType = testMapStrIntfTyp
	// decode test-cbor-goldens.json into a list of []*testCborGolden
	// for each one,
	// - decode hex into []byte bs
	// - decode bs into interface{} v
	// - compare both using deepequal
	// - for any miss, record it
	var gs []*testCborGolden
	f, err := os.Open("test-cbor-goldens.json")
	if err != nil {
		t.Logf("error opening test-cbor-goldens.json: %v", err)
		t.FailNow()
	}
	defer f.Close()
	jh := new(JsonHandle)
	jh.MapType = testMapStrIntfTyp
	// d := NewDecoder(f, jh)
	d := NewDecoder(bufio.NewReader(f), jh)
	// err = d.Decode(&gs)
	d.MustDecode(&gs)
	if err != nil {
		t.Logf("error json decoding test-cbor-goldens.json: %v", err)
		t.FailNow()
	}

	tagregex := regexp.MustCompile(`[\d]+\(.+?\)`)
	hexregex := regexp.MustCompile(`h'([0-9a-fA-F]*)'`)
	for i, g := range gs {
		// fmt.Printf("%v, skip: %v, isTag: %v, %s\n", i, g.Skip, tagregex.MatchString(g.Diagnostic), g.Diagnostic)
		// skip tags or simple or those with prefix, as we can't verify them.
		if g.Skip || strings.HasPrefix(g.Diagnostic, "simple(") || tagregex.MatchString(g.Diagnostic) {
			// fmt.Printf("%v: skipped\n", i)
			if testVerbose {
				t.Logf("[%v] skipping because skip=true OR unsupported simple value or Tag Value", i)
			}
			continue
		}
		// println("++++++++++++", i, "g.Diagnostic", g.Diagnostic)
		if hexregex.MatchString(g.Diagnostic) {
			// println(i, "g.Diagnostic matched hex")
			if s2 := g.Diagnostic[2 : len(g.Diagnostic)-1]; s2 == "" {
				g.Decoded = zeroByteSlice
			} else if bs2, err2 := hex.DecodeString(s2); err2 == nil {
				g.Decoded = bs2
			}
			// fmt.Printf("%v: hex: %v\n", i, g.Decoded)
		}
		bs, err := hex.DecodeString(g.Hex)
		if err != nil {
			t.Logf("[%v] error hex decoding %s [%v]: %v", i, g.Hex, g.Hex, err)
			t.FailNow()
		}
		var v interface{}
		NewDecoderBytes(bs, testCborH).MustDecode(&v)
		if _, ok := v.(RawExt); ok {
			continue
		}
		// check the diagnostics to compare
		switch g.Diagnostic {
		case "Infinity":
			b := math.IsInf(v.(float64), 1)
			testCborError(t, i, math.Inf(1), v, nil, &b)
		case "-Infinity":
			b := math.IsInf(v.(float64), -1)
			testCborError(t, i, math.Inf(-1), v, nil, &b)
		case "NaN":
			// println(i, "checking NaN")
			b := math.IsNaN(v.(float64))
			testCborError(t, i, math.NaN(), v, nil, &b)
		case "undefined":
			b := v == nil
			testCborError(t, i, nil, v, nil, &b)
		default:
			v0 := g.Decoded
			// testCborCoerceJsonNumber(rv4i(&v0))
			testCborError(t, i, v0, v, deepEqual(v0, v), nil)
		}
	}
}

func testCborError(t *testing.T, i int, v0, v1 interface{}, err error, equal *bool) {
	if err == nil && equal == nil {
		// fmt.Printf("%v testCborError passed (err and equal nil)\n", i)
		return
	}
	if err != nil {
		t.Logf("[%v] deepEqual error: %v", i, err)
		if testVerbose {
			t.Logf("    ....... GOLDEN:  (%T) %#v", v0, v0)
			t.Logf("    ....... DECODED: (%T) %#v", v1, v1)
		}
		t.FailNow()
	}
	if equal != nil && !*equal {
		t.Logf("[%v] values not equal", i)
		if testVerbose {
			t.Logf("    ....... GOLDEN:  (%T) %#v", v0, v0)
			t.Logf("    ....... DECODED: (%T) %#v", v1, v1)
		}
		t.FailNow()
	}
	// fmt.Printf("%v testCborError passed (checks passed)\n", i)
}

func TestCborHalfFloat(t *testing.T) {
	m := map[uint16]float64{
		// using examples from
		// https://en.wikipedia.org/wiki/Half-precision_floating-point_format
		0x3c00: 1,
		0x3c01: 1 + math.Pow(2, -10),
		0xc000: -2,
		0x7bff: 65504,
		0x0400: math.Pow(2, -14),
		0x03ff: math.Pow(2, -14) - math.Pow(2, -24),
		0x0001: math.Pow(2, -24),
		0x0000: 0,
		0x8000: -0.0,
	}
	var ba [3]byte
	ba[0] = cborBdFloat16
	var res float64
	for k, v := range m {
		res = 0
		bigen.PutUint16(ba[1:], k)
		testUnmarshalErr(&res, ba[:3], testCborH, t, "-")
		if res == v {
			if testVerbose {
				t.Logf("equal floats: from %x %b, %v", k, k, v)
			}
		} else {
			t.Logf("unequal floats: from %x %b, %v != %v", k, k, res, v)
			t.FailNow()
		}
	}
}

func TestCborSkipTags(t *testing.T) {
	type Tcbortags struct {
		A string
		M map[string]interface{}
		// A []interface{}
	}
	var b8 [8]byte
	var w bytesEncAppender
	w.b = []byte{}

	// To make it easier,
	//    - use tags between math.MaxUint8 and math.MaxUint16 (incl SelfDesc)
	//    - use 1 char strings for key names
	//    - use 3-6 char strings for map keys
	//    - use integers that fit in 2 bytes (between 0x20 and 0xff)

	var tags = [...]uint64{math.MaxUint8 * 2, math.MaxUint8 * 8, 55799, math.MaxUint16 / 2}
	var tagIdx int
	var doAddTag bool
	addTagFn8To16 := func() {
		if !doAddTag {
			return
		}
		// writes a tag between MaxUint8 and MaxUint16 (culled from cborEncDriver.encUint)
		w.writen1(cborBaseTag + 0x19)
		// bigenHelper.writeUint16
		bigen.PutUint16(b8[:2], uint16(tags[tagIdx%len(tags)]))
		w.writeb(b8[:2])
		tagIdx++
	}

	var v Tcbortags
	v.A = "cbor"
	v.M = make(map[string]interface{})
	v.M["111"] = uint64(111)
	v.M["111.11"] = 111.11
	v.M["true"] = true
	// v.A = append(v.A, 222, 22.22, "true")

	// make stream manually (interspacing tags around it)
	// WriteMapStart - e.encLen(cborBaseMap, length) - encUint(length, bd)
	// EncodeStringEnc - e.encStringBytesS(cborBaseString, v)

	fnEncode := func() {
		w.b = w.b[:0]
		addTagFn8To16()
		// write v (Tcbortags, with 3 fields = map with 3 entries)
		w.writen1(2 + cborBaseMap) // 3 fields = 3 entries
		// write v.A
		var s = "A"
		w.writen1(byte(len(s)) + cborBaseString)
		w.writestr(s)
		w.writen1(byte(len(v.A)) + cborBaseString)
		w.writestr(v.A)
		//w.writen1(0)

		addTagFn8To16()
		s = "M"
		w.writen1(byte(len(s)) + cborBaseString)
		w.writestr(s)

		addTagFn8To16()
		w.writen1(byte(len(v.M)) + cborBaseMap)

		addTagFn8To16()
		s = "111"
		w.writen1(byte(len(s)) + cborBaseString)
		w.writestr(s)
		w.writen2(cborBaseUint+0x18, uint8(111))

		addTagFn8To16()
		s = "111.11"
		w.writen1(byte(len(s)) + cborBaseString)
		w.writestr(s)
		w.writen1(cborBdFloat64)
		bigen.PutUint64(b8[:8], math.Float64bits(111.11))
		w.writeb(b8[:8])

		addTagFn8To16()
		s = "true"
		w.writen1(byte(len(s)) + cborBaseString)
		w.writestr(s)
		w.writen1(cborBdTrue)
	}

	var h CborHandle
	h.SkipUnexpectedTags = true
	h.Canonical = true

	var gold []byte
	NewEncoderBytes(&gold, &h).MustEncode(v)
	// xdebug2f("encoded:    gold: %v", gold)

	// w.b is the encoded bytes
	var v2 Tcbortags
	doAddTag = false
	fnEncode()
	// xdebug2f("manual:  no-tags: %v", w.b)

	testDeepEqualErr(gold, w.b, t, "cbor-skip-tags--bytes---")
	NewDecoderBytes(w.b, &h).MustDecode(&v2)
	testDeepEqualErr(v, v2, t, "cbor-skip-tags--no-tags-")

	var v3 Tcbortags
	doAddTag = true
	fnEncode()
	// xdebug2f("manual: has-tags: %v", w.b)
	NewDecoderBytes(w.b, &h).MustDecode(&v3)
	testDeepEqualErr(v, v2, t, "cbor-skip-tags--has-tags")

	// Github 300 - tests naked path
	{
		expected := []interface{}{"x", uint64(0x0)}
		toDecode := []byte{0x82, 0x61, 0x78, 0x00}

		var raw interface{}

		NewDecoderBytes(toDecode, &h).MustDecode(&raw)
		testDeepEqualErr(expected, raw, t, "cbor-skip-tags--gh-300---no-skips")

		toDecode = []byte{0xd9, 0xd9, 0xf7, 0x82, 0x61, 0x78, 0x00}
		raw = nil
		NewDecoderBytes(toDecode, &h).MustDecode(&raw)
		testDeepEqualErr(expected, raw, t, "cbor-skip-tags--gh-300--has-skips")
	}
}
