package main

import (
	"bufio"
	"io"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/rules/base"
)

const (
	word        bytescan.TokenType = "WORD"
	number      bytescan.TokenType = "NUMBER"
	punctuation bytescan.TokenType = "PUNCTUATION"
	whitespace  bytescan.TokenType = "WHITESPACE"
	newline     bytescan.TokenType = "NEWLINE"
)

// newScanner returns a scanner for Korean sample text that reads r. Korean
// separates words with spaces, so base.Word splits the text. Precomposed
// syllables (NFC) and conjoining jamo (NFD) are both letters, so either
// encoding of a word is one WORD.
//
// No two rules match the same text, so the registration order decides no token
// here.
func newScanner(r io.Reader) *bytescan.Scanner {
	lx := bytescan.New(bufio.NewReader(r))

	lx.MustAddRule(word, base.Word())
	lx.MustAddRule(number, base.TextNumber(',', '.'))
	lx.MustAddRule(punctuation, base.Punctuation())

	lx.MustAddRule(whitespace, base.Whitespace())
	lx.MustAddRule(newline, base.Newline())

	return lx
}
