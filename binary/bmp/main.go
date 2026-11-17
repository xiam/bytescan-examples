// Command bmp lexes a BMP file from standard input and writes one line per
// token to standard output, in the hex view of the bytesource package. The
// file header is lexed by a rule; the DIB header, the colour table, the gap
// and the pixel data are read by a driver, with the sizes and the offset that
// the headers give. It lexes only: it does not decode pixels. See ../README.md
// for the covered subset.
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
