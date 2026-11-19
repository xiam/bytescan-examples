// Package paint holds the paint of a text that changes: the type and the byte
// range of each token, updated from the change of each edit of bytescan's
// lexer with state (package statelex), and the writers that print the paint
// and a change. The highlight and repl examples paint their text with it.
package paint

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strconv"
	"unicode/utf8"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/statelex"
)

// WriteChange writes a change: its range, its shift, one "-" line per
// removed token and one "+" line per inserted token. old is the text before
// the edit and now the text after it.
func WriteChange(w io.Writer, ch statelex.Change, old, now []byte) error {
	if len(ch.Removed) == 0 && len(ch.Inserted) == 0 {
		_, err := fmt.Fprintln(w, "no change")
		return err
	}
	var out bytes.Buffer
	fmt.Fprintf(&out, "change [%d,%d) -> [%d,%d), shift %+d\n", ch.Start, ch.OldEnd, ch.Start, ch.NewEnd, ch.Shift())
	for _, tok := range ch.Removed {
		fmt.Fprintf(&out, "- %s [%d,%d) %s\n", tok.Type, tok.Start, tok.End, strconv.Quote(string(old[tok.Start:tok.End])))
	}
	for _, tok := range ch.Inserted {
		fmt.Fprintf(&out, "+ %s [%d,%d) %s\n", tok.Type, tok.Start, tok.End, strconv.Quote(string(now[tok.Start:tok.End])))
	}
	_, err := w.Write(out.Bytes())
	return err
}

// Style is how one token type is painted: a letter for the plain ruler and an
// ANSI SGR code for a terminal.
type Style struct {
	Letter byte
	SGR    string
}

// Highlighter holds the paint of a text: the type and the byte range of each
// token, in order. It knows token types and byte positions only. It never
// reads a rule, a mode or the lexer's state.
type Highlighter struct {
	// Styles gives the style of each painted token type. A type that is not
	// in Styles, such as a space or a line break, is not painted.
	Styles map[bytescan.TokenType]Style

	paint []statelex.Token
}

// Tokens returns the painted tokens, in order.
func (h *Highlighter) Tokens() []statelex.Token {
	return h.paint
}

// Repaint applies one change of the lexer to the paint, and it is the only
// input that the highlighter takes from the lexer:
//
//   - it removes the paint of the tokens in [ch.Start, ch.OldEnd), which are
//     the tokens in ch.Removed;
//   - it paints the tokens in ch.Inserted;
//   - it moves every token from ch.OldEnd on by ch.Shift() bytes.
//
// The tokens before ch.Start keep their paint and their place.
func (h *Highlighter) Repaint(ch statelex.Change) {
	// The paint is in order and has no overlap. The tokens before the change
	// end at or before ch.Start; the tokens after it start at or after
	// ch.OldEnd.
	before := sort.Search(len(h.paint), func(k int) bool { return h.paint[k].End > ch.Start })
	after := sort.Search(len(h.paint), func(k int) bool { return h.paint[k].Start >= ch.OldEnd })

	tail := h.paint[after:]
	next := make([]statelex.Token, 0, before+len(ch.Inserted)+len(tail))
	next = append(next, h.paint[:before]...)
	next = append(next, ch.Inserted...)
	d := ch.Shift()
	for _, tok := range tail {
		tok.Start += d
		tok.End += d
		next = append(next, tok)
	}
	h.paint = next
}

// WritePlain writes text with a ruler under each line. The ruler has one
// character for each rune of the line: the letter of the token type, upper
// case on the first rune of a token and lower case on the others, and a space
// for a rune that is not painted. So "total = 12" has the ruler
// "Wwwww O Nn" when the letters of the word, operator and number types are
// w, o and n.
func (h *Highlighter) WritePlain(w io.Writer, text []byte) error {
	var line, ruler bytes.Buffer
	flush := func() error {
		if _, err := w.Write([]byte("  " + line.String() + "\n")); err != nil {
			return err
		}
		_, err := w.Write([]byte("  " + string(bytes.TrimRight(ruler.Bytes(), " ")) + "\n"))
		line.Reset()
		ruler.Reset()
		return err
	}
	for _, tok := range h.paint {
		st, painted := h.Styles[tok.Type]
		b := text[tok.Start:tok.End]
		first := true
		for len(b) > 0 {
			r, n := utf8.DecodeRune(b)
			b = b[n:]
			if r == '\n' {
				if err := flush(); err != nil {
					return err
				}
				first = false
				continue
			}
			line.WriteRune(r)
			switch {
			case !painted:
				ruler.WriteByte(' ')
			case first:
				ruler.WriteByte(upper(st.Letter))
			default:
				ruler.WriteByte(st.Letter)
			}
			first = false
		}
	}
	if line.Len() > 0 {
		return flush()
	}
	return nil
}

// upper returns the upper case of an ASCII letter, and any other byte as it
// is.
func upper(c byte) byte {
	if 'a' <= c && c <= 'z' {
		return c - 'a' + 'A'
	}
	return c
}

// WriteColor writes text with each painted token in its ANSI color.
func (h *Highlighter) WriteColor(w io.Writer, text []byte) error {
	var out bytes.Buffer
	for _, tok := range h.paint {
		b := text[tok.Start:tok.End]
		if st, ok := h.Styles[tok.Type]; ok {
			out.WriteString("\x1b[" + st.SGR + "m")
			out.Write(b)
			out.WriteString("\x1b[0m")
			continue
		}
		out.Write(b)
	}
	if n := out.Len(); n > 0 && out.Bytes()[n-1] != '\n' {
		out.WriteByte('\n')
	}
	_, err := w.Write(out.Bytes())
	return err
}
