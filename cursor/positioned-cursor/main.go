// Command positioned-cursor shows the cursor layer of bytescan, the package
// bytescan/cursor, on its own: reading a bounded window ahead of the logical
// cursor, replaying it exactly, and lifting diagnostic context out of the
// retained input without moving the cursor.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/xiam/bytescan/cursor"
)

const document = `theme: dark
name: 世界
broken: 🦄
`

// readRule is the shape a tiny line-oriented scanner needs: read ahead until
// the newline, but do not commit to the bytes until the whole line is known.
func readRule(cr *cursor.Reader) (cursor.Span, error) {
	mark := cr.Checkpoint()
	defer func() { _ = mark.Release() }()

	var (
		first cursor.LocatedRune
		last  cursor.LocatedRune
		seen  bool
	)

	for {
		lr, err := cr.ReadLocatedRune()
		if err != nil {
			if errors.Is(err, io.EOF) && seen {
				return spanOf(first, last), nil
			}
			return cursor.Span{}, err
		}
		if !seen {
			first, seen = lr, true
		}
		last = lr

		if lr.Rune() == '\n' {
			break
		}
	}

	return spanOf(first, last), nil
}

func spanOf(first, last cursor.LocatedRune) cursor.Span {
	return cursor.NewSpan(first.Pos(), last.End())
}

// run reads r line by line and writes, for each line, its span, its text, its
// surrounding context and the position of the next read to w.
func run(w io.Writer, r io.Reader) error {
	cr := cursor.NewReader(r)

	for {
		// Hold a checkpoint across the whole read-ahead so the span stays
		// recoverable after the cursor has moved past it.
		mark := cr.Checkpoint()

		start := cr.Cursor()
		span, err := readRule(cr)
		if err != nil {
			_ = mark.Release()
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}

		text, err := cr.SpanText(span)
		if err != nil {
			_ = mark.Release()
			return err
		}

		context, err := cr.Context(span, 4, 4)
		if err != nil {
			_ = mark.Release()
			return err
		}

		end := cr.Cursor()
		fmt.Fprintf(w, "line starting at %s: %q\n", start, strings.TrimRight(text, "\n"))
		fmt.Fprintf(w, "  span: bytes %d..%d, runes %d..%d, %s>%s\n",
			span.Start().ByteOffset(), span.End().ByteOffset(),
			span.Start().RuneOffset(), span.End().RuneOffset(),
			span.Start(), span.End())
		fmt.Fprintf(w, "  context: %q\n", strings.ReplaceAll(context, "\n", "\\n"))
		fmt.Fprintf(w, "  next read is at %s (byte %d)\n\n", end, end.ByteOffset())

		// Release once the span is no longer needed. Until this point the
		// bytes behind the cursor were retained; afterwards the reader
		// compacts them away.
		_ = mark.Release()
	}

	fmt.Fprintln(w, "done")
	return nil
}

func main() {
	if err := run(os.Stdout, strings.NewReader(document)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
