// Copyright (c) 2012-2018 Ugorji Nwoke. All rights reserved.
// Use of this source code is governed by a MIT license found in the LICENSE file.

// Code generated from mammoth-test.go.tmpl - DO NOT EDIT.

package codec

import "testing"
import "fmt"

// TestMammoth has all the different paths optimized in fast-path
// It has all the primitives, slices and maps.
//
// For each of those types, it has a pointer and a non-pointer field.

func init() { _ = fmt.Printf } // so we can include fmt as needed

type TestMammoth struct {
	FIntf       interface{}
	FptrIntf    *interface{}
	FString     string
	FptrString  *string
	FBytes      []byte
	FptrBytes   *[]byte
	FFloat32    float32
	FptrFloat32 *float32
	FFloat64    float64
	FptrFloat64 *float64
	FUint       uint
	FptrUint    *uint
	FUint8      uint8
	FptrUint8   *uint8
	FUint16     uint16
	FptrUint16  *uint16
	FUint32     uint32
	FptrUint32  *uint32
	FUint64     uint64
	FptrUint64  *uint64
	FUintptr    uintptr
	FptrUintptr *uintptr
	FInt        int
	FptrInt     *int
	FInt8       int8
	FptrInt8    *int8
	FInt16      int16
	FptrInt16   *int16
	FInt32      int32
	FptrInt32   *int32
	FInt64      int64
	FptrInt64   *int64
	FBool       bool
	FptrBool    *bool

	FSliceIntf       []interface{}
	FptrSliceIntf    *[]interface{}
	FSliceString     []string
	FptrSliceString  *[]string
	FSliceBytes      [][]byte
	FptrSliceBytes   *[][]byte
	FSliceFloat32    []float32
	FptrSliceFloat32 *[]float32
	FSliceFloat64    []float64
	FptrSliceFloat64 *[]float64
	FSliceUint       []uint
	FptrSliceUint    *[]uint
	FSliceUint16     []uint16
	FptrSliceUint16  *[]uint16
	FSliceUint32     []uint32
	FptrSliceUint32  *[]uint32
	FSliceUint64     []uint64
	FptrSliceUint64  *[]uint64
	FSliceInt        []int
	FptrSliceInt     *[]int
	FSliceInt8       []int8
	FptrSliceInt8    *[]int8
	FSliceInt16      []int16
	FptrSliceInt16   *[]int16
	FSliceInt32      []int32
	FptrSliceInt32   *[]int32
	FSliceInt64      []int64
	FptrSliceInt64   *[]int64
	FSliceBool       []bool
	FptrSliceBool    *[]bool

	FMapStringIntf       map[string]interface{}
	FptrMapStringIntf    *map[string]interface{}
	FMapStringString     map[string]string
	FptrMapStringString  *map[string]string
	FMapStringBytes      map[string][]byte
	FptrMapStringBytes   *map[string][]byte
	FMapStringUint       map[string]uint
	FptrMapStringUint    *map[string]uint
	FMapStringUint8      map[string]uint8
	FptrMapStringUint8   *map[string]uint8
	FMapStringUint64     map[string]uint64
	FptrMapStringUint64  *map[string]uint64
	FMapStringInt        map[string]int
	FptrMapStringInt     *map[string]int
	FMapStringInt64      map[string]int64
	FptrMapStringInt64   *map[string]int64
	FMapStringFloat32    map[string]float32
	FptrMapStringFloat32 *map[string]float32
	FMapStringFloat64    map[string]float64
	FptrMapStringFloat64 *map[string]float64
	FMapStringBool       map[string]bool
	FptrMapStringBool    *map[string]bool
	FMapUintIntf         map[uint]interface{}
	FptrMapUintIntf      *map[uint]interface{}
	FMapUintString       map[uint]string
	FptrMapUintString    *map[uint]string
	FMapUintBytes        map[uint][]byte
	FptrMapUintBytes     *map[uint][]byte
	FMapUintUint         map[uint]uint
	FptrMapUintUint      *map[uint]uint
	FMapUintUint8        map[uint]uint8
	FptrMapUintUint8     *map[uint]uint8
	FMapUintUint64       map[uint]uint64
	FptrMapUintUint64    *map[uint]uint64
	FMapUintInt          map[uint]int
	FptrMapUintInt       *map[uint]int
	FMapUintInt64        map[uint]int64
	FptrMapUintInt64     *map[uint]int64
	FMapUintFloat32      map[uint]float32
	FptrMapUintFloat32   *map[uint]float32
	FMapUintFloat64      map[uint]float64
	FptrMapUintFloat64   *map[uint]float64
	FMapUintBool         map[uint]bool
	FptrMapUintBool      *map[uint]bool
	FMapUint8Intf        map[uint8]interface{}
	FptrMapUint8Intf     *map[uint8]interface{}
	FMapUint8String      map[uint8]string
	FptrMapUint8String   *map[uint8]string
	FMapUint8Bytes       map[uint8][]byte
	FptrMapUint8Bytes    *map[uint8][]byte
	FMapUint8Uint        map[uint8]uint
	FptrMapUint8Uint     *map[uint8]uint
	FMapUint8Uint8       map[uint8]uint8
	FptrMapUint8Uint8    *map[uint8]uint8
	FMapUint8Uint64      map[uint8]uint64
	FptrMapUint8Uint64   *map[uint8]uint64
	FMapUint8Int         map[uint8]int
	FptrMapUint8Int      *map[uint8]int
	FMapUint8Int64       map[uint8]int64
	FptrMapUint8Int64    *map[uint8]int64
	FMapUint8Float32     map[uint8]float32
	FptrMapUint8Float32  *map[uint8]float32
	FMapUint8Float64     map[uint8]float64
	FptrMapUint8Float64  *map[uint8]float64
	FMapUint8Bool        map[uint8]bool
	FptrMapUint8Bool     *map[uint8]bool
	FMapUint64Intf       map[uint64]interface{}
	FptrMapUint64Intf    *map[uint64]interface{}
	FMapUint64String     map[uint64]string
	FptrMapUint64String  *map[uint64]string
	FMapUint64Bytes      map[uint64][]byte
	FptrMapUint64Bytes   *map[uint64][]byte
	FMapUint64Uint       map[uint64]uint
	FptrMapUint64Uint    *map[uint64]uint
	FMapUint64Uint8      map[uint64]uint8
	FptrMapUint64Uint8   *map[uint64]uint8
	FMapUint64Uint64     map[uint64]uint64
	FptrMapUint64Uint64  *map[uint64]uint64
	FMapUint64Int        map[uint64]int
	FptrMapUint64Int     *map[uint64]int
	FMapUint64Int64      map[uint64]int64
	FptrMapUint64Int64   *map[uint64]int64
	FMapUint64Float32    map[uint64]float32
	FptrMapUint64Float32 *map[uint64]float32
	FMapUint64Float64    map[uint64]float64
	FptrMapUint64Float64 *map[uint64]float64
	FMapUint64Bool       map[uint64]bool
	FptrMapUint64Bool    *map[uint64]bool
	FMapIntIntf          map[int]interface{}
	FptrMapIntIntf       *map[int]interface{}
	FMapIntString        map[int]string
	FptrMapIntString     *map[int]string
	FMapIntBytes         map[int][]byte
	FptrMapIntBytes      *map[int][]byte
	FMapIntUint          map[int]uint
	FptrMapIntUint       *map[int]uint
	FMapIntUint8         map[int]uint8
	FptrMapIntUint8      *map[int]uint8
	FMapIntUint64        map[int]uint64
	FptrMapIntUint64     *map[int]uint64
	FMapIntInt           map[int]int
	FptrMapIntInt        *map[int]int
	FMapIntInt64         map[int]int64
	FptrMapIntInt64      *map[int]int64
	FMapIntFloat32       map[int]float32
	FptrMapIntFloat32    *map[int]float32
	FMapIntFloat64       map[int]float64
	FptrMapIntFloat64    *map[int]float64
	FMapIntBool          map[int]bool
	FptrMapIntBool       *map[int]bool
	FMapInt64Intf        map[int64]interface{}
	FptrMapInt64Intf     *map[int64]interface{}
	FMapInt64String      map[int64]string
	FptrMapInt64String   *map[int64]string
	FMapInt64Bytes       map[int64][]byte
	FptrMapInt64Bytes    *map[int64][]byte
	FMapInt64Uint        map[int64]uint
	FptrMapInt64Uint     *map[int64]uint
	FMapInt64Uint8       map[int64]uint8
	FptrMapInt64Uint8    *map[int64]uint8
	FMapInt64Uint64      map[int64]uint64
	FptrMapInt64Uint64   *map[int64]uint64
	FMapInt64Int         map[int64]int
	FptrMapInt64Int      *map[int64]int
	FMapInt64Int64       map[int64]int64
	FptrMapInt64Int64    *map[int64]int64
	FMapInt64Float32     map[int64]float32
	FptrMapInt64Float32  *map[int64]float32
	FMapInt64Float64     map[int64]float64
	FptrMapInt64Float64  *map[int64]float64
	FMapInt64Bool        map[int64]bool
	FptrMapInt64Bool     *map[int64]bool
}

type typMbsSliceIntf []interface{}

func (_ typMbsSliceIntf) MapBySlice() {}

type typMbsSliceString []string

func (_ typMbsSliceString) MapBySlice() {}

type typMbsSliceBytes [][]byte

func (_ typMbsSliceBytes) MapBySlice() {}

type typMbsSliceFloat32 []float32

func (_ typMbsSliceFloat32) MapBySlice() {}

type typMbsSliceFloat64 []float64

func (_ typMbsSliceFloat64) MapBySlice() {}

type typMbsSliceUint []uint

func (_ typMbsSliceUint) MapBySlice() {}

type typMbsSliceUint16 []uint16

func (_ typMbsSliceUint16) MapBySlice() {}

type typMbsSliceUint32 []uint32

func (_ typMbsSliceUint32) MapBySlice() {}

type typMbsSliceUint64 []uint64

func (_ typMbsSliceUint64) MapBySlice() {}

type typMbsSliceInt []int

func (_ typMbsSliceInt) MapBySlice() {}

type typMbsSliceInt8 []int8

func (_ typMbsSliceInt8) MapBySlice() {}

type typMbsSliceInt16 []int16

func (_ typMbsSliceInt16) MapBySlice() {}

type typMbsSliceInt32 []int32

func (_ typMbsSliceInt32) MapBySlice() {}

type typMbsSliceInt64 []int64

func (_ typMbsSliceInt64) MapBySlice() {}

type typMbsSliceBool []bool

func (_ typMbsSliceBool) MapBySlice() {}

type typMapMapStringIntf map[string]interface{}
type typMapMapStringString map[string]string
type typMapMapStringBytes map[string][]byte
type typMapMapStringUint map[string]uint
type typMapMapStringUint8 map[string]uint8
type typMapMapStringUint64 map[string]uint64
type typMapMapStringInt map[string]int
type typMapMapStringInt64 map[string]int64
type typMapMapStringFloat32 map[string]float32
type typMapMapStringFloat64 map[string]float64
type typMapMapStringBool map[string]bool
type typMapMapUintIntf map[uint]interface{}
type typMapMapUintString map[uint]string
type typMapMapUintBytes map[uint][]byte
type typMapMapUintUint map[uint]uint
type typMapMapUintUint8 map[uint]uint8
type typMapMapUintUint64 map[uint]uint64
type typMapMapUintInt map[uint]int
type typMapMapUintInt64 map[uint]int64
type typMapMapUintFloat32 map[uint]float32
type typMapMapUintFloat64 map[uint]float64
type typMapMapUintBool map[uint]bool
type typMapMapUint8Intf map[uint8]interface{}
type typMapMapUint8String map[uint8]string
type typMapMapUint8Bytes map[uint8][]byte
type typMapMapUint8Uint map[uint8]uint
type typMapMapUint8Uint8 map[uint8]uint8
type typMapMapUint8Uint64 map[uint8]uint64
type typMapMapUint8Int map[uint8]int
type typMapMapUint8Int64 map[uint8]int64
type typMapMapUint8Float32 map[uint8]float32
type typMapMapUint8Float64 map[uint8]float64
type typMapMapUint8Bool map[uint8]bool
type typMapMapUint64Intf map[uint64]interface{}
type typMapMapUint64String map[uint64]string
type typMapMapUint64Bytes map[uint64][]byte
type typMapMapUint64Uint map[uint64]uint
type typMapMapUint64Uint8 map[uint64]uint8
type typMapMapUint64Uint64 map[uint64]uint64
type typMapMapUint64Int map[uint64]int
type typMapMapUint64Int64 map[uint64]int64
type typMapMapUint64Float32 map[uint64]float32
type typMapMapUint64Float64 map[uint64]float64
type typMapMapUint64Bool map[uint64]bool
type typMapMapIntIntf map[int]interface{}
type typMapMapIntString map[int]string
type typMapMapIntBytes map[int][]byte
type typMapMapIntUint map[int]uint
type typMapMapIntUint8 map[int]uint8
type typMapMapIntUint64 map[int]uint64
type typMapMapIntInt map[int]int
type typMapMapIntInt64 map[int]int64
type typMapMapIntFloat32 map[int]float32
type typMapMapIntFloat64 map[int]float64
type typMapMapIntBool map[int]bool
type typMapMapInt64Intf map[int64]interface{}
type typMapMapInt64String map[int64]string
type typMapMapInt64Bytes map[int64][]byte
type typMapMapInt64Uint map[int64]uint
type typMapMapInt64Uint8 map[int64]uint8
type typMapMapInt64Uint64 map[int64]uint64
type typMapMapInt64Int map[int64]int
type typMapMapInt64Int64 map[int64]int64
type typMapMapInt64Float32 map[int64]float32
type typMapMapInt64Float64 map[int64]float64
type typMapMapInt64Bool map[int64]bool

