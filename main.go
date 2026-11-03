// Command goav1 is the go-av1 module's root CLI. The module root holds only
// this thin entry point; the library lives under pkg/ and the heavier
// benchmarking tools under cmd/.
//
// Usage:
//
//	goav1 <command> [flags] [args]
//
// Commands:
//
//	decode   Decode an AV1 IVF stream to raw YUV.
//	encode   Encode a raw I420 frame sequence to AV1 IVF.
//	rtp      Packetize a temporal unit into AV1 RTP payloads.
//	info     Print IVF / sequence-header metadata for a stream.
//	version  Print the module version banner.
//
// Run "goav1 <command> -h" for command-specific flags.
package main

import (
	"bufio"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	av1 "github.com/gtsteffaniak/go-av1/pkg/av1"
	"github.com/gtsteffaniak/go-av1/pkg/decode"
	"github.com/gtsteffaniak/go-av1/pkg/encode"
	"github.com/gtsteffaniak/go-av1/pkg/rtp"
)

const cliName = "goav1"

// buildVersion may be set at build time with
// -ldflags "-X main.buildVersion=v1.2.3".
var buildVersion = ""

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", cliName, err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		usage(stderr)
		return flag.ErrHelp
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "decode":
		return runDecode(rest, stdout, stderr)
	case "encode":
		return runEncode(rest, stdout, stderr)
	case "rtp":
		return runRTP(rest, stdout, stderr)
	case "info":
		return runInfo(rest, stdout, stderr)
	case "version", "-version", "--version":
		fmt.Fprintf(stdout, "%s %s\n", cliName, versionString())
		return nil
	case "-h", "--help", "help":
		usage(stdout)
		return nil
	default:
		usage(stderr)
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func usage(w io.Writer) {
	fmt.Fprintf(w, `%s — pure-Go AV1 codec toolkit

usage: %s <command> [flags] [args]

commands:
  decode   decode an AV1 IVF stream to raw YUV
  encode   encode a raw I420 frame sequence to AV1 IVF
  rtp      packetize a temporal unit into AV1 RTP payloads
  info     print IVF / sequence-header metadata for a stream
  version  print the module version banner

Run "%s <command> -h" for command-specific flags.
`, cliName, cliName, cliName)
}

func versionString() string {
	if buildVersion != "" {
		return buildVersion
	}
	return "devel"
}

// ---------------------------------------------------------------------------
// decode
// ---------------------------------------------------------------------------

const decodeOutputBufferSize = 64 << 10

func runDecode(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet(cliName+" decode", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("o", "", "write YUV output to this file (default: stdout)")
	workers := fs.Int("workers", 1, "tile worker goroutine count")
	frameDelay := fs.Int("framedelay", 0, "maximum frames in flight (0 = auto, 1 = no frame threading)")
	quiet := fs.Bool("quiet", false, "suppress per-frame timing on stderr")
	fs.Usage = func() {
		fmt.Fprintf(stderr, "usage: %s decode [-o out.yuv] [-workers N] [-framedelay N] [-quiet] input.ivf\n\n", cliName)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("expected exactly one input IVF path")
	}
	if *workers < 1 {
		return fmt.Errorf("workers must be >= 1, got %d", *workers)
	}
	if *frameDelay < 0 {
		return fmt.Errorf("framedelay must be >= 0, got %d", *frameDelay)
	}

	src, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}
	dec, err := decode.NewDecoderFromIVF(src, decode.WithWorkers(*workers), decode.WithMaxFrameDelay(*frameDelay))
	if err != nil {
		return fmt.Errorf("open decoder: %w", err)
	}
	defer dec.Close()

	dst := stdout
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			return fmt.Errorf("create output: %w", err)
		}
		defer f.Close()
		dst = f
	}

	totalBytes, completed, err := decodeIVFToYUV(dec, dst, *quiet, stderr)
	if err != nil {
		return err
	}
	if !*quiet {
		fmt.Fprintf(stderr, "decoded %d frames, %d bytes\n", completed, totalBytes)
	}
	return nil
}

