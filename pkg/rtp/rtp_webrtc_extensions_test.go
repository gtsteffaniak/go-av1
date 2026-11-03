package rtp_test

import (
	"encoding/hex"
	"errors"
	rtp "github.com/gtsteffaniak/go-av1/pkg/rtp"
	"testing"
)

func TestPublicRTPWebRTCHeaderExtensionConstants(t *testing.T) {
	if rtp.RTPCoordinationOfVideoOrientationHeaderExtensionSize != 1 {
		t.Fatalf("RTPCoordinationOfVideoOrientationHeaderExtensionSize = %d, want 1", rtp.RTPCoordinationOfVideoOrientationHeaderExtensionSize)
	}
	if rtp.RTPPlayoutDelayHeaderExtensionSize != 3 {
		t.Fatalf("RTPPlayoutDelayHeaderExtensionSize = %d, want 3", rtp.RTPPlayoutDelayHeaderExtensionSize)
	}
	if rtp.RTPTransportWideCCHeaderExtensionSize != 2 {
		t.Fatalf("RTPTransportWideCCHeaderExtensionSize = %d, want 2", rtp.RTPTransportWideCCHeaderExtensionSize)
	}
	if rtp.RTPTransportWideCC02HeaderExtensionSizeWithoutFeedbackRequest != 2 {
		t.Fatalf("RTPTransportWideCC02HeaderExtensionSizeWithoutFeedbackRequest = %d, want 2", rtp.RTPTransportWideCC02HeaderExtensionSizeWithoutFeedbackRequest)
	}
	if rtp.RTPTransportWideCC02HeaderExtensionSize != 4 {
		t.Fatalf("RTPTransportWideCC02HeaderExtensionSize = %d, want 4", rtp.RTPTransportWideCC02HeaderExtensionSize)
	}
	if rtp.RTPAbsoluteSendTimeHeaderExtensionSize != 3 {
		t.Fatalf("RTPAbsoluteSendTimeHeaderExtensionSize = %d, want 3", rtp.RTPAbsoluteSendTimeHeaderExtensionSize)
	}
	if rtp.RTPAbsoluteCaptureTimeHeaderExtensionSizeWithoutClockOffset != 8 {
		t.Fatalf("RTPAbsoluteCaptureTimeHeaderExtensionSizeWithoutClockOffset = %d, want 8", rtp.RTPAbsoluteCaptureTimeHeaderExtensionSizeWithoutClockOffset)
	}
	if rtp.RTPAbsoluteCaptureTimeHeaderExtensionSize != 16 {
		t.Fatalf("RTPAbsoluteCaptureTimeHeaderExtensionSize = %d, want 16", rtp.RTPAbsoluteCaptureTimeHeaderExtensionSize)
	}
	if rtp.RTPVideoContentTypeHeaderExtensionSize != 1 {
		t.Fatalf("RTPVideoContentTypeHeaderExtensionSize = %d, want 1", rtp.RTPVideoContentTypeHeaderExtensionSize)
	}
	if rtp.RTPVideoTimingHeaderExtensionSize != 13 {
		t.Fatalf("RTPVideoTimingHeaderExtensionSize = %d, want 13", rtp.RTPVideoTimingHeaderExtensionSize)
	}
	if rtp.RTPVideoTimingHeaderExtensionSizeWithoutFlags != 12 {
		t.Fatalf("RTPVideoTimingHeaderExtensionSizeWithoutFlags = %d, want 12", rtp.RTPVideoTimingHeaderExtensionSizeWithoutFlags)
	}
	if rtp.RTPColorSpaceHeaderExtensionSizeWithoutHDRMetadata != 4 {
		t.Fatalf("RTPColorSpaceHeaderExtensionSizeWithoutHDRMetadata = %d, want 4", rtp.RTPColorSpaceHeaderExtensionSizeWithoutHDRMetadata)
	}
	if rtp.RTPColorSpaceHeaderExtensionSize != 28 {
		t.Fatalf("RTPColorSpaceHeaderExtensionSize = %d, want 28", rtp.RTPColorSpaceHeaderExtensionSize)
	}
	if rtp.RTPVideoLayersAllocationMaxHeaderExtensionSize != 407 {
		t.Fatalf("RTPVideoLayersAllocationMaxHeaderExtensionSize = %d, want 407", rtp.RTPVideoLayersAllocationMaxHeaderExtensionSize)
	}
	if rtp.RTPPlayoutDelayMaxMilliseconds != 40950 {
		t.Fatalf("RTPPlayoutDelayMaxMilliseconds = %d, want 40950", rtp.RTPPlayoutDelayMaxMilliseconds)
	}
	if rtp.RTPTransportWideCC02MaxFeedbackSequenceCount != 0x7fff {
		t.Fatalf("RTPTransportWideCC02MaxFeedbackSequenceCount = %#x, want 0x7fff", rtp.RTPTransportWideCC02MaxFeedbackSequenceCount)
	}
	if rtp.RTPAbsoluteSendTimeMaxValue != 0x00ffffff {
		t.Fatalf("RTPAbsoluteSendTimeMaxValue = %#x, want 0x00ffffff", rtp.RTPAbsoluteSendTimeMaxValue)
	}
	if rtp.RTPVideoContentTypeUnspecified != 0 ||
		rtp.RTPVideoContentTypeScreenshare != 1 {
		t.Fatalf("unexpected RTP video content type constants")
	}
	if rtp.RTPVideoTimingFlagTriggeredByTimer != 0x01 ||
		rtp.RTPVideoTimingFlagFrameLargerThanKnown != 0x02 {
		t.Fatalf("unexpected RTP video timing flags")
	}
	if rtp.RTPColorSpacePrimaryBT2020 != 9 ||
		rtp.RTPColorSpaceTransferBT202010 != 14 ||
		rtp.RTPColorSpaceMatrixBT2020NCL != 9 ||
		rtp.RTPColorSpaceRangeFull != 2 ||
		rtp.RTPColorSpaceChromaSitingHalf != 2 {
		t.Fatalf("unexpected RTP color-space constants")
	}
}

