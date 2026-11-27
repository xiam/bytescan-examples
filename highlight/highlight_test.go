package main

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xiam/bytescan-examples/highlight/paint"
	"github.com/xiam/bytescan-examples/internal/lextest"
	"github.com/xiam/bytescan/statelex"
)

// testCase is one testdata/<name>.input and its edits, from
// testdata/<name>.edits when that file exists.
type testCase struct {
	name  string
	text  []byte
	edits []statelex.Edit
}

func loadCases(t *testing.T) []testCase {
	t.Helper()
	inputs, err := filepath.Glob(filepath.Join("testdata", "*.input"))
	require.NoError(t, err)
	require.NotZero(t, len(inputs), "no testdata/*.input files")
	var cases []testCase
	for _, input := range inputs {
		name := strings.TrimSuffix(filepath.Base(input), ".input")
		text, err := os.ReadFile(input)
		require.NoError(t, err)
		c := testCase{name: name, text: text}
		if f, err := os.Open(filepath.Join("testdata", name+".edits")); err == nil {
			c.edits, err = readEdits(f)
			f.Close()
			require.NoErrorf(t, err, "%s: %v", name, err)
		}
		cases = append(cases, c)
	}
	return cases
}

// TestGolden runs every case and compares the plain output with
// testdata/<name>.golden.
func TestGolden(t *testing.T) {
	for _, c := range loadCases(t) {
		t.Run(c.name, func(t *testing.T) {
			var got bytes.Buffer
			require.NoError(t, run(&got, c.text, c.edits, false))
			lextest.Compare(t, filepath.Join("testdata", c.name+".golden"), got.Bytes())
		})
	}
}

// TestColor compares the ANSI output of the document with
// testdata/document.color.golden.
func TestColor(t *testing.T) {
	text, err := os.ReadFile(filepath.Join("testdata", "document.input"))
	require.NoError(t, err)
	var got bytes.Buffer
	require.NoError(t, run(&got, text, nil, true))
	lextest.Compare(t, filepath.Join("testdata", "document.color.golden"), got.Bytes())
}

// TestRepaint checks, after each edit of each case, that the paint that the
// highlighter holds equals the paint of the final text painted from the start.
func TestRepaint(t *testing.T) {
	for _, c := range loadCases(t) {
		t.Run(c.name, func(t *testing.T) {
			steps := append([]statelex.Edit{statelex.Insert(0, c.text)}, c.edits...)
			checkRepaint(t, steps)
		})
	}
}

// TestRepaintGenerated does the same check after many edits made from a fixed
// seed: insertions of pieces that open and close strings and comments, and
// deletions of random ranges.
func TestRepaintGenerated(t *testing.T) {
	text, err := os.ReadFile(filepath.Join("testdata", "document.input"))
	require.NoError(t, err)
	pieces := []string{`"`, "/*", "*/", "*", "/", "=", "==", ".", "12", "3.5", "x", "word", " ", "\n", "@"}
	rng := rand.New(rand.NewSource(134))
	steps := []statelex.Edit{statelex.Insert(0, text)}
	n := len(text)
	for i := 0; i < 300; i++ {
		start := rng.Intn(n + 1)
		end := start
		if rng.Intn(2) == 0 {
			end = start + rng.Intn(min(8, n-start)+1)
		}
		var ins []byte
		if rng.Intn(3) > 0 {
			ins = []byte(pieces[rng.Intn(len(pieces))])
		}
		steps = append(steps, statelex.Edit{Start: start, End: end, Text: ins})
		n += len(ins) - (end - start)
	}
	checkRepaint(t, steps)
}

// checkRepaint applies steps to one lexer and one highlighter. After each
// step it checks that the tokens that the change removes are the painted
// tokens of its range, and that the paint equals the paint of a new
// highlighter that loads the whole new text.
func checkRepaint(t *testing.T, steps []statelex.Edit) {
	t.Helper()
	lx, err := statelex.New(newModeSet())
	require.NoError(t, err)
	h := &paint.Highlighter{Styles: styles}
	for i, e := range steps {
		ch, err := lx.Apply(e)
		require.NoErrorf(t, err, "step %d: %v", i, err)
		{
			got, want := fmt.Sprint(paintIn(h.Tokens(), ch.Start, ch.OldEnd)), fmt.Sprint(ch.Removed)
			if got != want {
				require.FailNowf(t, "", "step %d (%s): painted in [%d,%d) %s, removed %s", i, describe(e), ch.Start, ch.OldEnd, got, want)
			}
		}
		h.Repaint(ch)
		full := paintFromStart(t, lx.Text(0, lx.Len()))
		{
			got, want := fmt.Sprint(h.Tokens()), fmt.Sprint(full)
			if got != want {
				require.FailNowf(t, "", "step %d (%s): repaint\n%s\nfull paint\n%s", i, describe(e), got, want)
			}
		}
	}
}

// paintIn returns the painted tokens inside [start, end).
func paintIn(paint []statelex.Token, start, end int) []statelex.Token {
	var in []statelex.Token
	for _, tok := range paint {
		if tok.Start >= start && tok.End <= end {
			in = append(in, tok)
		}
	}
	return in
}

// paintFromStart paints text with a new lexer and a new highlighter.
func paintFromStart(t *testing.T, text []byte) []statelex.Token {
	t.Helper()
	lx, err := statelex.New(newModeSet())
	require.NoError(t, err)
	ch, err := lx.Apply(statelex.Insert(0, text))
	require.NoError(t, err)
	h := &paint.Highlighter{Styles: styles}
	h.Repaint(ch)
	return h.Tokens()
}

func TestReadEdits(t *testing.T) {
	in := "# a comment\n\n12 13\n4 4 \"\\\"\"\n0 12 \"say \\\"hi\\\"\\n\"\n"
	edits, err := readEdits(strings.NewReader(in))
	require.NoError(t, err)
	var got []string
	for _, e := range edits {
		got = append(got, describe(e))
	}
	want := []string{`delete [12,13)`, `insert "\"" at 4`, `replace [0,12) with "say \"hi\"\n"`}
	assert.Equalf(t, fmt.Sprint(want), fmt.Sprint(got), "got %q, want %q", got, want)
	for _, bad := range []string{"x 1", "1 y", "1 2 nope"} {
		{
			_, err := readEdits(strings.NewReader(bad))
			assert.Errorf(t, err, "%q: no error", bad)
		}
	}
}
