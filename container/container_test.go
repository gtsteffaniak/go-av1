package container_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gtsteffaniak/go-av1/container"
)

func TestParseAVIFAndDecodeTemporalUnit(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "i8_1080.avif"))
	if err != nil {
		t.Fatal(err)
	}
	primary, err := container.ParseAVIF(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(primary.Sample) == 0 {
		t.Fatal("empty sample")
	}
	unit, err := container.TemporalUnitFromAVIF(primary)
	if err != nil {
		t.Fatal(err)
	}
	if len(unit) < 8 {
		t.Fatalf("temporal unit too short: %d", len(unit))
	}
}

func TestParseMP4FirstSample(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "s8_1080.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	track, err := container.ParseMP4(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(track.Samples) == 0 {
		t.Fatal("no samples")
	}
	unit, err := container.TemporalUnitFromMP4Sample(track, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(unit) == 0 {
		t.Fatal("empty unit")
	}
}

func TestParseWebM(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "s8_4k.webm"))
	if err != nil {
		t.Fatal(err)
	}
	track, err := container.ParseWebM(data)
	if err != nil {
		t.Fatal(err)
	}
	unit, err := container.TemporalUnitFromWebMSample(track, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(unit) == 0 {
		t.Fatal("empty unit")
	}
}

func TestBuildTemporalUnitWithoutConfig(t *testing.T) {
	primary, err := container.ParseAVIF(mustRead(t, filepath.Join("testdata", "i8_1080.avif")))
	if err != nil {
		t.Fatal(err)
	}
	_, err = container.BuildTemporalUnit(nil, primary.Sample)
	if err != nil {
		t.Fatal(err)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
