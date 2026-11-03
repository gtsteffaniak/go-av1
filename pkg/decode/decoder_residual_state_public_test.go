package decode_test

import (
	"bytes"
	"errors"
	av1 "github.com/gtsteffaniak/go-av1/pkg/av1"
	decode "github.com/gtsteffaniak/go-av1/pkg/decode"
	rtp "github.com/gtsteffaniak/go-av1/pkg/rtp"
	"testing"
)

type publicDecoderResidualEventPostRunner struct {
	calls  int
	side   bool
	output *av1.Frame
	err    error
	sample byte
	active bool
	seq    av1.SequenceHeader
}

func (r *publicDecoderResidualEventPostRunner) Apply(ctx decode.DecoderFrameWorkPostFilterContext) error {
	r.calls++
	r.side = ctx.CDEFIndexMap != nil && ctx.LoopFilterMap != nil && ctx.RestorationFrameBuffers != nil
	r.output = ctx.Output
	r.active = ctx.ExecutedTileWork && ctx.Step.Kind == decode.DecoderFrameWorkStepBegin
	r.seq = ctx.Event.SequenceHeader
	if ctx.Output != nil {
		ctx.Output.Y.Pix[0] = r.sample
	}
	return r.err
}

func TestPublicDecoderTileResidualCDFStorageLifecycle(t *testing.T) {
	var initial decode.DecoderFrameWorkTileResidualCDFStorage
	if err := decode.InitDecoderFrameWorkTileResidualCDFStorageDefault(&initial, 64); err != nil {
		t.Fatal(err)
	}
	if err := initial.DeltaQ.Update(2); err != nil {
		t.Fatal(err)
	}
	initialCDFs := decode.DecoderFrameWorkTileResidualCDFsFromStorage(&initial)
	if initialCDFs.Loop.Partition == nil || initialCDFs.Coeff.Coeff == nil || initialCDFs.TransformType == nil {
		t.Fatalf("initial CDF view=%+v", initialCDFs)
	}

	var retained decode.DecoderFrameWorkTileResidualCDFStorage
	retainedValid := false
	batch := decode.DecoderFrameWorkBatch{
		Payload: []byte{0x00, 0xff},
		FrameWorkFrameContext: decode.DecoderFrameWorkFrameContext{
			Quantization: av1.QuantizationParams{BaseQIdx: 91},
		},
		InitialTileResidualCDFs:       &initial,
		RetainedTileResidualCDFs:      &retained,
		RetainedTileResidualCDFsValid: &retainedValid,
		Jobs: []av1.TileJob{
			{Tile: 0, Offset: 0, Size: 1},
			{Tile: 1, Offset: 1, Size: 1, UpdatesFrameContext: true},
		},
	}

	var storage decode.DecoderFrameWorkTileResidualCDFStorage
	if err := decode.InitDecoderFrameWorkTileResidualCDFStorage(batch, &storage); err != nil {
		t.Fatal(err)
	}
	if !publicCDFValuesEqual(storage.DeltaQ.Values(), initial.DeltaQ.Values()) {
		t.Fatalf("storage delta q=%v want %v", storage.DeltaQ.Values(), initial.DeltaQ.Values())
	}
	if err := storage.DeltaQ.Update(1); err != nil {
		t.Fatal(err)
	}

	var state av1.TileDecodeState
	if err := decode.InitDecoderFrameWorkJobDecodeState(batch, 0, &state); err != nil {
		t.Fatal(err)
	}
	if state.RetainFrameContext || state.CurrentBaseQIdx != 91 {
		t.Fatalf("state=%+v", state)
	}
	if err := decode.RetainDecoderFrameWorkTileResidualCDFStorage(batch, 0, &state, &storage); err != nil {
		t.Fatal(err)
	}
	if retainedValid {
		t.Fatal("non-update tile retained frame context")
	}

	if err := decode.InitDecoderFrameWorkJobDecodeState(batch, 1, &state); err != nil {
		t.Fatal(err)
	}
	if !state.RetainFrameContext || state.CurrentBaseQIdx != 91 {
		t.Fatalf("update state=%+v", state)
	}
	if err := decode.RetainDecoderFrameWorkTileResidualCDFStorage(batch, 1, &state, &storage); err != nil {
		t.Fatal(err)
	}
	if !retainedValid {
		t.Fatal("update tile did not retain frame context")
	}
	if !publicCDFValuesEqual(retained.DeltaQ.Values(), storage.DeltaQ.Values()) {
		t.Fatalf("retained delta q=%v want %v", retained.DeltaQ.Values(), storage.DeltaQ.Values())
	}

	batch.InitialTileResidualCDFs = nil
	if err := decode.InitDecoderFrameWorkTileResidualCDFStorage(batch, &storage); err != nil {
		t.Fatal(err)
	}
	if publicCDFValuesEqual(storage.DeltaQ.Values(), initial.DeltaQ.Values()) {
		t.Fatal("default fallback unexpectedly reused initial frame context")
	}
}

func TestPublicDecoderTileResidualStateRejectsInvalidInputs(t *testing.T) {
	batch := decode.DecoderFrameWorkBatch{
		Payload: []byte{0xaa},
		Jobs:    []av1.TileJob{{Offset: 0, Size: 2}},
	}
	var state av1.TileDecodeState
	if err := decode.InitDecoderFrameWorkTileResidualCDFStorage(batch, nil); !errors.Is(err, decode.ErrThreadingInvalidBatch) {
		t.Fatalf("nil cdf storage err=%v want %v", err, decode.ErrThreadingInvalidBatch)
	}
	if err := decode.InitDecoderFrameWorkJobDecodeState(batch, -1, &state); !errors.Is(err, decode.ErrThreadingInvalidBatch) {
		t.Fatalf("negative index err=%v want %v", err, decode.ErrThreadingInvalidBatch)
	}
	if err := decode.InitDecoderFrameWorkJobDecodeState(batch, 0, nil); !errors.Is(err, decode.ErrThreadingInvalidBatch) {
		t.Fatalf("nil state err=%v want %v", err, decode.ErrThreadingInvalidBatch)
	}
	if err := decode.InitDecoderFrameWorkJobDecodeState(batch, 0, &state); !errors.Is(err, decode.ErrTileInvalidPlan) {
		t.Fatalf("invalid range err=%v want %v", err, decode.ErrTileInvalidPlan)
	}
	var storage decode.DecoderFrameWorkTileResidualCDFStorage
	if err := decode.RetainDecoderFrameWorkTileResidualCDFStorage(batch, 0, &state, nil); !errors.Is(err, decode.ErrThreadingInvalidBatch) {
		t.Fatalf("nil retain storage err=%v want %v", err, decode.ErrThreadingInvalidBatch)
	}
	if err := decode.InitDecoderFrameWorkTileResidualCDFStorageDefault(nil, 0); !errors.Is(err, decode.ErrThreadingInvalidBatch) {
		t.Fatalf("nil default cdf storage err=%v want %v", err, decode.ErrThreadingInvalidBatch)
	}
	if got := decode.DecoderFrameWorkTileResidualCDFsFromStorage(nil); got.Loop.Partition != nil || got.Coeff.Coeff != nil || got.TransformType != nil {
		t.Fatalf("nil CDF view=%+v", got)
	}
	if err := decode.RetainDecoderFrameWorkTileResidualCDFStorage(batch, 0, nil, &storage); !errors.Is(err, decode.ErrThreadingInvalidBatch) {
		t.Fatalf("nil retain state err=%v want %v", err, decode.ErrThreadingInvalidBatch)
	}
}

func TestPublicDecoderBlockLoopRequestAndContextBinding(t *testing.T) {
	batch := publicDecoderBlockLoopBatch()
	rootColumns, err := decode.DecoderFrameWorkJobBlockLoopContextRootColumns(batch, 0)
	if err != nil {
		t.Fatal(err)
	}
	if rootColumns != 2 {
		t.Fatalf("root columns=%d want 2", rootColumns)
	}

	above := make([]av1.TileBlockLoopRootAboveContext, rootColumns+1)
	above[0].Partition[0] = 11
	above[rootColumns].Partition[0] = 7
	carrier, err := decode.BindTileBlockLoopContextCarrier(rootColumns, above)
	if err != nil {
		t.Fatal(err)
	}
	if len(carrier.Above) != rootColumns {
		t.Fatalf("carrier above=%d want %d", len(carrier.Above), rootColumns)
	}
	if above[0].Partition[0] != 0 {
		t.Fatalf("carrier bind did not clear active above storage: %+v", above[0].Partition)
	}
	if above[rootColumns].Partition[0] != 7 {
		t.Fatal("carrier bind touched caller storage past active root columns")
	}

	segMap := make([]uint8, 96*96)
	req, err := decode.DecoderFrameWorkJobBlockLoopRequest(batch, 0, segMap, nil, 96, &carrier)
	if err != nil {
		t.Fatal(err)
	}
	if req.ContextCarrier != &carrier {
		t.Fatalf("context carrier=%p want %p", req.ContextCarrier, &carrier)
	}
	if req.Walk.Root != av1.TileBlockLevel128x128 ||
		req.Walk.MIColStart != 32 || req.Walk.MIRowStart != 32 ||
		req.Walk.MIColEnd != 76 || req.Walk.MIRowEnd != 66 {
		t.Fatalf("walk=%+v", req.Walk)
	}
	if !req.SkipMode.Enabled || req.CDEF.Bits != 2 ||
		!req.Delta.DeltaQPresent || req.SBSizeMIB != 32 || !req.Monochrome ||
		len(req.CurrentSegmentMap) != len(segMap) || req.SegmentMapStride != 96 ||
		req.InterpolationFilter != av1.InterpolationSwitchable || !req.EnableDualFilter ||
		!req.EnableFilterIntra || !req.EnableOrderHint || req.OrderHintBits != 5 ||
		req.CurrentOrderHint != 9 || req.ReferenceOrderHints[4] != 5 ||
		req.RefFrameSide[av1.TileReferenceFrameLast2] != -1 ||
		req.RefFrameSide[av1.TileReferenceFrameLast3] != 1 ||
		!req.UseRefFrameMVS || !req.TemporalMVSampleUnavailable {
		t.Fatalf("request=%+v", req)
	}

	if _, err := decode.BindTileBlockLoopContextCarrier(-1, nil); !errors.Is(err, av1.ErrTileInvalidDecodeState) {
		t.Fatalf("negative carrier err=%v want %v", err, av1.ErrTileInvalidDecodeState)
	}
	if _, err := decode.BindTileBlockLoopContextCarrier(rootColumns, above[:rootColumns-1]); !errors.Is(err, av1.ErrFrameShortBuffer) {
		t.Fatalf("short carrier err=%v want %v", err, av1.ErrFrameShortBuffer)
	}
	if _, err := decode.DecoderFrameWorkJobBlockLoopRequest(batch, 1, nil, nil, 0, nil); !errors.Is(err, decode.ErrThreadingInvalidBatch) {
		t.Fatalf("bad job err=%v want %v", err, decode.ErrThreadingInvalidBatch)
	}
}

func TestPublicDecoderFrameWorkBlockTransformsDispatch(t *testing.T) {
	output := publicDecoderPostFilterFrame(t, av1.FrameFormat{Width: 64, Height: 64, BitDepth: 8, MonoChrome: true, Align: 64})
	batch := publicDecoderPredictionBatch(output)
	var state av1.TileDecodeState

	intra, err := decode.ReadDecoderFrameWorkBlockTransforms(batch, &state, publicDecoderPredictionIntraVisit(av1.TileIntraModeDC))
	if err != nil {
		t.Fatal(err)
	}
	if intra.Inter || !intra.ReadIntraTX || intra.ReadInterTX || intra.Luma != av1.TransformTypeDCTDCT {
		t.Fatalf("intra transforms=%+v", intra)
	}

	interVisit := publicDecoderPredictionInterVisit(av1.MotionVector{})
	interVisit.Prediction.Valid = true
	inter, err := decode.ReadDecoderFrameWorkBlockTransforms(batch, &state, interVisit)
	if err != nil {
		t.Fatal(err)
	}
	if !inter.Inter || inter.ReadIntraTX || !inter.ReadInterTX || inter.Luma != av1.TransformTypeDCTDCT {
		t.Fatalf("inter transforms=%+v", inter)
	}

	fallback, err := decode.ReadDecoderFrameWorkBlockTransforms(batch, &state, av1.TileBlockLoopVisit{})
	if err != nil {
		t.Fatal(err)
	}
	if !fallback.Inter || fallback.ReadIntraTX || !fallback.ReadInterTX {
		t.Fatalf("fallback transforms=%+v", fallback)
	}
	if _, err := decode.ReadDecoderFrameWorkBlockTransforms(batch, nil, publicDecoderPredictionIntraVisit(av1.TileIntraModeDC)); !errors.Is(err, decode.ErrThreadingInvalidBatch) {
		t.Fatalf("nil state err=%v want %v", err, decode.ErrThreadingInvalidBatch)
	}
}

func TestPublicDecoderFrameWorkResidualMaxScratchLen(t *testing.T) {
	batch, _, _, _, _ := publicDecoderResidualDriver(t)
	got32, got16, err := decode.DecoderFrameWorkResidualMaxScratchLen(batch, batch.Quantization.BaseQIdx, 0, decode.DecoderFrameWorkPlaneY)
	if err != nil {
		t.Fatal(err)
	}
	want32, want16, err := av1.ReconstructBlockMaxScratchLen(false)
	if err != nil {
		t.Fatal(err)
	}
	if got32 != want32 || got16 != want16 {
		t.Fatalf("decoder max scratch=%d/%d want %d/%d", got32, got16, want32, want16)
	}

	losslessBatch := batch
	losslessBatch.Quantization.BaseQIdx = 0
	lossless32, lossless16, err := decode.DecoderFrameWorkResidualMaxScratchLen(losslessBatch, 0, 0, decode.DecoderFrameWorkPlaneY)
	if err != nil {
		t.Fatal(err)
	}
	wantLossless32, wantLossless16, err := av1.ReconstructBlockMaxScratchLen(true)
	if err != nil {
		t.Fatal(err)
	}
	if lossless32 != wantLossless32 || lossless16 != wantLossless16 {
		t.Fatalf("decoder lossless max scratch=%d/%d want %d/%d", lossless32, lossless16, wantLossless32, wantLossless16)
	}

	if _, _, err := decode.DecoderFrameWorkResidualMaxScratchLen(batch, batch.Quantization.BaseQIdx, 0, decode.DecoderFrameWorkPlane(99)); !errors.Is(err, decode.ErrThreadingInvalidBatch) {
		t.Fatalf("invalid plane max scratch err=%v want %v", err, decode.ErrThreadingInvalidBatch)
	}

	colorBatch := batch
	colorBatch.Sequence.ColorConfig.MonoChrome = false
	colorBatch.Segmentation.Enabled = true
	colorBatch.Segmentation.Data.Segments[3].DeltaQ = -64
	frame32, frame16, err := decode.DecoderFrameWorkResidualMaxFrameScratchLen(colorBatch, colorBatch.Quantization.BaseQIdx)
	if err != nil {
		t.Fatal(err)
	}
	nonLossless32, nonLossless16, err := av1.ReconstructBlockMaxScratchLen(false)
	if err != nil {
		t.Fatal(err)
	}
	if frame32 != nonLossless32 || frame16 != nonLossless16 {
		t.Fatalf("frame scratch=%d/%d want %d/%d", frame32, frame16, nonLossless32, nonLossless16)
	}
}

func TestPublicDecoderFrameWorkBatchResidualScratchBinding(t *testing.T) {
	batch, state, storage, scratch, _ := publicDecoderResidualBatchDriver(t)
	size, err := decode.DecoderFrameWorkBatchResidualScratchLen(batch, batch.Quantization.BaseQIdx)
	if err != nil {
		t.Fatal(err)
	}
	want32, want16, err := decode.DecoderFrameWorkResidualMaxFrameScratchLen(batch, batch.Quantization.BaseQIdx)
	if err != nil {
		t.Fatal(err)
	}
	if size.Int32Scratch != want32 || size.ResidualScratch != want16 || size.LoopContextAbove != 1 {
		t.Fatalf("batch residual scratch=%+v want int32=%d residual=%d loop=1", size, want32, want16)
	}

	arena := decode.DecoderFrameWorkBatchResidualScratch{
		Int32Scratch:     make([]int32, size.Int32Scratch+1),
		ResidualScratch:  make([]int16, size.ResidualScratch+1),
		LoopContextAbove: make([]av1.TileBlockLoopRootAboveContext, size.LoopContextAbove+1),
	}
	arena.Int32Scratch[size.Int32Scratch] = 99
	arena.ResidualScratch[size.ResidualScratch] = 77
	arena.LoopContextAbove[size.LoopContextAbove].Partition[0] = 55
	bound, err := decode.BindDecoderFrameWorkBatchResidualRequestScratch(size, arena)
	if err != nil {
		t.Fatal(err)
	}
	if len(bound.Tile.Int32Scratch) != size.Int32Scratch ||
		len(bound.Tile.ResidualScratch) != size.ResidualScratch ||
		len(bound.LoopContextAbove) != size.LoopContextAbove {
		t.Fatalf("bound scratch lens int32=%d residual=%d loop=%d want %+v",
			len(bound.Tile.Int32Scratch), len(bound.Tile.ResidualScratch), len(bound.LoopContextAbove), size)
	}
	if arena.Int32Scratch[size.Int32Scratch] != 99 ||
		arena.ResidualScratch[size.ResidualScratch] != 77 ||
		arena.LoopContextAbove[size.LoopContextAbove].Partition[0] != 55 {
		t.Fatal("scratch bind touched storage past requested lengths")
	}

	bound.Tile.TransformMode = batch.TransformRef.TransformMode
	bound.Tile.UseDefaultTransforms = true
	stats, err := decode.DecodeAndRetainDecoderFrameWorkBatchResiduals(batch, state, &storage, &scratch, bound)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Loop.Blocks != 2 || stats.TXBs != 2 || stats.Residuals != 2 {
		t.Fatalf("stats=%+v", stats)
	}

	shortArena := arena
	shortArena.ResidualScratch = shortArena.ResidualScratch[:size.ResidualScratch-1]
	if _, err := decode.BindDecoderFrameWorkBatchResidualRequestScratch(size, shortArena); !errors.Is(err, av1.ErrFrameShortBuffer) {
		t.Fatalf("short residual scratch err=%v want %v", err, av1.ErrFrameShortBuffer)
	}
	negativeSize := size
	negativeSize.Int32Scratch = -1
	if _, err := decode.BindDecoderFrameWorkBatchResidualRequestScratch(negativeSize, arena); !errors.Is(err, av1.ErrFrameShortBuffer) {
		t.Fatalf("negative scratch size err=%v want %v", err, av1.ErrFrameShortBuffer)
	}
}

func TestPublicDecoderFrameWorkBatchResidualRunner(t *testing.T) {
	batch, _, _, _, _ := publicDecoderResidualBatchDriver(t)
	size, err := decode.DecoderFrameWorkBatchResidualRunnerScratchLen(batch, 2)
	if err != nil {
		t.Fatal(err)
	}
	if size.Workers != 2 || size.Int32Scratch != size.Batch.Int32Scratch*2 ||
		size.ResidualScratch != size.Batch.ResidualScratch*2 ||
		size.LoopContextAbove != size.Batch.LoopContextAbove*2 ||
		size.RestorationRequests != 2 {
		t.Fatalf("runner scratch size=%+v", size)
	}
	scratch := publicDecoderBatchResidualRunnerScratch(size)
	scratch.Int32Scratch[size.Int32Scratch] = 33
	scratch.ResidualScratch[size.ResidualScratch] = 44
	scratch.LoopContextAboveScratch[size.LoopContextAbove].Partition[0] = 66
	runner, err := decode.BindDecoderFrameWorkBatchResidualRunner(size, scratch)
	if err != nil {
		t.Fatal(err)
	}
	if !runner.UseDefaultPrediction {
		t.Fatal("runner did not enable default prediction scratch")
	}
	if scratch.PredictionScratch[1].Inter != &scratch.InterPredictionScratch[1] {
		t.Fatal("runner did not bind inter prediction scratch")
	}

	batch.Batch = decode.TileBatch{Worker: 1}
	statsErr := runner.ResetStats()
	if statsErr != nil {
		t.Fatal(statsErr)
	}
	if err := runner.Run(batch); err != nil {
		t.Fatal(err)
	}
	stats := runner.Stats[1]
	if stats.Loop.Blocks != 2 || stats.TXBs != 2 || stats.Residuals != 2 {
		t.Fatalf("runner stats=%+v", stats)
	}
	total, err := runner.TotalStats()
	if err != nil {
		t.Fatal(err)
	}
	if total.Loop.Blocks != stats.Loop.Blocks || total.TXBs != stats.TXBs {
		t.Fatalf("total stats=%+v worker stats=%+v", total, stats)
	}
	if scratch.Int32Scratch[size.Int32Scratch] != 33 ||
		scratch.ResidualScratch[size.ResidualScratch] != 44 ||
		scratch.LoopContextAboveScratch[size.LoopContextAbove].Partition[0] != 66 {
		t.Fatal("runner bind touched storage past requested lengths")
	}

	badWorker := batch
	badWorker.Batch.Worker = 2
	if err := runner.Run(badWorker); !errors.Is(err, decode.ErrThreadingInvalidBatch) {
		t.Fatalf("bad worker err=%v want %v", err, decode.ErrThreadingInvalidBatch)
	}
	brokenRunner := runner
	brokenRunner.Stats[0] = decode.DecoderFrameWorkTileResidualStats{TXBs: 88, Residuals: 77}
	brokenRunner.Int32Scratch = nil
	brokenBatch := batch
	brokenBatch.Batch.Worker = 0
	if err := brokenRunner.Run(brokenBatch); !errors.Is(err, av1.ErrFrameShortBuffer) {
		t.Fatalf("broken runner err=%v want %v", err, av1.ErrFrameShortBuffer)
	}
	if brokenRunner.Stats[0] != (decode.DecoderFrameWorkTileResidualStats{}) {
		t.Fatalf("failed worker stats were not cleared: %+v", brokenRunner.Stats[0])
	}
	shortScratch := publicDecoderBatchResidualRunnerScratch(size)
	shortScratch.Int32Scratch = shortScratch.Int32Scratch[:size.Int32Scratch-1]
	if _, err := decode.BindDecoderFrameWorkBatchResidualRunner(size, shortScratch); !errors.Is(err, av1.ErrFrameShortBuffer) {
		t.Fatalf("short runner scratch err=%v want %v", err, av1.ErrFrameShortBuffer)
	}
	if _, err := decode.DecoderFrameWorkBatchResidualRunnerScratchLen(batch, 0); !errors.Is(err, decode.ErrThreadingInvalidWorkerCount) {
		t.Fatalf("zero-worker scratch err=%v want %v", err, decode.ErrThreadingInvalidWorkerCount)
	}
	negativeSize := size
	negativeSize.RestorationRequests = -1
	if _, err := decode.BindDecoderFrameWorkBatchResidualRunner(negativeSize, scratch); !errors.Is(err, av1.ErrFrameShortBuffer) {
		t.Fatalf("negative restoration request size err=%v want %v", err, av1.ErrFrameShortBuffer)
	}
	var nilRunner *decode.DecoderFrameWorkBatchResidualRunner
	if err := nilRunner.Run(batch); !errors.Is(err, decode.ErrThreadingInvalidBatch) {
		t.Fatalf("nil runner err=%v want %v", err, decode.ErrThreadingInvalidBatch)
	}
}

func TestPublicDecoderFrameWorkResidualScratchSizeMax(t *testing.T) {
	batchA := decode.DecoderFrameWorkBatchResidualScratchSize{
		Int32Scratch:     1,
		ResidualScratch:  7,
		LoopContextAbove: 3,
	}
	batchB := decode.DecoderFrameWorkBatchResidualScratchSize{
		Int32Scratch:     5,
		ResidualScratch:  2,
		LoopContextAbove: 4,
	}
	if got, want := batchA.Max(batchB), (decode.DecoderFrameWorkBatchResidualScratchSize{
		Int32Scratch:     5,
		ResidualScratch:  7,
		LoopContextAbove: 4,
	}); got != want {
		t.Fatalf("batch residual scratch max=%+v want %+v", got, want)
	}

	runnerA := decode.DecoderFrameWorkBatchResidualRunnerScratchSize{
		Workers:             1,
		Batch:               batchA,
		RestorationRequests: 1,
		Int32Scratch:        1,
		ResidualScratch:     7,
		LoopContextAbove:    3,
	}
	runnerB := decode.DecoderFrameWorkBatchResidualRunnerScratchSize{
		Workers:             3,
		Batch:               batchB,
		RestorationRequests: 2,
		Int32Scratch:        15,
		ResidualScratch:     6,
		LoopContextAbove:    12,
	}
	if got, want := runnerA.Max(runnerB), (decode.DecoderFrameWorkBatchResidualRunnerScratchSize{
		Workers:             3,
		Batch:               batchA.Max(batchB),
		RestorationRequests: 2,
		Int32Scratch:        15,
		ResidualScratch:     7,
		LoopContextAbove:    12,
	}); got != want {
		t.Fatalf("runner residual scratch max=%+v want %+v", got, want)
	}

	eventA := decode.DecoderFrameWorkResidualEventScratchSize{
		Runner:   runnerA,
		SideData: decode.DecoderFrameWorkSideDataScratchSize{CDEFIndexMap: 2, LoopFilterMap: 8},
		Plan:     decode.DecoderTileWorkPlan{SpanCount: 2, JobCount: 2, BatchCount: 1},
		Outputs:  1,
	}
	eventB := decode.DecoderFrameWorkResidualEventScratchSize{
		Runner:   runnerB,
		SideData: decode.DecoderFrameWorkSideDataScratchSize{CDEFReadMap: 4, LoopFilterMap: 3},
		Plan:     decode.DecoderTileWorkPlan{SpanCount: 1, JobCount: 5, BatchCount: 2},
		Outputs:  3,
	}
	got := eventA.Max(eventB)
	if got.Runner != runnerA.Max(runnerB) ||
		got.SideData != (decode.DecoderFrameWorkSideDataScratchSize{CDEFIndexMap: 2, CDEFReadMap: 4, LoopFilterMap: 8}) ||
		got.Plan != (decode.DecoderTileWorkPlan{SpanCount: 2, JobCount: 5, BatchCount: 2}) ||
		got.Outputs != 3 {
		t.Fatalf("event residual scratch max=%+v", got)
	}

	streamA := decode.DecoderFrameWorkResidualStreamScratchSize{
		Events:    2,
		RTPBuffer: 16,
		RTPSpans:  1,
		Event:     eventA,
	}
	streamB := decode.DecoderFrameWorkResidualStreamScratchSize{
		Events:    4,
		RTPBuffer: 8,
		RTPSpans:  3,
		Event:     eventB,
	}
	streamGot := streamA.Max(streamB)
	if streamGot.Events != 4 ||
		streamGot.RTPBuffer != 16 ||
		streamGot.RTPSpans != 3 ||
		streamGot.Event != eventA.Max(eventB) {
		t.Fatalf("stream residual scratch max=%+v", streamGot)
	}

	sequenceA := av1.SequenceHeader{ColorConfig: av1.ColorConfig{BitDepth: 8}}
	sequenceB := av1.SequenceHeader{ColorConfig: av1.ColorConfig{BitDepth: 10}}
	planA := decode.DecoderFrameWorkResidualStreamPlan{
		Size: streamA,
		Bind: decode.DecoderFrameWorkResidualEventBindPlan{
			Sequence:   sequenceA,
			Event:      decode.DecoderEvent{Kind: decode.DecoderEventExistingFrame},
			EventIndex: 0,
		},
	}
	planB := decode.DecoderFrameWorkResidualStreamPlan{
		Size: streamB,
		Bind: decode.DecoderFrameWorkResidualEventBindPlan{
			Sequence:   sequenceB,
			Event:      decode.DecoderEvent{Kind: decode.DecoderEventTileGroup},
			EventIndex: 1,
		},
	}
	planGot := planA.Max(planB)
	if !planGot.HasEvent() ||
		planGot.Size != streamGot ||
		planGot.Bind.Event.Kind != decode.DecoderEventTileGroup ||
		planGot.Bind.Sequence.ColorConfig.BitDepth != 10 ||
		planGot.Bind.EventIndex != 1 {
		t.Fatalf("stream residual plan max=%+v", planGot)
	}
}

