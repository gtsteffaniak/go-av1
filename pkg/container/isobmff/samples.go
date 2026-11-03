package isobmff

import "fmt"

// Sample describes one encoded image frame within an AVIF image
// sequence. Offset is absolute within the source file; Size is the
// frame's byte length; Duration is its presentation time delta in
// the movie's timescale (see Mvhd.Timescale); IsSync marks sync
// samples (keyframes that can be decoded standalone). When stss is
// absent every sample is a sync sample.
type Sample struct {
	Offset   uint64
	Size     uint32
	Duration uint32
	IsSync   bool
}

// SampleTable walks the stts / stsc / stsz / stco+co64 trio under an
// Stbl and returns a flat list of per-sample (offset, size, duration)
// records. Only one track's worth of samples is resolved — AVIF
// sequences contain a single image track.
//
// Returns an error when the expected boxes are missing or malformed.
func (s *Stbl) SampleTable() ([]Sample, error) {
	var (
		stts *Stts
		stsc *Stsc
		stsz *Stsz
		stco *Stco
		co64 *Co64
		stss *Stss
	)
	for _, ch := range s.Children {
		switch b := ch.(type) {
		case *Stts:
			stts = b
		case *Stsc:
			stsc = b
		case *Stsz:
			stsz = b
		case *Stco:
			stco = b
		case *Co64:
			co64 = b
		case *Stss:
			stss = b
		}
	}
	if stts == nil {
		return nil, fmt.Errorf("%w: stbl missing stts", ErrInvalid)
	}
	if stsc == nil {
		return nil, fmt.Errorf("%w: stbl missing stsc", ErrInvalid)
	}
	if stsz == nil {
		return nil, fmt.Errorf("%w: stbl missing stsz", ErrInvalid)
	}
	chunkCount := 0
	chunkOffset := func(i int) uint64 {
		if stco != nil {
			return uint64(stco.Offsets[i])
		}
		return co64.Offsets[i]
	}
	switch {
	case stco != nil:
		chunkCount = len(stco.Offsets)
	case co64 != nil:
		chunkCount = len(co64.Offsets)
	default:
		return nil, fmt.Errorf("%w: stbl missing stco/co64", ErrInvalid)
	}

	// Expand stsc into a per-chunk samples-per-chunk slice.
	perChunk := make([]uint32, chunkCount)
	descIdx := make([]uint32, chunkCount)
	for i, e := range stsc.Entries {
		start := int(e.FirstChunk) - 1 // 1-based in the box, 0-based here
		end := chunkCount
		if i+1 < len(stsc.Entries) {
			end = int(stsc.Entries[i+1].FirstChunk) - 1
		}
		if start < 0 || end > chunkCount || start > end {
			return nil, fmt.Errorf("%w: stsc entry %d out of range", ErrInvalid, i)
		}
		for c := start; c < end; c++ {
			perChunk[c] = e.SamplesPerChunk
			descIdx[c] = e.DescriptionIdx
		}
	}

	deltas := stts.SampleDeltas()
	var out []Sample
	sampleIdx := uint32(0) // 0-based
	for c := 0; c < chunkCount; c++ {
		off := chunkOffset(c)
		for i := uint32(0); i < perChunk[c]; i++ {
			size := stsz.SizeOf(sampleIdx + 1)
			var dur uint32
			if int(sampleIdx) < len(deltas) {
				dur = deltas[sampleIdx]
			}
			out = append(out, Sample{
				Offset:   off,
				Size:     size,
				Duration: dur,
				IsSync:   stss.IsSync(sampleIdx + 1),
			})
			off += uint64(size)
			sampleIdx++
		}
	}
	if sampleIdx != stsz.SampleCount {
		return nil, fmt.Errorf("%w: stsc produced %d samples, stsz declares %d", ErrInvalid, sampleIdx, stsz.SampleCount)
	}
	return out, nil
}

