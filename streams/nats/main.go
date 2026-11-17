// Command nats lexes a stream of the NATS client protocol from standard input
// and writes one line per token to standard output. Rules lex the control
// lines; a driver reads each payload by the byte count of its line. It lexes
// only: it keeps no subscription, does not parse headers or JSON, and does
// not judge whether a line is valid. See README.md for the covered subset.
package main

import (
	"log"
	"os"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan-examples/internal/lexdump"
	"github.com/xiam/bytescan-examples/internal/streamkit"
)

// quoteBytes is the number of bytes that the dump shows of a token.
const quoteBytes = 48

// format returns the line for l in the quoted view of streamkit.Quote, with
// the first 48 bytes of the token, so control lines read as text and
// payload bytes stay on one line.
func format(l *bytescan.Token) string {
	return streamkit.Quote(l, quoteBytes)
}

// dumper writes every token with format.
var dumper = lexdump.Dumper{Format: format}

func main() {
	if err := dumper.Dump(os.Stdout, newScanner(os.Stdin)); err != nil {
		log.Fatal(err)
	}
}
