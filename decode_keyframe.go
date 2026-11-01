package goav1

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/gtsteffaniak/go-av1/container"
)

// KeyframeDecoder decodes standalone AV1 keyframes (still images and sync
// samples) while reusing scratch across calls. Construct it once from a
// representative temporal unit that sizes the largest expected frame, then
// call DecodeSample for each thumbnail.
type KeyframeDecoder struct {
	dec      *Decoder
	unitBuf  []byte
	payloads [][]byte
	out8     bool
}

// KeyframeOption configures a KeyframeDecoder.
type KeyframeOption func(*keyframeConfig)

type keyframeConfig struct {
	skipFilmGrain bool
	output8Bit    bool
}

// KeyframeSkipFilmGrain disables film-grain synthesis (recommended for thumbnails).
func KeyframeSkipFilmGrain() KeyframeOption {
	return func(c *keyframeConfig) { c.skipFilmGrain = true }
}

// KeyframeOutput8Bit down-converts decoded luma/chroma to 8-bit 4:2:0-style
// planes in KeyframeImage (10/12-bit sources are rounded and clipped).
func KeyframeOutput8Bit() KeyframeOption {
	return func(c *keyframeConfig) { c.output8Bit = true }
}

// NewKeyframeDecoder sizes decode scratch from probe, a low-overhead temporal
// unit that must decode successfully and be at least as large (resolution and
// payload bytes) as any later sample passed to DecodeSample. probe is retained
// by reference; the decoder mutates the underlying bytes on each DecodeSample.
func NewKeyframeDecoder(probe []byte, opts ...KeyframeOption) (*KeyframeDecoder, error) {
	if len(probe) == 0 {
		return nil, errors.New("goav1: empty keyframe probe")
	}
	var cfg keyframeConfig
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	decOpts := []Option{WithWorkers(1), WithMaxFrameDelay(1)}
	if cfg.skipFilmGrain {
		decOpts = append(decOpts, WithSkipFilmGrain())
	}
	unitBuf := append([]byte(nil), probe...)
	payloads := [][]byte{unitBuf}
	dec, err := NewDecoder(payloads, decOpts...)
	if err != nil {
		return nil, err
	}
	// Warm the runner once so the first DecodeSample matches steady-state cost.
	if err := dec.Reset(); err != nil {
		dec.Close()
		return nil, err
	}
	if _, ok, err := dec.DecodeNext(); err != nil || !ok {
		dec.Close()
		if err != nil {
			return nil, fmt.Errorf("goav1: keyframe probe decode: %w", err)
		}
		return nil, errors.New("goav1: keyframe probe produced no frame")
	}
	if err := dec.Reset(); err != nil {
		dec.Close()
		return nil, err
	}
	return &KeyframeDecoder{
		dec:      dec,
		unitBuf:  unitBuf,
		payloads: payloads,
		out8:     cfg.output8Bit,
	}, nil
}

// KeyframeImage is one decoded keyframe. Plane slices alias decoder-owned
// memory valid until the next DecodeSample or Close.
type KeyframeImage struct {
	Width, Height   int
	ChromaWidth     int
	ChromaHeight    int
	YStride         int
	UVStride        int
	BytesPerSample  int
	Y, U, V         []byte
	NativeChroma420 bool
}

// DecodeSample decodes one keyframe from optional av1C config OBUs and sample
// bytes (MP4/MKV/AVIF sample payload). configOBUs may be nil when the sequence
// header is carried in sample (ffmpeg AVIF).
func (k *KeyframeDecoder) DecodeSample(configOBUs []byte, sample []byte) (KeyframeImage, error) {
	if k == nil || k.dec == nil {
		return KeyframeImage{}, errors.New("goav1: nil keyframe decoder")
	}
	unit, err := container.BuildTemporalUnit(configOBUs, sample)
	if err != nil {
		return KeyframeImage{}, err
	}
	if len(unit) > cap(k.unitBuf) {
		return KeyframeImage{}, fmt.Errorf("goav1: sample temporal unit %d bytes exceeds probe capacity %d", len(unit), cap(k.unitBuf))
	}
	k.unitBuf = k.unitBuf[:len(unit)]
	copy(k.unitBuf, unit)
	k.payloads[0] = k.unitBuf
	if err := k.dec.Reset(); err != nil {
		return KeyframeImage{}, err
	}
	frames, ok, err := k.dec.DecodeNext()
	if err != nil {
		return KeyframeImage{}, err
	}
	if !ok || len(frames) == 0 {
		return KeyframeImage{}, errors.New("goav1: keyframe decode produced no frame")
	}
	return keyframeImageFromFrame(frames[0], k.out8), nil
}

