#!/usr/bin/env sh
set -eu

base_url="${LIBAOM_TEST_DATA_URL:-https://storage.googleapis.com/aom-test-data}"
fixture_dir="${LIBAOM_TEST_DATA_DIR:-internal/av1/testdata/libaom}"

mkdir -p "$fixture_dir"

# IVF files required by go test ./... (also committed under internal/av1/testdata/libaom).
for name in \
	av1-1-b8-00-quantizer-00.ivf \
	av1-1-b8-00-quantizer-00.ivf.md5 \
	av1-1-b8-01-size-226x226.ivf \
	av1-1-b8-04-cdfupdate.ivf \
	av1-1-b8-05-mv.ivf \
	av1-1-b8-06-mfmv.ivf \
	av1-1-b8-16-intra_only-intrabc-extreme-dv.ivf \
	av1-1-b8-22-svc-L1T2.ivf \
	av1-1-b8-22-svc-L2T1.ivf \
	av1-1-b8-22-svc-L2T2.ivf \
	av1-1-b8-23-film_grain-50.ivf \
	av1-1-b10-00-quantizer-00.ivf \
	av1-1-b10-00-quantizer-10.ivf \
	av1-1-b10-24-monochrome.ivf
do
	path="$fixture_dir/$name"
	if [ ! -f "$path" ]; then
		tmp="$path.tmp"
		curl -fsSL -o "$tmp" "$base_url/$name"
		mv "$tmp" "$path"
	fi
done
