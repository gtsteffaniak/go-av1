package rtp_test

import (
	"bytes"
	"errors"
	rtp "github.com/gtsteffaniak/go-av1/pkg/rtp"
	"testing"
)

func TestPublicRTCPGenericNACKPairSequenceNumbers(t *testing.T) {
	pairs := []rtp.RTCPGenericNACKPair{
		{PacketID: 100, LostPacketBitmask: 0x0005},
		{PacketID: 0xffff, LostPacketBitmask: 0x0003},
	}
	got, err := rtp.AppendRTCPGenericNACKPairSequenceNumbers(make([]uint16, 0, 6), pairs)
	if err != nil {
		t.Fatalf("AppendRTCPGenericNACKPairSequenceNumbers: %v", err)
	}
	want := []uint16{100, 101, 103, 0xffff, 0, 1}
	if !equalUint16s(got, want) {
		t.Fatalf("NACK sequence numbers=%v want %v", got, want)
	}
	prefix := make([]uint16, 1, 4)
	prefix[0] = 7
	prefixed, err := rtp.AppendRTCPGenericNACKPairSequenceNumbers(prefix[:1:4], pairs[:1])
	if err != nil {
		t.Fatalf("AppendRTCPGenericNACKPairSequenceNumbers prefixed: %v", err)
	}
	if !equalUint16s(prefixed, []uint16{7, 100, 101, 103}) {
		t.Fatalf("prefixed NACK sequence numbers=%v", prefixed)
	}
	if _, err := rtp.AppendRTCPGenericNACKPairSequenceNumbers(make([]uint16, 0, 2), pairs[:1]); !errors.Is(err, rtp.ErrRTCPShortBuffer) {
		t.Fatalf("short NACK sequence dst err=%v want %v", err, rtp.ErrRTCPShortBuffer)
	}
	if out, err := rtp.AppendRTCPGenericNACKPairSequenceNumbers(nil, nil); err != nil || len(out) != 0 {
		t.Fatalf("empty NACK sequence out=%v err=%v", out, err)
	}
}

func TestPublicRTPRetransmissionCacheStoresAndAppendsNACKPackets(t *testing.T) {
	if _, err := rtp.BindRTPRetransmissionCache(nil); !errors.Is(err, rtp.ErrRTPInvalidRetransmissionCache) {
		t.Fatalf("BindRTPRetransmissionCache nil err=%v want %v", err, rtp.ErrRTPInvalidRetransmissionCache)
	}
	cache := bindTestRetransmissionCache(t, 8, 64)
	packets := [][]byte{
		testRetransmissionRTPPacket(t, 0xfffe, []byte("before-wrap")),
		testRetransmissionRTPPacket(t, 0xffff, []byte("wrap-a")),
		testRetransmissionRTPPacket(t, 0, []byte("wrap-b")),
		testRetransmissionRTPPacket(t, 1, []byte("wrap-c")),
	}
	for _, packet := range packets {
		if err := cache.Store(packet); err != nil {
			t.Fatalf("Store seq %d: %v", packetSequence(t, packet), err)
		}
	}
	packets[1][len(packets[1])-1] ^= 0xff
	if !cache.Contains(0xffff) || cache.Contains(2) {
		t.Fatalf("cache contains wrap=%v missing=%v", cache.Contains(0xffff), cache.Contains(2))
	}
	pairs := []rtp.RTCPGenericNACKPair{{PacketID: 0xffff, LostPacketBitmask: 0x0003}}
	spans := make([]rtp.RTPRetransmissionPacketSpan, 3)
	out, count, err := cache.AppendPacketsForRTCPGenericNACKPairs(make([]byte, 0, 256), spans, pairs)
	if err != nil {
		t.Fatalf("AppendPacketsForRTCPGenericNACKPairs: %v", err)
	}
	if count != 3 {
		t.Fatalf("retransmission count=%d want 3", count)
	}
	wantSeqs := []uint16{0xffff, 0, 1}
	for i := 0; i < count; i++ {
		span := spans[i]
		if span.SequenceNumber != wantSeqs[i] {
			t.Fatalf("span %d seq=%d want %d", i, span.SequenceNumber, wantSeqs[i])
		}
		got := out[span.Offset : span.Offset+span.Length]
		if gotSeq := packetSequence(t, got); gotSeq != wantSeqs[i] {
			t.Fatalf("span %d packet seq=%d want %d", i, gotSeq, wantSeqs[i])
		}
	}
	if bytes.Equal(out[spans[0].Offset:spans[0].Offset+spans[0].Length], packets[1]) {
		t.Fatal("cached packet aliases caller-owned input after Store")
	}

	missingOnly := []rtp.RTCPGenericNACKPair{{PacketID: 22}}
	out, count, err = cache.AppendPacketsForRTCPGenericNACKPairs(out[:0], spans, missingOnly)
	if err != nil || count != 0 || len(out) != 0 {
		t.Fatalf("missing-only NACK out=%d count=%d err=%v", len(out), count, err)
	}
}

