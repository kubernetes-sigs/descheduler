// Copyright (c) 2012-2018 Ugorji Nwoke. All rights reserved.
// Use of this source code is governed by a MIT license found in the LICENSE file.

package codec

// bench_test is the "helper" file for all benchmarking tests.
//
// There are also benchmarks which depend on just codec and the stdlib,
// and benchmarks which depend on external libraries.
// It is an explicit goal that you can run benchmarks without external
// dependencies (which is why the 'x' build tag was explicitly introduced).
//
// There are 2 ways of running tests:
//    - generated
//    - not generated
//
// Consequently, we have 4 groups:
//    - codec_bench   (gen, !gen)
//    - stdlib_bench  (!gen only)
//    - x_bench       (!gen only)
//    - x_bench_gen   (gen only)
//
// We also have 4 matching suite files.
//    - z_all_bench (rename later to z_all_codec_bench???)
//    - z_all_stdlib_bench
//    - z_all_x_bench
//    - z_all_x_bench_gen
//
// Finally, we have a single test (TestBenchInit) that
// will log information about whether each format can
// encode or not, how long to encode (unscientifically),
// and the encode size.
//
// This test MUST be run always, as it calls init() internally

import (
	"reflect"
	"runtime"
	"testing"
	"time"
)

// Sample way to run:
// go test -bi -bv -bd=1 -benchmem -bench=.

const (
	benchUnscientificRes = true
	benchVerify          = true
	benchRecover         = true
	benchShowJsonOnError = true
)

var (
	benchTs       *TestStruc
	approxSize    int
	benchCheckers []benchChecker
)

type benchEncFn func(interface{}, []byte) ([]byte, error)
type benchDecFn func([]byte, interface{}) error
type benchIntfFn func() interface{}

type benchChecker struct {
	name     string
	encodefn benchEncFn
	decodefn benchDecFn
}

func init() {
	testPreInitFns = append(testPreInitFns, benchPreInit)
	// testPostInitFns = append(testPostInitFns, codecbenchPostInit)
}

func benchPreInit() {
	benchTs = newTestStruc(testDepth, testNumRepeatString, true, !testSkipIntf, testMapStringKeyOnly)
	approxSize = approxDataSize(reflect.ValueOf(benchTs)) * 3 / 2 // multiply by 1.5 to appease msgp, and prevent alloc
	// bytesLen := 1024 * 4 * (testDepth + 1) * (testDepth + 1)
	// if bytesLen < approxSize {
	// 	bytesLen = approxSize
	// }
}

func benchReinit() {
	benchCheckers = nil
}

func benchmarkDivider() {
	// logTv(nil, "-------------------------------\n")
	println()
}

// func Test0(t *testing.T) {
// 	testOnce.Do(testInitAll)
// }

func TestBenchInit(t *testing.T) {
	testOnce.Do(testInitAll)
	if !testing.Verbose() {
		return
	}
	// t.Logf("..............................................")
	t.Logf("BENCHMARK INIT: %v", time.Now())
	// t.Logf("To run full benchmark comparing encodings, use: \"go test -bench=.\"")
	t.Logf("Benchmark: ")
	t.Logf("\tStruct recursive Depth:             %d", testDepth)
	if approxSize > 0 {
		t.Logf("\tApproxDeepSize Of benchmark Struct: %d bytes", approxSize)
	}
	if benchUnscientificRes {
		t.Logf("Benchmark One-Pass Run (with Unscientific Encode/Decode times): ")
	} else {
		t.Logf("Benchmark One-Pass Run:")
	}
	for _, bc := range benchCheckers {
		doBenchCheck(t, bc.name, bc.encodefn, bc.decodefn)
	}
	if testVerbose {
		t.Logf("..............................................")
		t.Logf("<<<<====>>>> depth: %v, ts: %#v\n", testDepth, benchTs)
	}
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
}

var vBenchTs = TestStruc{}

func fnBenchNewTs() interface{} {
	vBenchTs = TestStruc{}
	return &vBenchTs
	// return new(TestStruc)
}

// const benchCheckDoDeepEqual = false

func benchRecoverPanic(t *testing.B) {
	if benchRecover {
		if r := recover(); r != nil {
			t.Logf("(recovered) panic: %v\n", r)
		}
	}
}

func benchRecoverPanicT(t *testing.T) {
	if benchRecover {
		if r := recover(); r != nil {
			t.Logf("(recovered) panic: %v\n", r)
		}
	}
}