// DecodeTemporalUnit decodes one low-overhead temporal unit produced by
// container.BuildTemporalUnit or an IVF frame packet.
func (k *KeyframeDecoder) DecodeTemporalUnit(unit []byte) (KeyframeImage, error) {
	if k == nil || k.dec == nil {
		return KeyframeImage{}, errors.New("goav1: nil keyframe decoder")
	}
	if len(unit) > cap(k.unitBuf) {
		return KeyframeImage{}, fmt.Errorf("goav1: temporal unit %d bytes exceeds probe capacity %d", len(unit), cap(k.unitBuf))
	}
	k.unitBuf = k.unitBuf[:len(unit)]
	copy(k.unitBuf, unit)
	k.payloads[0] = k.unitBuf
	if err := k.dec.Reset(); err != nil {
		return KeyframeImage{}, err
	}
	frames, ok, err := k.dec.DecodeNext()
	if err != nil {
		return KeyframeImage{}, err
	}
	if !ok || len(frames) == 0 {
		return KeyframeImage{}, errors.New("goav1: keyframe decode produced no frame")
	}
	return keyframeImageFromFrame(frames[0], k.out8), nil
}

// Close releases worker goroutines held by the underlying decoder.
func (k *KeyframeDecoder) Close() {
	if k == nil || k.dec == nil {
		return
	}
	k.dec.Close()
	k.dec = nil
}

func keyframeImageFromFrame(f *Frame, out8 bool) KeyframeImage {
	if f == nil {
		return KeyframeImage{}
	}
	bps := f.Layout.BytesPerSample
	img := KeyframeImage{
		Width:           f.Y.Width,
		Height:          f.Y.Height,
		ChromaWidth:     f.U.Width,
		ChromaHeight:    f.U.Height,
		YStride:         f.Y.Stride,
		UVStride:        f.U.Stride,
		BytesPerSample:  bps,
		NativeChroma420: bps == 1 && f.U.Width > 0,
	}
	if out8 && bps > 1 {
		img.BytesPerSample = 1
		img.YStride = f.Y.Width
		if f.U.Width > 0 {
			img.UVStride = f.U.Width
		}
		img.Y = downconvertPlane8(f.Y, bps)
		img.U = downconvertPlane8(f.U, bps)
		img.V = downconvertPlane8(f.V, bps)
		return img
	}
	img.Y = f.Y.Pix
	if f.U.Width > 0 {
		img.U = f.U.Pix
		img.V = f.V.Pix
	}
	return img
}

func downconvertPlane8(p FramePlane, bps int) []byte {
	if p.Width == 0 || p.Height == 0 {
		return nil
	}
	out := make([]byte, p.Width*p.Height)
	o := 0
	for row := 0; row < p.Height; row++ {
		off := row * p.Stride
		for col := 0; col < p.Width; col++ {
			sampleOff := off + col*bps
			var v int
			switch bps {
			case 2:
				v = int(binary.LittleEndian.Uint16(p.Pix[sampleOff:]))
				v = (v + 2) >> 2
			default:
				v = int(p.Pix[sampleOff]) << (8 - bps*8)
			}
			if v < 0 {
				v = 0
			} else if v > 255 {
				v = 255
			}
			out[o] = byte(v)
			o++
		}
	}
	return out
}
