package decode_test

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/gtsteffaniak/go-av1/pkg/container"
	decode "github.com/gtsteffaniak/go-av1/pkg/decode"
)

func ivfFirstFrame(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	n := binary.LittleEndian.Uint32(data[32:])
	return append([]byte(nil), data[44:44+n]...)
}

func TestKeyframeDecoderIVF(t *testing.T) {
	probe := ivfFirstFrame(t, filepath.Join("..", "..", "testdata", "thumbnailbench", "a8_1080.ivf"))
	dec, err := decode.NewKeyframeDecoder(probe, decode.KeyframeSkipFilmGrain(), decode.KeyframeOutput8Bit())
	if err != nil {
		t.Fatal(err)
	}
	defer dec.Close()
	img, err := dec.DecodeSample(nil, probe)
	if err != nil {
		t.Fatal(err)
	}
	if img.Width != 1920 || img.Height != 1080 || len(img.Y) == 0 {
		t.Fatalf("unexpected image %+v", img)
	}
}

func TestKeyframeDecoderAVIF(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "container", "testdata", "i8_1080.avif"))
	if err != nil {
		t.Fatal(err)
	}
	primary, err := container.ParseAVIF(data)
	if err != nil {
		t.Fatal(err)
	}
	probe, err := container.TemporalUnitFromAVIF(primary)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := decode.NewKeyframeDecoder(probe, decode.KeyframeSkipFilmGrain())
	if err != nil {
		t.Fatal(err)
	}
	defer dec.Close()
	var config []byte
	if primary.HasAv1C {
		config = primary.Av1C.ConfigOBUs
	}
	img, err := dec.DecodeSample(config, primary.Sample)
	if err != nil {
		t.Fatal(err)
	}
	if img.Width != 1920 || img.Height != 1080 {
		t.Fatalf("size %dx%d", img.Width, img.Height)
	}
}
