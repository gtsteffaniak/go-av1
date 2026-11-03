package av1_test

import (
	"bytes"
	"errors"
	av1 "github.com/gtsteffaniak/go-av1/pkg/av1"
	decode "github.com/gtsteffaniak/go-av1/pkg/decode"
	"testing"
)

type publicDecoderBlockCoeffTB interface {
	Helper()
	Fatalf(format string, args ...any)
}

func TestPublicReconstructDecoderFrameWorkBlockCoeffLuma(t *testing.T) {
	got, want, batch, req := publicDecoderBlockCoeffLumaFixture(t)
	plane, x, y, err := decode.DecoderFrameWorkBlockCoeffPlanePosition(batch, 0, req.Visit, req.Block)
	if err != nil {
		t.Fatal(err)
	}
	if plane != decode.DecoderFrameWorkPlaneY || x != 68 || y != 8 {
		t.Fatalf("position plane=%d x=%d y=%d", plane, x, y)
	}

	qIndex, lossless, err := decode.DecoderFrameWorkBlockQIndex(batch, req.CurrentQIndex, req.SegmentID)
	if err != nil {
		t.Fatal(err)
	}
	if qIndex != 45 || lossless {
		t.Fatalf("qindex=%d lossless=%v want 45,false", qIndex, lossless)
	}
	q, quantLossless, err := decode.DecoderFrameWorkBlockQuantizer(batch, req.CurrentQIndex, req.SegmentID, plane)
	if err != nil {
		t.Fatal(err)
	}
	wantQ, err := av1.PlaneQuantizer(batch.Quantization, qIndex, 8, av1.QuantizerPlaneY)
	if err != nil {
		t.Fatal(err)
	}
	if q != wantQ || quantLossless != lossless {
		t.Fatalf("quantizer=%+v/%v want %+v/%v", q, quantLossless, wantQ, lossless)
	}

	req.Int32Scratch, req.ResidualScratch = publicDecoderBlockCoeffScratch(t, batch, req, plane)
	if err := decode.ReconstructDecoderFrameWorkBlockCoeff(batch, 0, req); err != nil {
		t.Fatal(err)
	}
	publicReconstructDecoderFrameWorkBlockCoeffDirect(t, batch, want, plane, x, y, req)
	if !bytes.Equal(got.Y.Pix, want.Y.Pix) {
		t.Fatal("luma reconstruction did not match direct public reconstruction")
	}
}

func TestPublicReconstructDecoderFrameWorkBlockCoeffChroma420(t *testing.T) {
	got, want, batch, req := publicDecoderBlockCoeffChromaFixture(t)
	plane, x, y, err := decode.DecoderFrameWorkBlockCoeffPlanePosition(batch, 0, req.Visit, req.Block)
	if err != nil {
		t.Fatal(err)
	}
	if plane != decode.DecoderFrameWorkPlaneV || x != 36 || y != 40 {
		t.Fatalf("position plane=%d x=%d y=%d", plane, x, y)
	}

	qIndex, lossless, err := decode.DecoderFrameWorkBlockQIndex(batch, req.CurrentQIndex, req.SegmentID)
	if err != nil {
		t.Fatal(err)
	}
	if qIndex != 64 || lossless {
		t.Fatalf("qindex=%d lossless=%v want 64,false", qIndex, lossless)
	}
	q, quantLossless, err := decode.DecoderFrameWorkBlockQuantizer(batch, req.CurrentQIndex, req.SegmentID, plane)
	if err != nil {
		t.Fatal(err)
	}
	wantQ, err := av1.PlaneQuantizer(batch.Quantization, qIndex, 8, av1.QuantizerPlaneV)
	if err != nil {
		t.Fatal(err)
	}
	if q != wantQ || quantLossless != lossless {
		t.Fatalf("quantizer=%+v/%v want %+v/%v", q, quantLossless, wantQ, lossless)
	}

	req.Int32Scratch, req.ResidualScratch = publicDecoderBlockCoeffScratch(t, batch, req, plane)
	if err := decode.ReconstructDecoderFrameWorkBlockCoeff(batch, 0, req); err != nil {
		t.Fatal(err)
	}
	publicReconstructDecoderFrameWorkBlockCoeffDirect(t, batch, want, plane, x, y, req)
	if !bytes.Equal(got.V.Pix, want.V.Pix) {
		t.Fatal("chroma reconstruction did not match direct public reconstruction")
	}
}

