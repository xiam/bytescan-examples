// Command syslog lexes a stream of syslog messages (RFC 5424) sent over TCP
// (RFC 6587) from standard input and writes one line per token to standard
// output. A frame is octet-counted (a decimal length, a space and that many
// bytes of message) or newline-ended. The header fields and the structured
// data are lexed by rules; the message body of an octet-counted frame is read
// by a driver, with the length that the frame gives. It lexes only: it does
// not decode the message, check a field length or judge a timestamp. See
// README.md for the covered subset.
package main

import (
	"log"
	"os"
)

func main() {
	if err := dumper.Dump(os.Stdout, newScanner(os.Stdin)); err != nil {
		log.Fatal(err)
	}
}
