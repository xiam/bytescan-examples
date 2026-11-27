package main

import (
	"bytes"
	"testing"

	"github.com/xiam/bytescan-examples/internal/lextest"
)

// The binary test inputs are built here, so the testdata/*.input files are
// reproducible: TestInputs checks that each committed input equals the bytes
// built below, and go test -update rewrites the inputs before TestGolden
// rewrites the golden files.

// inputs maps the name of each test case to the bytes of its input.
var inputs = map[string]func() []byte{
	"session":          session,
	"lengths":          lengths,
	"invalid-length":   invalidLength,
	"ff-run":           ffRun,
	"zero-length":      zeroLength,
	"short-publish":    shortPublish,
	"truncated":        truncated,
	"truncated-length": truncatedLength,
	"empty":            func() []byte { return nil },
}

func TestInputs(t *testing.T) {
	lextest.Inputs(t, inputs)
}

// The fixed header bytes of the packets in the tests (MQTT 3.1.1, section
// 2.2): the packet type in the high four bits, and the flags that section
// 2.2.2 requires in the low four. PUBLISH takes its flags from publishHeader.
const (
	connect     = 0x10
	connack     = 0x20
	puback      = 0x40
	pubrec      = 0x50
	pubrel      = 0x62
	pubcomp     = 0x70
	subscribe   = 0x82
	suback      = 0x90
	pingreq     = 0xC0
	pingresp    = 0xD0
	disconnect  = 0xE0
	publishType = publish << 4
)

// publishHeader returns the fixed header byte of PUBLISH (section 3.3.1):
// DUP in bit 3, QoS in bits 2 and 1, RETAIN in bit 0.
func publishHeader(dup bool, qos byte, retain bool) byte {
	b := byte(publishType) | qos<<1
	if dup {
		b |= 0x08
	}
	if retain {
		b |= 0x01
	}
	return b
}

// encodeLength returns the remaining length n in width bytes (section
// 2.2.3). MQTT requires the fewest bytes; a width above that pads the
// encoding with continuation bytes of value 0, which the scanner takes as it
// takes any other encoding. A width below the fewest bytes is raised to it.
func encodeLength(n, width int) []byte {
	var b []byte
	for {
		c := byte(n & 0x7F)
		n >>= 7
		if n > 0 {
			c |= 0x80
		}
		b = append(b, c)
		if n == 0 {
			break
		}
	}
	for len(b) < width {
		b[len(b)-1] |= 0x80
		b = append(b, 0x00)
	}
	return b
}

// packet returns a packet: the fixed header byte, the remaining length in
// width bytes, and the parts after it.
func packet(header byte, width int, parts ...[]byte) []byte {
	var rest []byte
	for _, p := range parts {
		rest = append(rest, p...)
	}
	b := append([]byte{header}, encodeLength(len(rest), width)...)
	return append(b, rest...)
}

// str returns a UTF-8 string of MQTT (section 1.5.3): two bytes of length,
// most significant first, then the bytes.
func str(s string) []byte {
	return append(u16(len(s)), s...)
}

// u16 returns n as two bytes, most significant first.
func u16(n int) []byte {
	return []byte{byte(n >> 8), byte(n)}
}

// connectPacket returns a CONNECT packet (section 3.1): protocol name "MQTT",
// level 4, the clean session flag, a keep alive of 60 s, and the client
// identifier.
func connectPacket(clientID string) []byte {
	return packet(connect, 1, str("MQTT"), []byte{4, 0x02}, u16(60), str(clientID))
}

// session is a realistic exchange: CONNECT and CONNACK, SUBSCRIBE and
// SUBACK, PUBLISH at QoS 0, at QoS 1 with RETAIN and its PUBACK, at QoS 2 with
// DUP and its PUBREC, PUBREL and PUBCOMP, PINGREQ and PINGRESP, and
// DISCONNECT. The payloads are text, JSON and binary bytes.
func session() []byte {
	var b []byte
	b = append(b, connectPacket("sensor-7")...)
	b = append(b, packet(connack, 1, []byte{0x00, 0x00})...)
	b = append(b, packet(subscribe, 1, u16(1), str("sensors/+/temp"), []byte{1}, str("alerts/#"), []byte{2})...)
	b = append(b, packet(suback, 1, u16(1), []byte{1, 2})...)
	b = append(b, packet(publishHeader(false, 0, false), 1, str("sensors/7/temp"), []byte("21.5"))...)
	b = append(b, packet(publishHeader(false, 1, true), 1, str("sensors/7/meta"), u16(2), []byte(`{"unit":"°C","rate":10}`))...)
	b = append(b, packet(puback, 1, u16(2))...)
	b = append(b, packet(publishHeader(true, 2, false), 1, str("alerts/fire"), u16(3), []byte{0x00, 0x80, 0xFF, 0x30, 0x10})...)
	b = append(b, packet(pubrec, 1, u16(3))...)
	b = append(b, packet(pubrel, 1, u16(3))...)
	b = append(b, packet(pubcomp, 1, u16(3))...)
	b = append(b, packet(pingreq, 1)...)
	b = append(b, packet(pingresp, 1)...)
	b = append(b, packet(disconnect, 1)...)
	return b
}

