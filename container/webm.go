package container

import (
	"fmt"
)

// ParseWebM parses a WebM/MKV file with a V_AV1 track.
func ParseWebM(data []byte) (WebMTrack, error) {
	var codecPrivate []byte
	var blocks [][]byte
	var walk func([]byte)
	walk = func(buf []byte) {
		off := 0
		for off < len(buf) {
			id, sz, n, err := readEBMLElementHeader(buf[off:])
			if err != nil {
				return
			}
			off += n
			end := off + int(sz)
			if sz == ebmlUnknownSize || end > len(buf) {
				end = len(buf)
			}
			body := buf[off:end]
			switch id {
			case 0x1654AE6B: // Tracks
				if cp := tracksCodecPrivate(body); len(cp) > 0 {
					codecPrivate = cp
				}
			case 0xA3, 0xA1: // SimpleBlock / Block
				if block, ok := parseSimpleBlockPayload(body); ok {
					blocks = append(blocks, block)
				}
			default:
				// Master elements: recurse into children.
				if sz > 0 && isEBMLMaster(id) {
					walk(body)
				}
			}
			off = end
		}
	}
	walk(data)
	out := WebMTrack{}
	if len(codecPrivate) > 0 {
		if len(codecPrivate) >= 4 {
			p, err := ParseAv1C(codecPrivate)
			if err == nil {
				p.ConfigOBUs = append([]byte(nil), codecPrivate[4:]...)
				out.Av1C = p
				out.HasAv1C = true
			}
		}
	}
	for i, b := range blocks {
		out.Samples = append(out.Samples, WebMSample{Index: i, Data: b})
	}
	if len(out.Samples) == 0 {
		return WebMTrack{}, fmt.Errorf("container: webm has no AV1 blocks")
	}
	return out, nil
}

func isEBMLMaster(id uint32) bool {
	switch id {
	case 0x1A45DFA3, 0x18538067, 0x1F43B675, 0x1654AE6B, 0xAE, 0xBF:
		return true
	default:
		return false
	}
}

func tracksCodecPrivate(tracks []byte) []byte {
	off := 0
	for off < len(tracks) {
		id, sz, n, err := readEBMLElementHeader(tracks[off:])
		if err != nil {
			return nil
		}
		off += n
		end := off + int(sz)
		if id == 0xAE {
			if cp := findTrackCodecPrivate(tracks[off:end]); len(cp) > 0 {
				return cp
			}
		}
		off = end
	}
	return nil
}

// WebMSample is one AV1 frame from a Matroska/WebM SimpleBlock.
type WebMSample struct {
	Index int
	Data  []byte
}

// WebMTrack holds CodecPrivate (av1C-like config) and samples from a V_AV1 track.
type WebMTrack struct {
	Av1C    Av1C
	HasAv1C bool
	Samples []WebMSample
}

// TemporalUnitFromWebMSample builds a temporal unit for one WebM block payload.
func TemporalUnitFromWebMSample(track WebMTrack, sampleIndex int) ([]byte, error) {
	if sampleIndex < 0 || sampleIndex >= len(track.Samples) {
		return nil, fmt.Errorf("container: webm sample index %d out of range", sampleIndex)
	}
	var config []byte
	if track.HasAv1C {
		config = track.Av1C.ConfigOBUs
	}
	return BuildTemporalUnit(config, track.Samples[sampleIndex].Data)
}

func findTrackCodecPrivate(entry []byte) []byte {
	off := 0
	var codecID []byte
	var codecPrivate []byte
	for off < len(entry) {
		id, sz, n, err := readEBMLElementHeader(entry[off:])
		if err != nil {
			return nil
		}
		off += n
		end := off + int(sz)
		switch id {
		case 0x86:
			codecID = append([]byte(nil), entry[off:end]...)
		case 0x63A2:
			codecPrivate = append([]byte(nil), entry[off:end]...)
		}
		off = end
	}
	if string(codecID) == "V_AV1" {
		return codecPrivate
	}
	return nil
}

func parseSimpleBlockPayload(block []byte) ([]byte, bool) {
	if len(block) < 4 {
		return nil, false
	}
	trackLen := ebmlVIntLen(block[0])
	if trackLen == 0 || len(block) < trackLen+3 {
		return nil, false
	}
	payload := block[trackLen+2:]
	return append([]byte(nil), payload...), true
}

func readEBMLElementHeader(src []byte) (id uint32, size uint64, n int, err error) {
	if len(src) == 0 {
		return 0, 0, 0, fmt.Errorf("container: ebml eof")
	}
	idLen := ebmlVIntLen(src[0])
	if idLen == 0 || len(src) < idLen {
		return 0, 0, 0, fmt.Errorf("container: bad ebml id")
	}
	for i := 0; i < idLen; i++ {
		id = (id << 8) | uint32(src[i])
	}
	n = idLen
	size, sizeLen, err := decodeEBMLSize(src[n:])
	if err != nil {
		return 0, 0, 0, err
	}
	n += sizeLen
	return id, size, n, nil
}

func ebmlVIntLen(b byte) int {
	switch {
	case b&0x80 != 0:
		return 1
	case b&0x40 != 0:
		return 2
	case b&0x20 != 0:
		return 3
	case b&0x10 != 0:
		return 4
	default:
		return 0
	}
}

func decodeEBMLVInt(src []byte) uint64 {
	n := ebmlVIntLen(src[0])
	if n == 0 || len(src) < n {
		return 0
	}
	mask := byte(0xFF) >> n
	v := uint64(src[0] & mask)
	for i := 1; i < n; i++ {
		v = (v << 8) | uint64(src[i])
	}
	return v
}

const ebmlUnknownSize = ^uint64(0)

func ebmlSizeLen(b byte) int {
	if b == 0 {
		return 0
	}
	leading := 0
	for mask := byte(0x80); mask != 0 && b&mask == 0; mask >>= 1 {
		leading++
	}
	return 1 + leading
}

func decodeEBMLSize(src []byte) (uint64, int, error) {
	if len(src) == 0 {
		return 0, 0, fmt.Errorf("container: ebml size eof")
	}
	n := ebmlSizeLen(src[0])
	if n == 0 || len(src) < n {
		return 0, 0, fmt.Errorf("container: bad ebml size")
	}
	mask := byte(0xFF) >> n
	v := uint64(src[0] & mask)
	for i := 1; i < n; i++ {
		v = (v << 8) | uint64(src[i])
	}
	maxVal := uint64(1)<<(7*uint(n)) - 1
	if v == maxVal {
		return ebmlUnknownSize, n, nil
	}
	return v, n, nil
}
