package main

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"strings"
	"testing"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan-examples/internal/guardtest"
	"github.com/xiam/bytescan-examples/internal/hexdump"
	"github.com/xiam/bytescan-examples/internal/lexdump"
	"github.com/xiam/bytescan/bytesource"
	"github.com/xiam/bytescan/streamtest"
)

func TestGolden(t *testing.T) {
	lexdump.Golden(t, hexdump.Dumper, newScanner)
}

// TestGuard checks the production guard: it is twice streamtest.ByteGuard;
// a stream of data bytes with no END keeps producing DATA pieces of at most
// maxRun bytes, with no guard error and bounded memory; and a candidate
// longer than the guard fails Next with the guard error.
func TestGuard(t *testing.T) {
	if tokenGuard != 2*streamtest.ByteGuard {
		t.Fatalf("tokenGuard = %d, want 2*streamtest.ByteGuard (%d)", tokenGuard, 2*streamtest.ByteGuard)
	}
	guardtest.Bounded(t, newScanner, streamtest.NewGenerator(80, undelimitedRecord()), maxRun, dataType)

	lx := newScanner(bytes.NewReader(bytes.Repeat([]byte{0xFF}, tokenGuard)))
	guardtest.Active(t, lx, tokenGuard, lx.Next)
}

// tokens returns the tokens of input as "TYPE:hex" words separated by
// spaces.
func tokens(t *testing.T, input []byte) string {
	t.Helper()
	lx := newScanner(bytes.NewReader(input))
	var got []string
	for {
		l, err := lx.Next()
		if err == io.EOF {
			return strings.Join(got, " ")
		}
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, fmt.Sprintf("%s:%x", l.Type(), bytesource.Bytes(l.Text())))
	}
}

// TestEscapes checks each escape case of RFC 1055 and the UNKNOWN cases.
func TestEscapes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"escaped END", "a\xDB\xDCb", "DATA:61 ESC_END:dbdc DATA:62"},
		{"escaped ESC", "\xDB\xDD", "ESC_ESC:dbdd"},
		{"empty frames", "\xC0\xC0\xC0", "END:c0 END:c0 END:c0"},
		{"ESC and another byte", "\xDBA", "UNKNOWN:db DATA:41"},
		{"ESC before END", "\xDB\xC0x", "UNKNOWN:db END:c0 DATA:78"},
		{"ESC before an escape", "\xDB\xDB\xDD", "UNKNOWN:db ESC_ESC:dbdd"},
		{"ESC at the end", "x\xDB", "DATA:78 UNKNOWN:db"},
		{"ESC_END and ESC_ESC bytes alone", "\xDC\xDD", "DATA:dcdd"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tokens(t, []byte(tt.input)); got != tt.want {
				t.Errorf("tokens = %s, want %s", got, tt.want)
			}
		})
	}
}

// undelimitedRecord returns a streamtest.RecordFunc of 1 KiB records of data
// bytes, so the stream is one run with no END or ESC in it. Even records hold
// bytes of 0x80 and above, odd records bytes below 0x80.
func undelimitedRecord() streamtest.RecordFunc {
	return func(rng *rand.Rand, i int, dst []byte) []byte {
		for n := 0; n < 1<<10; {
			c := byte(rng.Intn(0x80))
			if i%2 == 0 {
				c |= 0x80
			}
			if c == end || c == esc {
				continue
			}
			dst = append(dst, c)
			n++
		}
		return dst
	}
}

// TestUndelimitedRun lexes one run of data bytes with no END in it, of the
// harness default budget (at least 64 KiB), half of it bytes of 0x80 and
// above: every token is DATA, every piece but the last is exactly maxRun
// bytes, memory stays under the harness ceiling, and the per-token guard of
// 2,048 bytes never rejects a piece of 1,000 high bytes (2,000 guard bytes).
func TestUndelimitedRun(t *testing.T) {
	gen := streamtest.NewGenerator(55, undelimitedRecord())
	var (
		pos  int64
		high int64
		last = -1
	)
	res := streamtest.Run(t, newScanner, gen, streamtest.WithTokenFunc(func(l *bytescan.Token) {
		if l.Type() != dataType {
			t.Fatalf("%s at %d, want only DATA", l.Type(), l.Offset())
		}
		if int64(l.Offset()) != pos {
			t.Fatalf("DATA at %d, want the next token at %d", l.Offset(), pos)
		}
		if last >= 0 && last != maxRun {
			t.Fatalf("DATA of %d bytes before the one at %d, want %d: only the last piece is shorter", last, pos, maxRun)
		}
		if l.Len() > maxRun {
			t.Fatalf("DATA of %d bytes at %d, want at most %d", l.Len(), pos, maxRun)
		}
		for _, r := range l.Text() {
			if r >= 0x80 {
				high++
			}
		}
		last = l.Len()
		pos += int64(l.Len())
	}))
	if pos != res.Bytes {
		t.Errorf("tokens end at %d, want the end of the stream at %d", pos, res.Bytes)
	}
	if high < 16<<10 {
		t.Errorf("%d bytes of 0x80 or above, want at least 16 KiB", high)
	}
	if want := (res.Bytes + maxRun - 1) / maxRun; res.Counts[dataType] != want {
		t.Errorf("DATA tokens = %d, want %d", res.Counts[dataType], want)
	}
}
