package main

import (
	"io"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan-examples/streams/sse/ssedriver"
	"github.com/xiam/bytescan/rules/core"
	"github.com/xiam/textreader"
)

// The token types of the JSON layer. The structural types are named after
// RFC 8259, section 2, as in the json example, with the JSON_ prefix so
// that they do not collide with the SSE types of ssedriver.
const (
	jsonBeginArray     bytescan.TokenType = "JSON_BEGIN_ARRAY"
	jsonEndArray       bytescan.TokenType = "JSON_END_ARRAY"
	jsonBeginObject    bytescan.TokenType = "JSON_BEGIN_OBJECT"
	jsonEndObject      bytescan.TokenType = "JSON_END_OBJECT"
	jsonNameSeparator  bytescan.TokenType = "JSON_NAME_SEPARATOR"
	jsonValueSeparator bytescan.TokenType = "JSON_VALUE_SEPARATOR"
	jsonTrue           bytescan.TokenType = "JSON_TRUE"
	jsonFalse          bytescan.TokenType = "JSON_FALSE"
	jsonNull           bytescan.TokenType = "JSON_NULL"
	jsonNumber         bytescan.TokenType = "JSON_NUMBER"
	jsonWhitespace     bytescan.TokenType = "JSON_WHITESPACE"
	jsonEOL            bytescan.TokenType = "JSON_EOL"
	jsonQuote          bytescan.TokenType = "JSON_QUOTE"
	jsonStringText     bytescan.TokenType = "JSON_STRING_TEXT"
	jsonEscape         bytescan.TokenType = "JSON_ESCAPE"
	unknown                               = bytescan.TokenTypeUnknown
)

// maxRun is the most runes in one JSON_STRING_TEXT or JSON_WHITESPACE token.
// A longer run is emitted as several tokens of the same type in a row.
const maxRun = 1000

// maxDigits is the most digits in each of the three digit runs of a
// JSON_NUMBER (integer, fraction, exponent). The longest number, a sign, 330
// digits, a point, 330 digits, an exponent letter, a sign and 330 digits, is
// 994 runes, so every JSON token is at most maxRun runes. A bound on each
// digit run alone at maxRun would allow one number of about 3,000 runes. A
// longer number is emitted as several JSON_NUMBER tokens in a row.
const maxDigits = 330

// maxTokenBytes bounds every token of every scanner of both layers: 1000 runes
// of at most 4 bytes, plus the look-ahead rune, is under 4,096 bytes. A rule
// that scanned further before it decides would fail Next with the limit
// error.
const maxTokenBytes = 4096

// The rules of the JSON layer, built once and shared by the scanners.
var (
	// A JSON number (RFC 8259, section 6) with each digit run bounded:
	//
	//	-?(?:0|[1-9][0-9]{0,329})(?:\.[0-9]{1,330})?(?:[eE][+-]?[0-9]{1,330})?
	numberRule = func() bytescan.Rule {
		digits := core.RuneRunMax(core.ASCIIDigit, maxDigits)
		integer := core.Choice(
			core.Literal("0"),
			core.Sequence(core.Rune(core.RuneRange('1', '9')), core.Optional(core.RuneRunMax(core.ASCIIDigit, maxDigits-1))),
		)
		fraction := core.Sequence(core.Literal("."), digits)
		exponent := core.Sequence(core.Rune(core.RuneSet("eE")), core.Optional(core.Rune(core.RuneSet("+-"))), digits)
		return core.Sequence(core.Optional(core.Literal("-")), integer, core.Optional(fraction), core.Optional(exponent))
	}()

	// JSON whitespace without the line feed (RFC 8259, section 2), in pieces:
	// [ \t\r]{1,1000}
	whitespaceRule = core.RuneRunMax(core.RuneSet(" \t\r"), maxRun)

	// The unescaped characters of a string (RFC 8259, section 7), in pieces:
	// [^"\\\x00-\x1F]{1,1000}
	stringTextRule = core.RuneRunMax(core.RuneSet(`"\`).Or(core.RuneRange(0x00, 0x1F)).Not(), maxRun)

	// One escape (RFC 8259, section 7): \\["\\/bfnrt] or \\u[0-9A-Fa-f]{4}.
	// The shape of \uXXXX is checked; whether a surrogate is paired is not.
	escapeRule = core.Sequence(
		core.Literal(`\`),
		core.Choice(
			core.Rune(core.RuneSet(`"\/bfnrt`)),
			core.Sequence(core.Literal("u"), core.Repeat(core.Rune(core.ASCIIHexDigit), 4, 4)),
		),
	)

	quoteRule = core.Literal(`"`)
	eolRule   = core.Literal("\n")
)

// newScanner returns the piped scanner that reads r: the SSE driver of ssedriver
// over the stream, the pipe that feeds the text of its data values to a
// second cursor, and the JSON mode driver over that cursor. Every scanner of
// both layers is bounded by maxTokenBytes.
func newScanner(r io.Reader) *driver {
	return newDriver(r, bytescan.WithMaxTokenBytes(maxTokenBytes))
}

// newDriver returns the piped scanner that reads r, with every scanner of both
// layers built with opts.
func newDriver(r io.Reader, opts ...bytescan.Option) *driver {
	p := newPipe(ssedriver.New(r, opts...))
	return &driver{
		pipe:  p,
		inner: newJSONDriver(textreader.New(p), opts...),
	}
}

// newValueScanner returns the scanner of value mode: the structural characters,
// the literals, numbers, whitespace, the line feed, and the quote that opens a
// string. No two rules match the same text.
func newValueScanner(cur bytescan.Cursor, opts ...bytescan.Option) *bytescan.Scanner {
	lx := bytescan.NewWithCursorOptions(cur, opts...)
	lx.MustAddRule(jsonBeginArray, core.Literal("["))
	lx.MustAddRule(jsonBeginObject, core.Literal("{"))
	lx.MustAddRule(jsonEndArray, core.Literal("]"))
	lx.MustAddRule(jsonEndObject, core.Literal("}"))
	lx.MustAddRule(jsonNameSeparator, core.Literal(":"))
	lx.MustAddRule(jsonValueSeparator, core.Literal(","))
	lx.MustAddRule(jsonTrue, core.Literal("true"))
	lx.MustAddRule(jsonFalse, core.Literal("false"))
	lx.MustAddRule(jsonNull, core.Literal("null"))
	lx.MustAddRule(jsonNumber, numberRule)
	lx.MustAddRule(jsonWhitespace, whitespaceRule)
	lx.MustAddRule(jsonEOL, eolRule)
	lx.MustAddRule(jsonQuote, quoteRule)
	return lx
}

// newStringScanner returns the scanner of string mode: unescaped text, one
// escape, the closing quote, and the line feed that ends an unterminated
// string. No two rules match the same text.
func newStringScanner(cur bytescan.Cursor, opts ...bytescan.Option) *bytescan.Scanner {
	lx := bytescan.NewWithCursorOptions(cur, opts...)
	lx.MustAddRule(jsonStringText, stringTextRule)
	lx.MustAddRule(jsonEscape, escapeRule)
	lx.MustAddRule(jsonQuote, quoteRule)
	lx.MustAddRule(jsonEOL, eolRule)
	return lx
}