func TestPublicRTPCoordinationOfVideoOrientationHeaderExtension(t *testing.T) {
	orientation := rtp.RTPCoordinationOfVideoOrientation{
		Camera:   true,
		Flip:     true,
		Rotation: 270,
	}
	var buf [2]byte
	n, err := rtp.PutRTPCoordinationOfVideoOrientationHeaderExtension(buf[:], orientation)
	if err != nil {
		t.Fatalf("PutRTPCoordinationOfVideoOrientationHeaderExtension returned error: %v", err)
	}
	if n != rtp.RTPCoordinationOfVideoOrientationHeaderExtensionSize || buf[0] != 0x0f {
		t.Fatalf("encoded CVO n=%d buf=%#v", n, buf)
	}
	got, err := rtp.ParseRTPCoordinationOfVideoOrientationHeaderExtension(buf[:n])
	if err != nil {
		t.Fatalf("ParseRTPCoordinationOfVideoOrientationHeaderExtension returned error: %v", err)
	}
	if got != orientation {
		t.Fatalf("ParseRTPCoordinationOfVideoOrientationHeaderExtension = %+v, want %+v", got, orientation)
	}
	got, err = rtp.ParseRTPCoordinationOfVideoOrientationHeaderExtension([]byte{0xf1})
	if err != nil {
		t.Fatalf("ParseRTPCoordinationOfVideoOrientationHeaderExtension ignored reserved bits: %v", err)
	}
	if got != (rtp.RTPCoordinationOfVideoOrientation{Rotation: 90}) {
		t.Fatalf("reserved-bit CVO parse = %+v", got)
	}
	if err := rtp.ValidateRTPCoordinationOfVideoOrientation(rtp.RTPCoordinationOfVideoOrientation{Rotation: 45}); !errors.Is(err, rtp.ErrRTPInvalidHeaderExtension) {
		t.Fatalf("invalid CVO rotation error = %v, want ErrRTPInvalidHeaderExtension", err)
	}
	if _, err := rtp.ParseRTPCoordinationOfVideoOrientationHeaderExtension(nil); !errors.Is(err, rtp.ErrRTPShortBuffer) {
		t.Fatalf("short ParseRTPCoordinationOfVideoOrientationHeaderExtension error = %v, want ErrRTPShortBuffer", err)
	}
	if _, err := rtp.ParseRTPCoordinationOfVideoOrientationHeaderExtension(buf[:2]); !errors.Is(err, rtp.ErrRTPInvalidHeaderExtension) {
		t.Fatalf("long ParseRTPCoordinationOfVideoOrientationHeaderExtension error = %v, want ErrRTPInvalidHeaderExtension", err)
	}
	if _, err := rtp.PutRTPCoordinationOfVideoOrientationHeaderExtension(nil, orientation); !errors.Is(err, rtp.ErrRTPShortBuffer) {
		t.Fatalf("short PutRTPCoordinationOfVideoOrientationHeaderExtension error = %v, want ErrRTPShortBuffer", err)
	}
}

func TestPublicRTPPlayoutDelayHeaderExtension(t *testing.T) {
	delay := rtp.RTPPlayoutDelay{MinDelayMs: 120, MaxDelayMs: 3450}
	var buf [4]byte
	n, err := rtp.PutRTPPlayoutDelayHeaderExtension(buf[:], delay)
	if err != nil {
		t.Fatalf("PutRTPPlayoutDelayHeaderExtension returned error: %v", err)
	}
	if n != rtp.RTPPlayoutDelayHeaderExtensionSize ||
		buf[0] != 0x00 || buf[1] != 0xc1 || buf[2] != 0x59 {
		t.Fatalf("encoded playout-delay n=%d buf=%#v", n, buf)
	}
	got, err := rtp.ParseRTPPlayoutDelayHeaderExtension(buf[:n])
	if err != nil {
		t.Fatalf("ParseRTPPlayoutDelayHeaderExtension returned error: %v", err)
	}
	if got != delay {
		t.Fatalf("ParseRTPPlayoutDelayHeaderExtension = %+v, want %+v", got, delay)
	}
	for _, invalid := range []rtp.RTPPlayoutDelay{
		{MinDelayMs: -10, MaxDelayMs: 0},
		{MinDelayMs: 100, MaxDelayMs: 90},
		{MinDelayMs: 0, MaxDelayMs: rtp.RTPPlayoutDelayMaxMilliseconds + 10},
		{MinDelayMs: 5, MaxDelayMs: 10},
	} {
		if err := rtp.ValidateRTPPlayoutDelay(invalid); !errors.Is(err, rtp.ErrRTPInvalidHeaderExtension) {
			t.Fatalf("ValidateRTPPlayoutDelay(%+v) error = %v, want ErrRTPInvalidHeaderExtension", invalid, err)
		}
	}
	if _, err := rtp.ParseRTPPlayoutDelayHeaderExtension(buf[:2]); !errors.Is(err, rtp.ErrRTPShortBuffer) {
		t.Fatalf("short ParseRTPPlayoutDelayHeaderExtension error = %v, want ErrRTPShortBuffer", err)
	}
	if _, err := rtp.ParseRTPPlayoutDelayHeaderExtension(buf[:4]); !errors.Is(err, rtp.ErrRTPInvalidHeaderExtension) {
		t.Fatalf("long ParseRTPPlayoutDelayHeaderExtension error = %v, want ErrRTPInvalidHeaderExtension", err)
	}
	if _, err := rtp.PutRTPPlayoutDelayHeaderExtension(buf[:2], delay); !errors.Is(err, rtp.ErrRTPShortBuffer) {
		t.Fatalf("short PutRTPPlayoutDelayHeaderExtension error = %v, want ErrRTPShortBuffer", err)
	}
}

