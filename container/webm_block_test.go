package container_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gtsteffaniak/go-av1/container"
)

func TestMatroskaBlockFramesWebMFixture(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "s8_4k.webm"))
	if err != nil {
		t.Fatal(err)
	}
	track, err := container.ParseWebM(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(track.Samples) == 0 {
		t.Fatal("no samples")
	}
	unit, err := container.TemporalUnitFromWebMSample(track, 0)
	if err != nil {
		t.Fatalf("TemporalUnitFromWebMSample: %v", err)
	}
	if len(unit) < 4 {
		t.Fatalf("unit len %d", len(unit))
	}
	if unit[0]&0x80 != 0 {
		t.Fatalf("forbidden obu bit set: %02x", unit[0])
	}
}
