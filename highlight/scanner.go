package main

import (
	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan-examples/highlight/paint"
	"github.com/xiam/bytescan/modes"
	"github.com/xiam/bytescan/rules/base"
	"github.com/xiam/bytescan/rules/core"
)

// The token types. They are the types of rule set A in bytescan's
// specification of resumable lexing (docs/specs/resumable-lexing.md, "Rule
// set A").
const (
	word       bytescan.TokenType = "WORD"
	number     bytescan.TokenType = "NUMBER"
	operator   bytescan.TokenType = "OP"
	stringLit  bytescan.TokenType = "STRING"
	comment    bytescan.TokenType = "COMMENT"
	whitespace bytescan.TokenType = "WS"
	newline    bytescan.TokenType = "NL"
)

// styles is how each token type is painted. WS and NL are not painted.
var styles = map[bytescan.TokenType]paint.Style{
	word:                      {Letter: 'w', SGR: "36"}, // cyan
	number:                    {Letter: 'n', SGR: "35"}, // magenta
	operator:                  {Letter: 'o', SGR: "33"}, // yellow
	stringLit:                 {Letter: 's', SGR: "32"}, // green
	comment:                   {Letter: 'c', SGR: "90"}, // grey
	bytescan.TokenTypeUnknown: {Letter: '?', SGR: "41"}, // red background
}

// newModeSet returns the rules of the language, as the lexer with state takes
// them: a mode set with one mode.
//
// No two rules match the same text, so the registration order does not decide
// any token here. The longest match decides the rest: "12.5" is one NUMBER,
// not NUMBER, OP and NUMBER; "==" is one OP; "/* x */" is one COMMENT, not
// two OP tokens and more. The order is the order of rule set A.
func newModeSet() *modes.ModeSet {
	ms := modes.NewModeSet("code")
	ms.Mode("code").
		Rule(word, core.RuneRun(core.ASCIILetter)).
		Rule(number, numberRule()).
		Rule(operator, core.AnyLiteral("==", "=", "+", "*", "/", ".")).
		Rule(stringLit, stringRule()).
		Rule(comment, base.BlockComment("/*", "*/")).
		Rule(whitespace, core.RuneRun(core.RuneSet(" "))).
		Rule(newline, core.Literal("\n"))
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

// stringRule returns a rule for a string:
//
//	"[^"\n]*"
//
// A string has no escape and ends on its line. A quote that is not closed on
// its line does not match, and the engine reports it as UNKNOWN.
func stringRule() bytescan.Rule {
	return core.Sequence(
		core.Literal(`"`),
		core.ZeroOrMore(core.Rune(core.RuneSet("\"\n").Not())),
		core.Literal(`"`),
	)
}
