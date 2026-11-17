package main

import (
	"bufio"
	"io"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/rules/core"
)

// The token types, named after the parts of a sentence in NMEA 0183.
const (
	start    bytescan.TokenType = "START"
	address  bytescan.TokenType = "ADDRESS"
	field    bytescan.TokenType = "FIELD"
	comma    bytescan.TokenType = "COMMA"
	checksum bytescan.TokenType = "CHECKSUM"
	eol      bytescan.TokenType = "EOL"
)

// maxRun is the most runes in one FIELD token. A longer run of field bytes,
// such as a stream that has lost its delimiters, is emitted as several FIELD
// tokens in a row, so no token grows with the stream.
const maxRun = 1000

// tokenGuard is the per-token byte bound of the scanner: a little above the
// largest token, a FIELD of maxRun ASCII bytes. Every rule is bounded, so the
// guard never fires; it turns a rule that scans without bound into an error
// instead of a growing heap. It equals streamtest.TextGuard.
const tokenGuard = 4096

// fieldByte is the class of the bytes of a field: printable ASCII (0x20 to
// 0x7E) except the delimiters $ ! * and the comma.
var fieldByte = core.RuneRange(0x20, 0x7E).And(core.RuneSet("$!*,").Not())

// newScanner returns an NMEA 0183 scanner that reads r, with the per-token guard.
//
// The rules are context-free. ADDRESS and FIELD both match a run of five
// upper-case letters that a delimiter follows, such as GPGGA; ADDRESS is
// registered first, so it wins that tie. A longer run, or one with another
// byte, is longer as a FIELD, and the longest match makes it a FIELD. No other
// two rules match the same text.
func newScanner(r io.Reader) *bytescan.Scanner {
	lx := bytescan.NewWithOptions(bufio.NewReader(r), bytescan.WithMaxTokenBytes(tokenGuard))

	lx.MustAddRule(start, core.Rune(core.RuneSet("$!")))
	lx.MustAddRule(address, addressRule())
	lx.MustAddRule(field, fieldRule())
	lx.MustAddRule(comma, core.Literal(","))
	lx.MustAddRule(checksum, checksumRule())
	lx.MustAddRule(eol, eolRule())

	return lx
}

// addressRule returns a rule for the address field: a two-letter talker
// identifier and a three-letter sentence formatter (GPGGA, AIVDM), or P and a
// four-letter proprietary code of the same length (PGRME):
//
//	[A-Z]{5}
func addressRule() bytescan.Rule {
	return core.Repeat(core.Rune(core.RuneRange('A', 'Z')), 5, 5)
}

// fieldRule returns a rule for a data field, a non-empty run of field bytes of
// at most maxRun runes:
//
//	[\x20-\x7E--[$!*,]]{1,1000}
//
// An empty field has no token: it shows as two adjacent COMMA tokens.
func fieldRule() bytescan.Rule {
	return core.RuneRunMax(fieldByte, maxRun)
}

// checksumRule returns a rule for the checksum field: an asterisk and two
// hexadecimal digits. The scanner does not compute or verify the checksum.
//
//	\*[0-9A-Fa-f]{2}
func checksumRule() bytescan.Rule {
	return core.Sequence(core.Literal("*"), core.Repeat(core.Rune(core.ASCIIHexDigit), 2, 2))
}

// eolRule returns a rule for the end of a sentence: carriage return and line
// feed, or a line feed alone, as logs written on Unix often have. It is not
// base.Newline, which also takes a carriage return alone: NMEA 0183 never
// ends a sentence with one.
//
//	\r?\n
func eolRule() bytescan.Rule {
	return core.Choice(core.Literal("\r\n"), core.Literal("\n"))
}