func TestPublicRTPRetransmissionCacheEvictionResetAndShortBuffers(t *testing.T) {
	cache := bindTestRetransmissionCache(t, 2, 40)
	first := testRetransmissionRTPPacket(t, 1, []byte("one"))
	replacement := testRetransmissionRTPPacket(t, 3, []byte("three"))
	if err := cache.Store(first); err != nil {
		t.Fatalf("Store first: %v", err)
	}
	if err := cache.Store(replacement); err != nil {
		t.Fatalf("Store replacement: %v", err)
	}
	if cache.Contains(1) || !cache.Contains(3) {
		t.Fatalf("eviction contains seq1=%v seq3=%v", cache.Contains(1), cache.Contains(3))
	}
	out, found, err := cache.AppendPacket(make([]byte, 0, len(replacement)), 3)
	if err != nil || !found || !bytes.Equal(out, replacement) {
		t.Fatalf("AppendPacket out=% x found=%v err=%v", out, found, err)
	}
	if _, found, err := cache.AppendPacket(out[:0], 1); err != nil || found {
		t.Fatalf("AppendPacket evicted found=%v err=%v", found, err)
	}
	if _, found, err := cache.AppendPacket(make([]byte, 0, len(replacement)-1), 3); !found || !errors.Is(err, rtp.ErrRTPShortBuffer) {
		t.Fatalf("short AppendPacket found=%v err=%v want %v", found, err, rtp.ErrRTPShortBuffer)
	}
	spans := make([]rtp.RTPRetransmissionPacketSpan, 0)
	if _, _, err := cache.AppendPacketsForRTCPGenericNACKPairs(make([]byte, 0, len(replacement)), spans, []rtp.RTCPGenericNACKPair{{PacketID: 3}}); !errors.Is(err, rtp.ErrRTPShortBuffer) {
		t.Fatalf("short retransmission spans err=%v want %v", err, rtp.ErrRTPShortBuffer)
	}
	cache.Reset()
	if cache.Contains(3) {
		t.Fatal("Reset left cached sequence present")
	}

	shortSlots := []rtp.RTPRetransmissionCacheSlot{{Packet: make([]byte, 0, 8)}}
	shortCache, err := rtp.BindRTPRetransmissionCache(shortSlots)
	if err != nil {
		t.Fatalf("Bind short slot cache: %v", err)
	}
	if err := shortCache.Store(first); !errors.Is(err, rtp.ErrRTPShortBuffer) {
		t.Fatalf("short Store err=%v want %v", err, rtp.ErrRTPShortBuffer)
	}
	var zero rtp.RTPRetransmissionCache
	if err := zero.Store(first); !errors.Is(err, rtp.ErrRTPInvalidRetransmissionCache) {
		t.Fatalf("zero Store err=%v want %v", err, rtp.ErrRTPInvalidRetransmissionCache)
	}
}

