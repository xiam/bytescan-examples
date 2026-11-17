// Command hello lexes text from standard input with the rules of the README
// example and writes one line per token to standard output, without the
// layout. See README.md.
package main

import (
	"log"
	"os"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan-examples/internal/lexdump"
)

// dumper skips the layout, as the README example does.
var dumper = lexdump.Dumper{Skip: []bytescan.TokenType{whitespace, newline}}

func main() {
	if err := dumper.Dump(os.Stdout, newScanner(os.Stdin)); err != nil {
		log.Fatal(err)
	}
}
