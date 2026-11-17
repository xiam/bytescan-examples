package main

import (
	"io"
	"testing"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan-examples/internal/lexdump"
)

func TestGolden(t *testing.T) {
	lexdump.Golden(t, lexdump.Dumper{}, newScanner)
}

// TestDropComments checks the filtered view. Its output must equal the golden
// output of each input with the COMMENT lines removed, so no second set of
// golden files is needed.
func TestDropComments(t *testing.T) {
	lexdump.GoldenWithout(t, func(r io.Reader) bytescan.Source { return withoutComments(newScanner(r)) }, func(typ bytescan.TokenType) bool {
		return typ == comment
	})
}
