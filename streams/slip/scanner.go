package main

import (
	"io"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/bytesource"
	"github.com/xiam/bytescan/rules/core"
	"github.com/xiam/bytescan/rules/octet"
)

// The token types. A byte that no rule matches, such as an ESC that does not
// start an escape sequence, is bytescan.TokenTypeUnknown.
const (
	endType    bytescan.TokenType = "END"
	dataType   bytescan.TokenType = "DATA"
	escEndType bytescan.TokenType = "ESC_END"
	escEscType bytescan.TokenType = "ESC_ESC"
)

// The special bytes of RFC 1055.
const (
	end    = 0xC0 // END: the frame delimiter
	esc    = 0xDB // ESC: starts an escape sequence
	escEnd = 0xDC // ESC_END: ESC ESC_END stands for an END byte in the data
	escEsc = 0xDD // ESC_ESC: ESC ESC_ESC stands for an ESC byte in the data
)

// maxRun is the most bytes in one DATA token. A longer run of data bytes is
// lexed as several DATA tokens in a row, so a stream that has lost its END
// bytes does not grow one token without bound.
const maxRun = 1000

// tokenGuard is the per-token byte bound of the scanner: twice
// streamtest.ByteGuard, because the guard counts a byte of 0x80 or above as 2
// bytes and a DATA piece of maxRun such bytes counts 2,000. Every rule is
// bounded, so the guard never fires on any input; it turns a rule that scans
// without bound into an error instead of a growing heap.
const tokenGuard = 2048

// dataByte is the class of the bytes that a frame carries as they are: every
// byte but END and ESC.
var dataByte = bytesource.AnyByte.And(core.RuneSet(bytesource.Text("\xC0\xDB")).Not())

// newScanner returns a SLIP scanner that reads r through the byte adapter, with
// the per-token guard. It has four rules:
//
//	END      \xC0
//	DATA     [^\xC0\xDB]{1,1000}
//	ESC_END  \xDB\xDC
//	ESC_ESC  \xDB\xDD
//
// The rules start with different bytes, except the two escape sequences,
// which differ in their second byte, so no tie arises. An ESC followed by any
// other byte, or by the end of the input, matches no rule: the ESC alone is
// UNKNOWN, and the byte after it is lexed on its own, so an END after a lone
// ESC still ends the frame.
func newScanner(r io.Reader) *bytescan.Scanner {
	lx := bytescan.NewWithCursorOptions(bytesource.NewCursor(r), bytescan.WithMaxTokenBytes(tokenGuard))
	lx.MustAddRule(endType, octet.Set(end))
	lx.MustAddRule(dataType, core.RuneRunMax(dataByte, maxRun))
	lx.MustAddRule(escEndType, octet.Literal([]byte{esc, escEnd}))
	lx.MustAddRule(escEscType, octet.Literal([]byte{esc, escEsc}))
	return lx
}
