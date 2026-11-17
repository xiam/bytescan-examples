package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/xiam/bytescan-examples/internal/lexdump"
	"github.com/xiam/bytescan/streamtest"
)

func newFilter(r io.Reader) *lineFilter {
	return newLineFilter(newScanner(r))
}

func TestGolden(t *testing.T) {
	lexdump.Golden(t, lexdump.Dumper{}, newFilter)
}

// TestBalanced checks that every input ends with as many DEDENT tokens as
// INDENT tokens, and that no DEDENT comes before its INDENT.
func TestBalanced(t *testing.T) {
	inputs, err := filepath.Glob(filepath.Join("testdata", "*.input"))
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range inputs {
		src, err := os.ReadFile(input)
		if err != nil {
			t.Fatal(err)
		}
		level := 0
		for _, l := range streamtest.LexAll(t, newFilter(bytes.NewReader(src))) {
			switch l.Type() {
			case indent:
				level++
			case dedent:
				level--
				if level < 0 {
					t.Errorf("%s: DEDENT at %d:%d has no INDENT", input, l.Line(), l.Column())
				}
			}
		}
		if level != 0 {
			t.Errorf("%s: %d INDENT tokens are not closed", input, level)
		}
	}
}

func TestWidth(t *testing.T) {
	for s, want := range map[string]int{
		"":           0,
		"    ":       4,
		"\t":         8,
		"   \t":      8,
		"        \t": 16,
		"\t  ":       10,
		"  \f ":      1,
	} {
		if got := width(s); got != want {
			t.Errorf("width(%q) = %d, want %d", s, got, want)
		}
	}
}
