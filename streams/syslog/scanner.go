package main

import (
	"io"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan-examples/internal/lexdump"
	"github.com/xiam/bytescan-examples/internal/streamkit"
	"github.com/xiam/bytescan/bytesource"
	"github.com/xiam/bytescan/rules/core"
)

// The token types. The names follow the ABNF of RFC 5424 section 6 and of
// RFC 6587 section 3.4.
const (
	msgLen         bytescan.TokenType = "MSG_LEN"
	sp             bytescan.TokenType = "SP"
	lf             bytescan.TokenType = "LF"
	pri            bytescan.TokenType = "PRI"
	version        bytescan.TokenType = "VERSION"
	timestamp      bytescan.TokenType = "TIMESTAMP"
	hostname       bytescan.TokenType = "HOSTNAME"
	appName        bytescan.TokenType = "APP_NAME"
	procID         bytescan.TokenType = "PROCID"
	msgID          bytescan.TokenType = "MSGID"
	structuredData bytescan.TokenType = "STRUCTURED_DATA"
	sdElementOpen  bytescan.TokenType = "SD_ELEMENT_OPEN"
	sdID           bytescan.TokenType = "SD_ID"
	sdParamName    bytescan.TokenType = "SD_PARAM_NAME"
	sdEquals       bytescan.TokenType = "SD_EQUALS"
	sdQuote        bytescan.TokenType = "SD_QUOTE"
	sdParamValue   bytescan.TokenType = "SD_PARAM_VALUE"
	sdElementClose bytescan.TokenType = "SD_ELEMENT_CLOSE"
	msg            bytescan.TokenType = "MSG"
	unknown                           = bytescan.TokenTypeUnknown
	digitRun       bytescan.TokenType = "DIGIT_RUN" // never emitted: the driver turns it into UNKNOWN
)

const (
	// maxRun is the most runes, here bytes, in one token of a run that ends
	// at a delimiter. A longer run is several tokens of the same type.
	maxRun = 1000

	// maxPiece is the most bytes in one MSG token that the driver reads by
	// the length of an octet-counted frame. A longer MSG is several tokens.
	maxPiece = 64 << 10

	// maxLenDigits is the most digits of MSG_LEN. Ten digits hold every
	// length up to 9,999,999,999 bytes.
	maxLenDigits = 10
)

// Classes of bytes. On the byte adapter one rune is one byte.
var (
	digit = core.Rune(core.ASCIIDigit)

	// printUSASCII is PRINTUSASCII of RFC 5424: %d33-126.
	printUSASCII = core.RuneRange(33, 126)

	// sdNameByte is a byte of SD-NAME: PRINTUSASCII except '=', SP, ']' and
	// '"'.
	sdNameByte = printUSASCII.And(core.RuneSet(`="]`).Not())

	// valueByte is a byte of PARAM-VALUE that needs no escape here: any byte
	// but '"', '\' and LF.
	valueByte = bytesource.AnyByte.And(core.RuneSet("\"\\\n").Not())

	// msgByte is a byte of a newline-ended MSG: any byte but LF.
	msgByte = bytesource.AnyByte.And(core.RuneSet("\n").Not())
)

// dumper writes one line per token. MSG is in the hex view of bytesource,
// because it may hold any byte; every other token is in the quoted view of
// streamkit.Quote, whole.
var dumper = lexdump.Dumper{Format: format}

func format(l *bytescan.Token) string {
	if l.Type() == msg {
		return bytesource.Hex(l)
	}
	return streamkit.Quote(l, 0)
}

// tokenGuard is the per-token byte bound of every scanner:
// streamtest.ByteGuard, a little above the largest bounded token in bytes.
// The longest token of a rule is a piece of maxRun bytes, so the guard never
// fires on any input.
const tokenGuard = 1024

// newScanner returns a syslog scanner that reads r: a driver over one byte
// cursor, with the per-token guard on every scanner.
func newScanner(r io.Reader) *driver {
	return newDriver(bytesource.NewCursor(r), bytescan.WithMaxTokenBytes(tokenGuard))
}

