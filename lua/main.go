// Command lua lexes Lua 5.4 source from standard input and writes one line per
// token to standard output. It is the first mode-driver example: long brackets
// such as [==[ … ]==] are lexed by a scanner built for the level of the opener,
// over the same cursor as the code-mode scanner. It lexes only: input that no
// rule matches becomes UNKNOWN tokens. See README.md for the covered subset.
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
