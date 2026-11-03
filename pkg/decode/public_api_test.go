package decode_test

import (
	av1 "github.com/gtsteffaniak/go-av1/pkg/av1"
	decode "github.com/gtsteffaniak/go-av1/pkg/decode"
	"testing"
)

func TestPublicDecoderPostFilterTypesAreNameable(t *testing.T) {
	var buffers decode.DecoderFrameWorkRestorationFrameBuffers
	ctx := decode.DecoderFrameWorkPostFilterContext{
		RestorationFrameBuffers: &buffers,
	}
	if ctx.RestorationFrameBuffers != &buffers {
		t.Fatal("restoration frame buffers did not round-trip through public context")
	}
	supportedStages, err := ctx.SupportedPostFilters()
	if err != nil {
		t.Fatal(err)
	}
	unsupportedStages, err := ctx.UnsupportedPostFilters()
	if err != nil {
		t.Fatal(err)
	}
	if !supportedStages.Empty() || !unsupportedStages.Empty() {
		t.Fatalf("supported=%b unsupported=%b", supportedStages, unsupportedStages)
	}
	if err := ctx.RequireSupportedPostFilters(); err != nil {
		t.Fatalf("RequireSupportedPostFilters err=%v", err)
	}

	edge := decode.DecoderFrameWorkLoopFilterPostFilterEdge{
		Plane:     av1.LoopFilterPlaneY,
		Edge:      av1.LoopFilterEdgeVertical,
		Transform: av1.TileTransformSize4x4,
	}
	if edge.Plane != av1.LoopFilterPlaneY || edge.Edge != av1.LoopFilterEdgeVertical {
		t.Fatalf("edge=%+v", edge)
	}

	records := [3][]av1.TileRestorationUnitRecord{
		{
			{
				Rect: av1.TileRestorationUnitRect{X1: 4, Y1: 4},
				Unit: av1.TileRestorationUnit{
					Type:   av1.RestorationWiener,
					Wiener: av1.RestorationWienerInfo{},
				},
			},
		},
	}
	boundaries := [3]av1.TileRestorationStripeBoundaries{}
	scratch := av1.TileRestorationUnitRecordBoundaryScratch{
		Unit: av1.TileRestorationUnitScratch{},
	}
	restorationReq := decode.DecoderFrameWorkRestorationPostFilterRequest{
		Records:    records,
		Boundaries: boundaries,
		Scratch:    scratch,
	}
	req := decode.DecoderFrameWorkPostFilterRequest{
		LoopFilter: decode.DecoderFrameWorkLoopFilterPostFilterRequest{
			Edges: []decode.DecoderFrameWorkLoopFilterPostFilterEdge{edge},
		},
		Restoration: restorationReq,
	}
	if req.Restoration.Records[0][0].Unit.Type != av1.RestorationWiener {
		t.Fatalf("restoration request=%+v", req.Restoration)
	}

	var loopResult decode.DecoderFrameWorkLoopFilterPostFilterApplyResult
	var restorationResult av1.TileRestorationFrameApplyResult
	result := decode.DecoderFrameWorkCallerPostFilterResult{
		FrameWorkPostFilterResult: decode.DecoderFrameWorkPostFilterResult{
			LoopFilter:  loopResult,
			Restoration: restorationResult,
		},
	}
	if !result.Completed.Empty() {
		t.Fatalf("result=%+v", result)
	}
	var callerRunner decode.DecoderFrameWorkCallerPostFilterRunner
	if callerRunner.Result.Completed != result.Completed {
		t.Fatalf("caller runner=%+v result=%+v", callerRunner, result)
	}
	if callerScratch, err := callerRunner.ScratchLen(ctx); err != nil || callerScratch.SuperRes.OutputFrame != 0 {
		t.Fatalf("caller runner scratch=%+v err=%v", callerScratch, err)
	}
	var supportedRunner decode.DecoderFrameWorkSupportedPostFilterRunner
	if supportedScratch, err := supportedRunner.ScratchLen(ctx); err != nil || supportedScratch.LoopFilter.Edges != 0 {
		t.Fatalf("supported runner scratch=%+v err=%v", supportedScratch, err)
	}

	var postScratch decode.DecoderFrameWorkPostFilterScratchSize
	postScratch.Restoration = decode.DecoderFrameWorkRestorationPostFilterScratchSize{
		Samples: av1.TileRestorationFrameSampleScratchSize{},
		Apply:   av1.TileRestorationUnitRecordBoundaryScratchSize{},
	}
	postScratch.LoopFilter.Edges = 2
	postScratch.SuperRes.OutputFrame = 16
	combinedScratch := postScratch.Max(decode.DecoderFrameWorkPostFilterScratchSize{
		LoopFilter: decode.DecoderFrameWorkLoopFilterPostFilterScratchSize{Edges: 4},
		SuperRes:   decode.DecoderFrameWorkSuperResPostFilterScratchSize{OutputFrame: 8},
	})
	if combinedScratch.LoopFilter.Edges != 4 || combinedScratch.SuperRes.OutputFrame != 16 {
		t.Fatalf("combined scratch=%+v", combinedScratch)
	}
	_ = postScratch
}
