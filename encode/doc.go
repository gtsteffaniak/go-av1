// Package encode holds the AV1 encoder and WebRTC encoder public API.
//
// The implementation still lives in the root goav1 package while the encoder
// split is staged. Import github.com/gtsteffaniak/go-av1/encode for new code
// that must not link decoder-only binaries against encoder symbols; this package
// re-exports the root API without adding a container→encoder dependency.
package encode

import goav1 "github.com/gtsteffaniak/go-av1"

type (
	VideoEncoder       = goav1.VideoEncoder
	VideoEncoderConfig = goav1.VideoEncoderConfig
	EncoderConfig      = goav1.EncoderConfig
	RTCEncoder         = goav1.RTCEncoder
	RTCFrame           = goav1.RTCFrame
)

var (
	NewVideoEncoder         = goav1.NewVideoEncoder
	NewRTCEncoderWithConfig = goav1.NewRTCEncoderWithConfig
)
