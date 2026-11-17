package main

import (
	"os"
	"strings"
	"testing"

	"github.com/xiam/bytescan-examples/internal/lexdump"
)

func TestGolden(t *testing.T) {
	lexdump.Golden(t, lexdump.Dumper{}, newScanner)
}

// TestTwoEncodings checks that the input still holds 한글 in both encodings.
// The two lines look the same, so an editor that normalizes the file would
// silently drop the NFD case.
func TestTwoEncodings(t *testing.T) {
	b, err := os.ReadFile("testdata/sentences.input")
	if err != nil {
		t.Fatal(err)
	}
	for name, s := range map[string]string{
		"NFC": "\n한글\n",
		"NFD": "\n한글\n",
	} {
		if !strings.Contains(string(b), s) {
			t.Errorf("sentences.input has no %s line %q", name, s)
		}
	}
}
