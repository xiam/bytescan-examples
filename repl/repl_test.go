package main

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/xiam/bytescan-examples/highlight/paint"
	"github.com/xiam/bytescan-examples/internal/lexdump"
	"github.com/xiam/bytescan/statelex"
)

// bs is one backspace key.
const bs = "\x7f"

// inputs builds the keys of each case in testdata. A backspace is a control
// byte, so the keys are written here, where they can be read.
var inputs = map[string][]byte{
	// Ten backspaces at the end of a line, then Enter.
	"backspaces": []byte("x = 12 + total * alpha" + strings.Repeat(bs, 10) + "\n"),
	// A string opened on one line and closed on the next.
	"string": []byte("say \"hello\nworld\"\n"),
	// The line `say "`, then a line that closes the string.
	"quote": []byte("say \"\nbye\"\n"),
	// A comment opened on one line and closed on the next.
	"comment": []byte("a /* note\nend */ b\n"),
	// The end of input inside an entry: an open string, and a last line
	// with no Enter.
	"eof": []byte("x = \"open\nstill"),
	// Two entries, a backspace on an empty entry, the BS byte, a character of
	// two bytes, and keys that are skipped: a tab and an escape.
	"keys": []byte(bs + "x = 1.5 + 2\n" + "café\x08e\t\x1b\n"),
}

func TestInputs(t *testing.T) {
	lexdump.Inputs(t, inputs)
}

// TestGolden runs the keys of every case and compares the plain output with
// testdata/<name>.golden.
func TestGolden(t *testing.T) {
	for name := range inputs {
		t.Run(name, func(t *testing.T) {
			keys, err := os.ReadFile(filepath.Join("testdata", name+".input"))
			if err != nil {
				t.Fatal(err)
			}
			var got bytes.Buffer
			if err := run(&got, bytes.NewReader(keys), false); err != nil {
				t.Fatal(err)
			}
			lexdump.Compare(t, filepath.Join("testdata", name+".golden"), got.Bytes())
		})
	}
}

// TestColor compares the ANSI output of the string case with
// testdata/string.color.golden.
func TestColor(t *testing.T) {
	var got bytes.Buffer
	if err := run(&got, bytes.NewReader(inputs["string"]), true); err != nil {
		t.Fatal(err)
	}
	lexdump.Compare(t, filepath.Join("testdata", "string.color.golden"), got.Bytes())
}

// press types keys into r, one rune at a time, and returns the change of
// each key that edits the entry.
func press(t *testing.T, r *repl, keys string) []statelex.Change {
	t.Helper()
	var changes []statelex.Change
	for len(keys) > 0 {
		k, size := utf8.DecodeRuneInString(keys)
		keys = keys[size:]
		ch, edited, err := r.press(k, size)
		if err != nil {
			t.Fatal(err)
		}
		if edited {
			changes = append(changes, ch)
		}
	}
	return changes
}

// TestTenBackspaces checks that ten backspaces give ten changes, and that no
// change holds every token of the entry.
func TestTenBackspaces(t *testing.T) {
	r, err := newREPL(io.Discard, false)
	if err != nil {
		t.Fatal(err)
	}
	press(t, r, "x = 12 + total * alpha")
	for i := 1; i <= 10; i++ {
		before := r.lx.Tokens(0, r.lx.Len())
		changes := press(t, r, bs)
		if len(changes) != 1 {
			t.Fatalf("backspace %d: %d changes, want 1", i, len(changes))
		}
		ch, after := changes[0], r.lx.Tokens(0, r.lx.Len())
		if len(ch.Removed) == 0 && len(ch.Inserted) == 0 {
			t.Errorf("backspace %d: empty change", i)
		}
		if len(ch.Removed) == len(before) || len(ch.Inserted) == len(after) {
			t.Errorf("backspace %d: the change holds every token of the entry: removed %v, inserted %v", i, ch.Removed, ch.Inserted)
		}
	}
	if got := string(r.lx.Text(0, r.lx.Len())); got != "x = 12 + tot" {
		t.Fatalf("entry %q, want %q", got, "x = 12 + tot")
	}
}

