package main

import (
	"bufio"
	"io"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/rules/base"
	"github.com/xiam/bytescan/rules/core"
)

// The token types. Each script run has the name of its script.
const (
	han         bytescan.TokenType = "HAN"
	hiragana    bytescan.TokenType = "HIRAGANA"
	katakana    bytescan.TokenType = "KATAKANA"
	latin       bytescan.TokenType = "LATIN"
	number      bytescan.TokenType = "NUMBER"
	punctuation bytescan.TokenType = "PUNCTUATION"
	whitespace  bytescan.TokenType = "WHITESPACE"
	newline     bytescan.TokenType = "NEWLINE"
)

// newScanner returns a scanner for Japanese sample text that reads r. Japanese
// writes no spaces between words, so each token of text is a script run, the
// nearest lexical unit without a dictionary.
//
// No two rules match the same text: the scripts do not overlap, the digits
// and punctuation are in none of the script runs, and "ー" only continues a
// katakana run. So the registration order decides no token here.
func newScanner(r io.Reader) *bytescan.Scanner {
	lx := bytescan.New(bufio.NewReader(r))

	lx.MustAddRule(han, base.ScriptRun("Han"))
	lx.MustAddRule(hiragana, base.ScriptRun("Hiragana"))
	// The prolonged sound mark "ー" (U+30FC) is in the Common script, so a
	// katakana run takes it as an extra class: "コーヒー" is one run, and
	// "ー" never starts one.
	lx.MustAddRule(katakana, base.ScriptRun("Katakana", core.RuneSet("ー")))
	lx.MustAddRule(latin, base.ScriptRun("Latin"))

	lx.MustAddRule(number, base.TextNumber(',', '.'))
	// \pP, one token per mark, such as "、" "。" "！" "？".
	lx.MustAddRule(punctuation, base.Punctuation())

	lx.MustAddRule(whitespace, base.Whitespace())
	lx.MustAddRule(newline, base.Newline())

	return lx
}