func TestPublicRTPTransportWideCCHeaderExtension(t *testing.T) {
	var buf [4]byte
	n, err := rtp.PutRTPTransportWideCCHeaderExtension(buf[:], 0x1234)
	if err != nil {
		t.Fatalf("PutRTPTransportWideCCHeaderExtension returned error: %v", err)
	}
	if n != rtp.RTPTransportWideCCHeaderExtensionSize || buf[0] != 0x12 || buf[1] != 0x34 {
		t.Fatalf("encoded transport-wide cc n=%d buf=%#v", n, buf)
	}
	got, err := rtp.ParseRTPTransportWideCCHeaderExtension(buf[:n])
	if err != nil {
		t.Fatalf("ParseRTPTransportWideCCHeaderExtension returned error: %v", err)
	}
	if got != 0x1234 {
		t.Fatalf("ParseRTPTransportWideCCHeaderExtension = %#x, want 0x1234", got)
	}
	if _, err := rtp.ParseRTPTransportWideCCHeaderExtension(buf[:1]); !errors.Is(err, rtp.ErrRTPShortBuffer) {
		t.Fatalf("short ParseRTPTransportWideCCHeaderExtension error = %v, want ErrRTPShortBuffer", err)
	}
	if _, err := rtp.ParseRTPTransportWideCCHeaderExtension(buf[:3]); !errors.Is(err, rtp.ErrRTPInvalidHeaderExtension) {
		t.Fatalf("long ParseRTPTransportWideCCHeaderExtension error = %v, want ErrRTPInvalidHeaderExtension", err)
	}
	if _, err := rtp.PutRTPTransportWideCCHeaderExtension(buf[:1], 1); !errors.Is(err, rtp.ErrRTPShortBuffer) {
		t.Fatalf("short PutRTPTransportWideCCHeaderExtension error = %v, want ErrRTPShortBuffer", err)
	}
}

func TestPublicRTPTransportWideCC02HeaderExtension(t *testing.T) {
	var buf [4]byte
	plain := rtp.RTPTransportWideCC02{SequenceNumber: 0x1234}
	size, err := rtp.RTPTransportWideCC02Size(plain)
	if err != nil {
		t.Fatalf("RTPTransportWideCC02Size plain returned error: %v", err)
	}
	if size != rtp.RTPTransportWideCC02HeaderExtensionSizeWithoutFeedbackRequest {
		t.Fatalf("plain TransportWideCC02 size=%d want %d", size, rtp.RTPTransportWideCC02HeaderExtensionSizeWithoutFeedbackRequest)
	}
	n, err := rtp.PutRTPTransportWideCC02HeaderExtension(buf[:], plain)
	if err != nil {
		t.Fatalf("PutRTPTransportWideCC02HeaderExtension plain returned error: %v", err)
	}
	if n != size || buf[0] != 0x12 || buf[1] != 0x34 {
		t.Fatalf("encoded plain transport-wide-cc-02 n=%d buf=%#v", n, buf)
	}
	got, err := rtp.ParseRTPTransportWideCC02HeaderExtension(buf[:n])
	if err != nil {
		t.Fatalf("ParseRTPTransportWideCC02HeaderExtension plain returned error: %v", err)
	}
	if got != plain {
		t.Fatalf("ParseRTPTransportWideCC02HeaderExtension plain = %+v, want %+v", got, plain)
	}

	request := rtp.RTPTransportWideCC02{
		SequenceNumber:        0xabcd,
		FeedbackRequest:       true,
		IncludeTimestamps:     true,
		FeedbackSequenceCount: 0x1234,
	}
	size, err = rtp.RTPTransportWideCC02Size(request)
	if err != nil {
		t.Fatalf("RTPTransportWideCC02Size request returned error: %v", err)
	}
	if size != rtp.RTPTransportWideCC02HeaderExtensionSize {
		t.Fatalf("request TransportWideCC02 size=%d want %d", size, rtp.RTPTransportWideCC02HeaderExtensionSize)
	}
	n, err = rtp.PutRTPTransportWideCC02HeaderExtension(buf[:], request)
	if err != nil {
		t.Fatalf("PutRTPTransportWideCC02HeaderExtension request returned error: %v", err)
	}
	want := []byte{0xab, 0xcd, 0x92, 0x34}
	if n != size || string(buf[:n]) != string(want) {
		t.Fatalf("encoded feedback transport-wide-cc-02 n=%d buf=%#v want %#v", n, buf[:n], want)
	}
	got, err = rtp.ParseRTPTransportWideCC02HeaderExtension(buf[:n])
	if err != nil {
		t.Fatalf("ParseRTPTransportWideCC02HeaderExtension request returned error: %v", err)
	}
	if got != request {
		t.Fatalf("ParseRTPTransportWideCC02HeaderExtension request = %+v, want %+v", got, request)
	}
	got, err = rtp.ParseRTPTransportWideCC02HeaderExtension([]byte{0xab, 0xcd, 0x80, 0x00})
	if err != nil {
		t.Fatalf("ParseRTPTransportWideCC02HeaderExtension zero count returned error: %v", err)
	}
	if got != (rtp.RTPTransportWideCC02{SequenceNumber: 0xabcd}) {
		t.Fatalf("zero-count transport-wide-cc-02 parse = %+v", got)
	}

	for _, invalid := range []rtp.RTPTransportWideCC02{
		{SequenceNumber: 1, FeedbackRequest: true},
		{SequenceNumber: 1, IncludeTimestamps: true},
		{SequenceNumber: 1, FeedbackSequenceCount: 1},
	} {
		if err := rtp.ValidateRTPTransportWideCC02(invalid); !errors.Is(err, rtp.ErrRTPInvalidHeaderExtension) {
			t.Fatalf("ValidateRTPTransportWideCC02(%+v) error = %v, want ErrRTPInvalidHeaderExtension", invalid, err)
		}
	}
	if _, err := rtp.ParseRTPTransportWideCC02HeaderExtension(buf[:1]); !errors.Is(err, rtp.ErrRTPShortBuffer) {
		t.Fatalf("short ParseRTPTransportWideCC02HeaderExtension error = %v, want ErrRTPShortBuffer", err)
	}
	if _, err := rtp.ParseRTPTransportWideCC02HeaderExtension(buf[:3]); !errors.Is(err, rtp.ErrRTPInvalidHeaderExtension) {
		t.Fatalf("bad-size ParseRTPTransportWideCC02HeaderExtension error = %v, want ErrRTPInvalidHeaderExtension", err)
	}
	if _, err := rtp.PutRTPTransportWideCC02HeaderExtension(buf[:1], plain); !errors.Is(err, rtp.ErrRTPShortBuffer) {
		t.Fatalf("short PutRTPTransportWideCC02HeaderExtension plain error = %v, want ErrRTPShortBuffer", err)
	}
	if _, err := rtp.PutRTPTransportWideCC02HeaderExtension(buf[:3], request); !errors.Is(err, rtp.ErrRTPShortBuffer) {
		t.Fatalf("short PutRTPTransportWideCC02HeaderExtension request error = %v, want ErrRTPShortBuffer", err)
	}
}

