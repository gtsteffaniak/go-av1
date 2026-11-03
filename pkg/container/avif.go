package container

import (
	"fmt"

	"github.com/gtsteffaniak/go-av1/pkg/container/isobmff"
)

// AVIFPrimary describes the primary still image item in an AVIF/HEIF file.
type AVIFPrimary struct {
	ItemID  uint32
	Width   uint32
	Height  uint32
	Av1C    Av1C
	HasAv1C bool
	Sample  []byte // encoded item bytes (OBU bitstream)
	IsGrid  bool
}

// ParseAVIF parses an AVIF or AVIS ISOBMFF file and returns the primary item's
// av1C (when present) and coded sample bytes.
func ParseAVIF(data []byte) (AVIFPrimary, error) {
	ct, err := isobmff.ParseContainer(data)
	if err != nil {
		return AVIFPrimary{}, err
	}
	if ct.Ftyp == nil || (!ct.Ftyp.HasBrand("avif") && !ct.Ftyp.HasBrand("avis")) {
		return AVIFPrimary{}, fmt.Errorf("container: ftyp missing avif/avis brand")
	}
	primaryID := ct.PrimaryItemID()
	if primaryID == 0 {
		return AVIFPrimary{}, fmt.Errorf("container: no primary item (pitm)")
	}
	out := AVIFPrimary{ItemID: primaryID}
	if ct.ItemType(primaryID) == isobmff.TypeGridItem {
		out.IsGrid = true
	}
	w, h, ok := primarySpatialExtents(ct, primaryID)
	if ok {
		out.Width, out.Height = w, h
	}
	if av1c, ok := primaryAv1C(ct, primaryID); ok {
		out.Av1C = av1c
		out.HasAv1C = true
	}
	sample, err := ct.ItemData(primaryID)
	if err != nil {
		return AVIFPrimary{}, err
	}
	out.Sample = sample
	return out, nil
}

// TemporalUnitFromAVIF builds a decoder temporal unit from a parsed AVIF primary item.
func TemporalUnitFromAVIF(p AVIFPrimary) ([]byte, error) {
	var config []byte
	if p.HasAv1C {
		config = p.Av1C.ConfigOBUs
	}
	return BuildTemporalUnit(config, p.Sample)
}

func primaryAv1C(ct *isobmff.Container, itemID uint32) (Av1C, bool) {
	iprp := findIprp(ct)
	if iprp == nil {
		return Av1C{}, false
	}
	for _, m := range iprp.Ipma {
		for _, e := range m.Entries {
			if e.ItemID != itemID {
				continue
			}
			for _, a := range e.Associations {
				if a.PropertyIndex == 0 || int(a.PropertyIndex) > len(iprp.Ipco.Properties) {
					continue
				}
				prop := iprp.Ipco.Properties[a.PropertyIndex-1]
				if c, ok := prop.(*isobmff.Av1C); ok {
					p, err := ParseAv1C(c.ConfigOBUs)
					if err != nil {
						return Av1C{}, false
					}
					// Preserve full config blob including header bytes for BuildTemporalUnit.
					p.ConfigOBUs = append([]byte(nil), c.ConfigOBUs...)
					return p, true
				}
			}
		}
	}
	return Av1C{}, false
}

func primarySpatialExtents(ct *isobmff.Container, itemID uint32) (uint32, uint32, bool) {
	iprp := findIprp(ct)
	if iprp == nil {
		return 0, 0, false
	}
	for _, m := range iprp.Ipma {
		for _, e := range m.Entries {
			if e.ItemID != itemID {
				continue
			}
			for _, a := range e.Associations {
				if a.PropertyIndex == 0 || int(a.PropertyIndex) > len(iprp.Ipco.Properties) {
					continue
				}
				if ispe, ok := iprp.Ipco.Properties[a.PropertyIndex-1].(*isobmff.Ispe); ok {
					return ispe.Width, ispe.Height, true
				}
			}
		}
	}
	return 0, 0, false
}

func findIprp(ct *isobmff.Container) *isobmff.Iprp {
	if ct.Meta == nil {
		return nil
	}
	for _, ch := range ct.Meta.Children {
		if ip, ok := ch.(*isobmff.Iprp); ok {
			return ip
		}
	}
	return nil
}
