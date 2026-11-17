// Command mqtt lexes a stream of MQTT 3.1.1 control packets from standard
// input and writes one line per token to standard output, in the hex view of
// the bytesource package. Each packet gives a FIXED_HEADER byte and a
// REMAINING_LENGTH of 1 to 4 bytes, lexed by rules; the driver uses the
// length to frame the rest of the packet: VARIABLE_HEADER and PAYLOAD for
// PUBLISH, BODY for every other type. It lexes only: it does not parse
// topics, payloads or properties, and it does not judge flags. See README.md
// for the covered subset.
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
