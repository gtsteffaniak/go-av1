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
			case 0xA3, 0xA1: // SimpleBlock / Block (inside BlockGroup)
				for _, frame := range matroskaBlockFrames(body) {
					blocks = append(blocks, frame)
				}
			case 0xA0: // BlockGroup — Block element carries frame(s)
				walkBlockGroup(body, func(blockBody []byte) {
					for _, frame := range matroskaBlockFrames(blockBody) {
						blocks = append(blocks, frame)
					}
				})
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

// matroskaBlockFrames splits a SimpleBlock or Block element body into one or
// more AV1 frame payloads (handles Matroska lacing).
func matroskaBlockFrames(block []byte) [][]byte {
	if len(block) < 4 {
		return nil
	}
	trackLen := ebmlVIntLen(block[0])
	if trackLen == 0 || len(block) < trackLen+3 {
		return nil
	}
	flags := block[trackLen+2]
	payload := block[trackLen+3:]
	lace := (flags >> 1) & 0x3
	switch lace {
	case 0:
		if len(payload) == 0 {
			return nil
		}
		return [][]byte{append([]byte(nil), payload...)}
	case 1:
		return splitXiphLacedFrames(payload)
	case 2:
		return splitFixedSizeLacedFrames(payload)
	case 3:
		return splitEBMLLacedFrames(payload)
	default:
		return nil
	}
}

func walkBlockGroup(group []byte, fn func(blockBody []byte)) {
	off := 0
	for off < len(group) {
		id, sz, n, err := readEBMLElementHeader(group[off:])
		if err != nil {
			return
		}
		off += n
		end := off + int(sz)
		if end > len(group) {
			end = len(group)
		}
		if id == 0xA1 {
			fn(group[off:end])
		}
		off = end
	}
}

func splitXiphLacedFrames(payload []byte) [][]byte {
	if len(payload) == 0 {
		return nil
	}
	off := 0
	readLaceSize := func() (int, bool) {
		size := 0
		start := off
		for off < len(payload) {
			b := payload[off]
			off++
			size += int(b)
			if b != 0xff {
				return size, true
			}
		}
		off = start
		return 0, false
	}
	var sizes []int
	for {
		if off >= len(payload) {
			break
		}
		start := off
		size, ok := readLaceSize()
		if !ok {
			off = start
			break
		}
		if size >= len(payload)-off {
			off = start
			break
		}
		sizes = append(sizes, size)
	}
	if len(sizes) == 0 {
		return [][]byte{append([]byte(nil), payload...)}
	}
	var out [][]byte
	off = 0
	for _, sz := range sizes {
		start := off
		laceSize, ok := readLaceSize()
		if !ok || laceSize != sz {
			return [][]byte{append([]byte(nil), payload...)}
		}
		_ = start
		if off+sz > len(payload) {
			return nil
		}
		out = append(out, append([]byte(nil), payload[off:off+sz]...))
		off += sz
	}
	if off < len(payload) {
		out = append(out, append([]byte(nil), payload[off:]...))
	}
	return out
}

func splitFixedSizeLacedFrames(payload []byte) [][]byte {
	if len(payload) < 1 {
		return nil
	}
	frameSize := int(payload[0]) + 1
	data := payload[1:]
	if frameSize <= 0 || len(data) == 0 {
		return nil
	}
	var out [][]byte
	for len(data) >= frameSize {
		out = append(out, append([]byte(nil), data[:frameSize]...))
		data = data[frameSize:]
	}
	if len(data) > 0 {
		out = append(out, append([]byte(nil), data...))
	}
	return out
}

func splitEBMLLacedFrames(payload []byte) [][]byte {
	if len(payload) == 0 {
		return nil
	}
	var sizes []int
	off := 0
	for off < len(payload) {
		if off >= len(payload) {
			break
		}
		sz, n, err := decodeEBMLSize(payload[off:])
		if err != nil || n == 0 {
			break
		}
		off += n
		sizes = append(sizes, int(sz))
	}
	if len(sizes) == 0 {
		return [][]byte{append([]byte(nil), payload...)}
	}
	var out [][]byte
	for i, size := range sizes {
		if i == len(sizes)-1 {
			out = append(out, append([]byte(nil), payload[off:]...))
			break
		}
		if off+size > len(payload) {
			return nil
		}
		out = append(out, append([]byte(nil), payload[off:off+size]...))
		off += size
	}
	return out
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
