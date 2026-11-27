// Package lextest provides golden-file and input checks for example tests.
// Production programs use lexdump without importing this test support package.
package lextest

import (
	"bytes"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan-examples/internal/lexdump"
)

// update is the -update test flag. It is registered only in test binaries, so
// an example program does not gain the flag.
var update = func() *bool {
	if testing.Testing() {
		return flag.Bool("update", false, "rewrite the golden files from the current output")
	}
	return new(bool)
}()

// Golden checks every testdata/*.input file of the calling package. For each
// input it builds a scanner with newScanner, dumps it with d, and compares the
// output with the .golden file of the same name. With go test -update it
// writes the output to the golden file first. It fails the test if there is no
// input file.
func Golden[L bytescan.Source](t *testing.T, d lexdump.Dumper, newScanner func(io.Reader) L) {
	t.Helper()
	inputs, err := filepath.Glob(filepath.Join("testdata", "*.input"))
	require.NoError(t, err)
	require.NotZero(t, len(inputs), "no testdata/*.input files")
	for _, input := range inputs {
		golden := strings.TrimSuffix(input, ".input") + ".golden"
		t.Run(filepath.Base(input), func(t *testing.T) {
			f, err := os.Open(input)
			require.NoError(t, err)
			defer f.Close()

			var got bytes.Buffer
			{
				err := d.Dump(&got, newScanner(f))
				require.NoErrorf(t, err, "dump %s: %v", input, err)
			}
			Compare(t, golden, got.Bytes())
		})
	}
}

// Compare checks got against the golden file golden. With go test -update it
// writes got to the golden file first. An example whose output is not a token
// dump, such as the highlighter, calls it directly.
func Compare(t *testing.T, golden string, got []byte) {
	t.Helper()
	if *update {
		require.NoError(t, os.WriteFile(golden, got, 0o644))
	}
	want, err := os.ReadFile(golden)
	require.NoErrorf(t, err, "%v (run go test -update to create it)", err)
	assert.Truef(t, bytes.Equal(got, want), "output differs from %s; review it, then run go test -update\n--- got ---\n%s--- want ---\n%s", golden, got, want)
}
