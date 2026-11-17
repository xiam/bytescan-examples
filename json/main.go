// Command json lexes JSON (RFC 8259) from standard input and writes one line
// per token to standard output. It lexes only: it does not check nesting,
// commas or duplicate names, and input that no rule matches becomes UNKNOWN
// tokens. See README.md for the covered subset.
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
