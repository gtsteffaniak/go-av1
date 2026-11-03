package encode

// This file holds the RTCP feedback helpers that bridge into the encoder
// configuration (REMB bitrate application and AV1 layer-refresh-request
// validation/targeting). They depend on the encoder config surface, so they
// live with the encoder rather than with the packet-syntax RTCP helpers.

import (
	"encoding/binary"

	rtp "github.com/gtsteffaniak/go-av1/pkg/rtp"
)

// EncoderWebRTCApplyReceiverEstimatedMaximumBitrate applies one REMB bitrate
// estimate to config's WebRTC target bitrate fields and returns a normalized
// config suitable for RTCEncoder.SetConfig or WebRTCEncoder.SetConfig. The
// estimate is rounded up to kbps, clamped by configured min/max bitrate limits,
// and distributed across active spatial layers in proportion to their current
// targets. Rate-control mode, framerate, scalability, and keyframe policy are
// left unchanged.
func EncoderWebRTCApplyReceiverEstimatedMaximumBitrate(
	config EncoderConfig,
	remb rtp.RTCPReceiverEstimatedMaximumBitrate,
) (EncoderConfig, error) {
	normalized, err := SetWebRTCEncoderSVCConfig(config, config.TemporalLayerCount, config.SpatialLayerCount)
	if err != nil {
		return EncoderConfig{}, err
	}
	targetKbps := encoderWebRTCReceiverEstimatedMaximumBitrateTargetKbps(remb.BitrateBps)
	targetKbps = encoderWebRTCClampReceiverEstimatedMaximumBitrateKbps(
		targetKbps,
		normalized.MinBitrateKbps,
		normalized.MaxBitrateKbps,
	)
	normalized.TargetBitrateKbps = targetKbps
	if normalized.MaxBitrateKbps == 0 {
		normalized.MaxBitrateKbps = targetKbps
	}
	encoderWebRTCApplyReceiverEstimatedMaximumBitrateSpatialLayers(&normalized, targetKbps)
	return SetWebRTCEncoderSVCConfig(normalized, normalized.TemporalLayerCount, normalized.SpatialLayerCount)
}

// EncoderWebRTCRTCPFeedbackApplyReceiverEstimatedMaximumBitrate parses one RTCP
// feedback packet and applies REMB feedback to config when present. ok is false
// for non-REMB feedback, and the returned config then aliases the input config.
func EncoderWebRTCRTCPFeedbackApplyReceiverEstimatedMaximumBitrate(
	config EncoderConfig,
	packet rtp.RTCPFeedbackPacket,
	ssrcScratch []uint32,
) (EncoderConfig, bool, error) {
	remb, ok, err := rtp.EncoderWebRTCRTCPFeedbackReceiverEstimatedMaximumBitrate(packet, ssrcScratch)
	if err != nil || !ok {
		return config, ok, err
	}
	updated, err := EncoderWebRTCApplyReceiverEstimatedMaximumBitrate(config, remb)
	if err != nil {
		return EncoderConfig{}, true, err
	}
	return updated, true, nil
}

// EncoderWebRTCRTCPPacketsApplyReceiverEstimatedMaximumBitrate scans parsed
// generic RTCP packets and applies the last REMB estimate in packet order.
func EncoderWebRTCRTCPPacketsApplyReceiverEstimatedMaximumBitrate(
	config EncoderConfig,
	packets []rtp.RTCPPacket,
	ssrcScratch []uint32,
) (EncoderConfig, bool, error) {
	remb, ok, err := rtp.EncoderWebRTCRTCPPacketsReceiverEstimatedMaximumBitrate(packets, ssrcScratch)
	if err != nil || !ok {
		return config, ok, err
	}
	updated, err := EncoderWebRTCApplyReceiverEstimatedMaximumBitrate(config, remb)
	if err != nil {
		return EncoderConfig{}, true, err
	}
	return updated, true, nil
}

