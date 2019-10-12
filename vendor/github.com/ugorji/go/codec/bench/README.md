# go-codec-bench

This is a comparison of different binary and text encodings.

We compare the codecs provided by github.com/ugorji/go/codec package,
against other libraries:

[github.com/ugorji/go/codec](http://github.com/ugorji/go) provides:

  - msgpack: [http://github.com/msgpack/msgpack] 
  - binc:    [http://github.com/ugorji/binc]
  - cbor:    [http://cbor.io] [http://tools.ietf.org/html/rfc7049]
  - simple: 
  - json:    [http://json.org] [http://tools.ietf.org/html/rfc7159] 

Other codecs compared include:

  - [gopkg.in/vmihailenco/msgpack.v2](http://gopkg.in/vmihailenco/msgpack.v2)
  - [gopkg.in/mgo.v2/bson](http://gopkg.in/mgo.v2/bson)
  - [github.com/davecgh/go-xdr/xdr2](https://godoc.org/github.com/davecgh/go-xdr/xdr)
  - [github.com/Sereal/Sereal/Go/sereal](https://godoc.org/github.com/Sereal/Sereal/Go/sereal)
  - [code.google.com/p/cbor/go](http://code.google.com/p/cbor/go)
  - [github.com/tinylib/msgp](http://github.com/tinylib/msgp)
  - [github.com/tinylib/msgp](http://godoc.org/github.com/tinylib/msgp)
  - [github.com/pquerna/ffjson/ffjson](http://godoc.org/github.com/pquerna/ffjson/ffjson)
  - [bitbucket.org/bodhisnarkva/cbor/go](http://godoc.org/bitbucket.org/bodhisnarkva/cbor/go)
  - [github.com/json-iterator/go](http://godoc.org/github.com/json-iterator/go)
  - [github.com/mailru/easyjson](http://godoc.org/github.com/mailru/easyjson)
  
# Data

The data being serialized is a `TestStruc` randomly generated values.
See https://github.com/ugorji/go-codec-bench/blob/master/codec/values_test.go for the
definition of the TestStruc.

# Run Benchmarks

See  https://github.com/ugorji/go-codec-bench/blob/master/codec/bench.sh 
for how to download the external libraries which we benchmark against,
generate the files for the types when needed, 
and run the suite of tests.

The 3 suite of benchmarks are

  - CodecSuite
  - XSuite
  - CodecXSuite

```
# Note that `bench.sh` may be in the codec sub-directory, and should be run from there.

# download the code and all its dependencies
./bench.sh -d

# code-generate files needed for benchmarks against ffjson, easyjson, msgp, etc
./bench.sh -c

# run the full suite of tests
./bench.sh -s

# Below, see how to just run some specific suite of tests, knowing the right tags and flags ...
# See bench.sh for different iterations

# Run suite of tests in default mode (selectively using unsafe in specific areas)
go test -tags "alltests x" -bench "CodecXSuite" -benchmem 
# Run suite of tests in safe mode (no usage of unsafe)
go test -tags "alltests x safe" -bench "CodecXSuite" -benchmem 
# Run suite of tests in codecgen mode, including all tests which are generated (msgp, ffjson, etc)
go test -tags "alltests x generated" -bench "CodecXGenSuite" -benchmem 

```

# Issues

The following issues are seen currently (11/20/2014):

- _code.google.com/p/cbor/go_ fails on encoding and decoding the test struct
- _github.com/davecgh/go-xdr/xdr2_ fails on encoding and decoding the test struct
- _github.com/Sereal/Sereal/Go/sereal_ fails on decoding the serialized test struct

# Representative Benchmark Results

Please see the [benchmarking blog post for detailed representative results](http://ugorji.net/blog/benchmarking-serialization-in-go).

A snapshot of some results on my 2016 MacBook Pro is below.  
**Note: errors are truncated, and lines re-arranged, for readability**.

Below are results of running the entire suite on 2017-11-20 (ie running ./bench.sh -s).

What you should notice:

- Results get better with codecgen, showing about 20-50% performance improvement.
  Users should carefully weigh the performance improvements against the 
  usability and binary-size increases, as performance is already extremely good 
  without the codecgen path.
  
See  https://github.com/ugorji/go-codec-bench/blob/master/bench.out.txt for latest run of bench.sh as of 2017-11-20

* snippet of bench.out.txt, running without codecgen *
```
BenchmarkCodecXSuite/options-false.../Benchmark__Msgpack____Encode-8         	   10000	    183961 ns/op	   10224 B/op	      75 allocs/op
BenchmarkCodecXSuite/options-false.../Benchmark__Binc_______Encode-8         	   10000	    206362 ns/op	   12551 B/op	      80 allocs/op
BenchmarkCodecXSuite/options-false.../Benchmark__Simple_____Encode-8         	   10000	    193966 ns/op	   10224 B/op	      75 allocs/op
BenchmarkCodecXSuite/options-false.../Benchmark__Cbor_______Encode-8         	   10000	    192666 ns/op	   10224 B/op	      75 allocs/op
BenchmarkCodecXSuite/options-false.../Benchmark__Json_______Encode-8         	    3000	    475767 ns/op	   10352 B/op	      75 allocs/op
BenchmarkCodecXSuite/options-false.../Benchmark__Std_Json___Encode-8         	    3000	    525223 ns/op	  256049 B/op	     835 allocs/op
BenchmarkCodecXSuite/options-false.../Benchmark__Gob________Encode-8         	    5000	    270550 ns/op	  333548 B/op	     959 allocs/op
BenchmarkCodecXSuite/options-false.../Benchmark__JsonIter___Encode-8         	    3000	    478130 ns/op	  183552 B/op	    3262 allocs/op
BenchmarkCodecXSuite/options-false.../Benchmark__Bson_______Encode-8         	    2000	    747360 ns/op	  715539 B/op	    5629 allocs/op
BenchmarkCodecXSuite/options-false.../Benchmark__VMsgpack___Encode-8         	    2000	    637388 ns/op	  320385 B/op	     542 allocs/op
BenchmarkCodecXSuite/options-false.../Benchmark__Sereal_____Encode-8         	    5000	    361369 ns/op	  294541 B/op	    4286 allocs/op
-------------------------------
BenchmarkCodecXSuite/options-false.../Benchmark__Msgpack____Decode-8         	    5000	    370340 ns/op	  120352 B/op	    1210 allocs/op
BenchmarkCodecXSuite/options-false.../Benchmark__Binc_______Decode-8         	    3000	    443650 ns/op	  126144 B/op	    1263 allocs/op
BenchmarkCodecXSuite/options-false.../Benchmark__Simple_____Decode-8         	    3000	    381155 ns/op	  120352 B/op	    1210 allocs/op
BenchmarkCodecXSuite/options-false.../Benchmark__Cbor_______Decode-8         	    5000	    370754 ns/op	  120352 B/op	    1210 allocs/op
BenchmarkCodecXSuite/options-false.../Benchmark__Json_______Decode-8         	    2000	    719658 ns/op	  159289 B/op	    1478 allocs/op
BenchmarkCodecXSuite/options-false.../Benchmark__Std_Json___Decode-8         	    1000	   2204258 ns/op	  276336 B/op	    6959 allocs/op
BenchmarkCodecXSuite/options-false.../Benchmark__Gob________Decode-8         	    5000	    383884 ns/op	  256684 B/op	    3261 allocs/op
BenchmarkCodecXSuite/options-false.../Benchmark__JsonIter___Decode-8         	    2000	    907079 ns/op	  301520 B/op	    7769 allocs/op
BenchmarkCodecXSuite/options-false.../Benchmark__Bson_______Decode-8         	    2000	   1146851 ns/op	  373121 B/op	   15703 allocs/op
```

* snippet of bench.out.txt, running with codecgen *
```
BenchmarkCodecXGenSuite/options-false.../Benchmark__Msgpack____Encode-8         	   10000	    124729 ns/op	    6224 B/op	       7 allocs/op
BenchmarkCodecXGenSuite/options-false.../Benchmark__Binc_______Encode-8         	   10000	    119745 ns/op	    6256 B/op	       7 allocs/op
BenchmarkCodecXGenSuite/options-false.../Benchmark__Simple_____Encode-8         	   10000	    132501 ns/op	    6224 B/op	       7 allocs/op
BenchmarkCodecXGenSuite/options-false.../Benchmark__Cbor_______Encode-8         	   10000	    129706 ns/op	    6224 B/op	       7 allocs/op
BenchmarkCodecXGenSuite/options-false.../Benchmark__Json_______Encode-8         	    3000	    436958 ns/op	    6352 B/op	       7 allocs/op
BenchmarkCodecXGenSuite/options-false.../Benchmark__Std_Json___Encode-8         	    3000	    539884 ns/op	  256049 B/op	     835 allocs/op
BenchmarkCodecXGenSuite/options-false.../Benchmark__Gob________Encode-8         	    5000	    270663 ns/op	  333548 B/op	     959 allocs/op
BenchmarkCodecXGenSuite/options-false.../Benchmark__JsonIter___Encode-8         	    3000	    476215 ns/op	  183552 B/op	    3262 allocs/op
BenchmarkCodecXGenSuite/options-false.../Benchmark__Bson_______Encode-8         	    2000	    741688 ns/op	  715539 B/op	    5629 allocs/op
BenchmarkCodecXGenSuite/options-false.../Benchmark__VMsgpack___Encode-8         	    2000	    649516 ns/op	  320385 B/op	     542 allocs/op
BenchmarkCodecXGenSuite/options-false.../Benchmark__Msgp_______Encode-8         	   30000	     57573 ns/op	       0 B/op	       0 allocs/op
BenchmarkCodecXGenSuite/options-false.../Benchmark__Easyjson___Encode-8         	    5000	    366701 ns/op	   92762 B/op	      14 allocs/op
BenchmarkCodecXGenSuite/options-false.../Benchmark__Ffjson_____Encode-8         	    3000	    568665 ns/op	  219803 B/op	    1569 allocs/op
BenchmarkCodecXGenSuite/options-false.../Benchmark__Sereal_____Encode-8         	    5000	    365595 ns/op	  296303 B/op	    4285 allocs/op
-------------------------------
BenchmarkCodecXGenSuite/options-false.../Benchmark__Msgpack____Decode-8         	   10000	    244013 ns/op	  131912 B/op	    1112 allocs/op
BenchmarkCodecXGenSuite/options-false.../Benchmark__Binc_______Decode-8         	    5000	    280478 ns/op	  131944 B/op	    1112 allocs/op
BenchmarkCodecXGenSuite/options-false.../Benchmark__Simple_____Decode-8         	    5000	    247863 ns/op	  131912 B/op	    1112 allocs/op
BenchmarkCodecXGenSuite/options-false.../Benchmark__Cbor_______Decode-8         	   10000	    244624 ns/op	  131912 B/op	    1112 allocs/op
BenchmarkCodecXGenSuite/options-false.../Benchmark__Json_______Decode-8         	    3000	    571572 ns/op	  170824 B/op	    1376 allocs/op
BenchmarkCodecXGenSuite/options-false.../Benchmark__Std_Json___Decode-8         	    1000	   2224320 ns/op	  276337 B/op	    6959 allocs/op
BenchmarkCodecXGenSuite/options-false.../Benchmark__Gob________Decode-8         	    5000	    387137 ns/op	  256683 B/op	    3261 allocs/op
BenchmarkCodecXGenSuite/options-false.../Benchmark__JsonIter___Decode-8         	    2000	    913324 ns/op	  301472 B/op	    7769 allocs/op
BenchmarkCodecXGenSuite/options-false.../Benchmark__Bson_______Decode-8         	    2000	   1139852 ns/op	  373121 B/op	   15703 allocs/op
BenchmarkCodecXGenSuite/options-false.../Benchmark__Msgp_______Decode-8         	   10000	    124270 ns/op	  112688 B/op	    1058 allocs/op
BenchmarkCodecXGenSuite/options-false.../Benchmark__Easyjson___Decode-8         	    3000	    521070 ns/op	  184176 B/op	    1371 allocs/op
BenchmarkCodecXGenSuite/options-false.../Benchmark__Ffjson_____Decode-8         	    2000	    970256 ns/op	  161798 B/op	    1927 allocs/op
```
