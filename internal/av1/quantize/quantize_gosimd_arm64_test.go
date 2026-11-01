//go:build goexperiment.simd && arm64 && !purego

package quantize

import "github.com/gtsteffaniak/go-av1/internal/av1/dsp/cpu"

func quantizeSIMDSupported() bool {
	return cpu.Detected.NEON
}
