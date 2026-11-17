package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan-examples/internal/lexdump"
)

func TestGolden(t *testing.T) {
	lexdump.Golden(t, lexdump.Dumper{}, newScanner)
}

// TestStrip checks the -strip view: the coloured sample becomes its plain
// text, the committed colours.txt.
func TestStrip(t *testing.T) {
	in, err := os.ReadFile(filepath.Join("testdata", "colours.input"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "colours.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var got bytes.Buffer
	if err := writeText(&got, stripped(newScanner(bytes.NewReader(in)))); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), want) {
		t.Errorf("stripped colours.input differs from colours.txt\n--- got ---\n%q\n--- want ---\n%q", got.Bytes(), want)
	}
}

// TestStripKeepsText checks the filter on every input against its golden
// file: the stripped output is the text of the TEXT, TAB, LF and CR lines, in
// order, so no second set of golden files is needed.
func TestStripKeepsText(t *testing.T) {
	lexdump.GoldenWithout(t, func(r io.Reader) bytescan.Source { return stripped(newScanner(r)) }, func(typ bytescan.TokenType) bool {
		switch typ {
		case text, tab, lf, cr:
			return false
		}
		return true
	})
}
