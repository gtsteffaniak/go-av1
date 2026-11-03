package decode_test

import (
	"errors"
	av1 "github.com/gtsteffaniak/go-av1/pkg/av1"
	decode "github.com/gtsteffaniak/go-av1/pkg/decode"
	"testing"
)

func FuzzPublicDecodeAndReconstructDecoderFrameWorkJobResiduals(f *testing.F) {
	f.Add([]byte{0x00, 0x00, 0x00, 0x00}, uint8(64), false)
	f.Add([]byte{0xff, 0x80, 0x00, 0x7f}, uint8(3), true)
	f.Add([]byte{0xaa, 0x55, 0x11, 0xee, 0x00}, uint8(127), false)

	f.Fuzz(func(t *testing.T, payload []byte, rawQ uint8, sideMaps bool) {
		if len(payload) == 0 || len(payload) > 128 {
			return
		}
		qIndex := rawQ | 1
		output := publicDecoderPostFilterFrame(t, av1.FrameFormat{Width: 64, Height: 64, BitDepth: 8, MonoChrome: true, Align: 64})
		publicFillDecoderPostFilterPlane(output.Y)
		batch := decode.DecoderFrameWorkBatch{
			Output:  output,
			Payload: payload,
			FrameWorkFrameContext: decode.DecoderFrameWorkFrameContext{
				Sequence: decode.DecoderFrameWorkSequenceContextFromHeader(av1.SequenceHeader{
					ColorConfig: av1.ColorConfig{BitDepth: 8, MonoChrome: true},
				}),
				FrameSize:    av1.FrameSize{CodedWidth: 64, UpscaledWidth: 64, Height: 64, SuperResDenominator: 8},
				Quantization: av1.QuantizationParams{BaseQIdx: qIndex},
				TransformRef: av1.TransformReferenceParams{TransformMode: av1.TransformModeLargest},
			},
			Jobs: []av1.TileJob{{SBCols: 1, SBRows: 1, Offset: 0, Size: uint32(len(payload))}},
		}
		if sideMaps {
			publicBindResidualFuzzSideMaps(t, &batch)
		}

		var state av1.TileDecodeState
		if err := decode.InitDecoderFrameWorkJobDecodeState(batch, 0, &state); err != nil {
			t.Fatal(err)
		}
		var storage decode.DecoderFrameWorkTileResidualCDFStorage
		if err := decode.InitDecoderFrameWorkTileResidualCDFStorageDefault(&storage, qIndex); err != nil {
			t.Fatal(err)
		}
		loopReq, err := decode.DecoderFrameWorkJobBlockLoopRequest(batch, 0, nil, nil, 0, nil)
		if err != nil {
			t.Fatal(err)
		}
		int32Len, int16Len, err := decode.DecoderFrameWorkResidualMaxScratchLen(batch, qIndex, 0, decode.DecoderFrameWorkPlaneY)
		if err != nil {
			t.Fatal(err)
		}

		var scratch decode.DecoderFrameWorkTileResidualScratch
		req := decode.DecoderFrameWorkTileResidualRequest{
			Loop:          loopReq,
			TransformMode: batch.TransformRef.TransformMode,
			Transforms: func(visit av1.TileBlockLoopVisit) (decode.DecoderFrameWorkBlockTransforms, error) {
				return decode.ReadDecoderFrameWorkBlockTransforms(batch, &state, visit)
			},
			Int32Scratch:    make([]int32, int32Len),
			ResidualScratch: make([]int16, int16Len),
		}
		stats, err := decode.DecodeAndReconstructDecoderFrameWorkJobResiduals(batch, 0, &state, decode.DecoderFrameWorkTileResidualCDFsFromStorage(&storage), &scratch, req)
		if err != nil {
			return
		}
		if stats.TXBs != stats.NonZero+stats.AllZero {
			t.Fatalf("residual stats=%+v inconsistent txb counts", stats)
		}
	})
}

