// Command mp3 lexes an MP3 file from standard input and writes one line per
// token to standard output, in the hex view of the bytesource package. The
// ID3v2 tag header, the tag frame headers and the audio frame headers are
// lexed by rules; the bodies are read by a driver, with the lengths that the
// tag frame headers state and that it computes from each audio frame header.
// It lexes only: it does not decode audio or interpret tag text. See
// ../README.md for the covered subset.
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
