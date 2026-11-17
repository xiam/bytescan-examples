// Package guardtest checks the production per-token guard of a stream
// example from both sides: input that never ends a record keeps producing
// bounded pieces and never reaches the guard, and a candidate that grows past
// the guard fails with the guard's error.
package guardtest

import (
	"bytes"
	"errors"
	"io"
	"math/rand"
	"testing"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/cursor"
	"github.com/xiam/bytescan/rules/core"
	"github.com/xiam/bytescan/streamtest"
)

// Filler returns a record function that writes 1 KiB of random bytes per
// record, none of them one of except: a stream that never ends a record when
// except holds the delimiters of the example.
func Filler(except ...byte) streamtest.RecordFunc {
	return func(rng *rand.Rand, _ int, dst []byte) []byte {
		for n := 0; n < 1<<10; {
			if c := byte(rng.Intn(256)); !bytes.Contains(except, []byte{c}) {
				dst, n = append(dst, c), n+1
			}
		}
		return dst
	}
}

// Bounded lexes src to the end with the production scanner that newScanner
// builds, through streamtest.Run: it fails t on any error, the guard error
// included, and when the live heap grows past the ceiling. It also fails t
// when a token is longer than maxLen bytes, or when fewer than two tokens of
// type piece come out, which would mean that the stream did not keep
// producing pieces. The options go to streamtest.Run, such as a ceiling for
// an example that holds a large piece; they must not set a token function.
func Bounded[L bytescan.Source](t *testing.T, newScanner func(io.Reader) L, src streamtest.Source, maxLen int, piece bytescan.TokenType, opts ...streamtest.Option) {
	t.Helper()
	var longest int
	opts = append(opts, streamtest.WithTokenFunc(func(l *bytescan.Token) {
		longest = max(longest, l.Len())
	}))
	res := streamtest.Run(t, newScanner, src, opts...)
	if longest > maxLen {
		t.Errorf("the longest token is %d bytes, want at most %d", longest, maxLen)
	}
	if n := res.Counts[piece]; n < 2 {
		t.Errorf("%d %s tokens, want a stream of pieces", n, piece)
	}
}

// Probe is the type of the rule that Active adds.
const Probe bytescan.TokenType = "PROBE"

// Active adds to lx, a scanner of a production driver, a rule for a run of any
// length of any rune: the candidate that grows past the guard, which no rule
// of a bounded example has. Then it calls next, the Next of the driver, until
// lx has run on input longer than the guard, and fails t unless that ends in
// an error that matches cursor.ErrRetentionExceeded before a token of
// more than guard bytes, or the end of the input, comes out.
func Active(t *testing.T, lx *bytescan.Scanner, guard int, next func() (*bytescan.Token, error)) {
	t.Helper()
	lx.MustAddRule(Probe, core.RuneRun(core.AnyRune))
	for {
		l, err := next()
		if errors.Is(err, cursor.ErrRetentionExceeded) {
			return
		}
		if err != nil {
			t.Fatalf("Next: %v, want an error that matches cursor.ErrRetentionExceeded", err)
		}
		if l.Len() > guard {
			t.Fatalf("%s token of %d bytes passed the guard of %d bytes", l.Type(), l.Len(), guard)
		}
	}
}
