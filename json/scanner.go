package main

import (
	"bufio"
	"io"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/rules/base"
	"github.com/xiam/bytescan/rules/core"
)

// The token types. The structural types and the literal names are those of
// RFC 8259, section 2 and section 3.
const (
	beginArray     bytescan.TokenType = "BEGIN_ARRAY"
	beginObject    bytescan.TokenType = "BEGIN_OBJECT"
	endArray       bytescan.TokenType = "END_ARRAY"
	endObject      bytescan.TokenType = "END_OBJECT"
	nameSeparator  bytescan.TokenType = "NAME_SEPARATOR"
	valueSeparator bytescan.TokenType = "VALUE_SEPARATOR"
	trueLit        bytescan.TokenType = "TRUE"
	falseLit       bytescan.TokenType = "FALSE"
	nullLit        bytescan.TokenType = "NULL"
	stringLit      bytescan.TokenType = "STRING"
	number         bytescan.TokenType = "NUMBER"
	whitespace     bytescan.TokenType = "WHITESPACE"
)

// newScanner returns a JSON scanner that reads r.
//
// No two rules match the same text, so the registration order does not decide
// any token here; it follows the order of RFC 8259.
func newScanner(r io.Reader) *bytescan.Scanner {
	lx := bytescan.New(bufio.NewReader(r))

	lx.MustAddRule(beginArray, core.Literal("["))
	lx.MustAddRule(beginObject, core.Literal("{"))
	lx.MustAddRule(endArray, core.Literal("]"))
	lx.MustAddRule(endObject, core.Literal("}"))
	lx.MustAddRule(nameSeparator, core.Literal(":"))
	lx.MustAddRule(valueSeparator, core.Literal(","))

	lx.MustAddRule(trueLit, core.Literal("true"))
	lx.MustAddRule(falseLit, core.Literal("false"))
	lx.MustAddRule(nullLit, core.Literal("null"))

	lx.MustAddRule(stringLit, stringRule())
	lx.MustAddRule(number, numberRule())

	// ws = *( %x20 / %x09 / %x0A / %x0D ). base.Whitespace also takes \v and
	// \f and leaves out line breaks, so the JSON set is composed here.
	lx.MustAddRule(whitespace, core.RuneRun(core.RuneSet(" \t\n\r")))

	return lx
}

// stringRule returns a rule for an RFC 8259 string (section 7):
//
//	"(?:[^"\\\x00-\x1F]|\\["\\/bfnrt]|\\u[0-9A-Fa-f]{4})*"
//
// The shape of \uXXXX is checked; whether a surrogate is paired is not.
// base.Quoted would accept any escaped rune, so the string is a
// base.QuotedWith with the escape grammar of RFC 8259.
func stringRule() bytescan.Rule {
	unescaped := core.RuneSet(`"\`).Or(core.RuneRange(0x00, 0x1F)).Not()
	escape := core.Sequence(
		core.Literal(`\`),
		core.Choice(
			core.Rune(core.RuneSet(`"\/bfnrt`)),
			core.Sequence(core.Literal("u"), core.Repeat(core.Rune(core.ASCIIHexDigit), 4, 4)),
		),
	)
	return base.QuotedWith('"', unescaped, escape)
}

// numberRule returns a rule for an RFC 8259 number (section 6):
//
//	-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?
//
// The grammar is strict, and the longest match decides the rest: "01" lexes as
// the numbers "0" and "1", and "1." as the number "1" and an UNKNOWN ".".
// base.Integer and base.Float with base.NoLeadingZeros are the forms without
// and with a fraction or an exponent; no base number rule matches a sign, so
// the minus sign is composed here.
func numberRule() bytescan.Rule {
	noLeadingZeros := base.NoLeadingZeros()
	return core.Sequence(
		core.Optional(core.Literal("-")),
		core.Choice(base.Integer(noLeadingZeros), base.Float(noLeadingZeros)),
	)
}
