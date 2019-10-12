// Copyright (c) 2012-2018 Ugorji Nwoke. All rights reserved.
// Use of this source code is governed by a MIT license found in the LICENSE file.

package codec

// This file sets up the variables used, including testInitFns.
// Each file should add initialization that should be performed
// after flags are parsed.
//
// init is a multi-step process:
//   - setup vars (handled by init functions in each file)
//   - parse flags
//   - setup derived vars (handled by pre-init registered functions - registered in init function)
//   - post init (handled by post-init registered functions - registered in init function)
// This way, no one has to manage carefully control the initialization
// using file names, etc.
//
// Tests which require external dependencies need the -tag=x parameter.
// They should be run as:
//    go test -tags=x -run=. <other parameters ...>
// Benchmarks should also take this parameter, to include the sereal, xdr, etc.
// To run against codecgen, etc, make sure you pass extra parameters.
// Example usage:
//    go test "-tags=x codecgen" -bench=. <other parameters ...>
//
// To fully test everything:
//    go test -tags=x -benchtime=100ms -tv -bg -bi  -brw -bu -v -run=. -bench=.

// Handling flags
// codec_test.go will define a set of global flags for testing, including:
//   - Use Reset
//   - Use IO reader/writer (vs direct bytes)
//   - Set Canonical
//   - Set InternStrings
//   - Use Symbols
//
// This way, we can test them all by running same set of tests with a different
// set of flags.
//
// Following this, all the benchmarks will utilize flags set by codec_test.go
// and will not redefine these "global" flags.

import (
	"bytes"
	"flag"
	"io"
	"io/ioutil"
	"log"
	"sync"
)

// __DO_NOT_REMOVE__NEEDED_FOR_REPLACING__IMPORT_PATH__FOR_CODEC_BENCH__

type testHED struct {
	H Handle
	E *Encoder
	D *Decoder
}

type ioReaderWrapper struct {
	r io.Reader
}

func (x ioReaderWrapper) Read(p []byte) (n int, err error) {
	return x.r.Read(p)
}

type ioWriterWrapper struct {
	w io.Writer
}

func (x ioWriterWrapper) Write(p []byte) (n int, err error) {
	return x.w.Write(p)
}

var (
	// testNoopH    = NoopHandle(8)
	testMsgpackH = &MsgpackHandle{}
	testBincH    = &BincHandle{}
	testSimpleH  = &SimpleHandle{}
	testCborH    = &CborHandle{}
	testJsonH    = &JsonHandle{}

	testHandles     []Handle
	testPreInitFns  []func()
	testPostInitFns []func()

	testOnce sync.Once

	testHEDs []testHED
)

// flag variables used by tests (and bench)
var (
	testVerbose bool

	//depth of 0 maps to ~400bytes json-encoded string, 1 maps to ~1400 bytes, etc
	//For depth>1, we likely trigger stack growth for encoders, making benchmarking unreliable.
	testDepth int

	testMaxInitLen int

	testInitDebug bool
	testUseReset  bool
	testSkipIntf  bool
	testUseMust   bool

	testUseIoEncDec  int
	testUseIoWrapper bool

	testNumRepeatString int

	testRpcBufsize       int
	testMapStringKeyOnly bool
)

// variables that are not flags, but which can configure the handles
var (
	testEncodeOptions EncodeOptions
	testDecodeOptions DecodeOptions
)

func init() {
	log.SetOutput(ioutil.Discard) // don't allow things log to standard out/err
	testHEDs = make([]testHED, 0, 32)
	testHandles = append(testHandles,
		// testNoopH,
		testMsgpackH, testBincH, testSimpleH, testCborH, testJsonH)
	// JSON should do HTMLCharsAsIs by default
	testJsonH.HTMLCharsAsIs = true
	// set ExplicitRelease on each handle
	testMsgpackH.ExplicitRelease = true
	testBincH.ExplicitRelease = true
	testSimpleH.ExplicitRelease = true
	testCborH.ExplicitRelease = true
	testJsonH.ExplicitRelease = true

	testInitFlags()
	benchInitFlags()
}

func testInitFlags() {
	// delete(testDecOpts.ExtFuncs, timeTyp)
	flag.BoolVar(&testVerbose, "tv", false, "Text Extra Verbose Logging if -v if set")
	flag.BoolVar(&testInitDebug, "tg", false, "Test Init Debug")
	flag.IntVar(&testUseIoEncDec, "ti", -1, "Use IO Reader/Writer for Marshal/Unmarshal ie >= 0")
	flag.BoolVar(&testUseIoWrapper, "tiw", false, "Wrap the IO Reader/Writer with a base pass-through reader/writer")

	flag.BoolVar(&testSkipIntf, "tf", false, "Skip Interfaces")
	flag.BoolVar(&testUseReset, "tr", false, "Use Reset")
	flag.IntVar(&testNumRepeatString, "trs", 8, "Create string variables by repeating a string N times")
	flag.BoolVar(&testUseMust, "tm", true, "Use Must(En|De)code")

	flag.IntVar(&testMaxInitLen, "tx", 0, "Max Init Len")

	flag.IntVar(&testDepth, "tsd", 0, "Test Struc Depth")
	flag.BoolVar(&testMapStringKeyOnly, "tsk", false, "use maps with string keys only")
}

func benchInitFlags() {
	// flags reproduced here for compatibility (duplicate some in testInitFlags)
	flag.BoolVar(&testMapStringKeyOnly, "bs", false, "use maps with string keys only")
	flag.IntVar(&testDepth, "bd", 1, "Bench Depth")
}

