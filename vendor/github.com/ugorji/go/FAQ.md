# FAQ

## Managing Binary Size

This package adds some size to any binary that depends on it.
This is because we include an auto-generated file: `fast-path.generated.go`
to help with performance when encoding/decoding slices and maps of
built in numeric, boolean, string and interface{} types.

Prior to 2019-05-16, this package could add about `11MB` to the size of
your binaries.  We have now trimmed that in half, and the package
contributes about `5.5MB`.  This compares favorably to other packages like
`json-iterator/go` `(3.3MB)` and `net/http` `(3.5MB)`.

Furthermore, you can bypass building `fast-path.generated.go`, by building 
(or running tests and benchmarks) with the tag: `notfastpath`.

    go install -tags notfastpath
    go build -tags notfastpath
    go test -tags notfastpath

With the tag `notfastpath`, we trim that size to about `2.8MB`.

Be aware that, at least in our representative microbenchmarks for cbor (for example),
passing `notfastpath` tag causes significant performance loss (about 33%).  
*YMMV*.

## Resolving Module Issues

Prior to v1.1.5, go-codec unknowingly introduced some headaches for its
users while introducing module support. We tried to make
`github.com/ugorji/go/codec` a module. At that time, multi-repository
module support was weak, so we reverted and made `github.com/ugorji/go/`
the module. However, folks previously used go-codec in module mode
before it formally supported modules. Eventually, different established packages
had go.mod files contain various real and pseudo versions of go-codec
which causes `go` to barf with `ambiguous import` error.

To resolve this, from v1.1.5 and up, we use a requirements cycle between
modules `github.com/ugorji/go/codec` and `github.com/ugorji/go/`,
tagging them with parallel consistent tags (`codec/vX.Y.Z and vX.Y.Z`)
to the same commit.

Fixing `ambiguous import` failure is now as simple as running

```
go get -u github.com/ugorji/go/codec@latest
```