// decodeIVFToYUV drives the public decoder one temporal unit at a time so
// per-frame timing is meaningful, writing each visible output frame in display
// order.
func decodeIVFToYUV(dec *decode.Decoder, dst io.Writer, quiet bool, log io.Writer) (int64, int, error) {
	if err := dec.Reset(); err != nil {
		return 0, 0, fmt.Errorf("decoder reset: %w", err)
	}
	buf := bufio.NewWriterSize(dst, decodeOutputBufferSize)
	var totalBytes int64
	completed := 0
	for i := 0; ; i++ {
		start := time.Now()
		frames, ok, err := dec.DecodeNext()
		if err != nil {
			return totalBytes, completed, fmt.Errorf("frame %d: %w", i, err)
		}
		if !ok {
			break
		}
		elapsed := time.Since(start)
		for _, frame := range frames {
			if frame == nil {
				continue
			}
			n, err := writeYUVFrame(buf, frame)
			if err != nil {
				return totalBytes, completed, fmt.Errorf("frame %d write: %w", i, err)
			}
			totalBytes += n
			completed++
		}
		if !quiet {
			fmt.Fprintf(log, "frame %d ms=%.3f outputs=%d completed_frames=%d\n",
				i, float64(elapsed.Microseconds())/1000.0, len(frames), completed)
		}
	}
	if err := buf.Flush(); err != nil {
		return totalBytes, completed, fmt.Errorf("flush output: %w", err)
	}
	return totalBytes, completed, nil
}

// writeYUVFrame writes a decoded frame's Y, U, V planes to dst, stripping
// per-plane stride padding. Monochrome streams omit U/V. 10/12-bit samples are
// written little-endian (two bytes per sample), matching yuv420p10le etc.
func writeYUVFrame(dst io.Writer, frame *av1.Frame) (int64, error) {
	var written int64
	for _, plane := range []av1.FramePlane{frame.Y, frame.U, frame.V} {
		if plane.Width == 0 || plane.Height == 0 || len(plane.Pix) == 0 {
			continue
		}
		rowBytes := plane.Width * frame.Layout.BytesPerSample
		for row := 0; row < plane.Height; row++ {
			off := row * plane.Stride
			n, err := dst.Write(plane.Pix[off : off+rowBytes])
			if err != nil {
				return written + int64(n), err
			}
			written += int64(n)
		}
	}
	return written, nil
}

// ---------------------------------------------------------------------------
// encode
// ---------------------------------------------------------------------------

func runEncode(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet(cliName+" encode", flag.ContinueOnError)
	fs.SetOutput(stderr)
	width := fs.Int("width", 0, "frame width in pixels (required)")
	height := fs.Int("height", 0, "frame height in pixels (required)")
	qindex := fs.Int("q", 0, "fixed CQ quantizer 1..255 (overrides -bitrate)")
	bitrate := fs.Int("bitrate", 0, "CBR target bits per second (0 = CQ mode)")
	framerate := fs.Int("fps", 30, "frames per second (required for CBR)")
	keyInterval := fs.Int("keyint", 0, "force a keyframe every N frames (0 = encoder default)")
	frames := fs.Int("frames", 0, "number of frames to encode (0 = all in input)")
	layers := fs.Int("layers", 0, "temporal layers (0 = encoder default)")
	input := fs.String("i", "", "raw I420 input file (default: read stdin)")
	out := fs.String("o", "", "write the encoded IVF to this path (default: stdout)")
	fs.Usage = func() {
		fmt.Fprintf(stderr, "usage: %s encode -width W -height H [-q Q | -bitrate B -fps F] [-keyint N] [-frames N] [-i in.yuv] [-o out.ivf]\n\n", cliName)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *width <= 0 || *height <= 0 {
		fs.Usage()
		return errors.New("width and height are required and must be positive")
	}
	if *qindex == 0 && *bitrate <= 0 {
		fs.Usage()
		return errors.New("set either -q (CQ) or -bitrate (CBR)")
	}

	cfg := encode.VideoEncoderConfig{
		Width:          *width,
		Height:         *height,
		TemporalLayers: *layers,
	}
	if *bitrate > 0 {
		cfg.TargetBitrate = *bitrate
		cfg.Framerate = *framerate
	} else {
		cfg.QIndex = uint8(*qindex)
	}

	enc, err := encode.NewVideoEncoder(cfg)
	if err != nil {
		return fmt.Errorf("new encoder: %w", err)
	}
	defer enc.Close()

	in := io.Reader(os.Stdin)
	if *input != "" {
		f, err := os.Open(*input)
		if err != nil {
			return fmt.Errorf("open input: %w", err)
		}
		defer f.Close()
		in = f
	}

	dst := stdout
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			return fmt.Errorf("create output: %w", err)
		}
		defer f.Close()
		dst = f
	}

	return encodeI420ToIVF(enc, in, dst, *width, *height, *framerate, *frames, *keyInterval, stderr)
}