func TestPublicReconstructDecoderFrameWorkCoeffReplayAdapters(t *testing.T) {
	gotY, wantY, lumaBatch, lumaReq := publicDecoderBlockCoeffLumaFixture(t)
	lumaPlane, lumaX, lumaY, err := decode.DecoderFrameWorkBlockCoeffPlanePosition(lumaBatch, 0, lumaReq.Visit, lumaReq.Block)
	if err != nil {
		t.Fatal(err)
	}
	lumaCtx := publicDecoderBlockCoeffReplayContext(t, lumaBatch, lumaReq, lumaPlane)
	lumaBlock := av1.TileLumaCoeffBlock{
		Block:     lumaReq.Block.Block,
		Transform: lumaReq.Transform,
		Result:    lumaReq.Block.Result,
		Coeffs:    lumaReq.Block.Coeffs,
		Scan:      lumaReq.Block.Scan,
	}
	replayReq := decode.DecoderFrameWorkLumaCoeffBlockReconstruction(lumaCtx, lumaBlock)
	if replayReq.Block.Plane != 0 || replayReq.Transform != lumaReq.Transform || replayReq.Block.Block != lumaReq.Block.Block {
		t.Fatalf("luma replay reconstruction=%+v", replayReq)
	}
	if err := decode.ReconstructDecoderFrameWorkLumaCoeffBlock(lumaBatch, 0, lumaCtx, lumaBlock); err != nil {
		t.Fatal(err)
	}
	publicReconstructDecoderFrameWorkBlockCoeffDirect(t, lumaBatch, wantY, lumaPlane, lumaX, lumaY, lumaReq)
	if !bytes.Equal(gotY.Y.Pix, wantY.Y.Pix) {
		t.Fatal("luma replay reconstruction adapter did not match direct public reconstruction")
	}

	gotV, wantV, chromaBatch, chromaReq := publicDecoderBlockCoeffChromaFixture(t)
	chromaPlane, chromaX, chromaY, err := decode.DecoderFrameWorkBlockCoeffPlanePosition(chromaBatch, 0, chromaReq.Visit, chromaReq.Block)
	if err != nil {
		t.Fatal(err)
	}
	chromaCtx := publicDecoderBlockCoeffReplayContext(t, chromaBatch, chromaReq, chromaPlane)
	chromaBlock := av1.TileChromaCoeffBlock{
		Plane:     chromaReq.Block.Plane,
		Block:     chromaReq.Block.Block,
		Transform: chromaReq.Transform,
		Result:    chromaReq.Block.Result,
		Coeffs:    chromaReq.Block.Coeffs,
		Scan:      chromaReq.Block.Scan,
	}
	replayReq = decode.DecoderFrameWorkChromaCoeffBlockReconstruction(chromaCtx, chromaBlock)
	if replayReq.Block.Plane != 2 || replayReq.Transform != chromaReq.Transform || replayReq.Block.Block != chromaReq.Block.Block {
		t.Fatalf("chroma replay reconstruction=%+v", replayReq)
	}
	if err := decode.ReconstructDecoderFrameWorkChromaCoeffBlock(chromaBatch, 0, chromaCtx, chromaBlock); err != nil {
		t.Fatal(err)
	}
	publicReconstructDecoderFrameWorkBlockCoeffDirect(t, chromaBatch, wantV, chromaPlane, chromaX, chromaY, chromaReq)
	if !bytes.Equal(gotV.V.Pix, wantV.V.Pix) {
		t.Fatal("chroma replay reconstruction adapter did not match direct public reconstruction")
	}
}

