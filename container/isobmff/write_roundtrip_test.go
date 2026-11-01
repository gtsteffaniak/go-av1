package isobmff

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteToRoundTripAVIFItemData(t *testing.T) {
	path := filepath.Join("..", "testdata", "i8_1080.avif")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	orig, err := ParseContainer(raw)
	if err != nil {
		t.Fatalf("ParseContainer: %v", err)
	}
	primary := orig.PrimaryItemID()
	if primary == 0 {
		t.Fatal("missing primary item")
	}
	want, err := orig.ItemData(primary)
	if err != nil {
		t.Fatalf("ItemData orig: %v", err)
	}

	encoded, err := orig.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	round, err := ParseContainer(encoded)
	if err != nil {
		t.Fatalf("ParseContainer round: %v", err)
	}
	got, err := round.ItemData(primary)
	if err != nil {
		t.Fatalf("ItemData round: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("item payload mismatch: got %d bytes, want %d", len(got), len(want))
	}
}

func TestWriteToRoundTripMultiMdatSamplePayloads(t *testing.T) {
	seq := Sequence{
		Width: 64, Height: 48, BitDepth: 8, Timescale: 1000,
		ConfigOBUs: []byte{0x0A, 0x0B},
		Frames: []SequenceFrame{
			{AV1Bitstream: []byte{0xDE, 0xAD}, DurationTicks: 1, IsSync: true},
			{AV1Bitstream: []byte{0xBE, 0xEF, 0x01}, DurationTicks: 1, IsSync: true},
		},
	}
	raw, err := BuildSequence(seq)
	if err != nil {
		t.Fatalf("BuildSequence: %v", err)
	}
	ct, err := ParseContainer(raw)
	if err != nil {
		t.Fatalf("ParseContainer: %v", err)
	}
	if len(ct.Mdats) != 1 {
		t.Fatalf("expected one mdat region, got %d", len(ct.Mdats))
	}
	samples, err := ct.Moov.ImageTrackStbl().SampleTable()
	if err != nil {
		t.Fatalf("SampleTable: %v", err)
	}
	if len(samples) != 2 {
		t.Fatalf("samples=%d want 2", len(samples))
	}

	reg := ct.Mdats[0]
	splitRel := int(samples[1].Offset - reg.PayloadOffset)
	if splitRel <= 0 || splitRel >= len(reg.Data) {
		t.Fatalf("unexpected split point %d in mdat len %d", splitRel, len(reg.Data))
	}
	ct.Mdats = []MdatRegion{
		{PayloadOffset: reg.PayloadOffset, Data: append([]byte(nil), reg.Data[:splitRel]...)},
		{PayloadOffset: samples[1].Offset, Data: append([]byte(nil), reg.Data[splitRel:]...)},
	}

	wantPayloads := make([][]byte, len(samples))
	for i, s := range samples {
		chunk, err := ct.ReadPayloadAt(s.Offset, uint64(s.Size))
		if err != nil {
			t.Fatalf("ReadPayloadAt sample %d: %v", i, err)
		}
		wantPayloads[i] = chunk
	}

	encoded, err := ct.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	round, err := ParseContainer(encoded)
	if err != nil {
		t.Fatalf("ParseContainer round: %v", err)
	}
	gotSamples, err := round.Moov.ImageTrackStbl().SampleTable()
	if err != nil {
		t.Fatalf("SampleTable round: %v", err)
	}
	if len(gotSamples) != len(wantPayloads) {
		t.Fatalf("samples=%d want %d", len(gotSamples), len(wantPayloads))
	}
	for i, want := range wantPayloads {
		got, err := round.ReadPayloadAt(gotSamples[i].Offset, uint64(gotSamples[i].Size))
		if err != nil {
			t.Fatalf("ReadPayloadAt sample %d: %v", i, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("sample %d payload mismatch", i)
		}
	}
}