// encodeI420ToIVF reads tightly packed I420 (or I400 when the input is short)
// frames from in, encodes them, and writes an IVF stream to dst. The IVF header
// frame count is patched at the end so callers always get an accurate count.
func encodeI420ToIVF(enc *encode.VideoEncoder, in io.Reader, dst io.Writer, w, h, fps, maxFrames, keyInterval int, log io.Writer) error {
	bw := bufio.NewWriterSize(dst, decodeOutputBufferSize)
	if err := writeIVFHeader(bw, w, h, fps, 0); err != nil {
		return err
	}

	luma := w * h
	chroma := (w / 2) * (h / 2)
	frameSize := luma + 2*chroma
	raw := make([]byte, frameSize)

	frame := encode.I420Frame{
		Y:            make([]byte, luma),
		U:            make([]byte, chroma),
		V:            make([]byte, chroma),
		YStride:      w,
		ChromaStride: w / 2,
		Width:        w,
		Height:       h,
	}

	count := uint32(0)
	var frameHeader [12]byte
	for {
		if maxFrames > 0 && int(count) >= maxFrames {
			break
		}
		if _, err := io.ReadFull(in, raw); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				break
			}
			return fmt.Errorf("read frame %d: %w", count, err)
		}
		copy(frame.Y, raw[:luma])
		copy(frame.U, raw[luma:luma+chroma])
		copy(frame.V, raw[luma+chroma:])
		forceKey := keyInterval > 0 && count > 0 && int(count)%keyInterval == 0
		ef, err := enc.Encode(frame, forceKey)
		if err != nil {
			return fmt.Errorf("encode frame %d: %w", count, err)
		}
		binary.LittleEndian.PutUint32(frameHeader[0:], uint32(len(ef.Data)))
		binary.LittleEndian.PutUint64(frameHeader[4:], uint64(count))
		if _, err := bw.Write(frameHeader[:]); err != nil {
			return err
		}
		if _, err := bw.Write(ef.Data); err != nil {
			return err
		}
		if log != nil {
			fmt.Fprintf(log, "frame %d key=%v temporal=%d bytes=%d\n", count, ef.Keyframe, ef.TemporalID, len(ef.Data))
		}
		count++
	}
	if err := bw.Flush(); err != nil {
		return err
	}
	// Patch the frame count into the IVF header when the sink is seekable.
	if s, ok := dst.(io.WriteSeeker); ok && count > 0 {
		if _, err := s.Seek(24, io.SeekStart); err == nil {
			var b [4]byte
			binary.LittleEndian.PutUint32(b[:], count)
			if _, err := s.Write(b[:]); err != nil {
				return fmt.Errorf("patch frame count: %w", err)
			}
		}
	}
	return nil
}

// writeIVFHeader writes a 32-byte IVF/DKIF header for AV01 streams.
func writeIVFHeader(w io.Writer, width, height, timebaseDen int, frameCount uint32) error {
	var hdr [32]byte
	copy(hdr[0:4], "DKIF")
	binary.LittleEndian.PutUint16(hdr[4:], 0)  // version
	binary.LittleEndian.PutUint16(hdr[6:], 32) // header size
	copy(hdr[8:12], "AV01")
	binary.LittleEndian.PutUint16(hdr[12:], uint16(width))
	binary.LittleEndian.PutUint16(hdr[14:], uint16(height))
	binary.LittleEndian.PutUint32(hdr[16:], uint32(timebaseDen))
	binary.LittleEndian.PutUint32(hdr[20:], 1) // timebase denominator
	binary.LittleEndian.PutUint32(hdr[24:], frameCount)
	// hdr[28:32] is the unused field and stays zero.
	_, err := w.Write(hdr[:])
	return err
}

// ---------------------------------------------------------------------------
// rtp
// ---------------------------------------------------------------------------

