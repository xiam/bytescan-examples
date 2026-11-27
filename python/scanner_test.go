package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xiam/bytescan-examples/internal/lexdump"
	"github.com/xiam/bytescan-examples/internal/lextest"
	"github.com/xiam/bytescan/streamtest"
)

func newFilter(r io.Reader) *lineFilter {
	return newLineFilter(newScanner(r))
}

func TestGolden(t *testing.T) {
	lextest.Golden(t, lexdump.Dumper{}, newFilter)
}

// TestBalanced checks that every input ends with as many DEDENT tokens as
// INDENT tokens, and that no DEDENT comes before its INDENT.
func TestBalanced(t *testing.T) {
	inputs, err := filepath.Glob(filepath.Join("testdata", "*.input"))
	require.NoError(t, err)
	for _, input := range inputs {
		src, err := os.ReadFile(input)
		require.NoError(t, err)
		level := 0
		for _, l := range streamtest.LexAll(t, newFilter(bytes.NewReader(src))) {
			switch l.Type() {
			case indent:
				level++
			case dedent:
				level--
				if level < 0 {
					assert.Failf(t, "", "%s: DEDENT at %d:%d has no INDENT", input, l.Line(), l.Column())
				}
			}
		}
		assert.Zerof(t, level, "%s: %d INDENT tokens are not closed", input, level)
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
		{
			got := width(s)
			assert.Equalf(t, want, got, "width(%q) = %d, want %d", s, got, want)
		}
	}
}