func doBenchCheck(t *testing.T, name string, encfn benchEncFn, decfn benchDecFn) {
	// if benchUnscientificRes {
	// 	t.Logf("-------------- %s ----------------", name)
	// }
	defer benchRecoverPanicT(t)
	runtime.GC()
	tnow := time.Now()
	buf, err := encfn(benchTs, nil)
	if err != nil {
		t.Logf("\t%10s: **** Error encoding benchTs: %v", name, err)
		return
	}
	encDur := time.Since(tnow)
	encLen := len(buf)
	runtime.GC()
	if !benchUnscientificRes {
		t.Logf("\t%10s: len: %d bytes\n", name, encLen)
		return
	}
	tnow = time.Now()
	var ts2 TestStruc
	if err = decfn(buf, &ts2); err != nil {
		t.Logf("\t%10s: **** Error decoding into new TestStruc: %v", name, err)
		return
	}
	decDur := time.Since(tnow)
	// if benchCheckDoDeepEqual {
	if benchVerify {
		err = deepEqual(benchTs, &ts2)
		if err == nil {
			t.Logf("\t%10s: len: %d bytes,\t encode: %v,\t decode: %v,\tencoded == decoded", name, encLen, encDur, decDur)
		} else {
			t.Logf("\t%10s: len: %d bytes,\t encode: %v,\t decode: %v,\tencoded != decoded: %v", name, encLen, encDur, decDur, err)
			// if benchShowJsonOnError && strings.Contains(name, "json") {
			// 	fmt.Printf("\n\n%s\n\n", buf)
			// 	//fmt.Printf("\n\n%#v\n\n", benchTs)
			// 	//fmt.Printf("\n\n%#v\n\n", &ts2)
			// 	return
			// }

			// if strings.Contains(name, "json") {
			// 	println(">>>>>")
			// 	f1, _ := os.Create("1.out")
			// 	f2, _ := os.Create("2.out")
			// 	f3, _ := os.Create("3.json")
			// 	buf3, _ := json.MarshalIndent(&ts2, "", "\t")
			// 	spew.Config.SortKeys = true
			// 	spew.Config.SpewKeys = true
			// 	println("^^^^^^^^^^^^^^")
			// 	spew.Fdump(f1, benchTs)
			// 	println("^^^^^^^^^^^^^^")
			// 	spew.Fdump(f2, &ts2)
			// 	println("^^^^^^^^^^^^^^")
			// 	f3.Write(buf3)
			// 	f1.Close()
			// 	f2.Close()
			// 	f3.Close()
			// }
			// t.Logf("\t: err: %v,\n benchTs: %#v\n\n, ts2: %#v\n\n", err, benchTs, ts2) // TODO: remove
			// t.Logf("BenchVerify: Error comparing en|decoded TestStruc: %v", err)
			// return
			// t.Logf("BenchVerify: Error comparing benchTs: %v\n--------\n%v\n--------\n%v", err, benchTs, ts2)
			// if strings.Contains(name, "json") {
			// 	t.Logf("\n\tDECODED FROM\n--------\n%s", buf)
			// }
		}
	} else {
		t.Logf("\t%10s: len: %d bytes,\t encode: %v,\t decode: %v", name, encLen, encDur, decDur)
	}
	return
}

func fnBenchmarkEncode(b *testing.B, encName string, ts interface{}, encfn benchEncFn) {
	defer benchRecoverPanic(b)
	testOnce.Do(testInitAll)
	var err error
	bs := make([]byte, 0, approxSize)
	runtime.GC()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err = encfn(ts, bs); err != nil {
			break
		}
	}
	if err != nil {
		b.Logf("Error encoding benchTs: %s: %v", encName, err)
		b.FailNow()
	}
}

func fnBenchmarkDecode(b *testing.B, encName string, ts interface{},
	encfn benchEncFn, decfn benchDecFn, newfn benchIntfFn,
) {
	defer benchRecoverPanic(b)
	testOnce.Do(testInitAll)
	bs := make([]byte, 0, approxSize)
	buf, err := encfn(ts, bs)
	if err != nil {
		b.Logf("Error encoding benchTs: %s: %v", encName, err)
		b.FailNow()
	}
	// if false && benchVerify { // do not do benchVerify during decode
	// 	// ts2 := newfn()
	// 	ts1 := ts.(*TestStruc)
	// 	ts2 := new(TestStruc)
	// 	if err = decfn(buf, ts2); err != nil {
	// 		failT(b, "BenchVerify: Error decoding benchTs: %s: %v", encName, err)
	// 	}
	// 	if err = deepEqual(ts1, ts2); err != nil {
	// 		failT(b, "BenchVerify: Error comparing benchTs: %s: %v", encName, err)
	// 	}
	// }
	runtime.GC()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ts = newfn()
		if err = decfn(buf, ts); err != nil {
			break
		}
	}
	if err != nil {
		b.Logf("Error decoding into new TestStruc: %s: %v", encName, err)
		b.FailNow()
	}
}
