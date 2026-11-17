package ssedriver

import (
	"bufio"
	"io"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/rules/base"
	"github.com/xiam/bytescan/rules/core"
	"github.com/xiam/textreader"
)

// The token types. The names of the fields that the WHATWG parsing rules act
// on (event, data, id, retry) get their own types; any other field name is
// FIELD.
const (
	BOM      bytescan.TokenType = "BOM"
	Field    bytescan.TokenType = "FIELD"
	Event    bytescan.TokenType = "EVENT"
	Data     bytescan.TokenType = "DATA"
	ID       bytescan.TokenType = "ID"
	Retry    bytescan.TokenType = "RETRY"
	Colon    bytescan.TokenType = "COLON"
	Space    bytescan.TokenType = "SPACE"
	Value    bytescan.TokenType = "VALUE"
	Comment  bytescan.TokenType = "COMMENT"
	EOL      bytescan.TokenType = "EOL"
	EventEnd bytescan.TokenType = "EVENT_END"
)

// MaxRun is the most runes in one FIELD, VALUE or COMMENT token. These run to
// a line end or a colon, so a stream that has lost its line endings would grow
// one token without bound; a longer run is emitted as several tokens of the
// same type in a row. 1000 is the run bound of every stream example.
const MaxRun = 1000

// The character classes of the WHATWG stream grammar ("Parsing an event
// stream"):
//
//	any-char  = %x0000-0009 / %x000B-000C / %x000E-10FFFF
//	name-char = %x0000-0009 / %x000B-000C / %x000E-0039 / %x003B-10FFFF
//
// An invalid UTF-8 byte reaches the rules as U+FFFD, which is an any-char,
// as the UTF-8 decode the specification requires makes it.
var (
	anyChar  = core.RuneSet("\r\n").Not()
	nameChar = core.RuneSet("\r\n:").Not()
)

// The rules, built once and shared by the mode scanners (a core rule keeps no
// state, so one value can be registered on several scanners).
var (
	// end-of-line = ( cr lf / cr / lf )
	eolRule = base.Newline()

	// A field name in pieces: [^\r\n:]{1,1000}
	nameRule = core.RuneRunMax(nameChar, MaxRun)

	// The first piece of a field name at the start of the stream, where a
	// leading U+FEFF is the BOM: [^\r\n:\x{FEFF}][^\r\n:]{0,999}
	firstNameRule = core.Sequence(
		core.Rune(nameChar.And(core.RuneSet("\uFEFF").Not())),
		core.Optional(core.RuneRunMax(nameChar, MaxRun-1)),
	)

	// The names that the parsing rules act on. Each also matches nameRule
	// with the same length, so it is registered first; a longer name such as
	// "datax" is a FIELD by the longest match.
	knownNames = []struct {
		typ  bytescan.TokenType
		rule bytescan.Rule
	}{
		{Event, core.Literal("event")},
		{Data, core.Literal("data")},
		{ID, core.Literal("id")},
		{Retry, core.Literal("retry")},
	}

	// The first piece of a comment line: :[^\r\n]{0,999}
	commentRule = core.Sequence(core.Literal(":"), core.Optional(core.RuneRunMax(anyChar, MaxRun-1)))

	// A later piece of a value or a comment: [^\r\n]{1,1000}
	runRule = core.RuneRunMax(anyChar, MaxRun)

	// The first piece of a value right after the colon, which does not start
	// with the optional space: [^\r\n ][^\r\n]{0,999}
	firstValueRule = core.Sequence(
		core.Rune(anyChar.And(core.RuneSet(" ").Not())),
		core.Optional(core.RuneRunMax(anyChar, MaxRun-1)),
	)
)

// New returns a Server-Sent Events scanner that reads r: a mode driver over one
// cursor, with one scanner per mode. Every mode scanner is built with opts, such
// as bytescan.WithMaxTokenBytes for a per-token guard.
func New(r io.Reader, opts ...bytescan.Option) *Driver {
	return newDriver(textreader.New(bufio.NewReader(r)), opts...)
}

// newLineScanner returns the scanner for the start of a line: a field name, a
// comment, or a blank line, which ends the event. With start set it is the
// scanner for the first line of the stream, which may begin with the BOM.
//
// One tie is broken by registration order: EVENT, DATA, ID and RETRY are
// registered before FIELD, because each name also matches the FIELD rule. The
// longest match decides "datax" (FIELD). At the start of the stream the BOM
// and a field name never match the same text, because the first FIELD piece
// there does not start with U+FEFF; on a later line U+FEFF is a name-char.
func newLineScanner(cur bytescan.Cursor, start bool, opts ...bytescan.Option) *bytescan.Scanner {
	lx := bytescan.NewWithCursorOptions(cur, opts...)
	name := nameRule
	if start {
		lx.MustAddRule(BOM, core.Literal("\uFEFF"))
		name = firstNameRule
	}
	for _, k := range knownNames {
		lx.MustAddRule(k.typ, k.rule)
	}
	lx.MustAddRule(Field, name)
	lx.MustAddRule(Comment, commentRule)
	lx.MustAddRule(EventEnd, eolRule)
	return lx
}

// newNameScanner returns the scanner for the text after a field name: the colon,
// the line end of a field with no colon, or the next piece of a field name
// longer than maxRun runes. No two rules match the same text.
func newNameScanner(cur bytescan.Cursor, opts ...bytescan.Option) *bytescan.Scanner {
	lx := bytescan.NewWithCursorOptions(cur, opts...)
	lx.MustAddRule(Colon, core.Literal(":"))
	lx.MustAddRule(Field, nameRule)
	lx.MustAddRule(EOL, eolRule)
	return lx
}

// newColonScanner returns the scanner for the text right after the colon of a
// field: the one optional space, the first piece of the value, or the line
// end of an empty value. No two rules match the same text, because the first
// value piece does not start with a space.
func newColonScanner(cur bytescan.Cursor, opts ...bytescan.Option) *bytescan.Scanner {
	lx := bytescan.NewWithCursorOptions(cur, opts...)
	lx.MustAddRule(Space, core.Literal(" "))
	lx.MustAddRule(Value, firstValueRule)
	lx.MustAddRule(EOL, eolRule)
	return lx
}

// newRunScanner returns the scanner for the rest of a line whose type is already
// known: the pieces of a value (typ VALUE) or of a comment (typ COMMENT), and
// the line end. No two rules match the same text.
func newRunScanner(cur bytescan.Cursor, typ bytescan.TokenType, opts ...bytescan.Option) *bytescan.Scanner {
	lx := bytescan.NewWithCursorOptions(cur, opts...)
	lx.MustAddRule(typ, runRule)
	lx.MustAddRule(EOL, eolRule)
	return lx
}