// EncoderWebRTCRTCPCompoundPacketsApplyReceiverEstimatedMaximumBitrate parses a
// raw compound RTCP packet stream and applies the last REMB estimate in packet
// order. Parsed packets are appended into packetScratch without allocation and
// returned as the newly parsed suffix.
func EncoderWebRTCRTCPCompoundPacketsApplyReceiverEstimatedMaximumBitrate(
	config EncoderConfig,
	compound []byte,
	packetScratch []rtp.RTCPPacket,
	ssrcScratch []uint32,
) (EncoderConfig, bool, []rtp.RTCPPacket, error) {
	remb, ok, packets, err := rtp.EncoderWebRTCRTCPCompoundPacketsReceiverEstimatedMaximumBitrate(
		compound,
		packetScratch,
		ssrcScratch,
	)
	if err != nil || !ok {
		return config, ok, packets, err
	}
	updated, err := EncoderWebRTCApplyReceiverEstimatedMaximumBitrate(config, remb)
	if err != nil {
		return EncoderConfig{}, true, packets, err
	}
	return updated, true, packets, nil
}

func encoderWebRTCReceiverEstimatedMaximumBitrateTargetKbps(bitrateBps uint64) int32 {
	maxBps := uint64(EncoderWebRTCMaxBitrateKbps) * 1000
	if bitrateBps >= maxBps {
		return EncoderWebRTCMaxBitrateKbps
	}
	if bitrateBps == 0 {
		return 1
	}
	return int32((bitrateBps + 999) / 1000)
}

func encoderWebRTCClampReceiverEstimatedMaximumBitrateKbps(targetKbps int32, minKbps int32, maxKbps int32) int32 {
	if targetKbps < 1 {
		targetKbps = 1
	}
	if maxKbps > 0 && targetKbps > maxKbps {
		targetKbps = maxKbps
	}
	if minKbps > 0 && targetKbps < minKbps {
		targetKbps = minKbps
	}
	return targetKbps
}

func encoderWebRTCApplyReceiverEstimatedMaximumBitrateSpatialLayers(config *EncoderConfig, targetKbps int32) {
	layerCount := int(config.SpatialLayerCount)
	if layerCount <= 0 || layerCount > EncoderMaxSpatialLayers {
		return
	}

	lastActive := -1
	var targetSum int64
	var weightSum int64
	for i := 0; i < layerCount; i++ {
		layer := &config.SpatialLayers[i]
		if !layer.Active {
			continue
		}
		lastActive = i
		if layer.TargetBitrateKbps > 0 {
			targetSum += int64(layer.TargetBitrateKbps)
		}
		weightSum += int64(i + 1)
	}
	if lastActive < 0 {
		return
	}

	var assignedKbps int64
	for i := 0; i < layerCount; i++ {
		layer := &config.SpatialLayers[i]
		if !layer.Active {
			continue
		}

		layerTarget := targetKbps
		if layerCount > 1 {
			if i == lastActive {
				layerTarget = int32(int64(targetKbps) - assignedKbps)
			} else if targetSum > 0 && layer.TargetBitrateKbps > 0 {
				layerTarget = int32((int64(targetKbps) * int64(layer.TargetBitrateKbps)) / targetSum)
				assignedKbps += int64(layerTarget)
			} else if weightSum > 0 {
				weight := int64(i + 1)
				layerTarget = int32((int64(targetKbps) * weight) / weightSum)
				assignedKbps += int64(layerTarget)
			}
			if layerTarget < 1 && i != lastActive {
				assignedKbps += int64(1 - layerTarget)
				layerTarget = 1
			}
		}

		layerTarget = encoderWebRTCClampReceiverEstimatedMaximumBitrateKbps(
			layerTarget,
			layer.MinBitrateKbps,
			layer.MaxBitrateKbps,
		)
		layer.TargetBitrateKbps = layerTarget
		if layer.MaxBitrateKbps == 0 {
			layer.MaxBitrateKbps = layerTarget
		}
	}
}

// EncoderWebRTCValidateLayerRefreshRequest validates that entry's target and
// current AV1 LRR layer indices fit config's WebRTC scalability grid.
func EncoderWebRTCValidateLayerRefreshRequest(config EncoderConfig, entry rtp.AV1RTCPLayerRefreshRequestEntry) error {
	normalized, err := SetWebRTCEncoderSVCConfig(config, config.TemporalLayerCount, config.SpatialLayerCount)
	if err != nil {
		return err
	}
	return encoderWebRTCValidateLayerRefreshRequestForConfig(normalized, entry)
}