// newFrameScanner returns the scanner of a frame start. RFC 6587 section 3.4.1:
//
//	MSG-LEN  = NONZERO-DIGIT *DIGIT
//
// MSG_LEN is 1 to 10 digits followed by SP; it does not include the SP. A run
// of digits that is not a MSG_LEN is DIGIT_RUN, at most maxRun digits, which
// the driver emits as UNKNOWN.
//
// Ties: MSG_LEN and DIGIT_RUN match the same digits when MSG_LEN matches, so
// MSG_LEN is registered first.
func newFrameScanner(cur bytescan.Cursor, opts ...bytescan.Option) *bytescan.Scanner {
	lx := bytescan.NewWithCursorOptions(cur, opts...)
	lx.MustAddRule(msgLen, core.FollowedBy(core.RuneRunMax(core.ASCIIDigit, maxLenDigits), core.Literal(" ")))
	lx.MustAddRule(digitRun, core.RuneRunMax(core.ASCIIDigit, maxRun))
	lx.MustAddRule(sp, core.Literal(" "))
	return lx
}

// newPRIScanner returns the scanner of the start of a message:
//
//	PRI = "<" PRIVAL ">"   ; PRIVAL is 1 to 3 digits
//
// and LF, which ends a newline-ended message. No two rules match the same
// text.
func newPRIScanner(cur bytescan.Cursor, opts ...bytescan.Option) *bytescan.Scanner {
	lx := bytescan.NewWithCursorOptions(cur, opts...)
	lx.MustAddRule(pri, core.Sequence(core.Literal("<"), core.RuneRunMax(core.ASCIIDigit, 3), core.Literal(">")))
	lx.MustAddRule(lf, core.Literal("\n"))
	return lx
}

// newFieldScanner returns the scanner of one header field after PRI: the field
// rule, SP, which ends the field, and LF. No two rules match the same text.
func newFieldScanner(cur bytescan.Cursor, typ bytescan.TokenType, rule bytescan.Rule, opts ...bytescan.Option) *bytescan.Scanner {
	lx := bytescan.NewWithCursorOptions(cur, opts...)
	lx.MustAddRule(typ, rule)
	lx.MustAddRule(sp, core.Literal(" "))
	lx.MustAddRule(lf, core.Literal("\n"))
	return lx
}

// versionRule is VERSION, 1 to 3 digits. RFC 5424 asks for a non-zero first
// digit; the scanner does not judge it.
func versionRule() bytescan.Rule {
	return core.RuneRunMax(core.ASCIIDigit, 3)
}

// timestampRule is TIMESTAMP of RFC 5424 section 6.2.3, the RFC 3339 shape
// or the nil value:
//
//	TIMESTAMP    = NILVALUE / FULL-DATE "T" FULL-TIME
//	FULL-DATE    = 4DIGIT "-" 2DIGIT "-" 2DIGIT
//	FULL-TIME    = PARTIAL-TIME TIME-OFFSET
//	PARTIAL-TIME = 2DIGIT ":" 2DIGIT ":" 2DIGIT ["." 1*6DIGIT]
//	TIME-OFFSET  = "Z" / ("+" / "-") 2DIGIT ":" 2DIGIT
//
// The shape is bounded: it reads at most 32 bytes. It does not check that a
// date or time exists.
func timestampRule() bytescan.Rule {
	d2 := core.Repeat(digit, 2, 2)
	return core.Choice(
		core.Literal("-"),
		core.Sequence(
			core.Repeat(digit, 4, 4), core.Literal("-"), d2, core.Literal("-"), d2,
			core.Literal("T"),
			d2, core.Literal(":"), d2, core.Literal(":"), d2,
			core.Optional(core.Sequence(core.Literal("."), core.RuneRunMax(core.ASCIIDigit, 6))),
			core.Choice(
				core.Literal("Z"),
				core.Sequence(core.Rune(core.RuneSet("+-")), d2, core.Literal(":"), d2),
			),
		),
	)
}

// tokenRule is a piece of HOSTNAME, APP-NAME, PROCID or MSGID: 1 to maxRun
// bytes of PRINTUSASCII. The nil value "-" is a token of one byte. RFC 5424
// limits each field to 32 to 255 bytes; the scanner does not, and a longer run
// is several tokens.
func tokenRule() bytescan.Rule {
	return core.RuneRunMax(printUSASCII, maxRun)
}

