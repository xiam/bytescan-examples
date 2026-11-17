// Command korean splits Korean sample text into words, numbers and
// punctuation. It reads standard input and writes one line per token to
// standard output. It lexes only: it does not judge the text. See README.md.
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