// EncoderWebRTCValidateLayerRefreshRequests validates that every entry's
// target and current AV1 LRR layer indices fit config's WebRTC scalability grid.
func EncoderWebRTCValidateLayerRefreshRequests(config EncoderConfig, entries []rtp.AV1RTCPLayerRefreshRequestEntry) error {
	normalized, err := SetWebRTCEncoderSVCConfig(config, config.TemporalLayerCount, config.SpatialLayerCount)
	if err != nil {
		return err
	}
	for i := range entries {
		if err := encoderWebRTCValidateLayerRefreshRequestForConfig(normalized, entries[i]); err != nil {
			return err
		}
	}
	return nil
}

// EncoderWebRTCLayerRefreshRequestTarget validates entries against config and
// returns the inclusive spatial/temporal cap that covers all requested LRR
// target layers. The returned cap can be passed to
// RTCPicture.ActiveDecodeTargetsRTPOptions after the forced refresh picture is
// encoded. ok is false when entries is empty.
func EncoderWebRTCLayerRefreshRequestTarget(
	config EncoderConfig,
	entries []rtp.AV1RTCPLayerRefreshRequestEntry,
) (target rtp.AV1RTCPLayerRefreshLayerIndex, ok bool, err error) {
	normalized, err := SetWebRTCEncoderSVCConfig(config, config.TemporalLayerCount, config.SpatialLayerCount)
	if err != nil {
		return rtp.AV1RTCPLayerRefreshLayerIndex{}, false, err
	}
	for i := range entries {
		if err := encoderWebRTCValidateLayerRefreshRequestForConfig(normalized, entries[i]); err != nil {
			return rtp.AV1RTCPLayerRefreshLayerIndex{}, false, err
		}
		if !ok || entries[i].Target.SpatialID > target.SpatialID {
			target.SpatialID = entries[i].Target.SpatialID
		}
		if !ok || entries[i].Target.TemporalID > target.TemporalID {
			target.TemporalID = entries[i].Target.TemporalID
		}
		ok = true
	}
	return target, ok, nil
}

// EncoderWebRTCRTCPFeedbackLayerRefreshTarget parses a single RTCP feedback
// packet and returns the inclusive active decode-target cap requested by valid
// AV1 LRR feedback. Non-LRR RTPFB/PSFB feedback returns ok=false.
func EncoderWebRTCRTCPFeedbackLayerRefreshTarget(
	config EncoderConfig,
	packet rtp.RTCPFeedbackPacket,
	lrrScratch []rtp.AV1RTCPLayerRefreshRequestEntry,
) (target rtp.AV1RTCPLayerRefreshLayerIndex, ok bool, err error) {
	switch packet.PacketType {
	case rtp.RTCPRTPFBPacketType:
		return rtp.AV1RTCPLayerRefreshLayerIndex{}, false, nil
	case rtp.RTCPPSFBPacketType:
	default:
		return rtp.AV1RTCPLayerRefreshLayerIndex{}, false, rtp.ErrRTCPInvalidFeedback
	}
	if packet.FMT != rtp.RTCPPSFBLayerRefreshRequestFMT {
		return rtp.AV1RTCPLayerRefreshLayerIndex{}, false, nil
	}
	start := len(lrrScratch)
	entries, err := rtp.ParseAV1RTCPLayerRefreshRequestEntries(packet.FCI, lrrScratch)
	if err != nil {
		return rtp.AV1RTCPLayerRefreshLayerIndex{}, false, err
	}
	return EncoderWebRTCLayerRefreshRequestTarget(config, entries[start:])
}

