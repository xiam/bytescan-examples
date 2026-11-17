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

// newScanner returns a scanner for Spanish sample text that reads r. It registers
// only base rules.
//
// No two rules match the same text, so the registration order decides no token
// here. The NUMBER rule takes "." as the thousands separator and "," as the
// decimal separator, as Spanish writes numbers.
func newScanner(r io.Reader) *bytescan.Scanner {
	lx := bytescan.New(bufio.NewReader(r))

	lx.MustAddRule(word, base.Word())
	lx.MustAddRule(number, base.TextNumber('.', ','))
	lx.MustAddRule(punctuation, base.Punctuation())

	lx.MustAddRule(whitespace, base.Whitespace())
	lx.MustAddRule(newline, base.Newline())

	return lx
}
