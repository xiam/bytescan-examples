// Command wav lexes a WAV file from standard input and writes one line per
// token to standard output, in the hex view of the bytesource package. The
// RIFF header and the header of each chunk are lexed by rules; the payload of
// each chunk is read by a driver, with the length that the chunk header
// gives. It lexes only: it does not decode samples or check the RIFF size.
// See ../README.md for the covered subset.
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
