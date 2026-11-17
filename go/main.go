// Command go lexes Go source code from standard input and writes one line per
// token to standard output. A filter above the scanner inserts the semicolons
// of rule 1 of the Go specification; each inserted one is marked "inserted".
// The program lexes only: it does not parse, and input that no rule matches
// becomes UNKNOWN tokens. See README.md for the covered subset.
package main

import (
	"log"
	"os"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan-examples/internal/lexdump"
)

// dumper writes every token as lexdump.Text does, and marks a semicolon that
// the filter inserted.
var dumper = lexdump.Dumper{Format: func(l *bytescan.Token) string {
	if isInserted(l) {
		return lexdump.Text(l) + " inserted"
	}
	return lexdump.Text(l)
}}

func main() {
	if err := dumper.Dump(os.Stdout, newSemicolons(newScanner(os.Stdin))); err != nil {
		log.Fatal(err)
	}
}