// newSDScanner returns the scanner of STRUCTURED-DATA between its elements
// (RFC 5424 section 6.3):
//
//	STRUCTURED-DATA = NILVALUE / 1*SD-ELEMENT
//
// It lexes the nil value "-" as STRUCTURED_DATA, the "[" that opens an
// element, the SP before MSG, and LF. No two rules match the same text.
func newSDScanner(cur bytescan.Cursor, opts ...bytescan.Option) *bytescan.Scanner {
	lx := bytescan.NewWithCursorOptions(cur, opts...)
	lx.MustAddRule(sdElementOpen, core.Literal("["))
	lx.MustAddRule(structuredData, core.Literal("-"))
	lx.MustAddRule(sp, core.Literal(" "))
	lx.MustAddRule(lf, core.Literal("\n"))
	return lx
}

// newElementScanner returns the scanner of the inside of an SD-ELEMENT:
//
//	SD-ELEMENT = "[" SD-ID *(SP SD-PARAM) "]"
//	SD-PARAM   = PARAM-NAME "=" %d34 PARAM-VALUE %d34
//	SD-ID      = SD-NAME
//	PARAM-NAME = SD-NAME
//	SD-NAME    = 1*32PRINTUSASCII ; except '=', SP, ']', %d34 (")
//
// SD_PARAM_NAME is an SD-NAME followed by "="; any other SD-NAME is SD_ID. A
// name runs to a delimiter, so it is a piece of 1 to maxRun bytes.
//
// Ties: SD_PARAM_NAME and SD_ID match the same name when "=" follows it, so
// SD_PARAM_NAME is registered first.
func newElementScanner(cur bytescan.Cursor, opts ...bytescan.Option) *bytescan.Scanner {
	name := core.RuneRunMax(sdNameByte, maxRun)
	lx := bytescan.NewWithCursorOptions(cur, opts...)
	lx.MustAddRule(sdParamName, core.FollowedBy(name, core.Literal("=")))
	lx.MustAddRule(sdID, name)
	lx.MustAddRule(sdEquals, core.Literal("="))
	lx.MustAddRule(sdQuote, core.Literal(`"`))
	lx.MustAddRule(sp, core.Literal(" "))
	lx.MustAddRule(sdElementClose, core.Literal("]"))
	lx.MustAddRule(lf, core.Literal("\n"))
	return lx
}

// newValueScanner returns the scanner of a PARAM-VALUE between its quotes. RFC
// 5424 section 6.3.3: '"', '\' and ']' are escaped with '\'. A value piece is
//
//	([^"\\\n] | \\[^\n]){1,500}
//
// so an escape pair is never split and a piece has at most maxRun bytes. A
// value is UTF-8, which the scanner does not check. The closing '"' is
// SD_QUOTE. No two rules match the same text.
func newValueScanner(cur bytescan.Cursor, opts ...bytescan.Option) *bytescan.Scanner {
	unit := core.Choice(
		core.Rune(valueByte),
		core.Sequence(core.Literal(`\`), core.Rune(msgByte)),
	)
	lx := bytescan.NewWithCursorOptions(cur, opts...)
	lx.MustAddRule(sdParamValue, core.Repeat(unit, 1, maxRun/2))
	lx.MustAddRule(sdQuote, core.Literal(`"`))
	lx.MustAddRule(lf, core.Literal("\n"))
	return lx
}

// newMsgScanner returns the scanner of MSG in a newline-ended frame: pieces of 1
// to maxRun bytes other than LF, and the LF that ends the frame. A BOM is part
// of MSG. No two rules match the same text.
func newMsgScanner(cur bytescan.Cursor, opts ...bytescan.Option) *bytescan.Scanner {
	lx := bytescan.NewWithCursorOptions(cur, opts...)
	lx.MustAddRule(msg, core.RuneRunMax(msgByte, maxRun))
	lx.MustAddRule(lf, core.Literal("\n"))
	return lx
}