func TestPublicRTPRetransmissionCacheAllocs(t *testing.T) {
	cache := bindTestRetransmissionCache(t, 8, 64)
	for seq := uint16(10); seq < 14; seq++ {
		if err := cache.Store(testRetransmissionRTPPacket(t, seq, []byte{byte(seq)})); err != nil {
			t.Fatalf("Store seq %d: %v", seq, err)
		}
	}
	pairs := []rtp.RTCPGenericNACKPair{{PacketID: 10, LostPacketBitmask: 0x0007}}
	dst := make([]byte, 0, 256)
	spans := make([]rtp.RTPRetransmissionPacketSpan, 4)
	allocs := testing.AllocsPerRun(1000, func() {
		var err error
		dst, _, err = cache.AppendPacketsForRTCPGenericNACKPairs(dst[:0], spans, pairs)
		if err != nil {
			t.Fatalf("AppendPacketsForRTCPGenericNACKPairs: %v", err)
		}
	})
	if allocs != 0 {
		t.Fatalf("RTPRetransmissionCache append allocs/run=%f want 0", allocs)
	}
}

func TestPublicRTPRTXPacketWrapParseAndRestore(t *testing.T) {
	originalPayload := []byte{0x81, 0x55, 0x66, 0x77}
	originalHeader := rtp.RTPHeader{
		Marker:           true,
		PayloadType:      96,
		SequenceNumber:   0xfffe,
		Timestamp:        0x01020304,
		SSRC:             0x11111111,
		CSRCCount:        2,
		ExtensionProfile: rtp.RTPExtensionProfileOneByte,
		ExtensionPayload: []byte{0x21, 0xaa, 0xbb, 0x00},
	}
	originalHeader.CSRC[0] = 0xaaaa0001
	originalHeader.CSRC[1] = 0xaaaa0002
	originalSize, err := rtp.RTPPacketSize(originalHeader, originalPayload, 3)
	if err != nil {
		t.Fatalf("RTPPacketSize original: %v", err)
	}
	original := make([]byte, originalSize)
	n, err := rtp.PutRTPPacket(original, originalHeader, originalPayload, 3)
	if err != nil {
		t.Fatalf("PutRTPPacket original: %v", err)
	}
	original = original[:n]

	config := rtp.RTPRTXPacketConfig{
		PayloadType:    97,
		SequenceNumber: 7,
		SSRC:           0x22222222,
	}
	rtxSize, err := rtp.RTPRTXPacketSize(original, config)
	if err != nil {
		t.Fatalf("RTPRTXPacketSize: %v", err)
	}
	rtx := make([]byte, rtxSize)
	n, err = rtp.PutRTPRTXPacket(rtx, original, config)
	if err != nil {
		t.Fatalf("PutRTPRTXPacket: %v", err)
	}
	if n != rtxSize {
		t.Fatalf("RTX packet size n=%d want %d", n, rtxSize)
	}
	rtx = rtx[:n]

	parsed, err := rtp.ParseRTPRTXPacket(rtx)
	if err != nil {
		t.Fatalf("ParseRTPRTXPacket: %v", err)
	}
	if parsed.Header.PayloadType != 97 ||
		parsed.Header.SequenceNumber != 7 ||
		parsed.Header.Timestamp != originalHeader.Timestamp ||
		parsed.Header.SSRC != config.SSRC ||
		!parsed.Header.Marker ||
		parsed.Header.Padding {
		t.Fatalf("parsed RTX header=%+v", parsed.Header)
	}
	if parsed.Header.CSRCCount != originalHeader.CSRCCount ||
		parsed.Header.CSRC[0] != originalHeader.CSRC[0] ||
		parsed.Header.CSRC[1] != originalHeader.CSRC[1] {
		t.Fatalf("parsed RTX CSRC=%+v", parsed.Header.CSRC[:parsed.Header.CSRCCount])
	}
	if parsed.Header.ExtensionProfile != originalHeader.ExtensionProfile ||
		!bytes.Equal(parsed.Header.ExtensionPayload, originalHeader.ExtensionPayload) {
		t.Fatalf("parsed RTX extension profile=%#x payload=%#v", parsed.Header.ExtensionProfile, parsed.Header.ExtensionPayload)
	}
	if parsed.OriginalSequenceNumber != originalHeader.SequenceNumber ||
		!bytes.Equal(parsed.OriginalPayload, originalPayload) {
		t.Fatalf("parsed RTX OSN=%d payload=%#v", parsed.OriginalSequenceNumber, parsed.OriginalPayload)
	}

	restoredSize, err := rtp.RTPPacketFromRTXSize(rtx, originalHeader.PayloadType, originalHeader.SSRC)
	if err != nil {
		t.Fatalf("RTPPacketFromRTXSize: %v", err)
	}
	restored := make([]byte, restoredSize)
	n, err = rtp.PutRTPPacketFromRTX(restored, rtx, originalHeader.PayloadType, originalHeader.SSRC)
	if err != nil {
		t.Fatalf("PutRTPPacketFromRTX: %v", err)
	}
	if n != restoredSize {
		t.Fatalf("restored size n=%d want %d", n, restoredSize)
	}
	restoredPacket, err := rtp.ParseRTPPacket(restored[:n])
	if err != nil {
		t.Fatalf("ParseRTPPacket restored: %v", err)
	}
	if restoredPacket.Header.PayloadType != originalHeader.PayloadType ||
		restoredPacket.Header.SequenceNumber != originalHeader.SequenceNumber ||
		restoredPacket.Header.Timestamp != originalHeader.Timestamp ||
		restoredPacket.Header.SSRC != originalHeader.SSRC ||
		restoredPacket.Header.Padding {
		t.Fatalf("restored header=%+v", restoredPacket.Header)
	}
	if !bytes.Equal(restoredPacket.Payload, originalPayload) {
		t.Fatalf("restored payload=%#v want %#v", restoredPacket.Payload, originalPayload)
	}

	override := config
	override.SequenceNumber = 8
	override.ExtensionOverride = true
	override.ExtensionProfile = rtp.RTPExtensionProfileTwoByte
	override.ExtensionPayload = []byte{3, 1, 0xcc, 0x00}
	overridden, err := rtp.AppendRTPRTXPacket(make([]byte, 0, rtxSize+8), original, override)
	if err != nil {
		t.Fatalf("AppendRTPRTXPacket override: %v", err)
	}
	parsedOverride, err := rtp.ParseRTPRTXPacket(overridden)
	if err != nil {
		t.Fatalf("ParseRTPRTXPacket override: %v", err)
	}
	if parsedOverride.Header.ExtensionProfile != override.ExtensionProfile ||
		!bytes.Equal(parsedOverride.Header.ExtensionPayload, override.ExtensionPayload) {
		t.Fatalf("override extension profile=%#x payload=%#v", parsedOverride.Header.ExtensionProfile, parsedOverride.Header.ExtensionPayload)
	}
}

