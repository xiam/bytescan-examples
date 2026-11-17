// Command whitespace lexes Whitespace, the esoteric language whose only tokens
// are space, tab and line feed, and writes one line per token to standard
// output. Every other character is comment. It lexes only: it does not group
// the tokens into instructions. See README.md.
package main

import (
	"flag"
	"log"
	"os"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan-examples/internal/lexdump"
)

func main() {
	drop := flag.Bool("drop-comments", false, "leave the COMMENT tokens out of the output")
	flag.Parse()

	var src bytescan.Source = newScanner(os.Stdin)
	if *drop {
		src = withoutComments(src)
	}
	if err := (lexdump.Dumper{}).Dump(os.Stdout, src); err != nil {
		log.Fatal(err)
	}
}

// withoutComments is a filter above Next. Comment is not part of a Whitespace
// program, so the filter reads past each COMMENT token and returns the next
// token. It judges nothing else about the input.
func withoutComments(lx bytescan.Source) bytescan.Source {
	return bytescan.Skip(lx, comment)
}
