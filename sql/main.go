// Command sql lexes standard SQL (ISO/IEC 9075-2, clause 5) from standard
// input and writes one line per token to standard output. It lexes only: it
// does not check that a statement is valid SQL, and input that no rule
// matches becomes UNKNOWN tokens. Reserved words are matched without regard
// to case and never as the start of a longer identifier. See README.md for
// the covered subset.
package main

import (
	"log"
	"os"

	"github.com/xiam/bytescan-examples/internal/lexdump"
)

func main() {
	if err := (lexdump.Dumper{}).Dump(os.Stdout, newScanner(os.Stdin)); err != nil {
		log.Fatal(err)
	}
}
