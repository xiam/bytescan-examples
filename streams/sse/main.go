// Command sse lexes a Server-Sent Events stream (the WHATWG text/event-stream
// format) from standard input and writes one line per token to standard
// output. The type of a token depends on where it is in its line, so a mode
// driver picks the scanner for each position over one cursor. It lexes only: it
// does not dispatch events or interpret fields, and it reads an endless stream
// with constant memory. See README.md for the covered subset.
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
