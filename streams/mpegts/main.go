// Command mpegts lexes an MPEG transport stream (ISO/IEC 13818-1) from
// standard input and writes one line per token to standard output, in the hex
// view of the bytesource package. Each 188-byte packet gives a SYNC byte, a
// HEADER, an ADAPTATION_FIELD when the header announces one, and a PAYLOAD.
// After corrupt data the driver skips bytes as UNKNOWN until a sync byte that
// another one confirms a packet later. It lexes only: it does not parse PES
// packets or program tables, and it does not check continuity counters. See
// README.md for the covered subset.
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