func testHEDGet(h Handle) *testHED {
	for i := range testHEDs {
		v := &testHEDs[i]
		if v.H == h {
			return v
		}
	}
	testHEDs = append(testHEDs, testHED{h, NewEncoder(nil, h), NewDecoder(nil, h)})
	return &testHEDs[len(testHEDs)-1]
}

func testReinit() {
	testOnce = sync.Once{}
	testHEDs = nil
}

func testInitAll() {
	// only parse it once.
	if !flag.Parsed() {
		flag.Parse()
	}
	for _, f := range testPreInitFns {
		f()
	}
	for _, f := range testPostInitFns {
		f()
	}
}

func sTestCodecEncode(ts interface{}, bsIn []byte, fn func([]byte) *bytes.Buffer,
	h Handle, bh *BasicHandle) (bs []byte, err error) {
	// bs = make([]byte, 0, approxSize)
	var e *Encoder
	var buf *bytes.Buffer
	if testUseReset {
		e = testHEDGet(h).E
	} else {
		e = NewEncoder(nil, h)
	}
	var oldWriteBufferSize int
	if testUseIoEncDec >= 0 {
		buf = fn(bsIn)
		// set the encode options for using a buffer
		oldWriteBufferSize = bh.WriterBufferSize
		bh.WriterBufferSize = testUseIoEncDec
		if testUseIoWrapper {
			e.Reset(ioWriterWrapper{buf})
		} else {
			e.Reset(buf)
		}
	} else {
		bs = bsIn
		e.ResetBytes(&bs)
	}
	if testUseMust {
		e.MustEncode(ts)
	} else {
		err = e.Encode(ts)
	}
	if testUseIoEncDec >= 0 {
		bs = buf.Bytes()
		bh.WriterBufferSize = oldWriteBufferSize
	}
	if !testUseReset {
		e.Release()
	}
	return
}

func sTestCodecDecode(bs []byte, ts interface{}, h Handle, bh *BasicHandle) (err error) {
	var d *Decoder
	// var buf *bytes.Reader
	if testUseReset {
		d = testHEDGet(h).D
	} else {
		d = NewDecoder(nil, h)
	}
	var oldReadBufferSize int
	if testUseIoEncDec >= 0 {
		buf := bytes.NewReader(bs)
		oldReadBufferSize = bh.ReaderBufferSize
		bh.ReaderBufferSize = testUseIoEncDec
		if testUseIoWrapper {
			d.Reset(ioReaderWrapper{buf})
		} else {
			d.Reset(buf)
		}
	} else {
		d.ResetBytes(bs)
	}
	if testUseMust {
		d.MustDecode(ts)
	} else {
		err = d.Decode(ts)
	}
	if testUseIoEncDec >= 0 {
		bh.ReaderBufferSize = oldReadBufferSize
	}
	if !testUseReset {
		d.Release()
	}
	return
}

// // --- functions below are used by both benchmarks and tests

// // log message only when testVerbose = true (ie go test ... -- -tv).
// //
// // These are for intormational messages that do not necessarily
// // help with diagnosing a failure, or which are too large.
// func logTv(x interface{}, format string, args ...interface{}) {
// 	if !testVerbose {
// 		return
// 	}
// 	if t, ok := x.(testing.TB); ok { // only available from go 1.9
// 		t.Helper()
// 	}
// 	logT(x, format, args...)
// }

// // logT logs messages when running as go test -v
// //
// // Use it for diagnostics messages that help diagnost failure,
// // and when the output is not too long ie shorter than like 100 characters.
// //
// // In general, any logT followed by failT should call this.
// func logT(x interface{}, format string, args ...interface{}) {
// 	if x == nil {
// 		if len(format) == 0 || format[len(format)-1] != '\n' {
// 			format = format + "\n"
// 		}
// 		fmt.Printf(format, args...)
// 		return
// 	}
// 	if t, ok := x.(testing.TB); ok { // only available from go 1.9
// 		t.Helper()
// 		t.Logf(format, args...)
// 	}
// }

// func failTv(x testing.TB, args ...interface{}) {
// 	x.Helper()
// 	if testVerbose {
// 		failTMsg(x, args...)
// 	}
// 	x.FailNow()
// }

// func failT(x testing.TB, args ...interface{}) {
// 	x.Helper()
// 	failTMsg(x, args...)
// 	x.FailNow()
// }

// func failTMsg(x testing.TB, args ...interface{}) {
// 	x.Helper()
// 	if len(args) > 0 {
// 		if format, ok := args[0].(string); ok {
// 			logT(x, format, args[1:]...)
// 		} else if len(args) == 1 {
// 			logT(x, "%v", args[0])
// 		} else {
// 			logT(x, "%v", args)
// 		}
// 	}
// }

// --- functions below are used only by benchmarks alone

func fnBenchmarkByteBuf(bsIn []byte) (buf *bytes.Buffer) {
	// var buf bytes.Buffer
	// buf.Grow(approxSize)
	buf = bytes.NewBuffer(bsIn)
	buf.Truncate(0)
	return
}

// func benchFnCodecEncode(ts interface{}, bsIn []byte, h Handle) (bs []byte, err error) {
// 	return testCodecEncode(ts, bsIn, fnBenchmarkByteBuf, h)
// }

// func benchFnCodecDecode(bs []byte, ts interface{}, h Handle) (err error) {
// 	return testCodecDecode(bs, ts, h)
// }
