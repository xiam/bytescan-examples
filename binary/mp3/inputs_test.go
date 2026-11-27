package main

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/xiam/bytescan-examples/internal/lextest"
)

// The binary test inputs are written byte by byte here, so the
// testdata/*.input files are reproducible: TestInputs checks that each
// committed input equals the bytes built below, and go test -update rewrites
// the inputs before TestGolden rewrites the golden files.

// A sample is a test input and the lengths of the audio frames written into
// it, headers included, in order.
type sample struct {
	b      []byte
	frames []int
}

// samples maps the name of each test case to its input.
var samples = map[string]sample{
	"document":     document(),
	"mpeg2":        mpeg2Sample(),
	"tag-mismatch": tagMismatch(),
	"id3v22":       id3v22(),
	"near-misses":  nearMisses(),
	"truncated":    truncated(),
}

func TestInputs(t *testing.T) {
	inputs := map[string][]byte{}
	for name, s := range samples {
		inputs[name] = s.b
	}
	lextest.Inputs(t, inputs)
}

// synchsafeBytes returns n as a 28-bit synchsafe integer.
func synchsafeBytes(n int) []byte {
	return []byte{byte(n >> 21 & 0x7F), byte(n >> 14 & 0x7F), byte(n >> 7 & 0x7F), byte(n & 0x7F)}
}

// tag returns an ID3v2 tag of major version v that holds frames and the
// given number of padding bytes.
func tag(v byte, padding int, frames ...[]byte) []byte {
	var body []byte
	for _, f := range frames {
		body = append(body, f...)
	}
	body = append(body, make([]byte, padding)...)
	b := append([]byte{'I', 'D', '3', v, 0, 0}, synchsafeBytes(len(body))...)
	return append(b, body...)
}

// tagFrame returns an ID3v2 frame: its id, the size of its body (synchsafe in
// v2.4, a plain 32-bit number in v2.3), two bytes of flags and the body.
func tagFrame(v byte, id string, body []byte) []byte {
	b := []byte(id)
	if v == 4 {
		b = append(b, synchsafeBytes(len(body))...)
	} else {
		b = binary.BigEndian.AppendUint32(b, uint32(len(body)))
	}
	b = append(b, 0, 0)
	return append(b, body...)
}

// frame appends to s an audio frame with header h, whose length frameLength
// computes, and a body that starts with mark and is zero after it.
func (s *sample) frame(h [4]byte, mark []byte) {
	n := frameLength(h[:])
	f := make([]byte, n)
	copy(f, h[:])
	copy(f[4:], mark)
	s.b = append(s.b, f...)
	s.frames = append(s.frames, n)
}

// The frame headers of the samples. Byte 3 is 1100 0100: mono, original.
var (
	// MPEG-1 Layer III, no CRC, 32 kbit/s, 48 000 Hz: 96 bytes.
	mpeg1At32k = [4]byte{0xFF, 0xFB, 0x14, 0xC4}
	// MPEG-2 Layer III, no CRC, 8 kbit/s, 22 050 Hz: 26 bytes, 27 padded.
	mpeg2At8k       = [4]byte{0xFF, 0xF3, 0x10, 0xC4}
	mpeg2At8kPadded = [4]byte{0xFF, 0xF3, 0x12, 0xC4}
	// MPEG-1 Layer III, no CRC, 320 kbit/s, 32 000 Hz: 1440 bytes.
	mpeg1At320k = [4]byte{0xFF, 0xFB, 0xE8, 0xC4}
)

// document is an ID3v2.4 tag with a title, an artist, a private frame whose
// body holds the bytes FF FB 14 C4 of a frame header, and padding; then three
// MPEG-1 frames, the second of which holds a frame header in its body.
func document() sample {
	s := sample{b: tag(4, 10,
		tagFrame(4, "TIT2", []byte("\x03Sine")),
		tagFrame(4, "TPE1", []byte("\x03textlexer")),
		tagFrame(4, "PRIV", []byte("lexer\x00\xff\xfb\x14\xc4")),
	)}
	s.frame(mpeg1At32k, nil)
	s.frame(mpeg1At32k, mpeg1At32k[:])
	s.frame(mpeg1At32k, nil)
	return s
}

// mpeg2Sample is an ID3v2.3 tag with a comment of 130 bytes, whose size
// 00 00 00 82 is a plain number (as a synchsafe number it would be 2), and no
// padding; then four MPEG-2 frames, the second and fourth padded.
func mpeg2Sample() sample {
	comment := append([]byte("\x00eng\x00"), bytes.Repeat([]byte("ab"), 62)...)
	comment = append(comment, '.')
	s := sample{b: tag(3, 0, tagFrame(3, "COMM", comment))}
	s.frame(mpeg2At8k, nil)
	s.frame(mpeg2At8kPadded, nil)
	s.frame(mpeg2At8k, nil)
	s.frame(mpeg2At8kPadded, nil)
	return s
}

// tagMismatch is an ID3v2.3 tag that holds a frame, the bytes "t-x!" that
// are not a frame id, and a frame whose stated size runs past the end of the
// tag; then one audio frame.
func tagMismatch() sample {
	long := tagFrame(3, "TALB", []byte("Album"))
	binary.BigEndian.PutUint32(long[4:], 100)
	s := sample{b: tag(3, 0, tagFrame(3, "TIT2", []byte("\x00Sine")), []byte("t-x!"), long)}
	s.frame(mpeg1At32k, nil)
	return s
}

// id3v22 is an ID3v2.2 tag (out of the subset) whose frame body holds the
// bytes FF FB 14 C4, then one audio frame. The tag is not lexed as a tag, so
// those bytes are taken for an audio frame.
func id3v22() sample {
	body := append([]byte("TT2\x00\x00\x09\x00Sine"), 0xFF, 0xFB, 0x14, 0xC4)
	b := append([]byte{'I', 'D', '3', 2, 0, 0}, synchsafeBytes(len(body))...)
	s := sample{b: append(b, body...)}
	s.frame(mpeg1At32k, nil)
	return s
}

// nearMisses starts with an ID3 header whose size is not synchsafe, then
// holds two frames with bytes between them, near-misses of a frame header
// (free format, bitrate index 15, a reserved sampling rate, Layer I and MPEG
// 2.5), an ID3 header that is not at the start of the file, and a frame
// header cut by the end of the input.
func nearMisses() sample {
	s := sample{b: []byte("ID3\x04\x00\x00\x00\x00\x00\x80")}
	s.frame(mpeg1At32k, nil)
	s.b = append(s.b, "junk"...)
	s.frame(mpeg1At32k, nil)
	s.b = append(s.b,
		0xFF, 0xFB, 0x04, 0xC4, // bitrate index 0: free format
		0xFF, 0xFB, 0xF4, 0xC4, // bitrate index 15
		0xFF, 0xFB, 0x1C, 0xC4, // sampling-rate index 3: reserved
		0xFF, 0xFF, 0x14, 0xC4, // Layer I
		0xFF, 0xE3, 0x10, 0xC4, // MPEG 2.5
	)
	s.b = append(s.b, tag(4, 0)...)
	s.b = append(s.b, 0xFF, 0xFB, 0x14)
	return s
}

// truncated is the document cut 40 bytes before the end of its second frame.
func truncated() sample {
	s := document()
	cut := len(s.b) - s.frames[2] - 40
	return sample{b: s.b[:cut], frames: []int{s.frames[0], s.frames[1] - 40}}
}
