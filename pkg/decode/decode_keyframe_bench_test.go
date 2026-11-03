package decode_test

import (
	"encoding/binary"
	decode "github.com/gtsteffaniak/go-av1/pkg/decode"
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkKeyframeDecoder1080p8(b *testing.B) {
	benchKeyframeIVF(b, "a8_1080.ivf", decode.KeyframeSkipFilmGrain())
}

func BenchmarkKeyframeDecoder1080p10(b *testing.B) {
	benchKeyframeIVF(b, "a10_1080.ivf", decode.KeyframeSkipFilmGrain(), decode.KeyframeOutput8Bit())
}

func BenchmarkKeyframeDecoder4K8(b *testing.B) {
	benchKeyframeIVF(b, "s8_4k.ivf", decode.KeyframeSkipFilmGrain())
}

func benchKeyframeIVF(b *testing.B, name string, opts ...decode.KeyframeOption) {
	path := filepath.Join("..", "..", "testdata", "thumbnailbench", name)
	data, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	n := binary.LittleEndian.Uint32(data[32:])
	probe := data[44 : 44+n]
	dec, err := decode.NewKeyframeDecoder(probe, opts...)
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
	path := filepath.Join("..", "..", "testdata", "thumbnailbench", "a8_1080.ivf")
	data, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		frames, err := decode.DecodeIVF(data, decode.WithWorkers(1), decode.WithSkipFilmGrain())
		if err != nil || len(frames) == 0 {
			b.Fatal(err)
		}
	}
}
