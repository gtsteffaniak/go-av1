// Command sizeprobe is a minimal decode-only consumer used by
// scripts/check-size.sh to measure what a thumbnail/keyframe decode integration
// actually links, and to fail the build if encoder code leaks into it.
//
// It deliberately mirrors the go-media thumbnail path: build a keyframe decoder
// from a probe temporal unit and decode one sample. Nothing here references the
// encoder or the benchmarking tools, so any encoder symbols in the resulting
// binary are accidental linkage.
package main

import (
	"fmt"
	"os"

	"github.com/gtsteffaniak/go-av1/pkg/decode"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: sizeprobe <temporal-unit.obu>")
		os.Exit(2)
	}
	probe, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "read probe:", err)
		os.Exit(1)
	}
	kd, err := decode.NewKeyframeDecoder(probe,
		decode.KeyframeSkipFilmGrain(),
		decode.KeyframeOutput8Bit(),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "new keyframe decoder:", err)
		os.Exit(1)
	}
	defer kd.Close()

	img, err := kd.DecodeSample(nil, probe)
	if err != nil {
		fmt.Fprintln(os.Stderr, "decode sample:", err)
		os.Exit(1)
	}
	fmt.Printf("decoded %dx%d luma bytes=%d\n", img.Width, img.Height, len(img.Y))
}
