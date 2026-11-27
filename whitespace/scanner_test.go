package main

import (
	"io"
	"testing"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan-examples/internal/lexdump"
	"github.com/xiam/bytescan-examples/internal/lextest"
)

func TestGolden(t *testing.T) {
	lextest.Golden(t, lexdump.Dumper{}, newScanner)
}

// TestDropComments checks the filtered view. Its output must equal the golden
// output of each input with the COMMENT lines removed, so no second set of
// golden files is needed.
func TestDropComments(t *testing.T) {
	lextest.GoldenWithout(t, func(r io.Reader) bytescan.Source { return withoutComments(newScanner(r)) }, func(typ bytescan.TokenType) bool {
		return typ == comment
	})
}
