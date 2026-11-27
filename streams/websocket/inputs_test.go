package main

import (
	"encoding/binary"
	"testing"

	"github.com/xiam/bytescan-examples/internal/lextest"
)

// The binary test inputs are built here, so the testdata/*.input files are
// reproducible: TestInputs checks that each committed input equals the bytes
// built below, and go test -update rewrites the inputs before TestGolden
// rewrites the golden files.

// inputs maps the name of each test case to the bytes of its input.
var inputs = map[string]func() []byte{
	"frames":           frames,
	"pieces":           pieces,
	"fragmented":       fragmented,
	"truncated":        truncated,
	"truncated-header": func() []byte { return []byte{0x81} },
	"truncated-length": func() []byte { return []byte{0x82, 0x7F, 0x00, 0x00, 0x00} },
	"truncated-key":    func() []byte { return []byte{0x81, 0x85, 0x37, 0xFA} },
	"huge-length":      hugeLength,
	"empty":            func() []byte { return nil },
	"not-websocket":    notWebSocket,
}

func TestInputs(t *testing.T) {
	lextest.Inputs(t, inputs)
}

// The opcodes of RFC 6455, section 5.2.
const (
	opContinuation = 0x0
	opText         = 0x1
	opBinary       = 0x2
	opClose        = 0x8
	opPing         = 0x9
	opPong         = 0xA
)

// appendFrame appends one frame: FIN and the opcode, the payload length in
// its shortest form (7, 16 or 64 bits), and, when key is not nil, the masking
// key and the payload masked with it (RFC 6455, section 5.3).
func appendFrame(dst []byte, fin bool, opcode byte, key []byte, data []byte) []byte {
	b0 := opcode & 0x0F
	if fin {
		b0 |= 0x80
	}
	var b1 byte
	if key != nil {
		b1 = maskBit
	}
	n := len(data)
	switch {
	case n < length16:
		dst = append(dst, b0, b1|byte(n))
	case n <= 0xFFFF:
		dst = append(dst, b0, b1|length16)
		dst = binary.BigEndian.AppendUint16(dst, uint16(n))
	default:
		dst = append(dst, b0, b1|length64)
		dst = binary.BigEndian.AppendUint64(dst, uint64(n))
	}
	if key == nil {
		return append(dst, data...)
	}
	dst = append(dst, key...)
	for i, c := range data {
		dst = append(dst, c^key[i%keySize])
	}
	return dst
}

// rfcKey is the masking key of the examples of RFC 6455, section 5.7.
var rfcKey = []byte{0x37, 0xFA, 0x21, 0x3D}

// closeBody is the body of a close frame: a 2-byte status code and a reason.
func closeBody(code uint16, reason string) []byte {
	return append(binary.BigEndian.AppendUint16(nil, code), reason...)
}

// frames has the frames of the examples of RFC 6455, section 5.7, and more:
// an unmasked and a masked "Hello" (7-bit lengths), an unmasked ping, a
// 256-byte unmasked binary frame and a 300-byte masked one (16-bit lengths),
// a masked ping with no payload, and a masked close frame with status 1000.
func frames() []byte {
	var b []byte
	b = appendFrame(b, true, opText, nil, []byte("Hello"))
	b = appendFrame(b, true, opText, rfcKey, []byte("Hello"))
	b = appendFrame(b, true, opPing, nil, []byte("Hello"))
	b = appendFrame(b, true, opBinary, nil, lextest.Sequence(0x00, 256))
	b = appendFrame(b, true, opBinary, rfcKey, lextest.Sequence(0x80, 300))
	b = appendFrame(b, true, opPing, rfcKey, nil)
	b = appendFrame(b, true, opClose, rfcKey, closeBody(1000, "done"))
	return b
}

// pieces has a masked binary frame with a 64-bit length of 65,537 bytes, one
// more than maxPiece, so its payload is two PAYLOAD tokens; then an
// unmasked pong.
func pieces() []byte {
	var b []byte
	b = appendFrame(b, true, opBinary, rfcKey, lextest.Sequence(0x00, maxPiece+1))
	b = appendFrame(b, true, opPong, nil, []byte("pong"))
	return b
}

// fragmented is a masked text message in three fragments with a ping between
// the first two (a control frame may come between the fragments of a
// message, section 5.4), the pong of the server, and a close frame.
func fragmented() []byte {
	key := []byte{0x9E, 0x01, 0x5C, 0xA7}
	var b []byte
	b = appendFrame(b, false, opText, key, []byte("Hel"))
	b = appendFrame(b, true, opPing, key, []byte("are you there?"))
	b = appendFrame(b, false, opContinuation, key, []byte("lo, "))
	b = appendFrame(b, true, opContinuation, key, []byte("world"))
	b = appendFrame(b, true, opPong, nil, []byte("are you there?"))
	b = appendFrame(b, true, opClose, nil, closeBody(1001, ""))
	return b
}

// truncated is a frame and a second frame, with a 16-bit length, cut inside
// its payload.
func truncated() []byte {
	var b []byte
	b = appendFrame(b, true, opText, nil, []byte("Hello"))
	b = appendFrame(b, true, opBinary, rfcKey, lextest.Sequence(0x00, 200))
	return b[:len(b)-150]
}

// hugeLength is a frame whose 64-bit length is the largest value, 2^64-1,
// followed by a few bytes: its payload runs to the end of the input.
func hugeLength() []byte {
	b := []byte{0x82, length64, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	return append(b, "the rest of the stream"...)
}

// notWebSocket is a line of text, not frames: it lexes as frames all the same,
// because nothing in the frame format marks where a frame starts. (It has no
// CR byte, because a global core.autocrlf=input would store CR LF as LF.)
func notWebSocket() []byte {
	return []byte("This is a text file, not a stream of WebSocket frames.\n")
}
