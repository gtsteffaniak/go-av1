package decode

import (
	internaldecoder "github.com/gtsteffaniak/go-av1/internal/av1/decoder"
	av1 "github.com/gtsteffaniak/go-av1/pkg/av1"
)

// DecoderFrameLayerPool adapts a root FrameLayerPool to the decoder's
// external-reference provider and releaser interfaces.
type DecoderFrameLayerPool = internaldecoder.FrameLayerPool

// DecoderTemporalMotionReferenceProvider resolves caller-owned global surface
// IDs to temporal motion-vector reference metadata for custom SVC batch runners.
type DecoderTemporalMotionReferenceProvider = internaldecoder.TemporalMotionReferenceProvider

// DecoderTemporalMotionReferenceProviderFunc adapts a function to
// DecoderTemporalMotionReferenceProvider.
type DecoderTemporalMotionReferenceProviderFunc = internaldecoder.TemporalMotionReferenceProviderFunc

// NewDecoderFrameLayerPool wraps pool as a DecoderFrameSurfaceProvider and
// DecoderFrameSurfaceReleaser. The caller still owns pool and its backing
// storage.
func NewDecoderFrameLayerPool(pool *av1.FrameLayerPool) DecoderFrameLayerPool {
	return internaldecoder.NewFrameLayerPool(pool)
}

// DecoderAcquireLayerFrameSurface acquires a frame surface from the sub-pool
// matching the coded format implied by sequence and size. It returns the owning
// sub-pool, local surface index, LayerPool global surface ID, and frame pointer.
func DecoderAcquireLayerFrameSurface(pool *av1.FrameLayerPool, sequence av1.SequenceHeader, size av1.FrameSize, align int) (*av1.FramePool, int, int, *av1.Frame, error) {
	return internaldecoder.AcquireLayerFrameSurface(pool, sequence, size, align)
}

// DecoderLayerPoolGlobalSurfaceID returns the LayerPool-encoded global ID for a
// local surface in sub. It returns -1 when sub is not bound to pool or local is
// invalid.
func DecoderLayerPoolGlobalSurfaceID(pool *av1.FrameLayerPool, sub *av1.FramePool, local int) int {
	return internaldecoder.LayerPoolGlobalSurfaceID(pool, sub, local)
}

// ResolveDecoderFrameReferencesWithProvider resolves caller-owned global
// surface IDs through provider instead of a single FramePool.
func ResolveDecoderFrameReferencesWithProvider(provider DecoderFrameSurfaceProvider, surfaces []int, frames []*av1.Frame) (int, error) {
	return internaldecoder.ResolveFrameReferencesWithProvider(provider, surfaces, frames)
}

// ResolveDecoderTileListExternalReferencesWithProvider resolves the external
// reference frames named by a TileList. externalSurfaces and frames are indexed
// by TileListEntry.AnchorFrameIdx, matching libaom's ext_refs table and the
// source slice expected by CopyTileListToOutputFrame. The returned count is
// max(anchor_frame_idx)+1, and frames[:count] is rewritten atomically so unused
// holes are cleared to nil.
func ResolveDecoderTileListExternalReferencesWithProvider(provider DecoderFrameSurfaceProvider, list av1.TileList, externalSurfaces []int, frames []*av1.Frame) (int, error) {
	return internaldecoder.ResolveTileListExternalReferencesWithProvider(provider, list, externalSurfaces, frames)
}

// PublishDecoderTemporalMotionReference stores the decoded frame's temporal
// motion-vector reference metadata under surface in store.
func PublishDecoderTemporalMotionReference(event *DecoderEvent, surface int, current *av1.TileReferenceMVFrame, store []av1.TileTemporalMotionReferenceFrame) error {
	return internaldecoder.PublishTemporalMotionReference(event, surface, current, store)
}

// ResolveDecoderTemporalMotionReferences resolves local surface indices to
// temporal motion-vector reference metadata from store.
func ResolveDecoderTemporalMotionReferences(surfaces []int, store []av1.TileTemporalMotionReferenceFrame, dst []av1.TileTemporalMotionReferenceFrame) (int, error) {
	return internaldecoder.ResolveTemporalMotionReferences(surfaces, store, dst)
}

// ResolveDecoderTemporalMotionReferencesWithProvider resolves caller-owned
// global surface IDs through provider instead of a single temporal-motion store.
func ResolveDecoderTemporalMotionReferencesWithProvider(provider DecoderTemporalMotionReferenceProvider, surfaces []int, dst []av1.TileTemporalMotionReferenceFrame) (int, error) {
	return internaldecoder.ResolveTemporalMotionReferencesWithProvider(provider, surfaces, dst)
}
