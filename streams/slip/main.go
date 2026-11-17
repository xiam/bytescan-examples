// Command slip lexes a SLIP stream (RFC 1055) from standard input and writes
// one line per token to standard output, in the hex view of the bytesource
// package. END bytes separate frames; inside a frame, DATA runs alternate with
// the escape sequences ESC_END and ESC_ESC. It lexes only: it does not unstuff
// the escape sequences into payload bytes and does not interpret the frames.
// See README.md for the covered subset.
package main

import (
	"log"
	"os"

	"github.com/xiam/bytescan-examples/internal/hexdump"
)

func main() {
	if err := hexdump.Dumper.Dump(os.Stdout, newScanner(os.Stdin)); err != nil {
		log.Fatal(err)
	}
}
