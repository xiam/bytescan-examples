package main

import (
	"bytes"
	"testing"

	"github.com/xiam/bytescan-examples/internal/lexdump"
)

// The binary test inputs are built here, so the testdata/*.input files are
// reproducible: TestInputs checks that each committed input equals the bytes
// built below, and go test -update rewrites the inputs before TestGolden
// rewrites the golden files.

// inputs maps the name of each test case to the bytes of its input.
var inputs = map[string]func() []byte{
	"packets":          packets,
	"overrun":          overrun,
	"truncated":        truncated,
	"truncated-header": truncatedHeader,
	"resync":           resync,
	"empty":            func() []byte { return nil },
	"not-ts":           notTS,
}

func TestInputs(t *testing.T) {
	lexdump.Inputs(t, inputs)
}

// The values of adaptation_field_control (ISO/IEC 13818-1, table 2-5).
const (
	payloadOnly    = 0b01
	adaptationOnly = 0b10
	both           = 0b11
)

// The PIDs of the test streams: the program association table, a video and
// an audio elementary stream, and null packets (ISO/IEC 13818-1, table 2-3).
const (
	pidPAT   = 0x0000
	pidVideo = 0x0100
	pidAudio = 0x0101
	pidNull  = 0x1FFF
)

// header returns the sync byte and the 3 header bytes of a packet: no
// transport error, priority 0, not scrambled.
func header(pid int, start bool, control, counter byte) []byte {
	b1 := byte(pid>>8) & 0x1F
	if start {
		b1 |= 0x40
	}
	return []byte{syncByte, b1, byte(pid), control<<4 | counter&0x0F}
}

// packet returns a 188-byte packet: the header, then body, filled up with
// 0xFF stuffing to the packet size. It panics when body is too long.
func packet(pid int, start bool, control, counter byte, body ...[]byte) []byte {
	b := header(pid, start, control, counter)
	for _, part := range body {
		b = append(b, part...)
	}
	if len(b) > packetSize {
		panic("packet: body longer than 184 bytes")
	}
	return append(b, bytes.Repeat([]byte{0xFF}, packetSize-len(b))...)
}

// adaptation returns an adaptation field whose length byte is length, with
// the given bytes after it, filled up with 0xFF stuffing to length bytes. The
// length byte may announce more bytes than a packet holds.
func adaptation(length int, fields ...byte) []byte {
	b := append([]byte{byte(length)}, fields...)
	if n := min(length+1, bodySize); len(b) < n {
		b = append(b, bytes.Repeat([]byte{0xFF}, n-len(b))...)
	}
	return b
}

// pcr is an adaptation field of length 7: the flags byte with only
// PCR_flag set, then a 6-byte program clock reference.
var pcr = adaptation(7, 0x10, 0x00, 0x00, 0x47, 0x47, 0x7E, 0x00)

// pesStart is the start of a PES packet header carried in a payload (ISO/IEC
// 13818-1, section 2.4.3.6), which the example does not parse.
var pesStart = []byte{0x00, 0x00, 0x01, 0xE0, 0x00, 0x00, 0x80, 0x80, 0x05, 0x21, 0x00, 0x01, 0x00, 0x01}

// packets is a realistic stream of five packets: a payload-only packet of the
// program association table, a packet with an adaptation field and a payload
// whose bytes contain 0x47, an adaptation-only packet, a null packet, and a
// packet with an adaptation field of length 0 (one byte of stuffing).
func packets() []byte {
	var b []byte
	pat := []byte{0x00, 0x00, 0xB0, 0x0D, 0x00, 0x01, 0xC1, 0x00, 0x00, 0x00, 0x01, 0xE1, 0x00, 0x2A, 0x5D, 0x3C, 0x91}
	b = append(b, packet(pidPAT, true, payloadOnly, 0, pat)...)
	b = append(b, packet(pidVideo, true, both, 0, pcr, pesStart, []byte("GGG 0x47 in a payload"), lexdump.Sequence(0x40, 120))...)
	b = append(b, packet(pidVideo, false, adaptationOnly, 1, adaptation(183, 0x00))...)
	b = append(b, packet(pidNull, false, payloadOnly, 0)...)
	b = append(b, packet(pidAudio, false, both, 5, adaptation(0), lexdump.Sequence(0x47, 183))...)
	return b
}

// overrun has adaptation field lengths at and above the 183 bytes a packet
// holds: 183 with a payload announced (the payload is empty), 200 and 255
// (the field ends at the packet end), then a normal packet that starts at its
// sync byte.
func overrun() []byte {
	var b []byte
	b = append(b, packet(pidVideo, false, both, 2, adaptation(183, 0x00))...)
	b = append(b, packet(pidVideo, false, both, 3, adaptation(200, 0x00))...)
	b = append(b, packet(pidVideo, false, adaptationOnly, 4, adaptation(255, 0x00))...)
	b = append(b, packet(pidAudio, true, payloadOnly, 6, pesStart)...)
	return b
}

// truncated is a packet and a second packet cut inside its payload.
func truncated() []byte {
	return packets()[:packetSize+40]
}

// truncatedHeader is a packet and a second packet cut inside its header.
func truncatedHeader() []byte {
	return packets()[:packetSize+3]
}

// resync starts with garbage that holds a 0x47 that the byte one packet later
// does not confirm. Then come two good packets, a packet whose sync byte is
// corrupt (0x48) and whose payload holds unconfirmed 0x47 bytes, and two good
// packets again.
func resync() []byte {
	b := []byte{0x00, 0x13, syncByte, 0x37}
	b = append(b, packet(pidVideo, true, payloadOnly, 0, pesStart)...)
	b = append(b, packet(pidVideo, false, payloadOnly, 1, lexdump.Sequence(0x00, 184))...)
	bad := packet(pidVideo, false, payloadOnly, 2, []byte("GG"), lexdump.Sequence(0x00, 182))
	bad[0] = 0x48
	b = append(b, bad...)
	b = append(b, packet(pidAudio, false, payloadOnly, 0, lexdump.Sequence(0x10, 184))...)
	b = append(b, packet(pidAudio, false, payloadOnly, 1, lexdump.Sequence(0x20, 184))...)
	return b
}

// notTS is text with a 0x47 ('G') that no other one confirms.
func notTS() []byte {
	return []byte("not a transport stream: G\n")
}
