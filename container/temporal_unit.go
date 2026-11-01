package container

import (
	"errors"
	"fmt"

	internalencoder "github.com/thesyncim/goav1/internal/av1/encoder"
	internalobu "github.com/thesyncim/goav1/internal/av1/obu"
)

// Av1C holds a parsed AV1CodecConfigurationBox (av1C) payload. ConfigOBUs
// may be empty when the sequence header lives only in sample data (common for
// ffmpeg-produced AVIF).
type Av1C struct {
	Marker                           uint8
	Version                          uint8
	SeqProfile                       uint8
	SeqLevelIdx0                     uint8
	SeqTier0                         uint8
	HighBitdepth                     uint8
	TwelveBit                        uint8
	Monochrome                       uint8
	ChromaSubsamplingX               uint8
	ChromaSubsamplingY               uint8
	ChromaSamplePosition             uint8
	InitialPresentationDelayPresent  uint8
	InitialPresentationDelayMinusOne uint8
	ConfigOBUs                       []byte
}

// ParseAv1C parses the payload of an av1C box (without the box header).
func ParseAv1C(payload []byte) (Av1C, error) {
	if len(payload) < 4 {
		return Av1C{}, fmt.Errorf("container: av1C payload %d < 4 bytes", len(payload))
	}
	p := Av1C{
		Marker:               payload[0] >> 7,
		Version:              payload[0] & 0x7f,
		SeqProfile:           payload[1] >> 5,
		SeqLevelIdx0:         payload[1] & 0x1f,
		SeqTier0:             (payload[2] >> 7) & 0x01,
		HighBitdepth:         (payload[2] >> 6) & 0x01,
		TwelveBit:            (payload[2] >> 5) & 0x01,
		Monochrome:           (payload[2] >> 4) & 0x01,
		ChromaSubsamplingX:   (payload[2] >> 3) & 0x01,
		ChromaSubsamplingY:   (payload[2] >> 2) & 0x01,
		ChromaSamplePosition: payload[2] & 0x03,
	}
	p.InitialPresentationDelayPresent = (payload[3] >> 4) & 0x01
	if p.InitialPresentationDelayPresent != 0 {
		p.InitialPresentationDelayMinusOne = payload[3] & 0x0f
	}
	if len(payload) > 4 {
		p.ConfigOBUs = append([]byte(nil), payload[4:]...)
	}
	return p, nil
}

// BuildTemporalUnit wraps sample bytes as one AV1 low-overhead temporal unit.
// When configOBUs is non-empty, its OBUs are inserted after the temporal
// delimiter and before the sample OBUs (typical MP4/MOV av1C + sample layout).
// When configOBUs is empty, sample must carry a sequence header OBU if the
// decoder has not seen one yet (ffmpeg AVIF).
func BuildTemporalUnit(configOBUs []byte, sample []byte) ([]byte, error) {
	if len(sample) == 0 {
		return nil, errors.New("container: empty sample")
	}
	configUnits, err := splitLowOverheadOBUs(configOBUs)
	if err != nil {
		return nil, fmt.Errorf("container: config OBUs: %w", err)
	}
	sampleUnits, err := splitLowOverheadOBUs(sample)
	if err != nil {
		return nil, fmt.Errorf("container: sample OBUs: %w", err)
	}
	if len(sampleUnits) > 0 && sampleUnits[0].Header.Type == internalobu.TypeTemporalDelimiter {
		sampleUnits = sampleUnits[1:]
	}
	if len(configUnits) == 0 && len(sampleUnits) == 0 {
		return nil, errors.New("container: no OBUs in config or sample")
	}
	enc := make([]internalencoder.OBU, 0, len(configUnits)+len(sampleUnits))
	for _, u := range configUnits {
		if u.Header.Type == internalobu.TypeTemporalDelimiter {
			continue
		}
		enc = append(enc, encoderOBU(u))
	}
	for _, u := range sampleUnits {
		if u.Header.Type == internalobu.TypeTemporalDelimiter {
			continue
		}
		enc = append(enc, encoderOBU(u))
	}
	if len(enc) == 0 {
		return nil, errors.New("container: no frame OBUs after merge")
	}
	size, err := internalencoder.LowOverheadTemporalUnitSize(enc)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, size)
	out, err = internalencoder.AppendLowOverheadTemporalUnit(out, enc)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func encoderOBU(u internalobu.Unit) internalencoder.OBU {
	return internalencoder.OBU{
		Type:       u.Header.Type,
		TemporalID: u.Header.TemporalID,
		SpatialID:  u.Header.SpatialID,
		Payload:    u.Payload,
	}
}

func splitLowOverheadOBUs(src []byte) ([]internalobu.Unit, error) {
	if len(src) == 0 {
		return nil, nil
	}
	var out []internalobu.Unit
	rest := src
	for len(rest) > 0 {
		unit, n, err := internalobu.ParseLowOverhead(rest)
		if err != nil {
			return nil, err
		}
		out = append(out, unit)
		rest = rest[n:]
	}
	return out, nil
}