// EncoderWebRTCRTCPPacketsLayerRefreshTarget scans parsed generic RTCP packets
// and returns the inclusive active decode-target cap requested by AV1 LRR
// feedback. Non-feedback RTCP packets are ignored.
func EncoderWebRTCRTCPPacketsLayerRefreshTarget(
	config EncoderConfig,
	packets []rtp.RTCPPacket,
	lrrScratch []rtp.AV1RTCPLayerRefreshRequestEntry,
) (target rtp.AV1RTCPLayerRefreshLayerIndex, ok bool, err error) {
	for i := range packets {
		packet := packets[i]
		if packet.PacketType != rtp.RTCPRTPFBPacketType && packet.PacketType != rtp.RTCPPSFBPacketType {
			continue
		}
		if len(packet.Payload) < rtp.RTCPFeedbackCommonSize {
			return rtp.AV1RTCPLayerRefreshLayerIndex{}, false, rtp.ErrRTCPInvalidFeedback
		}
		current, currentOK, err := EncoderWebRTCRTCPFeedbackLayerRefreshTarget(config, rtp.RTCPFeedbackPacket{
			PacketType: packet.PacketType,
			FMT:        packet.Count,
			SenderSSRC: binary.BigEndian.Uint32(packet.Payload[0:4]),
			MediaSSRC:  binary.BigEndian.Uint32(packet.Payload[4:8]),
			FCI:        packet.Payload[rtp.RTCPFeedbackCommonSize:],
		}, lrrScratch)
		if err != nil {
			return rtp.AV1RTCPLayerRefreshLayerIndex{}, false, err
		}
		if !currentOK {
			continue
		}
		if !ok || current.SpatialID > target.SpatialID {
			target.SpatialID = current.SpatialID
		}
		if !ok || current.TemporalID > target.TemporalID {
			target.TemporalID = current.TemporalID
		}
		ok = true
	}
	return target, ok, nil
}

// EncoderWebRTCRTCPCompoundPacketsLayerRefreshTarget parses a raw compound RTCP
// packet stream and returns the inclusive active decode-target cap requested by
// AV1 LRR feedback. Parsed packets are appended into packetScratch without
// allocation and returned as the newly parsed suffix.
func EncoderWebRTCRTCPCompoundPacketsLayerRefreshTarget(
	config EncoderConfig,
	compound []byte,
	packetScratch []rtp.RTCPPacket,
	lrrScratch []rtp.AV1RTCPLayerRefreshRequestEntry,
) (target rtp.AV1RTCPLayerRefreshLayerIndex, ok bool, packets []rtp.RTCPPacket, err error) {
	packets, err = rtp.ParseRTCPCompoundPackets(compound, packetScratch)
	if err != nil {
		return rtp.AV1RTCPLayerRefreshLayerIndex{}, false, packets, err
	}
	target, ok, err = EncoderWebRTCRTCPPacketsLayerRefreshTarget(config, packets, lrrScratch)
	return target, ok, packets, err
}

// EncoderWebRTCRTCPFeedbackRequiresKeyFrame reports whether a parsed RTCP
// feedback packet should be satisfied by forcing the next WebRTC encoder
// picture to be a key picture. PLI and non-empty FIR feedback require a key
// picture. Valid AV1 LRR feedback also returns true because the current encoder
// satisfies layer-refresh requests by refreshing the full configured picture.
// Transport feedback, NACK, REMB, and unknown PSFB feedback return false after
// any packet-type-specific FCI validation performed here.
func EncoderWebRTCRTCPFeedbackRequiresKeyFrame(
	config EncoderConfig,
	packet rtp.RTCPFeedbackPacket,
	firScratch []rtp.RTCPFullIntraRequestEntry,
	lrrScratch []rtp.AV1RTCPLayerRefreshRequestEntry,
) (bool, error) {
	switch packet.PacketType {
	case rtp.RTCPRTPFBPacketType:
		return false, nil
	case rtp.RTCPPSFBPacketType:
	default:
		return false, rtp.ErrRTCPInvalidFeedback
	}

	switch packet.FMT {
	case rtp.RTCPPSFBPictureLossIndicationFMT:
		if err := rtp.ParseRTCPPictureLossIndicationFCI(packet.FCI); err != nil {
			return false, err
		}
		return true, nil
	case rtp.RTCPPSFBFullIntraRequestFMT:
		entries, err := rtp.ParseRTCPFullIntraRequestEntries(packet.FCI, firScratch)
		if err != nil {
			return false, err
		}
		return len(entries) > len(firScratch), nil
	case rtp.RTCPPSFBLayerRefreshRequestFMT:
		entries, err := rtp.ParseAV1RTCPLayerRefreshRequestEntries(packet.FCI, lrrScratch)
		if err != nil {
			return false, err
		}
		if err := EncoderWebRTCValidateLayerRefreshRequests(config, entries[len(lrrScratch):]); err != nil {
			return false, err
		}
		return len(entries) > len(lrrScratch), nil
	default:
		return false, nil
	}
}

