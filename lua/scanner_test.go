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
// lexes 4 KiB of code after a long string and a long comment, because the
// idle level scanners were released and pin none of it.
func TestRelease(t *testing.T) {
	src := "s = [[a]] --[==[b]==]\n" + strings.Repeat("x = y + 1 -- c\n", 300)
	streamtest.LexRetained(t, strings.NewReader(src), 1<<10, newCursorScanner)
}

// TestLevelCache checks that the driver builds one level scanner per kind and
// level, and reuses it when the same kind and level comes again.
func TestLevelCache(t *testing.T) {
	d := newScanner(strings.NewReader("[[a]] [=[b]=] [[c]] --[[d]] --[=[e]=] --[[f]] [=[g]=]"))
	streamtest.LexAll(t, d)
	if got, want := d.levels.Len(), 4; got != want {
		t.Errorf("cached level scanners = %d, want %d", got, want)
	}
}
