package main

import (
	"io"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/bytesource"
	"github.com/xiam/bytescan/rules/octet"
)

// The token types. The names follow the parts of a control packet in MQTT
// 3.1.1, section 2. The scanners emit FIXED_HEADER and REMAINING_LENGTH; the
// driver reads VARIABLE_HEADER, PAYLOAD and BODY from the cursor.
const (
	fixedHeader     bytescan.TokenType = "FIXED_HEADER"
	remainingLength bytescan.TokenType = "REMAINING_LENGTH"
	variableHeader  bytescan.TokenType = "VARIABLE_HEADER"
	payload         bytescan.TokenType = "PAYLOAD"
	body            bytescan.TokenType = "BODY"
	unknown                            = bytescan.TokenTypeUnknown

	// Never emitted: the driver turns them into UNKNOWN.
	continuation bytescan.TokenType = "CONTINUATION" // a length byte with the high bit set
	lengthEnd    bytescan.TokenType = "LENGTH_END"   // a length byte without it
)

const (
	// maxLengthBytes is the most bytes of a remaining length (section
	// 2.2.3): three bytes with the high bit set, then one without.
	maxLengthBytes = 4

	// maxPiece is the most bytes in one VARIABLE_HEADER, PAYLOAD or BODY
	// token that the driver reads by a declared length. A longer one is
	// several tokens of the same type.
	maxPiece = 64 << 10

	// publish is the packet type of PUBLISH, the high four bits of the
	// fixed header byte (section 2.2.1).
	publish = 3
)

// Rules of length bytes.
var (
	// more is a length byte whose high bit, the continuation bit, is set.
	more = octet.Range(0x80, 0xFF)

	// last is a length byte whose high bit is clear: the last byte of a
	// remaining length.
	last = octet.Range(0x00, 0x7F)
)

// tokenGuard is the per-token byte bound of every scanner: streamtest.ByteGuard.
// The longest token of a rule is a REMAINING_LENGTH of 4 bytes, 8 bytes for
// the guard, which counts a byte of 0x80 or above as 2, so the guard never
// fires on any input. VARIABLE_HEADER, PAYLOAD and BODY are read by the
// driver, not by a rule.
const tokenGuard = 1024

// newScanner returns an MQTT scanner that reads r: a mode driver over one byte
// cursor, with the per-token guard on every scanner.
func newScanner(r io.Reader) *driver {
	return newDriver(bytesource.NewCursor(r), bytescan.WithMaxTokenBytes(tokenGuard))
}

// newHeaderScanner returns the scanner of the first byte of a packet (section
// 2.2): the packet type in the high four bits and the flags in the low four.
// Every value is lexed, the reserved types 0 and 15 included:
//
//	[\x00-\xff]
//
// It has one rule, so no tie arises.
func newHeaderScanner(cur bytescan.Cursor, opts ...bytescan.Option) *bytescan.Scanner {
	lx := bytescan.NewWithCursorOptions(cur, opts...)
	lx.MustAddRule(fixedHeader, octet.Any())
	return lx
}

// newLengthScanner returns the scanner of the remaining length after the fixed
// header (section 2.2.3). Each byte holds 7 bits of the length, least
// significant group first, and its high bit says that another byte follows.
// At most four bytes are allowed:
//
//	[\x80-\xff]{0,3}[\x00-\x7f]
//
// The rule reads at most four bytes before it decides. When the fourth byte
// still has the high bit set, or the input ends inside the length,
// REMAINING_LENGTH does not match, and CONTINUATION matches the first byte:
// the driver emits it as UNKNOWN and lexes the rest of the length with the
// scanner of newBadLengthScanner.
//
// No two rules match the same text: a byte with the high bit set is never a
// whole REMAINING_LENGTH, and CONTINUATION matches only such a byte.
func newLengthScanner(cur bytescan.Cursor, opts ...bytescan.Option) *bytescan.Scanner {
	lx := bytescan.NewWithCursorOptions(cur, opts...)
	lx.MustAddRule(remainingLength, octet.Uvarint(maxLengthBytes))
	lx.MustAddRule(continuation, more)
	return lx
}

// newBadLengthScanner returns the scanner of the rest of an invalid remaining
// length, one byte at a time: CONTINUATION for a byte with the high bit set,
// LENGTH_END for the first byte without it, which ends the length. The driver
// emits both as UNKNOWN.
//
// No two rules match the same text.
func newBadLengthScanner(cur bytescan.Cursor, opts ...bytescan.Option) *bytescan.Scanner {
	lx := bytescan.NewWithCursorOptions(cur, opts...)
	lx.MustAddRule(continuation, more)
	lx.MustAddRule(lengthEnd, last)
	return lx
}
