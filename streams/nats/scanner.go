package main

import (
	"io"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/bytesource"
	"github.com/xiam/bytescan/rules/core"
)

// The token types. The scanners emit every type but PAYLOAD and HEADERS, which
// the driver reads from the cursor by the sizes of a control line.
const (
	op         bytescan.TokenType = "OP"
	subject    bytescan.TokenType = "SUBJECT"
	sid        bytescan.TokenType = "SID"
	replyTo    bytescan.TokenType = "REPLY_TO"
	queueGroup bytescan.TokenType = "QUEUE_GROUP"
	size       bytescan.TokenType = "SIZE"
	maxMsgs    bytescan.TokenType = "MAX_MSGS"
	jsonArg    bytescan.TokenType = "JSON"
	errorText  bytescan.TokenType = "ERROR_TEXT"
	sp         bytescan.TokenType = "SP"
	crlf       bytescan.TokenType = "CRLF"
	headers    bytescan.TokenType = "HEADERS"
	payload    bytescan.TokenType = "PAYLOAD"

	// longDigits is a digit run in the place of a size that is longer than
	// a size may be. The driver emits it as UNKNOWN.
	longDigits bytescan.TokenType = "LONG_DIGITS"
)

const (
	// maxRun is the most runes in one token that runs to a delimiter. A
	// longer run is emitted as several tokens of the same type in a row.
	maxRun = 1000

	// maxSizeDigits is the most digits of a SIZE. 10 digits hold every size
	// that a server allows, and a longer digit run is not read as a size.
	maxSizeDigits = 10

	// maxPiece is the most bytes in one PAYLOAD or HEADERS token. A longer
	// payload is emitted as several tokens in a row, so memory does not
	// follow a declared size, which may be corrupt.
	maxPiece = 64 << 10

	// tokenGuard is the per-token byte bound of every scanner: a little above
	// the most that a rule reads for one token, lookahead included. The
	// bound counts the UTF-8 length of each rune, and the byte adapter
	// returns a byte of 0x80 or above as a rune of two UTF-8 bytes, so a
	// piece of 1000 such bytes counts as 2000. The longest reads are a field
	// piece of 1000 such bytes with the lookahead of queueRule (2000 + 40
	// blanks + 2 = 2042 bytes), and a size with the lookahead of sizePairRule
	// (10 + 1000 + 10 + 1000 + 2 = 2022 ASCII bytes).
	tokenGuard = 2048

	// maxQueueBlanks is the most blanks that the lookahead of queueRule reads
	// after a field. It keeps that rule's read under tokenGuard when the field
	// is 1000 bytes of 0x80 or above.
	maxQueueBlanks = 40
)

// The byte classes of the rules. The byte adapter returns each byte as one
// rune from U+0000 to U+00FF.
var (
	// blank separates the fields of a control line: a space or a tab.
	blank = core.RuneSet(" \t")

	// digit is a decimal digit of a size.
	digit = core.ASCIIDigit

	// fieldByte is a byte of a subject, a reply subject, a queue group or a
	// subscription ID: anything but a blank, CR or LF.
	fieldByte = bytesource.AnyByte.And(core.RuneSet(" \t\r\n").Not())

	// textByte is a byte of the JSON of INFO and CONNECT, or of the text of
	// -ERR: anything but CR or LF.
	textByte = bytesource.AnyByte.And(core.RuneSet("\r\n").Not())
)

// newScanner returns a NATS client-protocol scanner that reads r: a driver over
// one byte cursor, with the per-token guard on every scanner. The size of a
// payload counts bytes, so the cursor must return one rune per byte: on a
// UTF-8 cursor a multi-byte sequence in a payload would count as one rune and
// misframe the stream.
func newScanner(r io.Reader) *driver {
	return newDriver(bytesource.NewCursor(r), bytescan.WithMaxTokenBytes(tokenGuard))
}

// newOpScanner returns the scanner at the start of a control line. It lexes the
// operation and an empty line's CRLF, which is also the CRLF after a
// payload. Any other byte is UNKNOWN, so garbage before a control line lexes
// one UNKNOWN per byte up to the next operation.
//
// No two rules match the same text.
func newOpScanner(cur bytescan.Cursor, opts ...bytescan.Option) *bytescan.Scanner {
	lx := bytescan.NewWithCursorOptions(cur, opts...)
	lx.MustAddRule(op, opRule)
	lx.MustAddRule(crlf, crlfRule)
	return lx
}

