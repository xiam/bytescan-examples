// Command words is a multilingual word scanner built only from package rules. It
// reads text from standard input and writes one line per token to standard
// output, skipping layout (whitespace and line breaks). See README.md.
package main

import (
	"log"
	"os"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan-examples/internal/lexdump"
)

// dumper skips layout, so the output lists only words, punctuation and the
// other tokens of the text.
var dumper = lexdump.Dumper{Skip: []bytescan.TokenType{whitespace, newline}}

func main() {
	if err := dumper.Dump(os.Stdout, newScanner(os.Stdin)); err != nil {
		log.Fatal(err)
	}
}