func runRTP(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet(cliName+" rtp", flag.ContinueOnError)
	fs.SetOutput(stderr)
	mtu := fs.Int("mtu", 1200, "maximum RTP payload size in bytes")
	key := fs.Bool("key", false, "treat the temporal unit as a keyframe (independent)")
	last := fs.Bool("last", true, "mark the temporal unit as the last of the picture")
	fs.Usage = func() {
		fmt.Fprintf(stderr, "usage: %s rtp [-mtu N] [-key] [-last=false] input.obu\n\n", cliName)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("expected exactly one input temporal-unit file")
	}
	if *mtu <= 0 {
		return fmt.Errorf("mtu must be positive, got %d", *mtu)
	}

	payload, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}
	limits := rtp.RTPPayloadSizeLimits{MaxPayloadLen: *mtu}
	// Size the OBU scratch first, then re-run sizing with that scratch so the
	// packet and work capacities are computed too.
	sizing, err := rtp.RTPPacketizerScratchLen(payload, limits, nil)
	if err != nil {
		return fmt.Errorf("scratch sizing: %w", err)
	}
	obus := make([]rtp.RTPPacketizerOBU, sizing.OBUs)
	scratch, err := rtp.RTPPacketizerScratchLen(payload, limits, obus)
	if err != nil {
		return fmt.Errorf("scratch sizing: %w", err)
	}
	plans := make([]rtp.RTPPacketPlan, scratch.Packets)
	work := make([]rtp.RTPPacketPlan, scratch.Work)
	p, err := rtp.NewRTPPacketizer(payload, limits, *key, *last, obus, plans, work)
	if err != nil {
		return fmt.Errorf("packetize: %w", err)
	}

	buf := make([]byte, *mtu+256)
	n := 0
	for {
		written, marker, ok, err := p.NextPacket(buf)
		if err != nil {
			return fmt.Errorf("packet %d: %w", n, err)
		}
		if !ok {
			break
		}
		fmt.Fprintf(stderr, "packet %d size=%d marker=%v\n", n, written, marker)
		if _, err := stdout.Write(buf[:written]); err != nil {
			return err
		}
		n++
	}
	fmt.Fprintf(stderr, "wrote %d RTP payloads\n", n)
	return nil
}

// ---------------------------------------------------------------------------
// info
// ---------------------------------------------------------------------------

func runInfo(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet(cliName+" info", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "usage: %s info input.ivf\n\n", cliName)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("expected exactly one input IVF path")
	}

	src, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}
	hdr, consumed, err := av1.ParseIVFHeader(src)
	if err != nil {
		return fmt.Errorf("parse IVF header: %w", err)
	}
	fmt.Fprintf(stdout, "IVF: version=%d fourcc=%s size=%dx%d frames=%d timebase=%d/%d header=%dB\n",
		hdr.Version, string(hdr.FourCC[:]), hdr.Width, hdr.Height, hdr.FrameCount,
		hdr.TimebaseDen, hdr.TimebaseNum, consumed)

	it, err := av1.NewIVFIterator(src)
	if err != nil {
		return fmt.Errorf("open IVF iterator: %w", err)
	}
	first, ok, err := it.Next()
	if err != nil {
		return fmt.Errorf("read first frame: %w", err)
	}
	if !ok {
		fmt.Fprintln(stdout, "no frames")
		return nil
	}
	seq, err := parseFirstSequenceHeader(first.Payload)
	if err != nil {
		fmt.Fprintf(stdout, "sequence header: %v\n", err)
		return nil
	}
	fmt.Fprintf(stdout, "sequence: profile=%d still=%v superres=%v cdef=%v restoration=%v max=%dx%d level=%d%s\n",
		seq.SeqProfile, seq.StillPicture, seq.EnableSuperRes, seq.EnableCDEF, seq.EnableRestoration,
		seq.MaxFrameWidth, seq.MaxFrameHeight,
		seq.OperatingPoints[0].SeqLevelIdx, tierName(seq.OperatingPoints[0].SeqTier))
	return nil
}

// parseFirstSequenceHeader walks the temporal unit's OBUs until it finds and
// parses the sequence header OBU.
func parseFirstSequenceHeader(tu []byte) (av1.SequenceHeader, error) {
	it := av1.NewLowOverheadIterator(tu)
	for {
		unit, ok, err := it.Next()
		if err != nil {
			return av1.SequenceHeader{}, err
		}
		if !ok {
			return av1.SequenceHeader{}, errors.New("no sequence header OBU in temporal unit")
		}
		if unit.Header.Type != av1.OBUSequenceHeader {
			continue
		}
		return av1.ParseSequenceHeader(unit.Payload)
	}
}

func tierName(tier uint8) string {
	switch tier {
	case 0:
		return " (main)"
	case 1:
		return " (high)"
	default:
		return ""
	}
}
