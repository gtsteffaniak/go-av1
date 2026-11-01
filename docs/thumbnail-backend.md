# Thumbnail / go-media backend notes

## Module

Import path: `github.com/gtsteffaniak/go-av1` (package name `goav1`).

## Container API (`container/`)

- `ParseAv1C`, `BuildTemporalUnit(configOBUs, sample)` — uses `internal/av1/obu` only (no encoder import). Optional av1C config (ffmpeg AVIF often omits sequence header in av1C).
- `ParseAVIF`, `TemporalUnitFromAVIF` — pitm/iinf/iloc/iprp/av1C/ispe primary item.
- `ParseMP4`, `TemporalUnitFromMP4Sample` — av01 sample description + mdat samples.
- `ParseWebM`, `TemporalUnitFromWebMSample` — Matroska SimpleBlock/Block, BlockGroup, lacing, CodecPrivate av1C.

`container/isobmff` is adapted from [KarpelesLab/goavif](https://github.com/KarpelesLab/goavif) (MIT); see `container/NOTICE`.

## Keyframe decode (`KeyframeDecoder`)

- `NewKeyframeDecoder(probe, KeyframeSkipFilmGrain(), KeyframeOutput8Bit())`
- `DecodeSample(configOBUs, sample)` / `DecodeTemporalUnit(unit)`
- `ParseAVIFGrid`, `DecodeAVIFGridKeyframe` — AVIF grid tile compositing (dimg iref + per-tile decode).

## SIMD / builds

- `scripts/go.sh` sets `GOEXPERIMENT=simd` by default (go-vpx pattern).
- amd64 hot paths use runtime `archsimd.X86.AVX2()` dispatch in `init()` (loop filter, CDEF direction, inverse transforms, intra, restoration, etc.) with pure-Go fallbacks and differential tests in-tree.
- Dedicated AVX-512 kernel variants are not added where narrowing would require AVX-512-only ops (see comments in `loopfilter/filter_lanes_amd64.go`, `restoration/wiener_vertical_gosimd_amd64.go`); tier remains AVX2 → scalar on amd64.
- Decode PGO (`decode.pgo` from keyframe benches) was tried on this VM and did not improve `BenchmarkKeyframeDecoder1080p8`; not checked in.

## Encoder / RTP isolation

- `container/` and decode internals do not import `internal/av1/encoder`.
- New import paths: `github.com/gtsteffaniak/go-av1/encode` and `/rtp` re-export root encoder/RTP APIs for apps that want an explicit dependency edge; full file split from root `goav1` remains a follow-up.

## Benchmarks (linux/amd64, GOEXPERIMENT=simd, GOMAXPROCS=1)

| Case | One-shot `DecodeIVF` | `KeyframeDecoder` (reuse) |
|------|----------------------|---------------------------|
| 1080p 8-bit keyframe | ~20 ms/op, ~74 MB/op, 64 allocs | ~14.2 ms/op, 4 KB/op, 5 allocs |
| 1080p 10-bit → 8-bit | — | ~23.6 ms/op |
| 4K 8-bit keyframe | — | ~39.1 ms/op |

| Kernel (amd64, AVX2 dispatch) | Approx. steady-state |
|-------------------------------|----------------------|
| Loop filter 4-tap vertical | ~850 ns/op |
| Loop filter 8-tap horizontal | ~260 ns/op |
| Loop filter 8-tap vertical 10-bit | ~923 ns/op |

Cross-compile (`scripts/xc.sh`, `CGO_ENABLED=0`): all listed targets `ok`.
