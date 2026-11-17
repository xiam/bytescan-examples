package main

import (
	"bufio"
	"io"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/rules/core"
)

// The token types: the three token characters of the language, and the
// comment between them.
const (
	space   bytescan.TokenType = "SPACE"
	tab     bytescan.TokenType = "TAB"
	lf      bytescan.TokenType = "LF"
	comment bytescan.TokenType = "COMMENT"
)

// newScanner returns a Whitespace scanner that reads r. The three token characters
// of the language, space, tab and line feed, are the only tokens, one token per
// character. Every other rune is comment, so COMMENT is the inverted case of
// the usual skip rule.
//
// No two rules match the same text: SPACE, TAB and LF each match one rune, and
// COMMENT matches only runes that none of them matches. So the registration
// order decides no tie here.
func newScanner(r io.Reader) *bytescan.Scanner {
	lx := bytescan.New(bufio.NewReader(r))

	lx.MustAddRule(space, core.Literal(" "))
	lx.MustAddRule(tab, core.Literal("\t"))
	lx.MustAddRule(lf, core.Literal("\n"))

	// A maximal run of every other rune. It includes \r, and Unicode spaces
	// such as U+00A0 and U+3000, because the language takes only the three
	// token characters.
	lx.MustAddRule(comment, core.RuneRun(core.RuneSet(" \t\n").Not()))

	return lx
}