func doTestMammothSlices(t *testing.T, h Handle) {
	var v17va [8]interface{}
	for _, v := range [][]interface{}{nil, {}, {"string-is-an-interface-2", nil, nil, "string-is-an-interface-3"}} {
		var v17v1, v17v2 []interface{}
		var bs17 []byte
		v17v1 = v
		bs17 = testMarshalErr(v17v1, h, t, "enc-slice-v17")
		if v != nil {
			if v == nil {
				v17v2 = nil
			} else {
				v17v2 = make([]interface{}, len(v))
			}
			testUnmarshalErr(v17v2, bs17, h, t, "dec-slice-v17")
			testDeepEqualErr(v17v1, v17v2, t, "equal-slice-v17")
			if v == nil {
				v17v2 = nil
			} else {
				v17v2 = make([]interface{}, len(v))
			}
			testUnmarshalErr(rv4i(v17v2), bs17, h, t, "dec-slice-v17-noaddr") // non-addressable value
			testDeepEqualErr(v17v1, v17v2, t, "equal-slice-v17-noaddr")
		}
		// ...
		bs17 = testMarshalErr(&v17v1, h, t, "enc-slice-v17-p")
		v17v2 = nil
		testUnmarshalErr(&v17v2, bs17, h, t, "dec-slice-v17-p")
		testDeepEqualErr(v17v1, v17v2, t, "equal-slice-v17-p")
		v17va = [8]interface{}{} // clear the array
		v17v2 = v17va[:1:1]
		testUnmarshalErr(&v17v2, bs17, h, t, "dec-slice-v17-p-1")
		testDeepEqualErr(v17v1, v17v2, t, "equal-slice-v17-p-1")
		v17va = [8]interface{}{} // clear the array
		v17v2 = v17va[:len(v17v1):len(v17v1)]
		testUnmarshalErr(&v17v2, bs17, h, t, "dec-slice-v17-p-len")
		testDeepEqualErr(v17v1, v17v2, t, "equal-slice-v17-p-len")
		v17va = [8]interface{}{} // clear the array
		v17v2 = v17va[:]
		testUnmarshalErr(&v17v2, bs17, h, t, "dec-slice-v17-p-cap")
		testDeepEqualErr(v17v1, v17v2, t, "equal-slice-v17-p-cap")
		if len(v17v1) > 1 {
			v17va = [8]interface{}{} // clear the array
			testUnmarshalErr((&v17va)[:len(v17v1)], bs17, h, t, "dec-slice-v17-p-len-noaddr")
			testDeepEqualErr(v17v1, v17va[:len(v17v1)], t, "equal-slice-v17-p-len-noaddr")
			v17va = [8]interface{}{} // clear the array
			testUnmarshalErr((&v17va)[:], bs17, h, t, "dec-slice-v17-p-cap-noaddr")
			testDeepEqualErr(v17v1, v17va[:len(v17v1)], t, "equal-slice-v17-p-cap-noaddr")
		}
		// ...
		var v17v3, v17v4 typMbsSliceIntf
		v17v2 = nil
		if v != nil {
			v17v2 = make([]interface{}, len(v))
		}
		v17v3 = typMbsSliceIntf(v17v1)
		v17v4 = typMbsSliceIntf(v17v2)
		if v != nil {
			bs17 = testMarshalErr(v17v3, h, t, "enc-slice-v17-custom")
			testUnmarshalErr(v17v4, bs17, h, t, "dec-slice-v17-custom")
			testDeepEqualErr(v17v3, v17v4, t, "equal-slice-v17-custom")
		}
		bs17 = testMarshalErr(&v17v3, h, t, "enc-slice-v17-custom-p")
		v17v2 = nil
		v17v4 = typMbsSliceIntf(v17v2)
		testUnmarshalErr(&v17v4, bs17, h, t, "dec-slice-v17-custom-p")
		testDeepEqualErr(v17v3, v17v4, t, "equal-slice-v17-custom-p")
	}
	var v18va [8]string
	for _, v := range [][]string{nil, {}, {"some-string-2", "", "", "some-string-3"}} {
		var v18v1, v18v2 []string
		var bs18 []byte
		v18v1 = v
		bs18 = testMarshalErr(v18v1, h, t, "enc-slice-v18")
		if v != nil {
			if v == nil {
				v18v2 = nil
			} else {
				v18v2 = make([]string, len(v))
			}
			testUnmarshalErr(v18v2, bs18, h, t, "dec-slice-v18")
			testDeepEqualErr(v18v1, v18v2, t, "equal-slice-v18")
			if v == nil {
				v18v2 = nil
			} else {
				v18v2 = make([]string, len(v))
			}
			testUnmarshalErr(rv4i(v18v2), bs18, h, t, "dec-slice-v18-noaddr") // non-addressable value
			testDeepEqualErr(v18v1, v18v2, t, "equal-slice-v18-noaddr")
		}
		// ...
		bs18 = testMarshalErr(&v18v1, h, t, "enc-slice-v18-p")
		v18v2 = nil
		testUnmarshalErr(&v18v2, bs18, h, t, "dec-slice-v18-p")
		testDeepEqualErr(v18v1, v18v2, t, "equal-slice-v18-p")
		v18va = [8]string{} // clear the array
		v18v2 = v18va[:1:1]
		testUnmarshalErr(&v18v2, bs18, h, t, "dec-slice-v18-p-1")
		testDeepEqualErr(v18v1, v18v2, t, "equal-slice-v18-p-1")
		v18va = [8]string{} // clear the array
		v18v2 = v18va[:len(v18v1):len(v18v1)]
		testUnmarshalErr(&v18v2, bs18, h, t, "dec-slice-v18-p-len")
		testDeepEqualErr(v18v1, v18v2, t, "equal-slice-v18-p-len")
		v18va = [8]string{} // clear the array
		v18v2 = v18va[:]
		testUnmarshalErr(&v18v2, bs18, h, t, "dec-slice-v18-p-cap")
		testDeepEqualErr(v18v1, v18v2, t, "equal-slice-v18-p-cap")
		if len(v18v1) > 1 {
			v18va = [8]string{} // clear the array
			testUnmarshalErr((&v18va)[:len(v18v1)], bs18, h, t, "dec-slice-v18-p-len-noaddr")
			testDeepEqualErr(v18v1, v18va[:len(v18v1)], t, "equal-slice-v18-p-len-noaddr")
			v18va = [8]string{} // clear the array
			testUnmarshalErr((&v18va)[:], bs18, h, t, "dec-slice-v18-p-cap-noaddr")
			testDeepEqualErr(v18v1, v18va[:len(v18v1)], t, "equal-slice-v18-p-cap-noaddr")
		}
		// ...
		var v18v3, v18v4 typMbsSliceString
		v18v2 = nil
		if v != nil {
			v18v2 = make([]string, len(v))
		}
		v18v3 = typMbsSliceString(v18v1)
		v18v4 = typMbsSliceString(v18v2)
		if v != nil {
			bs18 = testMarshalErr(v18v3, h, t, "enc-slice-v18-custom")
			testUnmarshalErr(v18v4, bs18, h, t, "dec-slice-v18-custom")
			testDeepEqualErr(v18v3, v18v4, t, "equal-slice-v18-custom")
		}
		bs18 = testMarshalErr(&v18v3, h, t, "enc-slice-v18-custom-p")
		v18v2 = nil
		v18v4 = typMbsSliceString(v18v2)
		testUnmarshalErr(&v18v4, bs18, h, t, "dec-slice-v18-custom-p")
		testDeepEqualErr(v18v3, v18v4, t, "equal-slice-v18-custom-p")
	}
	var v19va [8][]byte
	for _, v := range [][][]byte{nil, {}, {[]byte("some-string-2"), nil, nil, []byte("some-string-3")}} {
		var v19v1, v19v2 [][]byte
		var bs19 []byte
		v19v1 = v
		bs19 = testMarshalErr(v19v1, h, t, "enc-slice-v19")
		if v != nil {
			if v == nil {
				v19v2 = nil
			} else {
				v19v2 = make([][]byte, len(v))
			}
			testUnmarshalErr(v19v2, bs19, h, t, "dec-slice-v19")
			testDeepEqualErr(v19v1, v19v2, t, "equal-slice-v19")
			if v == nil {
				v19v2 = nil
			} else {
				v19v2 = make([][]byte, len(v))
			}
			testUnmarshalErr(rv4i(v19v2), bs19, h, t, "dec-slice-v19-noaddr") // non-addressable value
			testDeepEqualErr(v19v1, v19v2, t, "equal-slice-v19-noaddr")
		}
		// ...
		bs19 = testMarshalErr(&v19v1, h, t, "enc-slice-v19-p")
		v19v2 = nil
		testUnmarshalErr(&v19v2, bs19, h, t, "dec-slice-v19-p")
		testDeepEqualErr(v19v1, v19v2, t, "equal-slice-v19-p")
		v19va = [8][]byte{} // clear the array
		v19v2 = v19va[:1:1]
		testUnmarshalErr(&v19v2, bs19, h, t, "dec-slice-v19-p-1")
		testDeepEqualErr(v19v1, v19v2, t, "equal-slice-v19-p-1")
		v19va = [8][]byte{} // clear the array
		v19v2 = v19va[:len(v19v1):len(v19v1)]
		testUnmarshalErr(&v19v2, bs19, h, t, "dec-slice-v19-p-len")
		testDeepEqualErr(v19v1, v19v2, t, "equal-slice-v19-p-len")
		v19va = [8][]byte{} // clear the array
		v19v2 = v19va[:]
		testUnmarshalErr(&v19v2, bs19, h, t, "dec-slice-v19-p-cap")
		testDeepEqualErr(v19v1, v19v2, t, "equal-slice-v19-p-cap")
		if len(v19v1) > 1 {
			v19va = [8][]byte{} // clear the array
			testUnmarshalErr((&v19va)[:len(v19v1)], bs19, h, t, "dec-slice-v19-p-len-noaddr")
			testDeepEqualErr(v19v1, v19va[:len(v19v1)], t, "equal-slice-v19-p-len-noaddr")
			v19va = [8][]byte{} // clear the array
			testUnmarshalErr((&v19va)[:], bs19, h, t, "dec-slice-v19-p-cap-noaddr")
			testDeepEqualErr(v19v1, v19va[:len(v19v1)], t, "equal-slice-v19-p-cap-noaddr")
		}
		// ...
		var v19v3, v19v4 typMbsSliceBytes
		v19v2 = nil
		if v != nil {
			v19v2 = make([][]byte, len(v))
		}
		v19v3 = typMbsSliceBytes(v19v1)
		v19v4 = typMbsSliceBytes(v19v2)
		if v != nil {
			bs19 = testMarshalErr(v19v3, h, t, "enc-slice-v19-custom")
			testUnmarshalErr(v19v4, bs19, h, t, "dec-slice-v19-custom")
			testDeepEqualErr(v19v3, v19v4, t, "equal-slice-v19-custom")
		}
		bs19 = testMarshalErr(&v19v3, h, t, "enc-slice-v19-custom-p")
		v19v2 = nil
		v19v4 = typMbsSliceBytes(v19v2)
		testUnmarshalErr(&v19v4, bs19, h, t, "dec-slice-v19-custom-p")
		testDeepEqualErr(v19v3, v19v4, t, "equal-slice-v19-custom-p")
	}
	var v20va [8]float32
	for _, v := range [][]float32{nil, {}, {22.2, 0, 0, 33.3e3}} {
		var v20v1, v20v2 []float32
		var bs20 []byte
		v20v1 = v
		bs20 = testMarshalErr(v20v1, h, t, "enc-slice-v20")
		if v != nil {
			if v == nil {
				v20v2 = nil
			} else {
				v20v2 = make([]float32, len(v))
			}
			testUnmarshalErr(v20v2, bs20, h, t, "dec-slice-v20")
			testDeepEqualErr(v20v1, v20v2, t, "equal-slice-v20")
			if v == nil {
				v20v2 = nil
			} else {
				v20v2 = make([]float32, len(v))
			}
			testUnmarshalErr(rv4i(v20v2), bs20, h, t, "dec-slice-v20-noaddr") // non-addressable value
			testDeepEqualErr(v20v1, v20v2, t, "equal-slice-v20-noaddr")
		}
		// ...
		bs20 = testMarshalErr(&v20v1, h, t, "enc-slice-v20-p")
		v20v2 = nil
		testUnmarshalErr(&v20v2, bs20, h, t, "dec-slice-v20-p")
		testDeepEqualErr(v20v1, v20v2, t, "equal-slice-v20-p")
		v20va = [8]float32{} // clear the array
		v20v2 = v20va[:1:1]
		testUnmarshalErr(&v20v2, bs20, h, t, "dec-slice-v20-p-1")
		testDeepEqualErr(v20v1, v20v2, t, "equal-slice-v20-p-1")
		v20va = [8]float32{} // clear the array
		v20v2 = v20va[:len(v20v1):len(v20v1)]
		testUnmarshalErr(&v20v2, bs20, h, t, "dec-slice-v20-p-len")
		testDeepEqualErr(v20v1, v20v2, t, "equal-slice-v20-p-len")
		v20va = [8]float32{} // clear the array
		v20v2 = v20va[:]
		testUnmarshalErr(&v20v2, bs20, h, t, "dec-slice-v20-p-cap")
		testDeepEqualErr(v20v1, v20v2, t, "equal-slice-v20-p-cap")
		if len(v20v1) > 1 {
			v20va = [8]float32{} // clear the array
			testUnmarshalErr((&v20va)[:len(v20v1)], bs20, h, t, "dec-slice-v20-p-len-noaddr")
			testDeepEqualErr(v20v1, v20va[:len(v20v1)], t, "equal-slice-v20-p-len-noaddr")
			v20va = [8]float32{} // clear the array
			testUnmarshalErr((&v20va)[:], bs20, h, t, "dec-slice-v20-p-cap-noaddr")
			testDeepEqualErr(v20v1, v20va[:len(v20v1)], t, "equal-slice-v20-p-cap-noaddr")
		}
		// ...
		var v20v3, v20v4 typMbsSliceFloat32
		v20v2 = nil
		if v != nil {
			v20v2 = make([]float32, len(v))
		}
		v20v3 = typMbsSliceFloat32(v20v1)
		v20v4 = typMbsSliceFloat32(v20v2)
		if v != nil {
			bs20 = testMarshalErr(v20v3, h, t, "enc-slice-v20-custom")
			testUnmarshalErr(v20v4, bs20, h, t, "dec-slice-v20-custom")
			testDeepEqualErr(v20v3, v20v4, t, "equal-slice-v20-custom")
		}
		bs20 = testMarshalErr(&v20v3, h, t, "enc-slice-v20-custom-p")
		v20v2 = nil
		v20v4 = typMbsSliceFloat32(v20v2)
		testUnmarshalErr(&v20v4, bs20, h, t, "dec-slice-v20-custom-p")
		testDeepEqualErr(v20v3, v20v4, t, "equal-slice-v20-custom-p")
	}
	var v21va [8]float64
	for _, v := range [][]float64{nil, {}, {11.1, 0, 0, 22.2}} {
		var v21v1, v21v2 []float64
		var bs21 []byte
		v21v1 = v
		bs21 = testMarshalErr(v21v1, h, t, "enc-slice-v21")
		if v != nil {
			if v == nil {
				v21v2 = nil
			} else {
				v21v2 = make([]float64, len(v))
			}
			testUnmarshalErr(v21v2, bs21, h, t, "dec-slice-v21")
			testDeepEqualErr(v21v1, v21v2, t, "equal-slice-v21")
			if v == nil {
				v21v2 = nil
			} else {
				v21v2 = make([]float64, len(v))
			}
			testUnmarshalErr(rv4i(v21v2), bs21, h, t, "dec-slice-v21-noaddr") // non-addressable value
			testDeepEqualErr(v21v1, v21v2, t, "equal-slice-v21-noaddr")
		}
		// ...
		bs21 = testMarshalErr(&v21v1, h, t, "enc-slice-v21-p")
		v21v2 = nil
		testUnmarshalErr(&v21v2, bs21, h, t, "dec-slice-v21-p")
		testDeepEqualErr(v21v1, v21v2, t, "equal-slice-v21-p")
		v21va = [8]float64{} // clear the array
		v21v2 = v21va[:1:1]
		testUnmarshalErr(&v21v2, bs21, h, t, "dec-slice-v21-p-1")
		testDeepEqualErr(v21v1, v21v2, t, "equal-slice-v21-p-1")
		v21va = [8]float64{} // clear the array
		v21v2 = v21va[:len(v21v1):len(v21v1)]
		testUnmarshalErr(&v21v2, bs21, h, t, "dec-slice-v21-p-len")
		testDeepEqualErr(v21v1, v21v2, t, "equal-slice-v21-p-len")
		v21va = [8]float64{} // clear the array
		v21v2 = v21va[:]
		testUnmarshalErr(&v21v2, bs21, h, t, "dec-slice-v21-p-cap")
		testDeepEqualErr(v21v1, v21v2, t, "equal-slice-v21-p-cap")
		if len(v21v1) > 1 {
			v21va = [8]float64{} // clear the array
			testUnmarshalErr((&v21va)[:len(v21v1)], bs21, h, t, "dec-slice-v21-p-len-noaddr")
			testDeepEqualErr(v21v1, v21va[:len(v21v1)], t, "equal-slice-v21-p-len-noaddr")
			v21va = [8]float64{} // clear the array
			testUnmarshalErr((&v21va)[:], bs21, h, t, "dec-slice-v21-p-cap-noaddr")
			testDeepEqualErr(v21v1, v21va[:len(v21v1)], t, "equal-slice-v21-p-cap-noaddr")
		}
		// ...
		var v21v3, v21v4 typMbsSliceFloat64
		v21v2 = nil
		if v != nil {
			v21v2 = make([]float64, len(v))
		}
		v21v3 = typMbsSliceFloat64(v21v1)
		v21v4 = typMbsSliceFloat64(v21v2)
		if v != nil {
			bs21 = testMarshalErr(v21v3, h, t, "enc-slice-v21-custom")
			testUnmarshalErr(v21v4, bs21, h, t, "dec-slice-v21-custom")
			testDeepEqualErr(v21v3, v21v4, t, "equal-slice-v21-custom")
		}
		bs21 = testMarshalErr(&v21v3, h, t, "enc-slice-v21-custom-p")
		v21v2 = nil
		v21v4 = typMbsSliceFloat64(v21v2)
		testUnmarshalErr(&v21v4, bs21, h, t, "dec-slice-v21-custom-p")
		testDeepEqualErr(v21v3, v21v4, t, "equal-slice-v21-custom-p")
	}
	var v22va [8]uint
	for _, v := range [][]uint{nil, {}, {77, 0, 0, 127}} {
		var v22v1, v22v2 []uint
		var bs22 []byte
		v22v1 = v
		bs22 = testMarshalErr(v22v1, h, t, "enc-slice-v22")
		if v != nil {
			if v == nil {
				v22v2 = nil
			} else {
				v22v2 = make([]uint, len(v))
			}
			testUnmarshalErr(v22v2, bs22, h, t, "dec-slice-v22")
			testDeepEqualErr(v22v1, v22v2, t, "equal-slice-v22")
			if v == nil {
				v22v2 = nil
			} else {
				v22v2 = make([]uint, len(v))
			}
			testUnmarshalErr(rv4i(v22v2), bs22, h, t, "dec-slice-v22-noaddr") // non-addressable value
			testDeepEqualErr(v22v1, v22v2, t, "equal-slice-v22-noaddr")
		}
		// ...
		bs22 = testMarshalErr(&v22v1, h, t, "enc-slice-v22-p")
		v22v2 = nil
		testUnmarshalErr(&v22v2, bs22, h, t, "dec-slice-v22-p")
		testDeepEqualErr(v22v1, v22v2, t, "equal-slice-v22-p")
		v22va = [8]uint{} // clear the array
		v22v2 = v22va[:1:1]
		testUnmarshalErr(&v22v2, bs22, h, t, "dec-slice-v22-p-1")
		testDeepEqualErr(v22v1, v22v2, t, "equal-slice-v22-p-1")
		v22va = [8]uint{} // clear the array
		v22v2 = v22va[:len(v22v1):len(v22v1)]
		testUnmarshalErr(&v22v2, bs22, h, t, "dec-slice-v22-p-len")
		testDeepEqualErr(v22v1, v22v2, t, "equal-slice-v22-p-len")
		v22va = [8]uint{} // clear the array
		v22v2 = v22va[:]
		testUnmarshalErr(&v22v2, bs22, h, t, "dec-slice-v22-p-cap")
		testDeepEqualErr(v22v1, v22v2, t, "equal-slice-v22-p-cap")
		if len(v22v1) > 1 {
			v22va = [8]uint{} // clear the array
			testUnmarshalErr((&v22va)[:len(v22v1)], bs22, h, t, "dec-slice-v22-p-len-noaddr")
			testDeepEqualErr(v22v1, v22va[:len(v22v1)], t, "equal-slice-v22-p-len-noaddr")
			v22va = [8]uint{} // clear the array
			testUnmarshalErr((&v22va)[:], bs22, h, t, "dec-slice-v22-p-cap-noaddr")
			testDeepEqualErr(v22v1, v22va[:len(v22v1)], t, "equal-slice-v22-p-cap-noaddr")
		}
		// ...
		var v22v3, v22v4 typMbsSliceUint
		v22v2 = nil
		if v != nil {
			v22v2 = make([]uint, len(v))
		}
		v22v3 = typMbsSliceUint(v22v1)
		v22v4 = typMbsSliceUint(v22v2)
		if v != nil {
			bs22 = testMarshalErr(v22v3, h, t, "enc-slice-v22-custom")
			testUnmarshalErr(v22v4, bs22, h, t, "dec-slice-v22-custom")
			testDeepEqualErr(v22v3, v22v4, t, "equal-slice-v22-custom")
		}
		bs22 = testMarshalErr(&v22v3, h, t, "enc-slice-v22-custom-p")
		v22v2 = nil
		v22v4 = typMbsSliceUint(v22v2)
		testUnmarshalErr(&v22v4, bs22, h, t, "dec-slice-v22-custom-p")
		testDeepEqualErr(v22v3, v22v4, t, "equal-slice-v22-custom-p")
	}
	var v23va [8]uint16
	for _, v := range [][]uint16{nil, {}, {111, 0, 0, 77}} {
		var v23v1, v23v2 []uint16
		var bs23 []byte
		v23v1 = v
		bs23 = testMarshalErr(v23v1, h, t, "enc-slice-v23")
		if v != nil {
			if v == nil {
				v23v2 = nil
			} else {
				v23v2 = make([]uint16, len(v))
			}
			testUnmarshalErr(v23v2, bs23, h, t, "dec-slice-v23")
			testDeepEqualErr(v23v1, v23v2, t, "equal-slice-v23")
			if v == nil {
				v23v2 = nil
			} else {
				v23v2 = make([]uint16, len(v))
			}
			testUnmarshalErr(rv4i(v23v2), bs23, h, t, "dec-slice-v23-noaddr") // non-addressable value
			testDeepEqualErr(v23v1, v23v2, t, "equal-slice-v23-noaddr")
		}
		// ...
		bs23 = testMarshalErr(&v23v1, h, t, "enc-slice-v23-p")
		v23v2 = nil
		testUnmarshalErr(&v23v2, bs23, h, t, "dec-slice-v23-p")
		testDeepEqualErr(v23v1, v23v2, t, "equal-slice-v23-p")
		v23va = [8]uint16{} // clear the array
		v23v2 = v23va[:1:1]
		testUnmarshalErr(&v23v2, bs23, h, t, "dec-slice-v23-p-1")
		testDeepEqualErr(v23v1, v23v2, t, "equal-slice-v23-p-1")
		v23va = [8]uint16{} // clear the array
		v23v2 = v23va[:len(v23v1):len(v23v1)]
		testUnmarshalErr(&v23v2, bs23, h, t, "dec-slice-v23-p-len")
		testDeepEqualErr(v23v1, v23v2, t, "equal-slice-v23-p-len")
		v23va = [8]uint16{} // clear the array
		v23v2 = v23va[:]
		testUnmarshalErr(&v23v2, bs23, h, t, "dec-slice-v23-p-cap")
		testDeepEqualErr(v23v1, v23v2, t, "equal-slice-v23-p-cap")
		if len(v23v1) > 1 {
			v23va = [8]uint16{} // clear the array
			testUnmarshalErr((&v23va)[:len(v23v1)], bs23, h, t, "dec-slice-v23-p-len-noaddr")
			testDeepEqualErr(v23v1, v23va[:len(v23v1)], t, "equal-slice-v23-p-len-noaddr")
			v23va = [8]uint16{} // clear the array
			testUnmarshalErr((&v23va)[:], bs23, h, t, "dec-slice-v23-p-cap-noaddr")
			testDeepEqualErr(v23v1, v23va[:len(v23v1)], t, "equal-slice-v23-p-cap-noaddr")
		}
		// ...
		var v23v3, v23v4 typMbsSliceUint16
		v23v2 = nil
		if v != nil {
			v23v2 = make([]uint16, len(v))
		}
		v23v3 = typMbsSliceUint16(v23v1)
		v23v4 = typMbsSliceUint16(v23v2)
		if v != nil {
			bs23 = testMarshalErr(v23v3, h, t, "enc-slice-v23-custom")
			testUnmarshalErr(v23v4, bs23, h, t, "dec-slice-v23-custom")
			testDeepEqualErr(v23v3, v23v4, t, "equal-slice-v23-custom")
		}
		bs23 = testMarshalErr(&v23v3, h, t, "enc-slice-v23-custom-p")
		v23v2 = nil
		v23v4 = typMbsSliceUint16(v23v2)
		testUnmarshalErr(&v23v4, bs23, h, t, "dec-slice-v23-custom-p")
		testDeepEqualErr(v23v3, v23v4, t, "equal-slice-v23-custom-p")
	}
	var v24va [8]uint32
	for _, v := range [][]uint32{nil, {}, {127, 0, 0, 111}} {
		var v24v1, v24v2 []uint32
		var bs24 []byte
		v24v1 = v
		bs24 = testMarshalErr(v24v1, h, t, "enc-slice-v24")
		if v != nil {
			if v == nil {
				v24v2 = nil
			} else {
				v24v2 = make([]uint32, len(v))
			}
			testUnmarshalErr(v24v2, bs24, h, t, "dec-slice-v24")
			testDeepEqualErr(v24v1, v24v2, t, "equal-slice-v24")
			if v == nil {
				v24v2 = nil
			} else {
				v24v2 = make([]uint32, len(v))
			}
			testUnmarshalErr(rv4i(v24v2), bs24, h, t, "dec-slice-v24-noaddr") // non-addressable value
			testDeepEqualErr(v24v1, v24v2, t, "equal-slice-v24-noaddr")
		}
		// ...
		bs24 = testMarshalErr(&v24v1, h, t, "enc-slice-v24-p")
		v24v2 = nil
		testUnmarshalErr(&v24v2, bs24, h, t, "dec-slice-v24-p")
		testDeepEqualErr(v24v1, v24v2, t, "equal-slice-v24-p")
		v24va = [8]uint32{} // clear the array
		v24v2 = v24va[:1:1]
		testUnmarshalErr(&v24v2, bs24, h, t, "dec-slice-v24-p-1")
		testDeepEqualErr(v24v1, v24v2, t, "equal-slice-v24-p-1")
		v24va = [8]uint32{} // clear the array
		v24v2 = v24va[:len(v24v1):len(v24v1)]
		testUnmarshalErr(&v24v2, bs24, h, t, "dec-slice-v24-p-len")
		testDeepEqualErr(v24v1, v24v2, t, "equal-slice-v24-p-len")
		v24va = [8]uint32{} // clear the array
		v24v2 = v24va[:]
		testUnmarshalErr(&v24v2, bs24, h, t, "dec-slice-v24-p-cap")
		testDeepEqualErr(v24v1, v24v2, t, "equal-slice-v24-p-cap")
		if len(v24v1) > 1 {
			v24va = [8]uint32{} // clear the array
			testUnmarshalErr((&v24va)[:len(v24v1)], bs24, h, t, "dec-slice-v24-p-len-noaddr")
			testDeepEqualErr(v24v1, v24va[:len(v24v1)], t, "equal-slice-v24-p-len-noaddr")
			v24va = [8]uint32{} // clear the array
			testUnmarshalErr((&v24va)[:], bs24, h, t, "dec-slice-v24-p-cap-noaddr")
			testDeepEqualErr(v24v1, v24va[:len(v24v1)], t, "equal-slice-v24-p-cap-noaddr")
		}
		// ...
		var v24v3, v24v4 typMbsSliceUint32
		v24v2 = nil
		if v != nil {
			v24v2 = make([]uint32, len(v))
		}
		v24v3 = typMbsSliceUint32(v24v1)
		v24v4 = typMbsSliceUint32(v24v2)
		if v != nil {
			bs24 = testMarshalErr(v24v3, h, t, "enc-slice-v24-custom")
			testUnmarshalErr(v24v4, bs24, h, t, "dec-slice-v24-custom")
			testDeepEqualErr(v24v3, v24v4, t, "equal-slice-v24-custom")
		}
		bs24 = testMarshalErr(&v24v3, h, t, "enc-slice-v24-custom-p")
		v24v2 = nil
		v24v4 = typMbsSliceUint32(v24v2)
		testUnmarshalErr(&v24v4, bs24, h, t, "dec-slice-v24-custom-p")
		testDeepEqualErr(v24v3, v24v4, t, "equal-slice-v24-custom-p")
	}
	var v25va [8]uint64
	for _, v := range [][]uint64{nil, {}, {77, 0, 0, 127}} {
		var v25v1, v25v2 []uint64
		var bs25 []byte
		v25v1 = v
		bs25 = testMarshalErr(v25v1, h, t, "enc-slice-v25")
		if v != nil {
			if v == nil {
				v25v2 = nil
			} else {
				v25v2 = make([]uint64, len(v))
			}
			testUnmarshalErr(v25v2, bs25, h, t, "dec-slice-v25")
			testDeepEqualErr(v25v1, v25v2, t, "equal-slice-v25")
			if v == nil {
				v25v2 = nil
			} else {
				v25v2 = make([]uint64, len(v))
			}
			testUnmarshalErr(rv4i(v25v2), bs25, h, t, "dec-slice-v25-noaddr") // non-addressable value
			testDeepEqualErr(v25v1, v25v2, t, "equal-slice-v25-noaddr")
		}
		// ...
		bs25 = testMarshalErr(&v25v1, h, t, "enc-slice-v25-p")
		v25v2 = nil
		testUnmarshalErr(&v25v2, bs25, h, t, "dec-slice-v25-p")
		testDeepEqualErr(v25v1, v25v2, t, "equal-slice-v25-p")
		v25va = [8]uint64{} // clear the array
		v25v2 = v25va[:1:1]
		testUnmarshalErr(&v25v2, bs25, h, t, "dec-slice-v25-p-1")
		testDeepEqualErr(v25v1, v25v2, t, "equal-slice-v25-p-1")
		v25va = [8]uint64{} // clear the array
		v25v2 = v25va[:len(v25v1):len(v25v1)]
		testUnmarshalErr(&v25v2, bs25, h, t, "dec-slice-v25-p-len")
		testDeepEqualErr(v25v1, v25v2, t, "equal-slice-v25-p-len")
		v25va = [8]uint64{} // clear the array
		v25v2 = v25va[:]
		testUnmarshalErr(&v25v2, bs25, h, t, "dec-slice-v25-p-cap")
		testDeepEqualErr(v25v1, v25v2, t, "equal-slice-v25-p-cap")
		if len(v25v1) > 1 {
			v25va = [8]uint64{} // clear the array
			testUnmarshalErr((&v25va)[:len(v25v1)], bs25, h, t, "dec-slice-v25-p-len-noaddr")
			testDeepEqualErr(v25v1, v25va[:len(v25v1)], t, "equal-slice-v25-p-len-noaddr")
			v25va = [8]uint64{} // clear the array
			testUnmarshalErr((&v25va)[:], bs25, h, t, "dec-slice-v25-p-cap-noaddr")
			testDeepEqualErr(v25v1, v25va[:len(v25v1)], t, "equal-slice-v25-p-cap-noaddr")
		}
		// ...
		var v25v3, v25v4 typMbsSliceUint64
		v25v2 = nil
		if v != nil {
			v25v2 = make([]uint64, len(v))
		}
		v25v3 = typMbsSliceUint64(v25v1)
		v25v4 = typMbsSliceUint64(v25v2)
		if v != nil {
			bs25 = testMarshalErr(v25v3, h, t, "enc-slice-v25-custom")
			testUnmarshalErr(v25v4, bs25, h, t, "dec-slice-v25-custom")
			testDeepEqualErr(v25v3, v25v4, t, "equal-slice-v25-custom")
		}
		bs25 = testMarshalErr(&v25v3, h, t, "enc-slice-v25-custom-p")
		v25v2 = nil
		v25v4 = typMbsSliceUint64(v25v2)
		testUnmarshalErr(&v25v4, bs25, h, t, "dec-slice-v25-custom-p")
		testDeepEqualErr(v25v3, v25v4, t, "equal-slice-v25-custom-p")
	}
	var v26va [8]int
	for _, v := range [][]int{nil, {}, {111, 0, 0, 77}} {
		var v26v1, v26v2 []int
		var bs26 []byte
		v26v1 = v
		bs26 = testMarshalErr(v26v1, h, t, "enc-slice-v26")
		if v != nil {
			if v == nil {
				v26v2 = nil
			} else {
				v26v2 = make([]int, len(v))
			}
			testUnmarshalErr(v26v2, bs26, h, t, "dec-slice-v26")
			testDeepEqualErr(v26v1, v26v2, t, "equal-slice-v26")
			if v == nil {
				v26v2 = nil
			} else {
				v26v2 = make([]int, len(v))
			}
			testUnmarshalErr(rv4i(v26v2), bs26, h, t, "dec-slice-v26-noaddr") // non-addressable value
			testDeepEqualErr(v26v1, v26v2, t, "equal-slice-v26-noaddr")
		}
		// ...
		bs26 = testMarshalErr(&v26v1, h, t, "enc-slice-v26-p")
		v26v2 = nil
		testUnmarshalErr(&v26v2, bs26, h, t, "dec-slice-v26-p")
		testDeepEqualErr(v26v1, v26v2, t, "equal-slice-v26-p")
		v26va = [8]int{} // clear the array
		v26v2 = v26va[:1:1]
		testUnmarshalErr(&v26v2, bs26, h, t, "dec-slice-v26-p-1")
		testDeepEqualErr(v26v1, v26v2, t, "equal-slice-v26-p-1")
		v26va = [8]int{} // clear the array
		v26v2 = v26va[:len(v26v1):len(v26v1)]
		testUnmarshalErr(&v26v2, bs26, h, t, "dec-slice-v26-p-len")
		testDeepEqualErr(v26v1, v26v2, t, "equal-slice-v26-p-len")
		v26va = [8]int{} // clear the array
		v26v2 = v26va[:]
		testUnmarshalErr(&v26v2, bs26, h, t, "dec-slice-v26-p-cap")
		testDeepEqualErr(v26v1, v26v2, t, "equal-slice-v26-p-cap")
		if len(v26v1) > 1 {
			v26va = [8]int{} // clear the array
			testUnmarshalErr((&v26va)[:len(v26v1)], bs26, h, t, "dec-slice-v26-p-len-noaddr")
			testDeepEqualErr(v26v1, v26va[:len(v26v1)], t, "equal-slice-v26-p-len-noaddr")
			v26va = [8]int{} // clear the array
			testUnmarshalErr((&v26va)[:], bs26, h, t, "dec-slice-v26-p-cap-noaddr")
			testDeepEqualErr(v26v1, v26va[:len(v26v1)], t, "equal-slice-v26-p-cap-noaddr")
		}
		// ...
		var v26v3, v26v4 typMbsSliceInt
		v26v2 = nil
		if v != nil {
			v26v2 = make([]int, len(v))
		}
		v26v3 = typMbsSliceInt(v26v1)
		v26v4 = typMbsSliceInt(v26v2)
		if v != nil {
			bs26 = testMarshalErr(v26v3, h, t, "enc-slice-v26-custom")
			testUnmarshalErr(v26v4, bs26, h, t, "dec-slice-v26-custom")
			testDeepEqualErr(v26v3, v26v4, t, "equal-slice-v26-custom")
		}
		bs26 = testMarshalErr(&v26v3, h, t, "enc-slice-v26-custom-p")
		v26v2 = nil
		v26v4 = typMbsSliceInt(v26v2)
		testUnmarshalErr(&v26v4, bs26, h, t, "dec-slice-v26-custom-p")
		testDeepEqualErr(v26v3, v26v4, t, "equal-slice-v26-custom-p")
	}
	var v27va [8]int8
	for _, v := range [][]int8{nil, {}, {127, 0, 0, 111}} {
		var v27v1, v27v2 []int8
		var bs27 []byte
		v27v1 = v
		bs27 = testMarshalErr(v27v1, h, t, "enc-slice-v27")
		if v != nil {
			if v == nil {
				v27v2 = nil
			} else {
				v27v2 = make([]int8, len(v))
			}
			testUnmarshalErr(v27v2, bs27, h, t, "dec-slice-v27")
			testDeepEqualErr(v27v1, v27v2, t, "equal-slice-v27")
			if v == nil {
				v27v2 = nil
			} else {
				v27v2 = make([]int8, len(v))
			}
			testUnmarshalErr(rv4i(v27v2), bs27, h, t, "dec-slice-v27-noaddr") // non-addressable value
			testDeepEqualErr(v27v1, v27v2, t, "equal-slice-v27-noaddr")
		}
		// ...
		bs27 = testMarshalErr(&v27v1, h, t, "enc-slice-v27-p")
		v27v2 = nil
		testUnmarshalErr(&v27v2, bs27, h, t, "dec-slice-v27-p")
		testDeepEqualErr(v27v1, v27v2, t, "equal-slice-v27-p")
		v27va = [8]int8{} // clear the array
		v27v2 = v27va[:1:1]
		testUnmarshalErr(&v27v2, bs27, h, t, "dec-slice-v27-p-1")
		testDeepEqualErr(v27v1, v27v2, t, "equal-slice-v27-p-1")
		v27va = [8]int8{} // clear the array
		v27v2 = v27va[:len(v27v1):len(v27v1)]
		testUnmarshalErr(&v27v2, bs27, h, t, "dec-slice-v27-p-len")
		testDeepEqualErr(v27v1, v27v2, t, "equal-slice-v27-p-len")
		v27va = [8]int8{} // clear the array
		v27v2 = v27va[:]
		testUnmarshalErr(&v27v2, bs27, h, t, "dec-slice-v27-p-cap")
		testDeepEqualErr(v27v1, v27v2, t, "equal-slice-v27-p-cap")
		if len(v27v1) > 1 {
			v27va = [8]int8{} // clear the array
			testUnmarshalErr((&v27va)[:len(v27v1)], bs27, h, t, "dec-slice-v27-p-len-noaddr")
			testDeepEqualErr(v27v1, v27va[:len(v27v1)], t, "equal-slice-v27-p-len-noaddr")
			v27va = [8]int8{} // clear the array
			testUnmarshalErr((&v27va)[:], bs27, h, t, "dec-slice-v27-p-cap-noaddr")
			testDeepEqualErr(v27v1, v27va[:len(v27v1)], t, "equal-slice-v27-p-cap-noaddr")
		}
		// ...
		var v27v3, v27v4 typMbsSliceInt8
		v27v2 = nil
		if v != nil {
			v27v2 = make([]int8, len(v))
		}
		v27v3 = typMbsSliceInt8(v27v1)
		v27v4 = typMbsSliceInt8(v27v2)
		if v != nil {
			bs27 = testMarshalErr(v27v3, h, t, "enc-slice-v27-custom")
			testUnmarshalErr(v27v4, bs27, h, t, "dec-slice-v27-custom")
			testDeepEqualErr(v27v3, v27v4, t, "equal-slice-v27-custom")
		}
		bs27 = testMarshalErr(&v27v3, h, t, "enc-slice-v27-custom-p")
		v27v2 = nil
		v27v4 = typMbsSliceInt8(v27v2)
		testUnmarshalErr(&v27v4, bs27, h, t, "dec-slice-v27-custom-p")
		testDeepEqualErr(v27v3, v27v4, t, "equal-slice-v27-custom-p")
	}
	var v28va [8]int16
	for _, v := range [][]int16{nil, {}, {77, 0, 0, 127}} {
		var v28v1, v28v2 []int16
		var bs28 []byte
		v28v1 = v
		bs28 = testMarshalErr(v28v1, h, t, "enc-slice-v28")
		if v != nil {
			if v == nil {
				v28v2 = nil
			} else {
				v28v2 = make([]int16, len(v))
			}
			testUnmarshalErr(v28v2, bs28, h, t, "dec-slice-v28")
			testDeepEqualErr(v28v1, v28v2, t, "equal-slice-v28")
			if v == nil {
				v28v2 = nil
			} else {
				v28v2 = make([]int16, len(v))
			}
			testUnmarshalErr(rv4i(v28v2), bs28, h, t, "dec-slice-v28-noaddr") // non-addressable value
			testDeepEqualErr(v28v1, v28v2, t, "equal-slice-v28-noaddr")
		}
		// ...
		bs28 = testMarshalErr(&v28v1, h, t, "enc-slice-v28-p")
		v28v2 = nil
		testUnmarshalErr(&v28v2, bs28, h, t, "dec-slice-v28-p")
		testDeepEqualErr(v28v1, v28v2, t, "equal-slice-v28-p")
		v28va = [8]int16{} // clear the array
		v28v2 = v28va[:1:1]
		testUnmarshalErr(&v28v2, bs28, h, t, "dec-slice-v28-p-1")
		testDeepEqualErr(v28v1, v28v2, t, "equal-slice-v28-p-1")
		v28va = [8]int16{} // clear the array
		v28v2 = v28va[:len(v28v1):len(v28v1)]
		testUnmarshalErr(&v28v2, bs28, h, t, "dec-slice-v28-p-len")
		testDeepEqualErr(v28v1, v28v2, t, "equal-slice-v28-p-len")
		v28va = [8]int16{} // clear the array
		v28v2 = v28va[:]
		testUnmarshalErr(&v28v2, bs28, h, t, "dec-slice-v28-p-cap")
		testDeepEqualErr(v28v1, v28v2, t, "equal-slice-v28-p-cap")
		if len(v28v1) > 1 {
			v28va = [8]int16{} // clear the array
			testUnmarshalErr((&v28va)[:len(v28v1)], bs28, h, t, "dec-slice-v28-p-len-noaddr")
			testDeepEqualErr(v28v1, v28va[:len(v28v1)], t, "equal-slice-v28-p-len-noaddr")
			v28va = [8]int16{} // clear the array
			testUnmarshalErr((&v28va)[:], bs28, h, t, "dec-slice-v28-p-cap-noaddr")
			testDeepEqualErr(v28v1, v28va[:len(v28v1)], t, "equal-slice-v28-p-cap-noaddr")
		}
		// ...
		var v28v3, v28v4 typMbsSliceInt16
		v28v2 = nil
		if v != nil {
			v28v2 = make([]int16, len(v))
		}
		v28v3 = typMbsSliceInt16(v28v1)
		v28v4 = typMbsSliceInt16(v28v2)
		if v != nil {
			bs28 = testMarshalErr(v28v3, h, t, "enc-slice-v28-custom")
			testUnmarshalErr(v28v4, bs28, h, t, "dec-slice-v28-custom")
			testDeepEqualErr(v28v3, v28v4, t, "equal-slice-v28-custom")
		}
		bs28 = testMarshalErr(&v28v3, h, t, "enc-slice-v28-custom-p")
		v28v2 = nil
		v28v4 = typMbsSliceInt16(v28v2)
		testUnmarshalErr(&v28v4, bs28, h, t, "dec-slice-v28-custom-p")
		testDeepEqualErr(v28v3, v28v4, t, "equal-slice-v28-custom-p")
	}
	var v29va [8]int32
	for _, v := range [][]int32{nil, {}, {111, 0, 0, 77}} {
		var v29v1, v29v2 []int32
		var bs29 []byte
		v29v1 = v
		bs29 = testMarshalErr(v29v1, h, t, "enc-slice-v29")
		if v != nil {
			if v == nil {
				v29v2 = nil
			} else {
				v29v2 = make([]int32, len(v))
			}
			testUnmarshalErr(v29v2, bs29, h, t, "dec-slice-v29")
			testDeepEqualErr(v29v1, v29v2, t, "equal-slice-v29")
			if v == nil {
				v29v2 = nil
			} else {
				v29v2 = make([]int32, len(v))
			}
			testUnmarshalErr(rv4i(v29v2), bs29, h, t, "dec-slice-v29-noaddr") // non-addressable value
			testDeepEqualErr(v29v1, v29v2, t, "equal-slice-v29-noaddr")
		}
		// ...
		bs29 = testMarshalErr(&v29v1, h, t, "enc-slice-v29-p")
		v29v2 = nil
		testUnmarshalErr(&v29v2, bs29, h, t, "dec-slice-v29-p")
		testDeepEqualErr(v29v1, v29v2, t, "equal-slice-v29-p")
		v29va = [8]int32{} // clear the array
		v29v2 = v29va[:1:1]
		testUnmarshalErr(&v29v2, bs29, h, t, "dec-slice-v29-p-1")
		testDeepEqualErr(v29v1, v29v2, t, "equal-slice-v29-p-1")
		v29va = [8]int32{} // clear the array
		v29v2 = v29va[:len(v29v1):len(v29v1)]
		testUnmarshalErr(&v29v2, bs29, h, t, "dec-slice-v29-p-len")
		testDeepEqualErr(v29v1, v29v2, t, "equal-slice-v29-p-len")
		v29va = [8]int32{} // clear the array
		v29v2 = v29va[:]
		testUnmarshalErr(&v29v2, bs29, h, t, "dec-slice-v29-p-cap")
		testDeepEqualErr(v29v1, v29v2, t, "equal-slice-v29-p-cap")
		if len(v29v1) > 1 {
			v29va = [8]int32{} // clear the array
			testUnmarshalErr((&v29va)[:len(v29v1)], bs29, h, t, "dec-slice-v29-p-len-noaddr")
			testDeepEqualErr(v29v1, v29va[:len(v29v1)], t, "equal-slice-v29-p-len-noaddr")
			v29va = [8]int32{} // clear the array
			testUnmarshalErr((&v29va)[:], bs29, h, t, "dec-slice-v29-p-cap-noaddr")
			testDeepEqualErr(v29v1, v29va[:len(v29v1)], t, "equal-slice-v29-p-cap-noaddr")
		}
		// ...
		var v29v3, v29v4 typMbsSliceInt32
		v29v2 = nil
		if v != nil {
			v29v2 = make([]int32, len(v))
		}
		v29v3 = typMbsSliceInt32(v29v1)
		v29v4 = typMbsSliceInt32(v29v2)
		if v != nil {
			bs29 = testMarshalErr(v29v3, h, t, "enc-slice-v29-custom")
			testUnmarshalErr(v29v4, bs29, h, t, "dec-slice-v29-custom")
			testDeepEqualErr(v29v3, v29v4, t, "equal-slice-v29-custom")
		}
		bs29 = testMarshalErr(&v29v3, h, t, "enc-slice-v29-custom-p")
		v29v2 = nil
		v29v4 = typMbsSliceInt32(v29v2)
		testUnmarshalErr(&v29v4, bs29, h, t, "dec-slice-v29-custom-p")
		testDeepEqualErr(v29v3, v29v4, t, "equal-slice-v29-custom-p")
	}
	var v30va [8]int64
	for _, v := range [][]int64{nil, {}, {127, 0, 0, 111}} {
		var v30v1, v30v2 []int64
		var bs30 []byte
		v30v1 = v
		bs30 = testMarshalErr(v30v1, h, t, "enc-slice-v30")
		if v != nil {
			if v == nil {
				v30v2 = nil
			} else {
				v30v2 = make([]int64, len(v))
			}
			testUnmarshalErr(v30v2, bs30, h, t, "dec-slice-v30")
			testDeepEqualErr(v30v1, v30v2, t, "equal-slice-v30")
			if v == nil {
				v30v2 = nil
			} else {
				v30v2 = make([]int64, len(v))
			}
			testUnmarshalErr(rv4i(v30v2), bs30, h, t, "dec-slice-v30-noaddr") // non-addressable value
			testDeepEqualErr(v30v1, v30v2, t, "equal-slice-v30-noaddr")
		}
		// ...
		bs30 = testMarshalErr(&v30v1, h, t, "enc-slice-v30-p")
		v30v2 = nil
		testUnmarshalErr(&v30v2, bs30, h, t, "dec-slice-v30-p")
		testDeepEqualErr(v30v1, v30v2, t, "equal-slice-v30-p")
		v30va = [8]int64{} // clear the array
		v30v2 = v30va[:1:1]
		testUnmarshalErr(&v30v2, bs30, h, t, "dec-slice-v30-p-1")
		testDeepEqualErr(v30v1, v30v2, t, "equal-slice-v30-p-1")
		v30va = [8]int64{} // clear the array
		v30v2 = v30va[:len(v30v1):len(v30v1)]
		testUnmarshalErr(&v30v2, bs30, h, t, "dec-slice-v30-p-len")
		testDeepEqualErr(v30v1, v30v2, t, "equal-slice-v30-p-len")
		v30va = [8]int64{} // clear the array
		v30v2 = v30va[:]
		testUnmarshalErr(&v30v2, bs30, h, t, "dec-slice-v30-p-cap")
		testDeepEqualErr(v30v1, v30v2, t, "equal-slice-v30-p-cap")
		if len(v30v1) > 1 {
			v30va = [8]int64{} // clear the array
			testUnmarshalErr((&v30va)[:len(v30v1)], bs30, h, t, "dec-slice-v30-p-len-noaddr")
			testDeepEqualErr(v30v1, v30va[:len(v30v1)], t, "equal-slice-v30-p-len-noaddr")
			v30va = [8]int64{} // clear the array
			testUnmarshalErr((&v30va)[:], bs30, h, t, "dec-slice-v30-p-cap-noaddr")
			testDeepEqualErr(v30v1, v30va[:len(v30v1)], t, "equal-slice-v30-p-cap-noaddr")
		}
		// ...
		var v30v3, v30v4 typMbsSliceInt64
		v30v2 = nil
		if v != nil {
			v30v2 = make([]int64, len(v))
		}
		v30v3 = typMbsSliceInt64(v30v1)
		v30v4 = typMbsSliceInt64(v30v2)
		if v != nil {
			bs30 = testMarshalErr(v30v3, h, t, "enc-slice-v30-custom")
			testUnmarshalErr(v30v4, bs30, h, t, "dec-slice-v30-custom")
			testDeepEqualErr(v30v3, v30v4, t, "equal-slice-v30-custom")
		}
		bs30 = testMarshalErr(&v30v3, h, t, "enc-slice-v30-custom-p")
		v30v2 = nil
		v30v4 = typMbsSliceInt64(v30v2)
		testUnmarshalErr(&v30v4, bs30, h, t, "dec-slice-v30-custom-p")
		testDeepEqualErr(v30v3, v30v4, t, "equal-slice-v30-custom-p")
	}
	var v31va [8]bool
	for _, v := range [][]bool{nil, {}, {false, false, false, true}} {
		var v31v1, v31v2 []bool
		var bs31 []byte
		v31v1 = v
		bs31 = testMarshalErr(v31v1, h, t, "enc-slice-v31")
		if v != nil {
			if v == nil {
				v31v2 = nil
			} else {
				v31v2 = make([]bool, len(v))
			}
			testUnmarshalErr(v31v2, bs31, h, t, "dec-slice-v31")
			testDeepEqualErr(v31v1, v31v2, t, "equal-slice-v31")
			if v == nil {
				v31v2 = nil
			} else {
				v31v2 = make([]bool, len(v))
			}
			testUnmarshalErr(rv4i(v31v2), bs31, h, t, "dec-slice-v31-noaddr") // non-addressable value
			testDeepEqualErr(v31v1, v31v2, t, "equal-slice-v31-noaddr")
		}
		// ...
		bs31 = testMarshalErr(&v31v1, h, t, "enc-slice-v31-p")
		v31v2 = nil
		testUnmarshalErr(&v31v2, bs31, h, t, "dec-slice-v31-p")
		testDeepEqualErr(v31v1, v31v2, t, "equal-slice-v31-p")
		v31va = [8]bool{} // clear the array
		v31v2 = v31va[:1:1]
		testUnmarshalErr(&v31v2, bs31, h, t, "dec-slice-v31-p-1")
		testDeepEqualErr(v31v1, v31v2, t, "equal-slice-v31-p-1")
		v31va = [8]bool{} // clear the array
		v31v2 = v31va[:len(v31v1):len(v31v1)]
		testUnmarshalErr(&v31v2, bs31, h, t, "dec-slice-v31-p-len")
		testDeepEqualErr(v31v1, v31v2, t, "equal-slice-v31-p-len")
		v31va = [8]bool{} // clear the array
		v31v2 = v31va[:]
		testUnmarshalErr(&v31v2, bs31, h, t, "dec-slice-v31-p-cap")
		testDeepEqualErr(v31v1, v31v2, t, "equal-slice-v31-p-cap")
		if len(v31v1) > 1 {
			v31va = [8]bool{} // clear the array
			testUnmarshalErr((&v31va)[:len(v31v1)], bs31, h, t, "dec-slice-v31-p-len-noaddr")
			testDeepEqualErr(v31v1, v31va[:len(v31v1)], t, "equal-slice-v31-p-len-noaddr")
			v31va = [8]bool{} // clear the array
			testUnmarshalErr((&v31va)[:], bs31, h, t, "dec-slice-v31-p-cap-noaddr")
			testDeepEqualErr(v31v1, v31va[:len(v31v1)], t, "equal-slice-v31-p-cap-noaddr")
		}
		// ...
		var v31v3, v31v4 typMbsSliceBool
		v31v2 = nil
		if v != nil {
			v31v2 = make([]bool, len(v))
		}
		v31v3 = typMbsSliceBool(v31v1)
		v31v4 = typMbsSliceBool(v31v2)
		if v != nil {
			bs31 = testMarshalErr(v31v3, h, t, "enc-slice-v31-custom")
			testUnmarshalErr(v31v4, bs31, h, t, "dec-slice-v31-custom")
			testDeepEqualErr(v31v3, v31v4, t, "equal-slice-v31-custom")
		}
		bs31 = testMarshalErr(&v31v3, h, t, "enc-slice-v31-custom-p")
		v31v2 = nil
		v31v4 = typMbsSliceBool(v31v2)
		testUnmarshalErr(&v31v4, bs31, h, t, "dec-slice-v31-custom-p")
		testDeepEqualErr(v31v3, v31v4, t, "equal-slice-v31-custom-p")
	}

}

