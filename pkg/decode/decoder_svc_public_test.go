package decode_test

import (
	"errors"
	av1 "github.com/gtsteffaniak/go-av1/pkg/av1"
	decode "github.com/gtsteffaniak/go-av1/pkg/decode"
	"testing"
)

func TestPublicDecoderFrameLayerPoolProviderAndReleaser(t *testing.T) {
	layerPool := newPublicDecoderLayerPool(t, 2, 2)
	layer0 := av1.FrameFormat{Width: 640, Height: 360, BitDepth: 8, SubsamplingX: true, SubsamplingY: true, Align: 32}
	layer1 := av1.FrameFormat{Width: 1280, Height: 720, BitDepth: 8, SubsamplingX: true, SubsamplingY: true, Align: 32}

	id0, want0, err := layerPool.Acquire(layer0)
	if err != nil {
		t.Fatal(err)
	}
	id1, want1, err := layerPool.Acquire(layer1)
	if err != nil {
		t.Fatal(err)
	}

	adapter := decode.NewDecoderFrameLayerPool(&layerPool)
	var provider decode.DecoderFrameSurfaceProvider = adapter
	var releaser decode.DecoderFrameSurfaceReleaser = adapter
	if got, err := provider.FrameSurface(id0); err != nil || got != want0 {
		t.Fatalf("FrameSurface(layer0)=%p err=%v want %p", got, err, want0)
	}
	if got, err := provider.FrameSurface(id1); err != nil || got != want1 {
		t.Fatalf("FrameSurface(layer1)=%p err=%v want %p", got, err, want1)
	}

	gotRefs := make([]*av1.Frame, 2)
	count, err := decode.ResolveDecoderFrameReferencesWithProvider(provider, []int{id0, id1}, gotRefs)
	if err != nil {
		t.Fatalf("ResolveDecoderFrameReferencesWithProvider: %v", err)
	}
	if count != 2 || gotRefs[0] != want0 || gotRefs[1] != want1 {
		t.Fatalf("count=%d refs=%p,%p want %p,%p", count, gotRefs[0], gotRefs[1], want0, want1)
	}

	if err := releaser.ReleaseFrameSurfaces([]int{id0, id1}); err != nil {
		t.Fatalf("ReleaseFrameSurfaces: %v", err)
	}
	if _, err := layerPool.Frame(id0); err == nil {
		t.Fatalf("layer0 surface was not released")
	}
	if _, err := layerPool.Frame(id1); err == nil {
		t.Fatalf("layer1 surface was not released")
	}
}

func TestPublicResolveDecoderTileListExternalReferencesWithProvider(t *testing.T) {
	layerPool := newPublicDecoderLayerPool(t, 1, 2)
	format := av1.FrameFormat{Width: 128, Height: 64, BitDepth: 8, SubsamplingX: true, SubsamplingY: true, Align: 32}
	id0, want0, err := layerPool.Acquire(format)
	if err != nil {
		t.Fatal(err)
	}
	id5, want5, err := layerPool.Acquire(format)
	if err != nil {
		t.Fatal(err)
	}
	provider := decode.NewDecoderFrameLayerPool(&layerPool)
	list := av1.TileList{
		TileCountMinus1: 1,
		Entries: []av1.TileListEntry{
			{AnchorFrameIdx: 5},
			{AnchorFrameIdx: 0},
		},
	}
	surfaces := []int{id0, -1, -1, -1, -1, id5}
	sentinel := &av1.Frame{}
	refs := []*av1.Frame{sentinel, sentinel, sentinel, sentinel, sentinel, sentinel}

	count, err := decode.ResolveDecoderTileListExternalReferencesWithProvider(provider, list, surfaces, refs)
	if err != nil {
		t.Fatalf("ResolveDecoderTileListExternalReferencesWithProvider: %v", err)
	}
	if count != 6 || refs[0] != want0 || refs[5] != want5 {
		t.Fatalf("count=%d refs[0]=%p refs[5]=%p want %p,%p", count, refs[0], refs[5], want0, want5)
	}
	for i := 1; i < 5; i++ {
		if refs[i] != nil {
			t.Fatalf("hole refs[%d]=%p want nil", i, refs[i])
		}
	}
	if _, err := decode.ResolveDecoderTileListExternalReferencesWithProvider(provider, list, surfaces[:5], refs); !errors.Is(err, decode.ErrDecoderSurfaceReferenceBufferTooSmall) {
		t.Fatalf("short surfaces err=%v want %v", err, decode.ErrDecoderSurfaceReferenceBufferTooSmall)
	}
}