func TestPublicDecoderFrameWorkResidualStreamScratchCheckAndState(t *testing.T) {
	size := decode.DecoderFrameWorkResidualStreamScratchSize{
		Events:    2,
		RTPBuffer: 16,
		RTPSpans:  2,
		Event: decode.DecoderFrameWorkResidualEventScratchSize{
			Runner: decode.DecoderFrameWorkBatchResidualRunnerScratchSize{
				Workers:         1,
				Int32Scratch:    4,
				ResidualScratch: 4,
			},
			SideData: decode.DecoderFrameWorkSideDataScratchSize{
				CDEFIndexMap:  4,
				LoopFilterMap: 4,
			},
			Plan:    decode.DecoderTileWorkPlan{SpanCount: 1, JobCount: 1, BatchCount: 1},
			Outputs: 2,
		},
	}
	scratch := publicDecoderResidualStreamScratch(size)
	if err := scratch.Check(size); err != nil {
		t.Fatal(err)
	}
	shortEvents := scratch
	shortEvents.Events = shortEvents.Events[:size.Events-1]
	if err := shortEvents.Check(size); !errors.Is(err, decode.ErrDecoderEventBufferTooSmall) {
		t.Fatalf("short events err=%v want %v", err, decode.ErrDecoderEventBufferTooSmall)
	}
	shortRTP := scratch
	shortRTP.RTPBuffer = shortRTP.RTPBuffer[:size.RTPBuffer-1]
	if err := shortRTP.Check(size); !errors.Is(err, rtp.ErrRTPShortBuffer) {
		t.Fatalf("short RTP err=%v want %v", err, rtp.ErrRTPShortBuffer)
	}
	shortOutputs := scratch
	shortOutputs.Outputs = shortOutputs.Outputs[:size.Event.Outputs-1]
	if err := shortOutputs.Check(size); !errors.Is(err, av1.ErrFrameShortBuffer) {
		t.Fatalf("short outputs err=%v want %v", err, av1.ErrFrameShortBuffer)
	}
	shortEventScratch := scratch
	shortEventScratch.Event.Spans = shortEventScratch.Event.Spans[:0]
	if err := shortEventScratch.Check(size); !errors.Is(err, av1.ErrFrameShortBuffer) {
		t.Fatalf("short event scratch err=%v want %v", err, av1.ErrFrameShortBuffer)
	}

	var nilRunner *decode.DecoderFrameWorkResidualStreamRunner
	if state := nilRunner.State(); state != (decode.DecoderFrameWorkResidualStreamRunnerState{}) {
		t.Fatalf("nil runner state=%+v", state)
	}
	if _, ok := nilRunner.SequenceHeader(); ok {
		t.Fatal("nil runner reported sequence header")
	}

	var stream decode.DecoderStream
	var events [4]decode.DecoderEvent
	if count, err := stream.PushLowOverhead(publicDecoderResidualLowOverheadStream(), events[:]); err != nil {
		t.Fatal(err)
	} else if count != 3 || !stream.HasSequenceHeader() {
		t.Fatalf("low-overhead count=%d hasSequence=%v", count, stream.HasSequenceHeader())
	}
	runner := decode.DecoderFrameWorkResidualStreamRunner{
		Stream: &stream,
		EventRunner: decode.DecoderFrameWorkResidualEventRunner{
			Outputs: scratch.Outputs[:size.Event.Outputs],
		},
		Events:    scratch.Events[:size.Events],
		RTPBuffer: scratch.RTPBuffer[:size.RTPBuffer],
		RTPSpans:  scratch.RTPSpans[:size.RTPSpans],
	}
	state := runner.State()
	if !state.Bound ||
		!state.HasSequenceHeader ||
		state.InRTPFragment ||
		state.RTPUsed != 0 ||
		state.EventCapacity != size.Events ||
		state.RTPBufferCapacity != size.RTPBuffer ||
		state.RTPSpanCapacity != size.RTPSpans ||
		state.OutputCapacity != size.Event.Outputs {
		t.Fatalf("runner state=%+v", state)
	}
	sequence, ok := runner.SequenceHeader()
	if !ok || sequence.ColorConfig.BitDepth != 8 {
		t.Fatalf("sequence ok=%v header=%+v", ok, sequence)
	}

	frameHeader := publicDecoderResidualRTPElement(av1.OBUFrameHeader, publicDecoderResidualFrameHeaderPayload())
	var packet [8]byte
	n, _, more, err := rtp.PutRTPFragment(packet[:], frameHeader, 0, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if !more {
		t.Fatal("expected retained RTP fragment")
	}
	used, count, err := stream.PushRTPPayload(runner.RTPBuffer, 0, runner.RTPSpans, runner.Events, packet[:n])
	if err != nil {
		t.Fatal(err)
	}
	runner.RTPUsed = used
	state = runner.State()
	if count != 0 || !state.InRTPFragment || state.RTPUsed == 0 {
		t.Fatalf("fragment count=%d state=%+v", count, state)
	}
}

func TestPublicDecoderFrameWorkBatchResidualRunnerSideData(t *testing.T) {
	batch, _, _, _, _ := publicDecoderResidualBatchDriver(t)
	size, err := decode.DecoderFrameWorkBatchResidualRunnerScratchLen(batch, 1)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := decode.BindDecoderFrameWorkBatchResidualRunner(size, publicDecoderBatchResidualRunnerScratch(size))
	if err != nil {
		t.Fatal(err)
	}

	sequence := av1.SequenceHeader{ColorConfig: av1.ColorConfig{BitDepth: 8, MonoChrome: true}}
	cdef := av1.CDEFParams{Bits: 1, StrengthCount: 2}
	sideSize, err := decode.DecoderFrameWorkSideDataScratchLen(sequence, batch.FrameSize, cdef, av1.RestorationParams{})
	if err != nil {
		t.Fatal(err)
	}
	side, err := decode.BindDecoderFrameWorkSideData(sequence, batch.FrameSize, cdef, av1.RestorationParams{}, decode.DecoderFrameWorkSideDataScratch{
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
	side.LoopFilterMap.Records[0].Valid = true
	if err := decode.SetDecoderFrameWorkBatchResidualRunnerSideData(&runner, side); err != nil {
		t.Fatal(err)
	}
	if side.LoopFilterMap.Records[0].Valid {
		t.Fatal("runner side-data attach did not reset loop-filter map")
	}
	if err := runner.Run(batch); err != nil {
		t.Fatal(err)
	}
	seenLoopRecord := false
	for _, record := range side.LoopFilterMap.Records {
		if record.Valid {
			seenLoopRecord = true
			break
		}
	}
	if !seenLoopRecord {
		t.Fatal("runner did not write through attached loop-filter side data")
	}

	activeRestoration := av1.RestorationParams{
		Type:      [3]av1.RestorationType{av1.RestorationWiener},
		UnitSizeY: 64,
	}
	activeSideSize, err := decode.DecoderFrameWorkSideDataScratchLen(sequence, batch.FrameSize, cdef, activeRestoration)
	if err != nil {
		t.Fatal(err)
	}
	activeSide, err := decode.BindDecoderFrameWorkSideData(sequence, batch.FrameSize, cdef, activeRestoration, decode.DecoderFrameWorkSideDataScratch{
		CDEFIndexMap:             make([]uint8, activeSideSize.CDEFIndexMap),
		CDEFReadMap:              make([]bool, activeSideSize.CDEFReadMap),
		LoopFilterMap:            make([]decode.DecoderFrameWorkLoopFilterBlockRecord, activeSideSize.LoopFilterMap),
		RestorationRecords:       make([]av1.TileRestorationUnitRecord, activeSideSize.RestorationRecords),
		RestorationBoundaryAbove: make([]uint16, activeSideSize.RestorationBoundaryAbove),
		RestorationBoundaryBelow: make([]uint16, activeSideSize.RestorationBoundaryBelow),
	})
	if err != nil {
		t.Fatal(err)
	}
	noRestorationScratch := publicDecoderBatchResidualRunnerScratch(size)
	noRestorationScratch.RestorationRequests = nil
	noRestorationRunner, err := decode.BindDecoderFrameWorkBatchResidualRunner(size, noRestorationScratch)
	if err != nil {
		t.Fatal(err)
	}
	if err := decode.SetDecoderFrameWorkBatchResidualRunnerSideData(&noRestorationRunner, activeSide); !errors.Is(err, av1.ErrFrameShortBuffer) {
		t.Fatalf("missing restoration requests err=%v want %v", err, av1.ErrFrameShortBuffer)
	}
	var nilRunner *decode.DecoderFrameWorkBatchResidualRunner
	if err := decode.SetDecoderFrameWorkBatchResidualRunnerSideData(nilRunner, side); !errors.Is(err, decode.ErrThreadingInvalidBatch) {
		t.Fatalf("nil side-data runner err=%v want %v", err, decode.ErrThreadingInvalidBatch)
	}
}

func TestPublicDecoderFrameWorkResidualEventRunner(t *testing.T) {
	workerPool, err := decode.NewTileWorkerPool(1)
	if err != nil {
		t.Fatal(err)
	}
	defer workerPool.Close()

	sequence := av1.SequenceHeader{
		EnableCDEF: true,
		ColorConfig: av1.ColorConfig{
			BitDepth:   8,
			MonoChrome: true,
		},
	}
	event := publicDecoderResidualRunnerFrameEvent()
	var scratchSpans [2]av1.TileSpan
	var scratchJobs [2]av1.TileJob
	var scratchBatches [1]decode.TileBatch
	eventScratch, err := decode.DecoderFrameWorkResidualEventScratchLen(sequence, &event, 1, scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}
	runnerSize := eventScratch.Runner
	plan := eventScratch.Plan
	if plan != (decode.DecoderTileWorkPlan{SpanCount: 2, JobCount: 2, BatchCount: 1}) {
		t.Fatalf("event residual runner plan=%+v", plan)
	}
	runner, err := decode.BindDecoderFrameWorkBatchResidualRunner(runnerSize, publicDecoderBatchResidualRunnerScratch(runnerSize))
	if err != nil {
		t.Fatal(err)
	}
	side, err := decode.BindDecoderFrameWorkResidualEventSideData(sequence, &event, publicDecoderFrameWorkSideDataScratch(eventScratch.SideData))
	if err != nil {
		t.Fatal(err)
	}
	side.LoopFilterMap.Records[0].Valid = true

	pool := publicDecoderPostFilterFramePool(t, av1.FrameFormat{
		Width:      int(event.FrameSize.CodedWidth),
		Height:     int(event.FrameSize.Height),
		BitDepth:   8,
		MonoChrome: true,
		Align:      64,
	}, 2)
	var refs decode.DecoderSurfaceReferences
	var state decode.DecoderFrameWorkState
	var referenceSurfaces [av1.InterRefsPerFrame]int
	var referenceFrames [av1.InterRefsPerFrame]*av1.Frame
	var spans [2]av1.TileSpan
	var jobs [2]av1.TileJob
	var batches [1]decode.TileBatch
	var releases [av1.RefFrames]int
	var stats decode.DecoderFrameWorkTileResidualStats
	var postSawSideData bool
	eventRunner := decode.DecoderFrameWorkResidualEventRunner{
		State:             &state,
		Refs:              &refs,
		FramePool:         &pool,
		Align:             64,
		ReferenceSurfaces: referenceSurfaces[:],
		ReferenceFrames:   referenceFrames[:],
		Workers:           1,
		Spans:             spans[:],
		Jobs:              jobs[:],
		Batches:           batches[:],
		Releases:          releases[:],
		WorkerPool:        workerPool,
		BatchRunner:       &runner,
		Stats:             &stats,
	}

	result, err := eventRunner.Run(sequence, &event, &side, func(ctx decode.DecoderFrameWorkPostFilterContext) error {
		postSawSideData = ctx.CDEFIndexMap != nil && ctx.LoopFilterMap != nil && ctx.RestorationFrameBuffers != nil
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Step.Kind != decode.DecoderFrameWorkStepBegin ||
		result.Run != (decode.DecoderFrameWorkStepResult{ExecutedTileWork: true, CompletedFrame: true}) ||
		result.Output == nil ||
		state.Active() ||
		!postSawSideData {
		t.Fatalf("result=%+v output=%p active=%v postSide=%v", result, result.Output, state.Active(), postSawSideData)
	}
	seenLoopRecord := false
	for _, record := range side.LoopFilterMap.Records {
		if record.Valid {
			seenLoopRecord = true
			break
		}
	}
	if !seenLoopRecord {
		t.Fatal("event runner did not write loop-filter side data")
	}
	totalStats, err := runner.TotalStats()
	if err != nil {
		t.Fatal(err)
	}
	if stats != totalStats || stats.TXBs == 0 || stats.Residuals == 0 {
		t.Fatalf("stats=%+v total=%+v", stats, totalStats)
	}
	slot, ok := refs.ReferenceSlot(0)
	if !ok || slot < 0 {
		t.Fatalf("decoded frame was not published to reference slot 0: slot=%d ok=%v", slot, ok)
	}
	if _, err := decode.RunDecoderFrameWorkEventWithResidualRunner(decode.DecoderFrameWorkResidualEventRequest{
		Event:  decode.DecoderEvent{Kind: decode.DecoderEventFrame},
		Runner: nil,
	}); !errors.Is(err, decode.ErrThreadingInvalidBatch) {
		t.Fatalf("nil residual event runner err=%v want %v", err, decode.ErrThreadingInvalidBatch)
	}
}

func TestPublicDecoderFrameWorkResidualEventPostFilterRunner(t *testing.T) {
	workerPool, err := decode.NewTileWorkerPool(1)
	if err != nil {
		t.Fatal(err)
	}
	defer workerPool.Close()

	sequence := av1.SequenceHeader{
		EnableCDEF: true,
		ColorConfig: av1.ColorConfig{
			BitDepth:   8,
			MonoChrome: true,
		},
	}
	event := publicDecoderResidualRunnerFrameEvent()
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
	side, err := decode.BindDecoderFrameWorkResidualEventSideData(sequence, &event, publicDecoderFrameWorkSideDataScratch(eventScratch.SideData))
	if err != nil {
		t.Fatal(err)
	}

	pool := publicDecoderPostFilterFramePool(t, av1.FrameFormat{
		Width:      int(event.FrameSize.CodedWidth),
		Height:     int(event.FrameSize.Height),
		BitDepth:   8,
		MonoChrome: true,
		Align:      64,
	}, 2)
	var refs decode.DecoderSurfaceReferences
	var state decode.DecoderFrameWorkState
	var referenceSurfaces [av1.InterRefsPerFrame]int
	var referenceFrames [av1.InterRefsPerFrame]*av1.Frame
	var spans [2]av1.TileSpan
	var jobs [2]av1.TileJob
	var batches [1]decode.TileBatch
	var releases [av1.RefFrames]int
	eventRunner := decode.DecoderFrameWorkResidualEventRunner{
		State:             &state,
		Refs:              &refs,
		FramePool:         &pool,
		Align:             64,
		ReferenceSurfaces: referenceSurfaces[:],
		ReferenceFrames:   referenceFrames[:],
		Workers:           1,
		Spans:             spans[:],
		Jobs:              jobs[:],
		Batches:           batches[:],
		Releases:          releases[:],
		WorkerPool:        workerPool,
		BatchRunner:       &runner,
	}
	postRunner := publicDecoderResidualEventPostRunner{sample: 0x5a}

	result, err := eventRunner.RunWithPostFilterRunner(sequence, &event, &side, &postRunner)
	if err != nil {
		t.Fatal(err)
	}
	if result.Run != (decode.DecoderFrameWorkStepResult{ExecutedTileWork: true, CompletedFrame: true}) ||
		result.Output == nil ||
		state.Active() ||
		postRunner.calls != 1 ||
		!postRunner.side ||
		!postRunner.active ||
		postRunner.seq.ColorConfig.BitDepth != sequence.ColorConfig.BitDepth ||
		!postRunner.seq.EnableCDEF ||
		postRunner.output != result.Output {
		t.Fatalf("result=%+v active=%v post=%+v", result, state.Active(), postRunner)
	}
	slot, ok := refs.ReferenceSlot(0)
	if !ok || slot < 0 {
		t.Fatalf("decoded frame was not published to reference slot 0: slot=%d ok=%v", slot, ok)
	}
	published, err := pool.Frame(slot)
	if err != nil {
		t.Fatal(err)
	}
	if published.Y.Pix[0] != postRunner.sample {
		t.Fatalf("published sample=%d want %d", published.Y.Pix[0], postRunner.sample)
	}
	if _, err := decode.RunDecoderFrameWorkEventWithResidualRunner(decode.DecoderFrameWorkResidualEventRequest{
		Runner:     &runner,
		Post:       func(decode.DecoderFrameWorkPostFilterContext) error { return nil },
		PostRunner: &postRunner,
	}); !errors.Is(err, decode.ErrDecoderInvalidFrameWorkState) {
		t.Fatalf("mixed post callback/runner err=%v want %v", err, decode.ErrDecoderInvalidFrameWorkState)
	}
}

func TestPublicDecoderFrameWorkResidualEventRunnerResetsStats(t *testing.T) {
	workerPool, err := decode.NewTileWorkerPool(1)
	if err != nil {
		t.Fatal(err)
	}
	defer workerPool.Close()

	sequence := av1.SequenceHeader{
		ColorConfig: av1.ColorConfig{
			BitDepth:   8,
			MonoChrome: true,
		},
	}
	event := publicDecoderResidualRunnerFrameEvent()
	var scratchSpans [2]av1.TileSpan
	var scratchJobs [2]av1.TileJob
	var scratchBatches [2]decode.TileBatch
	runnerSize, _, err := decode.DecoderFrameWorkResidualEventRunnerScratchLen(sequence, &event, 2, scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}
	runner, err := decode.BindDecoderFrameWorkBatchResidualRunner(runnerSize, publicDecoderBatchResidualRunnerScratch(runnerSize))
	if err != nil {
		t.Fatal(err)
	}
	runner.Stats[1] = decode.DecoderFrameWorkTileResidualStats{TXBs: 77, Residuals: 55}

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
	eventRunner := decode.DecoderFrameWorkResidualEventRunner{
		State:             &state,
		Refs:              &refs,
		FramePool:         &pool,
		Align:             64,
		ReferenceSurfaces: referenceSurfaces[:],
		ReferenceFrames:   referenceFrames[:],
		Workers:           1,
		Spans:             spans[:],
		Jobs:              jobs[:],
		Batches:           batches[:],
		Releases:          releases[:],
		WorkerPool:        workerPool,
		BatchRunner:       &runner,
	}
	result, err := eventRunner.Run(sequence, &event, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Run != (decode.DecoderFrameWorkStepResult{ExecutedTileWork: true, CompletedFrame: true}) {
		t.Fatalf("result=%+v", result)
	}
	if runner.Stats[1] != (decode.DecoderFrameWorkTileResidualStats{}) {
		t.Fatalf("stale worker stats were not reset: %+v", runner.Stats[1])
	}
	total, err := runner.TotalStats()
	if err != nil {
		t.Fatal(err)
	}
	if total.TXBs != runner.Stats[0].TXBs || total.Residuals != runner.Stats[0].Residuals {
		t.Fatalf("total stats=%+v worker0=%+v worker1=%+v", total, runner.Stats[0], runner.Stats[1])
	}
}

func TestPublicDecoderFrameWorkResidualEventRunnerResetsStatsOnPlanError(t *testing.T) {
	sequence := av1.SequenceHeader{
		ColorConfig: av1.ColorConfig{
			BitDepth:   8,
			MonoChrome: true,
		},
	}
	event := publicDecoderResidualRunnerFrameEvent()
	var scratchSpans [2]av1.TileSpan
	var scratchJobs [2]av1.TileJob
	var scratchBatches [1]decode.TileBatch
	runnerSize, _, err := decode.DecoderFrameWorkResidualEventRunnerScratchLen(sequence, &event, 1, scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}
	runner, err := decode.BindDecoderFrameWorkBatchResidualRunner(runnerSize, publicDecoderBatchResidualRunnerScratch(runnerSize))
	if err != nil {
		t.Fatal(err)
	}
	runner.Stats[0] = decode.DecoderFrameWorkTileResidualStats{TXBs: 33, Residuals: 22}
	stats := decode.DecoderFrameWorkTileResidualStats{TXBs: 44, Residuals: 11}

	_, err = decode.RunDecoderFrameWorkEventWithResidualRunner(decode.DecoderFrameWorkResidualEventRequest{
		Runner:   &runner,
		Sequence: sequence,
		Event:    event,
		Workers:  1,
		Stats:    &stats,
	})
	if !errors.Is(err, decode.ErrDecoderInvalidFrameWorkState) {
		t.Fatalf("plan error=%v want %v", err, decode.ErrDecoderInvalidFrameWorkState)
	}
	if runner.Stats[0] != (decode.DecoderFrameWorkTileResidualStats{}) {
		t.Fatalf("plan-error stats were not reset: %+v", runner.Stats[0])
	}
	if stats != (decode.DecoderFrameWorkTileResidualStats{}) {
		t.Fatalf("plan-error aggregate stats were not reset: %+v", stats)
	}
}

func TestPublicDecoderFrameWorkResidualEventSupportedPostFilterScratchRunner(t *testing.T) {
	workerPool, err := decode.NewTileWorkerPool(1)
	if err != nil {
		t.Fatal(err)
	}
	defer workerPool.Close()

	sequence := av1.SequenceHeader{
		EnableCDEF: true,
		ColorConfig: av1.ColorConfig{
			BitDepth:   8,
			MonoChrome: true,
		},
	}
	event := publicDecoderResidualRunnerFrameEvent()
	event.CDEF = av1.CDEFParams{
		Damping:       5,
		StrengthCount: 1,
		YStrength:     [av1.MaxCDEFStrengths]uint8{63},
	}
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
	side, err := decode.BindDecoderFrameWorkResidualEventSideData(sequence, &event, publicDecoderFrameWorkSideDataScratch(eventScratch.SideData))
	if err != nil {
		t.Fatal(err)
	}

	var probeOutput av1.Frame
	probeCtx, err := decode.DecoderFrameWorkPostFilterScratchContext(sequence, &event, 64, &side, &probeOutput)
	if err != nil {
		t.Fatal(err)
	}
	var probe decode.DecoderFrameWorkSupportedPostFilterScratchRunner
	exact, err := probe.ScratchLen(probeCtx)
	if err != nil {
		t.Fatal(err)
	}
	arenaSize := decode.DecoderFrameWorkPostFilterRequestScratchLen(exact)
	postRunner := decode.DecoderFrameWorkSupportedPostFilterScratchRunner{
		Scratch: publicDecoderPostFilterRequestScratch(arenaSize),
	}

	pool := publicDecoderPostFilterFramePool(t, probeOutput.Format, 1)
	var refs decode.DecoderSurfaceReferences
	var state decode.DecoderFrameWorkState
	var referenceSurfaces [av1.InterRefsPerFrame]int
	var referenceFrames [av1.InterRefsPerFrame]*av1.Frame
	var spans [2]av1.TileSpan
	var jobs [2]av1.TileJob
	var batches [1]decode.TileBatch
	var releases [av1.RefFrames]int
	eventRunner := decode.DecoderFrameWorkResidualEventRunner{
		State:             &state,
		Refs:              &refs,
		FramePool:         &pool,
		Align:             64,
		ReferenceSurfaces: referenceSurfaces[:],
		ReferenceFrames:   referenceFrames[:],
		Workers:           1,
		Spans:             spans[:],
		Jobs:              jobs[:],
		Batches:           batches[:],
		Releases:          releases[:],
		WorkerPool:        workerPool,
		BatchRunner:       &runner,
	}

	result, err := eventRunner.RunWithPostFilterRunner(sequence, &event, &side, &postRunner)
	if err != nil {
		t.Fatal(err)
	}
	cdefRead := false
	for _, read := range side.CDEFIndexMap.Read {
		cdefRead = cdefRead || read
	}
	if result.Run != (decode.DecoderFrameWorkStepResult{ExecutedTileWork: true, CompletedFrame: true}) ||
		result.Output == nil ||
		state.Active() ||
		!postRunner.Result.Completed.Has(decode.DecoderFrameWorkPostFilterCDEF) ||
		postRunner.Result.CDEF.Units == 0 ||
		postRunner.Context.RemainingPostFilters() != 0 ||
		postRunner.Size.CDEF.Input != exact.CDEF.Input ||
		postRunner.Request.CDEF.IndexMap.Rows != side.CDEFIndexMap.Rows ||
		!cdefRead {
		t.Fatalf("result=%+v active=%v post=%+v side=%+v", result, state.Active(), postRunner, side.CDEFIndexMap)
	}
	if postRunner.Context.Event.SequenceHeader.ColorConfig.BitDepth != sequence.ColorConfig.BitDepth ||
		!postRunner.Context.Event.SequenceHeader.EnableCDEF {
		t.Fatalf("postfilter sequence=%+v want %+v", postRunner.Context.Event.SequenceHeader, sequence)
	}
	slot, ok := refs.ReferenceSlot(0)
	if !ok || slot < 0 {
		t.Fatalf("decoded frame was not published to reference slot 0: slot=%d ok=%v", slot, ok)
	}
}

func TestPublicDecoderFrameWorkResidualEventCallerPostFilterScratchRunner(t *testing.T) {
	workerPool, err := decode.NewTileWorkerPool(1)
	if err != nil {
		t.Fatal(err)
	}
	defer workerPool.Close()

	sequence := av1.SequenceHeader{
		EnableCDEF:     true,
		EnableSuperRes: true,
		ColorConfig: av1.ColorConfig{
			BitDepth:   8,
			MonoChrome: true,
		},
	}
	event := publicDecoderResidualRunnerFrameEvent()
	event.FrameSize.UpscaledWidth = 160
	event.FrameSize.SuperResEnabled = true
	event.FrameSize.SuperResDenominator = 13
	event.CDEF = av1.CDEFParams{
		Damping:       5,
		StrengthCount: 1,
		YStrength:     [av1.MaxCDEFStrengths]uint8{63},
	}
	event.FilmGrain = av1.FilmGrainParams{
		ParamsPresent: true,
		Apply:         true,
		Seed:          0x1234,
		BitDepth:      8,
		NumYPoints:    1,
		YPoints:       [av1.MaxFilmGrainYPoints][2]uint8{{0, 64}},
		ScalingShift:  8,
		Overlap:       true,
	}
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
	side, err := decode.BindDecoderFrameWorkResidualEventSideData(sequence, &event, publicDecoderFrameWorkSideDataScratch(eventScratch.SideData))
	if err != nil {
		t.Fatal(err)
	}

	var scratchOutput av1.Frame
	scratchCtx, err := decode.DecoderFrameWorkPostFilterScratchContext(sequence, &event, 64, &side, &scratchOutput)
	if err != nil {
		t.Fatal(err)
	}
	var postRunner decode.DecoderFrameWorkCallerPostFilterScratchRunner
	first, err := postRunner.ScratchLen(scratchCtx)
	if err != nil {
		t.Fatal(err)
	}
	postRunner.Scratch = publicDecoderPostFilterRequestScratch(decode.DecoderFrameWorkPostFilterRequestScratchLen(first))
	full, err := postRunner.ScratchLen(scratchCtx)
	if err != nil {
		t.Fatal(err)
	}
	postRunner.Scratch = publicDecoderPostFilterRequestScratch(decode.DecoderFrameWorkPostFilterRequestScratchLen(full))

	pool := publicDecoderPostFilterFramePool(t, scratchOutput.Format, 1)
	var refs decode.DecoderSurfaceReferences
	var state decode.DecoderFrameWorkState
	var referenceSurfaces [av1.InterRefsPerFrame]int
	var referenceFrames [av1.InterRefsPerFrame]*av1.Frame
	var spans [2]av1.TileSpan
	var jobs [2]av1.TileJob
	var batches [1]decode.TileBatch
	var releases [av1.RefFrames]int
	var outputs [2]*av1.Frame
	eventRunner := decode.DecoderFrameWorkResidualEventRunner{
		State:             &state,
		Refs:              &refs,
		FramePool:         &pool,
		Align:             64,
		ReferenceSurfaces: referenceSurfaces[:],
		ReferenceFrames:   referenceFrames[:],
		Workers:           1,
		Spans:             spans[:],
		Jobs:              jobs[:],
		Batches:           batches[:],
		Releases:          releases[:],
		WorkerPool:        workerPool,
		BatchRunner:       &runner,
		SideData:          &side,
		Outputs:           outputs[:],
	}

	result, err := eventRunner.RunWithPostFilterRunner(sequence, &event, &side, &postRunner)
	if err != nil {
		t.Fatal(err)
	}
	wantCompleted := decode.DecoderFrameWorkPostFilterCDEF |
		decode.DecoderFrameWorkPostFilterSuperRes |
		decode.DecoderFrameWorkPostFilterFilmGrain
	if result.Run != (decode.DecoderFrameWorkStepResult{ExecutedTileWork: true, CompletedFrame: true}) ||
		result.Output == nil ||
		state.Active() ||
		postRunner.Result.Completed != wantCompleted ||
		postRunner.Result.SuperRes.Output.Format.Width != int(event.FrameSize.UpscaledWidth) ||
		postRunner.Context.Output != result.Output ||
		postRunner.Context.Output.Format.Width != int(event.FrameSize.UpscaledWidth) ||
		!postRunner.Context.DetachedPostFilterOutput() ||
		postRunner.Context.RemainingPostFilters() != 0 {
		t.Fatalf("result=%+v active=%v post=%+v detached=%v", result, state.Active(), postRunner, postRunner.Context.DetachedPostFilterOutput())
	}
	slot, ok := refs.ReferenceSlot(0)
	if !ok || slot < 0 {
		t.Fatalf("decoded frame was not published to reference slot 0: slot=%d ok=%v", slot, ok)
	}

	pool.Reset()
	refs.Reset()
	state.Reset()
	outputs[0] = nil
	events := [...]decode.DecoderEvent{
		{Kind: decode.DecoderEventSequenceHeader, SequenceHeader: sequence, NewCodedVideoSequence: true},
		event,
	}
	listResult, err := eventRunner.RunEventsWithPostFilterRunner(av1.SequenceHeader{}, events[:], publicDecoderFrameWorkSideDataScratch(eventScratch.SideData), &postRunner)
	if err != nil {
		t.Fatal(err)
	}
	if listResult.OutputCount != 1 ||
		listResult.Last.Output == nil ||
		outputs[0] != listResult.Last.Output ||
		outputs[0] != postRunner.Context.Output ||
		postRunner.Context.Output.Format.Width != int(event.FrameSize.UpscaledWidth) ||
		!postRunner.Context.DetachedPostFilterOutput() {
		t.Fatalf("list result=%+v output=%p postOutput=%p", listResult, outputs[0], postRunner.Context.Output)
	}

	allocs := testing.AllocsPerRun(100, func() {
		pool.Reset()
		refs.Reset()
		state.Reset()
		result, err = eventRunner.RunWithPostFilterRunner(sequence, &event, &side, &postRunner)
		if err != nil {
			return
		}
		if result.Output == nil ||
			result.Run != (decode.DecoderFrameWorkStepResult{ExecutedTileWork: true, CompletedFrame: true}) ||
			postRunner.Result.Completed != wantCompleted ||
			postRunner.Context.RemainingPostFilters() != 0 ||
			!postRunner.Context.DetachedPostFilterOutput() {
			err = decode.ErrDecoderInvalidFrameWorkStep
			return
		}
		if _, ok := refs.ReferenceSlot(0); !ok {
			err = decode.ErrDecoderInvalidSurfaceReference
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if allocs != 0 {
		t.Fatalf("caller residual event runner allocated: %f", allocs)
	}
}

func TestPublicDecoderFrameWorkResidualEventRunnerScratchLen(t *testing.T) {
	sequence := av1.SequenceHeader{
		EnableCDEF: true,
		ColorConfig: av1.ColorConfig{
			BitDepth:   8,
			MonoChrome: true,
		},
	}
	event := publicDecoderResidualRunnerFrameEvent()
	var spans [2]av1.TileSpan
	var jobs [2]av1.TileJob
	var batches [1]decode.TileBatch
	size, plan, err := decode.DecoderFrameWorkResidualEventRunnerScratchLen(sequence, &event, 1, spans[:], jobs[:], batches[:])
	if err != nil {
		t.Fatal(err)
	}
	if plan != (decode.DecoderTileWorkPlan{SpanCount: 2, JobCount: 2, BatchCount: 1}) ||
		size.Workers != 1 ||
		size.RestorationRequests != 1 ||
		size.Batch.LoopContextAbove == 0 {
		t.Fatalf("size=%+v plan=%+v", size, plan)
	}
	if spans[0] != (av1.TileSpan{Tile: 0, Row: 0, Col: 0, Offset: 1, Size: 256}) ||
		spans[1] != (av1.TileSpan{Tile: 1, Row: 0, Col: 1, Offset: 257, Size: 256}) ||
		jobs[0].Tile != 0 ||
		jobs[1].Tile != 1 ||
		batches[0].Count != 2 {
		t.Fatalf("spans=%+v jobs=%+v batches=%+v", spans, jobs, batches)
	}
	var shortSpans [1]av1.TileSpan
	if _, _, err := decode.DecoderFrameWorkResidualEventRunnerScratchLen(sequence, &event, 1, shortSpans[:], jobs[:], batches[:]); !errors.Is(err, av1.ErrInvalidTileGroup) {
		t.Fatalf("short spans err=%v want %v", err, av1.ErrInvalidTileGroup)
	}
	if _, _, err := decode.DecoderFrameWorkResidualEventRunnerScratchLen(sequence, &decode.DecoderEvent{Kind: decode.DecoderEventSequenceHeader}, 1, spans[:], jobs[:], batches[:]); !errors.Is(err, decode.ErrDecoderInvalidTileWork) {
		t.Fatalf("invalid event err=%v want %v", err, decode.ErrDecoderInvalidTileWork)
	}

	allocs := testing.AllocsPerRun(1000, func() {
		_, _, err = decode.DecoderFrameWorkResidualEventRunnerScratchLen(sequence, &event, 1, spans[:], jobs[:], batches[:])
	})
	if err != nil {
		t.Fatal(err)
	}
	if allocs != 0 {
		t.Fatalf("DecoderFrameWorkResidualEventRunnerScratchLen allocated: %f", allocs)
	}

	combinedSize, err := decode.DecoderFrameWorkResidualEventScratchLen(sequence, &event, 1, spans[:], jobs[:], batches[:])
	if err != nil {
		t.Fatal(err)
	}
	if combinedSize.Runner != size ||
		combinedSize.Plan != plan ||
		combinedSize.SideData.CDEFIndexMap == 0 ||
		combinedSize.SideData.LoopFilterMap == 0 {
		t.Fatalf("combined residual event scratch size=%+v runner=%+v plan=%+v", combinedSize, size, plan)
	}
	side, err := decode.BindDecoderFrameWorkResidualEventSideData(sequence, &event, publicDecoderFrameWorkSideDataScratch(combinedSize.SideData))
	if err != nil {
		t.Fatal(err)
	}
	if len(side.CDEFIndexMap.Index) != combinedSize.SideData.CDEFIndexMap ||
		len(side.LoopFilterMap.Records) != combinedSize.SideData.LoopFilterMap {
		t.Fatalf("side=%+v size=%+v", side, combinedSize.SideData)
	}
	shortSideScratch := publicDecoderFrameWorkSideDataScratch(combinedSize.SideData)
	shortSideScratch.CDEFIndexMap = shortSideScratch.CDEFIndexMap[:combinedSize.SideData.CDEFIndexMap-1]
	if _, err := decode.BindDecoderFrameWorkResidualEventSideData(sequence, &event, shortSideScratch); !errors.Is(err, av1.ErrFrameShortBuffer) {
		t.Fatalf("short event side-data err=%v want %v", err, av1.ErrFrameShortBuffer)
	}

	allocs = testing.AllocsPerRun(1000, func() {
		_, err = decode.DecoderFrameWorkResidualEventScratchLen(sequence, &event, 1, spans[:], jobs[:], batches[:])
	})
	if err != nil {
		t.Fatal(err)
	}
	if allocs != 0 {
		t.Fatalf("DecoderFrameWorkResidualEventScratchLen allocated: %f", allocs)
	}

	sideScratch := publicDecoderFrameWorkSideDataScratch(combinedSize.SideData)
	allocs = testing.AllocsPerRun(1000, func() {
		_, err = decode.BindDecoderFrameWorkResidualEventSideData(sequence, &event, sideScratch)
	})
	if err != nil {
		t.Fatal(err)
	}
	if allocs != 0 {
		t.Fatalf("BindDecoderFrameWorkResidualEventSideData allocated: %f", allocs)
	}
}

func TestPublicBindDecoderFrameWorkResidualEventRunner(t *testing.T) {
	workerPool, err := decode.NewTileWorkerPool(1)
	if err != nil {
		t.Fatal(err)
	}
	defer workerPool.Close()

	sequence := av1.SequenceHeader{
		EnableCDEF: true,
		ColorConfig: av1.ColorConfig{
			BitDepth:   8,
			MonoChrome: true,
		},
	}
	event := publicDecoderResidualRunnerFrameEvent()
	var scratchSpans [2]av1.TileSpan
	var scratchJobs [2]av1.TileJob
	var scratchBatches [1]decode.TileBatch
	size, err := decode.DecoderFrameWorkResidualEventScratchLen(sequence, &event, 1, scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}

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
	var releases [av1.RefFrames]int
	var stats decode.DecoderFrameWorkTileResidualStats
	runtime := decode.DecoderFrameWorkResidualEventRuntime{
		State:             &state,
		Refs:              &refs,
		FramePool:         &pool,
		Align:             64,
		ReferenceSurfaces: referenceSurfaces[:],
		ReferenceFrames:   referenceFrames[:],
		Releases:          releases[:],
		WorkerPool:        workerPool,
		Stats:             &stats,
	}
	scratch := publicDecoderResidualEventScratch(size)
	var batchRunner decode.DecoderFrameWorkBatchResidualRunner
	eventRunner, side, err := decode.BindDecoderFrameWorkResidualEventRunner(size, sequence, &event, runtime, scratch, &batchRunner)
	if err != nil {
		t.Fatal(err)
	}
	if eventRunner.BatchRunner != &batchRunner ||
		eventRunner.Workers != size.Runner.Workers ||
		len(eventRunner.Spans) != size.Plan.SpanCount ||
		len(eventRunner.Jobs) != size.Plan.JobCount ||
		len(eventRunner.Batches) != size.Plan.BatchCount {
		t.Fatalf("event runner=%+v size=%+v", eventRunner, size)
	}

	result, err := eventRunner.Run(sequence, &event, &side, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Run != (decode.DecoderFrameWorkStepResult{ExecutedTileWork: true, CompletedFrame: true}) ||
		result.Output == nil ||
		state.Active() ||
		stats.TXBs == 0 ||
		stats.Residuals == 0 {
		t.Fatalf("result=%+v active=%v stats=%+v", result, state.Active(), stats)
	}
	if _, ok := refs.ReferenceSlot(0); !ok {
		t.Fatal("bound event runner did not publish decoded frame")
	}

	allocScratch := publicDecoderResidualEventScratch(size)
	var allocBatchRunner decode.DecoderFrameWorkBatchResidualRunner
	allocs := testing.AllocsPerRun(1000, func() {
		_, _, err = decode.BindDecoderFrameWorkResidualEventRunner(size, sequence, &event, runtime, allocScratch, &allocBatchRunner)
	})
	if err != nil {
		t.Fatal(err)
	}
	if allocs != 0 {
		t.Fatalf("BindDecoderFrameWorkResidualEventRunner allocated: %f", allocs)
	}
}

func TestPublicBindDecoderFrameWorkResidualEventRunnerRejectsInvalidScratch(t *testing.T) {
	sequence := av1.SequenceHeader{
		EnableCDEF: true,
		ColorConfig: av1.ColorConfig{
			BitDepth:   8,
			MonoChrome: true,
		},
	}
	event := publicDecoderResidualRunnerFrameEvent()
	var scratchSpans [2]av1.TileSpan
	var scratchJobs [2]av1.TileJob
	var scratchBatches [1]decode.TileBatch
	size, err := decode.DecoderFrameWorkResidualEventScratchLen(sequence, &event, 1, scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}
	runtime := decode.DecoderFrameWorkResidualEventRuntime{Align: 64}
	scratch := publicDecoderResidualEventScratch(size)

	if _, _, err := decode.BindDecoderFrameWorkResidualEventRunner(size, sequence, &event, runtime, scratch, nil); !errors.Is(err, decode.ErrThreadingInvalidBatch) {
		t.Fatalf("nil batch runner err=%v want %v", err, decode.ErrThreadingInvalidBatch)
	}

	var batchRunner decode.DecoderFrameWorkBatchResidualRunner
	shortPlanScratch := scratch
	shortPlanScratch.Jobs = shortPlanScratch.Jobs[:size.Plan.JobCount-1]
	if _, _, err := decode.BindDecoderFrameWorkResidualEventRunner(size, sequence, &event, runtime, shortPlanScratch, &batchRunner); !errors.Is(err, av1.ErrFrameShortBuffer) {
		t.Fatalf("short plan scratch err=%v want %v", err, av1.ErrFrameShortBuffer)
	}

	shortRunnerScratch := scratch
	shortRunnerScratch.Runner.Stats = shortRunnerScratch.Runner.Stats[:size.Runner.Workers-1]
	if _, _, err := decode.BindDecoderFrameWorkResidualEventRunner(size, sequence, &event, runtime, shortRunnerScratch, &batchRunner); !errors.Is(err, av1.ErrFrameShortBuffer) {
		t.Fatalf("short runner scratch err=%v want %v", err, av1.ErrFrameShortBuffer)
	}

	shortSideScratch := scratch
	shortSideScratch.SideData.LoopFilterMap = shortSideScratch.SideData.LoopFilterMap[:size.SideData.LoopFilterMap-1]
	if _, _, err := decode.BindDecoderFrameWorkResidualEventRunner(size, sequence, &event, runtime, shortSideScratch, &batchRunner); !errors.Is(err, av1.ErrFrameShortBuffer) {
		t.Fatalf("short side-data scratch err=%v want %v", err, av1.ErrFrameShortBuffer)
	}
}

func TestPublicDecoderFrameWorkResidualEventRunnerRunEvents(t *testing.T) {
	workerPool, err := decode.NewTileWorkerPool(1)
	if err != nil {
		t.Fatal(err)
	}
	defer workerPool.Close()

	sequence := av1.SequenceHeader{
		EnableCDEF: true,
		ColorConfig: av1.ColorConfig{
			BitDepth:   8,
			MonoChrome: true,
		},
	}
	frame := publicDecoderResidualRunnerFrameEvent()
	header := frame
	header.Kind = decode.DecoderEventFrameHeader
	header.Unit.Payload = nil
	tile := frame
	tile.Kind = decode.DecoderEventTileGroup
	events := [...]decode.DecoderEvent{
		{
			Kind:                  decode.DecoderEventSequenceHeader,
			SequenceHeader:        sequence,
			NewCodedVideoSequence: true,
		},
		header,
		tile,
	}

	var scratchSpans [2]av1.TileSpan
	var scratchJobs [2]av1.TileJob
	var scratchBatches [1]decode.TileBatch
	size, err := decode.DecoderFrameWorkResidualEventsScratchLen(sequence, events[:], 1, scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}
	if size.Plan != (decode.DecoderTileWorkPlan{SpanCount: 2, JobCount: 2, BatchCount: 1}) ||
		size.Runner.Workers != 1 ||
		size.SideData.CDEFIndexMap == 0 ||
		size.SideData.LoopFilterMap == 0 {
		t.Fatalf("events scratch size=%+v", size)
	}
	singleSize, err := decode.DecoderFrameWorkResidualEventScratchLen(sequence, &tile, 1, scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}
	if size != singleSize {
		t.Fatalf("events scratch=%+v single=%+v", size, singleSize)
	}

	pool := publicDecoderPostFilterFramePool(t, av1.FrameFormat{
		Width:      int(frame.FrameSize.CodedWidth),
		Height:     int(frame.FrameSize.Height),
		BitDepth:   8,
		MonoChrome: true,
		Align:      64,
	}, 1)
	var refs decode.DecoderSurfaceReferences
	var state decode.DecoderFrameWorkState
	var referenceSurfaces [av1.InterRefsPerFrame]int
	var referenceFrames [av1.InterRefsPerFrame]*av1.Frame
	var releases [av1.RefFrames]int
	var stats decode.DecoderFrameWorkTileResidualStats
	var eventSideData decode.DecoderFrameWorkSideData
	var batchRunner decode.DecoderFrameWorkBatchResidualRunner
	var outputs [2]*av1.Frame
	scratch := publicDecoderResidualEventScratch(size)
	eventRunner, _, err := decode.BindDecoderFrameWorkResidualEventRunner(size, sequence, &tile, decode.DecoderFrameWorkResidualEventRuntime{
		State:             &state,
		Refs:              &refs,
		FramePool:         &pool,
		Align:             64,
		ReferenceSurfaces: referenceSurfaces[:],
		ReferenceFrames:   referenceFrames[:],
		Releases:          releases[:],
		WorkerPool:        workerPool,
		SideData:          &eventSideData,
		Stats:             &stats,
		Outputs:           outputs[:],
	}, scratch, &batchRunner)
	if err != nil {
		t.Fatal(err)
	}
	postCalls := 0
	result, err := eventRunner.RunEvents(av1.SequenceHeader{}, events[:], scratch.SideData, func(ctx decode.DecoderFrameWorkPostFilterContext) error {
		postCalls++
		if ctx.Step.Kind != decode.DecoderFrameWorkStepTile || !ctx.ExecutedTileWork {
			t.Fatalf("postfilter ctx=%+v", ctx)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Count != len(events) ||
		result.ExecutedTileWork != 1 ||
		result.CompletedFrames != 1 ||
		result.OutputCount != 1 ||
		len(result.Outputs) != 1 ||
		result.Last.Step.Kind != decode.DecoderFrameWorkStepTile ||
		result.Last.Output == nil ||
		outputs[0] != result.Last.Output ||
		result.Outputs[0] != result.Last.Output ||
		state.Active() ||
		postCalls != 1 ||
		stats != result.Stats ||
		stats.TXBs == 0 ||
		stats.Residuals == 0 {
		t.Fatalf("result=%+v active=%v postCalls=%d stats=%+v", result, state.Active(), postCalls, stats)
	}
	if _, ok := refs.ReferenceSlot(0); !ok {
		t.Fatal("event list run did not publish decoded frame")
	}

	shortOutputRunner := eventRunner
	shortOutputRunner.Outputs = outputs[:0]
	if _, err := shortOutputRunner.RunEvents(sequence, events[:], scratch.SideData, nil); !errors.Is(err, av1.ErrFrameShortBuffer) {
		t.Fatalf("short event-list output err=%v want %v", err, av1.ErrFrameShortBuffer)
	}

	noSideRunner := eventRunner
	noSideRunner.SideData = nil
	if _, err := noSideRunner.RunEvents(sequence, events[:], scratch.SideData, nil); !errors.Is(err, decode.ErrDecoderInvalidFrameWorkState) {
		t.Fatalf("missing event-list side data err=%v want %v", err, decode.ErrDecoderInvalidFrameWorkState)
	}

	shortSideScratch := scratch.SideData
	shortSideScratch.CDEFIndexMap = shortSideScratch.CDEFIndexMap[:size.SideData.CDEFIndexMap-1]
	if _, err := eventRunner.RunEvents(sequence, events[:], shortSideScratch, nil); !errors.Is(err, av1.ErrFrameShortBuffer) {
		t.Fatalf("short event-list side scratch err=%v want %v", err, av1.ErrFrameShortBuffer)
	}
}

func TestPublicDecoderFrameWorkResidualEventRunnerRequiresRunnerForTileList(t *testing.T) {
	var eventRunner decode.DecoderFrameWorkResidualEventRunner
	sequence := av1.SequenceHeader{ColorConfig: av1.ColorConfig{BitDepth: 8, MonoChrome: true}}
	event := decode.DecoderEvent{
		Kind: decode.DecoderEventTileList,
		FrameSize: av1.FrameSize{
			CodedWidth:    64,
			UpscaledWidth: 64,
			Height:        64,
		},
		TileInfo: av1.TileInfo{
			Cols:       1,
			Rows:       1,
			ColStartSB: [av1.MaxTileCols + 1]uint16{0, 1},
			RowStartSB: [av1.MaxTileRows + 1]uint16{0, 1},
		},
		TileList: av1.TileList{
			OutputFrameWidthInTilesMinus1:  0,
			OutputFrameHeightInTilesMinus1: 0,
			TileCountMinus1:                0,
			Entries: []av1.TileListEntry{{
				AnchorFrameIdx:     0,
				AnchorTileRow:      0,
				AnchorTileCol:      0,
				TileDataSizeMinus1: 0,
				TileData:           []byte{0x80},
			}},
		},
	}

	result, err := eventRunner.RunEvents(sequence, []decode.DecoderEvent{event}, decode.DecoderFrameWorkSideDataScratch{}, nil)
	if !errors.Is(err, decode.ErrThreadingInvalidBatch) {
		t.Fatalf("RunEvents err=%v want %v", err, decode.ErrThreadingInvalidBatch)
	}
	if result.Count != 0 || result.OutputCount != 0 || result.CompletedFrames != 0 {
		t.Fatalf("result after rejected tile list=%+v", result)
	}
}

func TestPublicDecoderFrameWorkResidualEventRunnerPropagatesTileListParseError(t *testing.T) {
	var eventRunner decode.DecoderFrameWorkResidualEventRunner

	_, err := eventRunner.RunEvents(av1.SequenceHeader{}, []decode.DecoderEvent{
		{Kind: decode.DecoderEventTileList, TileListErr: av1.ErrTileListShortEntry},
	}, decode.DecoderFrameWorkSideDataScratch{}, nil)
	if !errors.Is(err, av1.ErrTileListShortEntry) {
		t.Fatalf("RunEvents err=%v want %v", err, av1.ErrTileListShortEntry)
	}
}

func TestPublicDecoderFrameWorkResidualEventRunnerValidatesTileListLayout(t *testing.T) {
	var eventRunner decode.DecoderFrameWorkResidualEventRunner
	tileList := av1.TileList{
		OutputFrameWidthInTilesMinus1:  0,
		OutputFrameHeightInTilesMinus1: 0,
		TileCountMinus1:                0,
		Entries: []av1.TileListEntry{{
			AnchorFrameIdx:     0,
			AnchorTileRow:      1,
			AnchorTileCol:      1,
			TileDataSizeMinus1: 0,
			TileData:           []byte{0xaa},
		}},
	}
	validTiles := av1.TileInfo{
		Cols:       2,
		Rows:       2,
		ColStartSB: [av1.MaxTileCols + 1]uint16{0, 2, 4},
		RowStartSB: [av1.MaxTileRows + 1]uint16{0, 1, 2},
	}

	_, err := eventRunner.RunEvents(av1.SequenceHeader{}, []decode.DecoderEvent{
		{Kind: decode.DecoderEventTileList, TileList: tileList, TileInfo: validTiles},
	}, decode.DecoderFrameWorkSideDataScratch{}, nil)
	if !errors.Is(err, decode.ErrThreadingInvalidBatch) {
		t.Fatalf("valid tile-list layout err=%v want %v", err, decode.ErrThreadingInvalidBatch)
	}

	badAnchor := validTiles
	badAnchor.Cols = 1
	_, err = eventRunner.RunEvents(av1.SequenceHeader{}, []decode.DecoderEvent{
		{Kind: decode.DecoderEventTileList, TileList: tileList, TileInfo: badAnchor},
	}, decode.DecoderFrameWorkSideDataScratch{}, nil)
	if !errors.Is(err, av1.ErrTileListInvalidAnchorTile) {
		t.Fatalf("bad anchor layout err=%v want %v", err, av1.ErrTileListInvalidAnchorTile)
	}

	nonUniform := validTiles
	nonUniform.ColStartSB[2] = 5
	_, err = eventRunner.RunEvents(av1.SequenceHeader{}, []decode.DecoderEvent{
		{Kind: decode.DecoderEventTileList, TileList: tileList, TileInfo: nonUniform},
	}, decode.DecoderFrameWorkSideDataScratch{}, nil)
	if !errors.Is(err, av1.ErrTileListNonUniformTileSize) {
		t.Fatalf("non-uniform layout err=%v want %v", err, av1.ErrTileListNonUniformTileSize)
	}
}

func TestPublicDecoderFrameWorkResidualEventRunnerPlaysTileList(t *testing.T) {
	workerPool, err := decode.NewTileWorkerPool(1)
	if err != nil {
		t.Fatal(err)
	}
	defer workerPool.Close()

	streamPayload := publicDecoderResidualLowOverheadStream()
	var stream decode.DecoderStream
	var parsed [4]decode.DecoderEvent
	count, err := stream.PushLowOverhead(streamPayload, parsed[:])
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 || parsed[2].Kind != decode.DecoderEventTileGroup {
		t.Fatalf("parsed=%+v", parsed[:count])
	}
	sequence, ok := stream.SequenceHeader()
	if !ok {
		t.Fatal("missing sequence")
	}
	event := parsed[2]
	event.Kind = decode.DecoderEventTileList
	event.Type = av1.OBUTileList
	event.Unit = av1.OBUUnit{}
	event.TileGroup = av1.TileGroup{}
	event.TileList = av1.TileList{
		OutputFrameWidthInTilesMinus1:  0,
		OutputFrameHeightInTilesMinus1: 0,
		TileCountMinus1:                0,
		Entries: []av1.TileListEntry{{
			AnchorFrameIdx:     0,
			AnchorTileRow:      0,
			AnchorTileCol:      0,
			TileDataSizeMinus1: 0,
			TileData:           []byte{0x80},
		}},
	}

	var scratchSpans [1]av1.TileSpan
	var scratchJobs [1]av1.TileJob
	var scratchBatches [1]decode.TileBatch
	size, err := decode.DecoderFrameWorkResidualEventScratchLen(sequence, &event, 1, scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}
	if size.Outputs != 1 ||
		size.Plan != (decode.DecoderTileWorkPlan{SpanCount: 1, JobCount: 1, BatchCount: 1}) ||
		size.Runner.Workers != 1 ||
		size.SideData != (decode.DecoderFrameWorkSideDataScratchSize{}) {
		t.Fatalf("tile-list scratch size=%+v", size)
	}
	runnerSize, runnerPlan, err := decode.DecoderFrameWorkResidualEventRunnerScratchLen(sequence, &event, 1, scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}
	if runnerSize != size.Runner || runnerPlan != size.Plan {
		t.Fatalf("tile-list runner scratch=%+v/%+v want %+v/%+v", runnerSize, runnerPlan, size.Runner, size.Plan)
	}
	geom, err := av1.TileListOutputGeometryForGrid(event.TileList, event.TileInfo, event.SequenceHeader.Use128x128Superblock)
	if err != nil {
		t.Fatal(err)
	}

	format, err := av1.TileListOutputFrameFormat(sequence, geom, 64)
	if err != nil {
		t.Fatal(err)
	}
	pool := publicDecoderPostFilterFramePool(t, format, 3)
	anchorSurface, anchorFrame, err := pool.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	publicFillDecoderBlockCoeffFrame(anchorFrame, 0x21)
	var refs decode.DecoderSurfaceReferences
	var releases [av1.RefFrames]int
	if _, err := refs.Refresh(1<<0, anchorSurface, releases[:]); err != nil {
		t.Fatal(err)
	}

	var state decode.DecoderFrameWorkState
	var batchRunner decode.DecoderFrameWorkBatchResidualRunner
	var outputs [1]*av1.Frame
	eventRunner, _, err := decode.BindDecoderFrameWorkResidualEventRunner(size, sequence, &event, decode.DecoderFrameWorkResidualEventRuntime{
		State:      &state,
		Refs:       &refs,
		FramePool:  &pool,
		Align:      64,
		Releases:   releases[:],
		WorkerPool: workerPool,
		Outputs:    outputs[:],
	}, publicDecoderResidualEventScratch(size), &batchRunner)
	if err != nil {
		t.Fatal(err)
	}

	result, err := eventRunner.RunEvents(sequence, []decode.DecoderEvent{event}, decode.DecoderFrameWorkSideDataScratch{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Count != 1 ||
		result.ExecutedTileWork != 1 ||
		result.CompletedFrames != 0 ||
		result.OutputCount != 1 ||
		len(result.Outputs) != 1 ||
		result.Last.Output == nil ||
		result.Last.Step.Kind != decode.DecoderFrameWorkStepTile ||
		result.Last.Run != (decode.DecoderFrameWorkStepResult{ExecutedTileWork: true}) ||
		result.Outputs[0] != result.Last.Output ||
		outputs[0] != result.Last.Output ||
		state.Active() {
		t.Fatalf("tile-list result=%+v active=%v", result, state.Active())
	}
	if surface, ok := refs.ReferenceSlot(0); !ok || surface != anchorSurface {
		t.Fatalf("tile-list mutated reference slot: surface=%d ok=%v want %d", surface, ok, anchorSurface)
	}
}

func TestPublicPlanDecoderTileListEntryWork(t *testing.T) {
	list := av1.TileList{
		OutputFrameWidthInTilesMinus1:  1,
		OutputFrameHeightInTilesMinus1: 0,
		TileCountMinus1:                1,
		Entries: []av1.TileListEntry{
			{
				AnchorFrameIdx:     0,
				AnchorTileRow:      0,
				AnchorTileCol:      1,
				TileDataSizeMinus1: 2,
				TileData:           []byte{0xaa, 0xbb, 0xcc},
			},
			{
				AnchorFrameIdx:     0,
				AnchorTileRow:      1,
				AnchorTileCol:      0,
				TileDataSizeMinus1: 1,
				TileData:           []byte{0xdd, 0xee},
			},
		},
	}
	tiles := av1.TileInfo{
		Cols:                2,
		Rows:                2,
		SBCols:              4,
		SBRows:              4,
		RefreshContext:      true,
		ContextUpdateTileID: 2,
		ColStartSB:          [av1.MaxTileCols + 1]uint16{0, 2, 4},
		RowStartSB:          [av1.MaxTileRows + 1]uint16{0, 2, 4},
	}
	var spans [1]av1.TileSpan
	var jobs [1]av1.TileJob
	var batches [1]decode.TileBatch

	plan, err := decode.PlanDecoderTileListEntryWork(list, tiles, 1, 1, spans[:], jobs[:], batches[:])
	if err != nil {
		t.Fatalf("PlanDecoderTileListEntryWork: %v", err)
	}
	if plan.SpanCount != 1 || plan.JobCount != 1 || plan.BatchCount != 1 {
		t.Fatalf("plan=%+v want 1/1/1", plan)
	}
	if spans[0] != (av1.TileSpan{Tile: 2, Row: 1, Col: 0, Offset: 0, Size: 2}) {
		t.Fatalf("span=%+v", spans[0])
	}
	if jobs[0] != (av1.TileJob{
		Tile:                2,
		Row:                 1,
		Col:                 0,
		SBX:                 0,
		SBY:                 2,
		SBCols:              2,
		SBRows:              2,
		Offset:              0,
		Size:                2,
		LastRow:             true,
		UpdatesFrameContext: true,
	}) {
		t.Fatalf("job=%+v", jobs[0])
	}
	if batches[0] != (decode.TileBatch{FirstJob: 0, Count: 1, Units: 4, Worker: 0, FirstTile: 2, LastTile: 2}) {
		t.Fatalf("batch=%+v", batches[0])
	}
	payload, err := jobs[0].Payload(list.Entries[1].TileData)
	if err != nil {
		t.Fatalf("job payload: %v", err)
	}
	if len(payload) != 2 || payload[0] != 0xdd || payload[1] != 0xee {
		t.Fatalf("payload=% x", payload)
	}
}

func TestPublicPlanDecoderTileListEntryWorkRejectsInvalid(t *testing.T) {
	list := av1.TileList{
		OutputFrameWidthInTilesMinus1:  0,
		OutputFrameHeightInTilesMinus1: 0,
		TileCountMinus1:                0,
		Entries: []av1.TileListEntry{{
			AnchorFrameIdx:     0,
			AnchorTileRow:      0,
			AnchorTileCol:      0,
			TileDataSizeMinus1: 0,
			TileData:           []byte{0xaa},
		}},
	}
	tiles := av1.TileInfo{
		Cols:       1,
		Rows:       1,
		SBCols:     2,
		SBRows:     2,
		ColStartSB: [av1.MaxTileCols + 1]uint16{0, 2},
		RowStartSB: [av1.MaxTileRows + 1]uint16{0, 2},
	}
	var spans [1]av1.TileSpan
	var jobs [1]av1.TileJob
	var batches [1]decode.TileBatch

	tests := []struct {
		name  string
		list  av1.TileList
		tiles av1.TileInfo
		index int
		spans []av1.TileSpan
		want  error
	}{
		{name: "entry-index", list: list, tiles: tiles, index: 1, spans: spans[:], want: av1.ErrTileListInvalidTileCount},
		{name: "short-spans", list: list, tiles: tiles, index: 0, spans: spans[:0], want: decode.ErrDecoderInvalidTileWork},
		{name: "short-tile-data", list: func() av1.TileList {
			short := list
			short.Entries = append([]av1.TileListEntry(nil), list.Entries...)
			short.Entries[0].TileDataSizeMinus1 = 2
			short.Entries[0].TileData = []byte{0xaa, 0xbb}
			return short
		}(), tiles: tiles, index: 0, spans: spans[:], want: av1.ErrTileListShortTileData},
		{name: "long-tile-data", list: func() av1.TileList {
			long := list
			long.Entries = append([]av1.TileListEntry(nil), list.Entries...)
			long.Entries[0].TileDataSizeMinus1 = 0
			long.Entries[0].TileData = []byte{0xaa, 0xbb}
			return long
		}(), tiles: tiles, index: 0, spans: spans[:], want: av1.ErrTileListTrailingBytes},
		{name: "non-uniform-layout", list: list, tiles: func() av1.TileInfo {
			nonUniform := tiles
			nonUniform.Cols = 2
			nonUniform.SBCols = 5
			nonUniform.ColStartSB = [av1.MaxTileCols + 1]uint16{0, 2, 5}
			return nonUniform
		}(), index: 0, spans: spans[:], want: av1.ErrTileListNonUniformTileSize},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := decode.PlanDecoderTileListEntryWork(tc.list, tc.tiles, tc.index, 1, tc.spans, jobs[:], batches[:])
			if !errors.Is(err, tc.want) {
				t.Fatalf("PlanDecoderTileListEntryWork err=%v want %v", err, tc.want)
			}
		})
	}
}

func TestPublicPlanDecoderTileListEntryDecode(t *testing.T) {
	event := publicTileListEntryDecodeEvent()
	anchors := make([]*av1.Frame, av1.TileListMaxExternalReferences)
	anchor := &av1.Frame{Format: av1.FrameFormat{Width: 256, Height: 256, BitDepth: 8, SubsamplingX: true, SubsamplingY: true}}
	anchors[7] = anchor
	references := []*av1.Frame{nil, &av1.Frame{Format: av1.FrameFormat{Width: 1, Height: 1, BitDepth: 8}}}
	var spans [1]av1.TileSpan
	var jobs [1]av1.TileJob
	var batches [1]decode.TileBatch

	plan, err := decode.PlanDecoderTileListEntryDecode(&event, 1, 3, anchors, references, 1, spans[:], jobs[:], batches[:])
	if err != nil {
		t.Fatalf("PlanDecoderTileListEntryDecode: %v", err)
	}
	if plan.Event.Kind != decode.DecoderEventTileGroup ||
		plan.Step.Kind != decode.DecoderFrameWorkStepTile ||
		plan.Step.Tile.Surface != 3 ||
		plan.ReferenceCount != 1 ||
		plan.Step.Tile.ReferenceCount != 1 ||
		plan.AnchorFrameIdx != 7 ||
		!bytes.Equal(plan.Payload, []byte{0xdd, 0xee}) ||
		!bytes.Equal(plan.Event.Unit.Payload, []byte{0xdd, 0xee}) {
		t.Fatalf("decode plan=%+v payload=% x", plan, plan.Event.Unit.Payload)
	}
	if references[0] != anchor {
		t.Fatalf("LAST reference was not bound to anchor frame: %p want %p", references[0], anchor)
	}
	if plan.Geometry.OutputFrameWidth != 256 || plan.Geometry.OutputFrameHeight != 128 {
		t.Fatalf("geometry=%+v", plan.Geometry)
	}
	if plan.Region != (av1.TileListTileRegion{SourceX: 0, SourceY: 128, DestX: 128, DestY: 0, Width: 128, Height: 128}) {
		t.Fatalf("region=%+v", plan.Region)
	}
	if spans[0] != (av1.TileSpan{Tile: 2, Row: 1, Col: 0, Offset: 0, Size: 2}) {
		t.Fatalf("span=%+v", spans[0])
	}
	if jobs[0] != (av1.TileJob{
		Tile:                2,
		Row:                 1,
		Col:                 0,
		SBX:                 0,
		SBY:                 2,
		SBCols:              2,
		SBRows:              2,
		Offset:              0,
		Size:                2,
		LastRow:             true,
		UpdatesFrameContext: true,
	}) {
		t.Fatalf("job=%+v", jobs[0])
	}

	size, scratchPlan, err := decode.DecoderTileListEntryDecodeRunnerScratchLen(&event, 1, 1, spans[:], jobs[:], batches[:])
	if err != nil {
		t.Fatalf("DecoderTileListEntryDecodeRunnerScratchLen: %v", err)
	}
	if scratchPlan != plan.Step.Tile.Tile {
		t.Fatalf("scratch plan=%+v want %+v", scratchPlan, plan.Step.Tile.Tile)
	}
	if size.Batch.Int32Scratch == 0 || size.Batch.ResidualScratch == 0 || size.Workers != 1 {
		t.Fatalf("scratch size=%+v", size)
	}
}

func TestPublicPlanDecoderTileListEntryDecodeRejectsInvalid(t *testing.T) {
	event := publicTileListEntryDecodeEvent()
	anchors := make([]*av1.Frame, av1.TileListMaxExternalReferences)
	anchor := &av1.Frame{Format: av1.FrameFormat{Width: 256, Height: 256, BitDepth: 8, SubsamplingX: true, SubsamplingY: true}}
	anchors[7] = anchor
	stale := &av1.Frame{}
	references := []*av1.Frame{stale}
	var spans [1]av1.TileSpan
	var jobs [1]av1.TileJob
	var batches [1]decode.TileBatch

	tests := []struct {
		name    string
		event   decode.DecoderEvent
		index   int
		surface int
		anchors []*av1.Frame
		refs    []*av1.Frame
		want    error
	}{
		{name: "not-tile-list", event: decode.DecoderEvent{Kind: decode.DecoderEventFrame}, index: 0, surface: 0, anchors: anchors, refs: references, want: decode.ErrDecoderInvalidSurfaceEvent},
		{name: "parse-error", event: func() decode.DecoderEvent {
			bad := event
			bad.TileListErr = av1.ErrTileListShortEntry
			return bad
		}(), index: 0, surface: 0, anchors: anchors, refs: references, want: av1.ErrTileListShortEntry},
		{name: "negative-surface", event: event, index: 0, surface: -1, anchors: anchors, refs: references, want: decode.ErrDecoderInvalidSurfaceReference},
		{name: "short-reference-table", event: event, index: 0, surface: 0, anchors: anchors, refs: nil, want: decode.ErrDecoderSurfaceReferenceBufferTooSmall},
		{name: "missing-anchor", event: event, index: 1, surface: 0, anchors: anchors[:7], refs: references, want: av1.ErrTileListInvalidAnchorIndex},
		{name: "nil-anchor", event: event, index: 1, surface: 0, anchors: append([]*av1.Frame(nil), anchors...), refs: references, want: av1.ErrTileListInvalidAnchorIndex},
		{name: "bad-index", event: event, index: 2, surface: 0, anchors: anchors, refs: references, want: av1.ErrTileListInvalidTileCount},
	}
	tests[5].anchors[7] = nil

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			references[0] = stale
			_, err := decode.PlanDecoderTileListEntryDecode(&tc.event, tc.index, tc.surface, tc.anchors, tc.refs, 1, spans[:], jobs[:], batches[:])
			if !errors.Is(err, tc.want) {
				t.Fatalf("PlanDecoderTileListEntryDecode err=%v want %v", err, tc.want)
			}
			if references[0] != stale {
				t.Fatalf("references mutated on error: %p want %p", references[0], stale)
			}
		})
	}

	if _, _, err := decode.DecoderTileListEntryDecodeRunnerScratchLen(&decode.DecoderEvent{Kind: decode.DecoderEventFrame}, 0, 1, spans[:], jobs[:], batches[:]); !errors.Is(err, decode.ErrDecoderInvalidSurfaceEvent) {
		t.Fatalf("scratch non-tile-list err=%v want %v", err, decode.ErrDecoderInvalidSurfaceEvent)
	}
	parseErrEvent := event
	parseErrEvent.TileListErr = av1.ErrTileListShortEntry
	if _, _, err := decode.DecoderTileListEntryDecodeRunnerScratchLen(&parseErrEvent, 0, 1, spans[:], jobs[:], batches[:]); !errors.Is(err, av1.ErrTileListShortEntry) {
		t.Fatalf("scratch parse err=%v want %v", err, av1.ErrTileListShortEntry)
	}
}

func publicTileListEntryDecodeEvent() decode.DecoderEvent {
	return decode.DecoderEvent{
		Kind: decode.DecoderEventTileList,
		SequenceHeader: av1.SequenceHeader{
			ColorConfig: av1.ColorConfig{BitDepth: 8, SubsamplingX: true, SubsamplingY: true},
		},
		FrameHeader: av1.FrameHeaderPrefix{FrameType: av1.FrameTypeInter},
		FrameSize: av1.FrameSize{
			CodedWidth:        256,
			UpscaledWidth:     256,
			Height:            256,
			RenderWidth:       256,
			RenderHeight:      256,
			RefreshFrameFlags: 0,
		},
		TileInfo: av1.TileInfo{
			Cols:                2,
			Rows:                2,
			SBCols:              4,
			SBRows:              4,
			RefreshContext:      true,
			ContextUpdateTileID: 2,
			ColStartSB:          [av1.MaxTileCols + 1]uint16{0, 2, 4},
			RowStartSB:          [av1.MaxTileRows + 1]uint16{0, 2, 4},
		},
		TileList: av1.TileList{
			OutputFrameWidthInTilesMinus1:  1,
			OutputFrameHeightInTilesMinus1: 0,
			TileCountMinus1:                1,
			Entries: []av1.TileListEntry{
				{
					AnchorFrameIdx:     0,
					AnchorTileRow:      0,
					AnchorTileCol:      1,
					TileDataSizeMinus1: 2,
					TileData:           []byte{0xaa, 0xbb, 0xcc},
				},
				{
					AnchorFrameIdx:     7,
					AnchorTileRow:      1,
					AnchorTileCol:      0,
					TileDataSizeMinus1: 1,
					TileData:           []byte{0xdd, 0xee},
				},
			},
		},
	}
}

func TestPublicDecoderFrameWorkResidualEventRunnerShowExistingOutput(t *testing.T) {
	sequence := av1.SequenceHeader{ColorConfig: av1.ColorConfig{
		BitDepth:   8,
		MonoChrome: true,
	}}
	event := decode.DecoderEvent{
		Kind: decode.DecoderEventExistingFrame,
		FrameHeader: av1.FrameHeaderPrefix{
			ShowExistingFrame: true,
			ExistingFrameIdx:  0,
		},
		ExistingFrame: av1.ReferenceFrame{FrameType: av1.FrameTypeInter},
		FrameSize: av1.FrameSize{
			CodedWidth:    16,
			UpscaledWidth: 16,
			Height:        16,
		},
	}
	var scratchSpans [1]av1.TileSpan
	var scratchJobs [1]av1.TileJob
	var scratchBatches [1]decode.TileBatch
	size, err := decode.DecoderFrameWorkResidualEventScratchLen(sequence, &event, 1, scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}
	if size.Runner != (decode.DecoderFrameWorkBatchResidualRunnerScratchSize{}) ||
		size.SideData != (decode.DecoderFrameWorkSideDataScratchSize{}) ||
		size.Plan != (decode.DecoderTileWorkPlan{}) ||
		size.Outputs != 1 {
		t.Fatalf("show-existing scratch size=%+v", size)
	}
	listSize, err := decode.DecoderFrameWorkResidualEventsScratchLen(sequence, []decode.DecoderEvent{event}, 1, scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}
	if listSize != size {
		t.Fatalf("show-existing list scratch=%+v single=%+v", listSize, size)
	}

	pool := publicDecoderPostFilterFramePool(t, av1.FrameFormat{
		Width:      16,
		Height:     16,
		BitDepth:   8,
		MonoChrome: true,
		Align:      64,
	}, 1)
	referenceSlot, referenceFrame, err := pool.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	var refs decode.DecoderSurfaceReferences
	var releases [av1.RefFrames]int
	if _, err := refs.Refresh(1<<0, referenceSlot, releases[:]); err != nil {
		t.Fatal(err)
	}

	var state decode.DecoderFrameWorkState
	var outputs [1]*av1.Frame
	eventRunner, _, err := decode.BindDecoderFrameWorkResidualEventRunner(size, sequence, &event, decode.DecoderFrameWorkResidualEventRuntime{
		State:     &state,
		Refs:      &refs,
		FramePool: &pool,
		Releases:  releases[:],
		Outputs:   outputs[:],
	}, publicDecoderResidualEventScratch(size), nil)
	if err != nil {
		t.Fatal(err)
	}

	result, err := eventRunner.RunEvents(sequence, []decode.DecoderEvent{event}, decode.DecoderFrameWorkSideDataScratch{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Count != 1 ||
		result.ExecutedTileWork != 0 ||
		result.CompletedFrames != 0 ||
		result.OutputCount != 1 ||
		len(result.Outputs) != 1 ||
		result.Last.Step.Kind != decode.DecoderFrameWorkStepShowExisting ||
		result.Last.Output != referenceFrame ||
		outputs[0] != referenceFrame ||
		result.Outputs[0] != referenceFrame ||
		state.Active() {
		t.Fatalf("show-existing result=%+v output=%p/%p active=%v", result, result.Last.Output, referenceFrame, state.Active())
	}

	shortOutputRunner := eventRunner
	shortOutputRunner.Outputs = outputs[:0]
	if _, err := shortOutputRunner.RunEvents(sequence, []decode.DecoderEvent{event}, decode.DecoderFrameWorkSideDataScratch{}, nil); !errors.Is(err, av1.ErrFrameShortBuffer) {
		t.Fatalf("short show-existing output err=%v want %v", err, av1.ErrFrameShortBuffer)
	}
}

func TestPublicDecoderFrameWorkResidualEventRunnerFrameHeaderWithoutBatchRunner(t *testing.T) {
	payload := publicDecoderResidualLowOverheadStream()
	var probeStream decode.DecoderStream
	var events [4]decode.DecoderEvent
	count, err := probeStream.PushLowOverhead(payload, events[:])
	if err != nil {
		t.Fatal(err)
	}
	if count < 2 || events[1].Kind != decode.DecoderEventFrameHeader {
		t.Fatalf("events=%+v", events[:count])
	}
	sequence, ok := probeStream.SequenceHeader()
	if !ok {
		t.Fatal("probe stream missing sequence")
	}
	var scratchSpans [1]av1.TileSpan
	var scratchJobs [1]av1.TileJob
	var scratchBatches [1]decode.TileBatch
	size, err := decode.DecoderFrameWorkResidualEventScratchLen(sequence, &events[1], 1, scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}
	if size.Runner != (decode.DecoderFrameWorkBatchResidualRunnerScratchSize{}) ||
		size.Plan != (decode.DecoderTileWorkPlan{}) ||
		size.Outputs != 0 ||
		size.SideData.CDEFIndexMap == 0 ||
		size.SideData.LoopFilterMap == 0 {
		t.Fatalf("frame-header scratch size=%+v", size)
	}

	pool := publicDecoderPostFilterFramePool(t, av1.FrameFormat{
		Width:        int(events[1].FrameSize.CodedWidth),
		Height:       int(events[1].FrameSize.Height),
		BitDepth:     8,
		MonoChrome:   true,
		SubsamplingX: true,
		SubsamplingY: true,
		Align:        64,
	}, 1)
	var state decode.DecoderFrameWorkState
	var refs decode.DecoderSurfaceReferences
	var side decode.DecoderFrameWorkSideData
	var stats decode.DecoderFrameWorkTileResidualStats
	eventRunner, _, err := decode.BindDecoderFrameWorkResidualEventRunner(size, sequence, &events[1], decode.DecoderFrameWorkResidualEventRuntime{
		State:     &state,
		Refs:      &refs,
		FramePool: &pool,
		Align:     64,
		SideData:  &side,
		Stats:     &stats,
	}, publicDecoderResidualEventScratch(size), nil)
	if err != nil {
		t.Fatal(err)
	}

	result, err := eventRunner.RunEvents(sequence, events[1:2], publicDecoderFrameWorkSideDataScratch(size.SideData), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Count != 1 ||
		result.ExecutedTileWork != 0 ||
		result.CompletedFrames != 0 ||
		result.OutputCount != 0 ||
		len(result.Outputs) != 0 ||
		result.Last.Step.Kind != decode.DecoderFrameWorkStepBegin ||
		!state.Active() ||
		stats != (decode.DecoderFrameWorkTileResidualStats{}) {
		t.Fatalf("frame-header result=%+v active=%v stats=%+v", result, state.Active(), stats)
	}
}

func TestPublicDecoderFrameWorkResidualEventRunnerRunEventsAllocs(t *testing.T) {
	workerPool, err := decode.NewTileWorkerPool(1)
	if err != nil {
		t.Fatal(err)
	}
	defer workerPool.Close()

	sequence := av1.SequenceHeader{
		EnableCDEF: true,
		ColorConfig: av1.ColorConfig{
			BitDepth:   8,
			MonoChrome: true,
		},
	}
	frame := publicDecoderResidualRunnerFrameEvent()
	events := [...]decode.DecoderEvent{
		{
			Kind:                  decode.DecoderEventSequenceHeader,
			SequenceHeader:        sequence,
			NewCodedVideoSequence: true,
		},
		frame,
	}
	var scratchSpans [2]av1.TileSpan
	var scratchJobs [2]av1.TileJob
	var scratchBatches [1]decode.TileBatch
	size, err := decode.DecoderFrameWorkResidualEventsScratchLen(sequence, events[:], 1, scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}
	pool := publicDecoderPostFilterFramePool(t, av1.FrameFormat{
		Width:      int(frame.FrameSize.CodedWidth),
		Height:     int(frame.FrameSize.Height),
		BitDepth:   8,
		MonoChrome: true,
		Align:      64,
	}, 1)
	var refs decode.DecoderSurfaceReferences
	var state decode.DecoderFrameWorkState
	var referenceSurfaces [av1.InterRefsPerFrame]int
	var referenceFrames [av1.InterRefsPerFrame]*av1.Frame
	var releases [av1.RefFrames]int
	var stats decode.DecoderFrameWorkTileResidualStats
	var eventSideData decode.DecoderFrameWorkSideData
	var batchRunner decode.DecoderFrameWorkBatchResidualRunner
	var outputs [1]*av1.Frame
	scratch := publicDecoderResidualEventScratch(size)
	eventRunner, _, err := decode.BindDecoderFrameWorkResidualEventRunner(size, sequence, &frame, decode.DecoderFrameWorkResidualEventRuntime{
		State:             &state,
		Refs:              &refs,
		FramePool:         &pool,
		Align:             64,
		ReferenceSurfaces: referenceSurfaces[:],
		ReferenceFrames:   referenceFrames[:],
		Releases:          releases[:],
		WorkerPool:        workerPool,
		SideData:          &eventSideData,
		Stats:             &stats,
		Outputs:           outputs[:],
	}, scratch, &batchRunner)
	if err != nil {
		t.Fatal(err)
	}

	allocs := testing.AllocsPerRun(1000, func() {
		pool.Reset()
		refs.Reset()
		state.Reset()
		result, runErr := eventRunner.RunEvents(sequence, events[:], scratch.SideData, nil)
		if runErr != nil {
			err = runErr
			return
		}
		if result.Count != len(events) ||
			result.ExecutedTileWork != 1 ||
			result.CompletedFrames != 1 ||
			result.OutputCount != 1 ||
			outputs[0] != result.Last.Output ||
			stats.TXBs == 0 ||
			stats.Residuals == 0 {
			err = decode.ErrThreadingInvalidBatch
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if allocs != 0 {
		t.Fatalf("DecoderFrameWorkResidualEventRunner.RunEvents allocated: %f", allocs)
	}
}

func TestPublicDecoderFrameWorkResidualStreamRunnerLowOverhead(t *testing.T) {
	workerPool, err := decode.NewTileWorkerPool(1)
	if err != nil {
		t.Fatal(err)
	}
	defer workerPool.Close()

	payload := publicDecoderResidualLowOverheadStream()
	var probeStream decode.DecoderStream
	var probeEvents [4]decode.DecoderEvent
	count, err := probeStream.PushLowOverhead(payload, probeEvents[:])
	if err != nil {
		t.Fatal(err)
	}
	sequence, ok := probeStream.SequenceHeader()
	if !ok {
		t.Fatal("probe stream missing sequence")
	}
	var scratchSpans [1]av1.TileSpan
	var scratchJobs [1]av1.TileJob
	var scratchBatches [1]decode.TileBatch
	size, err := decode.DecoderFrameWorkResidualEventsScratchLen(sequence, probeEvents[:count], 1, scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}

	pool := publicDecoderPostFilterFramePool(t, av1.FrameFormat{
		Width:        int(probeEvents[count-1].FrameSize.CodedWidth),
		Height:       int(probeEvents[count-1].FrameSize.Height),
		BitDepth:     8,
		MonoChrome:   true,
		SubsamplingX: true,
		SubsamplingY: true,
		Align:        64,
	}, 2)
	var stream decode.DecoderStream
	var refs decode.DecoderSurfaceReferences
	var state decode.DecoderFrameWorkState
	var referenceSurfaces [av1.InterRefsPerFrame]int
	var referenceFrames [av1.InterRefsPerFrame]*av1.Frame
	var releases [av1.RefFrames]int
	var stats decode.DecoderFrameWorkTileResidualStats
	var side decode.DecoderFrameWorkSideData
	var batchRunner decode.DecoderFrameWorkBatchResidualRunner
	var outputs [2]*av1.Frame
	scratch := publicDecoderResidualEventScratch(size)
	eventRunner, _, err := decode.BindDecoderFrameWorkResidualEventRunner(size, sequence, &probeEvents[count-1], decode.DecoderFrameWorkResidualEventRuntime{
		State:             &state,
		Refs:              &refs,
		FramePool:         &pool,
		Align:             64,
		ReferenceSurfaces: referenceSurfaces[:],
		ReferenceFrames:   referenceFrames[:],
		Releases:          releases[:],
		WorkerPool:        workerPool,
		SideData:          &side,
		Stats:             &stats,
		Outputs:           outputs[:],
	}, scratch, &batchRunner)
	if err != nil {
		t.Fatal(err)
	}
	runner := decode.DecoderFrameWorkResidualStreamRunner{
		Stream:          &stream,
		EventRunner:     eventRunner,
		Events:          make([]decode.DecoderEvent, len(probeEvents)),
		SideDataScratch: scratch.SideData,
	}

	result, err := runner.RunLowOverhead(payload, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.EventCount != count ||
		result.Run.Count != count ||
		result.Run.ExecutedTileWork != 1 ||
		result.Run.CompletedFrames != 1 ||
		result.Run.OutputCount != 1 ||
		len(result.Run.Outputs) != 1 ||
		result.Run.Last.Output == nil ||
		result.Run.Outputs[0] != result.Run.Last.Output ||
		state.Active() ||
		stats.TXBs == 0 ||
		stats.Residuals == 0 {
		t.Fatalf("stream result=%+v active=%v stats=%+v", result, state.Active(), stats)
	}
	if _, ok := refs.ReferenceSlot(0); !ok {
		t.Fatal("low-overhead stream runner did not publish decoded frame")
	}
}

func TestPublicDecoderFrameWorkResidualStreamRunnerLowOverheadShowExistingOutput(t *testing.T) {
	var stream decode.DecoderStream
	var prime []byte
	keyFrame := append([]byte{}, publicDecoderResidualShownKeyFrameHeaderPayload()...)
	keyFrame = append(keyFrame, 0xaa)
	interFrame := append([]byte{}, publicDecoderResidualInterFrameHeaderPayload()...)
	interFrame = append(interFrame, 0xbb)
	prime = appendPublicLowOverheadOBU(prime, av1.OBUSequenceHeader, publicDecoderResidualRealtimeSequenceHeaderPayload())
	prime = appendPublicLowOverheadOBU(prime, av1.OBUFrame, keyFrame)
	prime = appendPublicLowOverheadOBU(prime, av1.OBUFrame, interFrame)
	var primeEvents [3]decode.DecoderEvent
	if count, err := stream.PushLowOverhead(prime, primeEvents[:]); err != nil {
		t.Fatal(err)
	} else if count != len(primeEvents) || primeEvents[2].FrameHeader.FrameType != av1.FrameTypeInter {
		t.Fatalf("prime count=%d events=%+v", count, primeEvents)
	}

	var showExisting []byte
	showExisting = appendPublicLowOverheadOBU(showExisting, av1.OBUFrameHeader, publicDecoderResidualShowExistingFrameHeaderPayload(0))
	var events [1]decode.DecoderEvent
	var scratchSpans [1]av1.TileSpan
	var scratchJobs [1]av1.TileJob
	var scratchBatches [1]decode.TileBatch
	size, err := decode.DecoderFrameWorkResidualLowOverheadStreamScratchLen(stream, showExisting, 1, events[:], scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}
	if size.Events != 1 ||
		size.Event.Outputs != 1 ||
		size.Event.Runner != (decode.DecoderFrameWorkBatchResidualRunnerScratchSize{}) ||
		size.Event.SideData != (decode.DecoderFrameWorkSideDataScratchSize{}) ||
		size.Event.Plan != (decode.DecoderTileWorkPlan{}) ||
		events[0].Kind != decode.DecoderEventExistingFrame ||
		events[0].ExistingFrame.FrameType != av1.FrameTypeInter {
		t.Fatalf("show-existing stream scratch size=%+v event=%+v", size, events[0])
	}
	sequence, ok := stream.SequenceHeader()
	if !ok {
		t.Fatal("stream missing sequence")
	}

	pool := publicDecoderPostFilterFramePool(t, av1.FrameFormat{
		Width:        int(events[0].FrameSize.CodedWidth),
		Height:       int(events[0].FrameSize.Height),
		BitDepth:     8,
		SubsamplingX: true,
		SubsamplingY: true,
		Align:        64,
	}, 1)
	referenceSlot, referenceFrame, err := pool.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	var refs decode.DecoderSurfaceReferences
	var releases [av1.RefFrames]int
	if _, err := refs.Refresh(1<<0, referenceSlot, releases[:]); err != nil {
		t.Fatal(err)
	}

	var state decode.DecoderFrameWorkState
	var stats decode.DecoderFrameWorkTileResidualStats
	eventRunner, _, err := decode.BindDecoderFrameWorkResidualEventRunner(size.Event, sequence, &events[0], decode.DecoderFrameWorkResidualEventRuntime{
		State:     &state,
		Refs:      &refs,
		FramePool: &pool,
		Releases:  releases[:],
		Stats:     &stats,
	}, publicDecoderResidualEventScratch(size.Event), nil)
	if err != nil {
		t.Fatal(err)
	}
	streamScratch := publicDecoderResidualStreamScratch(size)
	runner, err := decode.BindDecoderFrameWorkResidualStreamRunner(size, &stream, eventRunner, streamScratch)
	if err != nil {
		t.Fatal(err)
	}

	result, err := runner.RunLowOverhead(showExisting, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.EventCount != 1 ||
		result.Run.Count != 1 ||
		result.Run.ExecutedTileWork != 0 ||
		result.Run.CompletedFrames != 0 ||
		result.Run.OutputCount != 1 ||
		len(result.Run.Outputs) != 1 ||
		result.Run.Last.Step.Kind != decode.DecoderFrameWorkStepShowExisting ||
		result.Run.Last.Output != referenceFrame ||
		result.Run.Outputs[0] != referenceFrame ||
		streamScratch.Outputs[0] != referenceFrame ||
		state.Active() ||
		stats != (decode.DecoderFrameWorkTileResidualStats{}) {
		t.Fatalf("show-existing stream result=%+v output=%p/%p active=%v stats=%+v", result, streamScratch.Outputs[0], referenceFrame, state.Active(), stats)
	}
}

func TestPublicDecoderFrameWorkResidualStreamRunnerReset(t *testing.T) {
	var stream decode.DecoderStream
	var events [4]decode.DecoderEvent
	if count, err := stream.PushLowOverhead(publicDecoderResidualLowOverheadStream(), events[:]); err != nil {
		t.Fatal(err)
	} else if count != 3 {
		t.Fatalf("count=%d events=%+v", count, events[:count])
	}
	runner := decode.DecoderFrameWorkResidualStreamRunner{
		Stream:  &stream,
		RTPUsed: 7,
	}
	if !stream.HasSequenceHeader() {
		t.Fatal("stream missing sequence before reset")
	}
	frameHeader := publicDecoderResidualRTPElement(av1.OBUFrameHeader, publicDecoderResidualFrameHeaderPayload())
	var packet [8]byte
	n, _, more, err := rtp.PutRTPFragment(packet[:], frameHeader, 0, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if !more {
		t.Fatal("expected frame header to need a second fragment")
	}
	var rtpBuffer [128]byte
	var rtpSpans [1]rtp.RTPObuSpan
	used, count, err := stream.PushRTPPayload(rtpBuffer[:], 0, rtpSpans[:], events[:1], packet[:n])
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 || !stream.InRTPFragment() || used == 0 {
		t.Fatalf("fragment push used=%d count=%d inFragment=%v", used, count, stream.InRTPFragment())
	}
	runner.RTPUsed = used
	if err := runner.ResetRTP(); err != nil {
		t.Fatal(err)
	}
	if runner.RTPUsed != 0 || !stream.HasSequenceHeader() || stream.InRTPFragment() {
		t.Fatalf("ResetRTP runner=%+v hasSequence=%v inFragment=%v", runner, stream.HasSequenceHeader(), stream.InRTPFragment())
	}
	runner.RTPUsed = 9
	if err := runner.Reset(); err != nil {
		t.Fatal(err)
	}
	if runner.RTPUsed != 0 || stream.HasSequenceHeader() || stream.InRTPFragment() {
		t.Fatalf("Reset runner=%+v hasSequence=%v inFragment=%v", runner, stream.HasSequenceHeader(), stream.InRTPFragment())
	}

	var nilRunner *decode.DecoderFrameWorkResidualStreamRunner
	if err := nilRunner.Reset(); !errors.Is(err, decode.ErrDecoderInvalidFrameWorkState) {
		t.Fatalf("nil reset err=%v want %v", err, decode.ErrDecoderInvalidFrameWorkState)
	}
	if err := nilRunner.ResetRTP(); !errors.Is(err, decode.ErrDecoderInvalidFrameWorkState) {
		t.Fatalf("nil rtp reset err=%v want %v", err, decode.ErrDecoderInvalidFrameWorkState)
	}
}

func TestPublicDecoderFrameWorkResidualEventsBindPlan(t *testing.T) {
	payload := publicDecoderResidualLowOverheadStream()
	var stream decode.DecoderStream
	var events [4]decode.DecoderEvent
	count, err := stream.PushLowOverhead(payload, events[:])
	if err != nil {
		t.Fatal(err)
	}
	plan := decode.DecoderFrameWorkResidualEventsBindPlan(av1.SequenceHeader{}, events[:count])
	if !plan.HasEvent() ||
		plan.EventIndex != count-1 ||
		plan.Event.Kind != decode.DecoderEventTileGroup ||
		plan.Sequence.ColorConfig.BitDepth != 8 ||
		!plan.Sequence.ColorConfig.MonoChrome {
		t.Fatalf("low-overhead bind plan=%+v count=%d", plan, count)
	}

	var realtime decode.DecoderStream
	var prime []byte
	keyFrame := append([]byte{}, publicDecoderResidualShownKeyFrameHeaderPayload()...)
	keyFrame = append(keyFrame, 0xaa)
	interFrame := append([]byte{}, publicDecoderResidualInterFrameHeaderPayload()...)
	interFrame = append(interFrame, 0xbb)
	prime = appendPublicLowOverheadOBU(prime, av1.OBUSequenceHeader, publicDecoderResidualRealtimeSequenceHeaderPayload())
	prime = appendPublicLowOverheadOBU(prime, av1.OBUFrame, keyFrame)
	prime = appendPublicLowOverheadOBU(prime, av1.OBUFrame, interFrame)
	var primeEvents [3]decode.DecoderEvent
	if count, err := realtime.PushLowOverhead(prime, primeEvents[:]); err != nil {
		t.Fatal(err)
	} else if count != len(primeEvents) {
		t.Fatalf("prime count=%d events=%+v", count, primeEvents)
	}
	sequence, ok := realtime.SequenceHeader()
	if !ok {
		t.Fatal("realtime stream missing sequence")
	}
	var showExisting []byte
	showExisting = appendPublicLowOverheadOBU(showExisting, av1.OBUFrameHeader, publicDecoderResidualShowExistingFrameHeaderPayload(0))
	var outputEvents [1]decode.DecoderEvent
	outputCount, err := realtime.PushLowOverhead(showExisting, outputEvents[:])
	if err != nil {
		t.Fatal(err)
	}
	outputPlan := decode.DecoderFrameWorkResidualEventsBindPlan(sequence, outputEvents[:outputCount])
	if !outputPlan.HasEvent() ||
		outputPlan.EventIndex != 0 ||
		outputPlan.Event.Kind != decode.DecoderEventExistingFrame ||
		outputPlan.Event.ExistingFrame.FrameType != av1.FrameTypeInter ||
		outputPlan.Sequence.ColorConfig.BitDepth != 8 {
		t.Fatalf("show-existing bind plan=%+v count=%d", outputPlan, outputCount)
	}

	sequenceOnly := av1.SequenceHeader{ColorConfig: av1.ColorConfig{
		BitDepth:   10,
		MonoChrome: true,
	}}
	noEventPlan := decode.DecoderFrameWorkResidualEventsBindPlan(av1.SequenceHeader{}, []decode.DecoderEvent{
		{Kind: decode.DecoderEventSequenceHeader, SequenceHeader: sequenceOnly},
	})
	if noEventPlan.HasEvent() ||
		noEventPlan.EventIndex != -1 ||
		noEventPlan.Sequence.ColorConfig.BitDepth != 10 ||
		!noEventPlan.Sequence.ColorConfig.MonoChrome {
		t.Fatalf("sequence-only bind plan=%+v", noEventPlan)
	}
}

func TestPublicDecoderFrameWorkResidualStreamPlan(t *testing.T) {
	lowOverhead := publicDecoderResidualLowOverheadStream()
	lowOverheads := [...][]byte{
		publicDecoderResidualLowOverheadStream(),
		publicDecoderResidualLowOverheadFrameStream(),
	}
	rtpPayload := publicDecoderResidualRTPPayload()
	rtpPayloads := [...][]byte{
		publicDecoderResidualRTPPayload(),
		publicDecoderResidualRTPFramePayload(),
	}
	var stream decode.DecoderStream
	var events [4]decode.DecoderEvent
	var rtpBuffer [256]byte
	var rtpSpans [4]rtp.RTPObuSpan
	var scratchSpans [1]av1.TileSpan
	var scratchJobs [1]av1.TileJob
	var scratchBatches [1]decode.TileBatch

	lowPlan, err := decode.DecoderFrameWorkResidualLowOverheadStreamPlan(stream, lowOverhead, 1, events[:], scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}
	if !lowPlan.HasEvent() ||
		lowPlan.Size.Events != 3 ||
		lowPlan.Size.Event.Outputs != 1 ||
		lowPlan.Bind.EventIndex != 2 ||
		lowPlan.Bind.Event.Kind != decode.DecoderEventTileGroup ||
		lowPlan.Bind.Sequence.ColorConfig.BitDepth != 8 {
		t.Fatalf("low-overhead stream plan=%+v", lowPlan)
	}
	lowSize, err := decode.DecoderFrameWorkResidualLowOverheadStreamScratchLen(stream, lowOverhead, 1, events[:], scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}
	if lowPlan.Size != lowSize {
		t.Fatalf("low-overhead plan size=%+v scratch size=%+v", lowPlan.Size, lowSize)
	}

	lowBatchPlan, err := decode.DecoderFrameWorkResidualLowOverheadStreamsPlan(stream, lowOverheads[:], 1, events[:], scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}
	if !lowBatchPlan.HasEvent() ||
		lowBatchPlan.Size.Events != 3 ||
		lowBatchPlan.Size.Event.Outputs != 2 ||
		lowBatchPlan.Bind.EventIndex != 1 ||
		lowBatchPlan.Bind.Event.Kind != decode.DecoderEventTileGroup {
		t.Fatalf("low-overhead stream batch plan=%+v", lowBatchPlan)
	}
	lowBatchSize, err := decode.DecoderFrameWorkResidualLowOverheadStreamsScratchLen(stream, lowOverheads[:], 1, events[:], scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}
	if lowBatchPlan.Size != lowBatchSize {
		t.Fatalf("low-overhead batch plan size=%+v scratch size=%+v", lowBatchPlan.Size, lowBatchSize)
	}

	rtpPlan, err := decode.DecoderFrameWorkResidualRTPPayloadStreamPlan(stream, 0, rtpPayload, 1, rtpBuffer[:], rtpSpans[:], events[:], scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}
	if !rtpPlan.HasEvent() ||
		rtpPlan.Size.Events != 3 ||
		rtpPlan.Size.RTPSpans != 3 ||
		rtpPlan.Size.Event.Outputs != 1 ||
		rtpPlan.Bind.EventIndex != 2 ||
		rtpPlan.Bind.Event.Kind != decode.DecoderEventTileGroup {
		t.Fatalf("rtp stream plan=%+v", rtpPlan)
	}

	batchPlan, err := decode.DecoderFrameWorkResidualRTPPayloadsStreamPlan(stream, 0, rtpPayloads[:], 1, rtpBuffer[:], rtpSpans[:], events[:], scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}
	if !batchPlan.HasEvent() ||
		batchPlan.Size.Events != 3 ||
		batchPlan.Size.RTPSpans != 3 ||
		batchPlan.Size.Event.Outputs != 2 ||
		batchPlan.Bind.EventIndex != 1 ||
		batchPlan.Bind.Event.Kind != decode.DecoderEventTileGroup ||
		batchPlan.Bind.Event.FrameHeader.OrderHint != rtpPlan.Bind.Event.FrameHeader.OrderHint {
		t.Fatalf("rtp payload batch stream plan=%+v", batchPlan)
	}
	batchSize, err := decode.DecoderFrameWorkResidualRTPPayloadsStreamScratchLen(stream, 0, rtpPayloads[:], 1, rtpBuffer[:], rtpSpans[:], events[:], scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}
	if batchPlan.Size != batchSize {
		t.Fatalf("batch plan size=%+v scratch size=%+v", batchPlan.Size, batchSize)
	}

	shortPlan, err := decode.DecoderFrameWorkResidualLowOverheadStreamPlan(stream, lowOverhead, 1, events[:2], scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if !errors.Is(err, decode.ErrDecoderEventBufferTooSmall) {
		t.Fatalf("short plan err=%v want %v", err, decode.ErrDecoderEventBufferTooSmall)
	}
	if shortPlan.Size.Events != 3 || shortPlan.HasEvent() {
		t.Fatalf("short low-overhead stream plan=%+v", shortPlan)
	}
}

func TestPublicDecoderFrameWorkResidualStreamRunnerRTPPayload(t *testing.T) {
	workerPool, err := decode.NewTileWorkerPool(1)
	if err != nil {
		t.Fatal(err)
	}
	defer workerPool.Close()

	lowOverhead := publicDecoderResidualLowOverheadStream()
	var probeStream decode.DecoderStream
	var probeEvents [4]decode.DecoderEvent
	count, err := probeStream.PushLowOverhead(lowOverhead, probeEvents[:])
	if err != nil {
		t.Fatal(err)
	}
	sequence, ok := probeStream.SequenceHeader()
	if !ok {
		t.Fatal("probe stream missing sequence")
	}
	var scratchSpans [1]av1.TileSpan
	var scratchJobs [1]av1.TileJob
	var scratchBatches [1]decode.TileBatch
	size, err := decode.DecoderFrameWorkResidualEventsScratchLen(sequence, probeEvents[:count], 1, scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}

	rtpPayload := publicDecoderResidualRTPPayload()
	pool := publicDecoderPostFilterFramePool(t, av1.FrameFormat{
		Width:        int(probeEvents[count-1].FrameSize.CodedWidth),
		Height:       int(probeEvents[count-1].FrameSize.Height),
		BitDepth:     8,
		MonoChrome:   true,
		SubsamplingX: true,
		SubsamplingY: true,
		Align:        64,
	}, 1)
	var stream decode.DecoderStream
	var refs decode.DecoderSurfaceReferences
	var state decode.DecoderFrameWorkState
	var referenceSurfaces [av1.InterRefsPerFrame]int
	var referenceFrames [av1.InterRefsPerFrame]*av1.Frame
	var releases [av1.RefFrames]int
	var stats decode.DecoderFrameWorkTileResidualStats
	var side decode.DecoderFrameWorkSideData
	var batchRunner decode.DecoderFrameWorkBatchResidualRunner
	var outputs [1]*av1.Frame
	scratch := publicDecoderResidualEventScratch(size)
	eventRunner, _, err := decode.BindDecoderFrameWorkResidualEventRunner(size, sequence, &probeEvents[count-1], decode.DecoderFrameWorkResidualEventRuntime{
		State:             &state,
		Refs:              &refs,
		FramePool:         &pool,
		Align:             64,
		ReferenceSurfaces: referenceSurfaces[:],
		ReferenceFrames:   referenceFrames[:],
		Releases:          releases[:],
		WorkerPool:        workerPool,
		SideData:          &side,
		Stats:             &stats,
		Outputs:           outputs[:],
	}, scratch, &batchRunner)
	if err != nil {
		t.Fatal(err)
	}
	runner := decode.DecoderFrameWorkResidualStreamRunner{
		Stream:          &stream,
		EventRunner:     eventRunner,
		Events:          make([]decode.DecoderEvent, len(probeEvents)),
		SideDataScratch: scratch.SideData,
		RTPBuffer:       make([]byte, len(lowOverhead)),
		RTPSpans:        make([]rtp.RTPObuSpan, len(probeEvents)),
	}

	result, err := runner.RunRTPPayload(rtpPayload, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.EventCount != count ||
		result.RTPUsed != 0 ||
		runner.RTPUsed != 0 ||
		result.Run.Count != count ||
		result.Run.ExecutedTileWork != 1 ||
		result.Run.CompletedFrames != 1 ||
		stats.TXBs == 0 ||
		stats.Residuals == 0 {
		t.Fatalf("rtp result=%+v retained=%d stats=%+v", result, runner.RTPUsed, stats)
	}
	if _, ok := refs.ReferenceSlot(0); !ok {
		t.Fatal("RTP stream runner did not publish decoded frame")
	}
}

func TestPublicDecoderFrameWorkResidualStreamRunnerLowOverheads(t *testing.T) {
	workerPool, err := decode.NewTileWorkerPool(1)
	if err != nil {
		t.Fatal(err)
	}
	defer workerPool.Close()

	payloads := [...][]byte{
		publicDecoderResidualLowOverheadStream(),
		publicDecoderResidualLowOverheadFrameStream(),
	}
	var probeStream decode.DecoderStream
	var probeEvents [4]decode.DecoderEvent
	var scratchSpans [1]av1.TileSpan
	var scratchJobs [1]av1.TileJob
	var scratchBatches [1]decode.TileBatch
	plan, err := decode.DecoderFrameWorkResidualLowOverheadStreamsPlan(probeStream, payloads[:], 1, probeEvents[:], scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}
	if plan.Size.Event.Outputs != 2 {
		t.Fatalf("low-overhead batch plan=%+v", plan)
	}

	pool := publicDecoderPostFilterFramePool(t, av1.FrameFormat{
		Width:        int(plan.Bind.Event.FrameSize.CodedWidth),
		Height:       int(plan.Bind.Event.FrameSize.Height),
		BitDepth:     8,
		MonoChrome:   true,
		SubsamplingX: true,
		SubsamplingY: true,
		Align:        64,
	}, 2)
	var stream decode.DecoderStream
	var refs decode.DecoderSurfaceReferences
	var state decode.DecoderFrameWorkState
	var referenceSurfaces [av1.InterRefsPerFrame]int
	var referenceFrames [av1.InterRefsPerFrame]*av1.Frame
	var releases [av1.RefFrames]int
	var stats decode.DecoderFrameWorkTileResidualStats
	var side decode.DecoderFrameWorkSideData
	var batchRunner decode.DecoderFrameWorkBatchResidualRunner
	streamScratch := publicDecoderResidualStreamScratch(plan.Size)
	runner, _, err := decode.BindDecoderFrameWorkResidualStreamPlanRunner(plan, &stream, decode.DecoderFrameWorkResidualEventRuntime{
		State:             &state,
		Refs:              &refs,
		FramePool:         &pool,
		Align:             64,
		ReferenceSurfaces: referenceSurfaces[:],
		ReferenceFrames:   referenceFrames[:],
		Releases:          releases[:],
		WorkerPool:        workerPool,
		SideData:          &side,
		Stats:             &stats,
	}, streamScratch, &batchRunner)
	if err != nil {
		t.Fatal(err)
	}

	var nilRunner *decode.DecoderFrameWorkResidualStreamRunner
	if _, err := nilRunner.RunLowOverheads(nil, nil); !errors.Is(err, decode.ErrDecoderInvalidFrameWorkState) {
		t.Fatalf("nil low-overhead batch runner err=%v want %v", err, decode.ErrDecoderInvalidFrameWorkState)
	}
	empty, err := runner.RunLowOverheads(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if empty.EventCount != 0 || empty.Run.Count != 0 || empty.Run.OutputCount != 0 {
		t.Fatalf("empty low-overhead batch result=%+v", empty)
	}

	result, err := runner.RunLowOverheads(payloads[:], nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.EventCount != 5 ||
		result.Run.Count != 5 ||
		result.Run.ExecutedTileWork != 2 ||
		result.Run.CompletedFrames != 2 ||
		result.Run.OutputCount != 2 ||
		len(result.Run.Outputs) != 2 ||
		streamScratch.Outputs[0] == nil ||
		streamScratch.Outputs[1] == nil ||
		result.Run.Outputs[0] != streamScratch.Outputs[0] ||
		result.Run.Outputs[1] != streamScratch.Outputs[1] ||
		result.Run.Last.Output != streamScratch.Outputs[1] ||
		stats.TXBs == 0 ||
		stats.Residuals == 0 {
		t.Fatalf("low-overhead batch result=%+v outputs=%p/%p stats=%+v", result, streamScratch.Outputs[0], streamScratch.Outputs[1], stats)
	}

	pool.Reset()
	refs.Reset()
	state.Reset()
	stats = decode.DecoderFrameWorkTileResidualStats{}
	if err := runner.Reset(); err != nil {
		t.Fatal(err)
	}
	result = decode.DecoderFrameWorkResidualStreamResult{}
	if err := runner.RunLowOverheadsInto(&result, payloads[:], nil); err != nil {
		t.Fatal(err)
	}
	if result.EventCount != 5 ||
		result.Run.CompletedFrames != 2 ||
		result.Run.OutputCount != 2 ||
		result.Run.Outputs[0] != streamScratch.Outputs[0] ||
		result.Run.Outputs[1] != streamScratch.Outputs[1] ||
		result.Run.Last.Output != streamScratch.Outputs[1] ||
		stats.TXBs == 0 ||
		stats.Residuals == 0 {
		t.Fatalf("low-overhead into result=%+v outputs=%p/%p stats=%+v", result, streamScratch.Outputs[0], streamScratch.Outputs[1], stats)
	}
}

func TestPublicDecoderFrameWorkResidualStreamRunnerRTPPayloadAfterLoss(t *testing.T) {
	workerPool, err := decode.NewTileWorkerPool(1)
	if err != nil {
		t.Fatal(err)
	}
	defer workerPool.Close()

	payloads := [...][]byte{
		publicDecoderResidualRTPPayload(),
		publicDecoderResidualRTPFramePayload(),
	}
	var probeStream decode.DecoderStream
	var probeEvents [4]decode.DecoderEvent
	var probeRTPBuffer [256]byte
	var probeRTPSpans [4]rtp.RTPObuSpan
	var scratchSpans [1]av1.TileSpan
	var scratchJobs [1]av1.TileJob
	var scratchBatches [1]decode.TileBatch
	plan, err := decode.DecoderFrameWorkResidualRTPPayloadsStreamPlan(probeStream, 0, payloads[:], 1, probeRTPBuffer[:], probeRTPSpans[:], probeEvents[:], scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}

	pool := publicDecoderPostFilterFramePool(t, av1.FrameFormat{
		Width:        int(plan.Bind.Event.FrameSize.CodedWidth),
		Height:       int(plan.Bind.Event.FrameSize.Height),
		BitDepth:     8,
		MonoChrome:   true,
		SubsamplingX: true,
		SubsamplingY: true,
		Align:        64,
	}, 2)
	var stream decode.DecoderStream
	var refs decode.DecoderSurfaceReferences
	var state decode.DecoderFrameWorkState
	var referenceSurfaces [av1.InterRefsPerFrame]int
	var referenceFrames [av1.InterRefsPerFrame]*av1.Frame
	var releases [av1.RefFrames]int
	var stats decode.DecoderFrameWorkTileResidualStats
	var side decode.DecoderFrameWorkSideData
	var batchRunner decode.DecoderFrameWorkBatchResidualRunner
	streamScratch := publicDecoderResidualStreamScratch(plan.Size)
	runner, _, err := decode.BindDecoderFrameWorkResidualStreamPlanRunner(plan, &stream, decode.DecoderFrameWorkResidualEventRuntime{
		State:             &state,
		Refs:              &refs,
		FramePool:         &pool,
		Align:             64,
		ReferenceSurfaces: referenceSurfaces[:],
		ReferenceFrames:   referenceFrames[:],
		Releases:          releases[:],
		WorkerPool:        workerPool,
		SideData:          &side,
		Stats:             &stats,
	}, streamScratch, &batchRunner)
	if err != nil {
		t.Fatal(err)
	}

	fragmented := publicDecoderResidualFragmentedRTPPayloads()
	if _, err := runner.RunRTPPayload(fragmented[0], nil); err != nil {
		t.Fatal(err)
	}
	firstFragment, err := runner.RunRTPPayload(fragmented[1], nil)
	if err != nil {
		t.Fatal(err)
	}
	if firstFragment.EventCount != 0 || firstFragment.RTPUsed == 0 || runner.RTPUsed == 0 || !runner.State().InRTPFragment {
		t.Fatalf("first fragment result=%+v state=%+v", firstFragment, runner.State())
	}
	if _, err := runner.RunRTPPayload(payloads[1], nil); !errors.Is(err, rtp.ErrRTPFragmentInterrupted) {
		t.Fatalf("interrupted fragment err=%v want %v", err, rtp.ErrRTPFragmentInterrupted)
	}
	if runner.RTPUsed == 0 || !runner.State().InRTPFragment {
		t.Fatalf("interrupted fragment did not retain state=%+v", runner.State())
	}

	recovered, err := runner.RunRTPPayloadAfterLoss(payloads[1], nil)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.EventCount != 2 ||
		recovered.RTPUsed != 0 ||
		runner.RTPUsed != 0 ||
		runner.State().InRTPFragment ||
		recovered.Run.CompletedFrames != 1 ||
		recovered.Run.OutputCount != 1 ||
		recovered.Run.Last.Output == nil ||
		stats.TXBs == 0 ||
		stats.Residuals == 0 {
		t.Fatalf("recovered result=%+v state=%+v stats=%+v", recovered, runner.State(), stats)
	}

	pool.Reset()
	refs.Reset()
	state.Reset()
	stats = decode.DecoderFrameWorkTileResidualStats{}
	if err := runner.Reset(); err != nil {
		t.Fatal(err)
	}
	var result decode.DecoderFrameWorkResidualStreamResult
	if err := runner.RunRTPPayloadsInto(&result, payloads[:1], nil); err != nil {
		t.Fatal(err)
	}
	if _, err := runner.RunRTPPayload(fragmented[1], nil); err != nil {
		t.Fatal(err)
	}
	if err := runner.RunRTPPayloadAfterLossInto(&result, payloads[1], nil); err != nil {
		t.Fatal(err)
	}
	if result.EventCount != 5 ||
		result.Run.CompletedFrames != 2 ||
		result.Run.OutputCount != 2 ||
		result.Run.Outputs[0] != streamScratch.Outputs[0] ||
		result.Run.Outputs[1] != streamScratch.Outputs[1] ||
		result.Run.Last.Output != streamScratch.Outputs[1] ||
		runner.RTPUsed != 0 ||
		runner.State().InRTPFragment ||
		stats.TXBs == 0 ||
		stats.Residuals == 0 {
		t.Fatalf("recovered into result=%+v state=%+v outputs=%p/%p stats=%+v", result, runner.State(), streamScratch.Outputs[0], streamScratch.Outputs[1], stats)
	}

	var nilRunner *decode.DecoderFrameWorkResidualStreamRunner
	if _, err := nilRunner.RunRTPPayloadAfterLoss(payloads[0], nil); !errors.Is(err, decode.ErrDecoderInvalidFrameWorkState) {
		t.Fatalf("nil after-loss err=%v want %v", err, decode.ErrDecoderInvalidFrameWorkState)
	}
	var nilResult *decode.DecoderFrameWorkResidualStreamResult
	if err := runner.RunRTPPayloadAfterLossInto(nilResult, payloads[0], nil); !errors.Is(err, decode.ErrDecoderInvalidFrameWorkState) {
		t.Fatalf("nil after-loss result err=%v want %v", err, decode.ErrDecoderInvalidFrameWorkState)
	}
}

func TestPublicDecoderFrameWorkResidualStreamRunnerRTPPayloads(t *testing.T) {
	workerPool, err := decode.NewTileWorkerPool(1)
	if err != nil {
		t.Fatal(err)
	}
	defer workerPool.Close()

	lowOverhead := publicDecoderResidualLowOverheadStream()
	rtpPayloads := publicDecoderResidualFragmentedRTPPayloads()
	var probeStream decode.DecoderStream
	var probeEvents [4]decode.DecoderEvent
	count, err := probeStream.PushLowOverhead(lowOverhead, probeEvents[:])
	if err != nil {
		t.Fatal(err)
	}
	sequence, ok := probeStream.SequenceHeader()
	if !ok {
		t.Fatal("probe stream missing sequence")
	}
	var scratchSpans [1]av1.TileSpan
	var scratchJobs [1]av1.TileJob
	var scratchBatches [1]decode.TileBatch
	size, err := decode.DecoderFrameWorkResidualEventsScratchLen(sequence, probeEvents[:count], 1, scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}

	pool := publicDecoderPostFilterFramePool(t, av1.FrameFormat{
		Width:        int(probeEvents[count-1].FrameSize.CodedWidth),
		Height:       int(probeEvents[count-1].FrameSize.Height),
		BitDepth:     8,
		MonoChrome:   true,
		SubsamplingX: true,
		SubsamplingY: true,
		Align:        64,
	}, 1)
	var stream decode.DecoderStream
	var refs decode.DecoderSurfaceReferences
	var state decode.DecoderFrameWorkState
	var referenceSurfaces [av1.InterRefsPerFrame]int
	var referenceFrames [av1.InterRefsPerFrame]*av1.Frame
	var releases [av1.RefFrames]int
	var stats decode.DecoderFrameWorkTileResidualStats
	var side decode.DecoderFrameWorkSideData
	var batchRunner decode.DecoderFrameWorkBatchResidualRunner
	var outputs [1]*av1.Frame
	scratch := publicDecoderResidualEventScratch(size)
	eventRunner, _, err := decode.BindDecoderFrameWorkResidualEventRunner(size, sequence, &probeEvents[count-1], decode.DecoderFrameWorkResidualEventRuntime{
		State:             &state,
		Refs:              &refs,
		FramePool:         &pool,
		Align:             64,
		ReferenceSurfaces: referenceSurfaces[:],
		ReferenceFrames:   referenceFrames[:],
		Releases:          releases[:],
		WorkerPool:        workerPool,
		SideData:          &side,
		Stats:             &stats,
		Outputs:           outputs[:],
	}, scratch, &batchRunner)
	if err != nil {
		t.Fatal(err)
	}
	runner := decode.DecoderFrameWorkResidualStreamRunner{
		Stream:          &stream,
		EventRunner:     eventRunner,
		Events:          make([]decode.DecoderEvent, len(probeEvents)),
		SideDataScratch: scratch.SideData,
		RTPBuffer:       make([]byte, len(lowOverhead)),
		RTPSpans:        make([]rtp.RTPObuSpan, len(probeEvents)),
	}

	var nilRunner *decode.DecoderFrameWorkResidualStreamRunner
	if _, err := nilRunner.RunRTPPayloads(nil, nil); !errors.Is(err, decode.ErrDecoderInvalidFrameWorkState) {
		t.Fatalf("nil RTP payload batch runner err=%v want %v", err, decode.ErrDecoderInvalidFrameWorkState)
	}
	empty, err := runner.RunRTPPayloads(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if empty.EventCount != 0 || empty.RTPUsed != 0 || empty.Run.Count != 0 || empty.Run.OutputCount != 0 {
		t.Fatalf("empty RTP payload batch result=%+v", empty)
	}

	postCalls := 0
	result, err := runner.RunRTPPayloads(rtpPayloads, func(ctx decode.DecoderFrameWorkPostFilterContext) error {
		postCalls++
		if !ctx.ExecutedTileWork || ctx.Step.Kind != decode.DecoderFrameWorkStepTile {
			t.Fatalf("postfilter ctx=%+v", ctx)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.EventCount != count ||
		result.RTPUsed != 0 ||
		runner.RTPUsed != 0 ||
		result.Run.Count != count ||
		result.Run.ExecutedTileWork != 1 ||
		result.Run.CompletedFrames != 1 ||
		result.Run.OutputCount != 1 ||
		len(result.Run.Outputs) != 1 ||
		result.Run.Last.Output == nil ||
		outputs[0] != result.Run.Last.Output ||
		result.Run.Outputs[0] != result.Run.Last.Output ||
		postCalls != 1 ||
		stats.TXBs == 0 ||
		stats.Residuals == 0 {
		t.Fatalf("rtp batch result=%+v retained=%d postCalls=%d stats=%+v", result, runner.RTPUsed, postCalls, stats)
	}
	if _, ok := refs.ReferenceSlot(0); !ok {
		t.Fatal("RTP payload batch runner did not publish decoded frame")
	}
}

func TestPublicDecoderFrameWorkResidualStreamRunnerRTPPayloadsMultipleOutputs(t *testing.T) {
	workerPool, err := decode.NewTileWorkerPool(1)
	if err != nil {
		t.Fatal(err)
	}
	defer workerPool.Close()

	payloads := [...][]byte{
		publicDecoderResidualRTPPayload(),
		publicDecoderResidualRTPFramePayload(),
	}
	var probeStream decode.DecoderStream
	var probeEvents [4]decode.DecoderEvent
	var probeRTPBuffer [256]byte
	var probeRTPSpans [4]rtp.RTPObuSpan
	var scratchSpans [1]av1.TileSpan
	var scratchJobs [1]av1.TileJob
	var scratchBatches [1]decode.TileBatch
	plan, err := decode.DecoderFrameWorkResidualRTPPayloadsStreamPlan(probeStream, 0, payloads[:], 1, probeRTPBuffer[:], probeRTPSpans[:], probeEvents[:], scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}
	size := plan.Size
	if size.Events != 3 || size.RTPSpans != 3 || size.Event.Outputs != 2 {
		t.Fatalf("multi-output RTP batch scratch size=%+v", size)
	}
	if probeEvents[0].Kind != decode.DecoderEventFrameHeader || probeEvents[1].Kind != decode.DecoderEventTileGroup {
		t.Fatalf("last RTP batch events=%+v", probeEvents[:2])
	}
	if !plan.HasEvent() ||
		plan.Bind.EventIndex != 1 ||
		plan.Bind.Event.Kind != decode.DecoderEventTileGroup {
		t.Fatalf("multi-output RTP batch bind plan=%+v", plan)
	}
	sequence := plan.Bind.Sequence
	tile := plan.Bind.Event

	pool := publicDecoderPostFilterFramePool(t, av1.FrameFormat{
		Width:        int(tile.FrameSize.CodedWidth),
		Height:       int(tile.FrameSize.Height),
		BitDepth:     8,
		MonoChrome:   true,
		SubsamplingX: true,
		SubsamplingY: true,
		Align:        64,
	}, 2)
	var stream decode.DecoderStream
	var refs decode.DecoderSurfaceReferences
	var state decode.DecoderFrameWorkState
	var referenceSurfaces [av1.InterRefsPerFrame]int
	var referenceFrames [av1.InterRefsPerFrame]*av1.Frame
	var releases [av1.RefFrames]int
	var stats decode.DecoderFrameWorkTileResidualStats
	var side decode.DecoderFrameWorkSideData
	var batchRunner decode.DecoderFrameWorkBatchResidualRunner
	eventScratch := publicDecoderResidualEventScratch(size.Event)
	eventRunner, _, err := decode.BindDecoderFrameWorkResidualEventRunner(size.Event, sequence, &tile, decode.DecoderFrameWorkResidualEventRuntime{
		State:             &state,
		Refs:              &refs,
		FramePool:         &pool,
		Align:             64,
		ReferenceSurfaces: referenceSurfaces[:],
		ReferenceFrames:   referenceFrames[:],
		Releases:          releases[:],
		WorkerPool:        workerPool,
		SideData:          &side,
		Stats:             &stats,
	}, eventScratch, &batchRunner)
	if err != nil {
		t.Fatal(err)
	}
	streamScratch := publicDecoderResidualStreamScratch(size)
	runner, err := decode.BindDecoderFrameWorkResidualStreamRunner(size, &stream, eventRunner, streamScratch)
	if err != nil {
		t.Fatal(err)
	}

	result, err := runner.RunRTPPayloads(payloads[:], nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.EventCount != 5 ||
		result.RTPUsed != 0 ||
		runner.RTPUsed != 0 ||
		result.Run.Count != 5 ||
		result.Run.ExecutedTileWork != 2 ||
		result.Run.CompletedFrames != 2 ||
		result.Run.OutputCount != 2 ||
		len(result.Run.Outputs) != 2 ||
		result.Run.Last.Output == nil ||
		streamScratch.Outputs[0] == nil ||
		streamScratch.Outputs[1] != result.Run.Last.Output ||
		result.Run.Outputs[0] != streamScratch.Outputs[0] ||
		result.Run.Outputs[1] != result.Run.Last.Output ||
		stats.TXBs == 0 ||
		stats.Residuals == 0 {
		t.Fatalf("multi-output RTP batch result=%+v retained=%d outputs=%p/%p stats=%+v", result, runner.RTPUsed, streamScratch.Outputs[0], streamScratch.Outputs[1], stats)
	}
	if _, ok := refs.ReferenceSlot(0); !ok {
		t.Fatal("multi-output RTP payload batch runner did not publish decoded frame")
	}
}

func TestPublicDecoderFrameWorkResidualStreamRunnerRTPPayloadInto(t *testing.T) {
	workerPool, err := decode.NewTileWorkerPool(1)
	if err != nil {
		t.Fatal(err)
	}
	defer workerPool.Close()

	payloads := [...][]byte{
		publicDecoderResidualRTPPayload(),
		publicDecoderResidualRTPFramePayload(),
	}
	var probeStream decode.DecoderStream
	var probeEvents [4]decode.DecoderEvent
	var probeRTPBuffer [256]byte
	var probeRTPSpans [4]rtp.RTPObuSpan
	var scratchSpans [1]av1.TileSpan
	var scratchJobs [1]av1.TileJob
	var scratchBatches [1]decode.TileBatch
	plan, err := decode.DecoderFrameWorkResidualRTPPayloadsStreamPlan(probeStream, 0, payloads[:], 1, probeRTPBuffer[:], probeRTPSpans[:], probeEvents[:], scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}

	pool := publicDecoderPostFilterFramePool(t, av1.FrameFormat{
		Width:        int(plan.Bind.Event.FrameSize.CodedWidth),
		Height:       int(plan.Bind.Event.FrameSize.Height),
		BitDepth:     8,
		MonoChrome:   true,
		SubsamplingX: true,
		SubsamplingY: true,
		Align:        64,
	}, 2)
	var stream decode.DecoderStream
	var refs decode.DecoderSurfaceReferences
	var state decode.DecoderFrameWorkState
	var referenceSurfaces [av1.InterRefsPerFrame]int
	var referenceFrames [av1.InterRefsPerFrame]*av1.Frame
	var releases [av1.RefFrames]int
	var stats decode.DecoderFrameWorkTileResidualStats
	var side decode.DecoderFrameWorkSideData
	var batchRunner decode.DecoderFrameWorkBatchResidualRunner
	streamScratch := publicDecoderResidualStreamScratch(plan.Size)
	runner, _, err := decode.BindDecoderFrameWorkResidualStreamPlanRunner(plan, &stream, decode.DecoderFrameWorkResidualEventRuntime{
		State:             &state,
		Refs:              &refs,
		FramePool:         &pool,
		Align:             64,
		ReferenceSurfaces: referenceSurfaces[:],
		ReferenceFrames:   referenceFrames[:],
		Releases:          releases[:],
		WorkerPool:        workerPool,
		SideData:          &side,
		Stats:             &stats,
	}, streamScratch, &batchRunner)
	if err != nil {
		t.Fatal(err)
	}

	var nilResult *decode.DecoderFrameWorkResidualStreamResult
	if err := runner.RunRTPPayloadInto(nilResult, payloads[0], nil); !errors.Is(err, decode.ErrDecoderInvalidFrameWorkState) {
		t.Fatalf("nil result err=%v want %v", err, decode.ErrDecoderInvalidFrameWorkState)
	}
	if err := runner.RunRTPPayloadsInto(nilResult, nil, nil); !errors.Is(err, decode.ErrDecoderInvalidFrameWorkState) {
		t.Fatalf("nil batch result err=%v want %v", err, decode.ErrDecoderInvalidFrameWorkState)
	}
	var result decode.DecoderFrameWorkResidualStreamResult
	if err := runner.RunRTPPayloadsInto(&result, payloads[:], nil); err != nil {
		t.Fatal(err)
	}
	if result.RTPUsed != 0 || runner.RTPUsed != 0 {
		t.Fatalf("retained result=%d runner=%d", result.RTPUsed, runner.RTPUsed)
	}
	if result.EventCount != 5 ||
		result.Run.Count != 5 ||
		result.Run.ExecutedTileWork != 2 ||
		result.Run.CompletedFrames != 2 ||
		result.Run.OutputCount != 2 ||
		len(result.Run.Outputs) != 2 ||
		streamScratch.Outputs[0] == nil ||
		streamScratch.Outputs[1] == nil ||
		result.Run.Outputs[0] != streamScratch.Outputs[0] ||
		result.Run.Outputs[1] != streamScratch.Outputs[1] ||
		result.Run.Last.Output != streamScratch.Outputs[1] ||
		stats.TXBs == 0 ||
		stats.Residuals == 0 {
		t.Fatalf("RTP payload into result=%+v outputs=%p/%p stats=%+v", result, streamScratch.Outputs[0], streamScratch.Outputs[1], stats)
	}
	if _, ok := refs.ReferenceSlot(0); !ok {
		t.Fatal("RTP payload into runner did not publish decoded frame")
	}
}

func TestPublicDecoderFrameWorkResidualStreamResultAccumulate(t *testing.T) {
	var firstFrame av1.Frame
	var secondFrame av1.Frame
	var result decode.DecoderFrameWorkResidualStreamResult
	if err := result.Accumulate(decode.DecoderFrameWorkResidualStreamResult{
		EventCount: 2,
		RTPUsed:    3,
		Run: decode.DecoderFrameWorkResidualEventsResult{
			Count:            2,
			ExecutedTileWork: 1,
			CompletedFrames:  1,
			OutputCount:      1,
			Last: decode.DecoderFrameWorkEventResult{
				Output: &firstFrame,
			},
			Stats: decode.DecoderFrameWorkTileResidualStats{
				TXBs:      1,
				Residuals: 2,
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := result.Accumulate(decode.DecoderFrameWorkResidualStreamResult{
		EventCount: 1,
		RTPUsed:    0,
		Run: decode.DecoderFrameWorkResidualEventsResult{
			Count:            1,
			ExecutedTileWork: 1,
			CompletedFrames:  1,
			OutputCount:      1,
			Last: decode.DecoderFrameWorkEventResult{
				Output: &secondFrame,
			},
			Stats: decode.DecoderFrameWorkTileResidualStats{
				TXBs:      3,
				Residuals: 5,
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	outputs := [...]*av1.Frame{&firstFrame, &secondFrame}
	if err := result.BindOutputs(outputs[:]); err != nil {
		t.Fatal(err)
	}
	if result.EventCount != 3 ||
		result.RTPUsed != 0 ||
		result.Run.Count != 3 ||
		result.Run.ExecutedTileWork != 2 ||
		result.Run.CompletedFrames != 2 ||
		result.Run.OutputCount != 2 ||
		result.Run.Last.Output != &secondFrame ||
		result.Run.Outputs[0] != &firstFrame ||
		result.Run.Outputs[1] != &secondFrame ||
		result.Run.Stats.TXBs != 4 ||
		result.Run.Stats.Residuals != 7 {
		t.Fatalf("accumulated result=%+v", result)
	}
	if err := result.BindOutputs(outputs[:1]); !errors.Is(err, av1.ErrFrameShortBuffer) {
		t.Fatalf("short outputs err=%v want %v", err, av1.ErrFrameShortBuffer)
	}
	var nilStreamResult *decode.DecoderFrameWorkResidualStreamResult
	if err := nilStreamResult.Accumulate(result); !errors.Is(err, decode.ErrDecoderInvalidFrameWorkState) {
		t.Fatalf("nil stream accumulate err=%v want %v", err, decode.ErrDecoderInvalidFrameWorkState)
	}
	var nilEventsResult *decode.DecoderFrameWorkResidualEventsResult
	if err := nilEventsResult.Accumulate(result.Run); !errors.Is(err, decode.ErrDecoderInvalidFrameWorkState) {
		t.Fatalf("nil event accumulate err=%v want %v", err, decode.ErrDecoderInvalidFrameWorkState)
	}
	if err := nilEventsResult.BindOutputs(outputs[:]); !errors.Is(err, decode.ErrDecoderInvalidFrameWorkState) {
		t.Fatalf("nil event outputs err=%v want %v", err, decode.ErrDecoderInvalidFrameWorkState)
	}
}

func TestPublicBindDecoderFrameWorkResidualStreamRunner(t *testing.T) {
	workerPool, err := decode.NewTileWorkerPool(1)
	if err != nil {
		t.Fatal(err)
	}
	defer workerPool.Close()

	lowOverhead := publicDecoderResidualLowOverheadStream()
	rtpPayload := publicDecoderResidualRTPPayload()
	var probeStream decode.DecoderStream
	var probeEvents [4]decode.DecoderEvent
	var probeRTPBuffer [128]byte
	var probeRTPSpans [4]rtp.RTPObuSpan
	var scratchSpans [1]av1.TileSpan
	var scratchJobs [1]av1.TileJob
	var scratchBatches [1]decode.TileBatch
	lowPlan, err := decode.DecoderFrameWorkResidualLowOverheadStreamPlan(probeStream, lowOverhead, 1, probeEvents[:], scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}
	rtpPlan, err := decode.DecoderFrameWorkResidualRTPPayloadStreamPlan(probeStream, 0, rtpPayload, 1, probeRTPBuffer[:], probeRTPSpans[:], probeEvents[:], scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}
	streamPlan := lowPlan.Max(rtpPlan)
	streamSize := streamPlan.Size
	if !streamPlan.HasEvent() || streamPlan.Bind.Event.Kind != decode.DecoderEventTileGroup {
		t.Fatalf("stream bind plan=%+v", streamPlan)
	}
	sequence := streamPlan.Bind.Sequence
	tile := streamPlan.Bind.Event

	pool := publicDecoderPostFilterFramePool(t, av1.FrameFormat{
		Width:        int(tile.FrameSize.CodedWidth),
		Height:       int(tile.FrameSize.Height),
		BitDepth:     8,
		MonoChrome:   true,
		SubsamplingX: true,
		SubsamplingY: true,
		Align:        64,
	}, 1)
	var refs decode.DecoderSurfaceReferences
	var state decode.DecoderFrameWorkState
	var referenceSurfaces [av1.InterRefsPerFrame]int
	var referenceFrames [av1.InterRefsPerFrame]*av1.Frame
	var releases [av1.RefFrames]int
	var stats decode.DecoderFrameWorkTileResidualStats
	var side decode.DecoderFrameWorkSideData
	var batchRunner decode.DecoderFrameWorkBatchResidualRunner
	eventScratch := publicDecoderResidualEventScratch(streamSize.Event)
	eventRunner, _, err := decode.BindDecoderFrameWorkResidualEventRunner(streamSize.Event, sequence, &tile, decode.DecoderFrameWorkResidualEventRuntime{
		State:             &state,
		Refs:              &refs,
		FramePool:         &pool,
		Align:             64,
		ReferenceSurfaces: referenceSurfaces[:],
		ReferenceFrames:   referenceFrames[:],
		Releases:          releases[:],
		WorkerPool:        workerPool,
		SideData:          &side,
		Stats:             &stats,
	}, eventScratch, &batchRunner)
	if err != nil {
		t.Fatal(err)
	}

	var stream decode.DecoderStream
	streamScratch := publicDecoderResidualStreamScratch(streamSize)
	runner, err := decode.BindDecoderFrameWorkResidualStreamRunner(streamSize, &stream, eventRunner, streamScratch)
	if err != nil {
		t.Fatal(err)
	}
	if runner.Stream != &stream ||
		len(runner.Events) != streamSize.Events ||
		len(runner.RTPBuffer) != streamSize.RTPBuffer ||
		len(runner.RTPSpans) != streamSize.RTPSpans ||
		len(runner.SideDataScratch.CDEFIndexMap) != streamSize.Event.SideData.CDEFIndexMap ||
		len(runner.EventRunner.Outputs) != streamSize.Event.Outputs {
		t.Fatalf("bound runner=%+v size=%+v", runner, streamSize)
	}

	result, err := runner.RunLowOverhead(lowOverhead, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Run.CompletedFrames != 1 || stats.TXBs == 0 {
		t.Fatalf("low-overhead result=%+v stats=%+v", result, stats)
	}

	pool.Reset()
	refs.Reset()
	state.Reset()
	if err := runner.Reset(); err != nil {
		t.Fatal(err)
	}
	result, err = runner.RunRTPPayload(rtpPayload, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Run.CompletedFrames != 1 || runner.RTPUsed != 0 || stats.TXBs == 0 {
		t.Fatalf("rtp result=%+v retained=%d stats=%+v", result, runner.RTPUsed, stats)
	}

	if _, err := decode.BindDecoderFrameWorkResidualStreamRunner(streamSize, nil, eventRunner, streamScratch); !errors.Is(err, decode.ErrDecoderInvalidFrameWorkState) {
		t.Fatalf("nil stream err=%v want %v", err, decode.ErrDecoderInvalidFrameWorkState)
	}
	shortEvents := streamScratch
	shortEvents.Events = shortEvents.Events[:streamSize.Events-1]
	if _, err := decode.BindDecoderFrameWorkResidualStreamRunner(streamSize, &stream, eventRunner, shortEvents); !errors.Is(err, decode.ErrDecoderEventBufferTooSmall) {
		t.Fatalf("short events err=%v want %v", err, decode.ErrDecoderEventBufferTooSmall)
	}
	shortRTP := streamScratch
	shortRTP.RTPBuffer = shortRTP.RTPBuffer[:streamSize.RTPBuffer-1]
	if _, err := decode.BindDecoderFrameWorkResidualStreamRunner(streamSize, &stream, eventRunner, shortRTP); !errors.Is(err, rtp.ErrRTPShortBuffer) {
		t.Fatalf("short rtp err=%v want %v", err, rtp.ErrRTPShortBuffer)
	}
	shortSide := streamScratch
	shortSide.SideData.CDEFIndexMap = shortSide.SideData.CDEFIndexMap[:streamSize.Event.SideData.CDEFIndexMap-1]
	if _, err := decode.BindDecoderFrameWorkResidualStreamRunner(streamSize, &stream, eventRunner, shortSide); !errors.Is(err, av1.ErrFrameShortBuffer) {
		t.Fatalf("short side-data err=%v want %v", err, av1.ErrFrameShortBuffer)
	}
	shortOutputs := streamScratch
	shortOutputs.Outputs = shortOutputs.Outputs[:streamSize.Event.Outputs-1]
	if _, err := decode.BindDecoderFrameWorkResidualStreamRunner(streamSize, &stream, eventRunner, shortOutputs); !errors.Is(err, av1.ErrFrameShortBuffer) {
		t.Fatalf("short output err=%v want %v", err, av1.ErrFrameShortBuffer)
	}
}

func TestPublicBindDecoderFrameWorkResidualStreamEventRunner(t *testing.T) {
	workerPool, err := decode.NewTileWorkerPool(1)
	if err != nil {
		t.Fatal(err)
	}
	defer workerPool.Close()

	payloads := [...][]byte{
		publicDecoderResidualRTPPayload(),
		publicDecoderResidualRTPFramePayload(),
	}
	var probeStream decode.DecoderStream
	var probeEvents [4]decode.DecoderEvent
	var probeRTPBuffer [256]byte
	var probeRTPSpans [4]rtp.RTPObuSpan
	var scratchSpans [1]av1.TileSpan
	var scratchJobs [1]av1.TileJob
	var scratchBatches [1]decode.TileBatch
	plan, err := decode.DecoderFrameWorkResidualRTPPayloadsStreamPlan(probeStream, 0, payloads[:], 1, probeRTPBuffer[:], probeRTPSpans[:], probeEvents[:], scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}
	size := plan.Size
	if !plan.HasEvent() ||
		plan.Bind.EventIndex != 1 ||
		plan.Bind.Event.Kind != decode.DecoderEventTileGroup {
		t.Fatalf("stream event bind plan=%+v", plan)
	}
	sequence := plan.Bind.Sequence
	event := plan.Bind.Event

	pool := publicDecoderPostFilterFramePool(t, av1.FrameFormat{
		Width:        int(event.FrameSize.CodedWidth),
		Height:       int(event.FrameSize.Height),
		BitDepth:     8,
		MonoChrome:   true,
		SubsamplingX: true,
		SubsamplingY: true,
		Align:        64,
	}, 2)
	var stream decode.DecoderStream
	var refs decode.DecoderSurfaceReferences
	var state decode.DecoderFrameWorkState
	var referenceSurfaces [av1.InterRefsPerFrame]int
	var referenceFrames [av1.InterRefsPerFrame]*av1.Frame
	var releases [av1.RefFrames]int
	var stats decode.DecoderFrameWorkTileResidualStats
	var side decode.DecoderFrameWorkSideData
	var batchRunner decode.DecoderFrameWorkBatchResidualRunner
	scratch := publicDecoderResidualStreamScratch(size)
	runner, boundSide, err := decode.BindDecoderFrameWorkResidualStreamEventRunner(size, &stream, sequence, &event, decode.DecoderFrameWorkResidualEventRuntime{
		State:             &state,
		Refs:              &refs,
		FramePool:         &pool,
		Align:             64,
		ReferenceSurfaces: referenceSurfaces[:],
		ReferenceFrames:   referenceFrames[:],
		Releases:          releases[:],
		WorkerPool:        workerPool,
		SideData:          &side,
		Stats:             &stats,
	}, scratch, &batchRunner)
	if err != nil {
		t.Fatal(err)
	}
	if runner.Stream != &stream ||
		runner.EventRunner.BatchRunner != &batchRunner ||
		len(runner.EventRunner.Spans) != size.Event.Plan.SpanCount ||
		len(runner.SideDataScratch.CDEFIndexMap) != size.Event.SideData.CDEFIndexMap ||
		len(runner.EventRunner.Outputs) != size.Event.Outputs ||
		len(boundSide.CDEFIndexMap.Index) == 0 ||
		len(boundSide.LoopFilterMap.Records) == 0 {
		t.Fatalf("combined runner=%+v side=%+v size=%+v", runner, boundSide, size)
	}

	result, err := runner.RunRTPPayloads(payloads[:], nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.EventCount != 5 ||
		result.Run.CompletedFrames != 2 ||
		result.Run.OutputCount != 2 ||
		len(result.Run.Outputs) != 2 ||
		result.Run.Outputs[0] == nil ||
		result.Run.Outputs[1] != result.Run.Last.Output ||
		stats.TXBs == 0 ||
		stats.Residuals == 0 {
		t.Fatalf("combined run result=%+v stats=%+v", result, stats)
	}

	if _, _, err := decode.BindDecoderFrameWorkResidualStreamEventRunner(size, nil, sequence, &event, decode.DecoderFrameWorkResidualEventRuntime{}, scratch, &batchRunner); !errors.Is(err, decode.ErrDecoderInvalidFrameWorkState) {
		t.Fatalf("nil stream err=%v want %v", err, decode.ErrDecoderInvalidFrameWorkState)
	}
	var untouchedBatch decode.DecoderFrameWorkBatchResidualRunner
	shortEvents := scratch
	shortEvents.Events = shortEvents.Events[:size.Events-1]
	if _, _, err := decode.BindDecoderFrameWorkResidualStreamEventRunner(size, &stream, sequence, &event, decode.DecoderFrameWorkResidualEventRuntime{}, shortEvents, &untouchedBatch); !errors.Is(err, decode.ErrDecoderEventBufferTooSmall) {
		t.Fatalf("short stream event scratch err=%v want %v", err, decode.ErrDecoderEventBufferTooSmall)
	}
	if untouchedBatch.UseDefaultPrediction || len(untouchedBatch.States) != 0 {
		t.Fatalf("short stream scratch mutated batch runner: %+v", untouchedBatch)
	}
	shortEvent := scratch
	shortEvent.Event.Spans = shortEvent.Event.Spans[:size.Event.Plan.SpanCount-1]
	if _, _, err := decode.BindDecoderFrameWorkResidualStreamEventRunner(size, &stream, sequence, &event, decode.DecoderFrameWorkResidualEventRuntime{}, shortEvent, &batchRunner); !errors.Is(err, av1.ErrFrameShortBuffer) {
		t.Fatalf("short event scratch err=%v want %v", err, av1.ErrFrameShortBuffer)
	}
	if _, _, err := decode.BindDecoderFrameWorkResidualStreamEventRunner(size, &stream, sequence, &event, decode.DecoderFrameWorkResidualEventRuntime{}, scratch, nil); !errors.Is(err, decode.ErrThreadingInvalidBatch) {
		t.Fatalf("nil batch runner err=%v want %v", err, decode.ErrThreadingInvalidBatch)
	}
}

func TestPublicBindDecoderFrameWorkResidualStreamPlanRunner(t *testing.T) {
	workerPool, err := decode.NewTileWorkerPool(1)
	if err != nil {
		t.Fatal(err)
	}
	defer workerPool.Close()

	payloads := [...][]byte{
		publicDecoderResidualRTPPayload(),
		publicDecoderResidualRTPFramePayload(),
	}
	var probeStream decode.DecoderStream
	var probeEvents [4]decode.DecoderEvent
	var probeRTPBuffer [256]byte
	var probeRTPSpans [4]rtp.RTPObuSpan
	var scratchSpans [1]av1.TileSpan
	var scratchJobs [1]av1.TileJob
	var scratchBatches [1]decode.TileBatch
	plan, err := decode.DecoderFrameWorkResidualRTPPayloadsStreamPlan(probeStream, 0, payloads[:], 1, probeRTPBuffer[:], probeRTPSpans[:], probeEvents[:], scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}
	if !plan.HasEvent() || plan.Bind.Event.Kind != decode.DecoderEventTileGroup || plan.Size.Event.Outputs != 2 {
		t.Fatalf("stream plan=%+v", plan)
	}

	pool := publicDecoderPostFilterFramePool(t, av1.FrameFormat{
		Width:        int(plan.Bind.Event.FrameSize.CodedWidth),
		Height:       int(plan.Bind.Event.FrameSize.Height),
		BitDepth:     8,
		MonoChrome:   true,
		SubsamplingX: true,
		SubsamplingY: true,
		Align:        64,
	}, 2)
	var stream decode.DecoderStream
	var refs decode.DecoderSurfaceReferences
	var state decode.DecoderFrameWorkState
	var referenceSurfaces [av1.InterRefsPerFrame]int
	var referenceFrames [av1.InterRefsPerFrame]*av1.Frame
	var releases [av1.RefFrames]int
	var stats decode.DecoderFrameWorkTileResidualStats
	var side decode.DecoderFrameWorkSideData
	var batchRunner decode.DecoderFrameWorkBatchResidualRunner
	scratch := publicDecoderResidualStreamScratch(plan.Size)
	runner, boundSide, err := decode.BindDecoderFrameWorkResidualStreamPlanRunner(plan, &stream, decode.DecoderFrameWorkResidualEventRuntime{
		State:             &state,
		Refs:              &refs,
		FramePool:         &pool,
		Align:             64,
		ReferenceSurfaces: referenceSurfaces[:],
		ReferenceFrames:   referenceFrames[:],
		Releases:          releases[:],
		WorkerPool:        workerPool,
		SideData:          &side,
		Stats:             &stats,
	}, scratch, &batchRunner)
	if err != nil {
		t.Fatal(err)
	}
	if runner.Stream != &stream ||
		runner.EventRunner.BatchRunner != &batchRunner ||
		len(runner.Events) != plan.Size.Events ||
		len(runner.EventRunner.Outputs) != plan.Size.Event.Outputs ||
		len(boundSide.CDEFIndexMap.Index) == 0 ||
		len(boundSide.LoopFilterMap.Records) == 0 {
		t.Fatalf("plan-bound runner=%+v side=%+v plan=%+v", runner, boundSide, plan)
	}

	result, err := runner.RunRTPPayloads(payloads[:], nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.EventCount != 5 ||
		result.Run.CompletedFrames != 2 ||
		result.Run.OutputCount != 2 ||
		result.Run.Outputs[0] == nil ||
		result.Run.Outputs[1] != result.Run.Last.Output ||
		stats.TXBs == 0 ||
		stats.Residuals == 0 {
		t.Fatalf("plan-bound result=%+v stats=%+v", result, stats)
	}

	if _, _, err := decode.BindDecoderFrameWorkResidualStreamPlanRunner(plan, nil, decode.DecoderFrameWorkResidualEventRuntime{}, scratch, &batchRunner); !errors.Is(err, decode.ErrDecoderInvalidFrameWorkState) {
		t.Fatalf("nil stream plan bind err=%v want %v", err, decode.ErrDecoderInvalidFrameWorkState)
	}
}

func TestPublicDecoderFrameWorkResidualStreamPlanRunnerSupportedPostFilter(t *testing.T) {
	workerPool, err := decode.NewTileWorkerPool(1)
	if err != nil {
		t.Fatal(err)
	}
	defer workerPool.Close()

	payload := publicDecoderResidualRTPPayload()
	var probeStream decode.DecoderStream
	var probeEvents [4]decode.DecoderEvent
	var probeRTPBuffer [128]byte
	var probeRTPSpans [4]rtp.RTPObuSpan
	var scratchSpans [1]av1.TileSpan
	var scratchJobs [1]av1.TileJob
	var scratchBatches [1]decode.TileBatch
	plan, err := decode.DecoderFrameWorkResidualRTPPayloadStreamPlan(probeStream, 0, payload, 1, probeRTPBuffer[:], probeRTPSpans[:], probeEvents[:], scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}
	if !plan.HasEvent() || plan.Bind.Event.Kind != decode.DecoderEventTileGroup || plan.Size.Event.Outputs != 1 {
		t.Fatalf("stream plan=%+v", plan)
	}

	pool := publicDecoderPostFilterFramePool(t, av1.FrameFormat{
		Width:        int(plan.Bind.Event.FrameSize.CodedWidth),
		Height:       int(plan.Bind.Event.FrameSize.Height),
		BitDepth:     8,
		MonoChrome:   true,
		SubsamplingX: true,
		SubsamplingY: true,
		Align:        64,
	}, 1)
	var stream decode.DecoderStream
	var refs decode.DecoderSurfaceReferences
	var state decode.DecoderFrameWorkState
	var referenceSurfaces [av1.InterRefsPerFrame]int
	var referenceFrames [av1.InterRefsPerFrame]*av1.Frame
	var releases [av1.RefFrames]int
	var stats decode.DecoderFrameWorkTileResidualStats
	var side decode.DecoderFrameWorkSideData
	var batchRunner decode.DecoderFrameWorkBatchResidualRunner
	scratch := publicDecoderResidualStreamScratch(plan.Size)
	runner, boundSide, err := decode.BindDecoderFrameWorkResidualStreamPlanRunner(plan, &stream, decode.DecoderFrameWorkResidualEventRuntime{
		State:             &state,
		Refs:              &refs,
		FramePool:         &pool,
		Align:             64,
		ReferenceSurfaces: referenceSurfaces[:],
		ReferenceFrames:   referenceFrames[:],
		Releases:          releases[:],
		WorkerPool:        workerPool,
		SideData:          &side,
		Stats:             &stats,
	}, scratch, &batchRunner)
	if err != nil {
		t.Fatal(err)
	}

	var probeOutput av1.Frame
	probeCtx, err := decode.DecoderFrameWorkPostFilterScratchContext(plan.Bind.Sequence, &plan.Bind.Event, 64, &boundSide, &probeOutput)
	if err != nil {
		t.Fatal(err)
	}
	var probe decode.DecoderFrameWorkSupportedPostFilterScratchRunner
	exact, err := probe.ScratchLen(probeCtx)
	if err != nil {
		t.Fatal(err)
	}
	postRunner := decode.DecoderFrameWorkSupportedPostFilterScratchRunner{
		Scratch: publicDecoderPostFilterRequestScratch(decode.DecoderFrameWorkPostFilterRequestScratchLen(exact)),
	}

	result, err := runner.RunRTPPayloadWithPostFilterRunner(payload, &postRunner)
	if err != nil {
		t.Fatal(err)
	}
	cdefRead := false
	for _, read := range side.CDEFIndexMap.Read {
		cdefRead = cdefRead || read
	}
	if result.EventCount != 3 ||
		result.RTPUsed != 0 ||
		result.Run.CompletedFrames != 1 ||
		result.Run.OutputCount != 1 ||
		result.Run.Outputs[0] != result.Run.Last.Output ||
		result.Run.Last.Output == nil ||
		postRunner.Result.Completed != 0 ||
		postRunner.Context.RemainingPostFilters() != 0 ||
		postRunner.Context.Output != result.Run.Last.Output ||
		!cdefRead ||
		stats.TXBs == 0 ||
		stats.Residuals == 0 {
		t.Fatalf("postfilter stream result=%+v post=%+v stats=%+v cdefRead=%v", result, postRunner, stats, cdefRead)
	}
	if _, ok := refs.ReferenceSlot(0); !ok {
		t.Fatal("postfilter stream runner did not publish decoded frame")
	}
}

func TestPublicDecoderFrameWorkResidualStreamScratchLenLowOverhead(t *testing.T) {
	payload := publicDecoderResidualLowOverheadStream()
	var stream decode.DecoderStream
	var events [4]decode.DecoderEvent
	var scratchSpans [1]av1.TileSpan
	var scratchJobs [1]av1.TileJob
	var scratchBatches [1]decode.TileBatch
	size, err := decode.DecoderFrameWorkResidualLowOverheadStreamScratchLen(stream, payload, 1, events[:], scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}
	if size.Events != 3 ||
		size.RTPBuffer != 0 ||
		size.RTPSpans != 0 ||
		size.Event.Plan != (decode.DecoderTileWorkPlan{SpanCount: 1, JobCount: 1, BatchCount: 1}) ||
		size.Event.Outputs != 1 ||
		size.Event.Runner.Workers != 1 ||
		size.Event.SideData.CDEFIndexMap == 0 ||
		size.Event.SideData.LoopFilterMap == 0 {
		t.Fatalf("low-overhead stream scratch size=%+v", size)
	}
	if stream.HasSequenceHeader() {
		t.Fatal("low-overhead stream scratch sizing mutated caller stream")
	}
	if events[0].Kind != decode.DecoderEventSequenceHeader ||
		events[1].Kind != decode.DecoderEventFrameHeader ||
		events[2].Kind != decode.DecoderEventTileGroup {
		t.Fatalf("events=%+v", events[:3])
	}

	short, err := decode.DecoderFrameWorkResidualLowOverheadStreamScratchLen(stream, payload, 1, events[:2], scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if !errors.Is(err, decode.ErrDecoderEventBufferTooSmall) {
		t.Fatalf("short events err=%v want %v", err, decode.ErrDecoderEventBufferTooSmall)
	}
	if short.Events != 3 || short.Event != (decode.DecoderFrameWorkResidualEventScratchSize{}) {
		t.Fatalf("short low-overhead scratch size=%+v", short)
	}
}

func TestPublicDecoderFrameWorkResidualStreamScratchLenRTPPayload(t *testing.T) {
	payload := publicDecoderResidualRTPPayload()
	var stream decode.DecoderStream
	var events [4]decode.DecoderEvent
	var rtpBuffer [128]byte
	var rtpSpans [4]rtp.RTPObuSpan
	var scratchSpans [1]av1.TileSpan
	var scratchJobs [1]av1.TileJob
	var scratchBatches [1]decode.TileBatch
	size, err := decode.DecoderFrameWorkResidualRTPPayloadStreamScratchLen(stream, 0, payload, 1, rtpBuffer[:], rtpSpans[:], events[:], scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}
	if size.Events != 3 ||
		size.RTPBuffer == 0 ||
		size.RTPSpans != 3 ||
		size.Event.Plan != (decode.DecoderTileWorkPlan{SpanCount: 1, JobCount: 1, BatchCount: 1}) ||
		size.Event.Outputs != 1 ||
		size.Event.Runner.Workers != 1 ||
		size.Event.SideData.CDEFIndexMap == 0 ||
		size.Event.SideData.LoopFilterMap == 0 {
		t.Fatalf("rtp stream scratch size=%+v", size)
	}
	if stream.HasSequenceHeader() || stream.InRTPFragment() {
		t.Fatal("RTP stream scratch sizing mutated caller stream")
	}
	if events[0].Kind != decode.DecoderEventSequenceHeader ||
		events[1].Kind != decode.DecoderEventFrameHeader ||
		events[2].Kind != decode.DecoderEventTileGroup ||
		rtpSpans[0].Length == 0 ||
		rtpSpans[2].Length == 0 {
		t.Fatalf("events=%+v spans=%+v", events[:3], rtpSpans[:3])
	}

	shortRTP, err := decode.DecoderFrameWorkResidualRTPPayloadStreamScratchLen(stream, 0, payload, 1, rtpBuffer[:size.RTPBuffer-1], rtpSpans[:], events[:], scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if !errors.Is(err, rtp.ErrRTPShortBuffer) {
		t.Fatalf("short rtp buffer err=%v want %v", err, rtp.ErrRTPShortBuffer)
	}
	if shortRTP.Events != 3 || shortRTP.RTPBuffer != size.RTPBuffer || shortRTP.RTPSpans != 3 {
		t.Fatalf("short rtp scratch size=%+v want buffer %d", shortRTP, size.RTPBuffer)
	}

	shortEvents, err := decode.DecoderFrameWorkResidualRTPPayloadStreamScratchLen(stream, 0, payload, 1, rtpBuffer[:], rtpSpans[:], events[:2], scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if !errors.Is(err, decode.ErrDecoderEventBufferTooSmall) {
		t.Fatalf("short events err=%v want %v", err, decode.ErrDecoderEventBufferTooSmall)
	}
	if shortEvents.Events != 3 || shortEvents.RTPBuffer != size.RTPBuffer || shortEvents.RTPSpans != 3 {
		t.Fatalf("short events scratch size=%+v want buffer %d", shortEvents, size.RTPBuffer)
	}
}

func TestPublicDecoderFrameWorkResidualStreamScratchLenRTPPayloads(t *testing.T) {
	payload := publicDecoderResidualRTPPayload()
	payloads := publicDecoderResidualFragmentedRTPPayloads()
	var stream decode.DecoderStream
	var events [4]decode.DecoderEvent
	var rtpBuffer [128]byte
	var rtpSpans [4]rtp.RTPObuSpan
	var scratchSpans [1]av1.TileSpan
	var scratchJobs [1]av1.TileJob
	var scratchBatches [1]decode.TileBatch
	single, err := decode.DecoderFrameWorkResidualRTPPayloadStreamScratchLen(stream, 0, payload, 1, rtpBuffer[:], rtpSpans[:], events[:], scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}
	size, err := decode.DecoderFrameWorkResidualRTPPayloadsStreamScratchLen(stream, 0, payloads, 1, rtpBuffer[:], rtpSpans[:], events[:], scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}
	if size.Events != 1 ||
		size.RTPBuffer == 0 ||
		size.RTPSpans != 1 ||
		size.Event != single.Event {
		t.Fatalf("rtp payload batch scratch size=%+v single=%+v", size, single)
	}
	if stream.HasSequenceHeader() || stream.InRTPFragment() {
		t.Fatal("RTP payload batch scratch sizing mutated caller stream")
	}
	if events[0].Kind != decode.DecoderEventTileGroup {
		t.Fatalf("last sized event=%+v", events[0])
	}

	shortRTP, err := decode.DecoderFrameWorkResidualRTPPayloadsStreamScratchLen(stream, 0, payloads, 1, rtpBuffer[:size.RTPBuffer-1], rtpSpans[:], events[:], scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if !errors.Is(err, rtp.ErrRTPShortBuffer) {
		t.Fatalf("short rtp buffer err=%v want %v", err, rtp.ErrRTPShortBuffer)
	}
	if shortRTP.RTPBuffer != size.RTPBuffer || shortRTP.Events != 1 || shortRTP.RTPSpans != 1 {
		t.Fatalf("short rtp payload batch scratch size=%+v want buffer %d", shortRTP, size.RTPBuffer)
	}

	shortEvents, err := decode.DecoderFrameWorkResidualRTPPayloadsStreamScratchLen(stream, 0, payloads, 1, rtpBuffer[:], rtpSpans[:], events[:0], scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if !errors.Is(err, decode.ErrDecoderEventBufferTooSmall) {
		t.Fatalf("short events err=%v want %v", err, decode.ErrDecoderEventBufferTooSmall)
	}
	if shortEvents.Events != 1 || shortEvents.RTPBuffer == 0 || shortEvents.RTPSpans != 1 {
		t.Fatalf("short events payload batch scratch size=%+v", shortEvents)
	}
}

func TestPublicDecoderFrameWorkResidualStreamScratchLenRTPPayloadsMultipleOutputs(t *testing.T) {
	payloads := [...][]byte{
		publicDecoderResidualRTPPayload(),
		publicDecoderResidualRTPFramePayload(),
	}
	var stream decode.DecoderStream
	var events [4]decode.DecoderEvent
	var rtpBuffer [256]byte
	var rtpSpans [4]rtp.RTPObuSpan
	var scratchSpans [1]av1.TileSpan
	var scratchJobs [1]av1.TileJob
	var scratchBatches [1]decode.TileBatch
	size, err := decode.DecoderFrameWorkResidualRTPPayloadsStreamScratchLen(stream, 0, payloads[:], 1, rtpBuffer[:], rtpSpans[:], events[:], scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}
	if size.Events != 3 ||
		size.RTPBuffer == 0 ||
		size.RTPSpans != 3 ||
		size.Event.Plan != (decode.DecoderTileWorkPlan{SpanCount: 1, JobCount: 1, BatchCount: 1}) ||
		size.Event.Outputs != 2 ||
		size.Event.Runner.Workers != 1 ||
		size.Event.SideData.CDEFIndexMap == 0 ||
		size.Event.SideData.LoopFilterMap == 0 {
		t.Fatalf("multi-output RTP payload batch scratch size=%+v", size)
	}
	if stream.HasSequenceHeader() || stream.InRTPFragment() {
		t.Fatal("multi-output RTP payload batch scratch sizing mutated caller stream")
	}
	if events[0].Kind != decode.DecoderEventFrameHeader || events[1].Kind != decode.DecoderEventTileGroup {
		t.Fatalf("last multi-output RTP batch events=%+v", events[:2])
	}
}

func TestPublicDecoderFrameWorkResidualStreamScratchLenRTPFragment(t *testing.T) {
	var stream decode.DecoderStream
	var events [2]decode.DecoderEvent
	var rtpBuffer [128]byte
	var rtpSpans [2]rtp.RTPObuSpan

	sequenceElement := [...]rtp.RTPElement{
		{Data: publicDecoderResidualRTPElement(av1.OBUSequenceHeader, publicDecoderResidualSequenceHeaderPayload())},
	}
	var sequencePayload [128]byte
	n, err := rtp.PutRTPPayload(sequencePayload[:], rtp.RTPAggregationHeader{
		ElementCount:                1,
		StartsNewCodedVideoSequence: true,
	}, sequenceElement[:])
	if err != nil {
		t.Fatal(err)
	}
	used, count, err := stream.PushRTPPayload(rtpBuffer[:], 0, rtpSpans[:], events[:], sequencePayload[:n])
	if err != nil {
		t.Fatal(err)
	}
	if used == 0 || count != 1 || events[0].Kind != decode.DecoderEventSequenceHeader || !stream.HasSequenceHeader() {
		t.Fatalf("sequence used=%d count=%d event=%+v hasSequence=%v", used, count, events[0], stream.HasSequenceHeader())
	}

	frameHeader := publicDecoderResidualRTPElement(av1.OBUFrameHeader, publicDecoderResidualFrameHeaderPayload())
	var packet [8]byte
	n, next, more, err := rtp.PutRTPFragment(packet[:], frameHeader, 0, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if !more {
		t.Fatal("expected frame header to need a second fragment")
	}
	used = 0
	var scratchSpans [1]av1.TileSpan
	var scratchJobs [1]av1.TileJob
	var scratchBatches [1]decode.TileBatch
	first, err := decode.DecoderFrameWorkResidualRTPPayloadStreamScratchLen(stream, used, packet[:n], 1, rtpBuffer[:], rtpSpans[:], events[:], scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}
	if first.Events != 0 || first.RTPBuffer == 0 || first.RTPSpans != 0 || first.Event != (decode.DecoderFrameWorkResidualEventScratchSize{}) {
		t.Fatalf("first fragment scratch size=%+v", first)
	}
	if stream.InRTPFragment() {
		t.Fatal("first fragment sizing mutated stream RTP fragment state")
	}
	used, count, err = stream.PushRTPPayload(rtpBuffer[:], used, rtpSpans[:], events[:], packet[:n])
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 || !stream.InRTPFragment() {
		t.Fatalf("first fragment push used=%d count=%d inFragment=%v", used, count, stream.InRTPFragment())
	}

	n, _, more, err = rtp.PutRTPFragment(packet[:], frameHeader, next, len(packet), false)
	if err != nil {
		t.Fatal(err)
	}
	if more {
		t.Fatal("unexpected third fragment")
	}
	final, err := decode.DecoderFrameWorkResidualRTPPayloadStreamScratchLen(stream, used, packet[:n], 1, rtpBuffer[:], rtpSpans[:], events[:], scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}
	if final.Events != 1 ||
		final.RTPBuffer != len(frameHeader) ||
		final.RTPSpans != 1 ||
		final.Event.Plan != (decode.DecoderTileWorkPlan{}) ||
		final.Event.SideData.CDEFIndexMap == 0 ||
		events[0].Kind != decode.DecoderEventFrameHeader {
		t.Fatalf("final fragment scratch size=%+v event=%+v", final, events[0])
	}
	if !stream.InRTPFragment() {
		t.Fatal("final fragment sizing mutated live stream RTP state")
	}
}

func TestPublicDecoderFrameWorkResidualStreamRunnerAllocs(t *testing.T) {
	workerPool, err := decode.NewTileWorkerPool(1)
	if err != nil {
		t.Fatal(err)
	}
	defer workerPool.Close()

	lowOverhead := publicDecoderResidualLowOverheadStream()
	lowOverheads := [...][]byte{
		publicDecoderResidualLowOverheadStream(),
		publicDecoderResidualLowOverheadFrameStream(),
	}
	rtpPayload := publicDecoderResidualRTPPayload()
	rtpPayloads := publicDecoderResidualFragmentedRTPPayloads()
	var probeStream decode.DecoderStream
	var probeEvents [4]decode.DecoderEvent
	count, err := probeStream.PushLowOverhead(lowOverhead, probeEvents[:])
	if err != nil {
		t.Fatal(err)
	}
	sequence, ok := probeStream.SequenceHeader()
	if !ok {
		t.Fatal("probe stream missing sequence")
	}
	var scratchSpans [1]av1.TileSpan
	var scratchJobs [1]av1.TileJob
	var scratchBatches [1]decode.TileBatch
	size, err := decode.DecoderFrameWorkResidualEventsScratchLen(sequence, probeEvents[:count], 1, scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}
	pool := publicDecoderPostFilterFramePool(t, av1.FrameFormat{
		Width:        int(probeEvents[count-1].FrameSize.CodedWidth),
		Height:       int(probeEvents[count-1].FrameSize.Height),
		BitDepth:     8,
		MonoChrome:   true,
		SubsamplingX: true,
		SubsamplingY: true,
		Align:        64,
	}, 2)
	var stream decode.DecoderStream
	var refs decode.DecoderSurfaceReferences
	var state decode.DecoderFrameWorkState
	var referenceSurfaces [av1.InterRefsPerFrame]int
	var referenceFrames [av1.InterRefsPerFrame]*av1.Frame
	var releases [av1.RefFrames]int
	var stats decode.DecoderFrameWorkTileResidualStats
	var side decode.DecoderFrameWorkSideData
	var batchRunner decode.DecoderFrameWorkBatchResidualRunner
	var outputs [2]*av1.Frame
	scratch := publicDecoderResidualEventScratch(size)
	eventRunner, _, err := decode.BindDecoderFrameWorkResidualEventRunner(size, sequence, &probeEvents[count-1], decode.DecoderFrameWorkResidualEventRuntime{
		State:             &state,
		Refs:              &refs,
		FramePool:         &pool,
		Align:             64,
		ReferenceSurfaces: referenceSurfaces[:],
		ReferenceFrames:   referenceFrames[:],
		Releases:          releases[:],
		WorkerPool:        workerPool,
		SideData:          &side,
		Stats:             &stats,
		Outputs:           outputs[:],
	}, scratch, &batchRunner)
	if err != nil {
		t.Fatal(err)
	}
	var events [4]decode.DecoderEvent
	var rtpBuffer [128]byte
	var rtpSpans [4]rtp.RTPObuSpan
	runner := decode.DecoderFrameWorkResidualStreamRunner{
		Stream:          &stream,
		EventRunner:     eventRunner,
		Events:          events[:],
		SideDataScratch: scratch.SideData,
		RTPBuffer:       rtpBuffer[:],
		RTPSpans:        rtpSpans[:],
	}

	allocs := testing.AllocsPerRun(1000, func() {
		pool.Reset()
		refs.Reset()
		state.Reset()
		if resetErr := runner.Reset(); resetErr != nil {
			err = resetErr
			return
		}
		result, runErr := runner.RunLowOverhead(lowOverhead, nil)
		if runErr != nil {
			err = runErr
			return
		}
		if result.Run.CompletedFrames != 1 || result.Run.OutputCount != 1 || outputs[0] != result.Run.Last.Output || stats.TXBs == 0 {
			err = decode.ErrThreadingInvalidBatch
			return
		}

		pool.Reset()
		refs.Reset()
		state.Reset()
		if resetErr := runner.Reset(); resetErr != nil {
			err = resetErr
			return
		}
		result, runErr = runner.RunLowOverheads(lowOverheads[:], nil)
		if runErr != nil {
			err = runErr
			return
		}
		if result.Run.CompletedFrames != 2 || result.Run.OutputCount != 2 || outputs[1] != result.Run.Last.Output || stats.TXBs == 0 {
			err = decode.ErrThreadingInvalidBatch
			return
		}

		pool.Reset()
		refs.Reset()
		state.Reset()
		if resetErr := runner.Reset(); resetErr != nil {
			err = resetErr
			return
		}
		result = decode.DecoderFrameWorkResidualStreamResult{}
		if runErr = runner.RunLowOverheadsInto(&result, lowOverheads[:], nil); runErr != nil {
			err = runErr
			return
		}
		if result.Run.CompletedFrames != 2 || result.Run.OutputCount != 2 || outputs[1] != result.Run.Last.Output || stats.TXBs == 0 {
			err = decode.ErrThreadingInvalidBatch
			return
		}

		pool.Reset()
		refs.Reset()
		state.Reset()
		if resetErr := runner.Reset(); resetErr != nil {
			err = resetErr
			return
		}
		result, runErr = runner.RunRTPPayload(rtpPayload, nil)
		if runErr != nil {
			err = runErr
			return
		}
		if result.Run.CompletedFrames != 1 || result.Run.OutputCount != 1 || outputs[0] != result.Run.Last.Output || stats.TXBs == 0 || runner.RTPUsed != 0 {
			err = decode.ErrThreadingInvalidBatch
		}

		pool.Reset()
		refs.Reset()
		state.Reset()
		if resetErr := runner.Reset(); resetErr != nil {
			err = resetErr
			return
		}
		result, runErr = runner.RunRTPPayloads(rtpPayloads, nil)
		if runErr != nil {
			err = runErr
			return
		}
		if result.Run.CompletedFrames != 1 || result.Run.OutputCount != 1 || outputs[0] != result.Run.Last.Output || stats.TXBs == 0 || runner.RTPUsed != 0 {
			err = decode.ErrThreadingInvalidBatch
			return
		}

		pool.Reset()
		refs.Reset()
		state.Reset()
		if resetErr := runner.Reset(); resetErr != nil {
			err = resetErr
			return
		}
		result = decode.DecoderFrameWorkResidualStreamResult{}
		if runErr = runner.RunRTPPayloadsInto(&result, rtpPayloads, nil); runErr != nil {
			err = runErr
			return
		}
		if result.Run.CompletedFrames != 1 || result.Run.OutputCount != 1 || outputs[0] != result.Run.Last.Output || stats.TXBs == 0 || runner.RTPUsed != 0 {
			err = decode.ErrThreadingInvalidBatch
			return
		}

		pool.Reset()
		refs.Reset()
		state.Reset()
		if resetErr := runner.Reset(); resetErr != nil {
			err = resetErr
			return
		}
		result = decode.DecoderFrameWorkResidualStreamResult{}
		if runErr = runner.RunRTPPayloadsInto(&result, rtpPayloads[:2], nil); runErr != nil {
			err = runErr
			return
		}
		if runner.RTPUsed == 0 {
			err = decode.ErrThreadingInvalidBatch
			return
		}
		if runErr = runner.RunRTPPayloadAfterLossInto(&result, rtpPayload, nil); runErr != nil {
			err = runErr
			return
		}
		if result.Run.CompletedFrames != 1 || result.Run.OutputCount != 1 || outputs[0] != result.Run.Last.Output || stats.TXBs == 0 || runner.RTPUsed != 0 {
			err = decode.ErrThreadingInvalidBatch
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if allocs != 0 {
		t.Fatalf("DecoderFrameWorkResidualStreamRunner allocated: %f", allocs)
	}
}

func TestPublicDecoderFrameWorkResidualStreamPostFilterRunnerAllocs(t *testing.T) {
	workerPool, err := decode.NewTileWorkerPool(1)
	if err != nil {
		t.Fatal(err)
	}
	defer workerPool.Close()

	payload := publicDecoderResidualRTPPayload()
	var probeStream decode.DecoderStream
	var probeEvents [4]decode.DecoderEvent
	var probeRTPBuffer [128]byte
	var probeRTPSpans [4]rtp.RTPObuSpan
	var scratchSpans [1]av1.TileSpan
	var scratchJobs [1]av1.TileJob
	var scratchBatches [1]decode.TileBatch
	plan, err := decode.DecoderFrameWorkResidualRTPPayloadStreamPlan(probeStream, 0, payload, 1, probeRTPBuffer[:], probeRTPSpans[:], probeEvents[:], scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}

	pool := publicDecoderPostFilterFramePool(t, av1.FrameFormat{
		Width:        int(plan.Bind.Event.FrameSize.CodedWidth),
		Height:       int(plan.Bind.Event.FrameSize.Height),
		BitDepth:     8,
		MonoChrome:   true,
		SubsamplingX: true,
		SubsamplingY: true,
		Align:        64,
	}, 1)
	var stream decode.DecoderStream
	var refs decode.DecoderSurfaceReferences
	var state decode.DecoderFrameWorkState
	var referenceSurfaces [av1.InterRefsPerFrame]int
	var referenceFrames [av1.InterRefsPerFrame]*av1.Frame
	var releases [av1.RefFrames]int
	var stats decode.DecoderFrameWorkTileResidualStats
	var side decode.DecoderFrameWorkSideData
	var batchRunner decode.DecoderFrameWorkBatchResidualRunner
	scratch := publicDecoderResidualStreamScratch(plan.Size)
	runner, boundSide, err := decode.BindDecoderFrameWorkResidualStreamPlanRunner(plan, &stream, decode.DecoderFrameWorkResidualEventRuntime{
		State:             &state,
		Refs:              &refs,
		FramePool:         &pool,
		Align:             64,
		ReferenceSurfaces: referenceSurfaces[:],
		ReferenceFrames:   referenceFrames[:],
		Releases:          releases[:],
		WorkerPool:        workerPool,
		SideData:          &side,
		Stats:             &stats,
	}, scratch, &batchRunner)
	if err != nil {
		t.Fatal(err)
	}

	var probeOutput av1.Frame
	probeCtx, err := decode.DecoderFrameWorkPostFilterScratchContext(plan.Bind.Sequence, &plan.Bind.Event, 64, &boundSide, &probeOutput)
	if err != nil {
		t.Fatal(err)
	}
	var probe decode.DecoderFrameWorkSupportedPostFilterScratchRunner
	exact, err := probe.ScratchLen(probeCtx)
	if err != nil {
		t.Fatal(err)
	}
	postScratch := publicDecoderPostFilterRequestScratch(decode.DecoderFrameWorkPostFilterRequestScratchLen(exact))
	postRunner := decode.DecoderFrameWorkSupportedPostFilterScratchRunner{Scratch: postScratch}

	allocs := testing.AllocsPerRun(1000, func() {
		pool.Reset()
		refs.Reset()
		state.Reset()
		if resetErr := runner.Reset(); resetErr != nil {
			err = resetErr
			return
		}
		postRunner = decode.DecoderFrameWorkSupportedPostFilterScratchRunner{Scratch: postScratch}
		result, runErr := runner.RunRTPPayloadWithPostFilterRunner(payload, &postRunner)
		if runErr != nil {
			err = runErr
			return
		}
		if result.EventCount != 3 ||
			result.Run.CompletedFrames != 1 ||
			result.Run.OutputCount != 1 ||
			result.Run.Last.Output == nil ||
			postRunner.Result.Completed != 0 ||
			postRunner.Context.RemainingPostFilters() != 0 ||
			stats.TXBs == 0 ||
			stats.Residuals == 0 {
			err = decode.ErrThreadingInvalidBatch
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if allocs != 0 {
		t.Fatalf("DecoderFrameWorkResidualStreamRunner postfilter allocated: %f", allocs)
	}
}

func TestPublicDecoderFrameWorkResidualStreamScratchLenAllocs(t *testing.T) {
	lowOverhead := publicDecoderResidualLowOverheadStream()
	lowOverheads := [...][]byte{
		publicDecoderResidualLowOverheadStream(),
		publicDecoderResidualLowOverheadFrameStream(),
	}
	rtpPayload := publicDecoderResidualRTPPayload()
	rtpPayloads := publicDecoderResidualFragmentedRTPPayloads()
	var events [4]decode.DecoderEvent
	var rtpBuffer [128]byte
	var rtpSpans [4]rtp.RTPObuSpan
	var scratchSpans [1]av1.TileSpan
	var scratchJobs [1]av1.TileJob
	var scratchBatches [1]decode.TileBatch
	var err error
	var lowSize decode.DecoderFrameWorkResidualStreamScratchSize
	var lowBatchSize decode.DecoderFrameWorkResidualStreamScratchSize
	var rtpSize decode.DecoderFrameWorkResidualStreamScratchSize
	var rtpPayloadsSize decode.DecoderFrameWorkResidualStreamScratchSize

	allocs := testing.AllocsPerRun(1000, func() {
		var stream decode.DecoderStream
		lowSize, err = decode.DecoderFrameWorkResidualLowOverheadStreamScratchLen(stream, lowOverhead, 1, events[:], scratchSpans[:], scratchJobs[:], scratchBatches[:])
		if err != nil {
			return
		}
		lowBatchSize, err = decode.DecoderFrameWorkResidualLowOverheadStreamsScratchLen(stream, lowOverheads[:], 1, events[:], scratchSpans[:], scratchJobs[:], scratchBatches[:])
		if err != nil {
			return
		}
		rtpSize, err = decode.DecoderFrameWorkResidualRTPPayloadStreamScratchLen(stream, 0, rtpPayload, 1, rtpBuffer[:], rtpSpans[:], events[:], scratchSpans[:], scratchJobs[:], scratchBatches[:])
		if err != nil {
			return
		}
		rtpPayloadsSize, err = decode.DecoderFrameWorkResidualRTPPayloadsStreamScratchLen(stream, 0, rtpPayloads, 1, rtpBuffer[:], rtpSpans[:], events[:], scratchSpans[:], scratchJobs[:], scratchBatches[:])
	})
	if err != nil {
		t.Fatal(err)
	}
	if lowSize.Events != 3 ||
		lowSize.Event.Outputs != 1 ||
		lowBatchSize.Events != 3 ||
		lowBatchSize.Event.Outputs != 2 ||
		rtpSize.Events != 3 ||
		rtpSize.RTPBuffer == 0 ||
		rtpSize.RTPSpans != 3 ||
		rtpSize.Event.Outputs != 1 ||
		rtpPayloadsSize.Events != 1 ||
		rtpPayloadsSize.RTPBuffer == 0 ||
		rtpPayloadsSize.RTPSpans != 1 ||
		rtpPayloadsSize.Event.Outputs != 1 {
		t.Fatalf("scratch sizes low=%+v lowBatch=%+v rtp=%+v payloads=%+v", lowSize, lowBatchSize, rtpSize, rtpPayloadsSize)
	}
	if allocs != 0 {
		t.Fatalf("DecoderFrameWorkResidualStreamScratchLen allocated: %f", allocs)
	}
}

func TestPublicDecoderFrameWorkResidualEventsBindPlanAllocs(t *testing.T) {
	sequence := av1.SequenceHeader{ColorConfig: av1.ColorConfig{
		BitDepth:   8,
		MonoChrome: true,
	}}
	events := [...]decode.DecoderEvent{
		{Kind: decode.DecoderEventSequenceHeader, SequenceHeader: sequence},
		{Kind: decode.DecoderEventFrameHeader, SequenceHeader: sequence},
		{Kind: decode.DecoderEventTileGroup, SequenceHeader: sequence},
	}
	var plan decode.DecoderFrameWorkResidualEventBindPlan
	allocs := testing.AllocsPerRun(1000, func() {
		plan = decode.DecoderFrameWorkResidualEventsBindPlan(av1.SequenceHeader{}, events[:])
	})
	if !plan.HasEvent() || plan.EventIndex != 2 || plan.Event.Kind != decode.DecoderEventTileGroup {
		t.Fatalf("bind plan=%+v", plan)
	}
	if allocs != 0 {
		t.Fatalf("DecoderFrameWorkResidualEventsBindPlan allocated: %f", allocs)
	}
}

func TestPublicDecoderFrameWorkResidualStreamPlanAllocs(t *testing.T) {
	lowOverhead := publicDecoderResidualLowOverheadStream()
	lowOverheads := [...][]byte{
		publicDecoderResidualLowOverheadStream(),
		publicDecoderResidualLowOverheadFrameStream(),
	}
	rtpPayload := publicDecoderResidualRTPPayload()
	rtpPayloads := [...][]byte{
		publicDecoderResidualRTPPayload(),
		publicDecoderResidualRTPFramePayload(),
	}
	var events [4]decode.DecoderEvent
	var rtpBuffer [256]byte
	var rtpSpans [4]rtp.RTPObuSpan
	var scratchSpans [1]av1.TileSpan
	var scratchJobs [1]av1.TileJob
	var scratchBatches [1]decode.TileBatch
	var err error
	var lowPlan decode.DecoderFrameWorkResidualStreamPlan
	var lowBatchPlan decode.DecoderFrameWorkResidualStreamPlan
	var rtpPlan decode.DecoderFrameWorkResidualStreamPlan
	var batchPlan decode.DecoderFrameWorkResidualStreamPlan

	allocs := testing.AllocsPerRun(1000, func() {
		var stream decode.DecoderStream
		lowPlan, err = decode.DecoderFrameWorkResidualLowOverheadStreamPlan(stream, lowOverhead, 1, events[:], scratchSpans[:], scratchJobs[:], scratchBatches[:])
		if err != nil {
			return
		}
		lowBatchPlan, err = decode.DecoderFrameWorkResidualLowOverheadStreamsPlan(stream, lowOverheads[:], 1, events[:], scratchSpans[:], scratchJobs[:], scratchBatches[:])
		if err != nil {
			return
		}
		rtpPlan, err = decode.DecoderFrameWorkResidualRTPPayloadStreamPlan(stream, 0, rtpPayload, 1, rtpBuffer[:], rtpSpans[:], events[:], scratchSpans[:], scratchJobs[:], scratchBatches[:])
		if err != nil {
			return
		}
		batchPlan, err = decode.DecoderFrameWorkResidualRTPPayloadsStreamPlan(stream, 0, rtpPayloads[:], 1, rtpBuffer[:], rtpSpans[:], events[:], scratchSpans[:], scratchJobs[:], scratchBatches[:])
	})
	if err != nil {
		t.Fatal(err)
	}
	if !lowPlan.HasEvent() ||
		!lowBatchPlan.HasEvent() ||
		!rtpPlan.HasEvent() ||
		!batchPlan.HasEvent() ||
		lowBatchPlan.Size.Event.Outputs != 2 ||
		batchPlan.Size.Event.Outputs != 2 ||
		batchPlan.Bind.Event.Kind != decode.DecoderEventTileGroup {
		t.Fatalf("stream plans low=%+v lowBatch=%+v rtp=%+v batch=%+v", lowPlan, lowBatchPlan, rtpPlan, batchPlan)
	}
	if allocs != 0 {
		t.Fatalf("DecoderFrameWorkResidualStreamPlan allocated: %f", allocs)
	}

	var reusablePlan decode.DecoderFrameWorkResidualStreamPlan
	allocs = testing.AllocsPerRun(1000, func() {
		reusablePlan = lowPlan.Max(lowBatchPlan).Max(rtpPlan).Max(batchPlan)
	})
	if !reusablePlan.HasEvent() ||
		reusablePlan.Size.Event.Outputs != 2 ||
		reusablePlan.Bind.Event.Kind != decode.DecoderEventTileGroup {
		t.Fatalf("reusable stream plan=%+v", reusablePlan)
	}
	if allocs != 0 {
		t.Fatalf("DecoderFrameWorkResidualStreamPlan.Max allocated: %f", allocs)
	}
}

func TestPublicDecoderFrameWorkResidualStreamCheckAndStateAllocs(t *testing.T) {
	size := decode.DecoderFrameWorkResidualStreamScratchSize{
		Events:    3,
		RTPBuffer: 32,
		RTPSpans:  3,
		Event: decode.DecoderFrameWorkResidualEventScratchSize{
			Runner: decode.DecoderFrameWorkBatchResidualRunnerScratchSize{
				Workers:         1,
				Int32Scratch:    8,
				ResidualScratch: 8,
			},
			SideData: decode.DecoderFrameWorkSideDataScratchSize{
				CDEFIndexMap:  4,
				LoopFilterMap: 4,
			},
			Plan:    decode.DecoderTileWorkPlan{SpanCount: 1, JobCount: 1, BatchCount: 1},
			Outputs: 2,
		},
	}
	scratch := publicDecoderResidualStreamScratch(size)
	var stream decode.DecoderStream
	runner := decode.DecoderFrameWorkResidualStreamRunner{
		Stream: &stream,
		EventRunner: decode.DecoderFrameWorkResidualEventRunner{
			Outputs: scratch.Outputs[:size.Event.Outputs],
		},
		Events:    scratch.Events[:size.Events],
		RTPBuffer: scratch.RTPBuffer[:size.RTPBuffer],
		RTPSpans:  scratch.RTPSpans[:size.RTPSpans],
		RTPUsed:   7,
	}
	var state decode.DecoderFrameWorkResidualStreamRunnerState
	var err error
	var ok bool
	var sequence av1.SequenceHeader
	allocs := testing.AllocsPerRun(1000, func() {
		err = scratch.Check(size)
		state = runner.State()
		sequence, ok = runner.SequenceHeader()
	})
	if err != nil {
		t.Fatal(err)
	}
	if !state.Bound || state.RTPUsed != 7 || state.OutputCapacity != 2 || ok || sequence != (av1.SequenceHeader{}) {
		t.Fatalf("state=%+v ok=%v sequence=%+v", state, ok, sequence)
	}
	if allocs != 0 {
		t.Fatalf("DecoderFrameWorkResidualStream Check/State allocated: %f", allocs)
	}
}

func TestPublicBindDecoderFrameWorkResidualStreamRunnerAllocs(t *testing.T) {
	size := decode.DecoderFrameWorkResidualStreamScratchSize{
		Events:    3,
		RTPBuffer: 32,
		RTPSpans:  3,
		Event: decode.DecoderFrameWorkResidualEventScratchSize{
			SideData: decode.DecoderFrameWorkSideDataScratchSize{
				CDEFIndexMap:             4,
				CDEFReadMap:              4,
				LoopFilterMap:            4,
				RestorationRecords:       1,
				RestorationBoundaryAbove: 2,
				RestorationBoundaryBelow: 2,
			},
		},
	}
	scratch := publicDecoderResidualStreamScratch(size)
	var stream decode.DecoderStream
	var eventRunner decode.DecoderFrameWorkResidualEventRunner
	var runner decode.DecoderFrameWorkResidualStreamRunner
	var err error
	allocs := testing.AllocsPerRun(1000, func() {
		runner, err = decode.BindDecoderFrameWorkResidualStreamRunner(size, &stream, eventRunner, scratch)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.Events) != size.Events ||
		len(runner.RTPBuffer) != size.RTPBuffer ||
		len(runner.RTPSpans) != size.RTPSpans ||
		len(runner.SideDataScratch.RestorationBoundaryBelow) != size.Event.SideData.RestorationBoundaryBelow {
		t.Fatalf("bound runner=%+v size=%+v", runner, size)
	}
	if allocs != 0 {
		t.Fatalf("BindDecoderFrameWorkResidualStreamRunner allocated: %f", allocs)
	}

	sequence := av1.SequenceHeader{ColorConfig: av1.ColorConfig{
		BitDepth:   8,
		MonoChrome: true,
	}}
	event := decode.DecoderEvent{
		Kind: decode.DecoderEventFrameHeader,
		FrameSize: av1.FrameSize{
			CodedWidth:    16,
			UpscaledWidth: 16,
			Height:        16,
		},
	}
	var spans [1]av1.TileSpan
	var jobs [1]av1.TileJob
	var batches [1]decode.TileBatch
	eventSize, err := decode.DecoderFrameWorkResidualEventScratchLen(sequence, &event, 1, spans[:], jobs[:], batches[:])
	if err != nil {
		t.Fatal(err)
	}
	combinedSize := decode.DecoderFrameWorkResidualStreamScratchSize{
		Events:    1,
		RTPBuffer: 8,
		RTPSpans:  1,
		Event:     eventSize,
	}
	combinedPlan := decode.DecoderFrameWorkResidualStreamPlan{
		Size: combinedSize,
		Bind: decode.DecoderFrameWorkResidualEventBindPlan{
			Sequence:   sequence,
			Event:      event,
			EventIndex: 0,
		},
	}
	combinedScratch := publicDecoderResidualStreamScratch(combinedSize)
	var combinedRunner decode.DecoderFrameWorkResidualStreamRunner
	var side decode.DecoderFrameWorkSideData
	allocs = testing.AllocsPerRun(1000, func() {
		combinedRunner, side, err = decode.BindDecoderFrameWorkResidualStreamEventRunner(combinedSize, &stream, sequence, &event, decode.DecoderFrameWorkResidualEventRuntime{}, combinedScratch, nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(combinedRunner.Events) != combinedSize.Events ||
		len(combinedRunner.RTPBuffer) != combinedSize.RTPBuffer ||
		len(combinedRunner.EventRunner.Spans) != 0 ||
		len(side.CDEFIndexMap.Index) == 0 {
		t.Fatalf("combined bound runner=%+v side=%+v size=%+v", combinedRunner, side, combinedSize)
	}
	if allocs != 0 {
		t.Fatalf("BindDecoderFrameWorkResidualStreamEventRunner allocated: %f", allocs)
	}

	allocs = testing.AllocsPerRun(1000, func() {
		combinedRunner, side, err = decode.BindDecoderFrameWorkResidualStreamPlanRunner(combinedPlan, &stream, decode.DecoderFrameWorkResidualEventRuntime{}, combinedScratch, nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(combinedRunner.Events) != combinedSize.Events ||
		len(combinedRunner.RTPBuffer) != combinedSize.RTPBuffer ||
		len(combinedRunner.EventRunner.Spans) != 0 ||
		len(side.CDEFIndexMap.Index) == 0 {
		t.Fatalf("plan-bound runner=%+v side=%+v plan=%+v", combinedRunner, side, combinedPlan)
	}
	if allocs != 0 {
		t.Fatalf("BindDecoderFrameWorkResidualStreamPlanRunner allocated: %f", allocs)
	}
}

func TestPublicDecoderFrameWorkResidualEventRunnerAllocs(t *testing.T) {
	workerPool, err := decode.NewTileWorkerPool(1)
	if err != nil {
		t.Fatal(err)
	}
	defer workerPool.Close()

	sequence := av1.SequenceHeader{
		EnableCDEF: true,
		ColorConfig: av1.ColorConfig{
			BitDepth:   8,
			MonoChrome: true,
		},
	}
	event := publicDecoderResidualRunnerFrameEvent()
	event.CDEF = av1.CDEFParams{
		Damping:       5,
		StrengthCount: 1,
		YStrength:     [av1.MaxCDEFStrengths]uint8{63},
	}
	var scratchSpans [2]av1.TileSpan
	var scratchJobs [2]av1.TileJob
	var scratchBatches [1]decode.TileBatch
	eventScratch, err := decode.DecoderFrameWorkResidualEventScratchLen(sequence, &event, 1, scratchSpans[:], scratchJobs[:], scratchBatches[:])
	if err != nil {
		t.Fatal(err)
	}
	runnerSize := eventScratch.Runner
	runner, err := decode.BindDecoderFrameWorkBatchResidualRunner(runnerSize, publicDecoderBatchResidualRunnerScratch(runnerSize))
	if err != nil {
		t.Fatal(err)
	}
	side, err := decode.BindDecoderFrameWorkResidualEventSideData(sequence, &event, publicDecoderFrameWorkSideDataScratch(eventScratch.SideData))
	if err != nil {
		t.Fatal(err)
	}

	pool := publicDecoderPostFilterFramePool(t, av1.FrameFormat{
		Width:      int(event.FrameSize.CodedWidth),
		Height:     int(event.FrameSize.Height),
		BitDepth:   8,
		MonoChrome: true,
		Align:      64,
	}, 1)
	var refs decode.DecoderSurfaceReferences
	var referenceSurfaces [av1.InterRefsPerFrame]int
	var referenceFrames [av1.InterRefsPerFrame]*av1.Frame
	var spans [2]av1.TileSpan
	var jobs [2]av1.TileJob
	var batches [1]decode.TileBatch
	var releases [av1.RefFrames]int
	var state decode.DecoderFrameWorkState
	var stats decode.DecoderFrameWorkTileResidualStats
	var probeOutput av1.Frame
	probeCtx, err := decode.DecoderFrameWorkPostFilterScratchContext(sequence, &event, 64, &side, &probeOutput)
	if err != nil {
		t.Fatal(err)
	}
	var probe decode.DecoderFrameWorkSupportedPostFilterScratchRunner
	postScratchSize, err := probe.ScratchLen(probeCtx)
	if err != nil {
		t.Fatal(err)
	}
	postRunner := decode.DecoderFrameWorkSupportedPostFilterScratchRunner{
		Scratch: publicDecoderPostFilterRequestScratch(decode.DecoderFrameWorkPostFilterRequestScratchLen(postScratchSize)),
	}
	eventRunner := decode.DecoderFrameWorkResidualEventRunner{
		State:             &state,
		Refs:              &refs,
		FramePool:         &pool,
		Align:             64,
		ReferenceSurfaces: referenceSurfaces[:],
		ReferenceFrames:   referenceFrames[:],
		Workers:           1,
		Spans:             spans[:],
		Jobs:              jobs[:],
		Batches:           batches[:],
		Releases:          releases[:],
		WorkerPool:        workerPool,
		BatchRunner:       &runner,
		Stats:             &stats,
	}

	allocs := testing.AllocsPerRun(1000, func() {
		pool.Reset()
		refs.Reset()
		state.Reset()
		result, runErr := eventRunner.RunWithPostFilterRunner(sequence, &event, &side, &postRunner)
		if runErr != nil {
			err = runErr
			return
		}
		if result.Output == nil || result.Run != (decode.DecoderFrameWorkStepResult{ExecutedTileWork: true, CompletedFrame: true}) {
			err = decode.ErrDecoderInvalidFrameWorkStep
			return
		}
		if !postRunner.Result.Completed.Has(decode.DecoderFrameWorkPostFilterCDEF) || postRunner.Context.RemainingPostFilters() != 0 {
			err = decode.ErrDecoderUnsupportedPostFilter
			return
		}
		if stats.TXBs == 0 || stats.Residuals == 0 {
			err = decode.ErrThreadingInvalidBatch
			return
		}
		if _, ok := refs.ReferenceSlot(0); !ok {
			err = decode.ErrDecoderInvalidSurfaceReference
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if allocs != 0 {
		t.Fatalf("DecoderFrameWorkResidualEventRunner.RunWithPostFilterRunner allocated: %f", allocs)
	}
}

func TestPublicDecodeAndReconstructDecoderFrameWorkJobResiduals(t *testing.T) {
	batch, state, cdfs, scratch, req := publicDecoderResidualDriver(t)
	predictions := 0
	transforms := 0
	req.Predict = func(visit av1.TileBlockLoopVisit) error {
		predictions++
		if visit.Block.MICol != 0 || visit.Block.MIRow != 0 {
			t.Fatalf("visit=%+v", visit.Block)
		}
		return nil
	}
	req.Transforms = func(visit av1.TileBlockLoopVisit) (decode.DecoderFrameWorkBlockTransforms, error) {
		transforms++
		return decode.ReadDecoderFrameWorkBlockTransforms(batch, state, visit)
	}

	stats, err := decode.DecodeAndReconstructDecoderFrameWorkJobResiduals(batch, 0, state, cdfs, &scratch, req)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Loop.Blocks != 1 || stats.Loop.PartitionReads != 1 ||
		predictions != 1 || transforms != 1 || stats.Predictions != 1 {
		t.Fatalf("stats=%+v predictions=%d transforms=%d", stats, predictions, transforms)
	}
	if stats.CoefficientBlocks != 1 || stats.SkippedBlocks != 0 ||
		stats.TXBs != 1 || stats.Residuals != 1 || stats.TXBs != stats.NonZero+stats.AllZero {
		t.Fatalf("residual stats=%+v", stats)
	}
	if stats.Loop.CoefficientBlocks != 1 {
		t.Fatalf("loop coefficient stats=%+v residual stats=%+v", stats.Loop, stats)
	}
}

func TestPublicDecodeAndRetainDecoderFrameWorkJobResiduals(t *testing.T) {
	batch, state, _, scratch, req := publicDecoderResidualDriver(t)
	var storage decode.DecoderFrameWorkTileResidualCDFStorage
	if err := decode.InitDecoderFrameWorkTileResidualCDFStorageDefault(&storage, batch.Quantization.BaseQIdx); err != nil {
		t.Fatal(err)
	}
	var retained decode.DecoderFrameWorkTileResidualCDFStorage
	retainedValid := false
	batch.RetainedTileResidualCDFs = &retained
	batch.RetainedTileResidualCDFsValid = &retainedValid
	batch.Jobs[0].UpdatesFrameContext = true

	stats, err := decode.DecodeAndRetainDecoderFrameWorkJobResiduals(batch, 0, state, &storage, &scratch, req)
	if err != nil {
		t.Fatal(err)
	}
	if stats.TXBs != 1 || stats.TXBs != stats.NonZero+stats.AllZero {
		t.Fatalf("stats=%+v", stats)
	}
	if !retainedValid {
		t.Fatal("update job did not retain residual CDF storage")
	}
	if !publicCDFValuesEqual(retained.DeltaQ.Values(), storage.DeltaQ.Values()) {
		t.Fatalf("retained delta q=%v want %v", retained.DeltaQ.Values(), storage.DeltaQ.Values())
	}

	if err := decode.InitDecoderFrameWorkTileResidualCDFStorageDefault(&storage, batch.Quantization.BaseQIdx); err != nil {
		t.Fatal(err)
	}
	retainedValid = false
	batch.Jobs[0].UpdatesFrameContext = false
	if _, err := decode.DecodeAndRetainDecoderFrameWorkJobResiduals(batch, 0, state, &storage, &scratch, req); err != nil {
		t.Fatal(err)
	}
	if retainedValid {
		t.Fatal("non-update job retained residual CDF storage")
	}
}

func TestPublicDecodeAndRetainDecoderFrameWorkJobResidualsDefaultTransforms(t *testing.T) {
	batch, state, _, scratch, req := publicDecoderResidualDriver(t)
	req.Transforms = nil
	req.UseDefaultTransforms = true
	var storage decode.DecoderFrameWorkTileResidualCDFStorage
	if err := decode.InitDecoderFrameWorkTileResidualCDFStorageDefault(&storage, batch.Quantization.BaseQIdx); err != nil {
		t.Fatal(err)
	}

	stats, err := decode.DecodeAndRetainDecoderFrameWorkJobResiduals(batch, 0, state, &storage, &scratch, req)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Loop.Blocks != 1 || stats.TXBs != 1 || stats.Residuals != 1 {
		t.Fatalf("stats=%+v", stats)
	}
}

func TestPublicDecodeAndRetainDecoderFrameWorkBatchResiduals(t *testing.T) {
	batch, state, storage, scratch, req := publicDecoderResidualBatchDriver(t)
	var retained decode.DecoderFrameWorkTileResidualCDFStorage
	retainedValid := false
	batch.RetainedTileResidualCDFs = &retained
	batch.RetainedTileResidualCDFsValid = &retainedValid
	visits := 0
	req.Tile.Loop.BeforeCoefficients = func(visit av1.TileBlockLoopVisit) error {
		visits++
		if visits == 2 && visit.Block.MICol != 16 {
			t.Fatalf("second job block=%+v", visit.Block)
		}
		return nil
	}

	stats, err := decode.DecodeAndRetainDecoderFrameWorkBatchResiduals(batch, state, &storage, &scratch, req)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Loop.Blocks != 2 || stats.Loop.PartitionReads != 2 || visits != 2 {
		t.Fatalf("stats=%+v visits=%d", stats, visits)
	}
	if stats.CoefficientBlocks != 2 || stats.Residuals != 2 ||
		stats.TXBs != 2 || stats.TXBs != stats.NonZero+stats.AllZero {
		t.Fatalf("residual stats=%+v", stats)
	}
	if !retainedValid {
		t.Fatal("update job did not retain residual CDF storage")
	}
	if !publicCDFValuesEqual(retained.DeltaQ.Values(), storage.DeltaQ.Values()) {
		t.Fatalf("retained delta q=%v want %v", retained.DeltaQ.Values(), storage.DeltaQ.Values())
	}
}

func TestPublicDecodeAndReconstructDecoderFrameWorkJobResidualsMarksSideMaps(t *testing.T) {
	batch, state, cdfs, scratch, req := publicDecoderResidualDriver(t)
	batch.CDEF = av1.CDEFParams{Bits: 0, StrengthCount: 1}
	sequence := av1.SequenceHeader{
		Use128x128Superblock: batch.Sequence.SBSizeMIB == 32,
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
	state.DeltaLFFromBase = -3
	state.DeltaLF = [av1.TileFrameLoopFilterCount]int8{4, 3, 2, 1}

	stats, err := decode.DecodeAndReconstructDecoderFrameWorkJobResiduals(batch, 0, state, cdfs, &scratch, req)
	if err != nil {
		t.Fatal(err)
	}
	if stats.CoefficientBlocks != 1 || stats.Loop.CoefficientBlocks != 1 {
		t.Fatalf("stats=%+v", stats)
	}
	if !cdefMap.Read[0] || cdefMap.Index[0] != 0 {
		t.Fatalf("cdef map read=%v index=%d want true,0", cdefMap.Read[0], cdefMap.Index[0])
	}
	record := loopMap.Records[0]
	if !record.Valid || record.Block.MICol != 0 || record.Block.MIRow != 0 ||
		record.DeltaLFFromBase != -3 || record.DeltaLF != state.DeltaLF {
		t.Fatalf("loop-filter record=%+v state delta=%v", record, state.DeltaLF)
	}
}

func TestPublicDecodeAndReconstructDecoderFrameWorkJobResidualsRejectsInvalidInputs(t *testing.T) {
	batch, state, cdfs, scratch, req := publicDecoderResidualDriver(t)
	if _, err := decode.DecodeAndReconstructDecoderFrameWorkJobResiduals(batch, 0, nil, cdfs, &scratch, req); !errors.Is(err, decode.ErrThreadingInvalidBatch) {
		t.Fatalf("nil state err=%v want %v", err, decode.ErrThreadingInvalidBatch)
	}
	if _, err := decode.DecodeAndReconstructDecoderFrameWorkJobResiduals(batch, 0, state, cdfs, nil, req); !errors.Is(err, decode.ErrThreadingInvalidBatch) {
		t.Fatalf("nil scratch err=%v want %v", err, decode.ErrThreadingInvalidBatch)
	}
	noTransforms := req
	noTransforms.Transforms = nil
	if _, err := decode.DecodeAndReconstructDecoderFrameWorkJobResiduals(batch, 0, state, cdfs, &scratch, noTransforms); !errors.Is(err, decode.ErrThreadingInvalidBatch) {
		t.Fatalf("nil transforms err=%v want %v", err, decode.ErrThreadingInvalidBatch)
	}
	shortCarrier := req
	scratch.LoopContext.Above = []av1.TileBlockLoopRootAboveContext{}
	if _, err := decode.DecodeAndReconstructDecoderFrameWorkJobResiduals(batch, 0, state, cdfs, &scratch, shortCarrier); !errors.Is(err, av1.ErrTileInvalidDecodeState) {
		t.Fatalf("short scratch carrier err=%v want %v", err, av1.ErrTileInvalidDecodeState)
	}
	if _, err := decode.DecodeAndRetainDecoderFrameWorkJobResiduals(batch, 0, state, nil, &scratch, req); !errors.Is(err, decode.ErrThreadingInvalidBatch) {
		t.Fatalf("nil retain storage err=%v want %v", err, decode.ErrThreadingInvalidBatch)
	}
	batchReq := decode.DecoderFrameWorkBatchResidualRequest{Tile: req, LoopContextAbove: []av1.TileBlockLoopRootAboveContext{}}
	if _, err := decode.DecodeAndRetainDecoderFrameWorkBatchResiduals(batch, state, &decode.DecoderFrameWorkTileResidualCDFStorage{}, &scratch, batchReq); !errors.Is(err, av1.ErrFrameShortBuffer) {
		t.Fatalf("short batch carrier err=%v want %v", err, av1.ErrFrameShortBuffer)
	}
	if _, err := decode.DecodeAndRetainDecoderFrameWorkBatchResiduals(batch, nil, &decode.DecoderFrameWorkTileResidualCDFStorage{}, &scratch, batchReq); !errors.Is(err, decode.ErrThreadingInvalidBatch) {
		t.Fatalf("nil batch state err=%v want %v", err, decode.ErrThreadingInvalidBatch)
	}
	if _, _, err := decode.DecoderFrameWorkResidualScratchLen(batch, batch.Quantization.BaseQIdx, 0, decode.DecoderFrameWorkPlane(99), av1.TransformSize{Width: 64, Height: 64}, av1.TransformTypeDCTDCT); !errors.Is(err, decode.ErrThreadingInvalidBatch) {
		t.Fatalf("bad scratch plane err=%v want %v", err, decode.ErrThreadingInvalidBatch)
	}
	if n, err := decode.DecoderFrameWorkBatchResidualLoopContextAboveLen(decode.DecoderFrameWorkBatch{}); err != nil || n != 0 {
		t.Fatalf("empty batch loop context len=%d err=%v want 0,nil", n, err)
	}
	if err := decode.InitDecoderFrameWorkTileRestorationRequestReferences(nil); !errors.Is(err, decode.ErrThreadingInvalidBatch) {
		t.Fatalf("nil restoration refs err=%v want %v", err, decode.ErrThreadingInvalidBatch)
	}
}

func TestPublicDecoderTileResidualStateAllocs(t *testing.T) {
	var initial decode.DecoderFrameWorkTileResidualCDFStorage
	if err := decode.InitDecoderFrameWorkTileResidualCDFStorageDefault(&initial, 64); err != nil {
		t.Fatal(err)
	}
	var retained decode.DecoderFrameWorkTileResidualCDFStorage
	retainedValid := false
	batch := decode.DecoderFrameWorkBatch{
		Payload: []byte{0x00, 0xff},
		FrameWorkFrameContext: decode.DecoderFrameWorkFrameContext{
			Quantization: av1.QuantizationParams{BaseQIdx: 73},
		},
		InitialTileResidualCDFs:       &initial,
		RetainedTileResidualCDFs:      &retained,
		RetainedTileResidualCDFsValid: &retainedValid,
		Jobs: []av1.TileJob{
			{Tile: 0, Offset: 0, Size: 1},
			{Tile: 1, Offset: 1, Size: 1, UpdatesFrameContext: true},
		},
	}
	var storage decode.DecoderFrameWorkTileResidualCDFStorage
	var state av1.TileDecodeState
	blockLoopBatch := publicDecoderBlockLoopBatch()
	rootColumns, err := decode.DecoderFrameWorkJobBlockLoopContextRootColumns(blockLoopBatch, 0)
	if err != nil {
		t.Fatal(err)
	}
	above := make([]av1.TileBlockLoopRootAboveContext, rootColumns)
	segMap := make([]uint8, 96*96)

	allocs := testing.AllocsPerRun(1000, func() {
		err = decode.InitDecoderFrameWorkTileResidualCDFStorage(batch, &storage)
		if err != nil {
			return
		}
		_ = decode.DecoderFrameWorkTileResidualCDFsFromStorage(&storage)
		err = decode.InitDecoderFrameWorkJobDecodeState(batch, 1, &state)
		if err != nil {
			return
		}
		err = decode.RetainDecoderFrameWorkTileResidualCDFStorage(batch, 1, &state, &storage)
		if err != nil {
			return
		}
		var carrier av1.TileBlockLoopContextCarrier
		carrier, err = decode.BindTileBlockLoopContextCarrier(rootColumns, above)
		if err != nil {
			return
		}
		_, err = decode.DecoderFrameWorkJobBlockLoopRequest(blockLoopBatch, 0, segMap, nil, 96, &carrier)
	})
	if err != nil {
		t.Fatal(err)
	}
	if allocs != 0 {
		t.Fatalf("allocs=%v want 0", allocs)
	}
}

func TestPublicDecodeAndReconstructDecoderFrameWorkJobResidualsAllocs(t *testing.T) {
	batch, state, cdfs, scratch, req := publicDecoderResidualDriver(t)
	allocs := testing.AllocsPerRun(1000, func() {
		if err := decode.InitDecoderFrameWorkJobDecodeState(batch, 0, state); err != nil {
			t.Fatal(err)
		}
		if _, err := decode.DecodeAndReconstructDecoderFrameWorkJobResiduals(batch, 0, state, cdfs, &scratch, req); err != nil {
			t.Fatal(err)
		}
	})
	if allocs != 0 {
		t.Fatalf("allocs=%v want 0", allocs)
	}
}

func TestPublicDecodeAndRetainDecoderFrameWorkJobResidualsAllocs(t *testing.T) {
	batch, state, _, scratch, req := publicDecoderResidualDriver(t)
	var storage decode.DecoderFrameWorkTileResidualCDFStorage
	allocs := testing.AllocsPerRun(1000, func() {
		if err := decode.InitDecoderFrameWorkTileResidualCDFStorageDefault(&storage, batch.Quantization.BaseQIdx); err != nil {
			t.Fatal(err)
		}
		if _, err := decode.DecodeAndRetainDecoderFrameWorkJobResiduals(batch, 0, state, &storage, &scratch, req); err != nil {
			t.Fatal(err)
		}
	})
	if allocs != 0 {
		t.Fatalf("allocs=%v want 0", allocs)
	}
}

func TestPublicDecodeAndRetainDecoderFrameWorkBatchResidualsAllocs(t *testing.T) {
	batch, state, storage, scratch, _ := publicDecoderResidualBatchDriver(t)
	scratchSize, err := decode.DecoderFrameWorkBatchResidualScratchLen(batch, batch.Quantization.BaseQIdx)
	if err != nil {
		t.Fatal(err)
	}
	arena := decode.DecoderFrameWorkBatchResidualScratch{
		Int32Scratch:     make([]int32, scratchSize.Int32Scratch),
		ResidualScratch:  make([]int16, scratchSize.ResidualScratch),
		LoopContextAbove: make([]av1.TileBlockLoopRootAboveContext, scratchSize.LoopContextAbove),
	}
	var req decode.DecoderFrameWorkBatchResidualRequest
	allocs := testing.AllocsPerRun(1000, func() {
		req, err = decode.BindDecoderFrameWorkBatchResidualRequestScratch(scratchSize, arena)
		if err != nil {
			t.Fatal(err)
		}
		req.Tile.TransformMode = batch.TransformRef.TransformMode
		req.Tile.UseDefaultTransforms = true
		if err := decode.InitDecoderFrameWorkTileResidualCDFStorageDefault(&storage, batch.Quantization.BaseQIdx); err != nil {
			t.Fatal(err)
		}
		if _, err := decode.DecodeAndRetainDecoderFrameWorkBatchResiduals(batch, state, &storage, &scratch, req); err != nil {
			t.Fatal(err)
		}
	})
	if allocs != 0 {
		t.Fatalf("allocs=%v want 0", allocs)
	}
}

func TestPublicDecoderFrameWorkBatchResidualRunnerAllocs(t *testing.T) {
	batch, _, _, _, _ := publicDecoderResidualBatchDriver(t)
	size, err := decode.DecoderFrameWorkBatchResidualRunnerScratchLen(batch, 1)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := decode.BindDecoderFrameWorkBatchResidualRunner(size, publicDecoderBatchResidualRunnerScratch(size))
	if err != nil {
		t.Fatal(err)
	}
	allocs := testing.AllocsPerRun(1000, func() {
		if err := runner.Run(batch); err != nil {
			t.Fatal(err)
		}
	})
	if allocs != 0 {
		t.Fatalf("allocs=%v want 0", allocs)
	}
}

func publicDecoderBlockLoopBatch() decode.DecoderFrameWorkBatch {
	return decode.DecoderFrameWorkBatch{
		FrameWorkFrameContext: decode.DecoderFrameWorkFrameContext{
			Sequence: decode.DecoderFrameWorkSequenceContextFromHeader(av1.SequenceHeader{
				Use128x128Superblock: true,
				EnableDualFilter:     true,
				EnableFilterIntra:    true,
				EnableOrderHint:      true,
				OrderHintBits:        5,
				ColorConfig:          av1.ColorConfig{BitDepth: 8, MonoChrome: true},
			}),
			FrameHeader:         av1.FrameHeaderPrefix{OrderHint: 9},
			FrameSize:           av1.FrameSize{CodedWidth: 300, UpscaledWidth: 300, Height: 260, SuperResDenominator: 8},
			TileInfo:            av1.TileInfo{InterpolationFilter: av1.InterpolationSwitchable, UseRefFrameMVS: true},
			ReferenceOrderHints: [av1.InterRefsPerFrame]uint8{1, 9, 10, 4, 5, 6, 7},
			SkipMode:            av1.SkipModeParams{Allowed: true, Enabled: true},
			CDEF:                av1.CDEFParams{Bits: 2, StrengthCount: 4},
			Delta:               av1.DeltaParams{DeltaQPresent: true, DeltaQResLog2: 1},
		},
		Jobs: []av1.TileJob{{Tile: 3, Row: 1, Col: 1, SBX: 1, SBY: 1, SBCols: 2, SBRows: 2}},
	}
}

func publicDecoderResidualDriver(t *testing.T) (decode.DecoderFrameWorkBatch, *av1.TileDecodeState, decode.DecoderFrameWorkTileResidualCDFs, decode.DecoderFrameWorkTileResidualScratch, decode.DecoderFrameWorkTileResidualRequest) {
	t.Helper()
	output := publicDecoderPostFilterFrame(t, av1.FrameFormat{Width: 64, Height: 64, BitDepth: 8, MonoChrome: true, Align: 64})
	publicFillDecoderPostFilterPlane(output.Y)
	batch := decode.DecoderFrameWorkBatch{
		Output:  output,
		Payload: make([]byte, 256),
		FrameWorkFrameContext: decode.DecoderFrameWorkFrameContext{
			Sequence: decode.DecoderFrameWorkSequenceContextFromHeader(av1.SequenceHeader{
				ColorConfig: av1.ColorConfig{BitDepth: 8, MonoChrome: true},
			}),
			FrameSize:    av1.FrameSize{CodedWidth: 64, UpscaledWidth: 64, Height: 64, SuperResDenominator: 8},
			Quantization: av1.QuantizationParams{BaseQIdx: 64},
			TransformRef: av1.TransformReferenceParams{TransformMode: av1.TransformModeLargest},
		},
		Jobs: []av1.TileJob{{SBCols: 1, SBRows: 1, Offset: 0, Size: 256}},
	}
	state := &av1.TileDecodeState{}
	if err := decode.InitDecoderFrameWorkJobDecodeState(batch, 0, state); err != nil {
		t.Fatal(err)
	}
	var storage decode.DecoderFrameWorkTileResidualCDFStorage
	if err := decode.InitDecoderFrameWorkTileResidualCDFStorageDefault(&storage, batch.Quantization.BaseQIdx); err != nil {
		t.Fatal(err)
	}
	loopReq, err := decode.DecoderFrameWorkJobBlockLoopRequest(batch, 0, nil, nil, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	int32Len, int16Len, err := decode.DecoderFrameWorkResidualMaxScratchLen(batch, batch.Quantization.BaseQIdx, 0, decode.DecoderFrameWorkPlaneY)
	if err != nil {
		t.Fatal(err)
	}
	var scratch decode.DecoderFrameWorkTileResidualScratch
	req := decode.DecoderFrameWorkTileResidualRequest{
		Loop:          loopReq,
		TransformMode: batch.TransformRef.TransformMode,
		Transforms: func(visit av1.TileBlockLoopVisit) (decode.DecoderFrameWorkBlockTransforms, error) {
			return decode.ReadDecoderFrameWorkBlockTransforms(batch, state, visit)
		},
		Int32Scratch:    make([]int32, int32Len),
		ResidualScratch: make([]int16, int16Len),
	}
	return batch, state, decode.DecoderFrameWorkTileResidualCDFsFromStorage(&storage), scratch, req
}

func publicDecoderResidualBatchDriver(t *testing.T) (decode.DecoderFrameWorkBatch, *av1.TileDecodeState, decode.DecoderFrameWorkTileResidualCDFStorage, decode.DecoderFrameWorkTileResidualScratch, decode.DecoderFrameWorkBatchResidualRequest) {
	t.Helper()
	output := publicDecoderPostFilterFrame(t, av1.FrameFormat{Width: 128, Height: 64, BitDepth: 8, MonoChrome: true, Align: 64})
	publicFillDecoderPostFilterPlane(output.Y)
	batch := decode.DecoderFrameWorkBatch{
		Output:  output,
		Payload: make([]byte, 512),
		FrameWorkFrameContext: decode.DecoderFrameWorkFrameContext{
			Sequence: decode.DecoderFrameWorkSequenceContextFromHeader(av1.SequenceHeader{
				ColorConfig: av1.ColorConfig{BitDepth: 8, MonoChrome: true},
			}),
			FrameSize:    av1.FrameSize{CodedWidth: 128, UpscaledWidth: 128, Height: 64, SuperResDenominator: 8},
			Quantization: av1.QuantizationParams{BaseQIdx: 64},
			TransformRef: av1.TransformReferenceParams{TransformMode: av1.TransformModeLargest},
		},
		Jobs: []av1.TileJob{
			{SBX: 0, SBY: 0, SBCols: 1, SBRows: 1, Offset: 0, Size: 256},
			{SBX: 1, SBY: 0, SBCols: 1, SBRows: 1, Offset: 256, Size: 256, UpdatesFrameContext: true},
		},
	}
	var retained decode.DecoderFrameWorkTileResidualCDFStorage
	retainedValid := false
	batch.RetainedTileResidualCDFs = &retained
	batch.RetainedTileResidualCDFsValid = &retainedValid
	var storage decode.DecoderFrameWorkTileResidualCDFStorage
	if err := decode.InitDecoderFrameWorkTileResidualCDFStorageDefault(&storage, batch.Quantization.BaseQIdx); err != nil {
		t.Fatal(err)
	}
	int32Len, int16Len, err := decode.DecoderFrameWorkResidualMaxScratchLen(batch, batch.Quantization.BaseQIdx, 0, decode.DecoderFrameWorkPlaneY)
	if err != nil {
		t.Fatal(err)
	}
	loopContextLen, err := decode.DecoderFrameWorkBatchResidualLoopContextAboveLen(batch)
	if err != nil {
		t.Fatal(err)
	}
	if loopContextLen != 1 {
		t.Fatalf("batch loop context len=%d want 1", loopContextLen)
	}
	state := &av1.TileDecodeState{}
	var scratch decode.DecoderFrameWorkTileResidualScratch
	req := decode.DecoderFrameWorkBatchResidualRequest{
		Tile: decode.DecoderFrameWorkTileResidualRequest{
			TransformMode: batch.TransformRef.TransformMode,
			Transforms: func(visit av1.TileBlockLoopVisit) (decode.DecoderFrameWorkBlockTransforms, error) {
				return decode.ReadDecoderFrameWorkBlockTransforms(batch, state, visit)
			},
			Int32Scratch:    make([]int32, int32Len),
			ResidualScratch: make([]int16, int16Len),
		},
		LoopContextAbove: make([]av1.TileBlockLoopRootAboveContext, loopContextLen),
	}
	return batch, state, storage, scratch, req
}

func publicDecoderBatchResidualRunnerScratch(size decode.DecoderFrameWorkBatchResidualRunnerScratchSize) decode.DecoderFrameWorkBatchResidualRunnerScratch {
	return decode.DecoderFrameWorkBatchResidualRunnerScratch{
		States:                  make([]av1.TileDecodeState, size.Workers+1),
		Storages:                make([]decode.DecoderFrameWorkTileResidualCDFStorage, size.Workers+1),
		TileScratch:             make([]decode.DecoderFrameWorkTileResidualScratch, size.Workers+1),
		RestorationRequests:     make([]decode.DecoderFrameWorkTileRestorationRequest, size.RestorationRequests+1),
		PredictionScratch:       make([]decode.DecoderFrameWorkPredictionScratch, size.Workers+1),
		InterPredictionScratch:  make([]decode.DecoderFrameWorkInterPredictionScratch, size.Workers+1),
		Stats:                   make([]decode.DecoderFrameWorkTileResidualStats, size.Workers+1),
		Int32Scratch:            make([]int32, size.Int32Scratch+1),
		ResidualScratch:         make([]int16, size.ResidualScratch+1),
		LoopContextAboveScratch: make([]av1.TileBlockLoopRootAboveContext, size.LoopContextAbove+1),
	}
}

func publicDecoderFrameWorkSideDataScratch(size decode.DecoderFrameWorkSideDataScratchSize) decode.DecoderFrameWorkSideDataScratch {
	return decode.DecoderFrameWorkSideDataScratch{
		CDEFIndexMap:             make([]uint8, size.CDEFIndexMap+1),
		CDEFReadMap:              make([]bool, size.CDEFReadMap+1),
		LoopFilterMap:            make([]decode.DecoderFrameWorkLoopFilterBlockRecord, size.LoopFilterMap+1),
		RestorationRecords:       make([]av1.TileRestorationUnitRecord, size.RestorationRecords+1),
		RestorationBoundaryAbove: make([]uint16, size.RestorationBoundaryAbove+1),
		RestorationBoundaryBelow: make([]uint16, size.RestorationBoundaryBelow+1),
	}
}

func publicDecoderResidualEventScratch(size decode.DecoderFrameWorkResidualEventScratchSize) decode.DecoderFrameWorkResidualEventScratch {
	return decode.DecoderFrameWorkResidualEventScratch{
		Runner:   publicDecoderBatchResidualRunnerScratch(size.Runner),
		SideData: publicDecoderFrameWorkSideDataScratch(size.SideData),
		Spans:    make([]av1.TileSpan, size.Plan.SpanCount+1),
		Jobs:     make([]av1.TileJob, size.Plan.JobCount+1),
		Batches:  make([]decode.TileBatch, size.Plan.BatchCount+1),
	}
}

func publicDecoderResidualStreamScratch(size decode.DecoderFrameWorkResidualStreamScratchSize) decode.DecoderFrameWorkResidualStreamScratch {
	return decode.DecoderFrameWorkResidualStreamScratch{
		Events:    make([]decode.DecoderEvent, size.Events+1),
		Event:     publicDecoderResidualEventScratch(size.Event),
		SideData:  publicDecoderFrameWorkSideDataScratch(size.Event.SideData),
		Outputs:   make([]*av1.Frame, size.Event.Outputs+1),
		RTPBuffer: make([]byte, size.RTPBuffer+1),
		RTPSpans:  make([]rtp.RTPObuSpan, size.RTPSpans+1),
	}
}

type publicDecoderResidualBitWriter struct {
	buf [128]byte
	bit int
}

func (w *publicDecoderResidualBitWriter) writeBits(value uint64, n uint8) {
	for i := int(n) - 1; i >= 0; i-- {
		if (value>>uint(i))&1 != 0 {
			w.buf[w.bit>>3] |= 1 << uint(7-(w.bit&7))
		}
		w.bit++
	}
}

func (w *publicDecoderResidualBitWriter) writeBool(value bool) {
	if value {
		w.writeBits(1, 1)
		return
	}
	w.writeBits(0, 1)
}

func (w *publicDecoderResidualBitWriter) bytes() []byte {
	return w.buf[:(w.bit+7)>>3]
}

func (w *publicDecoderResidualBitWriter) trailingBits() []byte {
	w.writeBits(1, 1)
	for w.bit&7 != 0 {
		w.writeBits(0, 1)
	}
	return w.buf[:w.bit>>3]
}

func publicDecoderResidualLowOverheadStream() []byte {
	var stream []byte
	stream = appendPublicLowOverheadOBU(stream, av1.OBUSequenceHeader, publicDecoderResidualSequenceHeaderPayload())
	stream = appendPublicLowOverheadOBU(stream, av1.OBUFrameHeader, publicDecoderResidualFrameHeaderPayload())
	stream = appendPublicLowOverheadOBU(stream, av1.OBUTileGroup, []byte{0x80})
	return stream
}

func publicDecoderResidualLowOverheadFrameStream() []byte {
	var stream []byte
	stream = appendPublicLowOverheadOBU(stream, av1.OBUFrameHeader, publicDecoderResidualFrameHeaderPayload())
	stream = appendPublicLowOverheadOBU(stream, av1.OBUTileGroup, []byte{0x80})
	return stream
}

func publicDecoderResidualRTPPayload() []byte {
	elements := [...]rtp.RTPElement{
		{Data: publicDecoderResidualRTPElement(av1.OBUSequenceHeader, publicDecoderResidualSequenceHeaderPayload())},
		{Data: publicDecoderResidualRTPElement(av1.OBUFrameHeader, publicDecoderResidualFrameHeaderPayload())},
		{Data: publicDecoderResidualRTPElement(av1.OBUTileGroup, []byte{0x80})},
	}
	payload := make([]byte, 128)
	n, err := rtp.PutRTPPayload(payload, rtp.RTPAggregationHeader{
		ElementCount:                uint8(len(elements)),
		StartsNewCodedVideoSequence: true,
	}, elements[:])
	if err != nil {
		panic(err)
	}
	return payload[:n]
}

func publicDecoderResidualRTPFramePayload() []byte {
	elements := [...]rtp.RTPElement{
		{Data: publicDecoderResidualRTPElement(av1.OBUFrameHeader, publicDecoderResidualFrameHeaderPayload())},
		{Data: publicDecoderResidualRTPElement(av1.OBUTileGroup, []byte{0x80})},
	}
	payload := make([]byte, 128)
	n, err := rtp.PutRTPPayload(payload, rtp.RTPAggregationHeader{
		ElementCount: uint8(len(elements)),
	}, elements[:])
	if err != nil {
		panic(err)
	}
	return payload[:n]
}

func publicDecoderResidualFragmentedRTPPayloads() [][]byte {
	payloads := make([][]byte, 0, 4)

	sequence := [...]rtp.RTPElement{
		{Data: publicDecoderResidualRTPElement(av1.OBUSequenceHeader, publicDecoderResidualSequenceHeaderPayload())},
	}
	var packet [128]byte
	n, err := rtp.PutRTPPayload(packet[:], rtp.RTPAggregationHeader{
		ElementCount:                1,
		StartsNewCodedVideoSequence: true,
	}, sequence[:])
	if err != nil {
		panic(err)
	}
	payloads = append(payloads, append([]byte(nil), packet[:n]...))

	frameHeader := publicDecoderResidualRTPElement(av1.OBUFrameHeader, publicDecoderResidualFrameHeaderPayload())
	n, next, more, err := rtp.PutRTPFragment(packet[:], frameHeader, 0, 2, false)
	if err != nil {
		panic(err)
	}
	if !more {
		panic("expected frame header RTP fragment")
	}
	payloads = append(payloads, append([]byte(nil), packet[:n]...))
	n, _, more, err = rtp.PutRTPFragment(packet[:], frameHeader, next, len(packet), false)
	if err != nil {
		panic(err)
	}
	if more {
		panic("unexpected third frame header RTP fragment")
	}
	payloads = append(payloads, append([]byte(nil), packet[:n]...))

	tile := [...]rtp.RTPElement{
		{Data: publicDecoderResidualRTPElement(av1.OBUTileGroup, []byte{0x80})},
	}
	n, err = rtp.PutRTPPayload(packet[:], rtp.RTPAggregationHeader{ElementCount: 1}, tile[:])
	if err != nil {
		panic(err)
	}
	payloads = append(payloads, append([]byte(nil), packet[:n]...))

	return payloads
}

func publicDecoderResidualRTPElement(typ av1.OBUType, payload []byte) []byte {
	var header [2]byte
	n, err := av1.PutOBUHeader(header[:], av1.OBUHeader{Type: typ})
	if err != nil {
		panic(err)
	}
	element := make([]byte, 0, n+len(payload))
	element = append(element, header[:n]...)
	element = append(element, payload...)
	return element
}

func publicDecoderResidualSequenceHeaderPayload() []byte {
	var w publicDecoderResidualBitWriter
	w.writeBits(0, 3)  // seq_profile
	w.writeBool(true)  // still_picture
	w.writeBool(true)  // reduced_still_picture_header
	w.writeBits(5, 5)  // seq_level_idx[0]
	w.writeBits(7, 4)  // frame_width_bits_minus_1
	w.writeBits(7, 4)  // frame_height_bits_minus_1
	w.writeBits(15, 8) // max_frame_width_minus_1
	w.writeBits(15, 8) // max_frame_height_minus_1
	w.writeBool(false) // use_128x128_superblock
	w.writeBool(true)  // enable_filter_intra
	w.writeBool(true)  // enable_intra_edge_filter
	w.writeBool(false) // enable_superres
	w.writeBool(true)  // enable_cdef
	w.writeBool(false) // enable_restoration
	w.writeBool(false) // high_bitdepth
	w.writeBool(true)  // mono_chrome
	w.writeBool(false) // color_description_present_flag
	w.writeBool(false) // color_range
	w.writeBool(false) // film_grain_params_present
	return w.trailingBits()
}

func publicDecoderResidualRealtimeSequenceHeaderPayload() []byte {
	var w publicDecoderResidualBitWriter
	w.writeBits(0, 3)  // seq_profile
	w.writeBool(false) // still_picture
	w.writeBool(false) // reduced_still_picture_header
	w.writeBool(false) // timing_info_present_flag
	w.writeBool(false) // initial_display_delay_present_flag
	w.writeBits(0, 5)  // operating_points_cnt_minus_1
	w.writeBits(0, 12) // operating_point_idc[0]
	w.writeBits(5, 5)  // seq_level_idx[0]
	w.writeBits(3, 4)  // frame_width_bits_minus_1
	w.writeBits(3, 4)  // frame_height_bits_minus_1
	w.writeBits(15, 4) // max_frame_width_minus_1
	w.writeBits(8, 4)  // max_frame_height_minus_1
	w.writeBool(false) // frame_id_numbers_present_flag
	w.writeBool(false) // use_128x128_superblock
	w.writeBool(true)  // enable_filter_intra
	w.writeBool(true)  // enable_intra_edge_filter
	w.writeBool(true)  // enable_interintra_compound
	w.writeBool(true)  // enable_masked_compound
	w.writeBool(false) // enable_warped_motion
	w.writeBool(true)  // enable_dual_filter
	w.writeBool(false) // enable_order_hint
	w.writeBool(false) // seq_choose_screen_content_tools
	w.writeBits(0, 1)  // seq_force_screen_content_tools
	w.writeBool(false) // enable_superres
	w.writeBool(true)  // enable_cdef
	w.writeBool(false) // enable_restoration
	w.writeBool(false) // high_bitdepth
	w.writeBool(false) // mono_chrome
	w.writeBool(false) // color_description_present_flag
	w.writeBool(false) // color_range
	w.writeBits(0, 2)  // chroma_sample_position
	w.writeBool(true)  // separate_uv_delta_q
	w.writeBool(false) // film_grain_params_present
	return w.trailingBits()
}

func publicDecoderResidualShownKeyFrameHeaderPayload() []byte {
	var w publicDecoderResidualBitWriter
	w.writeBool(false)                       // show_existing_frame
	w.writeBits(uint64(av1.FrameTypeKey), 2) // frame_type
	w.writeBool(true)                        // show_frame
	w.writeBool(false)                       // disable_cdf_update
	w.writeBool(false)                       // frame_size_override_flag
	w.writeBool(false)                       // render_and_frame_size_different
	w.writeBool(false)                       // disable_frame_end_update_cdf
	w.writeBool(false)                       // uniform_tile_spacing_flag
	publicDecoderResidualWriteColorQuantParams(&w)
	publicDecoderResidualWriteZeroSegmentationParams(&w)
	w.writeBool(false) // reduced_tx_set
	return w.bytes()
}

func publicDecoderResidualInterFrameHeaderPayload() []byte {
	var w publicDecoderResidualBitWriter
	w.writeBool(false)                         // show_existing_frame
	w.writeBits(uint64(av1.FrameTypeInter), 2) // frame_type
	w.writeBool(true)                          // show_frame
	w.writeBool(false)                         // error_resilient_mode
	w.writeBool(false)                         // disable_cdf_update
	w.writeBool(false)                         // frame_size_override_flag
	w.writeBits(0, 3)                          // primary_ref_frame
	w.writeBits(0x01, 8)                       // refresh_frame_flags
	for range av1.InterRefsPerFrame {
		w.writeBits(0, 3) // ref_frame_idx[i]
	}
	w.writeBool(false) // render_and_frame_size_different
	w.writeBool(false) // allow_high_precision_mv
	w.writeBool(false) // interpolation_filter is fixed
	w.writeBits(0, 2)  // interpolation_filter = EIGHTTAP
	w.writeBool(false) // is_motion_mode_switchable
	w.writeBool(false) // disable_frame_end_update_cdf
	w.writeBool(false) // uniform_tile_spacing_flag
	publicDecoderResidualWriteColorQuantParams(&w)
	publicDecoderResidualWriteZeroSegmentationParams(&w)
	w.writeBool(false) // reference_select
	w.writeBool(false) // reduced_tx_set
	for range av1.InterRefsPerFrame {
		w.writeBool(false) // global_motion_is_global
	}
	return w.bytes()
}

func publicDecoderResidualShowExistingFrameHeaderPayload(index uint8) []byte {
	var w publicDecoderResidualBitWriter
	w.writeBool(true) // show_existing_frame
	w.writeBits(uint64(index&7), 3)
	return w.bytes()
}

func publicDecoderResidualWriteColorQuantParams(w *publicDecoderResidualBitWriter) {
	w.writeBits(0, 8)  // base_q_idx
	w.writeBool(false) // y_dc_delta_q
	w.writeBool(false) // diff_uv_delta
	w.writeBool(false) // u_dc_delta_q
	w.writeBool(false) // u_ac_delta_q
	w.writeBool(false) // using_qmatrix
}

func publicDecoderResidualWriteZeroSegmentationParams(w *publicDecoderResidualBitWriter) {
	w.writeBool(false) // segmentation_enabled
}

func publicDecoderResidualFrameHeaderPayload() []byte {
	var w publicDecoderResidualBitWriter
	w.writeBool(true)  // disable_cdf_update
	w.writeBool(false) // render_and_frame_size_different
	w.writeBool(false) // uniform_tile_spacing_flag
	w.writeBits(0, 8)  // base_q_idx
	w.writeBool(false) // y_dc_delta_q
	w.writeBool(false) // using_qmatrix
	w.writeBool(false) // segmentation_enabled
	w.writeBool(false) // reduced_tx_set
	return w.bytes()
}

func publicDecoderResidualRunnerFrameEvent() decode.DecoderEvent {
	payload := make([]byte, 513)
	payload[0] = 0xff
	return decode.DecoderEvent{
		Kind: decode.DecoderEventFrame,
		Unit: av1.OBUUnit{Payload: payload},
		FrameHeader: av1.FrameHeaderPrefix{
			FrameType: av1.FrameTypeKey,
			ShowFrame: true,
		},
		FrameSize: av1.FrameSize{
			CodedWidth:          128,
			UpscaledWidth:       128,
			Height:              64,
			SuperResDenominator: 8,
			RefreshFrameFlags:   0xff,
		},
		TileInfo: av1.TileInfo{
			TileSizeBytes: 1,
			SBCols:        2,
			SBRows:        1,
			Cols:          2,
			Rows:          1,
			ColStartSB:    [av1.MaxTileCols + 1]uint16{0, 1, 2},
			RowStartSB:    [av1.MaxTileRows + 1]uint16{0, 1},
		},
		TileGroup: av1.TileGroup{
			StartTile: 0,
			EndTile:   1,
			TileCount: 2,
			Final:     true,
		},
		CDEF:         av1.CDEFParams{Bits: 0, StrengthCount: 1},
		Quantization: av1.QuantizationParams{BaseQIdx: 64},
		TransformRef: av1.TransformReferenceParams{TransformMode: av1.TransformModeLargest},
	}
}

func publicCDFValuesEqual(a []uint16, b []uint16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