func TestPublicRTPRTXPacketRejectsShortBuffersAndPayloads(t *testing.T) {
	original := testRetransmissionRTPPacket(t, 44, []byte("payload"))
	config := rtp.RTPRTXPacketConfig{PayloadType: 97, SequenceNumber: 10, SSRC: 0x22222222}
	size, err := rtp.RTPRTXPacketSize(original, config)
	if err != nil {
		t.Fatalf("RTPRTXPacketSize: %v", err)
	}
	if _, err := rtp.PutRTPRTXPacket(make([]byte, size-1), original, config); !errors.Is(err, rtp.ErrRTPShortBuffer) {
		t.Fatalf("short PutRTPRTXPacket err=%v want %v", err, rtp.ErrRTPShortBuffer)
	}
	if out, err := rtp.AppendRTPRTXPacket(make([]byte, 0, size-1), original, config); !errors.Is(err, rtp.ErrRTPShortBuffer) || len(out) != 0 {
		t.Fatalf("short AppendRTPRTXPacket out=%d err=%v", len(out), err)
	}
	if _, err := rtp.RTPRTXPacketSize([]byte{0x80}, config); !errors.Is(err, rtp.ErrRTPShortPayload) {
		t.Fatalf("short original RTP err=%v want %v", err, rtp.ErrRTPShortPayload)
	}

	shortRTX := testRetransmissionRTPPacket(t, 55, []byte{0xaa})
	if _, err := rtp.ParseRTPRTXPacket(shortRTX); !errors.Is(err, rtp.ErrRTPShortPayload) {
		t.Fatalf("short ParseRTPRTXPacket err=%v want %v", err, rtp.ErrRTPShortPayload)
	}
	rtx, err := rtp.AppendRTPRTXPacket(make([]byte, 0, size), original, config)
	if err != nil {
		t.Fatalf("AppendRTPRTXPacket valid: %v", err)
	}
	restoredSize, err := rtp.RTPPacketFromRTXSize(rtx, 96, 0x11111111)
	if err != nil {
		t.Fatalf("RTPPacketFromRTXSize: %v", err)
	}
	if _, err := rtp.PutRTPPacketFromRTX(make([]byte, restoredSize-1), rtx, 96, 0x11111111); !errors.Is(err, rtp.ErrRTPShortBuffer) {
		t.Fatalf("short PutRTPPacketFromRTX err=%v want %v", err, rtp.ErrRTPShortBuffer)
	}
	if out, err := rtp.AppendRTPPacketFromRTX(make([]byte, 0, restoredSize-1), rtx, 96, 0x11111111); !errors.Is(err, rtp.ErrRTPShortBuffer) || len(out) != 0 {
		t.Fatalf("short AppendRTPPacketFromRTX out=%d err=%v", len(out), err)
	}
}

