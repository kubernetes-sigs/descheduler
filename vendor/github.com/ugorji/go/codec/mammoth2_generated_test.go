// +build !notfastpath

// Copyright (c) 2012-2018 Ugorji Nwoke. All rights reserved.
// Use of this source code is governed by a MIT license found in the LICENSE file.

// Code generated from mammoth2-test.go.tmpl - DO NOT EDIT.

package codec

// Increase codecoverage by covering all the codecgen paths, in fast-path and gen-helper.go....
//
// Add:
// - test file for creating a mammoth generated file as _mammoth_generated.go
//   - generate a second mammoth files in a different file: mammoth2_generated_test.go
//     - mammoth-test.go.tmpl will do this
//   - run codecgen on it, into mammoth2_codecgen_generated_test.go (no build tags)
//   - as part of TestMammoth, run it also
//   - this will cover all the codecgen, gen-helper, etc in one full run
//   - check in mammoth* files into github also
// - then
//
// Now, add some types:
//  - some that implement BinaryMarshal, TextMarshal, JSONMarshal, and one that implements none of it
//  - create a wrapper type that includes TestMammoth2, with it in slices, and maps, and the custom types
//  - this wrapper object is what we work encode/decode (so that the codecgen methods are called)

// import "encoding/binary"
import "fmt"

type TestMammoth2 struct {
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

// -----------

type testMammoth2Binary uint64

func (x testMammoth2Binary) MarshalBinary() (data []byte, err error) {
	data = make([]byte, 8)
	bigen.PutUint64(data, uint64(x))
	return
}
func (x *testMammoth2Binary) UnmarshalBinary(data []byte) (err error) {
	*x = testMammoth2Binary(bigen.Uint64(data))
	return
}

type testMammoth2Text uint64

func (x testMammoth2Text) MarshalText() (data []byte, err error) {
	data = []byte(fmt.Sprintf("%b", uint64(x)))
	return
}
func (x *testMammoth2Text) UnmarshalText(data []byte) (err error) {
	_, err = fmt.Sscanf(string(data), "%b", (*uint64)(x))
	return
}

type testMammoth2Json uint64

func (x testMammoth2Json) MarshalJSON() (data []byte, err error) {
	data = []byte(fmt.Sprintf("%v", uint64(x)))
	return
}
func (x *testMammoth2Json) UnmarshalJSON(data []byte) (err error) {
	_, err = fmt.Sscanf(string(data), "%v", (*uint64)(x))
	return
}

type testMammoth2Basic [4]uint64

type TestMammoth2Wrapper struct {
	V TestMammoth2
	T testMammoth2Text
	B testMammoth2Binary
	J testMammoth2Json
	C testMammoth2Basic
	M map[testMammoth2Basic]TestMammoth2
	L []TestMammoth2
	A [4]int64
}
