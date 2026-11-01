// Package rtp holds AV1 RTP/RTCP helpers and WebRTC packetization.
//
// Implementations are re-exported from the root goav1 package for now so
// decoder-only imports can use github.com/gtsteffaniak/go-av1 without RTP,
// while media clients import this package explicitly.
package rtp

import goav1 "github.com/gtsteffaniak/go-av1"

type (
	RTPDepacketizer     = goav1.RTPDepacketizer
	RTPPacketizer       = goav1.RTPPacketizer
	RTPPayloadLimits    = goav1.RTPPayloadSizeLimits
	RTPPayloadIterator  = goav1.RTPPayloadIterator
)

var (
	NewRTPPayloadIterator = goav1.NewRTPPayloadIterator
	NewRTPPacketizer      = goav1.NewRTPPacketizer
)