// newSepScanner returns the scanner between an operation and its first field,
// and after the last field of a line: blanks and the CRLF. Any other byte is
// UNKNOWN. No two rules match the same text.
func newSepScanner(cur bytescan.Cursor, opts ...bytescan.Option) *bytescan.Scanner {
	lx := bytescan.NewWithCursorOptions(cur, opts...)
	lx.MustAddRule(sp, blankRule)
	lx.MustAddRule(crlf, crlfRule)
	return lx
}

// newFieldScanner returns the scanner of a field of type typ that holds any byte
// but a blank, CR or LF: SUBJECT, SID, REPLY_TO or QUEUE_GROUP. It lexes every
// piece of the field, the blanks after it and the CRLF. No two rules match the
// same text.
func newFieldScanner(cur bytescan.Cursor, typ bytescan.TokenType, opts ...bytescan.Option) *bytescan.Scanner {
	lx := bytescan.NewWithCursorOptions(cur, opts...)
	lx.MustAddRule(typ, fieldRule)
	lx.MustAddRule(sp, blankRule)
	lx.MustAddRule(crlf, crlfRule)
	return lx
}

// newSizeScanner returns the scanner of a size field. A run of at most 10 digits
// is a SIZE, a longer one is LONG_DIGITS (emitted as UNKNOWN), and the
// longest match decides between them. No two rules match the same text.
func newSizeScanner(cur bytescan.Cursor, opts ...bytescan.Option) *bytescan.Scanner {
	lx := bytescan.NewWithCursorOptions(cur, opts...)
	lx.MustAddRule(size, sizeRule)
	lx.MustAddRule(longDigits, longDigitsRule)
	lx.MustAddRule(sp, blankRule)
	lx.MustAddRule(crlf, crlfRule)
	return lx
}

// newLongDigitsScanner returns the scanner after a LONG_DIGITS piece: the rest of
// the digit run is LONG_DIGITS too, in pieces of at most 1000 digits, however
// short the last piece is. No two rules match the same text.
func newLongDigitsScanner(cur bytescan.Cursor, opts ...bytescan.Option) *bytescan.Scanner {
	lx := bytescan.NewWithCursorOptions(cur, opts...)
	lx.MustAddRule(longDigits, digitsRule)
	lx.MustAddRule(sp, blankRule)
	lx.MustAddRule(crlf, crlfRule)
	return lx
}

// newMaxMsgsScanner returns the scanner of the optional count of UNSUB. No two
// rules match the same text.
func newMaxMsgsScanner(cur bytescan.Cursor, opts ...bytescan.Option) *bytescan.Scanner {
	lx := bytescan.NewWithCursorOptions(cur, opts...)
	lx.MustAddRule(maxMsgs, digitsRule)
	lx.MustAddRule(sp, blankRule)
	lx.MustAddRule(crlf, crlfRule)
	return lx
}

// newOptionalReplyScanner returns the scanner of the field after which a size
// may follow: the optional reply subject of PUB and MSG (last is true), or of
// HPUB and HMSG (last is false), or else the first size.
//
// A size is a run of at most 10 digits that the rest of the line confirms:
// the CRLF when last is true, or blanks, a second size and the CRLF when last
// is false, with any blanks before the CRLF. Any other field is a REPLY_TO,
// so a reply subject of at most 10 digits is still one. A run of more than 10
// digits is LONG_DIGITS.
//
// The order breaks two ties: SIZE before REPLY_TO, because a confirmed size
// is also a field; LONG_DIGITS before REPLY_TO, because a long digit run is
// also a field.
func newOptionalReplyScanner(cur bytescan.Cursor, last bool, opts ...bytescan.Option) *bytescan.Scanner {
	lx := bytescan.NewWithCursorOptions(cur, opts...)
	if last {
		lx.MustAddRule(size, sizeLastRule)
	} else {
		lx.MustAddRule(size, sizePairRule)
	}
	lx.MustAddRule(longDigits, longDigitsRule)
	lx.MustAddRule(replyTo, fieldRule)
	lx.MustAddRule(sp, blankRule)
	lx.MustAddRule(crlf, crlfRule)
	return lx
}

// newOptionalQueueScanner returns the scanner of the second field of SUB: the
// optional queue group, or else the subscription ID. A field that at most 40
// blanks and another field follow is a QUEUE_GROUP; any other is the SID, so
// a long SID stays a SID in every piece.
//
// The order breaks one tie: QUEUE_GROUP before SID, because a confirmed queue
// group is also a field.
func newOptionalQueueScanner(cur bytescan.Cursor, opts ...bytescan.Option) *bytescan.Scanner {
	lx := bytescan.NewWithCursorOptions(cur, opts...)
	lx.MustAddRule(queueGroup, queueRule)
	lx.MustAddRule(sid, fieldRule)
	lx.MustAddRule(sp, blankRule)
	lx.MustAddRule(crlf, crlfRule)
	return lx
}

