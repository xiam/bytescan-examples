package main

import (
	"io"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/bytesource"
	"github.com/xiam/bytescan/rules/octet"
)

// The token types. The header scanner emits FRAME_HEADER; the driver reads the
// extended length, the masking key and the payload from the cursor.
const (
	frameHeader    bytescan.TokenType = "FRAME_HEADER"
	extendedLength bytescan.TokenType = "EXTENDED_LENGTH"
	maskingKey     bytescan.TokenType = "MASKING_KEY"
	payload        bytescan.TokenType = "PAYLOAD"
)

// The frame layout of RFC 6455, section 5.2.
const (
	// headerSize is the 2-byte header: FIN, RSV1-3 and the opcode, then MASK
	// and the 7-bit payload length.
	headerSize = 2
	// maskBit is the MASK bit of the second header byte.
	maskBit = 0x80
	// lengthBits are the 7 bits of the payload length in the second header
	// byte.
	lengthBits = 0x7F
	// length16 and length64 are the 7-bit lengths that announce a 16-bit and
	// a 64-bit extended payload length.
	length16 = 126
	length64 = 127
	// keySize is the size of the masking key.
	keySize = 4
)

// maxPiece is the most bytes in one PAYLOAD token. The driver reads a longer
// payload as several PAYLOAD tokens in a row, so memory follows the piece
// size and not the declared length, which may be corrupt.
const maxPiece = 64 << 10

// tokenGuard is the per-token byte bound of the header scanner: twice
// streamtest.ByteGuard, because frame headers hold bytes of 0x80 and above,
// which the guard counts as 2 bytes. The only rule reads 2 bytes, so the
// guard never fires on any input.
const tokenGuard = 2048

// newScanner returns a WebSocket frame scanner that reads r: a driver over one
// byte cursor, with a scanner for the frame header, with the per-token guard,
// and a queue of the fields that the header announces.
func newScanner(r io.Reader) *driver {
	return newDriver(bytesource.NewCursor(r), bytescan.WithMaxTokenBytes(tokenGuard))
}

// newHeaderScanner returns the scanner for the 2-byte frame header over cur (RFC
// 6455, section 5.2): FIN, RSV1, RSV2, RSV3, the 4-bit opcode, MASK and the
// 7-bit payload length. Every value of these bits is lexed:
//
//	[\x00-\xff]{2}
//
// A header cut by the end of the input does not match; its byte is UNKNOWN.
// It has one rule, so no tie arises.
func newHeaderScanner(cur bytescan.Cursor, opts ...bytescan.Option) *bytescan.Scanner {
	lx := bytescan.NewWithCursorOptions(cur, opts...)
	lx.MustAddRule(frameHeader, octet.Fixed(headerSize))
	return lx
}
