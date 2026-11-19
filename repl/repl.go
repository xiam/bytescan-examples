package main

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/xiam/bytescan-examples/highlight/paint"
	"github.com/xiam/bytescan/modes"
	"github.com/xiam/bytescan/statelex"
)

// The keys that are not typed into the entry.
const (
	enter     = '\n'
	backspace = 0x7F // the DEL byte, which most terminals send
	ctrlH     = 0x08 // the BS byte
)

// The prompts: one for the first line of an entry, one for each line after it.
const (
	prompt         = ">"
	continuePrompt = "..."
)

// repl holds one entry in a lexer with state. Each key is one edit of the
// lexer. A highlighter keeps the paint of the entry from the change of each
// edit.
type repl struct {
	w     io.Writer
	color bool
	lx    *statelex.Lexer
	start modes.Stack // the mode state of an empty entry
	h     *paint.Highlighter
}

func newREPL(w io.Writer, color bool) (*repl, error) {
	lx, err := statelex.New(newModeSet())
	if err != nil {
		return nil, err
	}
	return &repl{
		w:     w,
		color: color,
		lx:    lx,
		start: lx.ModeAtEnd(),
		h:     &paint.Highlighter{Styles: styles},
	}, nil
}

// run reads keys from in until the end of input, and writes what the REPL
// does to w. It writes after each key, so a terminal shows each step.
func run(w io.Writer, in io.Reader, color bool) error {
	bw := bufio.NewWriter(w)
	r, err := newREPL(bw, color)
	if err != nil {
		return err
	}
	fmt.Fprintln(bw, prompt)
	if err := bw.Flush(); err != nil {
		return err
	}
	keys := bufio.NewReader(in)
	for {
		k, size, err := keys.ReadRune()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if _, _, err := r.press(k, size); err != nil {
			return err
		}
		if err := bw.Flush(); err != nil {
			return err
		}
	}
	if err := r.end(); err != nil {
		return err
	}
	return bw.Flush()
}

// press handles one key. A key that edits the entry returns its change and
// true.
func (r *repl) press(k rune, size int) (statelex.Change, bool, error) {
	n := r.lx.Len()
	switch {
	case k == utf8.RuneError && size == 1:
		fmt.Fprintln(r.w, "key: skipped, a byte that is not UTF-8")
		return statelex.Change{}, false, nil

	case k == enter:
		ch, err := r.apply("key enter", statelex.Insert(n, []byte{enter}))
		if err != nil {
			return ch, true, err
		}
		return ch, true, r.decide()

	case k == backspace || k == ctrlH:
		if n == 0 {
			fmt.Fprintln(r.w, "key backspace: the entry is empty")
			return statelex.Change{}, false, nil
		}
		// The last character of the entry, not its last byte: an edit that
		// splits a character is refused (statelex.ErrSplitRune).
		_, last := utf8.DecodeLastRune(r.lx.Text(n-utf8.UTFMax, n))
		ch, err := r.apply("key backspace", statelex.Delete(n-last, n))
		return ch, true, err

	case unicode.IsPrint(k):
		ch, err := r.apply("key "+strconv.QuoteRune(k), statelex.Insert(n, []byte(string(k))))
		return ch, true, err

	default:
		fmt.Fprintf(r.w, "key %U: skipped\n", k)
		return statelex.Change{}, false, nil
	}
}

// apply applies one edit to the entry, repaints it from the change, and
// writes the edit, the change and the painted entry.
func (r *repl) apply(key string, e statelex.Edit) (statelex.Change, error) {
	if e.Start == e.End {
		fmt.Fprintf(r.w, "%s: insert %q at %d\n", key, e.Text, e.Start)
	} else {
		fmt.Fprintf(r.w, "%s: delete [%d,%d)\n", key, e.Start, e.End)
	}

	// The old text is kept only to print the removed tokens.
	old := r.lx.Text(0, r.lx.Len())
	ch, err := r.lx.Apply(e)
	if err != nil {
		return ch, err
	}
	r.h.Repaint(ch)

	now := r.lx.Text(0, r.lx.Len())
	if err := paint.WriteChange(r.w, ch, old, now); err != nil {
		return ch, err
	}
	if r.color {
		return ch, r.h.WriteColor(r.w, now)
	}
	return ch, r.h.WritePlain(r.w, now)
}

// decide runs the entry or continues it, after Enter. The REPL continues
// when the lexer says that a string or a comment is open. Otherwise it writes
// the tokens of the entry and starts a new entry.
func (r *repl) decide() error {
	open, err := r.open()
	if err != nil {
		return err
	}
	if len(open) > 0 {
		fmt.Fprintf(r.w, "continue: %s\n", strings.Join(open, "; "))
		fmt.Fprintln(r.w, continuePrompt)
		return nil
	}

	fmt.Fprintln(r.w, "run")
	if err := r.writeTokens(); err != nil {
		return err
	}
	// The next entry starts from an empty lexer. The change of this deletion
	// removes every token, so the paint is empty too.
	ch, err := r.lx.Apply(statelex.Delete(0, r.lx.Len()))
	if err != nil {
		return err
	}
	r.h.Repaint(ch)
	fmt.Fprintln(r.w, prompt)
	return nil
}

// open returns why the entry is not complete, from two queries of the lexer.
// It returns nothing when the entry is complete.
//
//   - Unfinished: a token read the end of the text, so it needs more input.
//     A comment with no end is one.
//   - ModeAtEnd: the mode state at the end of the text is not the state of
//     an empty entry. A string with no closing quote is one.
func (r *repl) open() ([]string, error) {
	var why []string
	if at, ok := r.lx.Unfinished(); ok {
		line, col, err := r.lx.Position(at)
		if err != nil {
			return nil, err
		}
		why = append(why, fmt.Sprintf("the token at %d:%d needs more input", line, col))
	}
	if end := r.lx.ModeAtEnd(); !end.Equal(r.start) {
		why = append(why, fmt.Sprintf("the text ends in the mode %s", end.Top()))
	}
	return why, nil
}

// writeTokens writes every token of the entry, one per line, as the other
// examples write them: LINE:COLUMN TYPE "TEXT".
func (r *repl) writeTokens() error {
	text := r.lx.Text(0, r.lx.Len())
	for _, tok := range r.lx.Tokens(0, r.lx.Len()) {
		line, col, err := r.lx.Position(tok.Start)
		if err != nil {
			return err
		}
		fmt.Fprintf(r.w, "%d:%d %s %s\n", line, col, tok.Type, strconv.Quote(string(text[tok.Start:tok.End])))
	}
	return nil
}

// end handles the end of input. The lexer already lexes the end of its text
// as the end of input, so the tokens it holds are the final tokens. When the
// input ends inside an entry, end writes them and says why the entry was not
// finished.
func (r *repl) end() error {
	n := r.lx.Len()
	if n == 0 {
		fmt.Fprintln(r.w, "end of input")
		return nil
	}
	why, err := r.open()
	if err != nil {
		return err
	}
	if r.lx.Text(n-1, n)[0] != enter {
		why = append(why, "the last line has no enter")
	}
	fmt.Fprintf(r.w, "end of input: the entry was not finished: %s\n", strings.Join(why, "; "))
	return r.writeTokens()
}