// newTextScanner returns the scanner of the argument of INFO and CONNECT (JSON)
// or of -ERR (ERROR_TEXT), which runs to the end of the line. When first is
// true it lexes the first piece, which starts after the blanks; otherwise it
// lexes the next pieces, which may start with a blank. No two rules match the
// same text.
func newTextScanner(cur bytescan.Cursor, typ bytescan.TokenType, first bool, opts ...bytescan.Option) *bytescan.Scanner {
	lx := bytescan.NewWithCursorOptions(cur, opts...)
	if first {
		lx.MustAddRule(typ, textFirstRule)
		lx.MustAddRule(sp, blankRule)
	} else {
		lx.MustAddRule(typ, textNextRule)
	}
	lx.MustAddRule(crlf, crlfRule)
	return lx
}

// The rules, built once and shared by every scanner: a rule value keeps no
// state between tokens, and a core.Repeat of up to 1000 counts is large to
// build (about 120 KB for a run of one class). A run of one class is
// core.RuneRunMax, whose cost does not grow with the count.
var (
	// opRule is the rule for the operations of the client protocol, in any
	// case, as the server accepts them:
	//
	//	(?i)INFO|CONNECT|PUB|HPUB|SUB|UNSUB|MSG|HMSG|PING|PONG|\+OK|-ERR
	opRule = core.Choice(
		core.LiteralIgnoreCase("INFO"), core.LiteralIgnoreCase("CONNECT"),
		core.LiteralIgnoreCase("PUB"), core.LiteralIgnoreCase("HPUB"),
		core.LiteralIgnoreCase("SUB"), core.LiteralIgnoreCase("UNSUB"),
		core.LiteralIgnoreCase("MSG"), core.LiteralIgnoreCase("HMSG"),
		core.LiteralIgnoreCase("PING"), core.LiteralIgnoreCase("PONG"),
		core.LiteralIgnoreCase("+OK"), core.LiteralIgnoreCase("-ERR"),
	)

	// crlfRule is the end of a control line: \r\n.
	crlfRule = core.Literal("\r\n")

	// blankRule is the blanks between fields, in pieces of at most 1000:
	// [ \t]{1,1000}.
	blankRule = core.RuneRunMax(blank, maxRun)

	// fieldRule is one piece of a field: [^ \t\r\n]{1,1000}.
	fieldRule = core.RuneRunMax(fieldByte, maxRun)

	// sizeRule is a size, at most 10 digits: [0-9]{1,10}.
	sizeRule = core.RuneRunMax(digit, maxSizeDigits)

	// longDigitsRule is the first piece of a digit run that is too long for
	// a size: [0-9]{11,1000}.
	longDigitsRule = core.Repeat(core.Rune(digit), maxSizeDigits+1, maxRun)

	// digitsRule is one piece of a digit run: [0-9]{1,1000}. It lexes the
	// rest of a LONG_DIGITS run and MAX_MSGS.
	digitsRule = core.RuneRunMax(digit, maxRun)

	// lineEndRule is the end of a line after its last field, with any blanks
	// before it: [ \t]{0,1000}\r\n. The server splits the arguments of a
	// line on blanks, so it accepts blanks before the CRLF.
	lineEndRule = core.Sequence(core.Optional(blankRule), crlfRule)

	// sizeLastRule is a size that ends the line:
	// [0-9]{1,10}(?=[ \t]{0,1000}\r\n).
	sizeLastRule = core.FollowedBy(sizeRule, lineEndRule)

	// sizePairRule is a size that a second size and the end of the line
	// follow: [0-9]{1,10}(?=[ \t]{1,1000}[0-9]{1,10}[ \t]{0,1000}\r\n).
	sizePairRule = core.FollowedBy(sizeRule, core.Sequence(blankRule, sizeRule, lineEndRule))

	// queueRule is a field that at most 40 blanks and another field follow:
	// [^ \t\r\n]{1,1000}(?=[ \t]{1,40}[^ \t\r\n]).
	queueRule = core.FollowedBy(fieldRule, core.Sequence(core.RuneRunMax(blank, maxQueueBlanks), core.Rune(fieldByte)))

	// textFirstRule is the first piece of a JSON or ERROR_TEXT, which starts
	// after the blanks: [^ \t\r\n][^\r\n]{0,999}.
	textFirstRule = core.Sequence(core.Rune(textByte.And(blank.Not())), core.Optional(core.RuneRunMax(textByte, maxRun-1)))

	// textNextRule is a next piece of a JSON or ERROR_TEXT: [^\r\n]{1,1000}.
	textNextRule = core.RuneRunMax(textByte, maxRun)
)