func TestPublicRTPAbsoluteSendTimeHeaderExtension(t *testing.T) {
	var buf [4]byte
	n, err := rtp.PutRTPAbsoluteSendTimeHeaderExtension(buf[:], 0xabcdef)
	if err != nil {
		t.Fatalf("PutRTPAbsoluteSendTimeHeaderExtension returned error: %v", err)
	}
	if n != rtp.RTPAbsoluteSendTimeHeaderExtensionSize ||
		buf[0] != 0xab || buf[1] != 0xcd || buf[2] != 0xef {
		t.Fatalf("encoded abs-send-time n=%d buf=%#v", n, buf)
	}
	got, err := rtp.ParseRTPAbsoluteSendTimeHeaderExtension(buf[:n])
	if err != nil {
		t.Fatalf("ParseRTPAbsoluteSendTimeHeaderExtension returned error: %v", err)
	}
	if got != 0xabcdef {
		t.Fatalf("ParseRTPAbsoluteSendTimeHeaderExtension = %#x, want 0xabcdef", got)
	}
	if _, err := rtp.ParseRTPAbsoluteSendTimeHeaderExtension(buf[:2]); !errors.Is(err, rtp.ErrRTPShortBuffer) {
		t.Fatalf("short ParseRTPAbsoluteSendTimeHeaderExtension error = %v, want ErrRTPShortBuffer", err)
	}
	if _, err := rtp.ParseRTPAbsoluteSendTimeHeaderExtension(buf[:4]); !errors.Is(err, rtp.ErrRTPInvalidHeaderExtension) {
		t.Fatalf("long ParseRTPAbsoluteSendTimeHeaderExtension error = %v, want ErrRTPInvalidHeaderExtension", err)
	}
	if _, err := rtp.PutRTPAbsoluteSendTimeHeaderExtension(buf[:2], 1); !errors.Is(err, rtp.ErrRTPShortBuffer) {
		t.Fatalf("short PutRTPAbsoluteSendTimeHeaderExtension error = %v, want ErrRTPShortBuffer", err)
	}
	if _, err := rtp.PutRTPAbsoluteSendTimeHeaderExtension(buf[:], 0x01000000); !errors.Is(err, rtp.ErrRTPInvalidHeaderExtension) {
		t.Fatalf("large PutRTPAbsoluteSendTimeHeaderExtension error = %v, want ErrRTPInvalidHeaderExtension", err)
	}
}

func TestPublicRTPAbsoluteCaptureTimeHeaderExtension(t *testing.T) {
	var buf [16]byte
	plain := rtp.RTPAbsoluteCaptureTime{AbsoluteCaptureTimestamp: 0x0102030405060708}
	size, err := rtp.RTPAbsoluteCaptureTimeSize(plain)
	if err != nil {
		t.Fatalf("RTPAbsoluteCaptureTimeSize plain returned error: %v", err)
	}
	if size != rtp.RTPAbsoluteCaptureTimeHeaderExtensionSizeWithoutClockOffset {
		t.Fatalf("plain abs-capture-time size=%d want %d", size, rtp.RTPAbsoluteCaptureTimeHeaderExtensionSizeWithoutClockOffset)
	}
	n, err := rtp.PutRTPAbsoluteCaptureTimeHeaderExtension(buf[:], plain)
	if err != nil {
		t.Fatalf("PutRTPAbsoluteCaptureTimeHeaderExtension plain returned error: %v", err)
	}
	wantPlain := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	if n != size || string(buf[:n]) != string(wantPlain) {
		t.Fatalf("encoded plain abs-capture-time n=%d buf=%#v want %#v", n, buf[:n], wantPlain)
	}
	got, err := rtp.ParseRTPAbsoluteCaptureTimeHeaderExtension(buf[:n])
	if err != nil {
		t.Fatalf("ParseRTPAbsoluteCaptureTimeHeaderExtension plain returned error: %v", err)
	}
	if got != plain {
		t.Fatalf("ParseRTPAbsoluteCaptureTimeHeaderExtension plain = %+v, want %+v", got, plain)
	}

	withOffset := rtp.RTPAbsoluteCaptureTime{
		AbsoluteCaptureTimestamp:           0x1112131415161718,
		EstimatedCaptureClockOffsetPresent: true,
		EstimatedCaptureClockOffset:        -2,
	}
	size, err = rtp.RTPAbsoluteCaptureTimeSize(withOffset)
	if err != nil {
		t.Fatalf("RTPAbsoluteCaptureTimeSize offset returned error: %v", err)
	}
	if size != rtp.RTPAbsoluteCaptureTimeHeaderExtensionSize {
		t.Fatalf("offset abs-capture-time size=%d want %d", size, rtp.RTPAbsoluteCaptureTimeHeaderExtensionSize)
	}
	n, err = rtp.PutRTPAbsoluteCaptureTimeHeaderExtension(buf[:], withOffset)
	if err != nil {
		t.Fatalf("PutRTPAbsoluteCaptureTimeHeaderExtension offset returned error: %v", err)
	}
	wantOffset := []byte{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xfe}
	if n != size || string(buf[:n]) != string(wantOffset) {
		t.Fatalf("encoded offset abs-capture-time n=%d buf=%#v want %#v", n, buf[:n], wantOffset)
	}
	got, err = rtp.ParseRTPAbsoluteCaptureTimeHeaderExtension(buf[:n])
	if err != nil {
		t.Fatalf("ParseRTPAbsoluteCaptureTimeHeaderExtension offset returned error: %v", err)
	}
	if got != withOffset {
		t.Fatalf("ParseRTPAbsoluteCaptureTimeHeaderExtension offset = %+v, want %+v", got, withOffset)
	}
	invalid := rtp.RTPAbsoluteCaptureTime{AbsoluteCaptureTimestamp: 1, EstimatedCaptureClockOffset: 1}
	if err := rtp.ValidateRTPAbsoluteCaptureTime(invalid); !errors.Is(err, rtp.ErrRTPInvalidHeaderExtension) {
		t.Fatalf("ValidateRTPAbsoluteCaptureTime invalid error = %v, want ErrRTPInvalidHeaderExtension", err)
	}
	if _, err := rtp.ParseRTPAbsoluteCaptureTimeHeaderExtension(buf[:7]); !errors.Is(err, rtp.ErrRTPShortBuffer) {
		t.Fatalf("short ParseRTPAbsoluteCaptureTimeHeaderExtension error = %v, want ErrRTPShortBuffer", err)
	}
	if _, err := rtp.ParseRTPAbsoluteCaptureTimeHeaderExtension(buf[:9]); !errors.Is(err, rtp.ErrRTPInvalidHeaderExtension) {
		t.Fatalf("bad-size ParseRTPAbsoluteCaptureTimeHeaderExtension error = %v, want ErrRTPInvalidHeaderExtension", err)
	}
	if _, err := rtp.PutRTPAbsoluteCaptureTimeHeaderExtension(buf[:7], plain); !errors.Is(err, rtp.ErrRTPShortBuffer) {
		t.Fatalf("short PutRTPAbsoluteCaptureTimeHeaderExtension plain error = %v, want ErrRTPShortBuffer", err)
	}
	if _, err := rtp.PutRTPAbsoluteCaptureTimeHeaderExtension(buf[:15], withOffset); !errors.Is(err, rtp.ErrRTPShortBuffer) {
		t.Fatalf("short PutRTPAbsoluteCaptureTimeHeaderExtension offset error = %v, want ErrRTPShortBuffer", err)
	}
}