func TestPublicRTPRetransmissionCacheAppendsRTXNACKPackets(t *testing.T) {
	cache := bindTestRetransmissionCache(t, 8, 96)
	for _, seq := range []uint16{10, 11, 13} {
		if err := cache.Store(testRetransmissionRTPPacket(t, seq, []byte{byte(seq), 0xee})); err != nil {
			t.Fatalf("Store seq %d: %v", seq, err)
		}
	}
	pairs := []rtp.RTCPGenericNACKPair{{PacketID: 10, LostPacketBitmask: 0x0007}}
	spans := make([]rtp.RTPRTXPacketSpan, 4)
	config := rtp.RTPRTXPacketConfig{PayloadType: 97, SequenceNumber: 0xfffe, SSRC: 0x22222222}
	out, count, next, err := cache.AppendRTXPacketsForRTCPGenericNACKPairs(make([]byte, 0, 512), spans, pairs, config)
	if err != nil {
		t.Fatalf("AppendRTXPacketsForRTCPGenericNACKPairs: %v", err)
	}
	if count != 3 || next.SequenceNumber != 1 {
		t.Fatalf("RTX count=%d nextSeq=%d want 3/1", count, next.SequenceNumber)
	}
	wantOriginal := []uint16{10, 11, 13}
	wantRTX := []uint16{0xfffe, 0xffff, 0}
	for i := 0; i < count; i++ {
		span := spans[i]
		if span.OriginalSequenceNumber != wantOriginal[i] || span.RTXSequenceNumber != wantRTX[i] {
			t.Fatalf("span %d=%+v want original=%d rtx=%d", i, span, wantOriginal[i], wantRTX[i])
		}
		rtx := out[span.Offset : span.Offset+span.Length]
		parsed, err := rtp.ParseRTPRTXPacket(rtx)
		if err != nil {
			t.Fatalf("ParseRTPRTXPacket span %d: %v", i, err)
		}
		if parsed.Header.PayloadType != config.PayloadType ||
			parsed.Header.SequenceNumber != wantRTX[i] ||
			parsed.Header.SSRC != config.SSRC ||
			parsed.OriginalSequenceNumber != wantOriginal[i] {
			t.Fatalf("parsed RTX span %d=%+v osn=%d", i, parsed.Header, parsed.OriginalSequenceNumber)
		}
		restored, err := rtp.AppendRTPPacketFromRTX(make([]byte, 0, 96), rtx, 96, 0xdec0de)
		if err != nil {
			t.Fatalf("AppendRTPPacketFromRTX span %d: %v", i, err)
		}
		restoredPacket, err := rtp.ParseRTPPacket(restored)
		if err != nil {
			t.Fatalf("ParseRTPPacket restored span %d: %v", i, err)
		}
		if restoredPacket.Header.SequenceNumber != wantOriginal[i] ||
			!bytes.Equal(restoredPacket.Payload, []byte{byte(wantOriginal[i]), 0xee}) {
			t.Fatalf("restored span %d header=%+v payload=%#v", i, restoredPacket.Header, restoredPacket.Payload)
		}
	}

	if _, _, _, err := cache.AppendRTXPacketsForRTCPGenericNACKPairs(out[:0], spans[:0], pairs, config); !errors.Is(err, rtp.ErrRTPShortBuffer) {
		t.Fatalf("short RTX spans err=%v want %v", err, rtp.ErrRTPShortBuffer)
	}
	var zero rtp.RTPRetransmissionCache
	if _, _, _, err := zero.AppendRTXPacketsForRTCPGenericNACKPairs(out[:0], spans, pairs, config); !errors.Is(err, rtp.ErrRTPInvalidRetransmissionCache) {
		t.Fatalf("zero RTX cache err=%v want %v", err, rtp.ErrRTPInvalidRetransmissionCache)
	}
}

