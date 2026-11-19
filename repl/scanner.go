package main

import (
	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan-examples/highlight/paint"
	"github.com/xiam/bytescan/modes"
	"github.com/xiam/bytescan/rules/base"
	"github.com/xiam/bytescan/rules/core"
)

// The token types. In the mode code they are the types of rule set A in
// bytescan's specification of resumable lexing, without its STRING: a
// string is a mode here, as in rule set B (docs/specs/resumable-lexing.md,
// "Rule set A" and "Rule set B: modes").
const (
	word       bytescan.TokenType = "WORD"
	number     bytescan.TokenType = "NUMBER"
	operator   bytescan.TokenType = "OP"
	comment    bytescan.TokenType = "COMMENT"
	whitespace bytescan.TokenType = "WS"
	newline    bytescan.TokenType = "NL"
	quote      bytescan.TokenType = "QUOTE"
	text       bytescan.TokenType = "TEXT"
)

// styles is how each token type is painted. WS and NL are not painted.
var styles = map[bytescan.TokenType]paint.Style{
	word:                      {Letter: 'w', SGR: "36"}, // cyan
	number:                    {Letter: 'n', SGR: "35"}, // magenta
	operator:                  {Letter: 'o', SGR: "33"}, // yellow
	quote:                     {Letter: 'q', SGR: "32"}, // green
	text:                      {Letter: 's', SGR: "32"}, // green
	comment:                   {Letter: 'c', SGR: "90"}, // grey
	bytescan.TokenTypeUnknown: {Letter: '?', SGR: "41"}, // red background
}

// newModeSet returns the rules of the language: two modes, code and str.
//
// In code, a QUOTE pushes str. In str, TEXT is the characters of the string,
// a line break is its own NL token, and a QUOTE pops back to code. So a
// string can run over several lines, and a line that ends inside a string
// ends in the mode str.
//
// In code, no two rules match the same text, so the registration order does
// not decide any token. The longest match decides the rest: "12.5" is one
// NUMBER, "==" is one OP, and "/* x */" is one COMMENT, not two OP tokens and
// more. In str, no two rules match the same text either.
func newModeSet() *modes.ModeSet {
	ms := modes.NewModeSet("code")
	ms.Mode("code").
		Rule(word, core.RuneRun(core.ASCIILetter)).
		Rule(number, numberRule()).
		Rule(operator, core.AnyLiteral("==", "=", "+", "*", "/", ".")).
		Rule(comment, base.BlockComment("/*", "*/")).
		Rule(whitespace, core.RuneRun(core.RuneSet(" "))).
		Rule(newline, core.Literal("\n")).
		Push(quote, core.Literal(`"`), "str")
	ms.Mode("str").
		Rule(text, core.RuneRun(core.RuneSet("\"\n").Not())).
		Rule(newline, core.Literal("\n")).
		Pop(quote, core.Literal(`"`))
	return ms
}

// numberRule returns a rule for a number:
//
//	[0-9]+(\.[0-9]+)?
//
// A "." with no digit after it is not part of the number: "1." lexes as
// NUMBER "1" and OP ".".
func numberRule() bytescan.Rule {
	digits := core.RuneRun(core.ASCIIDigit)
	return core.Sequence(digits, core.Optional(core.Sequence(core.Literal("."), digits)))
}
