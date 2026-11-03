package encoder_test

import (
	"testing"

	"github.com/gtsteffaniak/go-av1/internal/av1/encoder"
	av1 "github.com/gtsteffaniak/go-av1/pkg/av1"
	decode "github.com/gtsteffaniak/go-av1/pkg/decode"
)

func TestEncodeScaledReferencePFrameLayerPoolDecodeMatchesRecon(t *testing.T) {
	const (
		baseW, baseH = 64, 48
		topW, topH   = 128, 96
		keyQ         = 64
	)

	baseSrc := scaledReferenceDecodeBaseFrame(baseW, baseH)
	keyTU, baseRecon, err := encoder.EncodeKeyframeWithSequenceMax(baseSrc, keyQ, topW, topH)
	if err != nil {
		t.Fatalf("EncodeKeyframeWithSequenceMax: %v", err)
	}

	topSrc := scaleSource420(baseRecon, topW, topH)
	for _, tc := range []struct {
		name   string
		qIndex uint8
	}{
		{name: "lossless", qIndex: 0},
		{name: "lossy", qIndex: 72},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scaledTU, topRecon, err := encoder.EncodeScaledReferencePFrame(topSrc, baseRecon, tc.qIndex)
			if err != nil {
				t.Fatalf("EncodeScaledReferencePFrame: %v", err)
			}
			if tc.qIndex == 0 {
				compareBytePlane(t, "lossless source Y", topRecon.Y, topRecon.YStride, topSrc.Y, topSrc.YStride, topW, topH)
				compareBytePlane(t, "lossless source U", topRecon.U, topRecon.ChromaStride, topSrc.U, topSrc.ChromaStride, topW/2, topH/2)
				compareBytePlane(t, "lossless source V", topRecon.V, topRecon.ChromaStride, topSrc.V, topSrc.ChromaStride, topW/2, topH/2)
			}

			frames := decodeLayerPoolLowOverheads(t, keyTU, scaledTU)
			if len(frames) != 2 {
				t.Fatalf("decoded %d frames, want 2", len(frames))
			}
			comparePlane(t, "base Y", frames[0].Y, baseRecon.Y, baseW, baseH, baseW)
			comparePlane(t, "base U", frames[0].U, baseRecon.U, baseW/2, baseH/2, baseW/2)
			comparePlane(t, "base V", frames[0].V, baseRecon.V, baseW/2, baseH/2, baseW/2)
			comparePlane(t, "top Y", frames[1].Y, topRecon.Y, topW, topH, topW)
			comparePlane(t, "top U", frames[1].U, topRecon.U, topW/2, topH/2, topW/2)
			comparePlane(t, "top V", frames[1].V, topRecon.V, topW/2, topH/2, topW/2)
		})
	}
}

func compareBytePlane(t *testing.T, name string, got []byte, gotStride int, want []byte, wantStride int, w, h int) {
	t.Helper()
	for y := range h {
		gotRow := got[y*gotStride : y*gotStride+w]
		wantRow := want[y*wantStride : y*wantStride+w]
		for x := range w {
			if gotRow[x] != wantRow[x] {
				t.Fatalf("%s differs at (%d,%d): got %d want %d", name, x, y, gotRow[x], wantRow[x])
			}
		}
	}
}