func TestPublicRTPRetransmissionCacheRTXAllocs(t *testing.T) {
	cache := bindTestRetransmissionCache(t, 8, 96)
	for seq := uint16(20); seq < 24; seq++ {
		if err := cache.Store(testRetransmissionRTPPacket(t, seq, []byte{byte(seq)})); err != nil {
			t.Fatalf("Store seq %d: %v", seq, err)
		}
	}
	pairs := []rtp.RTCPGenericNACKPair{{PacketID: 20, LostPacketBitmask: 0x0007}}
	dst := make([]byte, 0, 512)
	spans := make([]rtp.RTPRTXPacketSpan, 4)
	config := rtp.RTPRTXPacketConfig{PayloadType: 97, SequenceNumber: 30, SSRC: 0x22222222}
	allocs := testing.AllocsPerRun(1000, func() {
		var err error
		var count int
		cfg := config
		dst, count, cfg, err = cache.AppendRTXPacketsForRTCPGenericNACKPairs(dst[:0], spans, pairs, cfg)
		if err != nil {
			t.Fatalf("AppendRTXPacketsForRTCPGenericNACKPairs: %v", err)
		}
		if count != 4 || cfg.SequenceNumber != config.SequenceNumber+4 {
			t.Fatalf("count=%d next=%d", count, cfg.SequenceNumber)
		}
	})
	if allocs != 0 {
		t.Fatalf("RTPRetransmissionCache RTX append allocs/run=%f want 0", allocs)
	}
}

func bindTestRetransmissionCache(t *testing.T, slots int, packetCap int) rtp.RTPRetransmissionCache {
	t.Helper()
	storage := make([]rtp.RTPRetransmissionCacheSlot, slots)
	for i := range storage {
		storage[i].Packet = make([]byte, 0, packetCap)
	}
	cache, err := rtp.BindRTPRetransmissionCache(storage)
	if err != nil {
		t.Fatalf("BindRTPRetransmissionCache: %v", err)
	}
	return cache
}

func testRetransmissionRTPPacket(t *testing.T, sequence uint16, payload []byte) []byte {
	t.Helper()
	header := rtp.RTPHeader{
		Marker:         true,
		PayloadType:    96,
		SequenceNumber: sequence,
		Timestamp:      90000 + uint32(sequence),
		SSRC:           0xdec0de,
	}
	size, err := rtp.RTPPacketSize(header, payload, 0)
	if err != nil {
		t.Fatalf("RTPPacketSize: %v", err)
	}
	packet := make([]byte, size)
	n, err := rtp.PutRTPPacket(packet, header, payload, 0)
	if err != nil {
		t.Fatalf("PutRTPPacket: %v", err)
	}
	return packet[:n]
}

func packetSequence(t *testing.T, packet []byte) uint16 {
	t.Helper()
	parsed, err := rtp.ParseRTPPacket(packet)
	if err != nil {
		t.Fatalf("ParseRTPPacket: %v", err)
	}
	return parsed.Header.SequenceNumber
}

func equalUint16s(a, b []uint16) bool {
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
