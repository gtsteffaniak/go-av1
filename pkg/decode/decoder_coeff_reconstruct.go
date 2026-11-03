package decode

import av1 "github.com/gtsteffaniak/go-av1/pkg/av1"

// DecoderFrameWorkCoeffReconstructionContext carries the per-block frame-work
// state needed to place a coefficient replay block into the output frame.
type DecoderFrameWorkCoeffReconstructionContext struct {
	Visit av1.TileBlockVisit

	CurrentQIndex uint8
	SegmentID     uint8

	Int32Scratch    []int32
	ResidualScratch []int16
}

// DecoderFrameWorkBlockCoeffDecodeRequest composes one block coefficient decode
// request with the frame-work reconstruction context for the same block.
type DecoderFrameWorkBlockCoeffDecodeRequest struct {
	Decode         av1.TileBlockCoeffRequest
	Reconstruction DecoderFrameWorkCoeffReconstructionContext
}

// DecoderFrameWorkLumaCoeffBlockReconstruction packages a decoded luma
// coefficient block together with the reconstruction context required to
// apply it to the output frame.
func DecoderFrameWorkLumaCoeffBlockReconstruction(ctx DecoderFrameWorkCoeffReconstructionContext, block av1.TileLumaCoeffBlock) DecoderFrameWorkBlockCoeffReconstruction {
	return DecoderFrameWorkBlockCoeffReconstruction{
		Visit: ctx.Visit,
		Block: av1.TileBlockCoeffBlock{
			Plane:     0,
			Block:     block.Block,
			Transform: block.Transform,
			Result:    block.Result,
			Coeffs:    block.Coeffs,
			Scan:      block.Scan,
		},
		Transform:       block.Transform,
		CurrentQIndex:   ctx.CurrentQIndex,
		SegmentID:       ctx.SegmentID,
		Int32Scratch:    ctx.Int32Scratch,
		ResidualScratch: ctx.ResidualScratch,
	}
}

// DecoderFrameWorkChromaCoeffBlockReconstruction packages a decoded chroma
// coefficient block together with the reconstruction context required to
// apply it to the output frame.
func DecoderFrameWorkChromaCoeffBlockReconstruction(ctx DecoderFrameWorkCoeffReconstructionContext, block av1.TileChromaCoeffBlock) DecoderFrameWorkBlockCoeffReconstruction {
	return DecoderFrameWorkBlockCoeffReconstruction{
		Visit: ctx.Visit,
		Block: av1.TileBlockCoeffBlock{
			Plane:     uint8(block.Plane),
			Block:     block.Block,
			Transform: block.Transform,
			Result:    block.Result,
			Coeffs:    block.Coeffs,
			Scan:      block.Scan,
		},
		Transform:       block.Transform,
		CurrentQIndex:   ctx.CurrentQIndex,
		SegmentID:       ctx.SegmentID,
		Int32Scratch:    ctx.Int32Scratch,
		ResidualScratch: ctx.ResidualScratch,
	}
}

// ReconstructDecoderFrameWorkLumaCoeffBlock reconstructs a single luma
// coefficient block into the output frame for the batch entry at index.
func ReconstructDecoderFrameWorkLumaCoeffBlock(batch DecoderFrameWorkBatch, index int, ctx DecoderFrameWorkCoeffReconstructionContext, block av1.TileLumaCoeffBlock) error {
	return ReconstructDecoderFrameWorkBlockCoeff(batch, index, DecoderFrameWorkLumaCoeffBlockReconstruction(ctx, block))
}

// ReconstructDecoderFrameWorkChromaCoeffBlock reconstructs a single chroma
// coefficient block into the output frame for the batch entry at index.
func ReconstructDecoderFrameWorkChromaCoeffBlock(batch DecoderFrameWorkBatch, index int, ctx DecoderFrameWorkCoeffReconstructionContext, block av1.TileChromaCoeffBlock) error {
	return ReconstructDecoderFrameWorkBlockCoeff(batch, index, DecoderFrameWorkChromaCoeffBlockReconstruction(ctx, block))
}

// DecodeAndReconstructDecoderFrameWorkBlockCoefficients decodes the block
// coefficients described by req.Decode and immediately reconstructs each
// emitted coefficient block into the output frame, then forwards the block
// to visit. It fuses the two decode/reconstruct steps for callers that do
// not need to separate them.
func DecodeAndReconstructDecoderFrameWorkBlockCoefficients(batch DecoderFrameWorkBatch, index int, state *av1.TileDecodeState, cdfs av1.TileBlockCoeffCDFs, transformCtx *av1.TileTransformContext, coeffCtx *av1.TileCoeffEntropyContext, scratch *av1.TileBlockCoeffScratch, req DecoderFrameWorkBlockCoeffDecodeRequest, visit av1.TileBlockCoeffVisitor) (av1.TileBlockCoeffResult, error) {
	return av1.DecodeTileBlockCoefficients(state, cdfs, transformCtx, coeffCtx, scratch, req.Decode, func(block av1.TileBlockCoeffBlock) error {
		if err := ReconstructDecoderFrameWorkBlockCoeff(batch, index, DecoderFrameWorkBlockCoeffReconstruction{
			Visit:           req.Reconstruction.Visit,
			Block:           block,
			Transform:       block.Transform,
			CurrentQIndex:   req.Reconstruction.CurrentQIndex,
			SegmentID:       req.Reconstruction.SegmentID,
			Int32Scratch:    req.Reconstruction.Int32Scratch,
			ResidualScratch: req.Reconstruction.ResidualScratch,
		}); err != nil {
			return err
		}
		if visit != nil {
			return visit(block)
		}
		return nil
	})
}
