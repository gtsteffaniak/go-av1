package container

import (
	"fmt"

	"github.com/gtsteffaniak/go-av1/pkg/container/isobmff"
)

// MP4Sample is one AV1 sample from an MP4/MOV track (typically one temporal unit).
type MP4Sample struct {
	Index  int
	Offset uint64
	Size   uint32
	IsSync bool
	Data   []byte
}

// MP4Track holds av1C and samples from the first AV1 (av01) video track.
type MP4Track struct {
	Av1C      Av1C
	HasAv1C   bool
	Samples   []MP4Sample
	Timescale uint32
}

// ParseMP4 parses an MP4/MOV file with an AV1 track (handler type vide, sample entry av01).
func ParseMP4(data []byte) (MP4Track, error) {
	ct, err := isobmff.ParseContainer(data)
	if err != nil {
		return MP4Track{}, err
	}
	if ct.Moov == nil {
		return MP4Track{}, fmt.Errorf("container: mp4 missing moov")
	}
	stbl := ct.Moov.ImageTrackStbl()
	if stbl == nil {
		return MP4Track{}, fmt.Errorf("container: mp4 missing AV1 video stbl")
	}
	av1c, ok := mp4TrackAv1C(stbl)
	out := MP4Track{}
	if ts, ok := ct.Moov.AV1TrackMediaTimescale(); ok {
		out.Timescale = ts
	}
	if ok {
		p, err := ParseAv1C(av1c.ConfigOBUs)
		if err != nil {
			return MP4Track{}, err
		}
		p.ConfigOBUs = append([]byte(nil), av1c.ConfigOBUs...)
		out.Av1C = p
		out.HasAv1C = true
	}
	table, err := stbl.SampleTable()
	if err != nil {
		return MP4Track{}, err
	}
	if len(ct.Mdats) == 0 && ct.Mdat == nil {
		return MP4Track{}, fmt.Errorf("container: mp4 missing mdat")
	}
	out.Samples = make([]MP4Sample, 0, len(table))
	for i, s := range table {
		if s.Size == 0 {
			return MP4Track{}, fmt.Errorf("container: sample %d empty", i)
		}
		if _, err := checkedSampleEnd(s.Offset, uint64(s.Size)); err != nil {
			return MP4Track{}, fmt.Errorf("container: sample %d: %w", i, err)
		}
		chunk, err := ct.ReadPayloadAt(s.Offset, uint64(s.Size))
		if err != nil {
			return MP4Track{}, fmt.Errorf("container: sample %d: %w", i, err)
		}
		out.Samples = append(out.Samples, MP4Sample{
			Index:  i,
			Offset: s.Offset,
			Size:   s.Size,
			IsSync: s.IsSync,
			Data:   append([]byte(nil), chunk...),
		})
	}
	return out, nil
}

func checkedSampleEnd(offset, size uint64) (uint64, error) {
	if size > ^uint64(0)-offset {
		return 0, fmt.Errorf("sample size overflow")
	}
	return offset + size, nil
}

// TemporalUnitFromMP4Sample builds a low-overhead temporal unit for one MP4 sample.
func TemporalUnitFromMP4Sample(track MP4Track, sampleIndex int) ([]byte, error) {
	if sampleIndex < 0 || sampleIndex >= len(track.Samples) {
		return nil, fmt.Errorf("container: sample index %d out of range", sampleIndex)
	}
	var config []byte
	if track.HasAv1C {
		config = track.Av1C.ConfigOBUs
	}
	return BuildTemporalUnit(config, track.Samples[sampleIndex].Data)
}

func mp4TrackAv1C(stbl *isobmff.Stbl) (*isobmff.Av1C, bool) {
	for _, ch := range stbl.Children {
		rb, ok := ch.(*isobmff.RawBox)
		if !ok || rb.Type != isobmff.TypeStsd {
			continue
		}
		payload := rb.Payload
		if len(payload) < 8 {
			continue
		}
		entryCount := uint32(payload[4])<<24 | uint32(payload[5])<<16 | uint32(payload[6])<<8 | uint32(payload[7])
		if entryCount == 0 {
			continue
		}
		rest := payload[8:]
		entries, err := isobmff.ReadChildren(rest)
		if err != nil {
			continue
		}
		for _, ent := range entries {
			er, ok := ent.(*isobmff.RawBox)
			if !ok || !er.Type.Equal("av01") {
				continue
			}
			inner, err := isobmff.ReadChildren(er.Payload[8:])
			if err != nil {
				continue
			}
			for _, c := range inner {
				if c2, ok := c.(*isobmff.RawBox); ok && c2.Type == isobmff.TypeAv1C {
					av, err := isobmff.ParseAv1C(c2.Payload)
					if err != nil {
						continue
					}
					return av, true
				}
				if av, ok := c.(*isobmff.Av1C); ok {
					return av, true
				}
			}
		}
	}
	return nil, false
}
