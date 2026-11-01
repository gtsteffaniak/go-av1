package obu

import (
	"errors"

	"github.com/gtsteffaniak/go-av1/internal/av1/bitstream"
)

var (
	// ErrInvalidTemporalUnit is returned when OBU inputs cannot form a unit.
	ErrInvalidTemporalUnit = errors.New("obu: invalid temporal unit")
)

// LowOverheadTemporalUnitSize returns the exact number of bytes required to emit
// a low-overhead temporal unit: one temporal-delimiter OBU followed by units.
func LowOverheadTemporalUnitSize(units []Unit) (int, error) {
	if len(units) == 0 {
		return 0, ErrInvalidTemporalUnit
	}
	size := lowOverheadOBUSizeUnchecked(Unit{Header: Header{Type: TypeTemporalDelimiter}})
	for i := range units {
		u := units[i]
		if u.Header.Type == TypeTemporalDelimiter {
			return 0, ErrInvalidTemporalUnit
		}
		if err := validateLowOverheadUnit(u); err != nil {
			return 0, err
		}
		size += lowOverheadOBUSizeUnchecked(u)
	}
	return size, nil
}

// AppendLowOverheadTemporalUnit appends one temporal-delimiter OBU followed by
// units. The function never grows dst and validates the full unit before writing.
func AppendLowOverheadTemporalUnit(dst []byte, units []Unit) ([]byte, error) {
	size, err := LowOverheadTemporalUnitSize(units)
	if err != nil {
		return dst, err
	}
	if cap(dst)-len(dst) < size {
		return dst, bitstream.ErrShortBuffer
	}
	out, err := AppendLowOverheadOBU(dst, Unit{Header: Header{Type: TypeTemporalDelimiter}})
	if err != nil {
		return dst, err
	}
	for i := range units {
		out, err = AppendLowOverheadOBU(out, units[i])
		if err != nil {
			return dst, err
		}
	}
	return out, nil
}

// AppendLowOverheadOBU appends unit as one low-overhead OBU with obu_has_size_field set.
func AppendLowOverheadOBU(dst []byte, unit Unit) ([]byte, error) {
	if err := validateLowOverheadUnit(unit); err != nil {
		return dst, err
	}
	size := lowOverheadOBUSizeUnchecked(unit)
	if cap(dst)-len(dst) < size {
		return dst, bitstream.ErrShortBuffer
	}
	off := len(dst)
	out := dst[:off+size]
	h := unit.Header
	h.Extension = h.TemporalID != 0 || h.SpatialID != 0
	h.HasSizeField = true
	n, err := PutHeader(out[off:], h)
	if err != nil {
		return dst, err
	}
	off += n
	n, err = bitstream.PutLEB128(out[off:], uint32(len(unit.Payload)))
	if err != nil {
		return dst, err
	}
	off += n
	copy(out[off:], unit.Payload)
	return out, nil
}

func validateLowOverheadUnit(unit Unit) error {
	if unit.Header.Type > 15 || unit.Header.Type.Reserved() {
		return ErrInvalidType
	}
	if unit.Header.TemporalID > 7 || unit.Header.SpatialID > 3 {
		return ErrInvalidType
	}
	if uint64(len(unit.Payload)) > bitstream.MaxLEB128Value {
		return bitstream.ErrLEB128Overflow
	}
	return nil
}

func lowOverheadOBUSizeUnchecked(unit Unit) int {
	headerLen := 1
	if unit.Header.TemporalID != 0 || unit.Header.SpatialID != 0 {
		headerLen = 2
	}
	return headerLen + bitstream.LEB128Len(uint32(len(unit.Payload))) + len(unit.Payload)
}

// TemporalUnit is one Section 5 low-overhead temporal unit. Raw aliases the
// caller-provided stream bytes.
type TemporalUnit struct {
	Raw   []byte
	Index uint32
}

// TemporalUnitIterator groups AV1 low-overhead OBUs into temporal units.
type TemporalUnitIterator struct {
	src   []byte
	off   uint32
	index uint32
}

func NewTemporalUnitIterator(src []byte) TemporalUnitIterator {
	return TemporalUnitIterator{src: src}
}

func (it *TemporalUnitIterator) Next() (TemporalUnit, bool, error) {
	srcLen := len(it.src)
	if uint64(srcLen) > uint64(^uint32(0)) {
		return TemporalUnit{}, false, ErrShortPayload
	}
	off := int(it.off)
	if off == srcLen {
		return TemporalUnit{}, false, nil
	}
	if off > srcLen {
		return TemporalUnit{}, false, ErrShortPayload
	}

	start := off
	first := true
	for off < srcLen {
		unit, consumed, err := ParseLowOverhead(it.src[off:])
		if err != nil {
			return TemporalUnit{}, false, err
		}
		if first {
			if unit.Header.Type != TypeTemporalDelimiter {
				return TemporalUnit{}, false, ErrMissingTemporalDelimiter
			}
			first = false
			off += consumed
			it.off = uint32(off)
			continue
		}
		if unit.Header.Type == TypeTemporalDelimiter {
			result := TemporalUnit{Raw: it.src[start:off], Index: it.index}
			it.index++
			return result, true, nil
		}
		off += consumed
		it.off = uint32(off)
	}

	result := TemporalUnit{Raw: it.src[start:off], Index: it.index}
	it.index++
	return result, true, nil
}
