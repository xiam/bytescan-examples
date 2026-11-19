package lexdump

import (
	"bytes"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiam/bytescan"
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
func Golden[L bytescan.Source](t *testing.T, d Dumper, newScanner func(io.Reader) L) {
	t.Helper()
	inputs, err := filepath.Glob(filepath.Join("testdata", "*.input"))
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) == 0 {
		t.Fatal("no testdata/*.input files")
	}
	for _, input := range inputs {
		golden := strings.TrimSuffix(input, ".input") + ".golden"
		t.Run(filepath.Base(input), func(t *testing.T) {
			f, err := os.Open(input)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()

			var got bytes.Buffer
			if err := d.Dump(&got, newScanner(f)); err != nil {
				t.Fatalf("dump %s: %v", input, err)
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
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("%v (run go test -update to create it)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("output differs from %s; review it, then run go test -update\n--- got ---\n%s--- want ---\n%s", golden, got, want)
	}
}
