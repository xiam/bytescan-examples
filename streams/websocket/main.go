// Command websocket lexes a stream of WebSocket frames (RFC 6455, section 5.2)
// from standard input and writes one line per token to standard output, in
// the hex view of the bytesource package. Each frame gives a FRAME_HEADER, an
// EXTENDED_LENGTH when the 7-bit length is 126 or 127, a MASKING_KEY when the
// MASK bit is set, and its payload as PAYLOAD pieces of at most 64 KiB. It
// lexes only: it does not unmask payloads, validate UTF-8, reassemble
// fragmented messages or read the opening handshake. See README.md for the
// covered subset.
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
