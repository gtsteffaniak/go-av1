package isobmff

import "fmt"

// MdatRegion is one mdat box payload slice and its absolute file offset.
type MdatRegion struct {
	PayloadOffset uint64
	Data          []byte
}

func (ct *Container) appendMdatRegion(payloadOffset uint64, data []byte) {
	if ct == nil {
		return
	}
	ct.Mdats = append(ct.Mdats, MdatRegion{PayloadOffset: payloadOffset, Data: data})
	ct.Mdat = &Mdat{Data: data}
	ct.MdatOffset = payloadOffset
}

func (ct *Container) readFileRange(fileOff, length uint64) ([]byte, error) {
	if length == 0 {
		return nil, nil
	}
	end, err := checkedAddU64(fileOff, length)
	if err != nil {
		return nil, err
	}
	regions := ct.Mdats
	if len(regions) == 0 && ct.Mdat != nil {
		regions = []MdatRegion{{PayloadOffset: ct.MdatOffset, Data: ct.Mdat.Data}}
	}
	for _, m := range regions {
		mEnd, err := checkedAddU64(m.PayloadOffset, uint64(len(m.Data)))
		if err != nil {
			return nil, err
		}
		if fileOff >= m.PayloadOffset && end <= mEnd {
			rel := fileOff - m.PayloadOffset
			return m.Data[rel : rel+length], nil
		}
	}
	return nil, fmt.Errorf("%w: file range %d..%d not in any mdat", ErrInvalid, fileOff, end)
}

func checkedAddU64(a, b uint64) (uint64, error) {
	if a > ^uint64(0)-b {
		return 0, fmt.Errorf("%w: offset overflow", ErrInvalid)
	}
	return a + b, nil
}

// ReadPayloadAt returns length bytes at absolute file offset fileOff from an mdat region.
func (ct *Container) ReadPayloadAt(fileOff, length uint64) ([]byte, error) {
	return ct.readFileRange(fileOff, length)
}
