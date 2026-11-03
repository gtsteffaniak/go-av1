package decode

import av1 "github.com/gtsteffaniak/go-av1/pkg/av1"

// DecoderFrameWorkReferenceMVFrameShape returns the column count, row count,
// and total entry count required to back a TileReferenceMVFrame for the given
// sequence and frame size. Callers use the returned length to size the
// entries slice passed to BindDecoderFrameWorkReferenceMVFrame.
func DecoderFrameWorkReferenceMVFrameShape(sequence av1.SequenceHeader, size av1.FrameSize) (cols int, rows int, length int, err error) {
	batch := decoderFrameWorkFrameBatch(sequence, size)
	return batch.ReferenceMVFrameShape()
}

// BindDecoderFrameWorkReferenceMVFrame wires entries into a
// TileReferenceMVFrame matching (sequence, size). The slice must be at least
// the length reported by DecoderFrameWorkReferenceMVFrameShape.
func BindDecoderFrameWorkReferenceMVFrame(sequence av1.SequenceHeader, size av1.FrameSize, entries []av1.TileReferenceMVEntry) (av1.TileReferenceMVFrame, error) {
	batch := decoderFrameWorkFrameBatch(sequence, size)
	return batch.BindReferenceMVFrame(entries)
}

// BindDecoderFrameWorkTemporalMotionField wires entries into a
// TileTemporalMotionField matching (sequence, size) for temporal motion-vector
// projection.
func BindDecoderFrameWorkTemporalMotionField(sequence av1.SequenceHeader, size av1.FrameSize, entries []av1.TileTemporalMotionEntry) (av1.TileTemporalMotionField, error) {
	batch := decoderFrameWorkFrameBatch(sequence, size)
	return batch.BindTemporalMotionField(entries)
}
