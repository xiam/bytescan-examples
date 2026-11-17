// Command ssejson lexes JSON inside Server-Sent Events, the shape of a
// streaming HTTP API that sends JSON documents in text/event-stream, from
// standard input, and writes one line per token to standard output. Two
// layers run at the same time over two cursors: the SSE layer lexes the
// stream, and the JSON layer lexes the text of its data values, which a pull
// pipe feeds to the second cursor without a goroutine. Both layers lex only:
// nothing is parsed, decoded or dispatched, and an endless stream is read with
// constant memory. See README.md.
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
