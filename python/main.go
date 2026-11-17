// Command python lexes a subset of Python 3.12 from standard input and writes
// one line per token to standard output. A filter above the scanner adds the
// NEWLINE, INDENT and DEDENT tokens of the line structure. It lexes only: it
// does not report bad indentation or unbalanced brackets, and input that no
// rule matches becomes UNKNOWN tokens. See README.md for the covered subset.
package main

import (
	"log"
	"os"

	"github.com/xiam/bytescan-examples/internal/lexdump"
)

func main() {
	if err := (lexdump.Dumper{}).Dump(os.Stdout, newLineFilter(newScanner(os.Stdin))); err != nil {
		log.Fatal(err)
	}
}
