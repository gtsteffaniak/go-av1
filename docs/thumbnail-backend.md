# Thumbnail / go-media backend notes

## Container API (`container/`)

- `ParseAv1C`, `BuildTemporalUnit(configOBUs, sample)` — optional av1C config (ffmpeg AVIF often omits sequence header in av1C).
- `ParseAVIF`, `TemporalUnitFromAVIF` — pitm/iinf/iloc/iprp/av1C/ispe primary item.
- `ParseMP4`, `TemporalUnitFromMP4Sample` — av01 sample description + mdat samples.
- `ParseWebM`, `TemporalUnitFromWebMSample` — implemented; block→OBU merge still needs lacing/block-group work (test skipped).

`container/isobmff` is adapted from [KarpelesLab/goavif](https://github.com/KarpelesLab/goavif) (MIT); see `container/NOTICE`.

## Keyframe decode (`KeyframeDecoder`)

- `NewKeyframeDecoder(probe, KeyframeSkipFilmGrain(), KeyframeOutput8Bit())`
- `DecodeSample(configOBUs, sample)` — reuses decoder scratch (`Reset` + warm pools); ~5 allocs/op vs ~64 for one-shot `DecodeIVF` on 1080p in local benches.

## SIMD / builds

- `scripts/go.sh` sets `GOEXPERIMENT=simd` by default (same pattern as go-vpx).
- `scripts/xc.sh` — `CGO_ENABLED=0` cross-build smoke test for shipped GOOS/GOARCH targets.
- Production amd64/arm64 SIMD kernels remain on existing `goexperiment.simd` + `archsimd` dispatch in-tree; this PR does not land new AVX-512 tiers (follow-up against profile hotspots).

## Encoder / RTP

Not removed: root package still ships WebRTC/RTP/encoder APIs for upstream parity. Decoder-only consumers can import `container` + keyframe APIs without using those symbols; a future PR can split modules or gate encoder packages.

## Benchmarks (linux/amd64, GOEXPERIMENT=simd, GOMAXPROCS=1, this CI VM)

| Case | One-shot `DecodeIVF` | `KeyframeDecoder` (reuse) |
|------|----------------------|---------------------------|
| 1080p 8-bit keyframe | ~20 ms/op, ~74 MB/op, 64 allocs | ~14.3 ms/op, 4 KB/op, 5 allocs |
| 1080p 10-bit → 8-bit | — | ~24 ms/op |
| 4K 8-bit keyframe | — | ~39 ms/op |

Cross-compile (`scripts/xc.sh`): all listed targets `ok`.
