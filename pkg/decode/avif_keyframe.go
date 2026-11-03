package decode

import (
	"errors"
	"fmt"

	"github.com/gtsteffaniak/go-av1/pkg/container"
	"github.com/gtsteffaniak/go-av1/pkg/container/isobmff"
)

// KeyframeAVIFGrid describes a grid-type AVIF primary item and its tile samples.
type KeyframeAVIFGrid struct {
	Grid      *isobmff.ImageGrid
	TileUnits [][]byte // temporal units per tile, row-major
	Av1C      container.Av1C
	HasAv1C   bool
}

// ParseAVIFGrid parses a grid primary AVIF and returns per-tile temporal units
// plus grid geometry. Non-grid files return ErrNotAVIFGrid.
func ParseAVIFGrid(data []byte) (KeyframeAVIFGrid, error) {
	ct, err := isobmff.ParseContainer(data)
	if err != nil {
		return KeyframeAVIFGrid{}, err
	}
	primaryID := ct.PrimaryItemID()
	if primaryID == 0 {
		return KeyframeAVIFGrid{}, fmt.Errorf("goav1: no primary item")
	}
	if ct.ItemType(primaryID) != isobmff.TypeGridItem {
		return KeyframeAVIFGrid{}, ErrNotAVIFGrid
	}
	gridBytes, err := ct.ItemData(primaryID)
	if err != nil {
		return KeyframeAVIFGrid{}, err
	}
	grid, err := isobmff.ParseImageGrid(gridBytes)
	if err != nil {
		return KeyframeAVIFGrid{}, err
	}
	tileIDs := ct.FindDimgTargets(primaryID)
	want := int(grid.Rows) * int(grid.Columns)
	if len(tileIDs) != want {
		return KeyframeAVIFGrid{}, fmt.Errorf("goav1: grid expects %d tiles, iref has %d", want, len(tileIDs))
	}
	out := KeyframeAVIFGrid{Grid: grid}
	if av1c, ok := avifTileAv1C(ct, tileIDs[0]); ok {
		out.Av1C = av1c
		out.HasAv1C = true
	}
	out.TileUnits = make([][]byte, len(tileIDs))
	var config []byte
	if out.HasAv1C {
		config = out.Av1C.ConfigOBUs
	}
	for i, tid := range tileIDs {
		sample, err := ct.ItemData(tid)
		if err != nil {
			return KeyframeAVIFGrid{}, err
		}
		unit, err := container.BuildTemporalUnit(config, sample)
		if err != nil {
			return KeyframeAVIFGrid{}, fmt.Errorf("goav1: tile %d: %w", i, err)
		}
		out.TileUnits[i] = unit
	}
	return out, nil
}

// ErrNotAVIFGrid is returned when ParseAVIFGrid is called on a non-grid AVIF.
var ErrNotAVIFGrid = errors.New("goav1: not an AVIF grid item")

// DecodeAVIFGridKeyframe decodes each tile with kd and composites Y/UV planes into
// one KeyframeImage cropped to grid output dimensions.
func DecodeAVIFGridKeyframe(kd *KeyframeDecoder, data []byte) (KeyframeImage, error) {
	grid, err := ParseAVIFGrid(data)
	if err != nil {
		return KeyframeImage{}, err
	}
	if kd == nil {
		return KeyframeImage{}, fmt.Errorf("goav1: nil KeyframeDecoder")
	}
	cols := int(grid.Grid.Columns)
	var tileW, tileH int
	var tiles []KeyframeImage
	for i, unit := range grid.TileUnits {
		img, err := kd.DecodeTemporalUnit(unit)
		if err != nil {
			return KeyframeImage{}, fmt.Errorf("goav1: tile %d: %w", i, err)
		}
		if i == 0 {
			tileW = img.YStride
			if img.Width > 0 {
				tileW = img.Width
			}
			tileH = img.Height
		}
		tiles = append(tiles, img)
	}
	outW := int(grid.Grid.OutputWidth)
	outH := int(grid.Grid.OutputHeight)
	out := KeyframeImage{
		Width:  outW,
		Height: outH,
	}
	out.YStride = outW
	out.UVStride = (outW + 1) / 2
	out.Y = make([]byte, outW*outH)
	out.U = make([]byte, ((outW+1)/2)*((outH+1)/2))
	out.V = make([]byte, len(out.U))
	for i, tile := range tiles {
		row := i / cols
		col := i % cols
		dstX := col * tileW
		dstY := row * tileH
		pasteKeyframeYUV(&out, dstX, dstY, tile)
	}
	return out, nil
}

func avifTileAv1C(ct *isobmff.Container, itemID uint32) (container.Av1C, bool) {
	if ct == nil || ct.Meta == nil {
		return container.Av1C{}, false
	}
	for _, ch := range ct.Meta.Children {
		iprp, ok := ch.(*isobmff.Iprp)
		if !ok {
			continue
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
					if c, ok := iprp.Ipco.Properties[a.PropertyIndex-1].(*isobmff.Av1C); ok {
						p, err := container.ParseAv1C(c.ConfigOBUs)
						if err != nil {
							return container.Av1C{}, false
						}
						p.ConfigOBUs = append([]byte(nil), c.ConfigOBUs...)
						return p, true
					}
				}
			}
		}
	}
	return container.Av1C{}, false
}

func pasteKeyframeYUV(dst *KeyframeImage, dstX, dstY int, tile KeyframeImage) {
	tw := tile.Width
	th := tile.Height
	if tw == 0 {
		tw = tile.YStride
	}
	for y := 0; y < th; y++ {
		dy := dstY + y
		if dy >= dst.Height {
			break
		}
		for x := 0; x < tw; x++ {
			dx := dstX + x
			if dx >= dst.Width {
				break
			}
			dst.Y[dy*dst.YStride+dx] = tile.Y[y*tile.YStride+x]
		}
	}
	uScale := 2
	for y := 0; y < (th+uScale-1)/uScale; y++ {
		dy := (dstY / uScale) + y
		if dy >= (dst.Height+uScale-1)/uScale {
			break
		}
		for x := 0; x < (tw+uScale-1)/uScale; x++ {
			dx := (dstX / uScale) + x
			if dx >= (dst.Width+uScale-1)/uScale {
				break
			}
			if y*tile.UVStride+x >= len(tile.U) {
				continue
			}
			dst.U[dy*dst.UVStride+dx] = tile.U[y*tile.UVStride+x]
			dst.V[dy*dst.UVStride+dx] = tile.V[y*tile.UVStride+x]
		}
	}
}
