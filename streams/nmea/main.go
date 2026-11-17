// Command nmea lexes NMEA 0183 sentences from standard input and writes one
// line per token to standard output. It lexes only: it does not compute or
// verify checksums, interpret fields, or judge whether a sentence is valid.
// Bytes that no rule matches become UNKNOWN tokens, and lexing is correct
// again from the next start character. See README.md for the covered subset.
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
