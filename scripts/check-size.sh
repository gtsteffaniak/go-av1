#!/usr/bin/env bash
#
# check-size.sh reports the linked size of a decode-only consumer and fails if
# encoder (or other top-of-graph) code leaks into it.
#
# It builds tools/sizeprobe three ways:
#   1. default (pure-Go, unstripped)
#   2. stripped            (-ldflags="-s -w")
#   3. SIMD                (GOEXPERIMENT=simd)
#
# and, for the decoder-only closure, asserts that no internal/av1/encoder or
# pkg/encode symbols appear. The SIMD build is reported for contrast only.
#
# Usage: scripts/check-size.sh [probe.obu]
#   The probe argument is used for an optional run through the binary; it
#   defaults to the first vendored profile clip payload.
#
# Exit status is non-zero if encoder symbols leak into the decode-only build.

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

out_dir="$(mktemp -d)"
trap 'rm -rf "$out_dir"' EXIT

echo "== decode-only closure: forbidden symbols =="
# go list -deps is the linker's view; check the decode path directly.
forbidden='internal/av1/encoder|pkg/encode'
if deps="$(go list -deps ./pkg/decode 2>/dev/null | grep -E "$forbidden" || true)"; then
	if [[ -n "$deps" ]]; then
		echo "FAIL: pkg/decode depends on encoder packages:" >&2
		echo "$deps" >&2
		exit 1
	fi
fi
echo "ok: pkg/decode closure excludes $forbidden"

report_size() {
	local label="$1" bin="$2"
	local size
	size="$(stat -c '%s' "$bin" 2>/dev/null || stat -f '%z' "$bin")"
	printf '%-22s %10s bytes  (%s MB)\n' "$label" "$size" "$(awk -v b="$size" 'BEGIN{printf "%.2f", b/1048576}')"
}

echo
echo "== linked sizes (decode-only probe) =="
default_bin="$out_dir/default"
go build -o "$default_bin" ./tools/sizeprobe
report_size "default" "$default_bin"

stripped_bin="$out_dir/stripped"
go build -ldflags="-s -w" -o "$stripped_bin" ./tools/sizeprobe
report_size "stripped (-s -w)" "$stripped_bin"

if GOEXPERIMENT=simd go build -o "$out_dir/simd" ./tools/sizeprobe 2>/dev/null; then
	report_size "simd (GOEXPERIMENT)" "$out_dir/simd"
else
	echo "simd (GOEXPERIMENT)    unavailable on this toolchain"
fi

echo
echo "== symbol audit (default, unstripped) =="
# Guard against accidental encoder linkage at the symbol level too.
if go tool nm -size "$default_bin" 2>/dev/null | grep -E 'internal/av1/encoder|/pkg/encode\.' | head -20 | grep -q .; then
	echo "FAIL: encoder symbols present in decode-only binary:" >&2
	go tool nm -size "$default_bin" | grep -E 'internal/av1/encoder|/pkg/encode\.' | head -20 >&2
	exit 1
fi
echo "ok: no encoder symbols in decode-only binary"

if [[ $# -ge 1 ]]; then
	echo
	echo "== probe run =="
	"$default_bin" "$1"
fi

echo
echo "check-size: PASS"