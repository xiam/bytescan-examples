package main

import (
	"bufio"
	"io"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/rules/base"
	"github.com/xiam/bytescan/rules/special"
)

const (
	url         bytescan.TokenType = "URL"
	emoji       bytescan.TokenType = "EMOJI"
	han         bytescan.TokenType = "HAN"
	word        bytescan.TokenType = "WORD"
	punctuation bytescan.TokenType = "PUNCTUATION"
	whitespace  bytescan.TokenType = "WHITESPACE"
	newline     bytescan.TokenType = "NEWLINE"
)

// newScanner returns a scanner that reads r and has every word rule registered.
func newScanner(r io.Reader) *bytescan.Scanner {
	lx := bytescan.New(bufio.NewReader(r))

	// Specific rules first: the URL, the emoji and the Han run each match text
	// that base.Word also matches, and registration order breaks the tie.
	lx.MustAddRule(url, special.URL())
	lx.MustAddRule(emoji, special.Emoji())
	lx.MustAddRule(han, base.ScriptRun("Han"))

	lx.MustAddRule(word, base.Word())
	lx.MustAddRule(punctuation, base.Punctuation())

	lx.MustAddRule(whitespace, base.Whitespace())
	lx.MustAddRule(newline, base.Newline())

	return lx
}
