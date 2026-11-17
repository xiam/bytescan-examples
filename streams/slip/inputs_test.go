package main

import (
	"testing"

	"github.com/xiam/bytescan-examples/internal/lexdump"
)

// The binary test inputs are built here, so the testdata/*.input files are
// reproducible: TestInputs checks that each committed input equals the bytes
// built below, and go test -update rewrites the inputs before TestGolden
// rewrites the golden files.

// inputs maps the name of each test case to the bytes of its input.
var inputs = map[string]func() []byte{
	"frames":   frames,
	"invalid":  invalid,
	"long-run": longRun,
	"empty":    func() []byte { return nil },
	"text":     func() []byte { return []byte("plain text, no SLIP bytes\n") },
}

func TestInputs(t *testing.T) {
	lexdump.Inputs(t, inputs)
}

// stuff appends payload to dst with the byte stuffing of RFC 1055: an END
// byte becomes ESC ESC_END and an ESC byte becomes ESC ESC_ESC.
func stuff(dst, payload []byte) []byte {
	for _, c := range payload {
		switch c {
		case end:
			dst = append(dst, esc, escEnd)
		case esc:
			dst = append(dst, esc, escEsc)
		default:
			dst = append(dst, c)
		}
	}
	return dst
}

// frame returns payload stuffed and followed by END, the way RFC 1055 sends a
// packet.
func frame(payload []byte) []byte {
	return append(stuff(nil, payload), end)
}

// ipv4 is the start of an IPv4 header with an END and an ESC byte in it
// (identification 0xC0DB), which the example does not parse.
var ipv4 = []byte{0x45, 0x00, 0x00, 0x1C, 0xC0, 0xDB, 0x40, 0x00, 0x40, 0x11, 0xB8, 0x61, 0xC0, 0xA8, 0x00, 0x01}

// frames is a stream the way RFC 1055 recommends sending it: an END first to
// flush line noise, then frames that each end with END. The first frame has
// both escape sequences in the middle of its payload; then come two empty
// frames (END END END), a frame of the bytes 0x80 to 0x9F, a frame that ends
// with an escaped END, and a frame of one escaped ESC.
func frames() []byte {
	b := []byte{end}
	b = append(b, frame(ipv4)...)
	b = append(b, end, end)
	b = append(b, frame(lexdump.Sequence(0x80, 32))...)
	b = append(b, frame([]byte("tail\xC0"))...)
	b = append(b, frame([]byte{esc})...)
	return b
}

// invalid has an ESC before a byte that does not finish an escape sequence
// ('A'), an ESC before END (the END still ends the frame), an ESC before an
// escape sequence (ESC ESC ESC_END), and ends with an ESC, so the input ends
// inside an escape sequence.
func invalid() []byte {
	b := []byte{end}
	b = append(b, 'o', 'k', esc, 'A', 'b', end)
	b = append(b, esc, end)
	b = append(b, esc, esc, escEnd, end)
	b = append(b, 'c', 'u', 't', esc)
	return b
}

// longRun is one run of 2,500 data bytes with no END or ESC in it, then an
// END: it is lexed as DATA pieces of 1,000, 1,000 and 500 bytes.
func longRun() []byte {
	b := make([]byte, 0, 2501)
	for len(b) < 2500 {
		b = append(b, lexdump.Sequence(0x00, 0xC0)...)
	}
	return append(b[:2500], end)
}