func decodeLayerPoolLowOverheads(t *testing.T, payloads ...[]byte) []*av1.Frame {
	t.Helper()

	const workers = 1
	workerPool, err := decode.NewTileWorkerPool(workers)
	if err != nil {
		t.Fatalf("NewTileWorkerPool: %v", err)
	}
	defer workerPool.Close()

	layerPool := newScaledReferenceDecodeLayerPool(t, 4, av1.RefFrames+1)
	adapter := decode.NewDecoderFrameLayerPool(&layerPool)

	var (
		stream        decode.DecoderStream
		refs          decode.DecoderSurfaceReferences
		state         decode.DecoderFrameWorkState
		frameContexts decode.DecoderSharedFrameContextStore
		stats         decode.DecoderFrameWorkTileResidualStats
		postFilter    decode.DecoderFrameWorkReusableSupportedPostFilterRunner
		sequence      av1.SequenceHeader
		haveSequence  bool
	)
	referenceSurfaces := make([]int, av1.InterRefsPerFrame)
	referenceFrames := make([]*av1.Frame, av1.InterRefsPerFrame)
	releases := make([]int, av1.RefFrames)
	events := make([]decode.DecoderEvent, 16)
	planSpans := make([]av1.TileSpan, av1.MaxTiles)
	planJobs := make([]av1.TileJob, av1.MaxTiles)
	planBatches := make([]decode.TileBatch, av1.MaxTiles)
	outputs := make([]*av1.Frame, 0, len(payloads))

	for payloadIndex, payload := range payloads {
		count, err := stream.PushLowOverhead(payload, events)
		if err != nil {
			t.Fatalf("payload %d PushLowOverhead: %v", payloadIndex, err)
		}
		for eventIndex := 0; eventIndex < count; eventIndex++ {
			event := events[eventIndex]
			if event.Kind == decode.DecoderEventSequenceHeader {
				sequence = event.SequenceHeader
				haveSequence = true
			} else if !haveSequence {
				if seq, ok := stream.SequenceHeader(); ok {
					sequence = seq
					haveSequence = true
				}
			}

			framePool := scaledReferenceDecodeFramePool(t, &layerPool, sequence, event)
			size, err := decode.DecoderFrameWorkResidualEventScratchLen(sequence, &event, workers, planSpans, planJobs, planBatches)
			if err != nil {
				t.Fatalf("payload %d event %d scratch len: %v", payloadIndex, eventIndex, err)
			}
			scratch := scaledReferenceDecodeEventScratch(size)

			var batchRunner decode.DecoderFrameWorkBatchResidualRunner
			var batchRunnerPtr *decode.DecoderFrameWorkBatchResidualRunner
			if size.Runner.Workers != 0 {
				batchRunner, err = decode.BindDecoderFrameWorkBatchResidualRunner(size.Runner, scratch.Runner)
				if err != nil {
					t.Fatalf("payload %d event %d bind residual runner: %v", payloadIndex, eventIndex, err)
				}
				batchRunnerPtr = &batchRunner
			}

			var sideData decode.DecoderFrameWorkSideData
			var sideDataPtr *decode.DecoderFrameWorkSideData
			if scaledReferenceDecodeEventNeedsSideData(event) {
				sideData, err = decode.BindDecoderFrameWorkSideData(sequence, event.FrameSize, event.CDEF, event.Restoration, scratch.SideData)
				if err != nil {
					t.Fatalf("payload %d event %d bind side data: %v", payloadIndex, eventIndex, err)
				}
				sideDataPtr = &sideData
			}

			globalSurface := func(local int) int {
				if framePool == nil {
					return -1
				}
				return decode.DecoderLayerPoolGlobalSurfaceID(&layerPool, framePool, local)
			}
			result, err := decode.RunDecoderFrameWorkEventWithResidualRunner(decode.DecoderFrameWorkResidualEventRequest{
				State:             &state,
				Refs:              &refs,
				FramePool:         framePool,
				Sequence:          sequence,
				Event:             event,
				Align:             64,
				ReferenceSurfaces: referenceSurfaces,
				ReferenceFrames:   referenceFrames,
				Workers:           workers,
				Spans:             scratch.Spans,
				Jobs:              scratch.Jobs,
				Batches:           scratch.Batches,
				Releases:          releases,
				WorkerPool:        workerPool,
				Runner:            batchRunnerPtr,
				SideData:          sideDataPtr,
				PostRunner:        &postFilter,
				Stats:             &stats,
				External: decode.DecoderFrameWorkExternalReferenceRuntime{
					Provider:      adapter,
					GlobalSurface: globalSurface,
					Releaser:      adapter,
					FrameContexts: &frameContexts,
				},
			})
			if err != nil {
				t.Fatalf("payload %d event %d run: %v", payloadIndex, eventIndex, err)
			}
			if decode.DecoderEventOutputsFrame(&event) {
				if result.Output == nil {
					t.Fatalf("payload %d event %d output frame is nil", payloadIndex, eventIndex)
				}
				outputs = append(outputs, result.Output)
			}
		}
	}
	return outputs
}

func scaledReferenceDecodeFramePool(t *testing.T, pool *av1.FrameLayerPool, sequence av1.SequenceHeader, event decode.DecoderEvent) *av1.FramePool {
	t.Helper()
	switch event.Kind {
	case decode.DecoderEventFrameHeader, decode.DecoderEventFrame, decode.DecoderEventTileGroup:
		format, err := av1.FrameCodedFormatFromHeaders(sequence, event.FrameSize, 64)
		if err != nil {
			t.Fatalf("FrameCodedFormatFromHeaders: %v", err)
		}
		framePool, err := pool.SubPool(format)
		if err != nil {
			t.Fatalf("layer SubPool: %v", err)
		}
		return framePool
	default:
		return nil
	}
}

func scaledReferenceDecodeEventNeedsSideData(event decode.DecoderEvent) bool {
	switch event.Kind {
	case decode.DecoderEventFrameHeader, decode.DecoderEventFrame, decode.DecoderEventTileGroup:
		return true
	default:
		return false
	}
}