func doTestMammothMaps(t *testing.T, h Handle) {
	for _, v := range []map[string]interface{}{nil, {}, {"some-string-1": nil, "some-string-2": "string-is-an-interface-1"}} {
		// fmt.Printf(">>>> running mammoth map v32: %v\n", v)
		var v32v1, v32v2 map[string]interface{}
		var bs32 []byte
		v32v1 = v
		bs32 = testMarshalErr(v32v1, h, t, "enc-map-v32")
		if v != nil {
			if v == nil {
				v32v2 = nil
			} else {
				v32v2 = make(map[string]interface{}, len(v))
			} // reset map
			testUnmarshalErr(v32v2, bs32, h, t, "dec-map-v32")
			testDeepEqualErr(v32v1, v32v2, t, "equal-map-v32")
			if v == nil {
				v32v2 = nil
			} else {
				v32v2 = make(map[string]interface{}, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v32v2), bs32, h, t, "dec-map-v32-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v32v1, v32v2, t, "equal-map-v32-noaddr")
		}
		if v == nil {
			v32v2 = nil
		} else {
			v32v2 = make(map[string]interface{}, len(v))
		} // reset map
		testUnmarshalErr(&v32v2, bs32, h, t, "dec-map-v32-p-len")
		testDeepEqualErr(v32v1, v32v2, t, "equal-map-v32-p-len")
		bs32 = testMarshalErr(&v32v1, h, t, "enc-map-v32-p")
		v32v2 = nil
		testUnmarshalErr(&v32v2, bs32, h, t, "dec-map-v32-p-nil")
		testDeepEqualErr(v32v1, v32v2, t, "equal-map-v32-p-nil")
		// ...
		if v == nil {
			v32v2 = nil
		} else {
			v32v2 = make(map[string]interface{}, len(v))
		} // reset map
		var v32v3, v32v4 typMapMapStringIntf
		v32v3 = typMapMapStringIntf(v32v1)
		v32v4 = typMapMapStringIntf(v32v2)
		if v != nil {
			bs32 = testMarshalErr(v32v3, h, t, "enc-map-v32-custom")
			testUnmarshalErr(v32v4, bs32, h, t, "dec-map-v32-p-len")
			testDeepEqualErr(v32v3, v32v4, t, "equal-map-v32-p-len")
		}
	}
	for _, v := range []map[string]string{nil, {}, {"some-string-3": "", "some-string-1": "some-string-2"}} {
		// fmt.Printf(">>>> running mammoth map v33: %v\n", v)
		var v33v1, v33v2 map[string]string
		var bs33 []byte
		v33v1 = v
		bs33 = testMarshalErr(v33v1, h, t, "enc-map-v33")
		if v != nil {
			if v == nil {
				v33v2 = nil
			} else {
				v33v2 = make(map[string]string, len(v))
			} // reset map
			testUnmarshalErr(v33v2, bs33, h, t, "dec-map-v33")
			testDeepEqualErr(v33v1, v33v2, t, "equal-map-v33")
			if v == nil {
				v33v2 = nil
			} else {
				v33v2 = make(map[string]string, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v33v2), bs33, h, t, "dec-map-v33-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v33v1, v33v2, t, "equal-map-v33-noaddr")
		}
		if v == nil {
			v33v2 = nil
		} else {
			v33v2 = make(map[string]string, len(v))
		} // reset map
		testUnmarshalErr(&v33v2, bs33, h, t, "dec-map-v33-p-len")
		testDeepEqualErr(v33v1, v33v2, t, "equal-map-v33-p-len")
		bs33 = testMarshalErr(&v33v1, h, t, "enc-map-v33-p")
		v33v2 = nil
		testUnmarshalErr(&v33v2, bs33, h, t, "dec-map-v33-p-nil")
		testDeepEqualErr(v33v1, v33v2, t, "equal-map-v33-p-nil")
		// ...
		if v == nil {
			v33v2 = nil
		} else {
			v33v2 = make(map[string]string, len(v))
		} // reset map
		var v33v3, v33v4 typMapMapStringString
		v33v3 = typMapMapStringString(v33v1)
		v33v4 = typMapMapStringString(v33v2)
		if v != nil {
			bs33 = testMarshalErr(v33v3, h, t, "enc-map-v33-custom")
			testUnmarshalErr(v33v4, bs33, h, t, "dec-map-v33-p-len")
			testDeepEqualErr(v33v3, v33v4, t, "equal-map-v33-p-len")
		}
	}
	for _, v := range []map[string][]byte{nil, {}, {"some-string-3": nil, "some-string-1": []byte("some-string-1")}} {
		// fmt.Printf(">>>> running mammoth map v34: %v\n", v)
		var v34v1, v34v2 map[string][]byte
		var bs34 []byte
		v34v1 = v
		bs34 = testMarshalErr(v34v1, h, t, "enc-map-v34")
		if v != nil {
			if v == nil {
				v34v2 = nil
			} else {
				v34v2 = make(map[string][]byte, len(v))
			} // reset map
			testUnmarshalErr(v34v2, bs34, h, t, "dec-map-v34")
			testDeepEqualErr(v34v1, v34v2, t, "equal-map-v34")
			if v == nil {
				v34v2 = nil
			} else {
				v34v2 = make(map[string][]byte, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v34v2), bs34, h, t, "dec-map-v34-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v34v1, v34v2, t, "equal-map-v34-noaddr")
		}
		if v == nil {
			v34v2 = nil
		} else {
			v34v2 = make(map[string][]byte, len(v))
		} // reset map
		testUnmarshalErr(&v34v2, bs34, h, t, "dec-map-v34-p-len")
		testDeepEqualErr(v34v1, v34v2, t, "equal-map-v34-p-len")
		bs34 = testMarshalErr(&v34v1, h, t, "enc-map-v34-p")
		v34v2 = nil
		testUnmarshalErr(&v34v2, bs34, h, t, "dec-map-v34-p-nil")
		testDeepEqualErr(v34v1, v34v2, t, "equal-map-v34-p-nil")
		// ...
		if v == nil {
			v34v2 = nil
		} else {
			v34v2 = make(map[string][]byte, len(v))
		} // reset map
		var v34v3, v34v4 typMapMapStringBytes
		v34v3 = typMapMapStringBytes(v34v1)
		v34v4 = typMapMapStringBytes(v34v2)
		if v != nil {
			bs34 = testMarshalErr(v34v3, h, t, "enc-map-v34-custom")
			testUnmarshalErr(v34v4, bs34, h, t, "dec-map-v34-p-len")
			testDeepEqualErr(v34v3, v34v4, t, "equal-map-v34-p-len")
		}
	}
	for _, v := range []map[string]uint{nil, {}, {"some-string-2": 0, "some-string-3": 77}} {
		// fmt.Printf(">>>> running mammoth map v35: %v\n", v)
		var v35v1, v35v2 map[string]uint
		var bs35 []byte
		v35v1 = v
		bs35 = testMarshalErr(v35v1, h, t, "enc-map-v35")
		if v != nil {
			if v == nil {
				v35v2 = nil
			} else {
				v35v2 = make(map[string]uint, len(v))
			} // reset map
			testUnmarshalErr(v35v2, bs35, h, t, "dec-map-v35")
			testDeepEqualErr(v35v1, v35v2, t, "equal-map-v35")
			if v == nil {
				v35v2 = nil
			} else {
				v35v2 = make(map[string]uint, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v35v2), bs35, h, t, "dec-map-v35-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v35v1, v35v2, t, "equal-map-v35-noaddr")
		}
		if v == nil {
			v35v2 = nil
		} else {
			v35v2 = make(map[string]uint, len(v))
		} // reset map
		testUnmarshalErr(&v35v2, bs35, h, t, "dec-map-v35-p-len")
		testDeepEqualErr(v35v1, v35v2, t, "equal-map-v35-p-len")
		bs35 = testMarshalErr(&v35v1, h, t, "enc-map-v35-p")
		v35v2 = nil
		testUnmarshalErr(&v35v2, bs35, h, t, "dec-map-v35-p-nil")
		testDeepEqualErr(v35v1, v35v2, t, "equal-map-v35-p-nil")
		// ...
		if v == nil {
			v35v2 = nil
		} else {
			v35v2 = make(map[string]uint, len(v))
		} // reset map
		var v35v3, v35v4 typMapMapStringUint
		v35v3 = typMapMapStringUint(v35v1)
		v35v4 = typMapMapStringUint(v35v2)
		if v != nil {
			bs35 = testMarshalErr(v35v3, h, t, "enc-map-v35-custom")
			testUnmarshalErr(v35v4, bs35, h, t, "dec-map-v35-p-len")
			testDeepEqualErr(v35v3, v35v4, t, "equal-map-v35-p-len")
		}
	}
	for _, v := range []map[string]uint8{nil, {}, {"some-string-1": 0, "some-string-2": 127}} {
		// fmt.Printf(">>>> running mammoth map v36: %v\n", v)
		var v36v1, v36v2 map[string]uint8
		var bs36 []byte
		v36v1 = v
		bs36 = testMarshalErr(v36v1, h, t, "enc-map-v36")
		if v != nil {
			if v == nil {
				v36v2 = nil
			} else {
				v36v2 = make(map[string]uint8, len(v))
			} // reset map
			testUnmarshalErr(v36v2, bs36, h, t, "dec-map-v36")
			testDeepEqualErr(v36v1, v36v2, t, "equal-map-v36")
			if v == nil {
				v36v2 = nil
			} else {
				v36v2 = make(map[string]uint8, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v36v2), bs36, h, t, "dec-map-v36-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v36v1, v36v2, t, "equal-map-v36-noaddr")
		}
		if v == nil {
			v36v2 = nil
		} else {
			v36v2 = make(map[string]uint8, len(v))
		} // reset map
		testUnmarshalErr(&v36v2, bs36, h, t, "dec-map-v36-p-len")
		testDeepEqualErr(v36v1, v36v2, t, "equal-map-v36-p-len")
		bs36 = testMarshalErr(&v36v1, h, t, "enc-map-v36-p")
		v36v2 = nil
		testUnmarshalErr(&v36v2, bs36, h, t, "dec-map-v36-p-nil")
		testDeepEqualErr(v36v1, v36v2, t, "equal-map-v36-p-nil")
		// ...
		if v == nil {
			v36v2 = nil
		} else {
			v36v2 = make(map[string]uint8, len(v))
		} // reset map
		var v36v3, v36v4 typMapMapStringUint8
		v36v3 = typMapMapStringUint8(v36v1)
		v36v4 = typMapMapStringUint8(v36v2)
		if v != nil {
			bs36 = testMarshalErr(v36v3, h, t, "enc-map-v36-custom")
			testUnmarshalErr(v36v4, bs36, h, t, "dec-map-v36-p-len")
			testDeepEqualErr(v36v3, v36v4, t, "equal-map-v36-p-len")
		}
	}
	for _, v := range []map[string]uint64{nil, {}, {"some-string-3": 0, "some-string-1": 111}} {
		// fmt.Printf(">>>> running mammoth map v37: %v\n", v)
		var v37v1, v37v2 map[string]uint64
		var bs37 []byte
		v37v1 = v
		bs37 = testMarshalErr(v37v1, h, t, "enc-map-v37")
		if v != nil {
			if v == nil {
				v37v2 = nil
			} else {
				v37v2 = make(map[string]uint64, len(v))
			} // reset map
			testUnmarshalErr(v37v2, bs37, h, t, "dec-map-v37")
			testDeepEqualErr(v37v1, v37v2, t, "equal-map-v37")
			if v == nil {
				v37v2 = nil
			} else {
				v37v2 = make(map[string]uint64, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v37v2), bs37, h, t, "dec-map-v37-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v37v1, v37v2, t, "equal-map-v37-noaddr")
		}
		if v == nil {
			v37v2 = nil
		} else {
			v37v2 = make(map[string]uint64, len(v))
		} // reset map
		testUnmarshalErr(&v37v2, bs37, h, t, "dec-map-v37-p-len")
		testDeepEqualErr(v37v1, v37v2, t, "equal-map-v37-p-len")
		bs37 = testMarshalErr(&v37v1, h, t, "enc-map-v37-p")
		v37v2 = nil
		testUnmarshalErr(&v37v2, bs37, h, t, "dec-map-v37-p-nil")
		testDeepEqualErr(v37v1, v37v2, t, "equal-map-v37-p-nil")
		// ...
		if v == nil {
			v37v2 = nil
		} else {
			v37v2 = make(map[string]uint64, len(v))
		} // reset map
		var v37v3, v37v4 typMapMapStringUint64
		v37v3 = typMapMapStringUint64(v37v1)
		v37v4 = typMapMapStringUint64(v37v2)
		if v != nil {
			bs37 = testMarshalErr(v37v3, h, t, "enc-map-v37-custom")
			testUnmarshalErr(v37v4, bs37, h, t, "dec-map-v37-p-len")
			testDeepEqualErr(v37v3, v37v4, t, "equal-map-v37-p-len")
		}
	}
	for _, v := range []map[string]int{nil, {}, {"some-string-2": 0, "some-string-3": 77}} {
		// fmt.Printf(">>>> running mammoth map v38: %v\n", v)
		var v38v1, v38v2 map[string]int
		var bs38 []byte
		v38v1 = v
		bs38 = testMarshalErr(v38v1, h, t, "enc-map-v38")
		if v != nil {
			if v == nil {
				v38v2 = nil
			} else {
				v38v2 = make(map[string]int, len(v))
			} // reset map
			testUnmarshalErr(v38v2, bs38, h, t, "dec-map-v38")
			testDeepEqualErr(v38v1, v38v2, t, "equal-map-v38")
			if v == nil {
				v38v2 = nil
			} else {
				v38v2 = make(map[string]int, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v38v2), bs38, h, t, "dec-map-v38-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v38v1, v38v2, t, "equal-map-v38-noaddr")
		}
		if v == nil {
			v38v2 = nil
		} else {
			v38v2 = make(map[string]int, len(v))
		} // reset map
		testUnmarshalErr(&v38v2, bs38, h, t, "dec-map-v38-p-len")
		testDeepEqualErr(v38v1, v38v2, t, "equal-map-v38-p-len")
		bs38 = testMarshalErr(&v38v1, h, t, "enc-map-v38-p")
		v38v2 = nil
		testUnmarshalErr(&v38v2, bs38, h, t, "dec-map-v38-p-nil")
		testDeepEqualErr(v38v1, v38v2, t, "equal-map-v38-p-nil")
		// ...
		if v == nil {
			v38v2 = nil
		} else {
			v38v2 = make(map[string]int, len(v))
		} // reset map
		var v38v3, v38v4 typMapMapStringInt
		v38v3 = typMapMapStringInt(v38v1)
		v38v4 = typMapMapStringInt(v38v2)
		if v != nil {
			bs38 = testMarshalErr(v38v3, h, t, "enc-map-v38-custom")
			testUnmarshalErr(v38v4, bs38, h, t, "dec-map-v38-p-len")
			testDeepEqualErr(v38v3, v38v4, t, "equal-map-v38-p-len")
		}
	}
	for _, v := range []map[string]int64{nil, {}, {"some-string-1": 0, "some-string-2": 127}} {
		// fmt.Printf(">>>> running mammoth map v39: %v\n", v)
		var v39v1, v39v2 map[string]int64
		var bs39 []byte
		v39v1 = v
		bs39 = testMarshalErr(v39v1, h, t, "enc-map-v39")
		if v != nil {
			if v == nil {
				v39v2 = nil
			} else {
				v39v2 = make(map[string]int64, len(v))
			} // reset map
			testUnmarshalErr(v39v2, bs39, h, t, "dec-map-v39")
			testDeepEqualErr(v39v1, v39v2, t, "equal-map-v39")
			if v == nil {
				v39v2 = nil
			} else {
				v39v2 = make(map[string]int64, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v39v2), bs39, h, t, "dec-map-v39-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v39v1, v39v2, t, "equal-map-v39-noaddr")
		}
		if v == nil {
			v39v2 = nil
		} else {
			v39v2 = make(map[string]int64, len(v))
		} // reset map
		testUnmarshalErr(&v39v2, bs39, h, t, "dec-map-v39-p-len")
		testDeepEqualErr(v39v1, v39v2, t, "equal-map-v39-p-len")
		bs39 = testMarshalErr(&v39v1, h, t, "enc-map-v39-p")
		v39v2 = nil
		testUnmarshalErr(&v39v2, bs39, h, t, "dec-map-v39-p-nil")
		testDeepEqualErr(v39v1, v39v2, t, "equal-map-v39-p-nil")
		// ...
		if v == nil {
			v39v2 = nil
		} else {
			v39v2 = make(map[string]int64, len(v))
		} // reset map
		var v39v3, v39v4 typMapMapStringInt64
		v39v3 = typMapMapStringInt64(v39v1)
		v39v4 = typMapMapStringInt64(v39v2)
		if v != nil {
			bs39 = testMarshalErr(v39v3, h, t, "enc-map-v39-custom")
			testUnmarshalErr(v39v4, bs39, h, t, "dec-map-v39-p-len")
			testDeepEqualErr(v39v3, v39v4, t, "equal-map-v39-p-len")
		}
	}
	for _, v := range []map[string]float32{nil, {}, {"some-string-3": 0, "some-string-1": 33.3e3}} {
		// fmt.Printf(">>>> running mammoth map v40: %v\n", v)
		var v40v1, v40v2 map[string]float32
		var bs40 []byte
		v40v1 = v
		bs40 = testMarshalErr(v40v1, h, t, "enc-map-v40")
		if v != nil {
			if v == nil {
				v40v2 = nil
			} else {
				v40v2 = make(map[string]float32, len(v))
			} // reset map
			testUnmarshalErr(v40v2, bs40, h, t, "dec-map-v40")
			testDeepEqualErr(v40v1, v40v2, t, "equal-map-v40")
			if v == nil {
				v40v2 = nil
			} else {
				v40v2 = make(map[string]float32, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v40v2), bs40, h, t, "dec-map-v40-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v40v1, v40v2, t, "equal-map-v40-noaddr")
		}
		if v == nil {
			v40v2 = nil
		} else {
			v40v2 = make(map[string]float32, len(v))
		} // reset map
		testUnmarshalErr(&v40v2, bs40, h, t, "dec-map-v40-p-len")
		testDeepEqualErr(v40v1, v40v2, t, "equal-map-v40-p-len")
		bs40 = testMarshalErr(&v40v1, h, t, "enc-map-v40-p")
		v40v2 = nil
		testUnmarshalErr(&v40v2, bs40, h, t, "dec-map-v40-p-nil")
		testDeepEqualErr(v40v1, v40v2, t, "equal-map-v40-p-nil")
		// ...
		if v == nil {
			v40v2 = nil
		} else {
			v40v2 = make(map[string]float32, len(v))
		} // reset map
		var v40v3, v40v4 typMapMapStringFloat32
		v40v3 = typMapMapStringFloat32(v40v1)
		v40v4 = typMapMapStringFloat32(v40v2)
		if v != nil {
			bs40 = testMarshalErr(v40v3, h, t, "enc-map-v40-custom")
			testUnmarshalErr(v40v4, bs40, h, t, "dec-map-v40-p-len")
			testDeepEqualErr(v40v3, v40v4, t, "equal-map-v40-p-len")
		}
	}
	for _, v := range []map[string]float64{nil, {}, {"some-string-2": 0, "some-string-3": 11.1}} {
		// fmt.Printf(">>>> running mammoth map v41: %v\n", v)
		var v41v1, v41v2 map[string]float64
		var bs41 []byte
		v41v1 = v
		bs41 = testMarshalErr(v41v1, h, t, "enc-map-v41")
		if v != nil {
			if v == nil {
				v41v2 = nil
			} else {
				v41v2 = make(map[string]float64, len(v))
			} // reset map
			testUnmarshalErr(v41v2, bs41, h, t, "dec-map-v41")
			testDeepEqualErr(v41v1, v41v2, t, "equal-map-v41")
			if v == nil {
				v41v2 = nil
			} else {
				v41v2 = make(map[string]float64, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v41v2), bs41, h, t, "dec-map-v41-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v41v1, v41v2, t, "equal-map-v41-noaddr")
		}
		if v == nil {
			v41v2 = nil
		} else {
			v41v2 = make(map[string]float64, len(v))
		} // reset map
		testUnmarshalErr(&v41v2, bs41, h, t, "dec-map-v41-p-len")
		testDeepEqualErr(v41v1, v41v2, t, "equal-map-v41-p-len")
		bs41 = testMarshalErr(&v41v1, h, t, "enc-map-v41-p")
		v41v2 = nil
		testUnmarshalErr(&v41v2, bs41, h, t, "dec-map-v41-p-nil")
		testDeepEqualErr(v41v1, v41v2, t, "equal-map-v41-p-nil")
		// ...
		if v == nil {
			v41v2 = nil
		} else {
			v41v2 = make(map[string]float64, len(v))
		} // reset map
		var v41v3, v41v4 typMapMapStringFloat64
		v41v3 = typMapMapStringFloat64(v41v1)
		v41v4 = typMapMapStringFloat64(v41v2)
		if v != nil {
			bs41 = testMarshalErr(v41v3, h, t, "enc-map-v41-custom")
			testUnmarshalErr(v41v4, bs41, h, t, "dec-map-v41-p-len")
			testDeepEqualErr(v41v3, v41v4, t, "equal-map-v41-p-len")
		}
	}
	for _, v := range []map[string]bool{nil, {}, {"some-string-1": false, "some-string-2": true}} {
		// fmt.Printf(">>>> running mammoth map v42: %v\n", v)
		var v42v1, v42v2 map[string]bool
		var bs42 []byte
		v42v1 = v
		bs42 = testMarshalErr(v42v1, h, t, "enc-map-v42")
		if v != nil {
			if v == nil {
				v42v2 = nil
			} else {
				v42v2 = make(map[string]bool, len(v))
			} // reset map
			testUnmarshalErr(v42v2, bs42, h, t, "dec-map-v42")
			testDeepEqualErr(v42v1, v42v2, t, "equal-map-v42")
			if v == nil {
				v42v2 = nil
			} else {
				v42v2 = make(map[string]bool, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v42v2), bs42, h, t, "dec-map-v42-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v42v1, v42v2, t, "equal-map-v42-noaddr")
		}
		if v == nil {
			v42v2 = nil
		} else {
			v42v2 = make(map[string]bool, len(v))
		} // reset map
		testUnmarshalErr(&v42v2, bs42, h, t, "dec-map-v42-p-len")
		testDeepEqualErr(v42v1, v42v2, t, "equal-map-v42-p-len")
		bs42 = testMarshalErr(&v42v1, h, t, "enc-map-v42-p")
		v42v2 = nil
		testUnmarshalErr(&v42v2, bs42, h, t, "dec-map-v42-p-nil")
		testDeepEqualErr(v42v1, v42v2, t, "equal-map-v42-p-nil")
		// ...
		if v == nil {
			v42v2 = nil
		} else {
			v42v2 = make(map[string]bool, len(v))
		} // reset map
		var v42v3, v42v4 typMapMapStringBool
		v42v3 = typMapMapStringBool(v42v1)
		v42v4 = typMapMapStringBool(v42v2)
		if v != nil {
			bs42 = testMarshalErr(v42v3, h, t, "enc-map-v42-custom")
			testUnmarshalErr(v42v4, bs42, h, t, "dec-map-v42-p-len")
			testDeepEqualErr(v42v3, v42v4, t, "equal-map-v42-p-len")
		}
	}
	for _, v := range []map[uint]interface{}{nil, {}, {111: nil, 77: "string-is-an-interface-2"}} {
		// fmt.Printf(">>>> running mammoth map v43: %v\n", v)
		var v43v1, v43v2 map[uint]interface{}
		var bs43 []byte
		v43v1 = v
		bs43 = testMarshalErr(v43v1, h, t, "enc-map-v43")
		if v != nil {
			if v == nil {
				v43v2 = nil
			} else {
				v43v2 = make(map[uint]interface{}, len(v))
			} // reset map
			testUnmarshalErr(v43v2, bs43, h, t, "dec-map-v43")
			testDeepEqualErr(v43v1, v43v2, t, "equal-map-v43")
			if v == nil {
				v43v2 = nil
			} else {
				v43v2 = make(map[uint]interface{}, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v43v2), bs43, h, t, "dec-map-v43-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v43v1, v43v2, t, "equal-map-v43-noaddr")
		}
		if v == nil {
			v43v2 = nil
		} else {
			v43v2 = make(map[uint]interface{}, len(v))
		} // reset map
		testUnmarshalErr(&v43v2, bs43, h, t, "dec-map-v43-p-len")
		testDeepEqualErr(v43v1, v43v2, t, "equal-map-v43-p-len")
		bs43 = testMarshalErr(&v43v1, h, t, "enc-map-v43-p")
		v43v2 = nil
		testUnmarshalErr(&v43v2, bs43, h, t, "dec-map-v43-p-nil")
		testDeepEqualErr(v43v1, v43v2, t, "equal-map-v43-p-nil")
		// ...
		if v == nil {
			v43v2 = nil
		} else {
			v43v2 = make(map[uint]interface{}, len(v))
		} // reset map
		var v43v3, v43v4 typMapMapUintIntf
		v43v3 = typMapMapUintIntf(v43v1)
		v43v4 = typMapMapUintIntf(v43v2)
		if v != nil {
			bs43 = testMarshalErr(v43v3, h, t, "enc-map-v43-custom")
			testUnmarshalErr(v43v4, bs43, h, t, "dec-map-v43-p-len")
			testDeepEqualErr(v43v3, v43v4, t, "equal-map-v43-p-len")
		}
	}
	for _, v := range []map[uint]string{nil, {}, {127: "", 111: "some-string-3"}} {
		// fmt.Printf(">>>> running mammoth map v44: %v\n", v)
		var v44v1, v44v2 map[uint]string
		var bs44 []byte
		v44v1 = v
		bs44 = testMarshalErr(v44v1, h, t, "enc-map-v44")
		if v != nil {
			if v == nil {
				v44v2 = nil
			} else {
				v44v2 = make(map[uint]string, len(v))
			} // reset map
			testUnmarshalErr(v44v2, bs44, h, t, "dec-map-v44")
			testDeepEqualErr(v44v1, v44v2, t, "equal-map-v44")
			if v == nil {
				v44v2 = nil
			} else {
				v44v2 = make(map[uint]string, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v44v2), bs44, h, t, "dec-map-v44-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v44v1, v44v2, t, "equal-map-v44-noaddr")
		}
		if v == nil {
			v44v2 = nil
		} else {
			v44v2 = make(map[uint]string, len(v))
		} // reset map
		testUnmarshalErr(&v44v2, bs44, h, t, "dec-map-v44-p-len")
		testDeepEqualErr(v44v1, v44v2, t, "equal-map-v44-p-len")
		bs44 = testMarshalErr(&v44v1, h, t, "enc-map-v44-p")
		v44v2 = nil
		testUnmarshalErr(&v44v2, bs44, h, t, "dec-map-v44-p-nil")
		testDeepEqualErr(v44v1, v44v2, t, "equal-map-v44-p-nil")
		// ...
		if v == nil {
			v44v2 = nil
		} else {
			v44v2 = make(map[uint]string, len(v))
		} // reset map
		var v44v3, v44v4 typMapMapUintString
		v44v3 = typMapMapUintString(v44v1)
		v44v4 = typMapMapUintString(v44v2)
		if v != nil {
			bs44 = testMarshalErr(v44v3, h, t, "enc-map-v44-custom")
			testUnmarshalErr(v44v4, bs44, h, t, "dec-map-v44-p-len")
			testDeepEqualErr(v44v3, v44v4, t, "equal-map-v44-p-len")
		}
	}
	for _, v := range []map[uint][]byte{nil, {}, {77: nil, 127: []byte("some-string-2")}} {
		// fmt.Printf(">>>> running mammoth map v45: %v\n", v)
		var v45v1, v45v2 map[uint][]byte
		var bs45 []byte
		v45v1 = v
		bs45 = testMarshalErr(v45v1, h, t, "enc-map-v45")
		if v != nil {
			if v == nil {
				v45v2 = nil
			} else {
				v45v2 = make(map[uint][]byte, len(v))
			} // reset map
			testUnmarshalErr(v45v2, bs45, h, t, "dec-map-v45")
			testDeepEqualErr(v45v1, v45v2, t, "equal-map-v45")
			if v == nil {
				v45v2 = nil
			} else {
				v45v2 = make(map[uint][]byte, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v45v2), bs45, h, t, "dec-map-v45-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v45v1, v45v2, t, "equal-map-v45-noaddr")
		}
		if v == nil {
			v45v2 = nil
		} else {
			v45v2 = make(map[uint][]byte, len(v))
		} // reset map
		testUnmarshalErr(&v45v2, bs45, h, t, "dec-map-v45-p-len")
		testDeepEqualErr(v45v1, v45v2, t, "equal-map-v45-p-len")
		bs45 = testMarshalErr(&v45v1, h, t, "enc-map-v45-p")
		v45v2 = nil
		testUnmarshalErr(&v45v2, bs45, h, t, "dec-map-v45-p-nil")
		testDeepEqualErr(v45v1, v45v2, t, "equal-map-v45-p-nil")
		// ...
		if v == nil {
			v45v2 = nil
		} else {
			v45v2 = make(map[uint][]byte, len(v))
		} // reset map
		var v45v3, v45v4 typMapMapUintBytes
		v45v3 = typMapMapUintBytes(v45v1)
		v45v4 = typMapMapUintBytes(v45v2)
		if v != nil {
			bs45 = testMarshalErr(v45v3, h, t, "enc-map-v45-custom")
			testUnmarshalErr(v45v4, bs45, h, t, "dec-map-v45-p-len")
			testDeepEqualErr(v45v3, v45v4, t, "equal-map-v45-p-len")
		}
	}
	for _, v := range []map[uint]uint{nil, {}, {111: 0, 77: 127}} {
		// fmt.Printf(">>>> running mammoth map v46: %v\n", v)
		var v46v1, v46v2 map[uint]uint
		var bs46 []byte
		v46v1 = v
		bs46 = testMarshalErr(v46v1, h, t, "enc-map-v46")
		if v != nil {
			if v == nil {
				v46v2 = nil
			} else {
				v46v2 = make(map[uint]uint, len(v))
			} // reset map
			testUnmarshalErr(v46v2, bs46, h, t, "dec-map-v46")
			testDeepEqualErr(v46v1, v46v2, t, "equal-map-v46")
			if v == nil {
				v46v2 = nil
			} else {
				v46v2 = make(map[uint]uint, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v46v2), bs46, h, t, "dec-map-v46-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v46v1, v46v2, t, "equal-map-v46-noaddr")
		}
		if v == nil {
			v46v2 = nil
		} else {
			v46v2 = make(map[uint]uint, len(v))
		} // reset map
		testUnmarshalErr(&v46v2, bs46, h, t, "dec-map-v46-p-len")
		testDeepEqualErr(v46v1, v46v2, t, "equal-map-v46-p-len")
		bs46 = testMarshalErr(&v46v1, h, t, "enc-map-v46-p")
		v46v2 = nil
		testUnmarshalErr(&v46v2, bs46, h, t, "dec-map-v46-p-nil")
		testDeepEqualErr(v46v1, v46v2, t, "equal-map-v46-p-nil")
		// ...
		if v == nil {
			v46v2 = nil
		} else {
			v46v2 = make(map[uint]uint, len(v))
		} // reset map
		var v46v3, v46v4 typMapMapUintUint
		v46v3 = typMapMapUintUint(v46v1)
		v46v4 = typMapMapUintUint(v46v2)
		if v != nil {
			bs46 = testMarshalErr(v46v3, h, t, "enc-map-v46-custom")
			testUnmarshalErr(v46v4, bs46, h, t, "dec-map-v46-p-len")
			testDeepEqualErr(v46v3, v46v4, t, "equal-map-v46-p-len")
		}
	}
	for _, v := range []map[uint]uint8{nil, {}, {111: 0, 77: 127}} {
		// fmt.Printf(">>>> running mammoth map v47: %v\n", v)
		var v47v1, v47v2 map[uint]uint8
		var bs47 []byte
		v47v1 = v
		bs47 = testMarshalErr(v47v1, h, t, "enc-map-v47")
		if v != nil {
			if v == nil {
				v47v2 = nil
			} else {
				v47v2 = make(map[uint]uint8, len(v))
			} // reset map
			testUnmarshalErr(v47v2, bs47, h, t, "dec-map-v47")
			testDeepEqualErr(v47v1, v47v2, t, "equal-map-v47")
			if v == nil {
				v47v2 = nil
			} else {
				v47v2 = make(map[uint]uint8, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v47v2), bs47, h, t, "dec-map-v47-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v47v1, v47v2, t, "equal-map-v47-noaddr")
		}
		if v == nil {
			v47v2 = nil
		} else {
			v47v2 = make(map[uint]uint8, len(v))
		} // reset map
		testUnmarshalErr(&v47v2, bs47, h, t, "dec-map-v47-p-len")
		testDeepEqualErr(v47v1, v47v2, t, "equal-map-v47-p-len")
		bs47 = testMarshalErr(&v47v1, h, t, "enc-map-v47-p")
		v47v2 = nil
		testUnmarshalErr(&v47v2, bs47, h, t, "dec-map-v47-p-nil")
		testDeepEqualErr(v47v1, v47v2, t, "equal-map-v47-p-nil")
		// ...
		if v == nil {
			v47v2 = nil
		} else {
			v47v2 = make(map[uint]uint8, len(v))
		} // reset map
		var v47v3, v47v4 typMapMapUintUint8
		v47v3 = typMapMapUintUint8(v47v1)
		v47v4 = typMapMapUintUint8(v47v2)
		if v != nil {
			bs47 = testMarshalErr(v47v3, h, t, "enc-map-v47-custom")
			testUnmarshalErr(v47v4, bs47, h, t, "dec-map-v47-p-len")
			testDeepEqualErr(v47v3, v47v4, t, "equal-map-v47-p-len")
		}
	}
	for _, v := range []map[uint]uint64{nil, {}, {111: 0, 77: 127}} {
		// fmt.Printf(">>>> running mammoth map v48: %v\n", v)
		var v48v1, v48v2 map[uint]uint64
		var bs48 []byte
		v48v1 = v
		bs48 = testMarshalErr(v48v1, h, t, "enc-map-v48")
		if v != nil {
			if v == nil {
				v48v2 = nil
			} else {
				v48v2 = make(map[uint]uint64, len(v))
			} // reset map
			testUnmarshalErr(v48v2, bs48, h, t, "dec-map-v48")
			testDeepEqualErr(v48v1, v48v2, t, "equal-map-v48")
			if v == nil {
				v48v2 = nil
			} else {
				v48v2 = make(map[uint]uint64, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v48v2), bs48, h, t, "dec-map-v48-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v48v1, v48v2, t, "equal-map-v48-noaddr")
		}
		if v == nil {
			v48v2 = nil
		} else {
			v48v2 = make(map[uint]uint64, len(v))
		} // reset map
		testUnmarshalErr(&v48v2, bs48, h, t, "dec-map-v48-p-len")
		testDeepEqualErr(v48v1, v48v2, t, "equal-map-v48-p-len")
		bs48 = testMarshalErr(&v48v1, h, t, "enc-map-v48-p")
		v48v2 = nil
		testUnmarshalErr(&v48v2, bs48, h, t, "dec-map-v48-p-nil")
		testDeepEqualErr(v48v1, v48v2, t, "equal-map-v48-p-nil")
		// ...
		if v == nil {
			v48v2 = nil
		} else {
			v48v2 = make(map[uint]uint64, len(v))
		} // reset map
		var v48v3, v48v4 typMapMapUintUint64
		v48v3 = typMapMapUintUint64(v48v1)
		v48v4 = typMapMapUintUint64(v48v2)
		if v != nil {
			bs48 = testMarshalErr(v48v3, h, t, "enc-map-v48-custom")
			testUnmarshalErr(v48v4, bs48, h, t, "dec-map-v48-p-len")
			testDeepEqualErr(v48v3, v48v4, t, "equal-map-v48-p-len")
		}
	}
	for _, v := range []map[uint]int{nil, {}, {111: 0, 77: 127}} {
		// fmt.Printf(">>>> running mammoth map v49: %v\n", v)
		var v49v1, v49v2 map[uint]int
		var bs49 []byte
		v49v1 = v
		bs49 = testMarshalErr(v49v1, h, t, "enc-map-v49")
		if v != nil {
			if v == nil {
				v49v2 = nil
			} else {
				v49v2 = make(map[uint]int, len(v))
			} // reset map
			testUnmarshalErr(v49v2, bs49, h, t, "dec-map-v49")
			testDeepEqualErr(v49v1, v49v2, t, "equal-map-v49")
			if v == nil {
				v49v2 = nil
			} else {
				v49v2 = make(map[uint]int, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v49v2), bs49, h, t, "dec-map-v49-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v49v1, v49v2, t, "equal-map-v49-noaddr")
		}
		if v == nil {
			v49v2 = nil
		} else {
			v49v2 = make(map[uint]int, len(v))
		} // reset map
		testUnmarshalErr(&v49v2, bs49, h, t, "dec-map-v49-p-len")
		testDeepEqualErr(v49v1, v49v2, t, "equal-map-v49-p-len")
		bs49 = testMarshalErr(&v49v1, h, t, "enc-map-v49-p")
		v49v2 = nil
		testUnmarshalErr(&v49v2, bs49, h, t, "dec-map-v49-p-nil")
		testDeepEqualErr(v49v1, v49v2, t, "equal-map-v49-p-nil")
		// ...
		if v == nil {
			v49v2 = nil
		} else {
			v49v2 = make(map[uint]int, len(v))
		} // reset map
		var v49v3, v49v4 typMapMapUintInt
		v49v3 = typMapMapUintInt(v49v1)
		v49v4 = typMapMapUintInt(v49v2)
		if v != nil {
			bs49 = testMarshalErr(v49v3, h, t, "enc-map-v49-custom")
			testUnmarshalErr(v49v4, bs49, h, t, "dec-map-v49-p-len")
			testDeepEqualErr(v49v3, v49v4, t, "equal-map-v49-p-len")
		}
	}
	for _, v := range []map[uint]int64{nil, {}, {111: 0, 77: 127}} {
		// fmt.Printf(">>>> running mammoth map v50: %v\n", v)
		var v50v1, v50v2 map[uint]int64
		var bs50 []byte
		v50v1 = v
		bs50 = testMarshalErr(v50v1, h, t, "enc-map-v50")
		if v != nil {
			if v == nil {
				v50v2 = nil
			} else {
				v50v2 = make(map[uint]int64, len(v))
			} // reset map
			testUnmarshalErr(v50v2, bs50, h, t, "dec-map-v50")
			testDeepEqualErr(v50v1, v50v2, t, "equal-map-v50")
			if v == nil {
				v50v2 = nil
			} else {
				v50v2 = make(map[uint]int64, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v50v2), bs50, h, t, "dec-map-v50-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v50v1, v50v2, t, "equal-map-v50-noaddr")
		}
		if v == nil {
			v50v2 = nil
		} else {
			v50v2 = make(map[uint]int64, len(v))
		} // reset map
		testUnmarshalErr(&v50v2, bs50, h, t, "dec-map-v50-p-len")
		testDeepEqualErr(v50v1, v50v2, t, "equal-map-v50-p-len")
		bs50 = testMarshalErr(&v50v1, h, t, "enc-map-v50-p")
		v50v2 = nil
		testUnmarshalErr(&v50v2, bs50, h, t, "dec-map-v50-p-nil")
		testDeepEqualErr(v50v1, v50v2, t, "equal-map-v50-p-nil")
		// ...
		if v == nil {
			v50v2 = nil
		} else {
			v50v2 = make(map[uint]int64, len(v))
		} // reset map
		var v50v3, v50v4 typMapMapUintInt64
		v50v3 = typMapMapUintInt64(v50v1)
		v50v4 = typMapMapUintInt64(v50v2)
		if v != nil {
			bs50 = testMarshalErr(v50v3, h, t, "enc-map-v50-custom")
			testUnmarshalErr(v50v4, bs50, h, t, "dec-map-v50-p-len")
			testDeepEqualErr(v50v3, v50v4, t, "equal-map-v50-p-len")
		}
	}
	for _, v := range []map[uint]float32{nil, {}, {111: 0, 77: 22.2}} {
		// fmt.Printf(">>>> running mammoth map v51: %v\n", v)
		var v51v1, v51v2 map[uint]float32
		var bs51 []byte
		v51v1 = v
		bs51 = testMarshalErr(v51v1, h, t, "enc-map-v51")
		if v != nil {
			if v == nil {
				v51v2 = nil
			} else {
				v51v2 = make(map[uint]float32, len(v))
			} // reset map
			testUnmarshalErr(v51v2, bs51, h, t, "dec-map-v51")
			testDeepEqualErr(v51v1, v51v2, t, "equal-map-v51")
			if v == nil {
				v51v2 = nil
			} else {
				v51v2 = make(map[uint]float32, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v51v2), bs51, h, t, "dec-map-v51-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v51v1, v51v2, t, "equal-map-v51-noaddr")
		}
		if v == nil {
			v51v2 = nil
		} else {
			v51v2 = make(map[uint]float32, len(v))
		} // reset map
		testUnmarshalErr(&v51v2, bs51, h, t, "dec-map-v51-p-len")
		testDeepEqualErr(v51v1, v51v2, t, "equal-map-v51-p-len")
		bs51 = testMarshalErr(&v51v1, h, t, "enc-map-v51-p")
		v51v2 = nil
		testUnmarshalErr(&v51v2, bs51, h, t, "dec-map-v51-p-nil")
		testDeepEqualErr(v51v1, v51v2, t, "equal-map-v51-p-nil")
		// ...
		if v == nil {
			v51v2 = nil
		} else {
			v51v2 = make(map[uint]float32, len(v))
		} // reset map
		var v51v3, v51v4 typMapMapUintFloat32
		v51v3 = typMapMapUintFloat32(v51v1)
		v51v4 = typMapMapUintFloat32(v51v2)
		if v != nil {
			bs51 = testMarshalErr(v51v3, h, t, "enc-map-v51-custom")
			testUnmarshalErr(v51v4, bs51, h, t, "dec-map-v51-p-len")
			testDeepEqualErr(v51v3, v51v4, t, "equal-map-v51-p-len")
		}
	}
	for _, v := range []map[uint]float64{nil, {}, {127: 0, 111: 33.3e3}} {
		// fmt.Printf(">>>> running mammoth map v52: %v\n", v)
		var v52v1, v52v2 map[uint]float64
		var bs52 []byte
		v52v1 = v
		bs52 = testMarshalErr(v52v1, h, t, "enc-map-v52")
		if v != nil {
			if v == nil {
				v52v2 = nil
			} else {
				v52v2 = make(map[uint]float64, len(v))
			} // reset map
			testUnmarshalErr(v52v2, bs52, h, t, "dec-map-v52")
			testDeepEqualErr(v52v1, v52v2, t, "equal-map-v52")
			if v == nil {
				v52v2 = nil
			} else {
				v52v2 = make(map[uint]float64, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v52v2), bs52, h, t, "dec-map-v52-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v52v1, v52v2, t, "equal-map-v52-noaddr")
		}
		if v == nil {
			v52v2 = nil
		} else {
			v52v2 = make(map[uint]float64, len(v))
		} // reset map
		testUnmarshalErr(&v52v2, bs52, h, t, "dec-map-v52-p-len")
		testDeepEqualErr(v52v1, v52v2, t, "equal-map-v52-p-len")
		bs52 = testMarshalErr(&v52v1, h, t, "enc-map-v52-p")
		v52v2 = nil
		testUnmarshalErr(&v52v2, bs52, h, t, "dec-map-v52-p-nil")
		testDeepEqualErr(v52v1, v52v2, t, "equal-map-v52-p-nil")
		// ...
		if v == nil {
			v52v2 = nil
		} else {
			v52v2 = make(map[uint]float64, len(v))
		} // reset map
		var v52v3, v52v4 typMapMapUintFloat64
		v52v3 = typMapMapUintFloat64(v52v1)
		v52v4 = typMapMapUintFloat64(v52v2)
		if v != nil {
			bs52 = testMarshalErr(v52v3, h, t, "enc-map-v52-custom")
			testUnmarshalErr(v52v4, bs52, h, t, "dec-map-v52-p-len")
			testDeepEqualErr(v52v3, v52v4, t, "equal-map-v52-p-len")
		}
	}
	for _, v := range []map[uint]bool{nil, {}, {77: false, 127: false}} {
		// fmt.Printf(">>>> running mammoth map v53: %v\n", v)
		var v53v1, v53v2 map[uint]bool
		var bs53 []byte
		v53v1 = v
		bs53 = testMarshalErr(v53v1, h, t, "enc-map-v53")
		if v != nil {
			if v == nil {
				v53v2 = nil
			} else {
				v53v2 = make(map[uint]bool, len(v))
			} // reset map
			testUnmarshalErr(v53v2, bs53, h, t, "dec-map-v53")
			testDeepEqualErr(v53v1, v53v2, t, "equal-map-v53")
			if v == nil {
				v53v2 = nil
			} else {
				v53v2 = make(map[uint]bool, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v53v2), bs53, h, t, "dec-map-v53-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v53v1, v53v2, t, "equal-map-v53-noaddr")
		}
		if v == nil {
			v53v2 = nil
		} else {
			v53v2 = make(map[uint]bool, len(v))
		} // reset map
		testUnmarshalErr(&v53v2, bs53, h, t, "dec-map-v53-p-len")
		testDeepEqualErr(v53v1, v53v2, t, "equal-map-v53-p-len")
		bs53 = testMarshalErr(&v53v1, h, t, "enc-map-v53-p")
		v53v2 = nil
		testUnmarshalErr(&v53v2, bs53, h, t, "dec-map-v53-p-nil")
		testDeepEqualErr(v53v1, v53v2, t, "equal-map-v53-p-nil")
		// ...
		if v == nil {
			v53v2 = nil
		} else {
			v53v2 = make(map[uint]bool, len(v))
		} // reset map
		var v53v3, v53v4 typMapMapUintBool
		v53v3 = typMapMapUintBool(v53v1)
		v53v4 = typMapMapUintBool(v53v2)
		if v != nil {
			bs53 = testMarshalErr(v53v3, h, t, "enc-map-v53-custom")
			testUnmarshalErr(v53v4, bs53, h, t, "dec-map-v53-p-len")
			testDeepEqualErr(v53v3, v53v4, t, "equal-map-v53-p-len")
		}
	}
	for _, v := range []map[uint8]interface{}{nil, {}, {111: nil, 77: "string-is-an-interface-3"}} {
		// fmt.Printf(">>>> running mammoth map v54: %v\n", v)
		var v54v1, v54v2 map[uint8]interface{}
		var bs54 []byte
		v54v1 = v
		bs54 = testMarshalErr(v54v1, h, t, "enc-map-v54")
		if v != nil {
			if v == nil {
				v54v2 = nil
			} else {
				v54v2 = make(map[uint8]interface{}, len(v))
			} // reset map
			testUnmarshalErr(v54v2, bs54, h, t, "dec-map-v54")
			testDeepEqualErr(v54v1, v54v2, t, "equal-map-v54")
			if v == nil {
				v54v2 = nil
			} else {
				v54v2 = make(map[uint8]interface{}, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v54v2), bs54, h, t, "dec-map-v54-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v54v1, v54v2, t, "equal-map-v54-noaddr")
		}
		if v == nil {
			v54v2 = nil
		} else {
			v54v2 = make(map[uint8]interface{}, len(v))
		} // reset map
		testUnmarshalErr(&v54v2, bs54, h, t, "dec-map-v54-p-len")
		testDeepEqualErr(v54v1, v54v2, t, "equal-map-v54-p-len")
		bs54 = testMarshalErr(&v54v1, h, t, "enc-map-v54-p")
		v54v2 = nil
		testUnmarshalErr(&v54v2, bs54, h, t, "dec-map-v54-p-nil")
		testDeepEqualErr(v54v1, v54v2, t, "equal-map-v54-p-nil")
		// ...
		if v == nil {
			v54v2 = nil
		} else {
			v54v2 = make(map[uint8]interface{}, len(v))
		} // reset map
		var v54v3, v54v4 typMapMapUint8Intf
		v54v3 = typMapMapUint8Intf(v54v1)
		v54v4 = typMapMapUint8Intf(v54v2)
		if v != nil {
			bs54 = testMarshalErr(v54v3, h, t, "enc-map-v54-custom")
			testUnmarshalErr(v54v4, bs54, h, t, "dec-map-v54-p-len")
			testDeepEqualErr(v54v3, v54v4, t, "equal-map-v54-p-len")
		}
	}
	for _, v := range []map[uint8]string{nil, {}, {127: "", 111: "some-string-1"}} {
		// fmt.Printf(">>>> running mammoth map v55: %v\n", v)
		var v55v1, v55v2 map[uint8]string
		var bs55 []byte
		v55v1 = v
		bs55 = testMarshalErr(v55v1, h, t, "enc-map-v55")
		if v != nil {
			if v == nil {
				v55v2 = nil
			} else {
				v55v2 = make(map[uint8]string, len(v))
			} // reset map
			testUnmarshalErr(v55v2, bs55, h, t, "dec-map-v55")
			testDeepEqualErr(v55v1, v55v2, t, "equal-map-v55")
			if v == nil {
				v55v2 = nil
			} else {
				v55v2 = make(map[uint8]string, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v55v2), bs55, h, t, "dec-map-v55-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v55v1, v55v2, t, "equal-map-v55-noaddr")
		}
		if v == nil {
			v55v2 = nil
		} else {
			v55v2 = make(map[uint8]string, len(v))
		} // reset map
		testUnmarshalErr(&v55v2, bs55, h, t, "dec-map-v55-p-len")
		testDeepEqualErr(v55v1, v55v2, t, "equal-map-v55-p-len")
		bs55 = testMarshalErr(&v55v1, h, t, "enc-map-v55-p")
		v55v2 = nil
		testUnmarshalErr(&v55v2, bs55, h, t, "dec-map-v55-p-nil")
		testDeepEqualErr(v55v1, v55v2, t, "equal-map-v55-p-nil")
		// ...
		if v == nil {
			v55v2 = nil
		} else {
			v55v2 = make(map[uint8]string, len(v))
		} // reset map
		var v55v3, v55v4 typMapMapUint8String
		v55v3 = typMapMapUint8String(v55v1)
		v55v4 = typMapMapUint8String(v55v2)
		if v != nil {
			bs55 = testMarshalErr(v55v3, h, t, "enc-map-v55-custom")
			testUnmarshalErr(v55v4, bs55, h, t, "dec-map-v55-p-len")
			testDeepEqualErr(v55v3, v55v4, t, "equal-map-v55-p-len")
		}
	}
	for _, v := range []map[uint8][]byte{nil, {}, {77: nil, 127: []byte("some-string-3")}} {
		// fmt.Printf(">>>> running mammoth map v56: %v\n", v)
		var v56v1, v56v2 map[uint8][]byte
		var bs56 []byte
		v56v1 = v
		bs56 = testMarshalErr(v56v1, h, t, "enc-map-v56")
		if v != nil {
			if v == nil {
				v56v2 = nil
			} else {
				v56v2 = make(map[uint8][]byte, len(v))
			} // reset map
			testUnmarshalErr(v56v2, bs56, h, t, "dec-map-v56")
			testDeepEqualErr(v56v1, v56v2, t, "equal-map-v56")
			if v == nil {
				v56v2 = nil
			} else {
				v56v2 = make(map[uint8][]byte, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v56v2), bs56, h, t, "dec-map-v56-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v56v1, v56v2, t, "equal-map-v56-noaddr")
		}
		if v == nil {
			v56v2 = nil
		} else {
			v56v2 = make(map[uint8][]byte, len(v))
		} // reset map
		testUnmarshalErr(&v56v2, bs56, h, t, "dec-map-v56-p-len")
		testDeepEqualErr(v56v1, v56v2, t, "equal-map-v56-p-len")
		bs56 = testMarshalErr(&v56v1, h, t, "enc-map-v56-p")
		v56v2 = nil
		testUnmarshalErr(&v56v2, bs56, h, t, "dec-map-v56-p-nil")
		testDeepEqualErr(v56v1, v56v2, t, "equal-map-v56-p-nil")
		// ...
		if v == nil {
			v56v2 = nil
		} else {
			v56v2 = make(map[uint8][]byte, len(v))
		} // reset map
		var v56v3, v56v4 typMapMapUint8Bytes
		v56v3 = typMapMapUint8Bytes(v56v1)
		v56v4 = typMapMapUint8Bytes(v56v2)
		if v != nil {
			bs56 = testMarshalErr(v56v3, h, t, "enc-map-v56-custom")
			testUnmarshalErr(v56v4, bs56, h, t, "dec-map-v56-p-len")
			testDeepEqualErr(v56v3, v56v4, t, "equal-map-v56-p-len")
		}
	}
	for _, v := range []map[uint8]uint{nil, {}, {111: 0, 77: 127}} {
		// fmt.Printf(">>>> running mammoth map v57: %v\n", v)
		var v57v1, v57v2 map[uint8]uint
		var bs57 []byte
		v57v1 = v
		bs57 = testMarshalErr(v57v1, h, t, "enc-map-v57")
		if v != nil {
			if v == nil {
				v57v2 = nil
			} else {
				v57v2 = make(map[uint8]uint, len(v))
			} // reset map
			testUnmarshalErr(v57v2, bs57, h, t, "dec-map-v57")
			testDeepEqualErr(v57v1, v57v2, t, "equal-map-v57")
			if v == nil {
				v57v2 = nil
			} else {
				v57v2 = make(map[uint8]uint, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v57v2), bs57, h, t, "dec-map-v57-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v57v1, v57v2, t, "equal-map-v57-noaddr")
		}
		if v == nil {
			v57v2 = nil
		} else {
			v57v2 = make(map[uint8]uint, len(v))
		} // reset map
		testUnmarshalErr(&v57v2, bs57, h, t, "dec-map-v57-p-len")
		testDeepEqualErr(v57v1, v57v2, t, "equal-map-v57-p-len")
		bs57 = testMarshalErr(&v57v1, h, t, "enc-map-v57-p")
		v57v2 = nil
		testUnmarshalErr(&v57v2, bs57, h, t, "dec-map-v57-p-nil")
		testDeepEqualErr(v57v1, v57v2, t, "equal-map-v57-p-nil")
		// ...
		if v == nil {
			v57v2 = nil
		} else {
			v57v2 = make(map[uint8]uint, len(v))
		} // reset map
		var v57v3, v57v4 typMapMapUint8Uint
		v57v3 = typMapMapUint8Uint(v57v1)
		v57v4 = typMapMapUint8Uint(v57v2)
		if v != nil {
			bs57 = testMarshalErr(v57v3, h, t, "enc-map-v57-custom")
			testUnmarshalErr(v57v4, bs57, h, t, "dec-map-v57-p-len")
			testDeepEqualErr(v57v3, v57v4, t, "equal-map-v57-p-len")
		}
	}
	for _, v := range []map[uint8]uint8{nil, {}, {111: 0, 77: 127}} {
		// fmt.Printf(">>>> running mammoth map v58: %v\n", v)
		var v58v1, v58v2 map[uint8]uint8
		var bs58 []byte
		v58v1 = v
		bs58 = testMarshalErr(v58v1, h, t, "enc-map-v58")
		if v != nil {
			if v == nil {
				v58v2 = nil
			} else {
				v58v2 = make(map[uint8]uint8, len(v))
			} // reset map
			testUnmarshalErr(v58v2, bs58, h, t, "dec-map-v58")
			testDeepEqualErr(v58v1, v58v2, t, "equal-map-v58")
			if v == nil {
				v58v2 = nil
			} else {
				v58v2 = make(map[uint8]uint8, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v58v2), bs58, h, t, "dec-map-v58-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v58v1, v58v2, t, "equal-map-v58-noaddr")
		}
		if v == nil {
			v58v2 = nil
		} else {
			v58v2 = make(map[uint8]uint8, len(v))
		} // reset map
		testUnmarshalErr(&v58v2, bs58, h, t, "dec-map-v58-p-len")
		testDeepEqualErr(v58v1, v58v2, t, "equal-map-v58-p-len")
		bs58 = testMarshalErr(&v58v1, h, t, "enc-map-v58-p")
		v58v2 = nil
		testUnmarshalErr(&v58v2, bs58, h, t, "dec-map-v58-p-nil")
		testDeepEqualErr(v58v1, v58v2, t, "equal-map-v58-p-nil")
		// ...
		if v == nil {
			v58v2 = nil
		} else {
			v58v2 = make(map[uint8]uint8, len(v))
		} // reset map
		var v58v3, v58v4 typMapMapUint8Uint8
		v58v3 = typMapMapUint8Uint8(v58v1)
		v58v4 = typMapMapUint8Uint8(v58v2)
		if v != nil {
			bs58 = testMarshalErr(v58v3, h, t, "enc-map-v58-custom")
			testUnmarshalErr(v58v4, bs58, h, t, "dec-map-v58-p-len")
			testDeepEqualErr(v58v3, v58v4, t, "equal-map-v58-p-len")
		}
	}
	for _, v := range []map[uint8]uint64{nil, {}, {111: 0, 77: 127}} {
		// fmt.Printf(">>>> running mammoth map v59: %v\n", v)
		var v59v1, v59v2 map[uint8]uint64
		var bs59 []byte
		v59v1 = v
		bs59 = testMarshalErr(v59v1, h, t, "enc-map-v59")
		if v != nil {
			if v == nil {
				v59v2 = nil
			} else {
				v59v2 = make(map[uint8]uint64, len(v))
			} // reset map
			testUnmarshalErr(v59v2, bs59, h, t, "dec-map-v59")
			testDeepEqualErr(v59v1, v59v2, t, "equal-map-v59")
			if v == nil {
				v59v2 = nil
			} else {
				v59v2 = make(map[uint8]uint64, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v59v2), bs59, h, t, "dec-map-v59-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v59v1, v59v2, t, "equal-map-v59-noaddr")
		}
		if v == nil {
			v59v2 = nil
		} else {
			v59v2 = make(map[uint8]uint64, len(v))
		} // reset map
		testUnmarshalErr(&v59v2, bs59, h, t, "dec-map-v59-p-len")
		testDeepEqualErr(v59v1, v59v2, t, "equal-map-v59-p-len")
		bs59 = testMarshalErr(&v59v1, h, t, "enc-map-v59-p")
		v59v2 = nil
		testUnmarshalErr(&v59v2, bs59, h, t, "dec-map-v59-p-nil")
		testDeepEqualErr(v59v1, v59v2, t, "equal-map-v59-p-nil")
		// ...
		if v == nil {
			v59v2 = nil
		} else {
			v59v2 = make(map[uint8]uint64, len(v))
		} // reset map
		var v59v3, v59v4 typMapMapUint8Uint64
		v59v3 = typMapMapUint8Uint64(v59v1)
		v59v4 = typMapMapUint8Uint64(v59v2)
		if v != nil {
			bs59 = testMarshalErr(v59v3, h, t, "enc-map-v59-custom")
			testUnmarshalErr(v59v4, bs59, h, t, "dec-map-v59-p-len")
			testDeepEqualErr(v59v3, v59v4, t, "equal-map-v59-p-len")
		}
	}
	for _, v := range []map[uint8]int{nil, {}, {111: 0, 77: 127}} {
		// fmt.Printf(">>>> running mammoth map v60: %v\n", v)
		var v60v1, v60v2 map[uint8]int
		var bs60 []byte
		v60v1 = v
		bs60 = testMarshalErr(v60v1, h, t, "enc-map-v60")
		if v != nil {
			if v == nil {
				v60v2 = nil
			} else {
				v60v2 = make(map[uint8]int, len(v))
			} // reset map
			testUnmarshalErr(v60v2, bs60, h, t, "dec-map-v60")
			testDeepEqualErr(v60v1, v60v2, t, "equal-map-v60")
			if v == nil {
				v60v2 = nil
			} else {
				v60v2 = make(map[uint8]int, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v60v2), bs60, h, t, "dec-map-v60-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v60v1, v60v2, t, "equal-map-v60-noaddr")
		}
		if v == nil {
			v60v2 = nil
		} else {
			v60v2 = make(map[uint8]int, len(v))
		} // reset map
		testUnmarshalErr(&v60v2, bs60, h, t, "dec-map-v60-p-len")
		testDeepEqualErr(v60v1, v60v2, t, "equal-map-v60-p-len")
		bs60 = testMarshalErr(&v60v1, h, t, "enc-map-v60-p")
		v60v2 = nil
		testUnmarshalErr(&v60v2, bs60, h, t, "dec-map-v60-p-nil")
		testDeepEqualErr(v60v1, v60v2, t, "equal-map-v60-p-nil")
		// ...
		if v == nil {
			v60v2 = nil
		} else {
			v60v2 = make(map[uint8]int, len(v))
		} // reset map
		var v60v3, v60v4 typMapMapUint8Int
		v60v3 = typMapMapUint8Int(v60v1)
		v60v4 = typMapMapUint8Int(v60v2)
		if v != nil {
			bs60 = testMarshalErr(v60v3, h, t, "enc-map-v60-custom")
			testUnmarshalErr(v60v4, bs60, h, t, "dec-map-v60-p-len")
			testDeepEqualErr(v60v3, v60v4, t, "equal-map-v60-p-len")
		}
	}
	for _, v := range []map[uint8]int64{nil, {}, {111: 0, 77: 127}} {
		// fmt.Printf(">>>> running mammoth map v61: %v\n", v)
		var v61v1, v61v2 map[uint8]int64
		var bs61 []byte
		v61v1 = v
		bs61 = testMarshalErr(v61v1, h, t, "enc-map-v61")
		if v != nil {
			if v == nil {
				v61v2 = nil
			} else {
				v61v2 = make(map[uint8]int64, len(v))
			} // reset map
			testUnmarshalErr(v61v2, bs61, h, t, "dec-map-v61")
			testDeepEqualErr(v61v1, v61v2, t, "equal-map-v61")
			if v == nil {
				v61v2 = nil
			} else {
				v61v2 = make(map[uint8]int64, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v61v2), bs61, h, t, "dec-map-v61-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v61v1, v61v2, t, "equal-map-v61-noaddr")
		}
		if v == nil {
			v61v2 = nil
		} else {
			v61v2 = make(map[uint8]int64, len(v))
		} // reset map
		testUnmarshalErr(&v61v2, bs61, h, t, "dec-map-v61-p-len")
		testDeepEqualErr(v61v1, v61v2, t, "equal-map-v61-p-len")
		bs61 = testMarshalErr(&v61v1, h, t, "enc-map-v61-p")
		v61v2 = nil
		testUnmarshalErr(&v61v2, bs61, h, t, "dec-map-v61-p-nil")
		testDeepEqualErr(v61v1, v61v2, t, "equal-map-v61-p-nil")
		// ...
		if v == nil {
			v61v2 = nil
		} else {
			v61v2 = make(map[uint8]int64, len(v))
		} // reset map
		var v61v3, v61v4 typMapMapUint8Int64
		v61v3 = typMapMapUint8Int64(v61v1)
		v61v4 = typMapMapUint8Int64(v61v2)
		if v != nil {
			bs61 = testMarshalErr(v61v3, h, t, "enc-map-v61-custom")
			testUnmarshalErr(v61v4, bs61, h, t, "dec-map-v61-p-len")
			testDeepEqualErr(v61v3, v61v4, t, "equal-map-v61-p-len")
		}
	}
	for _, v := range []map[uint8]float32{nil, {}, {111: 0, 77: 11.1}} {
		// fmt.Printf(">>>> running mammoth map v62: %v\n", v)
		var v62v1, v62v2 map[uint8]float32
		var bs62 []byte
		v62v1 = v
		bs62 = testMarshalErr(v62v1, h, t, "enc-map-v62")
		if v != nil {
			if v == nil {
				v62v2 = nil
			} else {
				v62v2 = make(map[uint8]float32, len(v))
			} // reset map
			testUnmarshalErr(v62v2, bs62, h, t, "dec-map-v62")
			testDeepEqualErr(v62v1, v62v2, t, "equal-map-v62")
			if v == nil {
				v62v2 = nil
			} else {
				v62v2 = make(map[uint8]float32, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v62v2), bs62, h, t, "dec-map-v62-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v62v1, v62v2, t, "equal-map-v62-noaddr")
		}
		if v == nil {
			v62v2 = nil
		} else {
			v62v2 = make(map[uint8]float32, len(v))
		} // reset map
		testUnmarshalErr(&v62v2, bs62, h, t, "dec-map-v62-p-len")
		testDeepEqualErr(v62v1, v62v2, t, "equal-map-v62-p-len")
		bs62 = testMarshalErr(&v62v1, h, t, "enc-map-v62-p")
		v62v2 = nil
		testUnmarshalErr(&v62v2, bs62, h, t, "dec-map-v62-p-nil")
		testDeepEqualErr(v62v1, v62v2, t, "equal-map-v62-p-nil")
		// ...
		if v == nil {
			v62v2 = nil
		} else {
			v62v2 = make(map[uint8]float32, len(v))
		} // reset map
		var v62v3, v62v4 typMapMapUint8Float32
		v62v3 = typMapMapUint8Float32(v62v1)
		v62v4 = typMapMapUint8Float32(v62v2)
		if v != nil {
			bs62 = testMarshalErr(v62v3, h, t, "enc-map-v62-custom")
			testUnmarshalErr(v62v4, bs62, h, t, "dec-map-v62-p-len")
			testDeepEqualErr(v62v3, v62v4, t, "equal-map-v62-p-len")
		}
	}
	for _, v := range []map[uint8]float64{nil, {}, {127: 0, 111: 22.2}} {
		// fmt.Printf(">>>> running mammoth map v63: %v\n", v)
		var v63v1, v63v2 map[uint8]float64
		var bs63 []byte
		v63v1 = v
		bs63 = testMarshalErr(v63v1, h, t, "enc-map-v63")
		if v != nil {
			if v == nil {
				v63v2 = nil
			} else {
				v63v2 = make(map[uint8]float64, len(v))
			} // reset map
			testUnmarshalErr(v63v2, bs63, h, t, "dec-map-v63")
			testDeepEqualErr(v63v1, v63v2, t, "equal-map-v63")
			if v == nil {
				v63v2 = nil
			} else {
				v63v2 = make(map[uint8]float64, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v63v2), bs63, h, t, "dec-map-v63-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v63v1, v63v2, t, "equal-map-v63-noaddr")
		}
		if v == nil {
			v63v2 = nil
		} else {
			v63v2 = make(map[uint8]float64, len(v))
		} // reset map
		testUnmarshalErr(&v63v2, bs63, h, t, "dec-map-v63-p-len")
		testDeepEqualErr(v63v1, v63v2, t, "equal-map-v63-p-len")
		bs63 = testMarshalErr(&v63v1, h, t, "enc-map-v63-p")
		v63v2 = nil
		testUnmarshalErr(&v63v2, bs63, h, t, "dec-map-v63-p-nil")
		testDeepEqualErr(v63v1, v63v2, t, "equal-map-v63-p-nil")
		// ...
		if v == nil {
			v63v2 = nil
		} else {
			v63v2 = make(map[uint8]float64, len(v))
		} // reset map
		var v63v3, v63v4 typMapMapUint8Float64
		v63v3 = typMapMapUint8Float64(v63v1)
		v63v4 = typMapMapUint8Float64(v63v2)
		if v != nil {
			bs63 = testMarshalErr(v63v3, h, t, "enc-map-v63-custom")
			testUnmarshalErr(v63v4, bs63, h, t, "dec-map-v63-p-len")
			testDeepEqualErr(v63v3, v63v4, t, "equal-map-v63-p-len")
		}
	}
	for _, v := range []map[uint8]bool{nil, {}, {77: false, 127: true}} {
		// fmt.Printf(">>>> running mammoth map v64: %v\n", v)
		var v64v1, v64v2 map[uint8]bool
		var bs64 []byte
		v64v1 = v
		bs64 = testMarshalErr(v64v1, h, t, "enc-map-v64")
		if v != nil {
			if v == nil {
				v64v2 = nil
			} else {
				v64v2 = make(map[uint8]bool, len(v))
			} // reset map
			testUnmarshalErr(v64v2, bs64, h, t, "dec-map-v64")
			testDeepEqualErr(v64v1, v64v2, t, "equal-map-v64")
			if v == nil {
				v64v2 = nil
			} else {
				v64v2 = make(map[uint8]bool, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v64v2), bs64, h, t, "dec-map-v64-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v64v1, v64v2, t, "equal-map-v64-noaddr")
		}
		if v == nil {
			v64v2 = nil
		} else {
			v64v2 = make(map[uint8]bool, len(v))
		} // reset map
		testUnmarshalErr(&v64v2, bs64, h, t, "dec-map-v64-p-len")
		testDeepEqualErr(v64v1, v64v2, t, "equal-map-v64-p-len")
		bs64 = testMarshalErr(&v64v1, h, t, "enc-map-v64-p")
		v64v2 = nil
		testUnmarshalErr(&v64v2, bs64, h, t, "dec-map-v64-p-nil")
		testDeepEqualErr(v64v1, v64v2, t, "equal-map-v64-p-nil")
		// ...
		if v == nil {
			v64v2 = nil
		} else {
			v64v2 = make(map[uint8]bool, len(v))
		} // reset map
		var v64v3, v64v4 typMapMapUint8Bool
		v64v3 = typMapMapUint8Bool(v64v1)
		v64v4 = typMapMapUint8Bool(v64v2)
		if v != nil {
			bs64 = testMarshalErr(v64v3, h, t, "enc-map-v64-custom")
			testUnmarshalErr(v64v4, bs64, h, t, "dec-map-v64-p-len")
			testDeepEqualErr(v64v3, v64v4, t, "equal-map-v64-p-len")
		}
	}
	for _, v := range []map[uint64]interface{}{nil, {}, {111: nil, 77: "string-is-an-interface-1"}} {
		// fmt.Printf(">>>> running mammoth map v65: %v\n", v)
		var v65v1, v65v2 map[uint64]interface{}
		var bs65 []byte
		v65v1 = v
		bs65 = testMarshalErr(v65v1, h, t, "enc-map-v65")
		if v != nil {
			if v == nil {
				v65v2 = nil
			} else {
				v65v2 = make(map[uint64]interface{}, len(v))
			} // reset map
			testUnmarshalErr(v65v2, bs65, h, t, "dec-map-v65")
			testDeepEqualErr(v65v1, v65v2, t, "equal-map-v65")
			if v == nil {
				v65v2 = nil
			} else {
				v65v2 = make(map[uint64]interface{}, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v65v2), bs65, h, t, "dec-map-v65-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v65v1, v65v2, t, "equal-map-v65-noaddr")
		}
		if v == nil {
			v65v2 = nil
		} else {
			v65v2 = make(map[uint64]interface{}, len(v))
		} // reset map
		testUnmarshalErr(&v65v2, bs65, h, t, "dec-map-v65-p-len")
		testDeepEqualErr(v65v1, v65v2, t, "equal-map-v65-p-len")
		bs65 = testMarshalErr(&v65v1, h, t, "enc-map-v65-p")
		v65v2 = nil
		testUnmarshalErr(&v65v2, bs65, h, t, "dec-map-v65-p-nil")
		testDeepEqualErr(v65v1, v65v2, t, "equal-map-v65-p-nil")
		// ...
		if v == nil {
			v65v2 = nil
		} else {
			v65v2 = make(map[uint64]interface{}, len(v))
		} // reset map
		var v65v3, v65v4 typMapMapUint64Intf
		v65v3 = typMapMapUint64Intf(v65v1)
		v65v4 = typMapMapUint64Intf(v65v2)
		if v != nil {
			bs65 = testMarshalErr(v65v3, h, t, "enc-map-v65-custom")
			testUnmarshalErr(v65v4, bs65, h, t, "dec-map-v65-p-len")
			testDeepEqualErr(v65v3, v65v4, t, "equal-map-v65-p-len")
		}
	}
	for _, v := range []map[uint64]string{nil, {}, {127: "", 111: "some-string-2"}} {
		// fmt.Printf(">>>> running mammoth map v66: %v\n", v)
		var v66v1, v66v2 map[uint64]string
		var bs66 []byte
		v66v1 = v
		bs66 = testMarshalErr(v66v1, h, t, "enc-map-v66")
		if v != nil {
			if v == nil {
				v66v2 = nil
			} else {
				v66v2 = make(map[uint64]string, len(v))
			} // reset map
			testUnmarshalErr(v66v2, bs66, h, t, "dec-map-v66")
			testDeepEqualErr(v66v1, v66v2, t, "equal-map-v66")
			if v == nil {
				v66v2 = nil
			} else {
				v66v2 = make(map[uint64]string, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v66v2), bs66, h, t, "dec-map-v66-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v66v1, v66v2, t, "equal-map-v66-noaddr")
		}
		if v == nil {
			v66v2 = nil
		} else {
			v66v2 = make(map[uint64]string, len(v))
		} // reset map
		testUnmarshalErr(&v66v2, bs66, h, t, "dec-map-v66-p-len")
		testDeepEqualErr(v66v1, v66v2, t, "equal-map-v66-p-len")
		bs66 = testMarshalErr(&v66v1, h, t, "enc-map-v66-p")
		v66v2 = nil
		testUnmarshalErr(&v66v2, bs66, h, t, "dec-map-v66-p-nil")
		testDeepEqualErr(v66v1, v66v2, t, "equal-map-v66-p-nil")
		// ...
		if v == nil {
			v66v2 = nil
		} else {
			v66v2 = make(map[uint64]string, len(v))
		} // reset map
		var v66v3, v66v4 typMapMapUint64String
		v66v3 = typMapMapUint64String(v66v1)
		v66v4 = typMapMapUint64String(v66v2)
		if v != nil {
			bs66 = testMarshalErr(v66v3, h, t, "enc-map-v66-custom")
			testUnmarshalErr(v66v4, bs66, h, t, "dec-map-v66-p-len")
			testDeepEqualErr(v66v3, v66v4, t, "equal-map-v66-p-len")
		}
	}
	for _, v := range []map[uint64][]byte{nil, {}, {77: nil, 127: []byte("some-string-1")}} {
		// fmt.Printf(">>>> running mammoth map v67: %v\n", v)
		var v67v1, v67v2 map[uint64][]byte
		var bs67 []byte
		v67v1 = v
		bs67 = testMarshalErr(v67v1, h, t, "enc-map-v67")
		if v != nil {
			if v == nil {
				v67v2 = nil
			} else {
				v67v2 = make(map[uint64][]byte, len(v))
			} // reset map
			testUnmarshalErr(v67v2, bs67, h, t, "dec-map-v67")
			testDeepEqualErr(v67v1, v67v2, t, "equal-map-v67")
			if v == nil {
				v67v2 = nil
			} else {
				v67v2 = make(map[uint64][]byte, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v67v2), bs67, h, t, "dec-map-v67-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v67v1, v67v2, t, "equal-map-v67-noaddr")
		}
		if v == nil {
			v67v2 = nil
		} else {
			v67v2 = make(map[uint64][]byte, len(v))
		} // reset map
		testUnmarshalErr(&v67v2, bs67, h, t, "dec-map-v67-p-len")
		testDeepEqualErr(v67v1, v67v2, t, "equal-map-v67-p-len")
		bs67 = testMarshalErr(&v67v1, h, t, "enc-map-v67-p")
		v67v2 = nil
		testUnmarshalErr(&v67v2, bs67, h, t, "dec-map-v67-p-nil")
		testDeepEqualErr(v67v1, v67v2, t, "equal-map-v67-p-nil")
		// ...
		if v == nil {
			v67v2 = nil
		} else {
			v67v2 = make(map[uint64][]byte, len(v))
		} // reset map
		var v67v3, v67v4 typMapMapUint64Bytes
		v67v3 = typMapMapUint64Bytes(v67v1)
		v67v4 = typMapMapUint64Bytes(v67v2)
		if v != nil {
			bs67 = testMarshalErr(v67v3, h, t, "enc-map-v67-custom")
			testUnmarshalErr(v67v4, bs67, h, t, "dec-map-v67-p-len")
			testDeepEqualErr(v67v3, v67v4, t, "equal-map-v67-p-len")
		}
	}
	for _, v := range []map[uint64]uint{nil, {}, {111: 0, 77: 127}} {
		// fmt.Printf(">>>> running mammoth map v68: %v\n", v)
		var v68v1, v68v2 map[uint64]uint
		var bs68 []byte
		v68v1 = v
		bs68 = testMarshalErr(v68v1, h, t, "enc-map-v68")
		if v != nil {
			if v == nil {
				v68v2 = nil
			} else {
				v68v2 = make(map[uint64]uint, len(v))
			} // reset map
			testUnmarshalErr(v68v2, bs68, h, t, "dec-map-v68")
			testDeepEqualErr(v68v1, v68v2, t, "equal-map-v68")
			if v == nil {
				v68v2 = nil
			} else {
				v68v2 = make(map[uint64]uint, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v68v2), bs68, h, t, "dec-map-v68-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v68v1, v68v2, t, "equal-map-v68-noaddr")
		}
		if v == nil {
			v68v2 = nil
		} else {
			v68v2 = make(map[uint64]uint, len(v))
		} // reset map
		testUnmarshalErr(&v68v2, bs68, h, t, "dec-map-v68-p-len")
		testDeepEqualErr(v68v1, v68v2, t, "equal-map-v68-p-len")
		bs68 = testMarshalErr(&v68v1, h, t, "enc-map-v68-p")
		v68v2 = nil
		testUnmarshalErr(&v68v2, bs68, h, t, "dec-map-v68-p-nil")
		testDeepEqualErr(v68v1, v68v2, t, "equal-map-v68-p-nil")
		// ...
		if v == nil {
			v68v2 = nil
		} else {
			v68v2 = make(map[uint64]uint, len(v))
		} // reset map
		var v68v3, v68v4 typMapMapUint64Uint
		v68v3 = typMapMapUint64Uint(v68v1)
		v68v4 = typMapMapUint64Uint(v68v2)
		if v != nil {
			bs68 = testMarshalErr(v68v3, h, t, "enc-map-v68-custom")
			testUnmarshalErr(v68v4, bs68, h, t, "dec-map-v68-p-len")
			testDeepEqualErr(v68v3, v68v4, t, "equal-map-v68-p-len")
		}
	}
	for _, v := range []map[uint64]uint8{nil, {}, {111: 0, 77: 127}} {
		// fmt.Printf(">>>> running mammoth map v69: %v\n", v)
		var v69v1, v69v2 map[uint64]uint8
		var bs69 []byte
		v69v1 = v
		bs69 = testMarshalErr(v69v1, h, t, "enc-map-v69")
		if v != nil {
			if v == nil {
				v69v2 = nil
			} else {
				v69v2 = make(map[uint64]uint8, len(v))
			} // reset map
			testUnmarshalErr(v69v2, bs69, h, t, "dec-map-v69")
			testDeepEqualErr(v69v1, v69v2, t, "equal-map-v69")
			if v == nil {
				v69v2 = nil
			} else {
				v69v2 = make(map[uint64]uint8, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v69v2), bs69, h, t, "dec-map-v69-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v69v1, v69v2, t, "equal-map-v69-noaddr")
		}
		if v == nil {
			v69v2 = nil
		} else {
			v69v2 = make(map[uint64]uint8, len(v))
		} // reset map
		testUnmarshalErr(&v69v2, bs69, h, t, "dec-map-v69-p-len")
		testDeepEqualErr(v69v1, v69v2, t, "equal-map-v69-p-len")
		bs69 = testMarshalErr(&v69v1, h, t, "enc-map-v69-p")
		v69v2 = nil
		testUnmarshalErr(&v69v2, bs69, h, t, "dec-map-v69-p-nil")
		testDeepEqualErr(v69v1, v69v2, t, "equal-map-v69-p-nil")
		// ...
		if v == nil {
			v69v2 = nil
		} else {
			v69v2 = make(map[uint64]uint8, len(v))
		} // reset map
		var v69v3, v69v4 typMapMapUint64Uint8
		v69v3 = typMapMapUint64Uint8(v69v1)
		v69v4 = typMapMapUint64Uint8(v69v2)
		if v != nil {
			bs69 = testMarshalErr(v69v3, h, t, "enc-map-v69-custom")
			testUnmarshalErr(v69v4, bs69, h, t, "dec-map-v69-p-len")
			testDeepEqualErr(v69v3, v69v4, t, "equal-map-v69-p-len")
		}
	}
	for _, v := range []map[uint64]uint64{nil, {}, {111: 0, 77: 127}} {
		// fmt.Printf(">>>> running mammoth map v70: %v\n", v)
		var v70v1, v70v2 map[uint64]uint64
		var bs70 []byte
		v70v1 = v
		bs70 = testMarshalErr(v70v1, h, t, "enc-map-v70")
		if v != nil {
			if v == nil {
				v70v2 = nil
			} else {
				v70v2 = make(map[uint64]uint64, len(v))
			} // reset map
			testUnmarshalErr(v70v2, bs70, h, t, "dec-map-v70")
			testDeepEqualErr(v70v1, v70v2, t, "equal-map-v70")
			if v == nil {
				v70v2 = nil
			} else {
				v70v2 = make(map[uint64]uint64, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v70v2), bs70, h, t, "dec-map-v70-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v70v1, v70v2, t, "equal-map-v70-noaddr")
		}
		if v == nil {
			v70v2 = nil
		} else {
			v70v2 = make(map[uint64]uint64, len(v))
		} // reset map
		testUnmarshalErr(&v70v2, bs70, h, t, "dec-map-v70-p-len")
		testDeepEqualErr(v70v1, v70v2, t, "equal-map-v70-p-len")
		bs70 = testMarshalErr(&v70v1, h, t, "enc-map-v70-p")
		v70v2 = nil
		testUnmarshalErr(&v70v2, bs70, h, t, "dec-map-v70-p-nil")
		testDeepEqualErr(v70v1, v70v2, t, "equal-map-v70-p-nil")
		// ...
		if v == nil {
			v70v2 = nil
		} else {
			v70v2 = make(map[uint64]uint64, len(v))
		} // reset map
		var v70v3, v70v4 typMapMapUint64Uint64
		v70v3 = typMapMapUint64Uint64(v70v1)
		v70v4 = typMapMapUint64Uint64(v70v2)
		if v != nil {
			bs70 = testMarshalErr(v70v3, h, t, "enc-map-v70-custom")
			testUnmarshalErr(v70v4, bs70, h, t, "dec-map-v70-p-len")
			testDeepEqualErr(v70v3, v70v4, t, "equal-map-v70-p-len")
		}
	}
	for _, v := range []map[uint64]int{nil, {}, {111: 0, 77: 127}} {
		// fmt.Printf(">>>> running mammoth map v71: %v\n", v)
		var v71v1, v71v2 map[uint64]int
		var bs71 []byte
		v71v1 = v
		bs71 = testMarshalErr(v71v1, h, t, "enc-map-v71")
		if v != nil {
			if v == nil {
				v71v2 = nil
			} else {
				v71v2 = make(map[uint64]int, len(v))
			} // reset map
			testUnmarshalErr(v71v2, bs71, h, t, "dec-map-v71")
			testDeepEqualErr(v71v1, v71v2, t, "equal-map-v71")
			if v == nil {
				v71v2 = nil
			} else {
				v71v2 = make(map[uint64]int, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v71v2), bs71, h, t, "dec-map-v71-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v71v1, v71v2, t, "equal-map-v71-noaddr")
		}
		if v == nil {
			v71v2 = nil
		} else {
			v71v2 = make(map[uint64]int, len(v))
		} // reset map
		testUnmarshalErr(&v71v2, bs71, h, t, "dec-map-v71-p-len")
		testDeepEqualErr(v71v1, v71v2, t, "equal-map-v71-p-len")
		bs71 = testMarshalErr(&v71v1, h, t, "enc-map-v71-p")
		v71v2 = nil
		testUnmarshalErr(&v71v2, bs71, h, t, "dec-map-v71-p-nil")
		testDeepEqualErr(v71v1, v71v2, t, "equal-map-v71-p-nil")
		// ...
		if v == nil {
			v71v2 = nil
		} else {
			v71v2 = make(map[uint64]int, len(v))
		} // reset map
		var v71v3, v71v4 typMapMapUint64Int
		v71v3 = typMapMapUint64Int(v71v1)
		v71v4 = typMapMapUint64Int(v71v2)
		if v != nil {
			bs71 = testMarshalErr(v71v3, h, t, "enc-map-v71-custom")
			testUnmarshalErr(v71v4, bs71, h, t, "dec-map-v71-p-len")
			testDeepEqualErr(v71v3, v71v4, t, "equal-map-v71-p-len")
		}
	}
	for _, v := range []map[uint64]int64{nil, {}, {111: 0, 77: 127}} {
		// fmt.Printf(">>>> running mammoth map v72: %v\n", v)
		var v72v1, v72v2 map[uint64]int64
		var bs72 []byte
		v72v1 = v
		bs72 = testMarshalErr(v72v1, h, t, "enc-map-v72")
		if v != nil {
			if v == nil {
				v72v2 = nil
			} else {
				v72v2 = make(map[uint64]int64, len(v))
			} // reset map
			testUnmarshalErr(v72v2, bs72, h, t, "dec-map-v72")
			testDeepEqualErr(v72v1, v72v2, t, "equal-map-v72")
			if v == nil {
				v72v2 = nil
			} else {
				v72v2 = make(map[uint64]int64, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v72v2), bs72, h, t, "dec-map-v72-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v72v1, v72v2, t, "equal-map-v72-noaddr")
		}
		if v == nil {
			v72v2 = nil
		} else {
			v72v2 = make(map[uint64]int64, len(v))
		} // reset map
		testUnmarshalErr(&v72v2, bs72, h, t, "dec-map-v72-p-len")
		testDeepEqualErr(v72v1, v72v2, t, "equal-map-v72-p-len")
		bs72 = testMarshalErr(&v72v1, h, t, "enc-map-v72-p")
		v72v2 = nil
		testUnmarshalErr(&v72v2, bs72, h, t, "dec-map-v72-p-nil")
		testDeepEqualErr(v72v1, v72v2, t, "equal-map-v72-p-nil")
		// ...
		if v == nil {
			v72v2 = nil
		} else {
			v72v2 = make(map[uint64]int64, len(v))
		} // reset map
		var v72v3, v72v4 typMapMapUint64Int64
		v72v3 = typMapMapUint64Int64(v72v1)
		v72v4 = typMapMapUint64Int64(v72v2)
		if v != nil {
			bs72 = testMarshalErr(v72v3, h, t, "enc-map-v72-custom")
			testUnmarshalErr(v72v4, bs72, h, t, "dec-map-v72-p-len")
			testDeepEqualErr(v72v3, v72v4, t, "equal-map-v72-p-len")
		}
	}
	for _, v := range []map[uint64]float32{nil, {}, {111: 0, 77: 33.3e3}} {
		// fmt.Printf(">>>> running mammoth map v73: %v\n", v)
		var v73v1, v73v2 map[uint64]float32
		var bs73 []byte
		v73v1 = v
		bs73 = testMarshalErr(v73v1, h, t, "enc-map-v73")
		if v != nil {
			if v == nil {
				v73v2 = nil
			} else {
				v73v2 = make(map[uint64]float32, len(v))
			} // reset map
			testUnmarshalErr(v73v2, bs73, h, t, "dec-map-v73")
			testDeepEqualErr(v73v1, v73v2, t, "equal-map-v73")
			if v == nil {
				v73v2 = nil
			} else {
				v73v2 = make(map[uint64]float32, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v73v2), bs73, h, t, "dec-map-v73-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v73v1, v73v2, t, "equal-map-v73-noaddr")
		}
		if v == nil {
			v73v2 = nil
		} else {
			v73v2 = make(map[uint64]float32, len(v))
		} // reset map
		testUnmarshalErr(&v73v2, bs73, h, t, "dec-map-v73-p-len")
		testDeepEqualErr(v73v1, v73v2, t, "equal-map-v73-p-len")
		bs73 = testMarshalErr(&v73v1, h, t, "enc-map-v73-p")
		v73v2 = nil
		testUnmarshalErr(&v73v2, bs73, h, t, "dec-map-v73-p-nil")
		testDeepEqualErr(v73v1, v73v2, t, "equal-map-v73-p-nil")
		// ...
		if v == nil {
			v73v2 = nil
		} else {
			v73v2 = make(map[uint64]float32, len(v))
		} // reset map
		var v73v3, v73v4 typMapMapUint64Float32
		v73v3 = typMapMapUint64Float32(v73v1)
		v73v4 = typMapMapUint64Float32(v73v2)
		if v != nil {
			bs73 = testMarshalErr(v73v3, h, t, "enc-map-v73-custom")
			testUnmarshalErr(v73v4, bs73, h, t, "dec-map-v73-p-len")
			testDeepEqualErr(v73v3, v73v4, t, "equal-map-v73-p-len")
		}
	}
	for _, v := range []map[uint64]float64{nil, {}, {127: 0, 111: 11.1}} {
		// fmt.Printf(">>>> running mammoth map v74: %v\n", v)
		var v74v1, v74v2 map[uint64]float64
		var bs74 []byte
		v74v1 = v
		bs74 = testMarshalErr(v74v1, h, t, "enc-map-v74")
		if v != nil {
			if v == nil {
				v74v2 = nil
			} else {
				v74v2 = make(map[uint64]float64, len(v))
			} // reset map
			testUnmarshalErr(v74v2, bs74, h, t, "dec-map-v74")
			testDeepEqualErr(v74v1, v74v2, t, "equal-map-v74")
			if v == nil {
				v74v2 = nil
			} else {
				v74v2 = make(map[uint64]float64, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v74v2), bs74, h, t, "dec-map-v74-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v74v1, v74v2, t, "equal-map-v74-noaddr")
		}
		if v == nil {
			v74v2 = nil
		} else {
			v74v2 = make(map[uint64]float64, len(v))
		} // reset map
		testUnmarshalErr(&v74v2, bs74, h, t, "dec-map-v74-p-len")
		testDeepEqualErr(v74v1, v74v2, t, "equal-map-v74-p-len")
		bs74 = testMarshalErr(&v74v1, h, t, "enc-map-v74-p")
		v74v2 = nil
		testUnmarshalErr(&v74v2, bs74, h, t, "dec-map-v74-p-nil")
		testDeepEqualErr(v74v1, v74v2, t, "equal-map-v74-p-nil")
		// ...
		if v == nil {
			v74v2 = nil
		} else {
			v74v2 = make(map[uint64]float64, len(v))
		} // reset map
		var v74v3, v74v4 typMapMapUint64Float64
		v74v3 = typMapMapUint64Float64(v74v1)
		v74v4 = typMapMapUint64Float64(v74v2)
		if v != nil {
			bs74 = testMarshalErr(v74v3, h, t, "enc-map-v74-custom")
			testUnmarshalErr(v74v4, bs74, h, t, "dec-map-v74-p-len")
			testDeepEqualErr(v74v3, v74v4, t, "equal-map-v74-p-len")
		}
	}
	for _, v := range []map[uint64]bool{nil, {}, {77: false, 127: true}} {
		// fmt.Printf(">>>> running mammoth map v75: %v\n", v)
		var v75v1, v75v2 map[uint64]bool
		var bs75 []byte
		v75v1 = v
		bs75 = testMarshalErr(v75v1, h, t, "enc-map-v75")
		if v != nil {
			if v == nil {
				v75v2 = nil
			} else {
				v75v2 = make(map[uint64]bool, len(v))
			} // reset map
			testUnmarshalErr(v75v2, bs75, h, t, "dec-map-v75")
			testDeepEqualErr(v75v1, v75v2, t, "equal-map-v75")
			if v == nil {
				v75v2 = nil
			} else {
				v75v2 = make(map[uint64]bool, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v75v2), bs75, h, t, "dec-map-v75-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v75v1, v75v2, t, "equal-map-v75-noaddr")
		}
		if v == nil {
			v75v2 = nil
		} else {
			v75v2 = make(map[uint64]bool, len(v))
		} // reset map
		testUnmarshalErr(&v75v2, bs75, h, t, "dec-map-v75-p-len")
		testDeepEqualErr(v75v1, v75v2, t, "equal-map-v75-p-len")
		bs75 = testMarshalErr(&v75v1, h, t, "enc-map-v75-p")
		v75v2 = nil
		testUnmarshalErr(&v75v2, bs75, h, t, "dec-map-v75-p-nil")
		testDeepEqualErr(v75v1, v75v2, t, "equal-map-v75-p-nil")
		// ...
		if v == nil {
			v75v2 = nil
		} else {
			v75v2 = make(map[uint64]bool, len(v))
		} // reset map
		var v75v3, v75v4 typMapMapUint64Bool
		v75v3 = typMapMapUint64Bool(v75v1)
		v75v4 = typMapMapUint64Bool(v75v2)
		if v != nil {
			bs75 = testMarshalErr(v75v3, h, t, "enc-map-v75-custom")
			testUnmarshalErr(v75v4, bs75, h, t, "dec-map-v75-p-len")
			testDeepEqualErr(v75v3, v75v4, t, "equal-map-v75-p-len")
		}
	}
	for _, v := range []map[int]interface{}{nil, {}, {111: nil, 77: "string-is-an-interface-2"}} {
		// fmt.Printf(">>>> running mammoth map v76: %v\n", v)
		var v76v1, v76v2 map[int]interface{}
		var bs76 []byte
		v76v1 = v
		bs76 = testMarshalErr(v76v1, h, t, "enc-map-v76")
		if v != nil {
			if v == nil {
				v76v2 = nil
			} else {
				v76v2 = make(map[int]interface{}, len(v))
			} // reset map
			testUnmarshalErr(v76v2, bs76, h, t, "dec-map-v76")
			testDeepEqualErr(v76v1, v76v2, t, "equal-map-v76")
			if v == nil {
				v76v2 = nil
			} else {
				v76v2 = make(map[int]interface{}, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v76v2), bs76, h, t, "dec-map-v76-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v76v1, v76v2, t, "equal-map-v76-noaddr")
		}
		if v == nil {
			v76v2 = nil
		} else {
			v76v2 = make(map[int]interface{}, len(v))
		} // reset map
		testUnmarshalErr(&v76v2, bs76, h, t, "dec-map-v76-p-len")
		testDeepEqualErr(v76v1, v76v2, t, "equal-map-v76-p-len")
		bs76 = testMarshalErr(&v76v1, h, t, "enc-map-v76-p")
		v76v2 = nil
		testUnmarshalErr(&v76v2, bs76, h, t, "dec-map-v76-p-nil")
		testDeepEqualErr(v76v1, v76v2, t, "equal-map-v76-p-nil")
		// ...
		if v == nil {
			v76v2 = nil
		} else {
			v76v2 = make(map[int]interface{}, len(v))
		} // reset map
		var v76v3, v76v4 typMapMapIntIntf
		v76v3 = typMapMapIntIntf(v76v1)
		v76v4 = typMapMapIntIntf(v76v2)
		if v != nil {
			bs76 = testMarshalErr(v76v3, h, t, "enc-map-v76-custom")
			testUnmarshalErr(v76v4, bs76, h, t, "dec-map-v76-p-len")
			testDeepEqualErr(v76v3, v76v4, t, "equal-map-v76-p-len")
		}
	}
	for _, v := range []map[int]string{nil, {}, {127: "", 111: "some-string-3"}} {
		// fmt.Printf(">>>> running mammoth map v77: %v\n", v)
		var v77v1, v77v2 map[int]string
		var bs77 []byte
		v77v1 = v
		bs77 = testMarshalErr(v77v1, h, t, "enc-map-v77")
		if v != nil {
			if v == nil {
				v77v2 = nil
			} else {
				v77v2 = make(map[int]string, len(v))
			} // reset map
			testUnmarshalErr(v77v2, bs77, h, t, "dec-map-v77")
			testDeepEqualErr(v77v1, v77v2, t, "equal-map-v77")
			if v == nil {
				v77v2 = nil
			} else {
				v77v2 = make(map[int]string, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v77v2), bs77, h, t, "dec-map-v77-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v77v1, v77v2, t, "equal-map-v77-noaddr")
		}
		if v == nil {
			v77v2 = nil
		} else {
			v77v2 = make(map[int]string, len(v))
		} // reset map
		testUnmarshalErr(&v77v2, bs77, h, t, "dec-map-v77-p-len")
		testDeepEqualErr(v77v1, v77v2, t, "equal-map-v77-p-len")
		bs77 = testMarshalErr(&v77v1, h, t, "enc-map-v77-p")
		v77v2 = nil
		testUnmarshalErr(&v77v2, bs77, h, t, "dec-map-v77-p-nil")
		testDeepEqualErr(v77v1, v77v2, t, "equal-map-v77-p-nil")
		// ...
		if v == nil {
			v77v2 = nil
		} else {
			v77v2 = make(map[int]string, len(v))
		} // reset map
		var v77v3, v77v4 typMapMapIntString
		v77v3 = typMapMapIntString(v77v1)
		v77v4 = typMapMapIntString(v77v2)
		if v != nil {
			bs77 = testMarshalErr(v77v3, h, t, "enc-map-v77-custom")
			testUnmarshalErr(v77v4, bs77, h, t, "dec-map-v77-p-len")
			testDeepEqualErr(v77v3, v77v4, t, "equal-map-v77-p-len")
		}
	}
	for _, v := range []map[int][]byte{nil, {}, {77: nil, 127: []byte("some-string-2")}} {
		// fmt.Printf(">>>> running mammoth map v78: %v\n", v)
		var v78v1, v78v2 map[int][]byte
		var bs78 []byte
		v78v1 = v
		bs78 = testMarshalErr(v78v1, h, t, "enc-map-v78")
		if v != nil {
			if v == nil {
				v78v2 = nil
			} else {
				v78v2 = make(map[int][]byte, len(v))
			} // reset map
			testUnmarshalErr(v78v2, bs78, h, t, "dec-map-v78")
			testDeepEqualErr(v78v1, v78v2, t, "equal-map-v78")
			if v == nil {
				v78v2 = nil
			} else {
				v78v2 = make(map[int][]byte, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v78v2), bs78, h, t, "dec-map-v78-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v78v1, v78v2, t, "equal-map-v78-noaddr")
		}
		if v == nil {
			v78v2 = nil
		} else {
			v78v2 = make(map[int][]byte, len(v))
		} // reset map
		testUnmarshalErr(&v78v2, bs78, h, t, "dec-map-v78-p-len")
		testDeepEqualErr(v78v1, v78v2, t, "equal-map-v78-p-len")
		bs78 = testMarshalErr(&v78v1, h, t, "enc-map-v78-p")
		v78v2 = nil
		testUnmarshalErr(&v78v2, bs78, h, t, "dec-map-v78-p-nil")
		testDeepEqualErr(v78v1, v78v2, t, "equal-map-v78-p-nil")
		// ...
		if v == nil {
			v78v2 = nil
		} else {
			v78v2 = make(map[int][]byte, len(v))
		} // reset map
		var v78v3, v78v4 typMapMapIntBytes
		v78v3 = typMapMapIntBytes(v78v1)
		v78v4 = typMapMapIntBytes(v78v2)
		if v != nil {
			bs78 = testMarshalErr(v78v3, h, t, "enc-map-v78-custom")
			testUnmarshalErr(v78v4, bs78, h, t, "dec-map-v78-p-len")
			testDeepEqualErr(v78v3, v78v4, t, "equal-map-v78-p-len")
		}
	}
	for _, v := range []map[int]uint{nil, {}, {111: 0, 77: 127}} {
		// fmt.Printf(">>>> running mammoth map v79: %v\n", v)
		var v79v1, v79v2 map[int]uint
		var bs79 []byte
		v79v1 = v
		bs79 = testMarshalErr(v79v1, h, t, "enc-map-v79")
		if v != nil {
			if v == nil {
				v79v2 = nil
			} else {
				v79v2 = make(map[int]uint, len(v))
			} // reset map
			testUnmarshalErr(v79v2, bs79, h, t, "dec-map-v79")
			testDeepEqualErr(v79v1, v79v2, t, "equal-map-v79")
			if v == nil {
				v79v2 = nil
			} else {
				v79v2 = make(map[int]uint, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v79v2), bs79, h, t, "dec-map-v79-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v79v1, v79v2, t, "equal-map-v79-noaddr")
		}
		if v == nil {
			v79v2 = nil
		} else {
			v79v2 = make(map[int]uint, len(v))
		} // reset map
		testUnmarshalErr(&v79v2, bs79, h, t, "dec-map-v79-p-len")
		testDeepEqualErr(v79v1, v79v2, t, "equal-map-v79-p-len")
		bs79 = testMarshalErr(&v79v1, h, t, "enc-map-v79-p")
		v79v2 = nil
		testUnmarshalErr(&v79v2, bs79, h, t, "dec-map-v79-p-nil")
		testDeepEqualErr(v79v1, v79v2, t, "equal-map-v79-p-nil")
		// ...
		if v == nil {
			v79v2 = nil
		} else {
			v79v2 = make(map[int]uint, len(v))
		} // reset map
		var v79v3, v79v4 typMapMapIntUint
		v79v3 = typMapMapIntUint(v79v1)
		v79v4 = typMapMapIntUint(v79v2)
		if v != nil {
			bs79 = testMarshalErr(v79v3, h, t, "enc-map-v79-custom")
			testUnmarshalErr(v79v4, bs79, h, t, "dec-map-v79-p-len")
			testDeepEqualErr(v79v3, v79v4, t, "equal-map-v79-p-len")
		}
	}
	for _, v := range []map[int]uint8{nil, {}, {111: 0, 77: 127}} {
		// fmt.Printf(">>>> running mammoth map v80: %v\n", v)
		var v80v1, v80v2 map[int]uint8
		var bs80 []byte
		v80v1 = v
		bs80 = testMarshalErr(v80v1, h, t, "enc-map-v80")
		if v != nil {
			if v == nil {
				v80v2 = nil
			} else {
				v80v2 = make(map[int]uint8, len(v))
			} // reset map
			testUnmarshalErr(v80v2, bs80, h, t, "dec-map-v80")
			testDeepEqualErr(v80v1, v80v2, t, "equal-map-v80")
			if v == nil {
				v80v2 = nil
			} else {
				v80v2 = make(map[int]uint8, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v80v2), bs80, h, t, "dec-map-v80-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v80v1, v80v2, t, "equal-map-v80-noaddr")
		}
		if v == nil {
			v80v2 = nil
		} else {
			v80v2 = make(map[int]uint8, len(v))
		} // reset map
		testUnmarshalErr(&v80v2, bs80, h, t, "dec-map-v80-p-len")
		testDeepEqualErr(v80v1, v80v2, t, "equal-map-v80-p-len")
		bs80 = testMarshalErr(&v80v1, h, t, "enc-map-v80-p")
		v80v2 = nil
		testUnmarshalErr(&v80v2, bs80, h, t, "dec-map-v80-p-nil")
		testDeepEqualErr(v80v1, v80v2, t, "equal-map-v80-p-nil")
		// ...
		if v == nil {
			v80v2 = nil
		} else {
			v80v2 = make(map[int]uint8, len(v))
		} // reset map
		var v80v3, v80v4 typMapMapIntUint8
		v80v3 = typMapMapIntUint8(v80v1)
		v80v4 = typMapMapIntUint8(v80v2)
		if v != nil {
			bs80 = testMarshalErr(v80v3, h, t, "enc-map-v80-custom")
			testUnmarshalErr(v80v4, bs80, h, t, "dec-map-v80-p-len")
			testDeepEqualErr(v80v3, v80v4, t, "equal-map-v80-p-len")
		}
	}
	for _, v := range []map[int]uint64{nil, {}, {111: 0, 77: 127}} {
		// fmt.Printf(">>>> running mammoth map v81: %v\n", v)
		var v81v1, v81v2 map[int]uint64
		var bs81 []byte
		v81v1 = v
		bs81 = testMarshalErr(v81v1, h, t, "enc-map-v81")
		if v != nil {
			if v == nil {
				v81v2 = nil
			} else {
				v81v2 = make(map[int]uint64, len(v))
			} // reset map
			testUnmarshalErr(v81v2, bs81, h, t, "dec-map-v81")
			testDeepEqualErr(v81v1, v81v2, t, "equal-map-v81")
			if v == nil {
				v81v2 = nil
			} else {
				v81v2 = make(map[int]uint64, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v81v2), bs81, h, t, "dec-map-v81-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v81v1, v81v2, t, "equal-map-v81-noaddr")
		}
		if v == nil {
			v81v2 = nil
		} else {
			v81v2 = make(map[int]uint64, len(v))
		} // reset map
		testUnmarshalErr(&v81v2, bs81, h, t, "dec-map-v81-p-len")
		testDeepEqualErr(v81v1, v81v2, t, "equal-map-v81-p-len")
		bs81 = testMarshalErr(&v81v1, h, t, "enc-map-v81-p")
		v81v2 = nil
		testUnmarshalErr(&v81v2, bs81, h, t, "dec-map-v81-p-nil")
		testDeepEqualErr(v81v1, v81v2, t, "equal-map-v81-p-nil")
		// ...
		if v == nil {
			v81v2 = nil
		} else {
			v81v2 = make(map[int]uint64, len(v))
		} // reset map
		var v81v3, v81v4 typMapMapIntUint64
		v81v3 = typMapMapIntUint64(v81v1)
		v81v4 = typMapMapIntUint64(v81v2)
		if v != nil {
			bs81 = testMarshalErr(v81v3, h, t, "enc-map-v81-custom")
			testUnmarshalErr(v81v4, bs81, h, t, "dec-map-v81-p-len")
			testDeepEqualErr(v81v3, v81v4, t, "equal-map-v81-p-len")
		}
	}
	for _, v := range []map[int]int{nil, {}, {111: 0, 77: 127}} {
		// fmt.Printf(">>>> running mammoth map v82: %v\n", v)
		var v82v1, v82v2 map[int]int
		var bs82 []byte
		v82v1 = v
		bs82 = testMarshalErr(v82v1, h, t, "enc-map-v82")
		if v != nil {
			if v == nil {
				v82v2 = nil
			} else {
				v82v2 = make(map[int]int, len(v))
			} // reset map
			testUnmarshalErr(v82v2, bs82, h, t, "dec-map-v82")
			testDeepEqualErr(v82v1, v82v2, t, "equal-map-v82")
			if v == nil {
				v82v2 = nil
			} else {
				v82v2 = make(map[int]int, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v82v2), bs82, h, t, "dec-map-v82-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v82v1, v82v2, t, "equal-map-v82-noaddr")
		}
		if v == nil {
			v82v2 = nil
		} else {
			v82v2 = make(map[int]int, len(v))
		} // reset map
		testUnmarshalErr(&v82v2, bs82, h, t, "dec-map-v82-p-len")
		testDeepEqualErr(v82v1, v82v2, t, "equal-map-v82-p-len")
		bs82 = testMarshalErr(&v82v1, h, t, "enc-map-v82-p")
		v82v2 = nil
		testUnmarshalErr(&v82v2, bs82, h, t, "dec-map-v82-p-nil")
		testDeepEqualErr(v82v1, v82v2, t, "equal-map-v82-p-nil")
		// ...
		if v == nil {
			v82v2 = nil
		} else {
			v82v2 = make(map[int]int, len(v))
		} // reset map
		var v82v3, v82v4 typMapMapIntInt
		v82v3 = typMapMapIntInt(v82v1)
		v82v4 = typMapMapIntInt(v82v2)
		if v != nil {
			bs82 = testMarshalErr(v82v3, h, t, "enc-map-v82-custom")
			testUnmarshalErr(v82v4, bs82, h, t, "dec-map-v82-p-len")
			testDeepEqualErr(v82v3, v82v4, t, "equal-map-v82-p-len")
		}
	}
	for _, v := range []map[int]int64{nil, {}, {111: 0, 77: 127}} {
		// fmt.Printf(">>>> running mammoth map v83: %v\n", v)
		var v83v1, v83v2 map[int]int64
		var bs83 []byte
		v83v1 = v
		bs83 = testMarshalErr(v83v1, h, t, "enc-map-v83")
		if v != nil {
			if v == nil {
				v83v2 = nil
			} else {
				v83v2 = make(map[int]int64, len(v))
			} // reset map
			testUnmarshalErr(v83v2, bs83, h, t, "dec-map-v83")
			testDeepEqualErr(v83v1, v83v2, t, "equal-map-v83")
			if v == nil {
				v83v2 = nil
			} else {
				v83v2 = make(map[int]int64, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v83v2), bs83, h, t, "dec-map-v83-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v83v1, v83v2, t, "equal-map-v83-noaddr")
		}
		if v == nil {
			v83v2 = nil
		} else {
			v83v2 = make(map[int]int64, len(v))
		} // reset map
		testUnmarshalErr(&v83v2, bs83, h, t, "dec-map-v83-p-len")
		testDeepEqualErr(v83v1, v83v2, t, "equal-map-v83-p-len")
		bs83 = testMarshalErr(&v83v1, h, t, "enc-map-v83-p")
		v83v2 = nil
		testUnmarshalErr(&v83v2, bs83, h, t, "dec-map-v83-p-nil")
		testDeepEqualErr(v83v1, v83v2, t, "equal-map-v83-p-nil")
		// ...
		if v == nil {
			v83v2 = nil
		} else {
			v83v2 = make(map[int]int64, len(v))
		} // reset map
		var v83v3, v83v4 typMapMapIntInt64
		v83v3 = typMapMapIntInt64(v83v1)
		v83v4 = typMapMapIntInt64(v83v2)
		if v != nil {
			bs83 = testMarshalErr(v83v3, h, t, "enc-map-v83-custom")
			testUnmarshalErr(v83v4, bs83, h, t, "dec-map-v83-p-len")
			testDeepEqualErr(v83v3, v83v4, t, "equal-map-v83-p-len")
		}
	}
	for _, v := range []map[int]float32{nil, {}, {111: 0, 77: 22.2}} {
		// fmt.Printf(">>>> running mammoth map v84: %v\n", v)
		var v84v1, v84v2 map[int]float32
		var bs84 []byte
		v84v1 = v
		bs84 = testMarshalErr(v84v1, h, t, "enc-map-v84")
		if v != nil {
			if v == nil {
				v84v2 = nil
			} else {
				v84v2 = make(map[int]float32, len(v))
			} // reset map
			testUnmarshalErr(v84v2, bs84, h, t, "dec-map-v84")
			testDeepEqualErr(v84v1, v84v2, t, "equal-map-v84")
			if v == nil {
				v84v2 = nil
			} else {
				v84v2 = make(map[int]float32, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v84v2), bs84, h, t, "dec-map-v84-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v84v1, v84v2, t, "equal-map-v84-noaddr")
		}
		if v == nil {
			v84v2 = nil
		} else {
			v84v2 = make(map[int]float32, len(v))
		} // reset map
		testUnmarshalErr(&v84v2, bs84, h, t, "dec-map-v84-p-len")
		testDeepEqualErr(v84v1, v84v2, t, "equal-map-v84-p-len")
		bs84 = testMarshalErr(&v84v1, h, t, "enc-map-v84-p")
		v84v2 = nil
		testUnmarshalErr(&v84v2, bs84, h, t, "dec-map-v84-p-nil")
		testDeepEqualErr(v84v1, v84v2, t, "equal-map-v84-p-nil")
		// ...
		if v == nil {
			v84v2 = nil
		} else {
			v84v2 = make(map[int]float32, len(v))
		} // reset map
		var v84v3, v84v4 typMapMapIntFloat32
		v84v3 = typMapMapIntFloat32(v84v1)
		v84v4 = typMapMapIntFloat32(v84v2)
		if v != nil {
			bs84 = testMarshalErr(v84v3, h, t, "enc-map-v84-custom")
			testUnmarshalErr(v84v4, bs84, h, t, "dec-map-v84-p-len")
			testDeepEqualErr(v84v3, v84v4, t, "equal-map-v84-p-len")
		}
	}
	for _, v := range []map[int]float64{nil, {}, {127: 0, 111: 33.3e3}} {
		// fmt.Printf(">>>> running mammoth map v85: %v\n", v)
		var v85v1, v85v2 map[int]float64
		var bs85 []byte
		v85v1 = v
		bs85 = testMarshalErr(v85v1, h, t, "enc-map-v85")
		if v != nil {
			if v == nil {
				v85v2 = nil
			} else {
				v85v2 = make(map[int]float64, len(v))
			} // reset map
			testUnmarshalErr(v85v2, bs85, h, t, "dec-map-v85")
			testDeepEqualErr(v85v1, v85v2, t, "equal-map-v85")
			if v == nil {
				v85v2 = nil
			} else {
				v85v2 = make(map[int]float64, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v85v2), bs85, h, t, "dec-map-v85-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v85v1, v85v2, t, "equal-map-v85-noaddr")
		}
		if v == nil {
			v85v2 = nil
		} else {
			v85v2 = make(map[int]float64, len(v))
		} // reset map
		testUnmarshalErr(&v85v2, bs85, h, t, "dec-map-v85-p-len")
		testDeepEqualErr(v85v1, v85v2, t, "equal-map-v85-p-len")
		bs85 = testMarshalErr(&v85v1, h, t, "enc-map-v85-p")
		v85v2 = nil
		testUnmarshalErr(&v85v2, bs85, h, t, "dec-map-v85-p-nil")
		testDeepEqualErr(v85v1, v85v2, t, "equal-map-v85-p-nil")
		// ...
		if v == nil {
			v85v2 = nil
		} else {
			v85v2 = make(map[int]float64, len(v))
		} // reset map
		var v85v3, v85v4 typMapMapIntFloat64
		v85v3 = typMapMapIntFloat64(v85v1)
		v85v4 = typMapMapIntFloat64(v85v2)
		if v != nil {
			bs85 = testMarshalErr(v85v3, h, t, "enc-map-v85-custom")
			testUnmarshalErr(v85v4, bs85, h, t, "dec-map-v85-p-len")
			testDeepEqualErr(v85v3, v85v4, t, "equal-map-v85-p-len")
		}
	}
	for _, v := range []map[int]bool{nil, {}, {77: false, 127: false}} {
		// fmt.Printf(">>>> running mammoth map v86: %v\n", v)
		var v86v1, v86v2 map[int]bool
		var bs86 []byte
		v86v1 = v
		bs86 = testMarshalErr(v86v1, h, t, "enc-map-v86")
		if v != nil {
			if v == nil {
				v86v2 = nil
			} else {
				v86v2 = make(map[int]bool, len(v))
			} // reset map
			testUnmarshalErr(v86v2, bs86, h, t, "dec-map-v86")
			testDeepEqualErr(v86v1, v86v2, t, "equal-map-v86")
			if v == nil {
				v86v2 = nil
			} else {
				v86v2 = make(map[int]bool, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v86v2), bs86, h, t, "dec-map-v86-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v86v1, v86v2, t, "equal-map-v86-noaddr")
		}
		if v == nil {
			v86v2 = nil
		} else {
			v86v2 = make(map[int]bool, len(v))
		} // reset map
		testUnmarshalErr(&v86v2, bs86, h, t, "dec-map-v86-p-len")
		testDeepEqualErr(v86v1, v86v2, t, "equal-map-v86-p-len")
		bs86 = testMarshalErr(&v86v1, h, t, "enc-map-v86-p")
		v86v2 = nil
		testUnmarshalErr(&v86v2, bs86, h, t, "dec-map-v86-p-nil")
		testDeepEqualErr(v86v1, v86v2, t, "equal-map-v86-p-nil")
		// ...
		if v == nil {
			v86v2 = nil
		} else {
			v86v2 = make(map[int]bool, len(v))
		} // reset map
		var v86v3, v86v4 typMapMapIntBool
		v86v3 = typMapMapIntBool(v86v1)
		v86v4 = typMapMapIntBool(v86v2)
		if v != nil {
			bs86 = testMarshalErr(v86v3, h, t, "enc-map-v86-custom")
			testUnmarshalErr(v86v4, bs86, h, t, "dec-map-v86-p-len")
			testDeepEqualErr(v86v3, v86v4, t, "equal-map-v86-p-len")
		}
	}
	for _, v := range []map[int64]interface{}{nil, {}, {111: nil, 77: "string-is-an-interface-3"}} {
		// fmt.Printf(">>>> running mammoth map v87: %v\n", v)
		var v87v1, v87v2 map[int64]interface{}
		var bs87 []byte
		v87v1 = v
		bs87 = testMarshalErr(v87v1, h, t, "enc-map-v87")
		if v != nil {
			if v == nil {
				v87v2 = nil
			} else {
				v87v2 = make(map[int64]interface{}, len(v))
			} // reset map
			testUnmarshalErr(v87v2, bs87, h, t, "dec-map-v87")
			testDeepEqualErr(v87v1, v87v2, t, "equal-map-v87")
			if v == nil {
				v87v2 = nil
			} else {
				v87v2 = make(map[int64]interface{}, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v87v2), bs87, h, t, "dec-map-v87-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v87v1, v87v2, t, "equal-map-v87-noaddr")
		}
		if v == nil {
			v87v2 = nil
		} else {
			v87v2 = make(map[int64]interface{}, len(v))
		} // reset map
		testUnmarshalErr(&v87v2, bs87, h, t, "dec-map-v87-p-len")
		testDeepEqualErr(v87v1, v87v2, t, "equal-map-v87-p-len")
		bs87 = testMarshalErr(&v87v1, h, t, "enc-map-v87-p")
		v87v2 = nil
		testUnmarshalErr(&v87v2, bs87, h, t, "dec-map-v87-p-nil")
		testDeepEqualErr(v87v1, v87v2, t, "equal-map-v87-p-nil")
		// ...
		if v == nil {
			v87v2 = nil
		} else {
			v87v2 = make(map[int64]interface{}, len(v))
		} // reset map
		var v87v3, v87v4 typMapMapInt64Intf
		v87v3 = typMapMapInt64Intf(v87v1)
		v87v4 = typMapMapInt64Intf(v87v2)
		if v != nil {
			bs87 = testMarshalErr(v87v3, h, t, "enc-map-v87-custom")
			testUnmarshalErr(v87v4, bs87, h, t, "dec-map-v87-p-len")
			testDeepEqualErr(v87v3, v87v4, t, "equal-map-v87-p-len")
		}
	}
	for _, v := range []map[int64]string{nil, {}, {127: "", 111: "some-string-1"}} {
		// fmt.Printf(">>>> running mammoth map v88: %v\n", v)
		var v88v1, v88v2 map[int64]string
		var bs88 []byte
		v88v1 = v
		bs88 = testMarshalErr(v88v1, h, t, "enc-map-v88")
		if v != nil {
			if v == nil {
				v88v2 = nil
			} else {
				v88v2 = make(map[int64]string, len(v))
			} // reset map
			testUnmarshalErr(v88v2, bs88, h, t, "dec-map-v88")
			testDeepEqualErr(v88v1, v88v2, t, "equal-map-v88")
			if v == nil {
				v88v2 = nil
			} else {
				v88v2 = make(map[int64]string, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v88v2), bs88, h, t, "dec-map-v88-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v88v1, v88v2, t, "equal-map-v88-noaddr")
		}
		if v == nil {
			v88v2 = nil
		} else {
			v88v2 = make(map[int64]string, len(v))
		} // reset map
		testUnmarshalErr(&v88v2, bs88, h, t, "dec-map-v88-p-len")
		testDeepEqualErr(v88v1, v88v2, t, "equal-map-v88-p-len")
		bs88 = testMarshalErr(&v88v1, h, t, "enc-map-v88-p")
		v88v2 = nil
		testUnmarshalErr(&v88v2, bs88, h, t, "dec-map-v88-p-nil")
		testDeepEqualErr(v88v1, v88v2, t, "equal-map-v88-p-nil")
		// ...
		if v == nil {
			v88v2 = nil
		} else {
			v88v2 = make(map[int64]string, len(v))
		} // reset map
		var v88v3, v88v4 typMapMapInt64String
		v88v3 = typMapMapInt64String(v88v1)
		v88v4 = typMapMapInt64String(v88v2)
		if v != nil {
			bs88 = testMarshalErr(v88v3, h, t, "enc-map-v88-custom")
			testUnmarshalErr(v88v4, bs88, h, t, "dec-map-v88-p-len")
			testDeepEqualErr(v88v3, v88v4, t, "equal-map-v88-p-len")
		}
	}
	for _, v := range []map[int64][]byte{nil, {}, {77: nil, 127: []byte("some-string-3")}} {
		// fmt.Printf(">>>> running mammoth map v89: %v\n", v)
		var v89v1, v89v2 map[int64][]byte
		var bs89 []byte
		v89v1 = v
		bs89 = testMarshalErr(v89v1, h, t, "enc-map-v89")
		if v != nil {
			if v == nil {
				v89v2 = nil
			} else {
				v89v2 = make(map[int64][]byte, len(v))
			} // reset map
			testUnmarshalErr(v89v2, bs89, h, t, "dec-map-v89")
			testDeepEqualErr(v89v1, v89v2, t, "equal-map-v89")
			if v == nil {
				v89v2 = nil
			} else {
				v89v2 = make(map[int64][]byte, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v89v2), bs89, h, t, "dec-map-v89-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v89v1, v89v2, t, "equal-map-v89-noaddr")
		}
		if v == nil {
			v89v2 = nil
		} else {
			v89v2 = make(map[int64][]byte, len(v))
		} // reset map
		testUnmarshalErr(&v89v2, bs89, h, t, "dec-map-v89-p-len")
		testDeepEqualErr(v89v1, v89v2, t, "equal-map-v89-p-len")
		bs89 = testMarshalErr(&v89v1, h, t, "enc-map-v89-p")
		v89v2 = nil
		testUnmarshalErr(&v89v2, bs89, h, t, "dec-map-v89-p-nil")
		testDeepEqualErr(v89v1, v89v2, t, "equal-map-v89-p-nil")
		// ...
		if v == nil {
			v89v2 = nil
		} else {
			v89v2 = make(map[int64][]byte, len(v))
		} // reset map
		var v89v3, v89v4 typMapMapInt64Bytes
		v89v3 = typMapMapInt64Bytes(v89v1)
		v89v4 = typMapMapInt64Bytes(v89v2)
		if v != nil {
			bs89 = testMarshalErr(v89v3, h, t, "enc-map-v89-custom")
			testUnmarshalErr(v89v4, bs89, h, t, "dec-map-v89-p-len")
			testDeepEqualErr(v89v3, v89v4, t, "equal-map-v89-p-len")
		}
	}
	for _, v := range []map[int64]uint{nil, {}, {111: 0, 77: 127}} {
		// fmt.Printf(">>>> running mammoth map v90: %v\n", v)
		var v90v1, v90v2 map[int64]uint
		var bs90 []byte
		v90v1 = v
		bs90 = testMarshalErr(v90v1, h, t, "enc-map-v90")
		if v != nil {
			if v == nil {
				v90v2 = nil
			} else {
				v90v2 = make(map[int64]uint, len(v))
			} // reset map
			testUnmarshalErr(v90v2, bs90, h, t, "dec-map-v90")
			testDeepEqualErr(v90v1, v90v2, t, "equal-map-v90")
			if v == nil {
				v90v2 = nil
			} else {
				v90v2 = make(map[int64]uint, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v90v2), bs90, h, t, "dec-map-v90-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v90v1, v90v2, t, "equal-map-v90-noaddr")
		}
		if v == nil {
			v90v2 = nil
		} else {
			v90v2 = make(map[int64]uint, len(v))
		} // reset map
		testUnmarshalErr(&v90v2, bs90, h, t, "dec-map-v90-p-len")
		testDeepEqualErr(v90v1, v90v2, t, "equal-map-v90-p-len")
		bs90 = testMarshalErr(&v90v1, h, t, "enc-map-v90-p")
		v90v2 = nil
		testUnmarshalErr(&v90v2, bs90, h, t, "dec-map-v90-p-nil")
		testDeepEqualErr(v90v1, v90v2, t, "equal-map-v90-p-nil")
		// ...
		if v == nil {
			v90v2 = nil
		} else {
			v90v2 = make(map[int64]uint, len(v))
		} // reset map
		var v90v3, v90v4 typMapMapInt64Uint
		v90v3 = typMapMapInt64Uint(v90v1)
		v90v4 = typMapMapInt64Uint(v90v2)
		if v != nil {
			bs90 = testMarshalErr(v90v3, h, t, "enc-map-v90-custom")
			testUnmarshalErr(v90v4, bs90, h, t, "dec-map-v90-p-len")
			testDeepEqualErr(v90v3, v90v4, t, "equal-map-v90-p-len")
		}
	}
	for _, v := range []map[int64]uint8{nil, {}, {111: 0, 77: 127}} {
		// fmt.Printf(">>>> running mammoth map v91: %v\n", v)
		var v91v1, v91v2 map[int64]uint8
		var bs91 []byte
		v91v1 = v
		bs91 = testMarshalErr(v91v1, h, t, "enc-map-v91")
		if v != nil {
			if v == nil {
				v91v2 = nil
			} else {
				v91v2 = make(map[int64]uint8, len(v))
			} // reset map
			testUnmarshalErr(v91v2, bs91, h, t, "dec-map-v91")
			testDeepEqualErr(v91v1, v91v2, t, "equal-map-v91")
			if v == nil {
				v91v2 = nil
			} else {
				v91v2 = make(map[int64]uint8, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v91v2), bs91, h, t, "dec-map-v91-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v91v1, v91v2, t, "equal-map-v91-noaddr")
		}
		if v == nil {
			v91v2 = nil
		} else {
			v91v2 = make(map[int64]uint8, len(v))
		} // reset map
		testUnmarshalErr(&v91v2, bs91, h, t, "dec-map-v91-p-len")
		testDeepEqualErr(v91v1, v91v2, t, "equal-map-v91-p-len")
		bs91 = testMarshalErr(&v91v1, h, t, "enc-map-v91-p")
		v91v2 = nil
		testUnmarshalErr(&v91v2, bs91, h, t, "dec-map-v91-p-nil")
		testDeepEqualErr(v91v1, v91v2, t, "equal-map-v91-p-nil")
		// ...
		if v == nil {
			v91v2 = nil
		} else {
			v91v2 = make(map[int64]uint8, len(v))
		} // reset map
		var v91v3, v91v4 typMapMapInt64Uint8
		v91v3 = typMapMapInt64Uint8(v91v1)
		v91v4 = typMapMapInt64Uint8(v91v2)
		if v != nil {
			bs91 = testMarshalErr(v91v3, h, t, "enc-map-v91-custom")
			testUnmarshalErr(v91v4, bs91, h, t, "dec-map-v91-p-len")
			testDeepEqualErr(v91v3, v91v4, t, "equal-map-v91-p-len")
		}
	}
	for _, v := range []map[int64]uint64{nil, {}, {111: 0, 77: 127}} {
		// fmt.Printf(">>>> running mammoth map v92: %v\n", v)
		var v92v1, v92v2 map[int64]uint64
		var bs92 []byte
		v92v1 = v
		bs92 = testMarshalErr(v92v1, h, t, "enc-map-v92")
		if v != nil {
			if v == nil {
				v92v2 = nil
			} else {
				v92v2 = make(map[int64]uint64, len(v))
			} // reset map
			testUnmarshalErr(v92v2, bs92, h, t, "dec-map-v92")
			testDeepEqualErr(v92v1, v92v2, t, "equal-map-v92")
			if v == nil {
				v92v2 = nil
			} else {
				v92v2 = make(map[int64]uint64, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v92v2), bs92, h, t, "dec-map-v92-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v92v1, v92v2, t, "equal-map-v92-noaddr")
		}
		if v == nil {
			v92v2 = nil
		} else {
			v92v2 = make(map[int64]uint64, len(v))
		} // reset map
		testUnmarshalErr(&v92v2, bs92, h, t, "dec-map-v92-p-len")
		testDeepEqualErr(v92v1, v92v2, t, "equal-map-v92-p-len")
		bs92 = testMarshalErr(&v92v1, h, t, "enc-map-v92-p")
		v92v2 = nil
		testUnmarshalErr(&v92v2, bs92, h, t, "dec-map-v92-p-nil")
		testDeepEqualErr(v92v1, v92v2, t, "equal-map-v92-p-nil")
		// ...
		if v == nil {
			v92v2 = nil
		} else {
			v92v2 = make(map[int64]uint64, len(v))
		} // reset map
		var v92v3, v92v4 typMapMapInt64Uint64
		v92v3 = typMapMapInt64Uint64(v92v1)
		v92v4 = typMapMapInt64Uint64(v92v2)
		if v != nil {
			bs92 = testMarshalErr(v92v3, h, t, "enc-map-v92-custom")
			testUnmarshalErr(v92v4, bs92, h, t, "dec-map-v92-p-len")
			testDeepEqualErr(v92v3, v92v4, t, "equal-map-v92-p-len")
		}
	}
	for _, v := range []map[int64]int{nil, {}, {111: 0, 77: 127}} {
		// fmt.Printf(">>>> running mammoth map v93: %v\n", v)
		var v93v1, v93v2 map[int64]int
		var bs93 []byte
		v93v1 = v
		bs93 = testMarshalErr(v93v1, h, t, "enc-map-v93")
		if v != nil {
			if v == nil {
				v93v2 = nil
			} else {
				v93v2 = make(map[int64]int, len(v))
			} // reset map
			testUnmarshalErr(v93v2, bs93, h, t, "dec-map-v93")
			testDeepEqualErr(v93v1, v93v2, t, "equal-map-v93")
			if v == nil {
				v93v2 = nil
			} else {
				v93v2 = make(map[int64]int, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v93v2), bs93, h, t, "dec-map-v93-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v93v1, v93v2, t, "equal-map-v93-noaddr")
		}
		if v == nil {
			v93v2 = nil
		} else {
			v93v2 = make(map[int64]int, len(v))
		} // reset map
		testUnmarshalErr(&v93v2, bs93, h, t, "dec-map-v93-p-len")
		testDeepEqualErr(v93v1, v93v2, t, "equal-map-v93-p-len")
		bs93 = testMarshalErr(&v93v1, h, t, "enc-map-v93-p")
		v93v2 = nil
		testUnmarshalErr(&v93v2, bs93, h, t, "dec-map-v93-p-nil")
		testDeepEqualErr(v93v1, v93v2, t, "equal-map-v93-p-nil")
		// ...
		if v == nil {
			v93v2 = nil
		} else {
			v93v2 = make(map[int64]int, len(v))
		} // reset map
		var v93v3, v93v4 typMapMapInt64Int
		v93v3 = typMapMapInt64Int(v93v1)
		v93v4 = typMapMapInt64Int(v93v2)
		if v != nil {
			bs93 = testMarshalErr(v93v3, h, t, "enc-map-v93-custom")
			testUnmarshalErr(v93v4, bs93, h, t, "dec-map-v93-p-len")
			testDeepEqualErr(v93v3, v93v4, t, "equal-map-v93-p-len")
		}
	}
	for _, v := range []map[int64]int64{nil, {}, {111: 0, 77: 127}} {
		// fmt.Printf(">>>> running mammoth map v94: %v\n", v)
		var v94v1, v94v2 map[int64]int64
		var bs94 []byte
		v94v1 = v
		bs94 = testMarshalErr(v94v1, h, t, "enc-map-v94")
		if v != nil {
			if v == nil {
				v94v2 = nil
			} else {
				v94v2 = make(map[int64]int64, len(v))
			} // reset map
			testUnmarshalErr(v94v2, bs94, h, t, "dec-map-v94")
			testDeepEqualErr(v94v1, v94v2, t, "equal-map-v94")
			if v == nil {
				v94v2 = nil
			} else {
				v94v2 = make(map[int64]int64, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v94v2), bs94, h, t, "dec-map-v94-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v94v1, v94v2, t, "equal-map-v94-noaddr")
		}
		if v == nil {
			v94v2 = nil
		} else {
			v94v2 = make(map[int64]int64, len(v))
		} // reset map
		testUnmarshalErr(&v94v2, bs94, h, t, "dec-map-v94-p-len")
		testDeepEqualErr(v94v1, v94v2, t, "equal-map-v94-p-len")
		bs94 = testMarshalErr(&v94v1, h, t, "enc-map-v94-p")
		v94v2 = nil
		testUnmarshalErr(&v94v2, bs94, h, t, "dec-map-v94-p-nil")
		testDeepEqualErr(v94v1, v94v2, t, "equal-map-v94-p-nil")
		// ...
		if v == nil {
			v94v2 = nil
		} else {
			v94v2 = make(map[int64]int64, len(v))
		} // reset map
		var v94v3, v94v4 typMapMapInt64Int64
		v94v3 = typMapMapInt64Int64(v94v1)
		v94v4 = typMapMapInt64Int64(v94v2)
		if v != nil {
			bs94 = testMarshalErr(v94v3, h, t, "enc-map-v94-custom")
			testUnmarshalErr(v94v4, bs94, h, t, "dec-map-v94-p-len")
			testDeepEqualErr(v94v3, v94v4, t, "equal-map-v94-p-len")
		}
	}
	for _, v := range []map[int64]float32{nil, {}, {111: 0, 77: 11.1}} {
		// fmt.Printf(">>>> running mammoth map v95: %v\n", v)
		var v95v1, v95v2 map[int64]float32
		var bs95 []byte
		v95v1 = v
		bs95 = testMarshalErr(v95v1, h, t, "enc-map-v95")
		if v != nil {
			if v == nil {
				v95v2 = nil
			} else {
				v95v2 = make(map[int64]float32, len(v))
			} // reset map
			testUnmarshalErr(v95v2, bs95, h, t, "dec-map-v95")
			testDeepEqualErr(v95v1, v95v2, t, "equal-map-v95")
			if v == nil {
				v95v2 = nil
			} else {
				v95v2 = make(map[int64]float32, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v95v2), bs95, h, t, "dec-map-v95-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v95v1, v95v2, t, "equal-map-v95-noaddr")
		}
		if v == nil {
			v95v2 = nil
		} else {
			v95v2 = make(map[int64]float32, len(v))
		} // reset map
		testUnmarshalErr(&v95v2, bs95, h, t, "dec-map-v95-p-len")
		testDeepEqualErr(v95v1, v95v2, t, "equal-map-v95-p-len")
		bs95 = testMarshalErr(&v95v1, h, t, "enc-map-v95-p")
		v95v2 = nil
		testUnmarshalErr(&v95v2, bs95, h, t, "dec-map-v95-p-nil")
		testDeepEqualErr(v95v1, v95v2, t, "equal-map-v95-p-nil")
		// ...
		if v == nil {
			v95v2 = nil
		} else {
			v95v2 = make(map[int64]float32, len(v))
		} // reset map
		var v95v3, v95v4 typMapMapInt64Float32
		v95v3 = typMapMapInt64Float32(v95v1)
		v95v4 = typMapMapInt64Float32(v95v2)
		if v != nil {
			bs95 = testMarshalErr(v95v3, h, t, "enc-map-v95-custom")
			testUnmarshalErr(v95v4, bs95, h, t, "dec-map-v95-p-len")
			testDeepEqualErr(v95v3, v95v4, t, "equal-map-v95-p-len")
		}
	}
	for _, v := range []map[int64]float64{nil, {}, {127: 0, 111: 22.2}} {
		// fmt.Printf(">>>> running mammoth map v96: %v\n", v)
		var v96v1, v96v2 map[int64]float64
		var bs96 []byte
		v96v1 = v
		bs96 = testMarshalErr(v96v1, h, t, "enc-map-v96")
		if v != nil {
			if v == nil {
				v96v2 = nil
			} else {
				v96v2 = make(map[int64]float64, len(v))
			} // reset map
			testUnmarshalErr(v96v2, bs96, h, t, "dec-map-v96")
			testDeepEqualErr(v96v1, v96v2, t, "equal-map-v96")
			if v == nil {
				v96v2 = nil
			} else {
				v96v2 = make(map[int64]float64, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v96v2), bs96, h, t, "dec-map-v96-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v96v1, v96v2, t, "equal-map-v96-noaddr")
		}
		if v == nil {
			v96v2 = nil
		} else {
			v96v2 = make(map[int64]float64, len(v))
		} // reset map
		testUnmarshalErr(&v96v2, bs96, h, t, "dec-map-v96-p-len")
		testDeepEqualErr(v96v1, v96v2, t, "equal-map-v96-p-len")
		bs96 = testMarshalErr(&v96v1, h, t, "enc-map-v96-p")
		v96v2 = nil
		testUnmarshalErr(&v96v2, bs96, h, t, "dec-map-v96-p-nil")
		testDeepEqualErr(v96v1, v96v2, t, "equal-map-v96-p-nil")
		// ...
		if v == nil {
			v96v2 = nil
		} else {
			v96v2 = make(map[int64]float64, len(v))
		} // reset map
		var v96v3, v96v4 typMapMapInt64Float64
		v96v3 = typMapMapInt64Float64(v96v1)
		v96v4 = typMapMapInt64Float64(v96v2)
		if v != nil {
			bs96 = testMarshalErr(v96v3, h, t, "enc-map-v96-custom")
			testUnmarshalErr(v96v4, bs96, h, t, "dec-map-v96-p-len")
			testDeepEqualErr(v96v3, v96v4, t, "equal-map-v96-p-len")
		}
	}
	for _, v := range []map[int64]bool{nil, {}, {77: false, 127: true}} {
		// fmt.Printf(">>>> running mammoth map v97: %v\n", v)
		var v97v1, v97v2 map[int64]bool
		var bs97 []byte
		v97v1 = v
		bs97 = testMarshalErr(v97v1, h, t, "enc-map-v97")
		if v != nil {
			if v == nil {
				v97v2 = nil
			} else {
				v97v2 = make(map[int64]bool, len(v))
			} // reset map
			testUnmarshalErr(v97v2, bs97, h, t, "dec-map-v97")
			testDeepEqualErr(v97v1, v97v2, t, "equal-map-v97")
			if v == nil {
				v97v2 = nil
			} else {
				v97v2 = make(map[int64]bool, len(v))
			} // reset map
			testUnmarshalErr(rv4i(v97v2), bs97, h, t, "dec-map-v97-noaddr") // decode into non-addressable map value
			testDeepEqualErr(v97v1, v97v2, t, "equal-map-v97-noaddr")
		}
		if v == nil {
			v97v2 = nil
		} else {
			v97v2 = make(map[int64]bool, len(v))
		} // reset map
		testUnmarshalErr(&v97v2, bs97, h, t, "dec-map-v97-p-len")
		testDeepEqualErr(v97v1, v97v2, t, "equal-map-v97-p-len")
		bs97 = testMarshalErr(&v97v1, h, t, "enc-map-v97-p")
		v97v2 = nil
		testUnmarshalErr(&v97v2, bs97, h, t, "dec-map-v97-p-nil")
		testDeepEqualErr(v97v1, v97v2, t, "equal-map-v97-p-nil")
		// ...
		if v == nil {
			v97v2 = nil
		} else {
			v97v2 = make(map[int64]bool, len(v))
		} // reset map
		var v97v3, v97v4 typMapMapInt64Bool
		v97v3 = typMapMapInt64Bool(v97v1)
		v97v4 = typMapMapInt64Bool(v97v2)
		if v != nil {
			bs97 = testMarshalErr(v97v3, h, t, "enc-map-v97-custom")
			testUnmarshalErr(v97v4, bs97, h, t, "dec-map-v97-p-len")
			testDeepEqualErr(v97v3, v97v4, t, "equal-map-v97-p-len")
		}
	}

}

func doTestMammothMapsAndSlices(t *testing.T, h Handle) {
	doTestMammothSlices(t, h)
	doTestMammothMaps(t, h)
}
