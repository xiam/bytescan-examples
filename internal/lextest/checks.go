package lextest

import (
	"bytes"
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

// inputFiles returns the testdata/*.input files of the calling package. It
// fails the test if there is none.
func inputFiles(t *testing.T) []string {
	t.Helper()
	inputs, err := filepath.Glob(filepath.Join("testdata", "*.input"))
	require.NoError(t, err)
	require.NotZero(t, len(inputs), "no testdata/*.input files")
	return inputs
}

// GoldenWithout checks a filtered view against the golden files, so the view
// needs no golden files of its own. For each testdata/*.input file it dumps
// newView over the input with Text, and compares the output with the lines of
// the .golden file whose type drop does not report. The golden files must be
// in the Text format, which has the type in its second field. -update does not
// write anything: the golden files belong to Golden.
func GoldenWithout[L bytescan.Source](t *testing.T, newView func(io.Reader) L, drop func(bytescan.TokenType) bool) {
	t.Helper()
	for _, input := range inputFiles(t) {
		golden := strings.TrimSuffix(input, ".input") + ".golden"
		t.Run(filepath.Base(input), func(t *testing.T) {
			f, err := os.Open(input)
			require.NoError(t, err)
			defer f.Close()

			var got bytes.Buffer
			{
				err := (lexdump.Dumper{}).Dump(&got, newView(f))
				require.NoErrorf(t, err, "dump %s: %v", input, err)
			}
			want, err := os.ReadFile(golden)
			require.NoErrorf(t, err, "%v (run go test -update to create it)", err)
			var kept strings.Builder
			for _, line := range strings.SplitAfter(string(want), "\n") {
				if fields := strings.Fields(line); len(fields) > 1 && !drop(bytescan.TokenType(fields[1])) {
					kept.WriteString(line)
				}
			}
			if got.String() != kept.String() {
				assert.Failf(t, "", "filtered output differs from %s without the dropped lines\n--- got ---\n%s--- want ---\n%s", golden, got.String(), kept.String())
			}
		})
	}
}

// Tiling checks that the tokens of every testdata/*.input file, joined, are
// the input: no token loses or repeats text, which a mode driver can do when
// it switches scanners.
func Tiling[L bytescan.Source](t *testing.T, newScanner func(io.Reader) L) {
	t.Helper()
	for _, input := range inputFiles(t) {
		src, err := os.ReadFile(input)
		require.NoError(t, err)
		lx := newScanner(bytes.NewReader(src))
		var got strings.Builder
		for {
			l, err := lx.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				require.FailNowf(t, "", "%s: after %d bytes: %v", input, got.Len(), err)
			}
			got.WriteString(l.Text())
		}
		if got.String() != string(src) {
			assert.Failf(t, "", "%s: tokens join to %q, want the input", input, got.String())
		}
	}
}

// Input is the bytes of one test input, or a function that builds them, with
// or without the test.
type Input interface {
	[]byte | func() []byte | func(*testing.T) []byte
}

// Inputs checks that each committed testdata/<name>.input file equals the
// bytes that inputs holds or builds for name, so a binary input is
// reproducible from the code that describes it. With go test -update it writes
// the built bytes to the file first, before Golden rewrites the golden files.
func Inputs[I Input](t *testing.T, inputs map[string]I) {
	t.Helper()
	for name, in := range inputs {
		path := filepath.Join("testdata", name+".input")
		var got []byte
		switch in := any(in).(type) {
		case []byte:
			got = in
		case func() []byte:
			got = in()
		case func(*testing.T) []byte:
			got = in(t)
		}
		if *update {
			require.NoError(t, os.WriteFile(path, got, 0o644))
		}
		want, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Truef(t, bytes.Equal(got, want), "%s differs from the bytes built in inputs_test.go; run go test -update", path)
	}
}

// Sequence returns n bytes counting up from first, wrapping at 0xFF, so a
// test input shows where a field is cut.
func Sequence(first byte, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = first + byte(i)
	}
	return b
}
