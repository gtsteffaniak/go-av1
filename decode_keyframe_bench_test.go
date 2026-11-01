package goav1_test

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	goav1 "github.com/thesyncim/goav1"
)

func BenchmarkKeyframeDecoder1080p8(b *testing.B) {
	benchKeyframeIVF(b, "a8_1080.ivf", goav1.KeyframeSkipFilmGrain())
}

func BenchmarkKeyframeDecoder1080p10(b *testing.B) {
	benchKeyframeIVF(b, "a10_1080.ivf", goav1.KeyframeSkipFilmGrain(), goav1.KeyframeOutput8Bit())
}

func BenchmarkKeyframeDecoder4K8(b *testing.B) {
	benchKeyframeIVF(b, "s8_4k.ivf", goav1.KeyframeSkipFilmGrain())
}

func benchKeyframeIVF(b *testing.B, name string, opts ...goav1.KeyframeOption) {
	path := filepath.Join("testdata", "thumbnailbench", name)
	data, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	n := binary.LittleEndian.Uint32(data[32:])
	probe := data[44 : 44+n]
	dec, err := goav1.NewKeyframeDecoder(probe, opts...)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(dec.Close)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := dec.DecodeSample(nil, probe); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeIVF1080p8Fresh(b *testing.B) {
	path := filepath.Join("testdata", "thumbnailbench", "a8_1080.ivf")
	data, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		frames, err := goav1.DecodeIVF(data, goav1.WithWorkers(1), goav1.WithSkipFilmGrain())
		if err != nil || len(frames) == 0 {
			b.Fatal(err)
		}
	}
}
