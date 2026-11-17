// Command typescript lexes a subset of TypeScript from standard input and
// writes one line per token to standard output. It is a mode-driver example:
// whether "/" starts a regular expression or is a division depends on the
// token before it, and template literals nest through a stack of modes. It
// lexes only: input that no rule matches becomes UNKNOWN tokens. See
// README.md for the covered subset.
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