func TestPublicRTPVideoContentTypeHeaderExtension(t *testing.T) {
	var buf [2]byte
	n, err := rtp.PutRTPVideoContentTypeHeaderExtension(buf[:], rtp.RTPVideoContentTypeScreenshare)
	if err != nil {
		t.Fatalf("PutRTPVideoContentTypeHeaderExtension returned error: %v", err)
	}
	if n != rtp.RTPVideoContentTypeHeaderExtensionSize || buf[0] != 0x01 {
		t.Fatalf("encoded video-content-type n=%d buf=%#v", n, buf)
	}
	got, err := rtp.ParseRTPVideoContentTypeHeaderExtension(buf[:n])
	if err != nil {
		t.Fatalf("ParseRTPVideoContentTypeHeaderExtension returned error: %v", err)
	}
	if got != rtp.RTPVideoContentTypeScreenshare {
		t.Fatalf("ParseRTPVideoContentTypeHeaderExtension = %d, want screenshare", got)
	}
	if err := rtp.ValidateRTPVideoContentType(rtp.RTPVideoContentType(2)); !errors.Is(err, rtp.ErrRTPInvalidHeaderExtension) {
		t.Fatalf("invalid ValidateRTPVideoContentType error = %v, want ErrRTPInvalidHeaderExtension", err)
	}
	if _, err := rtp.ParseRTPVideoContentTypeHeaderExtension(nil); !errors.Is(err, rtp.ErrRTPShortBuffer) {
		t.Fatalf("short ParseRTPVideoContentTypeHeaderExtension error = %v, want ErrRTPShortBuffer", err)
	}
	if _, err := rtp.ParseRTPVideoContentTypeHeaderExtension(buf[:2]); !errors.Is(err, rtp.ErrRTPInvalidHeaderExtension) {
		t.Fatalf("long ParseRTPVideoContentTypeHeaderExtension error = %v, want ErrRTPInvalidHeaderExtension", err)
	}
	if _, err := rtp.PutRTPVideoContentTypeHeaderExtension(nil, rtp.RTPVideoContentTypeScreenshare); !errors.Is(err, rtp.ErrRTPShortBuffer) {
		t.Fatalf("short PutRTPVideoContentTypeHeaderExtension error = %v, want ErrRTPShortBuffer", err)
	}
}

func TestPublicRTPVideoTimingHeaderExtension(t *testing.T) {
	timing := rtp.RTPVideoTiming{
		Flags:                        rtp.RTPVideoTimingFlagTriggeredByTimer | rtp.RTPVideoTimingFlagFrameLargerThanKnown,
		EncodeStartDeltaMs:           1,
		EncodeFinishDeltaMs:          2,
		PacketizationCompleteDeltaMs: 3,
		PacerExitDeltaMs:             4,
		NetworkTimestampDeltaMs:      5,
		NetworkTimestamp2DeltaMs:     6,
	}
	var buf [14]byte
	n, err := rtp.PutRTPVideoTimingHeaderExtension(buf[:], timing)
	if err != nil {
		t.Fatalf("PutRTPVideoTimingHeaderExtension returned error: %v", err)
	}
	want := []byte{0x03, 0, 1, 0, 2, 0, 3, 0, 4, 0, 5, 0, 6}
	if n != rtp.RTPVideoTimingHeaderExtensionSize || string(buf[:n]) != string(want) {
		t.Fatalf("encoded video-timing n=%d buf=%#v want %#v", n, buf[:n], want)
	}
	got, err := rtp.ParseRTPVideoTimingHeaderExtension(buf[:n])
	if err != nil {
		t.Fatalf("ParseRTPVideoTimingHeaderExtension returned error: %v", err)
	}
	if got != timing {
		t.Fatalf("ParseRTPVideoTimingHeaderExtension = %+v, want %+v", got, timing)
	}
	reserved, err := rtp.ParseRTPVideoTimingHeaderExtension(append([]byte{0xf3}, want[1:]...))
	if err != nil {
		t.Fatalf("ParseRTPVideoTimingHeaderExtension with reserved flags returned error: %v", err)
	}
	if reserved != timing {
		t.Fatalf("reserved-flag video timing parse = %+v, want %+v", reserved, timing)
	}
	legacy, err := rtp.ParseRTPVideoTimingHeaderExtension(want[1:])
	if err != nil {
		t.Fatalf("ParseRTPVideoTimingHeaderExtension legacy returned error: %v", err)
	}
	legacyWant := timing
	legacyWant.Flags = 0
	if legacy != legacyWant {
		t.Fatalf("legacy video timing parse = %+v, want %+v", legacy, legacyWant)
	}
	bad := timing
	bad.Flags = 0x80
	if err := rtp.ValidateRTPVideoTiming(bad); !errors.Is(err, rtp.ErrRTPInvalidHeaderExtension) {
		t.Fatalf("invalid ValidateRTPVideoTiming error = %v, want ErrRTPInvalidHeaderExtension", err)
	}
	if _, err := rtp.ParseRTPVideoTimingHeaderExtension(buf[:11]); !errors.Is(err, rtp.ErrRTPShortBuffer) {
		t.Fatalf("short ParseRTPVideoTimingHeaderExtension error = %v, want ErrRTPShortBuffer", err)
	}
	if _, err := rtp.ParseRTPVideoTimingHeaderExtension(buf[:14]); !errors.Is(err, rtp.ErrRTPInvalidHeaderExtension) {
		t.Fatalf("long ParseRTPVideoTimingHeaderExtension error = %v, want ErrRTPInvalidHeaderExtension", err)
	}
	if _, err := rtp.PutRTPVideoTimingHeaderExtension(buf[:12], timing); !errors.Is(err, rtp.ErrRTPShortBuffer) {
		t.Fatalf("short PutRTPVideoTimingHeaderExtension error = %v, want ErrRTPShortBuffer", err)
	}
}