func TestPublicDecodeAndReconstructDecoderFrameWorkBlockCoefficients(t *testing.T) {
	output := publicDecoderBlockCoeffFrame(t, av1.FrameFormat{Width: 64, Height: 64, BitDepth: 8, MonoChrome: true, Align: 64})
	fillPublicReconstructPlane(output.Y, output.Layout.BytesPerSample, 128)
	batch := publicDecoderBlockCoeffSimpleBatch(output)
	state, cdfs, transformCtx, coeffCtx, scratch, req := publicDecoderBlockCoeffDecodeDriver(t, batch, false)

	count := 0
	result, err := decode.DecodeAndReconstructDecoderFrameWorkBlockCoefficients(batch, 0, &state, cdfs, &transformCtx, &coeffCtx, &scratch, req, func(block av1.TileBlockCoeffBlock) error {
		count++
		if block.Plane != 0 || block.Block.Size != av1.TileTransformSize8x8 {
			t.Fatalf("block=%+v", block)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Luma.TXBs != 1 || result.TotalStats().TXBs != 1 || count != 1 {
		t.Fatalf("result=%+v total=%+v count=%d", result, result.TotalStats(), count)
	}

	state, cdfs, transformCtx, coeffCtx, scratch, req = publicDecoderBlockCoeffDecodeDriver(t, batch, true)
	if _, err := decode.DecodeAndReconstructDecoderFrameWorkBlockCoefficients(batch, 0, &state, cdfs, &transformCtx, &coeffCtx, &scratch, req, nil); !errors.Is(err, decode.ErrThreadingInvalidBatch) {
		t.Fatalf("short reconstruction scratch err=%v want %v", err, decode.ErrThreadingInvalidBatch)
	}
}

func TestPublicReconstructDecoderFrameWorkBlockCoeffRejectsInvalidInputs(t *testing.T) {
	output := publicDecoderBlockCoeffFrame(t, av1.FrameFormat{Width: 64, Height: 64, BitDepth: 8, MonoChrome: true, Align: 64})
	fillPublicReconstructPlane(output.Y, output.Layout.BytesPerSample, 128)
	batch := publicDecoderBlockCoeffSimpleBatch(output)
	req := publicDecoderBlockCoeffSimpleRequest()
	req.Int32Scratch, req.ResidualScratch = publicDecoderBlockCoeffScratch(t, batch, req, decode.DecoderFrameWorkPlaneY)

	invalidPlane := req
	invalidPlane.Block.Plane = 3
	if err := decode.ReconstructDecoderFrameWorkBlockCoeff(batch, 0, invalidPlane); !errors.Is(err, decode.ErrThreadingInvalidBatch) {
		t.Fatalf("invalid plane err=%v want %v", err, decode.ErrThreadingInvalidBatch)
	}
	if _, _, _, err := decode.DecoderFrameWorkBlockCoeffPlanePosition(batch, 0, req.Visit, invalidPlane.Block); !errors.Is(err, decode.ErrThreadingInvalidBatch) {
		t.Fatalf("invalid plane position err=%v want %v", err, decode.ErrThreadingInvalidBatch)
	}
	if _, _, err := decode.DecoderFrameWorkBlockQuantizer(batch, 32, 0, decode.DecoderFrameWorkPlane(99)); !errors.Is(err, decode.ErrThreadingInvalidBatch) {
		t.Fatalf("invalid quantizer plane err=%v want %v", err, decode.ErrThreadingInvalidBatch)
	}
	if _, _, err := decode.DecoderFrameWorkBlockQIndex(batch, 32, 1); !errors.Is(err, decode.ErrThreadingInvalidBatch) {
		t.Fatalf("disabled segment err=%v want %v", err, decode.ErrThreadingInvalidBatch)
	}
	if _, _, _, err := decode.DecoderFrameWorkBlockCoeffPlanePosition(batch, 1, req.Visit, req.Block); !errors.Is(err, decode.ErrThreadingInvalidBatch) {
		t.Fatalf("invalid job position err=%v want %v", err, decode.ErrThreadingInvalidBatch)
	}
}

func TestPublicReconstructDecoderFrameWorkBlockCoeffAllocs(t *testing.T) {
	output := publicDecoderBlockCoeffFrame(t, av1.FrameFormat{Width: 64, Height: 64, BitDepth: 8, MonoChrome: true, Align: 64})
	batch := publicDecoderBlockCoeffSimpleBatch(output)
	req := publicDecoderBlockCoeffSimpleRequest()
	req.Int32Scratch, req.ResidualScratch = publicDecoderBlockCoeffScratch(t, batch, req, decode.DecoderFrameWorkPlaneY)

	allocs := testing.AllocsPerRun(1000, func() {
		fillPublicReconstructPlane(output.Y, output.Layout.BytesPerSample, 128)
		if err := decode.ReconstructDecoderFrameWorkBlockCoeff(batch, 0, req); err != nil {
			t.Fatal(err)
		}
	})
	if allocs != 0 {
		t.Fatalf("allocs=%v want 0", allocs)
	}
}

func TestPublicReconstructDecoderFrameWorkCoeffReplayAdaptersAllocs(t *testing.T) {
	output := publicDecoderBlockCoeffFrame(t, av1.FrameFormat{Width: 64, Height: 64, BitDepth: 8, MonoChrome: true, Align: 64})
	batch := publicDecoderBlockCoeffSimpleBatch(output)
	req := publicDecoderBlockCoeffSimpleRequest()
	ctx := publicDecoderBlockCoeffReplayContext(t, batch, req, decode.DecoderFrameWorkPlaneY)
	block := av1.TileLumaCoeffBlock{
		Block:     req.Block.Block,
		Transform: req.Transform,
		Result:    req.Block.Result,
		Coeffs:    req.Block.Coeffs,
		Scan:      req.Block.Scan,
	}

	allocs := testing.AllocsPerRun(1000, func() {
		fillPublicReconstructPlane(output.Y, output.Layout.BytesPerSample, 128)
		if err := decode.ReconstructDecoderFrameWorkLumaCoeffBlock(batch, 0, ctx, block); err != nil {
			t.Fatal(err)
		}
	})
	if allocs != 0 {
		t.Fatalf("allocs=%v want 0", allocs)
	}
}

func TestPublicDecodeAndReconstructDecoderFrameWorkBlockCoefficientsAllocs(t *testing.T) {
	output := publicDecoderBlockCoeffFrame(t, av1.FrameFormat{Width: 64, Height: 64, BitDepth: 8, MonoChrome: true, Align: 64})
	batch := publicDecoderBlockCoeffSimpleBatch(output)
	var state av1.TileDecodeState
	var transformCDFs av1.TileTransformCDFs
	var coeffCDFs av1.TileCoeffCDFs
	cdfs := av1.TileBlockCoeffCDFs{Transform: &transformCDFs, Coeff: &coeffCDFs}
	var transformCtx av1.TileTransformContext
	var coeffCtx av1.TileCoeffEntropyContext
	var scratch av1.TileBlockCoeffScratch
	req := publicDecoderBlockCoeffDecodeRequest(t, batch, false)
	payload := make([]byte, 32)
	job := av1.TileJob{Offset: 0, Size: uint32(len(payload))}
	var err error

	allocs := testing.AllocsPerRun(1000, func() {
		err = av1.InitTileTransformCDFsDefault(&transformCDFs)
		if err != nil {
			return
		}
		err = av1.InitTileCoeffCDFsDefault(&coeffCDFs, 0)
		if err != nil {
			return
		}
		err = av1.ResetTileDecodeState(&state, payload, job, av1.TileDecodeOptions{})
		if err != nil {
			return
		}
		transformCtx.Reset()
		coeffCtx.Reset()
		fillPublicReconstructPlane(output.Y, output.Layout.BytesPerSample, 128)
		_, err = decode.DecodeAndReconstructDecoderFrameWorkBlockCoefficients(batch, 0, &state, cdfs, &transformCtx, &coeffCtx, &scratch, req, nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	if allocs != 0 {
		t.Fatalf("allocs=%v want 0", allocs)
	}
}

func publicDecoderBlockCoeffLumaFixture(tb publicDecoderBlockCoeffTB) (*av1.Frame, *av1.Frame, decode.DecoderFrameWorkBatch, decode.DecoderFrameWorkBlockCoeffReconstruction) {
	tb.Helper()
	format := av1.FrameFormat{Width: 160, Height: 128, BitDepth: 8, Align: 64}
	got := publicDecoderBlockCoeffFrame(tb, format)
	want := publicDecoderBlockCoeffFrame(tb, format)
	fillPublicReconstructPlane(got.Y, got.Layout.BytesPerSample, 128)
	fillPublicReconstructPlane(want.Y, want.Layout.BytesPerSample, 128)

	batch := decode.DecoderFrameWorkBatch{
		Output: got,
		FrameWorkFrameContext: decode.DecoderFrameWorkFrameContext{
			Sequence: decode.DecoderFrameWorkSequenceContextFromHeader(av1.SequenceHeader{
				ColorConfig: av1.ColorConfig{BitDepth: 8},
			}),
			FrameSize:    av1.FrameSize{CodedWidth: 160, Height: 128},
			Quantization: av1.QuantizationParams{BaseQIdx: 40},
			Segmentation: av1.SegmentationParams{Enabled: true},
		},
		Jobs: []av1.TileJob{{SBX: 1, SBY: 0, SBCols: 1, SBRows: 1}},
	}
	batch.Segmentation.Data.Segments[2].DeltaQ = 5
	coeffs := make([]int16, 16)
	coeffs[0] = 3
	req := decode.DecoderFrameWorkBlockCoeffReconstruction{
		Visit: av1.TileBlockVisit{
			MICol: 16, MIRow: 0, MIColEnd: 20, MIRowEnd: 4,
			X4: 0, Y4: 0, Size: av1.TileBlockSize16x16, VisibleW4: 4, VisibleH4: 4,
		},
		Block: av1.TileBlockCoeffBlock{
			Plane:  0,
			Block:  av1.TileTransformBlock{X4: 1, Y4: 2, Size: av1.TileTransformSize4x4, VisibleW4: 1, VisibleH4: 1},
			Result: av1.TileTXBDecodeResult{EOB: 1},
			Coeffs: coeffs,
		},
		Transform:     av1.TransformTypeDCTDCT,
		CurrentQIndex: 40,
		SegmentID:     2,
	}
	return got, want, batch, req
}

func publicDecoderBlockCoeffChromaFixture(tb publicDecoderBlockCoeffTB) (*av1.Frame, *av1.Frame, decode.DecoderFrameWorkBatch, decode.DecoderFrameWorkBlockCoeffReconstruction) {
	tb.Helper()
	format := av1.FrameFormat{
		Width:        160,
		Height:       160,
		BitDepth:     8,
		SubsamplingX: true,
		SubsamplingY: true,
		Align:        64,
	}
	got := publicDecoderBlockCoeffFrame(tb, format)
	want := publicDecoderBlockCoeffFrame(tb, format)
	publicFillDecoderBlockCoeffFrame(got, 96)
	publicFillDecoderBlockCoeffFrame(want, 96)

	batch := decode.DecoderFrameWorkBatch{
		Output: got,
		FrameWorkFrameContext: decode.DecoderFrameWorkFrameContext{
			Sequence: decode.DecoderFrameWorkSequenceContextFromHeader(av1.SequenceHeader{
				ColorConfig: av1.ColorConfig{BitDepth: 8, SubsamplingX: true, SubsamplingY: true},
			}),
			FrameSize:    av1.FrameSize{CodedWidth: 160, Height: 160},
			Quantization: av1.QuantizationParams{BaseQIdx: 64, VDCDelta: -1, VACDelta: 2},
		},
		Jobs: []av1.TileJob{{SBX: 1, SBY: 1, SBCols: 1, SBRows: 1}},
	}
	coeffs := make([]int16, 16)
	coeffs[0] = -2
	req := decode.DecoderFrameWorkBlockCoeffReconstruction{
		Visit: av1.TileBlockVisit{
			MICol: 16, MIRow: 16, MIColEnd: 24, MIRowEnd: 24,
			X4: 0, Y4: 0, Size: av1.TileBlockSize32x32, VisibleW4: 8, VisibleH4: 8,
		},
		Block: av1.TileBlockCoeffBlock{
			Plane:  2,
			Block:  av1.TileTransformBlock{X4: 1, Y4: 2, Size: av1.TileTransformSize4x4, VisibleW4: 1, VisibleH4: 1},
			Result: av1.TileTXBDecodeResult{EOB: 1},
			Coeffs: coeffs,
		},
		Transform:     av1.TransformTypeDCTDCT,
		CurrentQIndex: 64,
	}
	return got, want, batch, req
}

func publicDecoderBlockCoeffSimpleBatch(output *av1.Frame) decode.DecoderFrameWorkBatch {
	return decode.DecoderFrameWorkBatch{
		Output: output,
		FrameWorkFrameContext: decode.DecoderFrameWorkFrameContext{
			Sequence: decode.DecoderFrameWorkSequenceContextFromHeader(av1.SequenceHeader{
				ColorConfig: av1.ColorConfig{BitDepth: 8, MonoChrome: true},
			}),
			FrameSize:    av1.FrameSize{CodedWidth: 64, Height: 64},
			Quantization: av1.QuantizationParams{BaseQIdx: 32},
		},
		Jobs: []av1.TileJob{{SBCols: 1, SBRows: 1}},
	}
}

func publicDecoderBlockCoeffSimpleRequest() decode.DecoderFrameWorkBlockCoeffReconstruction {
	coeffs := make([]int16, 16)
	coeffs[0] = 2
	return decode.DecoderFrameWorkBlockCoeffReconstruction{
		Visit: av1.TileBlockVisit{
			MICol: 0, MIRow: 0, MIColEnd: 4, MIRowEnd: 4,
			X4: 0, Y4: 0, Size: av1.TileBlockSize16x16, VisibleW4: 4, VisibleH4: 4,
		},
		Block: av1.TileBlockCoeffBlock{
			Plane:  0,
			Block:  av1.TileTransformBlock{X4: 0, Y4: 0, Size: av1.TileTransformSize4x4, VisibleW4: 1, VisibleH4: 1},
			Result: av1.TileTXBDecodeResult{EOB: 1},
			Coeffs: coeffs,
		},
		Transform:     av1.TransformTypeDCTDCT,
		CurrentQIndex: 32,
	}
}

func publicDecoderBlockCoeffDecodeDriver(tb publicDecoderBlockCoeffTB, batch decode.DecoderFrameWorkBatch, shortScratch bool) (av1.TileDecodeState, av1.TileBlockCoeffCDFs, av1.TileTransformContext, av1.TileCoeffEntropyContext, av1.TileBlockCoeffScratch, decode.DecoderFrameWorkBlockCoeffDecodeRequest) {
	tb.Helper()
	var transformCDFs av1.TileTransformCDFs
	if err := av1.InitTileTransformCDFsDefault(&transformCDFs); err != nil {
		tb.Fatalf("InitTileTransformCDFsDefault err=%v", err)
	}
	var coeffCDFs av1.TileCoeffCDFs
	if err := av1.InitTileCoeffCDFsDefault(&coeffCDFs, 0); err != nil {
		tb.Fatalf("InitTileCoeffCDFsDefault err=%v", err)
	}
	var state av1.TileDecodeState
	payload := make([]byte, 32)
	if err := av1.ResetTileDecodeState(&state, payload, av1.TileJob{Offset: 0, Size: uint32(len(payload))}, av1.TileDecodeOptions{}); err != nil {
		tb.Fatalf("ResetTileDecodeState err=%v", err)
	}
	req := publicDecoderBlockCoeffDecodeRequest(tb, batch, shortScratch)
	return state, av1.TileBlockCoeffCDFs{Transform: &transformCDFs, Coeff: &coeffCDFs}, av1.TileTransformContext{}, av1.TileCoeffEntropyContext{}, av1.TileBlockCoeffScratch{}, req
}

func publicDecoderBlockCoeffDecodeRequest(tb publicDecoderBlockCoeffTB, batch decode.DecoderFrameWorkBatch, shortScratch bool) decode.DecoderFrameWorkBlockCoeffDecodeRequest {
	tb.Helper()
	visit := av1.TileBlockVisit{
		MICol: 0, MIRow: 0, MIColEnd: 2, MIRowEnd: 2,
		X4: 0, Y4: 0, Size: av1.TileBlockSize8x8, VisibleW4: 2, VisibleH4: 2,
	}
	reconstructReq := decode.DecoderFrameWorkBlockCoeffReconstruction{
		Visit: visit,
		Block: av1.TileBlockCoeffBlock{
			Plane: 0,
			Block: av1.TileTransformBlock{X4: 0, Y4: 0, Size: av1.TileTransformSize8x8, VisibleW4: 2, VisibleH4: 2},
		},
		Transform:     av1.TransformTypeDCTDCT,
		CurrentQIndex: 32,
	}
	ctx := publicDecoderBlockCoeffReplayContext(tb, batch, reconstructReq, decode.DecoderFrameWorkPlaneY)
	if shortScratch {
		ctx.Int32Scratch = nil
	}
	return decode.DecoderFrameWorkBlockCoeffDecodeRequest{
		Decode: av1.TileBlockCoeffRequest{
			Transform: av1.TileTransformTreeRequest{
				Size:          av1.TileBlockSize8x8,
				VisibleW4:     2,
				VisibleH4:     2,
				Color:         av1.ColorConfig{MonoChrome: true},
				Inter:         true,
				TransformMode: av1.TransformModeLargest,
			},
			LumaType: av1.TransformTypeDCTDCT,
		},
		Reconstruction: ctx,
	}
}

func publicDecoderBlockCoeffReplayContext(tb publicDecoderBlockCoeffTB, batch decode.DecoderFrameWorkBatch, req decode.DecoderFrameWorkBlockCoeffReconstruction, plane decode.DecoderFrameWorkPlane) decode.DecoderFrameWorkCoeffReconstructionContext {
	tb.Helper()
	int32Scratch, residualScratch := publicDecoderBlockCoeffScratch(tb, batch, req, plane)
	return decode.DecoderFrameWorkCoeffReconstructionContext{
		Visit:           req.Visit,
		CurrentQIndex:   req.CurrentQIndex,
		SegmentID:       req.SegmentID,
		Int32Scratch:    int32Scratch,
		ResidualScratch: residualScratch,
	}
}

func publicDecoderBlockCoeffScratch(tb publicDecoderBlockCoeffTB, batch decode.DecoderFrameWorkBatch, req decode.DecoderFrameWorkBlockCoeffReconstruction, plane decode.DecoderFrameWorkPlane) ([]int32, []int16) {
	tb.Helper()
	int32Len, int16Len, err := decode.DecoderFrameWorkResidualMaxScratchLen(batch, req.CurrentQIndex, req.SegmentID, plane)
	if err != nil {
		tb.Fatalf("DecoderFrameWorkResidualMaxScratchLen err=%v", err)
	}
	return make([]int32, int32Len), make([]int16, int16Len)
}

func publicReconstructDecoderFrameWorkBlockCoeffDirect(tb publicDecoderBlockCoeffTB, batch decode.DecoderFrameWorkBatch, dst *av1.Frame, plane decode.DecoderFrameWorkPlane, x int, y int, req decode.DecoderFrameWorkBlockCoeffReconstruction) {
	tb.Helper()
	size, err := av1.TileTransformSizePixels(req.Block.Block.Size)
	if err != nil {
		tb.Fatalf("TileTransformSizePixels err=%v", err)
	}
	scanSize, err := av1.TransformScanSize(size)
	if err != nil {
		tb.Fatalf("TransformScanSize err=%v", err)
	}
	q, lossless, err := decode.DecoderFrameWorkBlockQuantizer(batch, req.CurrentQIndex, req.SegmentID, plane)
	if err != nil {
		tb.Fatalf("DecoderFrameWorkBlockQuantizer err=%v", err)
	}
	cfg := av1.ReconstructBlock{
		Size:      size,
		Transform: req.Transform,
		Quantizer: q,
		Lossless:  lossless,
		EOB:       int16(req.Block.Result.EOB),
	}
	int32Len, int16Len, err := av1.ReconstructBlockScratchLen(cfg)
	if err != nil {
		tb.Fatalf("ReconstructBlockScratchLen err=%v", err)
	}
	if err := av1.ReconstructPlaneBlock(publicDecoderBlockCoeffPlane(dst, plane), dst.Layout.BytesPerSample, batch.Sequence.ColorConfig.BitDepth, x, y, req.Block.Coeffs, int(scanSize.Height), make([]int32, int32Len), make([]int16, int16Len), cfg); err != nil {
		tb.Fatalf("ReconstructPlaneBlock err=%v", err)
	}
}

func publicDecoderBlockCoeffFrame(tb publicDecoderBlockCoeffTB, format av1.FrameFormat) *av1.Frame {
	tb.Helper()
	layout, err := av1.FrameRequiredSize(format)
	if err != nil {
		tb.Fatalf("FrameRequiredSize err=%v", err)
	}
	frame, err := av1.BindFrame(make([]byte, layout.Size), format)
	if err != nil {
		tb.Fatalf("BindFrame err=%v", err)
	}
	return &frame
}

func publicFillDecoderBlockCoeffFrame(frame *av1.Frame, sample uint16) {
	fillPublicReconstructPlane(frame.Y, frame.Layout.BytesPerSample, sample)
	if len(frame.U.Pix) != 0 {
		fillPublicReconstructPlane(frame.U, frame.Layout.BytesPerSample, sample)
	}
	if len(frame.V.Pix) != 0 {
		fillPublicReconstructPlane(frame.V, frame.Layout.BytesPerSample, sample)
	}
}

func publicDecoderBlockCoeffPlane(frame *av1.Frame, plane decode.DecoderFrameWorkPlane) av1.FramePlane {
	switch plane {
	case decode.DecoderFrameWorkPlaneY:
		return frame.Y
	case decode.DecoderFrameWorkPlaneU:
		return frame.U
	case decode.DecoderFrameWorkPlaneV:
		return frame.V
	default:
		return av1.FramePlane{}
	}
}
