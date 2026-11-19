// Command highlight paints a text by token type, then applies a list of edits
// and repaints only what each edit changed. It reads the text from the file
// named on the command line, or from standard input, and the edits from the
// file of the -edits flag. For each edit it prints the change and the painted
// text. The plain output puts a ruler of token letters under each line; -color
// paints the text with ANSI colors instead.
//
// The lexer is the lexer with state of bytescan (package statelex). The
// highlighter of package paint keeps its own paint and updates it from the
// change of each edit. See README.md.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/xiam/bytescan-examples/highlight/paint"
	"github.com/xiam/bytescan/statelex"
)

func main() {
	editsPath := flag.String("edits", "", "read edits from `file`, one per line: START END \"TEXT\"")
	color := flag.Bool("color", false, "paint with ANSI colors instead of a ruler")
	flag.Parse()

	in := os.Stdin
	if flag.NArg() > 0 {
		f, err := os.Open(flag.Arg(0))
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		in = f
	}
	text, err := io.ReadAll(in)
	if err != nil {
		log.Fatal(err)
	}

	var edits []statelex.Edit
	if *editsPath != "" {
		f, err := os.Open(*editsPath)
		if err != nil {
			log.Fatal(err)
		}
		edits, err = readEdits(f)
		f.Close()
		if err != nil {
			log.Fatal(err)
		}
	}

	if err := run(os.Stdout, text, edits, *color); err != nil {
		log.Fatal(err)
	}
}

// run loads text into a new lexer and paints it, then applies each edit and
// repaints. After each step it writes the change and the painted text.
func run(w io.Writer, text []byte, edits []statelex.Edit, color bool) error {
	lx, err := statelex.New(newModeSet())
	if err != nil {
		return err
	}
	h := &paint.Highlighter{Styles: styles}
	bw := bufio.NewWriter(w)

	// Loading the text is one edit of an empty lexer.
	steps := append([]statelex.Edit{statelex.Insert(0, text)}, edits...)
	for i, e := range steps {
		if i == 0 {
			fmt.Fprintf(bw, "load %d bytes\n", len(e.Text))
		} else {
			fmt.Fprintf(bw, "\nedit %d: %s\n", i, describe(e))
		}

		// The old text is kept only to print the removed tokens.
		old := lx.Text(0, lx.Len())
		ch, err := lx.Apply(e)
		if err != nil {
			return fmt.Errorf("edit %d: %w", i, err)
		}
		h.Repaint(ch)

		now := lx.Text(0, lx.Len())
		if err := paint.WriteChange(bw, ch, old, now); err != nil {
			return err
		}
		if color {
			err = h.WriteColor(bw, now)
		} else {
			err = h.WritePlain(bw, now)
		}
		if err != nil {
			return err
		}
	}
	return bw.Flush()
}
