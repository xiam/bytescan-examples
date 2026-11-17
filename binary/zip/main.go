// Command zip lexes a ZIP archive from standard input and writes one line per
// token to standard output, in the hex view of the bytesource package. The
// fixed part of each record is lexed by rules; the file names, extra fields,
// file data and comments are read by a driver, with the lengths that the
// records give. It lexes only: it does not decompress, check a CRC or follow
// the offsets of the central directory. See ../README.md for the covered
// subset.
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