func TestPublicRTPColorSpaceHeaderExtension(t *testing.T) {
	colorSpace := rtp.RTPColorSpace{
		Primaries:              rtp.RTPColorSpacePrimaryBT709,
		Transfer:               rtp.RTPColorSpaceTransferBT202010,
		Matrix:                 rtp.RTPColorSpaceMatrixBT2020NCL,
		Range:                  rtp.RTPColorSpaceRangeFull,
		ChromaSitingHorizontal: rtp.RTPColorSpaceChromaSitingHalf,
		ChromaSitingVertical:   rtp.RTPColorSpaceChromaSitingCollocated,
	}
	var buf [28]byte
	size, err := rtp.RTPColorSpaceSize(colorSpace)
	if err != nil {
		t.Fatalf("RTPColorSpaceSize plain returned error: %v", err)
	}
	if size != rtp.RTPColorSpaceHeaderExtensionSizeWithoutHDRMetadata {
		t.Fatalf("plain color-space size=%d want %d", size, rtp.RTPColorSpaceHeaderExtensionSizeWithoutHDRMetadata)
	}
	n, err := rtp.PutRTPColorSpaceHeaderExtension(buf[:], colorSpace)
	if err != nil {
		t.Fatalf("PutRTPColorSpaceHeaderExtension plain returned error: %v", err)
	}
	wantPlain := []byte{0x01, 0x0e, 0x09, 0x29}
	if n != size || string(buf[:n]) != string(wantPlain) {
		t.Fatalf("encoded plain color-space n=%d buf=%#v want %#v", n, buf[:n], wantPlain)
	}
	got, err := rtp.ParseRTPColorSpaceHeaderExtension(buf[:n])
	if err != nil {
		t.Fatalf("ParseRTPColorSpaceHeaderExtension plain returned error: %v", err)
	}
	if got != colorSpace {
		t.Fatalf("ParseRTPColorSpaceHeaderExtension plain = %+v, want %+v", got, colorSpace)
	}

	withHDR := colorSpace
	withHDR.HDRMetadataPresent = true
	withHDR.HDRMetadata = rtp.RTPColorSpaceHDRMetadata{
		LuminanceMax:              1000,
		LuminanceMin:              50,
		PrimaryR:                  rtp.RTPColorSpaceChromaticity{X: 34000, Y: 16000},
		PrimaryG:                  rtp.RTPColorSpaceChromaticity{X: 13250, Y: 34500},
		PrimaryB:                  rtp.RTPColorSpaceChromaticity{X: 7500, Y: 3000},
		WhitePoint:                rtp.RTPColorSpaceChromaticity{X: 15635, Y: 16450},
		MaxContentLightLevel:      1000,
		MaxFrameAverageLightLevel: 400,
	}
	size, err = rtp.RTPColorSpaceSize(withHDR)
	if err != nil {
		t.Fatalf("RTPColorSpaceSize HDR returned error: %v", err)
	}
	if size != rtp.RTPColorSpaceHeaderExtensionSize {
		t.Fatalf("HDR color-space size=%d want %d", size, rtp.RTPColorSpaceHeaderExtensionSize)
	}
	n, err = rtp.PutRTPColorSpaceHeaderExtension(buf[:], withHDR)
	if err != nil {
		t.Fatalf("PutRTPColorSpaceHeaderExtension HDR returned error: %v", err)
	}
	wantHDR := []byte{
		0x01, 0x0e, 0x09, 0x29,
		0x03, 0xe8, 0x00, 0x32,
		0x84, 0xd0, 0x3e, 0x80,
		0x33, 0xc2, 0x86, 0xc4,
		0x1d, 0x4c, 0x0b, 0xb8,
		0x3d, 0x13, 0x40, 0x42,
		0x03, 0xe8, 0x01, 0x90,
	}
	if n != size || string(buf[:n]) != string(wantHDR) {
		t.Fatalf("encoded HDR color-space n=%d buf=%#v want %#v", n, buf[:n], wantHDR)
	}
	got, err = rtp.ParseRTPColorSpaceHeaderExtension(buf[:n])
	if err != nil {
		t.Fatalf("ParseRTPColorSpaceHeaderExtension HDR returned error: %v", err)
	}
	if got != withHDR {
		t.Fatalf("ParseRTPColorSpaceHeaderExtension HDR = %+v, want %+v", got, withHDR)
	}

	invalid := colorSpace
	invalid.Primaries = rtp.RTPColorSpacePrimary(3)
	if err := rtp.ValidateRTPColorSpace(invalid); !errors.Is(err, rtp.ErrRTPInvalidHeaderExtension) {
		t.Fatalf("ValidateRTPColorSpace invalid primary error = %v, want ErrRTPInvalidHeaderExtension", err)
	}
	invalid = colorSpace
	invalid.HDRMetadata.LuminanceMax = 1
	if err := rtp.ValidateRTPColorSpace(invalid); !errors.Is(err, rtp.ErrRTPInvalidHeaderExtension) {
		t.Fatalf("ValidateRTPColorSpace dropped HDR error = %v, want ErrRTPInvalidHeaderExtension", err)
	}
	badHDR := withHDR.HDRMetadata
	badHDR.LuminanceMax = 20001
	if err := rtp.ValidateRTPColorSpaceHDRMetadata(badHDR); !errors.Is(err, rtp.ErrRTPInvalidHeaderExtension) {
		t.Fatalf("ValidateRTPColorSpaceHDRMetadata invalid error = %v, want ErrRTPInvalidHeaderExtension", err)
	}
	if _, err := rtp.ParseRTPColorSpaceHeaderExtension(buf[:3]); !errors.Is(err, rtp.ErrRTPShortBuffer) {
		t.Fatalf("short ParseRTPColorSpaceHeaderExtension error = %v, want ErrRTPShortBuffer", err)
	}
	if _, err := rtp.ParseRTPColorSpaceHeaderExtension(buf[:5]); !errors.Is(err, rtp.ErrRTPInvalidHeaderExtension) {
		t.Fatalf("bad-size ParseRTPColorSpaceHeaderExtension error = %v, want ErrRTPInvalidHeaderExtension", err)
	}
	if _, err := rtp.ParseRTPColorSpaceHeaderExtension([]byte{0x03, 0x0e, 0x09, 0x29}); !errors.Is(err, rtp.ErrRTPInvalidHeaderExtension) {
		t.Fatalf("invalid-value ParseRTPColorSpaceHeaderExtension error = %v, want ErrRTPInvalidHeaderExtension", err)
	}
	if _, err := rtp.PutRTPColorSpaceHeaderExtension(buf[:3], colorSpace); !errors.Is(err, rtp.ErrRTPShortBuffer) {
		t.Fatalf("short PutRTPColorSpaceHeaderExtension plain error = %v, want ErrRTPShortBuffer", err)
	}
	if _, err := rtp.PutRTPColorSpaceHeaderExtension(buf[:27], withHDR); !errors.Is(err, rtp.ErrRTPShortBuffer) {
		t.Fatalf("short PutRTPColorSpaceHeaderExtension HDR error = %v, want ErrRTPShortBuffer", err)
	}
}

