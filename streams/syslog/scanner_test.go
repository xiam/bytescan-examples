package main

import (
	"bytes"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan-examples/internal/guardtest"
	"github.com/xiam/bytescan-examples/internal/lexdump"
	"github.com/xiam/bytescan/bytesource"
	"github.com/xiam/bytescan/streamtest"
)

func TestGolden(t *testing.T) {
	lexdump.Golden(t, dumper, newScanner)
}

// TestGuard checks the production guard: it is twice streamtest.ByteGuard;
// a newline-ended message that never ends, a header followed by any bytes
// but LF, keeps producing MSG pieces of at most maxRun bytes, with no guard
// error and bounded memory; and a candidate longer than the guard fails Next
// with the guard error.
func TestGuard(t *testing.T) {
	if tokenGuard != 2*streamtest.ByteGuard {
		t.Fatalf("tokenGuard = %d, want 2*streamtest.ByteGuard (%d)", tokenGuard, 2*streamtest.ByteGuard)
	}
	gen := streamtest.NewGenerator(80, guardtest.Filler('\n'))
	guardtest.Bounded(t, newScanner, streamtest.Prefix(gen, []byte("<1>1 - - - - - ")), maxRun, msg)

	d := newScanner(strings.NewReader(strings.Repeat("a", 2*tokenGuard)))
	guardtest.Active(t, d.scanners[modePRI], tokenGuard, d.Next)
}

// types returns the types of the tokens of input, separated by spaces.
func types(t *testing.T, input string) string {
	t.Helper()
	d := newScanner(strings.NewReader(input))
	var got []string
	for {
		l, err := d.Next()
		if err == io.EOF {
			return strings.Join(got, " ")
		}
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, string(l.Type()))
	}
}

// TestLengthCountsBytes checks that MSG_LEN counts bytes: a MSG of two
// two-byte runes ends after 4 bytes, and the next frame starts there.
func TestLengthCountsBytes(t *testing.T) {
	got := types(t, "21 <1>1 - - - - - - ñé<1>1 - - - - - -\n")
	want := "MSG_LEN SP PRI VERSION SP TIMESTAMP SP HOSTNAME SP APP_NAME SP PROCID SP MSGID SP STRUCTURED_DATA SP MSG " +
		"PRI VERSION SP TIMESTAMP SP HOSTNAME SP APP_NAME SP PROCID SP MSGID SP STRUCTURED_DATA LF"
	if got != want {
		t.Errorf("tokens =\n%s\nwant\n%s", got, want)
	}
}

// TestMsgPieces checks that a MSG framed by a declared length is read in
// pieces of at most maxPiece bytes, and that a length past the end of the
// input reads to the end.
func TestMsgPieces(t *testing.T) {
	header := "<1>1 - - - - - - "
	body := bytes.Repeat([]byte("x\xff\n"), (maxPiece+300)/3)
	frame := append([]byte(header), body...)
	input := append([]byte("1000000 "), frame...) // the length passes the end
	d := newScanner(bytes.NewReader(input))
	var pieces []int
	for {
		l, err := d.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if l.Type() == msg {
			pieces = append(pieces, l.Len())
		}
	}
	if len(pieces) != 2 || pieces[0] != maxPiece || pieces[1] != len(body)-maxPiece {
		t.Errorf("MSG pieces = %v, want %d, %d", pieces, maxPiece, len(body)-maxPiece)
	}
}

// TestReleaseBeforeRead checks rule 1 of the driver: the scanners are released
// before MSG is read by length, so a cursor whose retention is bounded to
// 1 KiB lexes a MSG of 64 KiB. Without the Release, the checkpoints of the
// header scanners would pin the MSG and the cursor would fail.
func TestReleaseBeforeRead(t *testing.T) {
	m := "<1>1 - - - - - - " + strings.Repeat("y", maxPiece)
	input := strconv.Itoa(len(m)) + " " + m + "<1>1 - - - - - - z\n"
	ls := streamtest.LexRetained(t, bytesource.NewReader(strings.NewReader(input)), 1<<10, func(cur bytescan.Cursor) *driver {
		return newDriver(cur, bytescan.WithMaxTokenBytes(tokenGuard))
	})
	n := 0
	for _, l := range ls {
		if l.Type() == msg {
			n++
		}
	}
	if n != 2 {
		t.Errorf("lexed %d MSG, want 2", n)
	}
}
