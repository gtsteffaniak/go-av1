# Binary size: measurement and levers

This documents how big a go-av1 integration is, why, and which build flags
actually change it. Measurement is reproducible with
[`scripts/check-size.sh`](../scripts/check-size.sh), which builds the decode-only
probe in [`tools/sizeprobe`](../tools/sizeprobe/main.go) — a copy of the
go-media thumbnail path (`KeyframeDecoder` + `KeyframeSkipFilmGrain` +
`KeyframeOutput8Bit`) and nothing else.

## Why the whole module does *not* land in your binary

Go's linker performs function-level dead-code elimination, so an integration
only pays for the code it can reach. Measured against this module (Go 1.27,
`linux/amd64`):

| Build                                             | Size    |
| ------------------------------------------------- | ------- |
| hello world                                       | 2.36 MB |
| blank-import root package (reference nothing)      | 2.73 MB |
| decoder-only (`sizeprobe`, default)                | 7.89 MB |
| decoder-only, `GOEXPERIMENT=simd`                  | 10.16 MB |
| decoder-only, stripped (`-ldflags="-s -w"`)        | 5.33 MB |
| decoder-only + `NewVideoEncoder` actually called   | ~10.2 MB |

The decoder-only footprint is the genuine reachable decode pipeline
(`internal/av1/tile`, `threading`, `quantize`, `motion`, `decoder`, `cdef`,
`transform`). The encoder is *already* dead-code-eliminated when it is not
called; only package-level `init` tables (~14 KB) survive, because Go always
links package initialization. The restructure into `pkg/` does not change this
— it buys compiler-enforced boundaries (see
[`conformance/layering_test.go`](../conformance/layering_test.go)) so the
encoder/RTP cannot be pulled in by accident, not a smaller decode binary.

## Levers

### 1. Strip symbols (largest, always available)

```sh
go build -ldflags="-s -w" ./cmd/your-app
```

Removes the DWARF debug info and symbol table: **7.89 MB → 5.33 MB** here. Do
this for release builds unless you need stack traces with symbol names.

### 2. Skip `GOEXPERIMENT=simd` for size-sensitive decode-only consumers

The SIMD experiment instantiates `archsimd` kernels in `transform`, `motion`,
`restoration`, and `prediction`, adding **+2.3 MB (+44% over default, +91% over
stripped)**. It is worth it only when the same process decodes enough volume to
be throughput-bound.

`GOEXPERIMENT` is a build-time switch and must match across every package in the
module graph (dependencies built without it are unaffected, but your main module
must be internally consistent):

```sh
# default: pure Go, smaller
go build ./cmd/your-app

# throughput-optimised
GOEXPERIMENT=simd go build ./cmd/your-app
```

The `filebrowser` backend does not set `GOEXPERIMENT`, and `archsimd` is off by
default in this toolchain, so it is already on the non-SIMD side.

### Matrix

| Levers                 | Approx. size |
| ---------------------- | ------------ |
| default                | 7.89 MB      |
| `-ldflags="-s -w"`     | 5.33 MB      |
| `GOEXPERIMENT=simd`    | 10.16 MB     |
| `simd` + stripped      | 6.63 MB      |

## Verifying

```sh
scripts/check-size.sh [probe.obu]
```

The script:

1. asserts `go list -deps ./pkg/decode` excludes `internal/av1/encoder` and
   `pkg/encode`;
2. builds the decode-only probe default / stripped / SIMD and prints sizes;
3. audits the unstripped binary with `go tool nm` for encoder symbols; and
4. optionally runs the probe on a supplied temporal unit.

It exits non-zero if encoder code leaks into the decode-only build.