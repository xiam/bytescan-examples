package main

import (
	"io"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	"github.com/xiam/bytescan-examples/internal/lexdump"
	"github.com/xiam/bytescan-examples/internal/lextest"
	"github.com/xiam/bytescan/streamtest"
)

func TestGolden(t *testing.T) {
	lextest.Golden(t, lexdump.Dumper{}, newScanner)
}

// TestGuard checks that every scanner of both layers is bounded by the
// per-token guard of the stream harness, that the guard holds the largest
// JSON token (1000 runes of 4 bytes, and its look-ahead rune), and that the
// longest JSON_NUMBER is within maxRun runes.
func TestGuard(t *testing.T) {
	require.Equalf(t, streamtest.TextGuard, maxTokenBytes, "maxTokenBytes = %d, want streamtest.TextGuard (%d)", maxTokenBytes, streamtest.TextGuard)
	require.Lessf(t, (maxRun+1)*utf8.UTFMax, maxTokenBytes, "a token of %d runes and its look-ahead may take %d bytes, not below the guard of %d", maxRun, (maxRun+1)*utf8.UTFMax, maxTokenBytes)
	{
		n := 1 + maxDigits + 1 + maxDigits + 2 + maxDigits
		require.LessOrEqualf(t, n, maxRun, "the longest JSON_NUMBER has %d runes, more than %d", n, maxRun)
	}
	// A data line of four-byte runes in a string longer than maxRun, and
	// the same as whitespace-separated text: every token of both layers
	// stays within the guard.
	run := strings.Repeat("\U0001F600", maxRun+1)
	in := "data: \"" + run + "\"\n:" + run + "\n\n"
	d := newScanner(strings.NewReader(in))
	for {
		l, err := d.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		if l.Len() > maxRun {
			require.FailNowf(t, "", "%s token of %d runes, more than %d", l.Type(), l.Len(), maxRun)
		}
	}
}