// TestQuote checks the line `say "`: three complete tokens and no unfinished
// token, and a mode state at the end that is not the start state. So the
// REPL continues because of the mode state.
func TestQuote(t *testing.T) {
	var out bytes.Buffer
	r, err := newREPL(&out, false)
	if err != nil {
		t.Fatal(err)
	}
	press(t, r, `say "`)
	if toks := r.lx.Tokens(0, r.lx.Len()); len(toks) != 3 {
		t.Fatalf("tokens %v, want 3", toks)
	}
	if _, ok := r.lx.Unfinished(); ok {
		t.Fatal("a token is unfinished")
	}
	if r.lx.ModeAtEnd().Equal(r.start) {
		t.Fatal("the mode state at the end is the start state")
	}
	press(t, r, "\n")
	if _, ok := r.lx.Unfinished(); ok {
		t.Fatal("after Enter, a token is unfinished")
	}
	if want := "continue: the text ends in the mode str\n...\n"; !strings.HasSuffix(out.String(), want) {
		t.Fatalf("output ends with\n%s\nwant\n%s", lastLines(out.String(), 2), want)
	}
}

// TestComment checks that an open comment continues because a token needs
// more input, with the mode state at the start state.
func TestComment(t *testing.T) {
	var out bytes.Buffer
	r, err := newREPL(&out, false)
	if err != nil {
		t.Fatal(err)
	}
	press(t, r, "a /* note\n")
	if !r.lx.ModeAtEnd().Equal(r.start) {
		t.Fatal("the mode state at the end is not the start state")
	}
	if want := "continue: the token at 1:2 needs more input\n...\n"; !strings.HasSuffix(out.String(), want) {
		t.Fatalf("output ends with\n%s\nwant\n%s", lastLines(out.String(), 2), want)
	}
}

func lastLines(s string, n int) string {
	lines := strings.SplitAfter(s, "\n")
	if len(lines) > n+1 {
		lines = lines[len(lines)-n-1:]
	}
	return strings.Join(lines, "")
}

// TestRepaint checks, after each key of each case, that the paint of the
// REPL equals the paint of the entry painted from the start.
func TestRepaint(t *testing.T) {
	for name, keys := range inputs {
		t.Run(name, func(t *testing.T) {
			r, err := newREPL(io.Discard, false)
			if err != nil {
				t.Fatal(err)
			}
			for i, k := range string(keys) {
				press(t, r, string(k))
				full := paintFromStart(t, r.lx.Text(0, r.lx.Len()))
				if got, want := fmt.Sprint(r.h.Tokens()), fmt.Sprint(full); got != want {
					t.Fatalf("key %d %q: repaint\n%s\nfull paint\n%s", i, k, got, want)
				}
			}
		})
	}
}

// TestRepaintGenerated does the same check for many keys chosen from a fixed
// seed: characters that open and close strings and comments, backspaces and
// Enter.
func TestRepaintGenerated(t *testing.T) {
	keys := []rune{'"', '/', '*', '=', '.', '1', 'x', 'é', ' ', '@', '\n', 0x7F, 0x7F, 0x08}
	rng := rand.New(rand.NewSource(135))
	r, err := newREPL(io.Discard, false)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 300; i++ {
		k := keys[rng.Intn(len(keys))]
		press(t, r, string(k))
		full := paintFromStart(t, r.lx.Text(0, r.lx.Len()))
		if got, want := fmt.Sprint(r.h.Tokens()), fmt.Sprint(full); got != want {
			t.Fatalf("key %d %q: repaint\n%s\nfull paint\n%s", i, k, got, want)
		}
	}
}

// paintFromStart paints text with a new lexer and a new highlighter.
func paintFromStart(t *testing.T, text []byte) []statelex.Token {
	t.Helper()
	lx, err := statelex.New(newModeSet())
	if err != nil {
		t.Fatal(err)
	}
	ch, err := lx.Apply(statelex.Insert(0, text))
	if err != nil {
		t.Fatal(err)
	}
	h := &paint.Highlighter{Styles: styles}
	h.Repaint(ch)
	return h.Tokens()
}

// TestPromptBeforeFirstKey checks that the first prompt reaches the output
// before the REPL reads a key, so a terminal shows it at once.
func TestPromptBeforeFirstKey(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()
	var out syncBuffer
	done := make(chan error, 1)
	go func() { done <- run(&out, pr, false) }()
	deadline := time.Now().Add(time.Second)
	for out.String() != prompt+"\n" {
		if time.Now().After(deadline) {
			t.Fatalf("output %q before the first key, want %q", out.String(), prompt+"\n")
		}
		time.Sleep(time.Millisecond)
	}
	pw.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// syncBuffer is a bytes.Buffer that two goroutines can use.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
