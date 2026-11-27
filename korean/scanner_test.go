package main

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xiam/bytescan-examples/internal/lexdump"
	"github.com/xiam/bytescan-examples/internal/lextest"
)

func TestGolden(t *testing.T) {
	lextest.Golden(t, lexdump.Dumper{}, newScanner)
}

// TestTwoEncodings checks that the input still holds 한글 in both encodings.
// The two lines look the same, so an editor that normalizes the file would
// silently drop the NFD case.
func TestTwoEncodings(t *testing.T) {
	b, err := os.ReadFile("testdata/sentences.input")
	require.NoError(t, err)
	for name, s := range map[string]string{
		"NFC": "\n한글\n",
		"NFD": "\n한글\n",
	} {
		assert.Truef(t, strings.Contains(string(b), s), "sentences.input has no %s line %q", name, s)
	}
}
