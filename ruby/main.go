// Command ruby lexes a subset of Ruby from standard input and writes one line
// per token to standard output. It is a mode-driver example: interpolation in
// double-quoted strings, percent literals whose delimiter the input chooses,
// and heredocs whose body starts on the line after the opener are each lexed
// by a scanner of their own over the same cursor as the code-mode scanner. It
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