func TestPublicRTPVideoLayersAllocationHeaderExtension(t *testing.T) {
	cases := []struct {
		name       string
		allocation rtp.RTPVideoLayersAllocation
		hex        string
	}{
		{
			name: "three streams",
			allocation: rtp.RTPVideoLayersAllocation{
				RTPStreamID:    0,
				RTPStreamCount: 3,
				ActiveSpatialLayers: []rtp.RTPVideoLayersAllocationLayer{
					{RTPStreamID: 0, SpatialID: 0, TargetBitratesKbps: []uint32{150}},
					{RTPStreamID: 1, SpatialID: 0, TargetBitratesKbps: []uint32{240, 400}},
					{RTPStreamID: 2, SpatialID: 0, TargetBitratesKbps: []uint32{720, 1200}},
				},
			},
			hex: "21149601f0019003d005b009",
		},
		{
			name: "three streams resolution",
			allocation: rtp.RTPVideoLayersAllocation{
				RTPStreamID:    2,
				RTPStreamCount: 3,
				ActiveSpatialLayers: []rtp.RTPVideoLayersAllocationLayer{
					{RTPStreamID: 0, SpatialID: 0, TargetBitratesKbps: []uint32{150}, Width: 320, Height: 180, Framerate: 30},
					{RTPStreamID: 1, SpatialID: 0, TargetBitratesKbps: []uint32{240, 400}, Width: 640, Height: 360, Framerate: 30},
					{RTPStreamID: 2, SpatialID: 0, TargetBitratesKbps: []uint32{720, 1200}, Width: 1280, Height: 720, Framerate: 30},
				},
				HasResolutionAndFramerate: true,
			},
			hex: "a1149601f0019003d005b009013f00b31e027f01671e04ff02cf1e",
		},
		{
			name: "paused middle stream",
			allocation: rtp.RTPVideoLayersAllocation{
				RTPStreamID:    1,
				RTPStreamCount: 3,
				ActiveSpatialLayers: []rtp.RTPVideoLayersAllocationLayer{
					{RTPStreamID: 0, SpatialID: 0, TargetBitratesKbps: []uint32{150}, Width: 320, Height: 180, Framerate: 30},
					{RTPStreamID: 2, SpatialID: 0, TargetBitratesKbps: []uint32{720, 1200}, Width: 1280, Height: 720, Framerate: 30},
				},
				HasResolutionAndFramerate: true,
			},
			hex: "601010109601d005b009013f00b31e04ff02cf1e",
		},
		{
			name: "multiple spatial layers",
			allocation: rtp.RTPVideoLayersAllocation{
				RTPStreamID:    0,
				RTPStreamCount: 1,
				ActiveSpatialLayers: []rtp.RTPVideoLayersAllocationLayer{
					{RTPStreamID: 0, SpatialID: 0, TargetBitratesKbps: []uint32{90}},
					{RTPStreamID: 0, SpatialID: 1, TargetBitratesKbps: []uint32{180, 260}},
					{RTPStreamID: 0, SpatialID: 2, TargetBitratesKbps: []uint32{360, 520, 700}},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			size, err := rtp.RTPVideoLayersAllocationSize(tc.allocation)
			if err != nil {
				t.Fatalf("RTPVideoLayersAllocationSize returned error: %v", err)
			}
			buf := make([]byte, size)
			n, err := rtp.PutRTPVideoLayersAllocationHeaderExtension(buf, tc.allocation)
			if err != nil {
				t.Fatalf("PutRTPVideoLayersAllocationHeaderExtension returned error: %v", err)
			}
			if n != size {
				t.Fatalf("PutRTPVideoLayersAllocationHeaderExtension n=%d want %d", n, size)
			}
			if tc.hex != "" {
				want, err := hex.DecodeString(tc.hex)
				if err != nil {
					t.Fatalf("DecodeString(%q): %v", tc.hex, err)
				}
				if string(buf) != string(want) {
					t.Fatalf("encoded allocation = %x, want %x", buf, want)
				}
			}
			got, err := rtp.ParseRTPVideoLayersAllocationHeaderExtension(buf)
			if err != nil {
				t.Fatalf("ParseRTPVideoLayersAllocationHeaderExtension returned error: %v", err)
			}
			if !sameRTPVideoLayersAllocation(got, tc.allocation) {
				t.Fatalf("ParseRTPVideoLayersAllocationHeaderExtension = %+v, want %+v", got, tc.allocation)
			}
		})
	}
}

func TestPublicRTPVideoLayersAllocationHeaderExtensionErrors(t *testing.T) {
	empty := rtp.RTPVideoLayersAllocation{}
	size, err := rtp.RTPVideoLayersAllocationSize(empty)
	if err != nil {
		t.Fatalf("RTPVideoLayersAllocationSize(empty) returned error: %v", err)
	}
	if size != 1 {
		t.Fatalf("RTPVideoLayersAllocationSize(empty)=%d want 1", size)
	}
	var one [1]byte
	n, err := rtp.PutRTPVideoLayersAllocationHeaderExtension(one[:], empty)
	if err != nil {
		t.Fatalf("PutRTPVideoLayersAllocationHeaderExtension(empty) returned error: %v", err)
	}
	if n != 1 || one[0] != 0 {
		t.Fatalf("empty allocation n=%d byte=%#x, want 1/0", n, one[0])
	}
	got, err := rtp.ParseRTPVideoLayersAllocationHeaderExtension(one[:])
	if err != nil {
		t.Fatalf("ParseRTPVideoLayersAllocationHeaderExtension(empty) returned error: %v", err)
	}
	if got.RTPStreamID != 0 || got.RTPStreamCount != 1 || len(got.ActiveSpatialLayers) != 0 {
		t.Fatalf("empty allocation parse = %+v", got)
	}
	negativeEmpty := rtp.RTPVideoLayersAllocation{RTPStreamCount: -1}
	if err := rtp.ValidateRTPVideoLayersAllocation(negativeEmpty); !errors.Is(err, rtp.ErrRTPInvalidHeaderExtension) {
		t.Fatalf("ValidateRTPVideoLayersAllocation negative empty error = %v, want ErrRTPInvalidHeaderExtension", err)
	}

	allocation := rtp.RTPVideoLayersAllocation{
		RTPStreamID:    0,
		RTPStreamCount: 1,
		ActiveSpatialLayers: []rtp.RTPVideoLayersAllocationLayer{
			{RTPStreamID: 0, SpatialID: 0, TargetBitratesKbps: []uint32{150}},
		},
	}
	if _, err := rtp.PutRTPVideoLayersAllocationHeaderExtension(nil, allocation); !errors.Is(err, rtp.ErrRTPShortBuffer) {
		t.Fatalf("short PutRTPVideoLayersAllocationHeaderExtension error = %v, want ErrRTPShortBuffer", err)
	}
	if _, err := rtp.ParseRTPVideoLayersAllocationHeaderExtension(nil); !errors.Is(err, rtp.ErrRTPShortBuffer) {
		t.Fatalf("short ParseRTPVideoLayersAllocationHeaderExtension error = %v, want ErrRTPShortBuffer", err)
	}
	if _, err := rtp.ParseRTPVideoLayersAllocationHeaderExtension([]byte{0x40}); !errors.Is(err, rtp.ErrRTPInvalidHeaderExtension) {
		t.Fatalf("bad RID ParseRTPVideoLayersAllocationHeaderExtension error = %v, want ErrRTPInvalidHeaderExtension", err)
	}
	if _, err := rtp.ParseRTPVideoLayersAllocationHeaderExtension([]byte{0x00, 0x00}); !errors.Is(err, rtp.ErrRTPInvalidHeaderExtension) {
		t.Fatalf("no-layer ParseRTPVideoLayersAllocationHeaderExtension error = %v, want ErrRTPInvalidHeaderExtension", err)
	}
	if _, err := rtp.ParseRTPVideoLayersAllocationHeaderExtension([]byte{0x01, 0x00, 0x80}); !errors.Is(err, rtp.ErrRTPShortBuffer) {
		t.Fatalf("truncated LEB ParseRTPVideoLayersAllocationHeaderExtension error = %v, want ErrRTPShortBuffer", err)
	}
	if _, err := rtp.ParseRTPVideoLayersAllocationHeaderExtension([]byte{0x01, 0x00, 0x96, 0x01, 0x00}); !errors.Is(err, rtp.ErrRTPShortBuffer) {
		t.Fatalf("short resolution ParseRTPVideoLayersAllocationHeaderExtension error = %v, want ErrRTPShortBuffer", err)
	}
	if _, err := rtp.ParseRTPVideoLayersAllocationHeaderExtension([]byte{0x01, 0x00, 0x96, 0x01, 0, 0, 0, 0, 0, 0}); !errors.Is(err, rtp.ErrRTPInvalidHeaderExtension) {
		t.Fatalf("extra resolution ParseRTPVideoLayersAllocationHeaderExtension error = %v, want ErrRTPInvalidHeaderExtension", err)
	}

	duplicate := allocation
	duplicate.ActiveSpatialLayers = append(duplicate.ActiveSpatialLayers, duplicate.ActiveSpatialLayers[0])
	if err := rtp.ValidateRTPVideoLayersAllocation(duplicate); !errors.Is(err, rtp.ErrRTPInvalidHeaderExtension) {
		t.Fatalf("ValidateRTPVideoLayersAllocation duplicate error = %v, want ErrRTPInvalidHeaderExtension", err)
	}
	staleResolution := allocation
	staleResolution.ActiveSpatialLayers[0].Width = 320
	if err := rtp.ValidateRTPVideoLayersAllocation(staleResolution); !errors.Is(err, rtp.ErrRTPInvalidHeaderExtension) {
		t.Fatalf("ValidateRTPVideoLayersAllocation stale resolution error = %v, want ErrRTPInvalidHeaderExtension", err)
	}
	badResolution := allocation
	badResolution.HasResolutionAndFramerate = true
	badResolution.ActiveSpatialLayers[0].Width = 1 << 16
	badResolution.ActiveSpatialLayers[0].Height = 720
	badResolution.ActiveSpatialLayers[0].Framerate = 30
	if err := rtp.ValidateRTPVideoLayersAllocation(badResolution); err != nil {
		t.Fatalf("ValidateRTPVideoLayersAllocation max resolution returned error: %v", err)
	}
	badResolution.ActiveSpatialLayers[0].Width = 1<<16 + 1
	if err := rtp.ValidateRTPVideoLayersAllocation(badResolution); !errors.Is(err, rtp.ErrRTPInvalidHeaderExtension) {
		t.Fatalf("ValidateRTPVideoLayersAllocation invalid resolution error = %v, want ErrRTPInvalidHeaderExtension", err)
	}
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
