package isobmff

import "fmt"

// mdatRegionPlan maps absolute file offsets inside parsed mdat payload
// regions from their on-disk positions to the positions they will occupy
// after [Container.WriteTo] lays boxes out as ftyp, extras, meta, moov,
// then each mdat in parse order.
type mdatRegionPlan struct {
	oldRegions []MdatRegion
	newStarts  []uint64
}

func (ct *Container) mdatRegionsForWrite() []MdatRegion {
	if len(ct.Mdats) > 0 {
		return ct.Mdats
	}
	if ct.Mdat != nil {
		off := ct.MdatOffset
		return []MdatRegion{{PayloadOffset: off, Data: ct.Mdat.Data}}
	}
	return nil
}

func planMdatPayloadStarts(regions []MdatRegion, prefixLen uint64) []uint64 {
	starts := make([]uint64, len(regions))
	off := prefixLen
	for i, reg := range regions {
		hdr := headerLen(uint64(len(reg.Data)), TypeMdat)
		starts[i] = off + hdr
		off += hdr + uint64(len(reg.Data))
	}
	return starts
}

func newMdatRegionPlan(oldRegions, newRegions []MdatRegion, newStarts []uint64) mdatRegionPlan {
	return mdatRegionPlan{oldRegions: oldRegions, newStarts: newStarts}
}

func (p mdatRegionPlan) mapFileOffset(oldAbs uint64) (uint64, error) {
	for i, reg := range p.oldRegions {
		end, err := checkedAddU64(reg.PayloadOffset, uint64(len(reg.Data)))
		if err != nil {
			return 0, err
		}
		if oldAbs >= reg.PayloadOffset && oldAbs < end {
			rel := oldAbs - reg.PayloadOffset
			return p.newStarts[i] + rel, nil
		}
	}
	return 0, fmt.Errorf("%w: file offset %d not in any mdat payload", ErrInvalid, oldAbs)
}

func (p mdatRegionPlan) mapPayloadRelative(rel uint64) (uint64, error) {
	if len(p.newStarts) == 0 {
		return 0, fmt.Errorf("%w: no mdat payload in write plan", ErrInvalid)
	}
	return p.newStarts[0] + rel, nil
}

func ilocUsesAbsoluteFileOffsets(meta *Meta, regions []MdatRegion) bool {
	if meta == nil || len(regions) == 0 {
		return false
	}
	firstStart := regions[0].PayloadOffset
	for _, ch := range meta.Children {
		il, ok := ch.(*Iloc)
		if !ok {
			continue
		}
		for _, item := range il.Items {
			for _, ex := range item.Extents {
				if item.BaseOffset+ex.Offset >= firstStart {
					return true
				}
			}
		}
	}
	return false
}

func remapIlocFileOffsets(meta *Meta, plan mdatRegionPlan, absolute bool) error {
	for _, ch := range meta.Children {
		il, ok := ch.(*Iloc)
		if !ok {
			continue
		}
		for i := range il.Items {
			item := &il.Items[i]
			if item.ConstructionMethod != ConstructionFileOffset && item.ConstructionMethod != 0 {
				return fmt.Errorf("%w: iloc item %d construction_method %d", ErrUnsupportedConstruction, item.ItemID, item.ConstructionMethod)
			}
			if absolute {
				for j := range item.Extents {
					ex := &item.Extents[j]
					oldEff := item.BaseOffset + ex.Offset
					newEff, err := plan.mapFileOffset(oldEff)
					if err != nil {
						return fmt.Errorf("iloc item %d extent %d: %w", item.ItemID, j, err)
					}
					item.BaseOffset = 0
					ex.Offset = newEff
				}
				continue
			}
			for j := range item.Extents {
				ex := &item.Extents[j]
				newEff, err := plan.mapPayloadRelative(item.BaseOffset + ex.Offset)
				if err != nil {
					return fmt.Errorf("iloc item %d extent %d: %w", item.ItemID, j, err)
				}
				item.BaseOffset = 0
				ex.Offset = newEff
			}
		}
	}
	return nil
}

func remapMoovChunkOffsets(moov *Moov, plan mdatRegionPlan) error {
	for _, ch := range moov.Children {
		trak, ok := ch.(*Trak)
		if !ok {
			continue
		}
		if err := remapTrakChunkOffsets(trak, plan); err != nil {
			return err
		}
	}
	return nil
}

func remapTrakChunkOffsets(trak *Trak, plan mdatRegionPlan) error {
	for _, tc := range trak.Children {
		mdia, ok := tc.(*Mdia)
		if !ok {
			continue
		}
		for _, mc := range mdia.Children {
			minf, ok := mc.(*Minf)
			if !ok {
				continue
			}
			for _, nc := range minf.Children {
				stbl, ok := nc.(*Stbl)
				if !ok {
					continue
				}
				for _, sc := range stbl.Children {
					switch box := sc.(type) {
					case *Stco:
						for i := range box.Offsets {
							newOff, err := plan.mapFileOffset(uint64(box.Offsets[i]))
							if err != nil {
								return fmt.Errorf("stco[%d]: %w", i, err)
							}
							if newOff > 0xffffffff {
								return fmt.Errorf("%w: stco[%d] offset %d exceeds 32-bit", ErrInvalid, i, newOff)
							}
							box.Offsets[i] = uint32(newOff)
						}
					case *Co64:
						for i := range box.Offsets {
							newOff, err := plan.mapFileOffset(box.Offsets[i])
							if err != nil {
								return fmt.Errorf("co64[%d]: %w", i, err)
							}
							box.Offsets[i] = newOff
						}
					}
				}
			}
		}
	}
	return nil
}

func refreshMdatRegionsAfterWrite(ct *Container, regions []MdatRegion, newStarts []uint64) {
	out := make([]MdatRegion, len(regions))
	for i := range regions {
		out[i] = MdatRegion{
			PayloadOffset: newStarts[i],
			Data:          regions[i].Data,
		}
	}
	ct.Mdats = out
	if len(out) > 0 {
		ct.Mdat = &Mdat{Data: out[len(out)-1].Data}
		ct.MdatOffset = out[0].PayloadOffset
	}
}
