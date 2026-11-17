// Command ansi lexes text mixed with terminal escape sequences (ECMA-48, the
// ANSI control functions that xterm and most terminals use) from standard
// input and writes one line per token to standard output. It lexes only: it
// does not decode what a sequence does, and input that no rule matches becomes
// UNKNOWN tokens. See README.md for the covered subset.
//
// With -strip it writes the plain text instead: the text of every TEXT, TAB,
// LF and CR token, in order, with every escape sequence and other control
// removed.
package main

import (
	"bufio"
	"flag"
	"io"
	"log"
	"os"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan-examples/internal/lexdump"
)

func main() {
	strip := flag.Bool("strip", false, "write the plain text, without escape sequences and other controls")
	flag.Parse()

	if *strip {
		if err := writeText(os.Stdout, stripped(newScanner(os.Stdin))); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := (lexdump.Dumper{}).Dump(os.Stdout, newScanner(os.Stdin)); err != nil {
		log.Fatal(err)
	}
}

// stripped returns the plain text of lx: a filter above Next that drops every
// token except TEXT, TAB, LF and CR, so escape sequences, other controls and
// UNKNOWN runes are left out. Skip names the types to drop, so the list holds
// every other type newScanner registers, plus UNKNOWN.
func stripped(lx bytescan.Source) bytescan.Source {
	return bytescan.Skip(lx, bel, bs, c0, csi, osc, escSeq, bytescan.TokenTypeUnknown)
}

// writeText writes the text of each token of lx to w, until io.EOF.
func writeText(w io.Writer, lx bytescan.Source) error {
	bw := bufio.NewWriter(w)
	for {
		l, err := lx.Next()
		if err == io.EOF {
			return bw.Flush()
		}
		if err != nil {
			bw.Flush()
			return err
		}
		if _, err := bw.WriteString(l.Text()); err != nil {
			return err
		}
	}
}
