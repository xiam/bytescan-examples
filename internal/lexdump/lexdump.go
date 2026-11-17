// Package lexdump runs a scanner to the end of its input and writes one line per
// token. Every example in this repository uses it, both in main.go and in
// its golden tests, so all examples print the same format.
//
// The Dumper's Format function turns a token into its line. The default, Text,
// writes three fields separated by one space:
//
//	LINE:COLUMN TYPE "TEXT"
//
// LINE is 1-based and COLUMN is the 0-based rune column, as Token.Line and
// Token.Column report them. TYPE is the token type. TEXT is the token text
// as a Go quoted string, so a line break or a control character stays on one
// line. An example whose input is not text, such as a binary format, supplies
// its own Format instead.
package lexdump

import (
	"bufio"
	"io"
	"strconv"

	"github.com/xiam/bytescan"
)

// Dumper writes the tokens of a bytescan.Source, one line each: a
// *bytescan.Scanner, a filter that wraps the Next method of a scanner, or a
// mode driver. The zero value writes every token with Text.
type Dumper struct {
	// Skip lists token types that are read but not written, such as layout.
	// Dump drops them with bytescan.Skip.
	Skip []bytescan.TokenType

	// Format returns the line for one token, without the line break. Nil
	// means Text.
	Format func(*bytescan.Token) string
}

// Text returns the line for l in the text view: LINE:COLUMN TYPE "TEXT".
func Text(l *bytescan.Token) string {
	return strconv.Itoa(l.Line()) + ":" + strconv.Itoa(l.Column()) + " " +
		string(l.Type()) + " " + strconv.Quote(l.Text())
}

// Dump reads lx until io.EOF and writes one line per token to w. It returns
// the first error of lx other than io.EOF, or the first write error.
func (d Dumper) Dump(w io.Writer, lx bytescan.Source) error {
	lx = bytescan.Skip(lx, d.Skip...)
	format := d.Format
	if format == nil {
		format = Text
	}
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
		if _, err := bw.WriteString(format(l) + "\n"); err != nil {
			return err
		}
	}
}