func FuzzPublicDecodeAndRetainDecoderFrameWorkBatchResiduals(f *testing.F) {
	f.Add([]byte{0x00, 0x00, 0x00, 0x00}, uint8(64), true, false)
	f.Add([]byte{0xff, 0x80, 0x00, 0x7f}, uint8(3), false, true)
	f.Add([]byte{0xaa, 0x55, 0x11, 0xee, 0x00, 0x42}, uint8(127), true, true)

	f.Fuzz(func(t *testing.T, payload []byte, rawQ uint8, updateContext bool, sideMaps bool) {
		if len(payload) < 2 || len(payload) > 128 {
			return
		}
		qIndex := rawQ | 1
		output := publicDecoderPostFilterFrame(t, av1.FrameFormat{Width: 128, Height: 64, BitDepth: 8, MonoChrome: true, Align: 64})
		publicFillDecoderPostFilterPlane(output.Y)
		firstSize := len(payload) / 2
		batch := decode.DecoderFrameWorkBatch{
			Output:  output,
			Payload: payload,
			FrameWorkFrameContext: decode.DecoderFrameWorkFrameContext{
				Sequence: decode.DecoderFrameWorkSequenceContextFromHeader(av1.SequenceHeader{
					ColorConfig: av1.ColorConfig{BitDepth: 8, MonoChrome: true},
				}),
				FrameSize:    av1.FrameSize{CodedWidth: 128, UpscaledWidth: 128, Height: 64, SuperResDenominator: 8},
				Quantization: av1.QuantizationParams{BaseQIdx: qIndex},
				TransformRef: av1.TransformReferenceParams{TransformMode: av1.TransformModeLargest},
			},
			Jobs: []av1.TileJob{
				{SBX: 0, SBY: 0, SBCols: 1, SBRows: 1, Offset: 0, Size: uint32(firstSize)},
				{SBX: 1, SBY: 0, SBCols: 1, SBRows: 1, Offset: uint32(firstSize), Size: uint32(len(payload) - firstSize), UpdatesFrameContext: updateContext},
			},
		}
		var retained decode.DecoderFrameWorkTileResidualCDFStorage
		retainedValid := false
		batch.RetainedTileResidualCDFs = &retained
		batch.RetainedTileResidualCDFsValid = &retainedValid
		if sideMaps {
			publicBindResidualFuzzSideMaps(t, &batch)
		}

		var storage decode.DecoderFrameWorkTileResidualCDFStorage
		if err := decode.InitDecoderFrameWorkTileResidualCDFStorageDefault(&storage, qIndex); err != nil {
			t.Fatal(err)
		}
		int32Len, int16Len, err := decode.DecoderFrameWorkResidualMaxScratchLen(batch, qIndex, 0, decode.DecoderFrameWorkPlaneY)
		if err != nil {
			t.Fatal(err)
		}
		loopContextLen, err := decode.DecoderFrameWorkBatchResidualLoopContextAboveLen(batch)
		if err != nil {
			t.Fatal(err)
		}

		var state av1.TileDecodeState
		var scratch decode.DecoderFrameWorkTileResidualScratch
		req := decode.DecoderFrameWorkBatchResidualRequest{
			Tile: decode.DecoderFrameWorkTileResidualRequest{
				TransformMode: batch.TransformRef.TransformMode,
				Transforms: func(visit av1.TileBlockLoopVisit) (decode.DecoderFrameWorkBlockTransforms, error) {
					return decode.ReadDecoderFrameWorkBlockTransforms(batch, &state, visit)
				},
				Int32Scratch:    make([]int32, int32Len),
				ResidualScratch: make([]int16, int16Len),
			},
			LoopContextAbove: make([]av1.TileBlockLoopRootAboveContext, loopContextLen),
		}
		stats, err := decode.DecodeAndRetainDecoderFrameWorkBatchResiduals(batch, &state, &storage, &scratch, req)
		if err != nil {
			return
		}
		if stats.TXBs != stats.NonZero+stats.AllZero {
			t.Fatalf("batch residual stats=%+v inconsistent txb counts", stats)
		}
		if retainedValid != updateContext {
			t.Fatalf("retained=%v want %v", retainedValid, updateContext)
		}
	})
}

