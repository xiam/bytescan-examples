package main

import (
	"strings"
	"testing"

	"github.com/xiam/bytescan-examples/internal/lexdump"
	"github.com/xiam/bytescan/streamtest"
)

func TestGolden(t *testing.T) {
	lexdump.Golden(t, lexdump.Dumper{}, newScanner)
}

// TestTiling checks that the tokens of every test input, joined, are the
// input: no mode switch loses or repeats text.
func TestTiling(t *testing.T) {
	lexdump.Tiling(t, newScanner)
}

// TestRelease checks rule 1 of the driver: a cursor that retains at most 1 KiB
// lexes 4 KiB of code after a string, a percent literal and a heredoc, because
// their idle scanners were released and pin none of it.
func TestRelease(t *testing.T) {
	src := "s = \"a#{b}c\" + %w[x y] + <<~A\n  body\n  A\n" + strings.Repeat("x = y + 1 # c\n", 300)
	streamtest.LexRetained(t, strings.NewReader(src), 1<<10, newDriver)
}

// TestScannerCache checks that the driver builds one percent scanner per kind,
// delimiter and nesting, and one heredoc scanner per terminator and indent, and
// reuses them when the same literal comes again.
func TestScannerCache(t *testing.T) {
	src := "%w[a [b]] %w[c] %q[d] %w|e| %w|f|\n" +
		"x = <<A + <<~A + <<A\na\nA\nb\n  A\nc\nA\n"
	d := newScanner(strings.NewReader(src))
	streamtest.LexAll(t, d)
	if got, want := d.percents.Len(), 4; got != want {
		t.Errorf("cached percent scanners = %d, want %d", got, want)
	}
	if got, want := d.heredocs.Len(), 2; got != want {
		t.Errorf("cached heredoc scanners = %d, want %d", got, want)
	}
}
