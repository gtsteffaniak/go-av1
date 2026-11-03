package encode_test

import (
	"crypto/md5"
	"errors"
	av1 "github.com/gtsteffaniak/go-av1/pkg/av1"
	decode "github.com/gtsteffaniak/go-av1/pkg/decode"
	encode "github.com/gtsteffaniak/go-av1/pkg/encode"
	rtp "github.com/gtsteffaniak/go-av1/pkg/rtp"
	"testing"
)

func appendPublicAnnexBStream(dst []byte, temporalUnits ...[][][]byte) []byte {
	for _, temporalUnit := range temporalUnits {
		var temporal []byte
		for _, frameUnit := range temporalUnit {
			var frame []byte
			for _, obu := range frameUnit {
				frame = appendPublicLEB128(frame, uint32(len(obu)))
				frame = append(frame, obu...)
			}
			temporal = appendPublicLEB128(temporal, uint32(len(frame)))
			temporal = append(temporal, frame...)
		}
		dst = appendPublicLEB128(dst, uint32(len(temporal)))
		dst = append(dst, temporal...)
	}
	return dst
}

func appendPublicIVF(dst []byte, width uint16, height uint16, timebaseNum uint32, timebaseDen uint32, frames []publicIVFFrame) []byte {
	dst = append(dst,
		'D', 'K', 'I', 'F',
		0, 0,
		byte(av1.IVFFileHeaderSize), 0,
		'A', 'V', '0', '1',
		byte(width), byte(width>>8),
		byte(height), byte(height>>8),
		byte(timebaseNum), byte(timebaseNum>>8), byte(timebaseNum>>16), byte(timebaseNum>>24),
		byte(timebaseDen), byte(timebaseDen>>8), byte(timebaseDen>>16), byte(timebaseDen>>24),
		byte(len(frames)), byte(len(frames)>>8), byte(len(frames)>>16), byte(len(frames)>>24),
		0, 0, 0, 0,
	)
	for _, frame := range frames {
		size := uint32(len(frame.payload))
		dst = append(dst,
			byte(size), byte(size>>8), byte(size>>16), byte(size>>24),
			byte(frame.timestamp), byte(frame.timestamp>>8), byte(frame.timestamp>>16), byte(frame.timestamp>>24),
			byte(frame.timestamp>>32), byte(frame.timestamp>>40), byte(frame.timestamp>>48), byte(frame.timestamp>>56),
		)
		dst = append(dst, frame.payload...)
	}
	return dst
}

func appendPublicLEB128(dst []byte, value uint32) []byte {
	for {
		b := byte(value & 0x7f)
		value >>= 7
		if value != 0 {
			b |= 0x80
		}
		dst = append(dst, b)
		if value == 0 {
			return dst
		}
	}
}

func appendPublicLowOverheadOBU(dst []byte, typ av1.OBUType, payload []byte) []byte {
	var header [2]byte
	n, err := av1.PutOBUHeader(header[:], av1.OBUHeader{Type: typ, HasSizeField: true})
	if err != nil {
		panic(err)
	}
	dst = append(dst, header[:n]...)
	dst = appendPublicLEB128(dst, uint32(len(payload)))
	dst = append(dst, payload...)
	return dst
}

func benchMaxProbeEvents(payloads [][]byte) int {
	// Each frame typically expands to a handful of events. Reserve a
	// generous bound so the streams planner never returns
	// ErrDecoderEventBufferTooSmall during sizing.
	const eventsPerFrame = 16
	n := max(len(payloads)*eventsPerFrame, 64)
	return n
}

// decodeBenchmarkHarness owns the scratch a residual stream runner needs
// to decode benchFrames end-to-end. It is allocated once and reused across
// benchmark iterations so per-iteration allocations are zero.
type decodeBenchmarkHarness struct {
	pool       av1.FramePool
	workerPool *decode.TileWorkerPool
	stream     decode.DecoderStream

	refs       decode.DecoderSurfaceReferences
	state      decode.DecoderFrameWorkState
	refSurface []int
	refFrames  []*av1.Frame
	releases   []int
	stats      decode.DecoderFrameWorkTileResidualStats
	sideData   decode.DecoderFrameWorkSideData
	batch      decode.DecoderFrameWorkBatchResidualRunner

	scratch decode.DecoderFrameWorkResidualStreamScratch
	runner  decode.DecoderFrameWorkResidualStreamRunner

	payloads [][]byte
}

func (h *decodeBenchmarkHarness) Close() {
	if h.workerPool != nil {
		h.workerPool.Close()
		h.workerPool = nil
	}
}

