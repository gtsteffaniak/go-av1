package decode_test

import (
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

func assertPublicRTCConfigBitrates(t *testing.T, got encode.EncoderConfig, want encode.EncoderConfig) {
	t.Helper()
	spatialLayers, _, _, ok := got.Scalability.Layers()
	if !ok {
		t.Fatalf("invalid normalized mode %s", got.Scalability)
	}
	want, err := encode.SetWebRTCEncoderSVCConfig(want, want.TemporalLayerCount, want.SpatialLayerCount)
	if err != nil {
		t.Fatalf("normalize expected config %s: %v", want.Scalability, err)
	}
	if got.SpatialLayerCount != spatialLayers {
		t.Fatalf("config spatial layers=%d want %d", got.SpatialLayerCount, spatialLayers)
	}
	if want.SpatialLayerCount != spatialLayers {
		t.Fatalf("expected config spatial layers=%d want %d", want.SpatialLayerCount, spatialLayers)
	}
	for i := uint8(0); i < spatialLayers; i++ {
		gotLayer := got.SpatialLayers[i]
		wantLayer := want.SpatialLayers[i]
		if gotLayer.MinBitrateKbps != wantLayer.MinBitrateKbps ||
			gotLayer.MaxBitrateKbps != wantLayer.MaxBitrateKbps ||
			gotLayer.TargetBitrateKbps != wantLayer.TargetBitrateKbps {
			t.Fatalf("layer %d bitrate got %+v want min=%d max=%d target=%d", i, gotLayer, wantLayer.MinBitrateKbps, wantLayer.MaxBitrateKbps, wantLayer.TargetBitrateKbps)
		}
	}
}

func assertPublicRTCConfigControls(t *testing.T, got encode.EncoderConfig, want encode.EncoderConfig) {
	t.Helper()
	want, err := encode.SetWebRTCEncoderSVCConfig(want, want.TemporalLayerCount, want.SpatialLayerCount)
	if err != nil {
		t.Fatalf("normalize expected config %s: %v", want.Scalability, err)
	}
	if got.Scalability != want.Scalability ||
		got.SpatialLayerCount != want.SpatialLayerCount ||
		got.TemporalLayerCount != want.TemporalLayerCount {
		t.Fatalf("config layers got mode=%s S%d/T%d want mode=%s S%d/T%d",
			got.Scalability, got.SpatialLayerCount, got.TemporalLayerCount,
			want.Scalability, want.SpatialLayerCount, want.TemporalLayerCount)
	}
	if got.MaxFramerate != want.MaxFramerate || got.RTPTimebase != want.RTPTimebase {
		t.Fatalf("config timing fps=%+v timebase=%+v want fps=%+v timebase=%+v",
			got.MaxFramerate, got.RTPTimebase, want.MaxFramerate, want.RTPTimebase)
	}
	gotDuration, err := encode.EncoderWebRTCRTPFrameDuration(got)
	if err != nil {
		t.Fatalf("got RTP frame duration: %v", err)
	}
	wantDuration, err := encode.EncoderWebRTCRTPFrameDuration(want)
	if err != nil {
		t.Fatalf("want RTP frame duration: %v", err)
	}
	if gotDuration != wantDuration {
		t.Fatalf("RTP frame duration=%+v want %+v", gotDuration, wantDuration)
	}
	if got.RateControl != want.RateControl || got.Quantizer != want.Quantizer || got.Content != want.Content {
		t.Fatalf("config rate control=%d q=%d content=%d want %d q=%d content=%d",
			got.RateControl, got.Quantizer, got.Content, want.RateControl, want.Quantizer, want.Content)
	}
	if got.Speed != want.Speed || got.MaxThreads != want.MaxThreads || got.KeyFrameInterval != want.KeyFrameInterval {
		t.Fatalf("config speed=%d threads=%d keyInterval=%d want speed=%d threads=%d keyInterval=%d",
			got.Speed, got.MaxThreads, got.KeyFrameInterval, want.Speed, want.MaxThreads, want.KeyFrameInterval)
	}
	for i := uint8(0); i < want.SpatialLayerCount; i++ {
		gotLayer := got.SpatialLayers[i]
		wantLayer := want.SpatialLayers[i]
		if gotLayer.Active != wantLayer.Active ||
			gotLayer.Resolution != wantLayer.Resolution ||
			gotLayer.ScalingFactor != wantLayer.ScalingFactor ||
			gotLayer.MaxFramerate != wantLayer.MaxFramerate ||
			gotLayer.TemporalLayers != wantLayer.TemporalLayers {
			t.Fatalf("layer %d controls got %+v want %+v", i, gotLayer, wantLayer)
		}
	}
	assertPublicRTCConfigBitrates(t, got, want)
}

func decodePublicLayeredDecoderLowOverheadDigests(t *testing.T, payloads ...[]byte) [][16]byte {
	t.Helper()
	dec, err := decode.NewLayeredDecoder(payloads)
	if err != nil {
		t.Fatalf("NewLayeredDecoder: %v", err)
	}
	defer dec.Close()

	var out [][16]byte
	for {
		frames, ok, err := dec.DecodeNext()
		if err != nil {
			t.Fatalf("LayeredDecoder low-overhead DecodeNext: %v", err)
		}
		if !ok {
			break
		}
		for _, frame := range frames {
			out = append(out, frameMD5Visible(frame))
		}
	}
	return out
}

func decodePublicLayeredDecoderRTPPacketDigests(t *testing.T, packets ...[]byte) [][16]byte {
	t.Helper()
	dec, err := decode.NewLayeredDecoderFromRTPPackets(packets)
	if err != nil {
		t.Fatalf("NewLayeredDecoderFromRTPPackets: %v", err)
	}
	defer dec.Close()

	var out [][16]byte
	for {
		frames, ok, err := dec.DecodeNext()
		if err != nil {
			t.Fatalf("LayeredDecoder packet DecodeNext: %v", err)
		}
		if !ok {
			break
		}
		for _, frame := range frames {
			out = append(out, frameMD5Visible(frame))
		}
	}
	if err := dec.Reset(); err != nil {
		t.Fatalf("LayeredDecoder packet Reset: %v", err)
	}
	if _, ok, err := dec.DecodeNext(); err != nil || !ok {
		t.Fatalf("LayeredDecoder packet DecodeNext after Reset ok=%v err=%v", ok, err)
	}
	return out
}

func decodePublicLayeredDecoderRTPPayloadDigests(t *testing.T, payloads ...[]byte) [][16]byte {
	t.Helper()
	dec, err := decode.NewLayeredDecoderFromRTPPayloads(payloads)
	if err != nil {
		t.Fatalf("NewLayeredDecoderFromRTPPayloads: %v", err)
	}
	defer dec.Close()

	var out [][16]byte
	for {
		frames, ok, err := dec.DecodeNext()
		if err != nil {
			t.Fatalf("LayeredDecoder DecodeNext: %v", err)
		}
		if !ok {
			break
		}
		for _, frame := range frames {
			out = append(out, frameMD5Visible(frame))
		}
	}
	if err := dec.Reset(); err != nil {
		t.Fatalf("LayeredDecoder Reset: %v", err)
	}
	if _, ok, err := dec.DecodeNext(); err != nil || !ok {
		t.Fatalf("LayeredDecoder DecodeNext after Reset ok=%v err=%v", ok, err)
	}
	return out
}

func decodePublicLayeredLiveDecoderRTPPacketDigests(t *testing.T, probePackets [][]byte, packets ...[]byte) [][16]byte {
	t.Helper()
	dec, err := decode.NewLayeredDecoderFromRTPPackets(probePackets)
	if err != nil {
		t.Fatalf("NewLayeredDecoderFromRTPPackets live: %v", err)
	}
	defer dec.Close()
	if err := dec.Reset(); err != nil {
		t.Fatalf("LayeredDecoder live packet Reset: %v", err)
	}

	var out [][16]byte
	for i, packet := range packets {
		frames, err := dec.DecodeRTPPacket(packet)
		if err != nil {
			t.Fatalf("LayeredDecoder DecodeRTPPacket packet %d: %v", i, err)
		}
		for _, frame := range frames {
			out = append(out, frameMD5Visible(frame))
		}
	}
	return out
}

func decodePublicLayeredLiveDecoderRTPPayloadDigests(t *testing.T, probePayloads [][]byte, payloads ...[]byte) [][16]byte {
	t.Helper()
	dec, err := decode.NewLayeredDecoderFromRTPPayloads(probePayloads)
	if err != nil {
		t.Fatalf("NewLayeredDecoderFromRTPPayloads live: %v", err)
	}
	defer dec.Close()
	if err := dec.Reset(); err != nil {
		t.Fatalf("LayeredDecoder live Reset: %v", err)
	}

	var out [][16]byte
	for i, payload := range payloads {
		frames, err := dec.DecodeRTPPayload(payload)
		if err != nil {
			t.Fatalf("LayeredDecoder DecodeRTPPayload packet %d: %v", i, err)
		}
		for _, frame := range frames {
			out = append(out, frameMD5Visible(frame))
		}
	}
	return out
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

func getPublicFrameSample(plane av1.FramePlane, bytesPerSample int, x int, y int) uint16 {
	off := y*plane.Stride + x*bytesPerSample
	if bytesPerSample == 1 {
		return uint16(plane.Pix[off])
	}
	return uint16(plane.Pix[off]) | uint16(plane.Pix[off+1])<<8
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

func publicPredictionPlane(width int, height int, bytesPerSample int, strideSamples int) av1.FramePlane {
	return av1.FramePlane{
		Pix:    make([]byte, strideSamples*height*bytesPerSample),
		Stride: strideSamples * bytesPerSample,
		Width:  width,
		Height: height,
	}
}

func publicRTCApplyControlBitrates(cfg *encode.EncoderConfig, targetKbps int32) {
	cfg.MinBitrateKbps = targetKbps / 4
	cfg.MaxBitrateKbps = targetKbps * 2
	cfg.TargetBitrateKbps = targetKbps
	spatialLayers, _, _, ok := cfg.Scalability.Layers()
	if !ok {
		return
	}
	for i := uint8(0); i < spatialLayers; i++ {
		layer := &cfg.SpatialLayers[i]
		if spatialLayers == 1 {
			layer.MinBitrateKbps = cfg.MinBitrateKbps
			layer.MaxBitrateKbps = cfg.MaxBitrateKbps
			layer.TargetBitrateKbps = cfg.TargetBitrateKbps
			continue
		}
		layerTarget := targetKbps * int32(i+1) / int32(spatialLayers)
		layer.MinBitrateKbps = layerTarget / 2
		layer.MaxBitrateKbps = layerTarget * 2
		layer.TargetBitrateKbps = layerTarget
	}
}

func publicRTCMatrixConfig(width, height int, mode encode.EncoderScalabilityMode) encode.EncoderConfig {
	return encode.EncoderConfig{
		Resolution:        encode.EncoderResolution{Width: int32(width), Height: int32(height)},
		MaxFramerate:      encode.EncoderRational{Num: 30, Den: 1},
		MinBitrateKbps:    120,
		MaxBitrateKbps:    1600,
		TargetBitrateKbps: 900,
		Scalability:       mode,
	}
}

func publicRTCMatrixControlBitrateKbps(t *testing.T, mode encode.EncoderScalabilityMode) int32 {
	t.Helper()
	spatial, _, _, ok := mode.Layers()
	if !ok {
		t.Fatalf("invalid scalability mode %s", mode)
	}
	switch spatial {
	case 1:
		return 420
	case 2:
		return 900
	case 3:
		return 1500
	default:
		t.Fatalf("unsupported spatial layer count %d for %s", spatial, mode)
		return 0
	}
}

func publicRTCMatrixFrame(width, height int, n int) encode.I420Frame {
	cw, ch := width/2, height/2
	f := encode.I420Frame{
		Y: make([]byte, width*height), U: make([]byte, cw*ch), V: make([]byte, cw*ch),
		YStride: width, ChromaStride: cw, Width: width, Height: height,
	}
	for y := 0; y < height; y++ {
		row := f.Y[y*width : y*width+width]
		for x := range row {
			row[x] = uint8(45 + (x*3+y*5+n*17)%175)
		}
	}
	for i := range f.U {
		f.U[i] = uint8(116 + (i+n)%9)
		f.V[i] = uint8(132 - (i+n)%7)
	}
	return f
}

func publicRTCMatrixGeometry(t *testing.T, mode encode.EncoderScalabilityMode) (int, int) {
	t.Helper()
	spatial, _, _, ok := mode.Layers()
	if !ok {
		t.Fatalf("invalid scalability mode %s", mode)
	}
	switch spatial {
	case 1:
		return 192, 128
	case 2:
		return 640, 360
	case 3:
		return 1008, 576
	default:
		t.Fatalf("unsupported spatial layer count %d for %s", spatial, mode)
		return 0, 0
	}
}

func publicRTCSharedReferenceSlotMode(mode encode.EncoderScalabilityMode) bool {
	return mode.UsesSharedReferenceSlots()
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

func warmPublicRTCEncoderEncodePictureHotPath(tb testing.TB, enc *encode.RTCEncoder, frames []encode.I420Frame) {
	tb.Helper()
	for i := 0; i < len(frames)*2; i++ {
		forceKey := i == 0 || i == len(frames)
		picture, err := enc.EncodePicture(frames[i%len(frames)], forceKey)
		if err != nil {
			tb.Fatalf("warm EncodePicture(%d): %v", i, err)
		}
		if picture.FrameNum == 0 {
			tb.Fatalf("warm EncodePicture(%d) emitted no frames", i)
		}
	}
}
