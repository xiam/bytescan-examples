package main

import (
	"io"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/bytesource"
	"github.com/xiam/bytescan/modes"
	"github.com/xiam/bytescan/rules/core"
	"github.com/xiam/bytescan/rules/octet"
)

// The token types. The scanners emit the three headers; the driver reads the
// bodies and the tag padding.
const (
	id3Header      bytescan.TokenType = "ID3_HEADER"
	id3FrameHeader bytescan.TokenType = "ID3_FRAME_HEADER"
	id3FrameBody   bytescan.TokenType = "ID3_FRAME_BODY"
	id3Padding     bytescan.TokenType = "ID3_PADDING"
	frameHeader    bytescan.TokenType = "FRAME_HEADER"
	frameBody      bytescan.TokenType = "FRAME_BODY"
)

// newScanner returns an MP3 scanner that reads r: a mode driver over one byte
// cursor, with a scanner for the start of the file, one for the frames of an
// ID3v2 tag, one for the audio frames, and a queue of the bodies that their
// headers announce.
func newScanner(r io.Reader) *driver {
	return newDriver(bytesource.NewCursor(r))
}

// newDriver returns the driver over cur.
func newDriver(cur bytescan.Cursor) *driver {
	start := newStartScanner(cur)
	return &driver{
		cur:    cur,
		modes:  modes.New(cur, start),
		start:  start,
		tag:    newTagScanner(cur),
		audio:  newAudioScanner(cur),
		fields: bytesource.NewFields(cur),
	}
}

// newStartScanner returns the scanner for the first token of the file over cur:
// an ID3v2 tag header or an audio frame header.
//
// No two rules match the same text: one starts with "ID3", the other with FF.
func newStartScanner(cur bytescan.Cursor) *bytescan.Scanner {
	lx := bytescan.NewWithCursor(cur)
	lx.MustAddRule(id3Header, id3HeaderRule())
	lx.MustAddRule(frameHeader, frameHeaderRule())
	return lx
}

// newTagScanner returns the scanner for the frame headers of an ID3v2.3 or v2.4
// tag over cur (ID3v2.3, section 3.3; ID3v2.4 structure, section 4): a frame
// id of four upper-case letters or digits, the four-byte size of the body and
// two bytes of flags:
//
//	[A-Z0-9]{4}[\x00-\xff]{6}
//
// It has one rule, so no tie arises.
func newTagScanner(cur bytescan.Cursor) *bytescan.Scanner {
	lx := bytescan.NewWithCursor(cur)
	lx.MustAddRule(id3FrameHeader, core.Sequence(
		core.Repeat(core.Choice(octet.Range('A', 'Z'), octet.Range('0', '9')), 4, 4),
		octet.Fixed(6),
	))
	return lx
}

// newAudioScanner returns the scanner for the audio frame headers over cur. Bytes
// that are not a frame header are UNKNOWN, one at a time, until the next one.
//
// It has one rule, so no tie arises.
func newAudioScanner(cur bytescan.Cursor) *bytescan.Scanner {
	lx := bytescan.NewWithCursor(cur)
	lx.MustAddRule(frameHeader, frameHeaderRule())
	return lx
}

// id3HeaderRule returns a rule for the 10-byte header of an ID3v2.3 or v2.4
// tag (ID3v2.3, section 3.1): "ID3", the major version 3 or 4, a revision
// below FF, a byte of flags, and the size of the tag after the header as a
// 28-bit synchsafe integer, four bytes of seven bits each:
//
//	ID3[\x03\x04][\x00-\xfe][\x00-\xff][\x00-\x7f]{4}
//
// A header cut by the end of the input does not match; its bytes are UNKNOWN.
func id3HeaderRule() bytescan.Rule {
	return core.Sequence(
		octet.Literal([]byte("ID3")),
		octet.Set(0x03, 0x04),
		octet.Range(0x00, 0xFE),
		octet.Any(),
		core.Repeat(octet.Range(0x00, 0x7F), 4, 4),
	)
}

// frameHeaderRule returns a rule for the 4-byte header of an MPEG-1 or MPEG-2
// Layer III audio frame (ISO/IEC 11172-3, section 2.4.2.3; ISO/IEC 13818-3,
// section 2.4.2.3). The first 11 bits are the sync word. The second byte holds
// the version (11 for MPEG-1, 10 for MPEG-2), the layer (01 for Layer III) and
// the protection bit. The third byte holds the bitrate index (1 to 14; 0 is
// free format and 15 is not allowed), the sampling-rate index (0 to 2; 3 is
// reserved), the padding bit and the private bit. The fourth byte holds the
// mode and is not checked:
//
//	\xff[\xf2\xf3\xfa\xfb]([\x10-\xeb] with a low nibble of 0 to B)[\x00-\xff]
func frameHeaderRule() bytescan.Rule {
	var third []bytescan.Rule
	for index := byte(1); index <= 14; index++ {
		third = append(third, octet.Range(index<<4, index<<4|0x0B))
	}
	return core.Sequence(
		octet.Literal([]byte{0xFF}),
		octet.Set(0xF2, 0xF3, 0xFA, 0xFB),
		core.Choice(third...),
		octet.Any(),
	)
}