func (h *decodeBenchmarkHarness) runOnce() (int, int, error) {
	h.pool.Reset()
	h.refs.Reset()
	h.state.Reset()
	h.stats = decode.DecoderFrameWorkTileResidualStats{}
	if err := h.runner.Reset(); err != nil {
		return 0, 0, err
	}
	var result decode.DecoderFrameWorkResidualStreamResult
	if err := h.runner.RunLowOverheadsInto(&result, h.payloads, nil); err != nil {
		return 0, 0, err
	}
	return result.Run.CompletedFrames, int(h.stats.TXBs), nil
}

func fillPublicMotionPlane(plane av1.FramePlane, bytesPerSample int) {
	for y := 0; y < plane.Height; y++ {
		for x := 0; x < plane.Width; x++ {
			setPublicFrameSample(plane, bytesPerSample, x, y, uint16(1000*(bytesPerSample-1)+10*y+x))
		}
	}
}

func fillPublicReconstructPlane(plane av1.FramePlane, bytesPerSample int, sample uint16) {
	for y := 0; y < plane.Height; y++ {
		for x := 0; x < plane.Width; x++ {
			setPublicFrameSample(plane, bytesPerSample, x, y, sample)
		}
	}
}

// frameMD5Visible reproduces the libaom per-frame digest layout: visible Y rows,
// then visible U rows, then visible V rows, each stripped of stride padding.
func frameMD5Visible(f *av1.Frame) [16]byte {
	h := md5.New()
	bps := f.Layout.BytesPerSample
	for _, plane := range []av1.FramePlane{f.Y, f.U, f.V} {
		if plane.Width == 0 || plane.Height == 0 || len(plane.Pix) == 0 {
			continue
		}
		rowBytes := plane.Width * bps
		for row := 0; row < plane.Height; row++ {
			off := row * plane.Stride
			h.Write(plane.Pix[off : off+rowBytes])
		}
	}
	var sum [16]byte
	h.Sum(sum[:0])
	return sum
}

func getPublicFrameSample(plane av1.FramePlane, bytesPerSample int, x int, y int) uint16 {
	off := y*plane.Stride + x*bytesPerSample
	if bytesPerSample == 1 {
		return uint16(plane.Pix[off])
	}
	return uint16(plane.Pix[off]) | uint16(plane.Pix[off+1])<<8
}

func newBenchBatchRunnerScratch(size decode.DecoderFrameWorkBatchResidualRunnerScratchSize) decode.DecoderFrameWorkBatchResidualRunnerScratch {
	return decode.DecoderFrameWorkBatchResidualRunnerScratch{
		States:                  make([]av1.TileDecodeState, size.Workers),
		Storages:                make([]decode.DecoderFrameWorkTileResidualCDFStorage, size.Workers),
		TileScratch:             make([]decode.DecoderFrameWorkTileResidualScratch, size.Workers),
		RestorationRequests:     make([]decode.DecoderFrameWorkTileRestorationRequest, size.RestorationRequests),
		PredictionScratch:       make([]decode.DecoderFrameWorkPredictionScratch, size.Workers),
		InterPredictionScratch:  make([]decode.DecoderFrameWorkInterPredictionScratch, size.Workers),
		Stats:                   make([]decode.DecoderFrameWorkTileResidualStats, size.Workers),
		Int32Scratch:            make([]int32, size.Int32Scratch),
		ResidualScratch:         make([]int16, size.ResidualScratch),
		LoopContextAboveScratch: make([]av1.TileBlockLoopRootAboveContext, size.LoopContextAbove),
	}
}

func newBenchEventScratch(size decode.DecoderFrameWorkResidualEventScratchSize) decode.DecoderFrameWorkResidualEventScratch {
	return decode.DecoderFrameWorkResidualEventScratch{
		Runner:   newBenchBatchRunnerScratch(size.Runner),
		SideData: newBenchSideDataScratch(size.SideData),
		Spans:    make([]av1.TileSpan, size.Plan.SpanCount),
		Jobs:     make([]av1.TileJob, size.Plan.JobCount),
		Batches:  make([]decode.TileBatch, size.Plan.BatchCount),
	}
}

func newBenchSideDataScratch(size decode.DecoderFrameWorkSideDataScratchSize) decode.DecoderFrameWorkSideDataScratch {
	return decode.DecoderFrameWorkSideDataScratch{
		CDEFIndexMap:             make([]uint8, size.CDEFIndexMap),
		CDEFReadMap:              make([]bool, size.CDEFReadMap),
		LoopFilterMap:            make([]decode.DecoderFrameWorkLoopFilterBlockRecord, size.LoopFilterMap),
		RestorationRecords:       make([]av1.TileRestorationUnitRecord, size.RestorationRecords),
		RestorationBoundaryAbove: make([]uint16, size.RestorationBoundaryAbove),
		RestorationBoundaryBelow: make([]uint16, size.RestorationBoundaryBelow),
	}
}