func TestPublicDecoderAcquireLayerFrameSurface(t *testing.T) {
	layerPool := newPublicDecoderLayerPool(t, 2, 2)
	sequence := av1.SequenceHeader{
		ColorConfig: av1.ColorConfig{BitDepth: 8, SubsamplingX: true, SubsamplingY: true},
	}
	layer0 := av1.FrameSize{CodedWidth: 640, Height: 360, UpscaledWidth: 640}
	layer1 := av1.FrameSize{CodedWidth: 1280, Height: 720, UpscaledWidth: 1280}

	sub0, local0, id0, frame0, err := decode.DecoderAcquireLayerFrameSurface(&layerPool, sequence, layer0, 32)
	if err != nil {
		t.Fatal(err)
	}
	sub1, local1, id1, frame1, err := decode.DecoderAcquireLayerFrameSurface(&layerPool, sequence, layer1, 32)
	if err != nil {
		t.Fatal(err)
	}
	if sub0 == sub1 {
		t.Fatalf("layer0 and layer1 used the same sub-pool")
	}
	if id0 == id1 {
		t.Fatalf("layer0 and layer1 used the same global surface ID")
	}
	if frame0.Format.Width != 640 || frame1.Format.Width != 1280 {
		t.Fatalf("frame widths=%d,%d want 640,1280", frame0.Format.Width, frame1.Format.Width)
	}
	if got := decode.DecoderLayerPoolGlobalSurfaceID(&layerPool, sub0, local0); got != id0 {
		t.Fatalf("layer0 global id=%d want %d", got, id0)
	}
	if got := decode.DecoderLayerPoolGlobalSurfaceID(&layerPool, sub1, local1); got != id1 {
		t.Fatalf("layer1 global id=%d want %d", got, id1)
	}
}

func TestPublicDecoderTemporalMotionReferenceHelpers(t *testing.T) {
	sequence := av1.SequenceHeader{
		ColorConfig: av1.ColorConfig{BitDepth: 8, SubsamplingX: true, SubsamplingY: true},
	}
	size := av1.FrameSize{CodedWidth: 64, Height: 64, UpscaledWidth: 64}
	_, _, length, err := decode.DecoderFrameWorkReferenceMVFrameShape(sequence, size)
	if err != nil {
		t.Fatal(err)
	}
	entries := make([]av1.TileReferenceMVEntry, length)
	mvFrame, err := decode.BindDecoderFrameWorkReferenceMVFrame(sequence, size, entries)
	if err != nil {
		t.Fatal(err)
	}
	event := decode.DecoderEvent{
		Kind:      decode.DecoderEventFrame,
		FrameSize: size,
		TileGroup: av1.TileGroup{Final: true},
		FrameHeader: av1.FrameHeaderPrefix{
			OrderHint: 9,
			FrameType: av1.FrameTypeKey,
		},
	}
	store := make([]av1.TileTemporalMotionReferenceFrame, 3)
	if err := decode.PublishDecoderTemporalMotionReference(&event, 1, &mvFrame, store); err != nil {
		t.Fatalf("PublishDecoderTemporalMotionReference: %v", err)
	}
	var dst [1]av1.TileTemporalMotionReferenceFrame
	count, err := decode.ResolveDecoderTemporalMotionReferences([]int{1}, store, dst[:])
	if err != nil {
		t.Fatalf("ResolveDecoderTemporalMotionReferences: %v", err)
	}
	if count != 1 || dst[0].Frame != &mvFrame || dst[0].OrderHint != 9 || !dst[0].IntraOnly {
		t.Fatalf("resolved=%+v count=%d", dst[0], count)
	}

	provider := decode.DecoderTemporalMotionReferenceProviderFunc(func(id int) (av1.TileTemporalMotionReferenceFrame, error) {
		if id != 256 {
			return av1.TileTemporalMotionReferenceFrame{}, decode.ErrDecoderInvalidSurfaceReference
		}
		return store[1], nil
	})
	count, err = decode.ResolveDecoderTemporalMotionReferencesWithProvider(provider, []int{256}, dst[:])
	if err != nil {
		t.Fatalf("ResolveDecoderTemporalMotionReferencesWithProvider: %v", err)
	}
	if count != 1 || dst[0].Frame != &mvFrame {
		t.Fatalf("provider resolved=%+v count=%d", dst[0], count)
	}
}

func TestPublicDecoderFrameLayerPoolNilRejected(t *testing.T) {
	adapter := decode.NewDecoderFrameLayerPool(nil)
	if _, err := adapter.FrameSurface(0); !errors.Is(err, decode.ErrDecoderInvalidSurfaceReference) {
		t.Fatalf("FrameSurface err=%v want %v", err, decode.ErrDecoderInvalidSurfaceReference)
	}
	if err := adapter.ReleaseFrameSurfaces([]int{0}); !errors.Is(err, decode.ErrDecoderInvalidSurfaceReference) {
		t.Fatalf("ReleaseFrameSurfaces err=%v want %v", err, decode.ErrDecoderInvalidSurfaceReference)
	}
}

func newPublicDecoderLayerPool(t *testing.T, layers int, surfacesPerLayer int) av1.FrameLayerPool {
	t.Helper()
	var backings [][]byte
	factory := av1.FrameLayerFactoryFunc(func(format av1.FrameFormat) (av1.FramePool, error) {
		_, backingSize, err := av1.FramePoolRequiredSize(format, surfacesPerLayer)
		if err != nil {
			return av1.FramePool{}, err
		}
		backing := make([]byte, backingSize)
		pool, err := av1.BindFramePool(backing, format,
			make([]av1.Frame, surfacesPerLayer),
			make([]int, surfacesPerLayer),
			make([]bool, surfacesPerLayer))
		if err != nil {
			return av1.FramePool{}, err
		}
		backings = append(backings, backing)
		return pool, nil
	})
	layerPool, err := av1.BindFrameLayerPool(
		make([]av1.FramePool, layers),
		make([]av1.FrameFormat, layers),
		make([]bool, layers),
		256,
		factory,
	)
	if err != nil {
		t.Fatal(err)
	}
	return layerPool
}
