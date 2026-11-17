package main

import (
	"io"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/cursor"
)

// lineFilter applies the line structure of the Python language reference
// (sections 2.1.1 to 2.1.8) to the tokens of newScanner. It wraps Next and:
//
//   - relabels the NL that ends a logical line as NEWLINE;
//   - leaves NL as it is on a blank or comment-only line, inside (), [] or
//     {}, and after LINE_JOIN, which all continue or skip the logical line;
//   - keeps a stack of indentation levels and, before the first token of a
//     logical line, emits one INDENT when the line is deeper than the top of
//     the stack and one DEDENT for each level it pops when the line is
//     shallower;
//   - at the end of input emits NEWLINE if a logical line is open, then one
//     DEDENT for each level above 0.
//
// Tabs advance the indentation to the next multiple of 8, and a formfeed
// resets it to 0. INDENT, DEDENT and a NEWLINE at the end of input are
// synthetic: their text is empty and their position is that of the next
// token, or of the end of input.
//
// The filter does not judge the program. A dedent to a column that matches
// no level pops the levels above it and continues at the nearest lower
// level, where Python reports an error. A closing bracket with no open one
// is a token like any other.
type lineFilter struct {
	lx bytescan.Source

	levels   []int // indentation stack; levels[0] is 0
	depth    int   // open brackets
	open     bool  // the logical line has a token
	measured bool  // the indentation of the logical line is known
	indent   int   // indentation of the logical line, once measured

	pending []*bytescan.Token // tokens to return before reading more
	end     cursor.Pos        // end of the last token read
	eof     bool
}

func newLineFilter(lx bytescan.Source) *lineFilter {
	return &lineFilter{lx: lx, levels: []int{0}}
}

// Next returns the next token, real or synthetic, and io.EOF after the last.
func (f *lineFilter) Next() (*bytescan.Token, error) {
	if len(f.pending) == 0 && !f.eof {
		if err := f.read(); err != nil {
			return nil, err
		}
	}
	if len(f.pending) == 0 {
		return nil, io.EOF
	}
	l := f.pending[0]
	f.pending = f.pending[1:]
	return l, nil
}

// read reads one token from the scanner and queues it with the synthetic
// tokens that come before it.
func (f *lineFilter) read() error {
	l, err := f.lx.Next()
	if err == io.EOF {
		f.eof = true
		if f.open {
			f.pending = append(f.pending, bytescan.NewEmptyToken(newline, f.end))
		}
		for len(f.levels) > 1 {
			f.levels = f.levels[:len(f.levels)-1]
			f.pending = append(f.pending, bytescan.NewEmptyToken(dedent, f.end))
		}
		return nil
	}
	if err != nil {
		return err
	}
	f.end = l.Span().End()

	switch l.Type() {
	case whitespace:
		// Only the leading whitespace of a logical line's first physical
		// line counts.
		if !f.measured {
			f.indent, f.measured = width(l.Text()), true
		}
	case comment:
		f.measured = true
	case lineJoin:
		// The next physical line continues this logical line, so its
		// leading whitespace is not indentation.
		f.measured = true
	case nl:
		if f.depth == 0 {
			if f.open {
				l = l.WithType(newline)
			}
			f.open, f.measured, f.indent = false, false, 0
		}
	default:
		if !f.open {
			f.open, f.measured = true, true
			f.indentation(l.Span().Start())
		}
		if l.Type() != delimiter {
			break
		}
		switch l.Text() {
		case "(", "[", "{":
			f.depth++
		case ")", "]", "}":
			if f.depth > 0 {
				f.depth--
			}
		}
	}
	f.pending = append(f.pending, l)
	return nil
}

// indentation queues the INDENT or DEDENTs that start a logical line whose
// first token starts at pos.
func (f *lineFilter) indentation(pos cursor.Pos) {
	top := f.levels[len(f.levels)-1]
	if f.indent > top {
		f.levels = append(f.levels, f.indent)
		f.pending = append(f.pending, bytescan.NewEmptyToken(indent, pos))
		return
	}
	for f.indent < f.levels[len(f.levels)-1] {
		f.levels = f.levels[:len(f.levels)-1]
		f.pending = append(f.pending, bytescan.NewEmptyToken(dedent, pos))
	}
}

// width returns the indentation of the leading whitespace s (section 2.1.8):
// a space adds 1, a tab advances to the next multiple of 8, and a formfeed
// resets the count to 0.
func width(s string) int {
	n := 0
	for _, r := range s {
		switch r {
		case ' ':
			n++
		case '\t':
			n = n/8*8 + 8
		case '\f':
			n = 0
		}
	}
	return n
}
