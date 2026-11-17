package lexdump

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/rules/core"
)

func newScanner(text string) *bytescan.Scanner {
	lx := bytescan.New(strings.NewReader(text))
	lx.MustAddRule("WORD", core.RuneRun(core.ASCIILetter))
	lx.MustAddRule("SPACE", core.RuneRun(core.ASCIISpace))
	return lx
}

func TestDump(t *testing.T) {
	var buf bytes.Buffer
	if err := (Dumper{}).Dump(&buf, newScanner("ab \"\ncd")); err != nil {
		t.Fatal(err)
	}
	want := "1:0 WORD \"ab\"\n1:2 SPACE \" \"\n1:3 UNKNOWN \"\\\"\"\n1:4 SPACE \"\\n\"\n2:0 WORD \"cd\"\n"
	if got := buf.String(); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestDumpSkipAndFormat(t *testing.T) {
	d := Dumper{
		Skip:   []bytescan.TokenType{"SPACE"},
		Format: func(l *bytescan.Token) string { return string(l.Type()) + " " + strings.ToUpper(l.Text()) },
	}
	var buf bytes.Buffer
	if err := d.Dump(&buf, newScanner("ab cd")); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), "WORD AB\nWORD CD\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// failing returns one token and then err.
type failing struct {
	err  error
	done bool
}

func (f *failing) Next() (*bytescan.Token, error) {
	if f.done {
		return nil, f.err
	}
	f.done = true
	return bytescan.NewTokenFromString("WORD", "a", 0), nil
}

func TestDumpError(t *testing.T) {
	boom := errors.New("boom")
	var buf bytes.Buffer
	if err := (Dumper{}).Dump(&buf, &failing{err: boom}); !errors.Is(err, boom) {
		t.Fatalf("got error %v, want %v", err, boom)
	}
	if got, want := buf.String(), "0:0 WORD \"a\"\n"; got != want {
		t.Errorf("lines before the error: got %q, want %q", got, want)
	}
}

func TestGolden(t *testing.T) {
	Golden(t, Dumper{}, func(r io.Reader) *bytescan.Scanner {
		b, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
		return newScanner(string(b))
	})
}

// newReaderScanner is newScanner over the bytes of r.
func newReaderScanner(t *testing.T) func(io.Reader) *bytescan.Scanner {
	return func(r io.Reader) *bytescan.Scanner {
		b, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
		return newScanner(string(b))
	}
}

func TestGoldenWithout(t *testing.T) {
	lx := newReaderScanner(t)
	GoldenWithout(t, func(r io.Reader) bytescan.Source { return bytescan.Skip(lx(r), "SPACE") }, func(typ bytescan.TokenType) bool {
		return typ == "SPACE"
	})
}

func TestTiling(t *testing.T) {
	Tiling(t, newReaderScanner(t))
}

func TestSequence(t *testing.T) {
	if got, want := Sequence(0xFE, 4), []byte{0xFE, 0xFF, 0x00, 0x01}; !bytes.Equal(got, want) {
		t.Errorf("Sequence(0xFE, 4) = % x, want % x", got, want)
	}
}