func scaledReferenceDecodeEventScratch(size decode.DecoderFrameWorkResidualEventScratchSize) decode.DecoderFrameWorkResidualEventScratch {
	return decode.DecoderFrameWorkResidualEventScratch{
		Runner: decode.DecoderFrameWorkBatchResidualRunnerScratch{
			States:                  make([]av1.TileDecodeState, size.Runner.Workers),
			Storages:                make([]decode.DecoderFrameWorkTileResidualCDFStorage, size.Runner.Workers),
			TileScratch:             make([]decode.DecoderFrameWorkTileResidualScratch, size.Runner.Workers),
			RestorationRequests:     make([]decode.DecoderFrameWorkTileRestorationRequest, size.Runner.RestorationRequests),
			PredictionScratch:       make([]decode.DecoderFrameWorkPredictionScratch, size.Runner.Workers),
			InterPredictionScratch:  make([]decode.DecoderFrameWorkInterPredictionScratch, size.Runner.Workers),
			Stats:                   make([]decode.DecoderFrameWorkTileResidualStats, size.Runner.Workers),
			Int32Scratch:            make([]int32, size.Runner.Int32Scratch),
			ResidualScratch:         make([]int16, size.Runner.ResidualScratch),
			LoopContextAboveScratch: make([]av1.TileBlockLoopRootAboveContext, size.Runner.LoopContextAbove),
		},
		SideData: decode.DecoderFrameWorkSideDataScratch{
			CDEFIndexMap:             make([]uint8, size.SideData.CDEFIndexMap),
			CDEFReadMap:              make([]bool, size.SideData.CDEFReadMap),
			LoopFilterMap:            make([]decode.DecoderFrameWorkLoopFilterBlockRecord, size.SideData.LoopFilterMap),
			RestorationRecords:       make([]av1.TileRestorationUnitRecord, size.SideData.RestorationRecords),
			RestorationBoundaryAbove: make([]uint16, size.SideData.RestorationBoundaryAbove),
			RestorationBoundaryBelow: make([]uint16, size.SideData.RestorationBoundaryBelow),
		},
		Spans:   make([]av1.TileSpan, size.Plan.SpanCount),
		Jobs:    make([]av1.TileJob, size.Plan.JobCount),
		Batches: make([]decode.TileBatch, size.Plan.BatchCount),
	}
}

func newScaledReferenceDecodeLayerPool(t *testing.T, layers int, surfacesPerLayer int) av1.FrameLayerPool {
	t.Helper()
	factory := av1.FrameLayerFactoryFunc(func(format av1.FrameFormat) (av1.FramePool, error) {
		_, backingSize, err := av1.FramePoolRequiredSize(format, surfacesPerLayer)
		if err != nil {
			return av1.FramePool{}, err
		}
		return av1.BindFramePool(make([]byte, backingSize), format,
			make([]av1.Frame, surfacesPerLayer),
			make([]int, surfacesPerLayer),
			make([]bool, surfacesPerLayer))
	})
	layerPool, err := av1.BindFrameLayerPool(
		make([]av1.FramePool, layers),
		make([]av1.FrameFormat, layers),
		make([]bool, layers),
		256,
		factory,
	)
	if err != nil {
		t.Fatalf("BindFrameLayerPool: %v", err)
	}
	return layerPool
}

func scaledReferenceDecodeBaseFrame(width, height int) encoder.SourceFrame420 {
	cw, ch := width/2, height/2
	f := encoder.SourceFrame420{
		Y:            make([]byte, width*height),
		U:            make([]byte, cw*ch),
		V:            make([]byte, cw*ch),
		YStride:      width,
		ChromaStride: cw,
		Width:        width,
		Height:       height,
	}
	for y := range height {
		for x := range width {
			f.Y[y*width+x] = byte(47 + 3*x + 5*y + x*y)
		}
	}
	for y := range ch {
		for x := range cw {
			f.U[y*cw+x] = byte(96 + 3*x + 7*y)
			f.V[y*cw+x] = byte(151 + 5*x + 2*y)
		}
	}
	return f
}

func scaleSource420(src encoder.SourceFrame420, width, height int) encoder.SourceFrame420 {
	cw, ch := width/2, height/2
	dst := encoder.SourceFrame420{
		Y:            make([]byte, width*height),
		U:            make([]byte, cw*ch),
		V:            make([]byte, cw*ch),
		YStride:      width,
		ChromaStride: cw,
		Width:        width,
		Height:       height,
	}
	scalePlane(dst.Y, dst.YStride, width, height, src.Y, src.YStride, src.Width, src.Height)
	scalePlane(dst.U, dst.ChromaStride, cw, ch, src.U, src.ChromaStride, src.Width/2, src.Height/2)
	scalePlane(dst.V, dst.ChromaStride, cw, ch, src.V, src.ChromaStride, src.Width/2, src.Height/2)
	return dst
}

func scalePlane(dst []byte, dstStride, dstWidth, dstHeight int, src []byte, srcStride, srcWidth, srcHeight int) {
	for y := range dstHeight {
		sy := y * srcHeight / dstHeight
		for x := range dstWidth {
			sx := x * srcWidth / dstWidth
			dst[y*dstStride+x] = src[sy*srcStride+sx]
		}
	}
}
