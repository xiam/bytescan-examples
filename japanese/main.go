// Command japanese splits Japanese sample text into script runs (Han,
// hiragana, katakana and Latin), numbers and punctuation. It reads standard
// input and writes one line per token to standard output. A script run is not
// a word: it lexes only, with no dictionary. See README.md.
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