// EncoderWebRTCRTCPPacketsRequireKeyFrame scans parsed generic RTCP packets and
// reports whether any RTPFB/PSFB feedback packet should force the next WebRTC
// encoder picture to be a key picture. Non-feedback RTCP packets are ignored.
func EncoderWebRTCRTCPPacketsRequireKeyFrame(
	config EncoderConfig,
	packets []rtp.RTCPPacket,
	firScratch []rtp.RTCPFullIntraRequestEntry,
	lrrScratch []rtp.AV1RTCPLayerRefreshRequestEntry,
) (bool, error) {
	for i := range packets {
		packet := packets[i]
		if packet.PacketType != rtp.RTCPRTPFBPacketType && packet.PacketType != rtp.RTCPPSFBPacketType {
			continue
		}
		if len(packet.Payload) < rtp.RTCPFeedbackCommonSize {
			return false, rtp.ErrRTCPInvalidFeedback
		}
		force, err := EncoderWebRTCRTCPFeedbackRequiresKeyFrame(config, rtp.RTCPFeedbackPacket{
			PacketType: packet.PacketType,
			FMT:        packet.Count,
			SenderSSRC: binary.BigEndian.Uint32(packet.Payload[0:4]),
			MediaSSRC:  binary.BigEndian.Uint32(packet.Payload[4:8]),
			FCI:        packet.Payload[rtp.RTCPFeedbackCommonSize:],
		}, firScratch, lrrScratch)
		if err != nil || force {
			return force, err
		}
	}
	return false, nil
}

// EncoderWebRTCRTCPCompoundPacketsRequireKeyFrame parses a raw compound RTCP
// packet stream and reports whether any feedback packet should force the next
// WebRTC encoder picture to be a key picture. Parsed packets are appended into
// packetScratch without allocation and returned as the newly parsed suffix.
// Non-feedback RTCP packets are ignored. PLI, non-empty FIR, and valid AV1 LRR
// feedback can force a key picture; transport feedback, NACK, REMB, and
// unknown PSFB feedback do not.
func EncoderWebRTCRTCPCompoundPacketsRequireKeyFrame(
	config EncoderConfig,
	compound []byte,
	packetScratch []rtp.RTCPPacket,
	firScratch []rtp.RTCPFullIntraRequestEntry,
	lrrScratch []rtp.AV1RTCPLayerRefreshRequestEntry,
) (bool, []rtp.RTCPPacket, error) {
	packets, err := rtp.ParseRTCPCompoundPackets(compound, packetScratch)
	if err != nil {
		return false, packets, err
	}
	force, err := EncoderWebRTCRTCPPacketsRequireKeyFrame(config, packets, firScratch, lrrScratch)
	return force, packets, err
}

func encoderWebRTCValidateLayerRefreshRequestForConfig(
	normalized EncoderConfig, entry rtp.AV1RTCPLayerRefreshRequestEntry,
) error {
	if err := entry.Validate(); err != nil {
		return err
	}
	if entry.Target.TemporalID >= normalized.TemporalLayerCount ||
		entry.Target.SpatialID >= normalized.SpatialLayerCount {
		return rtp.ErrRTCPInvalidLayerRefreshRequest
	}
	if entry.CurrentPresent &&
		(entry.Current.TemporalID >= normalized.TemporalLayerCount ||
			entry.Current.SpatialID >= normalized.SpatialLayerCount) {
		return rtp.ErrRTCPInvalidLayerRefreshRequest
	}
	return nil
}
