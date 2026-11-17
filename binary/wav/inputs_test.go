package main

import (
	"encoding/binary"
	"testing"

	"github.com/xiam/bytescan-examples/internal/lexdump"
)

// The binary test inputs are written byte by byte here, so the
// testdata/*.input files are reproducible: TestInputs checks that each
// committed input equals the bytes built below, and go test -update rewrites
// the inputs before TestGolden rewrites the golden files.

// inputs maps the name of each test case to the bytes of its input.
var inputs = map[string][]byte{
	"document":  document(),
	"chunks":    chunks(),
	"truncated": document()[:100],
	"not-wav":   notWAV(),
}

func TestInputs(t *testing.T) {
	lexdump.Inputs(t, inputs)
}

var (
	le16 = binary.LittleEndian.AppendUint16
	le32 = binary.LittleEndian.AppendUint32
)

// chunk returns a chunk: its id, the length of payload, the payload, and a
// pad byte when the length is odd.
func chunk(id string, payload []byte) []byte {
	b := le32([]byte(id), uint32(len(payload)))
	b = append(b, payload...)
	if len(payload)%2 == 1 {
		b = append(b, 0)
	}
	return b
}

// riff returns a RIFF WAVE file that holds chunks, with the size of the file
// after the size field in its header.
func riff(chunks ...[]byte) []byte {
	body := []byte("WAVE")
	for _, c := range chunks {
		body = append(body, c...)
	}
	return append(le32([]byte("RIFF"), uint32(len(body))), body...)
}

// pcmFormat returns the 16-byte payload of a fmt chunk for PCM samples
// (WAVE_FORMAT_PCM, format tag 1).
func pcmFormat(channels, rate, bits int) []byte {
	align := channels * bits / 8
	b := le16(nil, 1)
	b = le16(b, uint16(channels))
	b = le32(b, uint32(rate))
	b = le32(b, uint32(rate*align))
	b = le16(b, uint16(align))
	return le16(b, uint16(bits))
}

// samples returns n unsigned 8-bit samples of a sawtooth wave.
func samples(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i * 16)
	}
	return b
}

// document is a mono 8-bit file at 8000 Hz: a fmt chunk, an extra chunk of
// odd length whose payload holds the bytes "data", and a data chunk of 200
// samples.
func document() []byte {
	return riff(
		chunk("fmt ", pcmFormat(1, 8000, 8)),
		chunk("note", []byte("data!")),
		chunk("data", samples(200)),
	)
}

// chunks is a stereo 16-bit file with a data chunk of odd length and its pad
// byte, a LIST chunk (lexed as one chunk), and an empty chunk.
func chunks() []byte {
	info := []byte("INFO")
	info = append(info, chunk("INAM", []byte("Saw\x00"))...)
	return riff(
		chunk("fmt ", pcmFormat(2, 44100, 16)),
		chunk("data", []byte{1, 2, 3}),
		chunk("LIST", info),
		chunk("junk", nil),
	)
}

// notWAV holds near-misses of the RIFF header, then a RIFF header and
// near-misses of a chunk header: an id with a byte outside ASCII, an id with a
// control byte, and a chunk header cut by the end of the input.
func notWAV() []byte {
	b := []byte("RIFX\x04\x00\x00\x00WAVE riff\x04\x00\x00\x00WAVE RIFF\x04\x00\x00\x00WAVX ")
	b = append(b, "RIFF\x20\x00\x00\x00WAVE"...)
	b = append(b, "f\xe9t \x01\x00\x00\x00"...)
	b = append(b, "dat\x01\x01\x00\x00\x00"...)
	return append(b, "fmt \x10\x00"...)
}