// trakAV1VideoStbl returns the stbl for the first track whose handler is
// vide/pict and whose sample description includes an av01 entry.
func trakAV1VideoStbl(trak *Trak) *Stbl {
	if trak == nil {
		return nil
	}
	var hdlr *Hdlr
	var stbl *Stbl
	for _, ch := range trak.Children {
		mdia, ok := ch.(*Mdia)
		if !ok {
			continue
		}
		for _, mc := range mdia.Children {
			switch b := mc.(type) {
			case *Hdlr:
				hdlr = b
			case *Minf:
				for _, mic := range b.Children {
					if s, ok := mic.(*Stbl); ok {
						stbl = s
					}
				}
			}
		}
	}
	if stbl == nil || hdlr == nil {
		return nil
	}
	switch hdlr.HandlerType {
	case FourCCOf("vide"), FourCCOf("pict"):
	default:
		return nil
	}
	if !stblHasAv01SampleEntry(stbl) {
		return nil
	}
	return stbl
}

func stblHasAv01SampleEntry(stbl *Stbl) bool {
	for _, ch := range stbl.Children {
		rb, ok := ch.(*RawBox)
		if !ok || rb.Type != TypeStsd {
			continue
		}
		payload := rb.Payload
		if len(payload) < 8 {
			continue
		}
		entries, err := ReadChildren(payload[8:])
		if err != nil {
			continue
		}
		for _, ent := range entries {
			er, ok := ent.(*RawBox)
			if ok && er.Type.Equal("av01") {
				return true
			}
		}
	}
	return false
}

// ImageTrackStbl returns the Stbl of the first AV1 video/image track in
// the Moov, or nil if not present.
func (m *Moov) ImageTrackStbl() *Stbl {
	for _, ch := range m.Children {
		if trak, ok := ch.(*Trak); ok {
			if stbl := trakAV1VideoStbl(trak); stbl != nil {
				return stbl
			}
		}
	}
	return nil
}

// AV1TrackMediaTimescale returns the mdhd timescale for the first AV1 track.
func (m *Moov) AV1TrackMediaTimescale() (uint32, bool) {
	for _, ch := range m.Children {
		trak, ok := ch.(*Trak)
		if !ok || trakAV1VideoStbl(trak) == nil {
			continue
		}
		if ts, ok := trakMediaTimescale(trak); ok {
			return ts, true
		}
	}
	return 0, false
}

func trakMediaTimescale(trak *Trak) (uint32, bool) {
	for _, ch := range trak.Children {
		mdia, ok := ch.(*Mdia)
		if !ok {
			continue
		}
		for _, mc := range mdia.Children {
			if mdhd, ok := mc.(*Mdhd); ok {
				return mdhd.Timescale, true
			}
			if rb, ok := mc.(*RawBox); ok && rb.Type == TypeMdhd {
				mdhd, err := ParseMdhd(rb.Payload)
				if err != nil {
					return 0, false
				}
				return mdhd.Timescale, true
			}
		}
	}
	return 0, false
}

// Mdhd is the media header box (§8.4.2).
type Mdhd struct {
	FullBoxHeader
	Timescale uint32
	Duration  uint32
	Language  uint16 // ISO-639-2/T packed per §8.4.2
}

func (*Mdhd) BoxType() FourCC { return TypeMdhd }

func (m *Mdhd) MarshalPayload() ([]byte, error) {
	b := newBuilder()
	fbh := m.FullBoxHeader
	fbh.Version = 0
	b.buf = appendFullBoxHeader(b.buf, fbh)
	b.writeU32(0) // creation_time
	b.writeU32(0) // modification_time
	b.writeU32(m.Timescale)
	b.writeU32(m.Duration)
	lang := m.Language
	if lang == 0 {
		lang = 0x55C4 // und
	}
	b.writeU16(lang)
	b.writeU16(0) // pre_defined
	return b.bytes(), nil
}

func ParseMdhd(payload []byte) (*Mdhd, error) {
	fbh, rest, err := readFullBoxHeader(payload)
	if err != nil {
		return nil, err
	}
	c := newCursor(rest)
	m := &Mdhd{FullBoxHeader: fbh}
	if fbh.Version == 1 {
		_ = c.readU64() // creation
		_ = c.readU64() // modification
		m.Timescale = c.readU32()
		m.Duration = uint32(c.readU64())
	} else {
		_ = c.readU32()
		_ = c.readU32()
		m.Timescale = c.readU32()
		m.Duration = c.readU32()
	}
	m.Language = c.readU16()
	_ = c.readU16()
	if c.err != nil {
		return nil, c.err
	}
	return m, nil
}