// lengths has a remaining length of each width: 1 byte, 2 bytes (207, the
// fewest bytes for it), and 3 and 4 bytes that pad small lengths. A 4-byte
// length of the fewest bytes declares at least 2 MiB; the truncated case has
// one.
func lengths() []byte {
	var b []byte
	b = append(b, packet(publishHeader(false, 0, false), 1, str("a/b"), []byte("on"))...)
	b = append(b, packet(publishHeader(false, 1, false), 2, str("big"), u16(4), lextest.Sequence(0x00, 200))...)
	b = append(b, packet(puback, 3, u16(4))...)
	b = append(b, packet(publishHeader(false, 0, false), 4, str("t"), []byte("1234"))...)
	return b
}

// invalidLength has two remaining lengths of 5 and 6 bytes, whose fourth
// byte still has the high bit set. Their bytes up to the first byte without
// the high bit are UNKNOWN, and the next byte is a FIXED_HEADER.
func invalidLength() []byte {
	var b []byte
	b = append(b, publishType, 0x80, 0x80, 0x80, 0x80, 0x01)
	b = append(b, packet(pingreq, 1)...)
	b = append(b, connack, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x05)
	b = append(b, packet(pingresp, 1)...)
	return b
}

// ffRun is a PINGREQ, a run of 48 bytes of 0xFF, a PUBLISH and a PINGRESP.
// The first 0xFF is a FIXED_HEADER, the rest of the run and the first byte of
// the PUBLISH are UNKNOWN, and the packets after it are framed at the wrong
// bytes: MQTT has no sync marker.
func ffRun() []byte {
	var b []byte
	b = append(b, packet(pingreq, 1)...)
	b = append(b, bytes.Repeat([]byte{0xFF}, 48)...)
	b = append(b, packet(publishHeader(false, 0, false), 1, str("a"), []byte("hi"))...)
	b = append(b, packet(pingresp, 1)...)
	return b
}

// zeroLength is packets with a remaining length of 0: PINGREQ, PINGRESP,
// DISCONNECT, and a PUBLISH, which then has no VARIABLE_HEADER.
func zeroLength() []byte {
	var b []byte
	b = append(b, packet(pingreq, 1)...)
	b = append(b, packet(pingresp, 1)...)
	b = append(b, packet(disconnect, 1)...)
	b = append(b, packet(publishHeader(false, 0, false), 1)...)
	b = append(b, packet(pingreq, 1)...)
	return b
}

// shortPublish is PUBLISH packets whose remaining length ends inside the
// variable header: 1 byte, a topic length of 5 with 1 byte of topic, and a
// QoS 1 packet with no room for the packet identifier. VARIABLE_HEADER ends
// at the packet end, and the next packet follows.
func shortPublish() []byte {
	var b []byte
	b = append(b, packet(publishHeader(false, 0, false), 1, []byte{0x00})...)
	b = append(b, packet(publishHeader(false, 0, false), 1, u16(5), []byte("a"))...)
	b = append(b, packet(publishHeader(false, 1, false), 1, str("a/b"))...)
	b = append(b, packet(pingreq, 1)...)
	return b
}

// truncated is a CONNECT and a PUBLISH whose remaining length is the maximum,
// 268,435,455 bytes (0xFF 0xFF 0xFF 0x7F), cut after 12 bytes.
func truncated() []byte {
	b := connectPacket("c")
	b = append(b, publishHeader(false, 1, false), 0xFF, 0xFF, 0xFF, 0x7F)
	b = append(b, str("x/y")...)
	b = append(b, u16(9)...)
	return append(b, []byte("hello")...)
}

// truncatedLength is a PINGREQ and a PUBLISH cut inside its remaining
// length: the length bytes are UNKNOWN.
func truncatedLength() []byte {
	b := packet(pingreq, 1)
	return append(b, publishType, 0x80, 0x80)
}
