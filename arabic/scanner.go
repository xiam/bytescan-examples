package main

import (
	"bufio"
	"io"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/rules/base"
	"github.com/xiam/bytescan/rules/core"
)

const (
	word        bytescan.TokenType = "WORD"
	number      bytescan.TokenType = "NUMBER"
	punctuation bytescan.TokenType = "PUNCTUATION"
	whitespace  bytescan.TokenType = "WHITESPACE"
	newline     bytescan.TokenType = "NEWLINE"
)

// newScanner returns a scanner for Arabic sample text that reads r. A word is a
// run of letters and combining marks, so a vowelled word with its harakat is
// one WORD.
//
// No two rules match the same text, so the registration order decides no token
// here.
func newScanner(r io.Reader) *bytescan.Scanner {
	lx := bytescan.New(bufio.NewReader(r))

	lx.MustAddRule(word, base.Word())

	// Arabic-Indic digits, U+0660 to U+0669, with the Arabic thousands
	// separator "٬" (U+066C) and decimal separator "٫" (U+066B): "١٬٢٣٤٫٥٠".
	lx.MustAddRule(number, base.TextNumber('٬', '٫', base.TextDigits(core.RuneRange('٠', '٩'))))

	// \pP, one token per mark: the Arabic comma "،" (U+060C) and question
	// mark "؟" (U+061F) are punctuation as "!" is.
	lx.MustAddRule(punctuation, base.Punctuation())

	lx.MustAddRule(whitespace, base.Whitespace())
	lx.MustAddRule(newline, base.Newline())

	return lx
}
