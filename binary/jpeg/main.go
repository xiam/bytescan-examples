// Command jpeg lexes a JPEG file from standard input and writes one line per
// token to standard output, in the hex view of the bytesource package. The
// markers, the segment headers and the entropy-coded data are lexed by rules;
// the segment payloads are read by a mode driver, with the lengths that the
// segment headers give. In an Exif segment the driver lexes the TIFF header
// and the entries of the first IFD. It lexes only: it does not decode the
// image or interpret the Exif tags. See ../README.md for the covered subset.
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
