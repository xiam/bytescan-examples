// Command repl is a prompt loop that follows the line as the user types it.
// It reads standard input as a list of keys: a printable character is typed
// at the end of the entry, a backspace (byte 0x7F or 0x08) deletes the last
// character of the entry, and Enter (a line break) asks the REPL to run the
// entry or to wait for one more line. For each key it prints the change of the
// tokens and the painted entry; -color paints with ANSI colors instead of a
// ruler.
//
// The lexer is the lexer with state of bytescan (package statelex). The REPL
// holds no lexing code: it reads from the lexer whether a string or a comment
// is still open. See README.md.
package main

import (
	"flag"
	"log"
	"os"
)

func main() {
	color := flag.Bool("color", false, "paint with ANSI colors instead of a ruler")
	flag.Parse()

	if err := run(os.Stdout, os.Stdin, *color); err != nil {
		log.Fatal(err)
	}
}