func FuzzPublicDecoderFrameWorkBatchResidualRunnerSideData(f *testing.F) {
	f.Add(uint8(1), uint8(0), false, false, false)
	f.Add(uint8(2), uint8(1), true, false, false)
	f.Add(uint8(4), uint8(2), true, true, false)
	f.Add(uint8(3), uint8(3), false, false, true)

	f.Fuzz(func(t *testing.T, rawWorkers uint8, rawCDEFBits uint8, activeRestoration bool, omitRestorationScratch bool, corruptLoopMap bool) {
		workers := int(rawWorkers%4) + 1
		sequence := av1.SequenceHeader{
			EnableCDEF:        true,
			EnableRestoration: true,
			ColorConfig: av1.ColorConfig{
				BitDepth:   8,
				MonoChrome: true,
			},
		}
		size := av1.FrameSize{CodedWidth: 128, UpscaledWidth: 128, Height: 64, SuperResDenominator: 8}
		cdefBits := rawCDEFBits & 3
		cdef := av1.CDEFParams{
			Bits:          cdefBits,
			StrengthCount: uint8(1) << cdefBits,
		}
		restoration := av1.RestorationParams{}
		if activeRestoration {
			restoration = av1.RestorationParams{
				Type:      [3]av1.RestorationType{av1.RestorationWiener},
				UnitSizeY: 64,
			}
		}

		batch := decode.DecoderFrameWorkBatch{
			FrameWorkFrameContext: decode.DecoderFrameWorkFrameContext{
				Sequence:     decode.DecoderFrameWorkSequenceContextFromHeader(sequence),
				FrameSize:    size,
				CDEF:         cdef,
				Restoration:  restoration,
				Quantization: av1.QuantizationParams{BaseQIdx: 64},
				TransformRef: av1.TransformReferenceParams{TransformMode: av1.TransformModeLargest},
			},
			Jobs: []av1.TileJob{
				{SBX: 0, SBY: 0, SBCols: 1, SBRows: 1, Offset: 0, Size: 1},
				{SBX: 1, SBY: 0, SBCols: 1, SBRows: 1, Offset: 1, Size: 1},
			},
		}
		runnerSize, err := decode.DecoderFrameWorkBatchResidualRunnerScratchLen(batch, workers)
		if err != nil {
			t.Fatal(err)
		}
		runnerScratch := publicDecoderBatchResidualRunnerScratch(runnerSize)
		if omitRestorationScratch {
			runnerScratch.RestorationRequests = nil
		}
		runner, err := decode.BindDecoderFrameWorkBatchResidualRunner(runnerSize, runnerScratch)
		if err != nil {
			t.Fatal(err)
		}

		sideSize, err := decode.DecoderFrameWorkSideDataScratchLen(sequence, size, cdef, restoration)
		if err != nil {
			t.Fatal(err)
		}
		side, err := decode.BindDecoderFrameWorkSideData(sequence, size, cdef, restoration, decode.DecoderFrameWorkSideDataScratch{
			CDEFIndexMap:             make([]uint8, sideSize.CDEFIndexMap),
			CDEFReadMap:              make([]bool, sideSize.CDEFReadMap),
			LoopFilterMap:            make([]decode.DecoderFrameWorkLoopFilterBlockRecord, sideSize.LoopFilterMap),
			RestorationRecords:       make([]av1.TileRestorationUnitRecord, sideSize.RestorationRecords),
			RestorationBoundaryAbove: make([]uint16, sideSize.RestorationBoundaryAbove),
			RestorationBoundaryBelow: make([]uint16, sideSize.RestorationBoundaryBelow),
		})
		if err != nil {
			t.Fatal(err)
		}
		side.CDEFIndexMap.Read[0] = true
		side.LoopFilterMap.Records[0].Valid = true
		if len(side.RestorationFrameBuffers.Records[0]) != 0 {
			side.RestorationFrameBuffers.Records[0][0].Index = 99
		}
		if corruptLoopMap {
			side.LoopFilterMap = decode.DecoderFrameWorkLoopFilterMap{Stride: 1, Rows: 1}
		}

		err = decode.SetDecoderFrameWorkBatchResidualRunnerSideData(&runner, side)
		if activeRestoration && omitRestorationScratch {
			if !errors.Is(err, av1.ErrFrameShortBuffer) {
				t.Fatalf("missing restoration requests err=%v want %v", err, av1.ErrFrameShortBuffer)
			}
			return
		}
		if corruptLoopMap {
			if !errors.Is(err, decode.ErrThreadingInvalidBatch) {
				t.Fatalf("corrupt loop map err=%v want %v", err, decode.ErrThreadingInvalidBatch)
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		if side.CDEFIndexMap.Read[0] || side.LoopFilterMap.Records[0].Valid {
			t.Fatalf("side data was not reset: cdef=%v loop=%+v", side.CDEFIndexMap.Read[0], side.LoopFilterMap.Records[0])
		}
		if runner.CDEFIndexMap.Stride != side.CDEFIndexMap.Stride ||
			runner.LoopFilterMap.Stride != side.LoopFilterMap.Stride {
			t.Fatalf("runner side maps cdef=%+v loop=%+v side=%+v", runner.CDEFIndexMap, runner.LoopFilterMap, side)
		}
		if activeRestoration {
			if len(runner.RestorationRequests) < workers ||
				!runner.RestorationRequests[0].Buffers.Plan.Active {
				t.Fatalf("runner restoration requests=%+v workers=%d", runner.RestorationRequests, workers)
			}
		}
	})
}

func FuzzPublicRunDecoderFrameWorkEventWithResidualRunner(f *testing.F) {
	f.Add([]byte{0x00, 0x00}, uint8(64), uint8(0), true)
	f.Add([]byte{0xff, 0x80, 0x00, 0x7f}, uint8(3), uint8(1), false)
	f.Add([]byte{0xaa, 0x55, 0x11, 0xee, 0x00, 0x42}, uint8(127), uint8(2), true)

	f.Fuzz(func(t *testing.T, payload []byte, rawQ uint8, rawCDEFBits uint8, sideData bool) {
		if len(payload) < 2 || len(payload) > 256 {
			return
		}
		firstSize := len(payload) / 2
		if firstSize == 0 || firstSize > 256 {
			return
		}
		tilePayload := make([]byte, len(payload)+1)
		tilePayload[0] = byte(firstSize - 1)
		copy(tilePayload[1:], payload[:firstSize])
		copy(tilePayload[1+firstSize:], payload[firstSize:])

		sequence := av1.SequenceHeader{
			EnableCDEF: true,
			ColorConfig: av1.ColorConfig{
				BitDepth:   8,
				MonoChrome: true,
			},
		}
		cdefBits := rawCDEFBits & 3
		cdef := av1.CDEFParams{
			Bits:          cdefBits,
			StrengthCount: uint8(1) << cdefBits,
		}
		event := publicDecoderResidualRunnerFrameEvent()
		event.Unit.Payload = tilePayload
		event.CDEF = cdef
		event.Quantization.BaseQIdx = rawQ | 1

		var scratchSpans [2]av1.TileSpan
		var scratchJobs [2]av1.TileJob
		var scratchBatches [1]decode.TileBatch
		eventScratch, err := decode.DecoderFrameWorkResidualEventScratchLen(sequence, &event, 1, scratchSpans[:], scratchJobs[:], scratchBatches[:])
		if err != nil {
			t.Fatal(err)
		}
		runner, err := decode.BindDecoderFrameWorkBatchResidualRunner(eventScratch.Runner, publicDecoderBatchResidualRunnerScratch(eventScratch.Runner))
		if err != nil {
			t.Fatal(err)
		}

		var side *decode.DecoderFrameWorkSideData
		var boundSide decode.DecoderFrameWorkSideData
		if sideData {
			boundSide, err = decode.BindDecoderFrameWorkResidualEventSideData(sequence, &event, publicDecoderFrameWorkSideDataScratch(eventScratch.SideData))
			if err != nil {
				t.Fatal(err)
			}
			side = &boundSide
		}

		workerPool, err := decode.NewTileWorkerPool(1)
		if err != nil {
			t.Fatal(err)
		}
		defer workerPool.Close()

		pool := publicDecoderPostFilterFramePool(t, av1.FrameFormat{
			Width:      int(event.FrameSize.CodedWidth),
			Height:     int(event.FrameSize.Height),
			BitDepth:   8,
			MonoChrome: true,
			Align:      64,
		}, 1)
		var refs decode.DecoderSurfaceReferences
		var state decode.DecoderFrameWorkState
		var referenceSurfaces [av1.InterRefsPerFrame]int
		var referenceFrames [av1.InterRefsPerFrame]*av1.Frame
		var spans [2]av1.TileSpan
		var jobs [2]av1.TileJob
		var batches [1]decode.TileBatch
		var releases [av1.RefFrames]int

		result, err := decode.RunDecoderFrameWorkEventWithResidualRunner(decode.DecoderFrameWorkResidualEventRequest{
			State:             &state,
			Refs:              &refs,
			FramePool:         &pool,
			Sequence:          sequence,
			Event:             event,
			Align:             64,
			ReferenceSurfaces: referenceSurfaces[:],
			ReferenceFrames:   referenceFrames[:],
			Workers:           1,
			Spans:             spans[:],
			Jobs:              jobs[:],
			Batches:           batches[:],
			Releases:          releases[:],
			WorkerPool:        workerPool,
			Runner:            &runner,
			SideData:          side,
		})
		if err != nil {
			return
		}
		if result.Run != (decode.DecoderFrameWorkStepResult{ExecutedTileWork: true, CompletedFrame: true}) ||
			result.Output == nil ||
			state.Active() {
			t.Fatalf("event runner result=%+v output=%p active=%v", result, result.Output, state.Active())
		}
		if _, ok := refs.ReferenceSlot(0); !ok {
			t.Fatal("event runner did not publish decoded key frame")
		}
	})
}

func publicBindResidualFuzzSideMaps(t *testing.T, batch *decode.DecoderFrameWorkBatch) {
	t.Helper()
	batch.CDEF = av1.CDEFParams{Bits: 0, StrengthCount: 1}
	sequence := av1.SequenceHeader{
		Use128x128Superblock: batch.Sequence.Use128x128Superblock,
		ColorConfig:          batch.Sequence.ColorConfig,
	}
	_, _, cdefLength, err := decode.DecoderFrameWorkCDEFIndexMapShape(sequence, batch.FrameSize)
	if err != nil {
		t.Fatal(err)
	}
	cdefMap, err := decode.BindDecoderFrameWorkCDEFIndexMap(sequence, batch.FrameSize, batch.CDEF, make([]uint8, cdefLength), make([]bool, cdefLength))
	if err != nil {
		t.Fatal(err)
	}
	_, _, loopLength, err := decode.DecoderFrameWorkLoopFilterMapShape(sequence, batch.FrameSize)
	if err != nil {
		t.Fatal(err)
	}
	loopMap, err := decode.BindDecoderFrameWorkLoopFilterMap(sequence, batch.FrameSize, make([]decode.DecoderFrameWorkLoopFilterBlockRecord, loopLength))
	if err != nil {
		t.Fatal(err)
	}
	batch.CDEFIndexMap = &cdefMap
	batch.LoopFilterMap = &loopMap
}
