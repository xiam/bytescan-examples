package main

import (
	"bufio"
	"io"
	"strings"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/rules/base"
	"github.com/xiam/bytescan/rules/core"
	"github.com/xiam/textreader"
)

// The token types, named after the token classes of the Lua 5.4 reference
// manual, section 3.1. The code-mode scanner emits every type but the bodies and
// the closers of the long brackets, which the level scanners emit.
const (
	keyword          bytescan.TokenType = "KEYWORD"
	name             bytescan.TokenType = "NAME"
	operator         bytescan.TokenType = "OPERATOR"
	delimiter        bytescan.TokenType = "DELIMITER"
	numeral          bytescan.TokenType = "NUMERAL"
	stringLit        bytescan.TokenType = "STRING"
	comment          bytescan.TokenType = "COMMENT"
	whitespace       bytescan.TokenType = "WHITESPACE"
	newline          bytescan.TokenType = "NEWLINE"
	longStringOpen   bytescan.TokenType = "LONG_STRING_OPEN"
	longString       bytescan.TokenType = "LONG_STRING"
	longStringClose  bytescan.TokenType = "LONG_STRING_CLOSE"
	longCommentOpen  bytescan.TokenType = "LONG_COMMENT_OPEN"
	longComment      bytescan.TokenType = "LONG_COMMENT"
	longCommentClose bytescan.TokenType = "LONG_COMMENT_CLOSE"
)

// newScanner returns a Lua scanner that reads r: a mode driver over one cursor,
// with a code-mode scanner and a scanner per long-bracket level.
func newScanner(r io.Reader) *driver {
	return newCursorScanner(textreader.New(bufio.NewReader(r)))
}

// newCursorScanner returns the Lua scanner over cur.
func newCursorScanner(cur bytescan.Cursor) *driver {
	return newDriver(cur, newCodeScanner(cur))
}

// keywords are the 22 reserved words of the Lua 5.4 reference manual,
// section 3.1.
var keywords = []string{
	"and", "break", "do", "else", "elseif", "end",
	"false", "for", "function", "goto", "if", "in",
	"local", "nil", "not", "or", "repeat", "return",
	"then", "true", "until", "while",
}

// newCodeScanner returns the scanner for code mode over cur: every token of the
// Lua 5.4 reference manual, section 3.1, and the openers of the long brackets.
// The driver in driver.go switches to a level scanner after an opener.
//
// One tie is broken by registration order: KEYWORD is registered before NAME,
// because a reserved word also matches the name rule. The longest match
// decides every other overlap: "--[[" is LONG_COMMENT_OPEN, not OPERATOR "-"
// (COMMENT does not match when an opener follows "--"),
// "[[" is LONG_STRING_OPEN, not DELIMITER "[", ".5" is NUMERAL, not DELIMITER
// ".", and "..." is DELIMITER, not OPERATOR "..".
func newCodeScanner(cur bytescan.Cursor) *bytescan.Scanner {
	lx := bytescan.NewWithCursor(cur)

	lx.MustAddRule(keyword, core.Keywords(keywords...))
	lx.MustAddRule(name, base.ASCIIIdentifier())

	lx.MustAddRule(operator, core.AnyLiteral(
		"+", "-", "*", "/", "//", "%", "^", "#",
		"&", "~", "|", "<<", ">>",
		"==", "~=", "<=", ">=", "<", ">", "..",
	))
	lx.MustAddRule(delimiter, core.AnyLiteral(
		"=", "(", ")", "{", "}", "[", "]", "::",
		";", ":", ",", ".", "...",
	))

	lx.MustAddRule(numeral, numeralRule())
	lx.MustAddRule(stringLit, core.Choice(base.Quoted('"', '\\'), base.Quoted('\'', '\\')))

	lx.MustAddRule(longStringOpen, longBracket(""))
	lx.MustAddRule(longCommentOpen, longBracket("--"))
	lx.MustAddRule(comment, commentRule())

	// Lua's whitespace is space, \f, \n, \r, \t and \v. base.Whitespace takes
	// the horizontal ones and base.Newline the line breaks.
	lx.MustAddRule(whitespace, base.Whitespace())
	lx.MustAddRule(newline, base.Newline())

	return lx
}

// longBracket returns a rule for prefix followed by an opening long bracket
// (section 3.1):
//
//	prefix\[=*\[
//
// The level of the bracket is the number of "=".
func longBracket(prefix string) bytescan.Rule {
	open := core.Sequence(core.Literal("["), core.ZeroOrMore(core.Literal("=")), core.Literal("["))
	if prefix == "" {
		return open
	}
	return core.Sequence(core.Literal(prefix), open)
}

// commentRule returns a rule for a short comment (section 3.1): "--" and the
// rest of the line, when the text after "--" is not an opening long bracket:
//
//	--(?!\[=*\[)[^\n\r]*
//
// So "--[ x" and "--[=x" are short comments, and "--[==[" is LONG_COMMENT_OPEN.
// base.LineComment cannot leave out the opener, so the rule is composed here.
func commentRule() bytescan.Rule {
	return core.Sequence(
		core.NotFollowedBy(core.Literal("--"), longBracket("")),
		core.ZeroOrMore(core.Rune(core.RuneSet("\n\r").Not())),
	)
}

// numeralRule returns a rule for a Lua numeral (section 3.1): a decimal or
// hexadecimal integer or float, with an optional exponent.
//
//	[0-9]+(\.[0-9]*)?([eE][+-]?[0-9]+)?
//	\.[0-9]+([eE][+-]?[0-9]+)?
//	0[xX]([0-9A-Fa-f]+(\.[0-9A-Fa-f]*)?|\.[0-9A-Fa-f]+)([pP][+-]?[0-9]+)?
//
// Lua takes "3." and ".5", so the floats take both parts as optional, and a
// hexadecimal float with a point needs no exponent ("0x1.8").
func numeralRule() bytescan.Rule {
	optionalParts := []base.NumberOption{base.OptionalIntegerPart(), base.OptionalFractionPart()}
	return core.Choice(
		base.Integer(),
		base.Float(optionalParts...),
		base.HexInteger(),
		base.HexFloat(append(optionalParts, base.OptionalExponent())...),
	)
}

// newLevelScanner returns the scanner for the inside of a long bracket of the
// given level over cur. It emits the body as one token of type body, and the
// closing bracket "]", level times "=", "]" as one token of type close.
//
// The body is a run of runes that does not contain the closing bracket of this
// level (section 3.1):
//
//	([^\]]|\](?!={level}\]))+
//
// A closing bracket of another level is part of the body. At the end of the
// input the body ends with the last rune, and no closer follows. The body and
// the closer never match the same text: the body stops before the "]" that
// starts the closer.
func newLevelScanner(cur bytescan.Cursor, body, close bytescan.TokenType, level int) *bytescan.Scanner {
	tail := strings.Repeat("=", level) + "]"

	lx := bytescan.NewWithCursor(cur)
	lx.MustAddRule(body, core.OneOrMore(core.Choice(
		core.Rune(core.RuneSet("]").Not()),
		core.NotFollowedBy(core.Literal("]"), core.Literal(tail)),
	)))
	lx.MustAddRule(close, core.Literal("]"+tail))
	return lx
}
