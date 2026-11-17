package main

import (
	"bufio"
	"io"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/rules/base"
	"github.com/xiam/bytescan/rules/core"
)

// The token types.
const (
	keyword    bytescan.TokenType = "KEYWORD"
	word       bytescan.TokenType = "WORD"
	whitespace bytescan.TokenType = "WHITESPACE"
	newline    bytescan.TokenType = "NEWLINE"
)

// newScanner returns the scanner of the README example that reads r: the keyword
// "say", words and whitespace, and line breaks, so that it also reads a file.
//
// One tie is broken by registration order: "say" is also a word, so KEYWORD
// is registered before WORD. The keyword matches only when no letter follows
// it, so "sayonara" is one WORD.
func newScanner(r io.Reader) *bytescan.Scanner {
	lx := bytescan.New(bufio.NewReader(r))

	lx.MustAddRule(keyword, core.Keyword("say", core.UnicodeLetter))
	lx.MustAddRule(word, base.Word())
	lx.MustAddRule(whitespace, base.Whitespace())
	lx.MustAddRule(newline, base.Newline())

	return lx
}