func newBenchStreamScratch(size decode.DecoderFrameWorkResidualStreamScratchSize) decode.DecoderFrameWorkResidualStreamScratch {
	return decode.DecoderFrameWorkResidualStreamScratch{
		Events:    make([]decode.DecoderEvent, size.Events),
		Event:     newBenchEventScratch(size.Event),
		SideData:  newBenchSideDataScratch(size.Event.SideData),
		Outputs:   make([]*av1.Frame, size.Event.Outputs),
		RTPBuffer: make([]byte, size.RTPBuffer),
		RTPSpans:  make([]rtp.RTPObuSpan, size.RTPSpans),
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

func publicAssertMalformedRTPPacketError(t testing.TB, label string, err error, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("%s err=%v want %v", label, err, want)
	}
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

type publicDecoderBlockCoeffTB interface {
	Helper()
	Fatalf(format string, args ...any)
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

func publicDecoderRTPPacketsForFrameWithLimits(t testing.TB, frame encode.RTCFrame, limits rtp.RTPPayloadSizeLimits) [][]byte {
	t.Helper()
	return publicDecoderRTPPacketsForFrameWithLimitsAndOptions(t, frame, limits, encode.EncoderWebRTCRTPPacketDependencyDescriptorOptions{})
}

func publicDecoderRTPPacketsForFrameWithLimitsAndOptions(t testing.TB, frame encode.RTCFrame, limits rtp.RTPPayloadSizeLimits, options encode.EncoderWebRTCRTPPacketDependencyDescriptorOptions) [][]byte {
	t.Helper()
	const dependencyDescriptorExtensionID = 42
	firstSize, err := frame.RTPPacketScratchLenWithOptions(limits, nil, options)
	if err != nil {
		t.Fatalf("RTPPacketScratchLenWithOptions first: %v", err)
	}
	obuScratch := make([]rtp.RTPPacketizerOBU, firstSize.Packetizer.OBUs)
	size, err := frame.RTPPacketScratchLenWithOptions(limits, obuScratch, options)
	if err != nil {
		t.Fatalf("RTPPacketScratchLenWithOptions full: %v", err)
	}
	packetScratch := make([]rtp.RTPPacketPlan, size.Packetizer.Packets)
	workScratch := make([]rtp.RTPPacketPlan, size.Packetizer.Work)
	payloadBuf := make([]byte, 0, size.Packetizer.Packets*size.MaxPayloadBytes)
	descriptorBuf := make([]byte, 0, size.Packetizer.Packets*size.MaxDescriptorBytes)
	spans := make([]encode.EncoderWebRTCRTPPacketSpan, size.Packetizer.Packets)
	rtpPayloads, descriptors, packetCount, err := frame.AppendRTPPacketsWithOptions(payloadBuf, descriptorBuf, spans, limits, obuScratch, packetScratch, workScratch, options)
	if err != nil {
		t.Fatalf("AppendRTPPacketsWithOptions: %v", err)
	}
	headerConfig := encode.EncoderWebRTCRTPPacketHeaderConfig{
		PayloadType:                     96,
		SequenceNumber:                  uint16(frame.FrameID * 31),
		Timestamp:                       uint32(frame.FrameID * 3000),
		SSRC:                            0x11223344 + uint32(frame.SpatialID),
		DependencyDescriptorExtensionID: dependencyDescriptorExtensionID,
	}
	packetSize, err := encode.EncoderWebRTCRTPPacketsWithHeadersSize(headerConfig, rtpPayloads, descriptors, spans[:packetCount])
	if err != nil {
		t.Fatalf("EncoderWebRTCRTPPacketsWithHeadersSize: %v", err)
	}
	headerSpans := make([]encode.EncoderWebRTCRTPPacketHeaderSpan, packetCount)
	packets, fullCount, err := encode.AppendEncoderWebRTCRTPPacketsWithHeaders(make([]byte, 0, packetSize.Bytes), headerSpans, headerConfig, rtpPayloads, descriptors, spans[:packetCount])
	if err != nil {
		t.Fatalf("AppendEncoderWebRTCRTPPacketsWithHeaders: %v", err)
	}
	if fullCount != packetCount {
		t.Fatalf("full packet count=%d want %d", fullCount, packetCount)
	}
	out := make([][]byte, fullCount)
	for i := 0; i < fullCount; i++ {
		span := headerSpans[i]
		out[i] = append([]byte(nil), packets[span.Offset:span.Offset+span.Length]...)
	}
	return out
}

func publicDecoderRTPPayloadsForFrameWithLimits(t testing.TB, frame encode.RTCFrame, limits rtp.RTPPayloadSizeLimits) [][]byte {
	t.Helper()
	firstSize, err := frame.RTPPacketScratchLen(limits, nil)
	if err != nil {
		t.Fatalf("RTPPacketScratchLen first: %v", err)
	}
	obuScratch := make([]rtp.RTPPacketizerOBU, firstSize.Packetizer.OBUs)
	size, err := frame.RTPPacketScratchLen(limits, obuScratch)
	if err != nil {
		t.Fatalf("RTPPacketScratchLen full: %v", err)
	}
	packetScratch := make([]rtp.RTPPacketPlan, size.Packetizer.Packets)
	workScratch := make([]rtp.RTPPacketPlan, size.Packetizer.Work)
	payloadBuf := make([]byte, 0, size.Packetizer.Packets*size.MaxPayloadBytes)
	descriptorBuf := make([]byte, 0, size.Packetizer.Packets*size.MaxDescriptorBytes)
	spans := make([]encode.EncoderWebRTCRTPPacketSpan, size.Packetizer.Packets)
	rtpPayloads, _, packetCount, err := frame.AppendRTPPackets(payloadBuf, descriptorBuf, spans, limits, obuScratch, packetScratch, workScratch)
	if err != nil {
		t.Fatalf("AppendRTPPackets: %v", err)
	}
	out := make([][]byte, packetCount)
	for i := 0; i < packetCount; i++ {
		span := spans[i]
		out[i] = append([]byte(nil), rtpPayloads[span.PayloadOffset:span.PayloadOffset+span.PayloadLength]...)
	}
	return out
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

func publicFilmGrainLumaOffset(n int) int {
	return 3 + av1.FilmGrainLumaOverlapSamples*(3+n)
}

func publicFilmGrainRowBuffers(width int, height int, stride int, value uint16) ([]uint16, []uint16) {
	dst := make([]uint16, stride*height)
	src := make([]uint16, stride*height)
	for y := range height {
		for x := range width {
			src[y*stride+x] = value
		}
	}
	return dst, src
}

func publicFilmGrainScaling(value uint8) [av1.FilmGrainScalingLUTSize]uint8 {
	var lut [av1.FilmGrainScalingLUTSize]uint8
	for i := range lut {
		lut[i] = value
	}
	return lut
}

type publicIVFFrame struct {
	timestamp uint64
	payload   []byte
}

type publicMalformedRTPPacketCase struct {
	name   string
	packet []byte
	want   error
}

func publicMalformedRTPPacketCases() []publicMalformedRTPPacketCase {
	return []publicMalformedRTPPacketCase{
		{
			name:   "short fixed header",
			packet: []byte{0x80},
			want:   rtp.ErrRTPShortPayload,
		},
		{
			name: "invalid version",
			packet: []byte{
				0x00, 96, 0, 1,
				0, 0, 0, 1,
				0, 0, 0, 2,
			},
			want: rtp.ErrRTPInvalidHeader,
		},
		{
			name: "truncated extension envelope",
			packet: []byte{
				0x90, 96, 0, 1,
				0, 0, 0, 1,
				0, 0, 0, 2,
				0xbe, 0xde, 0, 1,
			},
			want: rtp.ErrRTPShortPayload,
		},
		{
			name: "invalid padding",
			packet: []byte{
				0xa0, 96, 0, 1,
				0, 0, 0, 1,
				0, 0, 0, 2,
				0xff, 3,
			},
			want: rtp.ErrRTPInvalidHeader,
		},
	}
}

func publicPredictionPlane(width int, height int, bytesPerSample int, strideSamples int) av1.FramePlane {
	return av1.FramePlane{
		Pix:    make([]byte, strideSamples*height*bytesPerSample),
		Stride: strideSamples * bytesPerSample,
		Width:  width,
		Height: height,
	}
}

func publicRewriteRTPPacketSequence(t testing.TB, packet []byte, sequenceNumber uint16) []byte {
	t.Helper()
	parsed, err := rtp.ParseRTPPacket(packet)
	if err != nil {
		t.Fatalf("ParseRTPPacket rewrite sequence: %v", err)
	}
	header := parsed.Header
	header.SequenceNumber = sequenceNumber
	size, err := rtp.RTPPacketSize(header, parsed.Payload, header.PaddingSize)
	if err != nil {
		t.Fatalf("RTPPacketSize rewrite sequence: %v", err)
	}
	buf := make([]byte, size)
	n, err := rtp.PutRTPPacket(buf, header, parsed.Payload, header.PaddingSize)
	if err != nil {
		t.Fatalf("PutRTPPacket rewrite sequence: %v", err)
	}
	return buf[:n]
}

func publicRewriteRTPPacketSequences(t testing.TB, packets [][]byte, next *uint16) [][]byte {
	t.Helper()
	out := make([][]byte, len(packets))
	for i := range packets {
		out[i] = publicRewriteRTPPacketSequence(t, packets[i], *next)
		*next = *next + 1
	}
	return out
}

func publicSetFilmGrainLuma(grain []int16, offset uint8, blockCol int, blockRow int, x int, y int, value int16) {
	col := publicFilmGrainLumaOffset(int(offset>>4)) + x + av1.FilmGrainLumaBlockSize*blockCol
	row := publicFilmGrainLumaOffset(int(offset&0x0f)) + y + av1.FilmGrainLumaBlockSize*blockRow
	grain[row*av1.FilmGrainLumaGrainWidth+col] = value
}

func sameRTPVideoLayersAllocation(a, b rtp.RTPVideoLayersAllocation) bool {
	if a.RTPStreamID != b.RTPStreamID ||
		a.RTPStreamCount != b.RTPStreamCount ||
		a.HasResolutionAndFramerate != b.HasResolutionAndFramerate ||
		len(a.ActiveSpatialLayers) != len(b.ActiveSpatialLayers) {
		return false
	}
	for i := range a.ActiveSpatialLayers {
		la := a.ActiveSpatialLayers[i]
		lb := b.ActiveSpatialLayers[i]
		if la.RTPStreamID != lb.RTPStreamID ||
			la.SpatialID != lb.SpatialID ||
			la.Width != lb.Width ||
			la.Height != lb.Height ||
			la.Framerate != lb.Framerate ||
			len(la.TargetBitratesKbps) != len(lb.TargetBitratesKbps) {
			return false
		}
		for j := range la.TargetBitratesKbps {
			if la.TargetBitratesKbps[j] != lb.TargetBitratesKbps[j] {
				return false
			}
		}
	}
	return true
}

func setPublicFrameSample(plane av1.FramePlane, bytesPerSample int, x int, y int, sample uint16) {
	off := y*plane.Stride + x*bytesPerSample
	plane.Pix[off] = byte(sample)
	if bytesPerSample == 2 {
		plane.Pix[off+1] = byte(sample >> 8)
	}
}

func testAV1RTCPFeedbackCompound(t *testing.T, packet rtp.RTCPFeedbackPacket) []byte {
	t.Helper()
	compound, err := rtp.AppendRTCPFeedbackPacket(
		make([]byte, 0, rtp.RTCPFeedbackPacketHeaderSize+len(packet.FCI)),
		packet,
	)
	if err != nil {
		t.Fatalf("AppendRTCPFeedbackPacket: %v", err)
	}
	return compound
}

func testAV1RTCPLayerRefreshFCI(t *testing.T, entries []rtp.AV1RTCPLayerRefreshRequestEntry) []byte {
	t.Helper()
	size, err := rtp.AV1RTCPLayerRefreshRequestEntriesSize(entries)
	if err != nil {
		t.Fatalf("AV1RTCPLayerRefreshRequestEntriesSize: %v", err)
	}
	fci, err := rtp.AppendAV1RTCPLayerRefreshRequestEntries(make([]byte, 0, size), entries)
	if err != nil {
		t.Fatalf("AppendAV1RTCPLayerRefreshRequestEntries: %v", err)
	}
	return fci
}

func testAV1RTCPValidLayerRefreshEntry(t *testing.T, mode encode.EncoderScalabilityMode) rtp.AV1RTCPLayerRefreshRequestEntry {
	t.Helper()
	spatialLayers, temporalLayers, _, ok := mode.Layers()
	if !ok {
		t.Fatalf("invalid scalability mode %s", mode)
	}
	target := rtp.AV1RTCPLayerRefreshLayerIndex{
		TemporalID: temporalLayers - 1,
		SpatialID:  spatialLayers - 1,
	}
	entry := rtp.AV1RTCPLayerRefreshRequestEntry{
		SSRC:           0x05060708,
		SequenceNumber: 17,
		PayloadType:    96,
		Target:         target,
	}
	if target != (rtp.AV1RTCPLayerRefreshLayerIndex{}) {
		entry.CurrentPresent = true
	}
	return entry
}
