package main

import (
	"io"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/xiam/bytescan-examples/internal/lexdump"
	"github.com/xiam/bytescan/streamtest"
)

func TestGolden(t *testing.T) {
	lexdump.Golden(t, lexdump.Dumper{}, newScanner)
}

// TestGuard checks that every mode scanner is bounded by the per-token guard of
// the stream harness, and that the guard holds the largest piece: maxRun runes
// of 4 bytes each.
func TestGuard(t *testing.T) {
	if maxTokenBytes != streamtest.TextGuard {
		t.Fatalf("maxTokenBytes = %d, want streamtest.TextGuard (%d)", maxTokenBytes, streamtest.TextGuard)
	}
	if maxRun*utf8.UTFMax >= maxTokenBytes {
		t.Fatalf("a piece of %d runes may take %d bytes, not below the guard of %d", maxRun, maxRun*utf8.UTFMax, maxTokenBytes)
	}
	// One line of four-byte runes, each run maxRun+1 runes long, in every
	// position: a name, a value and a comment.
	run := strings.Repeat("\U0001F600", maxRun+1)
	in := run + ":" + run + "\n:" + run + "\n\n"
	var got strings.Builder
	d := newScanner(strings.NewReader(in))
	for {
		l, err := d.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		got.WriteString(l.Text())
	}
	if got.String() != in {
		t.Fatal("the tokens do not join to the input")
	}
}
