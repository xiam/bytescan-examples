package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan-examples/internal/lexdump"
	"github.com/xiam/bytescan-examples/internal/streamkit"
	"github.com/xiam/bytescan/bytesource"
	"github.com/xiam/bytescan/streamtest"
)

func TestGolden(t *testing.T) {
	lexdump.Golden(t, dumper, newScanner)
}

// types returns the types of ls, separated by blanks.
func types(ls []*bytescan.Token) string {
	s := make([]string, len(ls))
	for i, l := range ls {
		s[i] = string(l.Type())
	}
	return strings.Join(s, " ")
}

// TestPayloadPieces checks that a payload longer than 64 KiB is emitted as
// PAYLOAD tokens of at most 64 KiB, and headers as HEADERS tokens, that
// together hold every byte, and that the next line lexes after them.
func TestPayloadPieces(t *testing.T) {
	body := bytes.Repeat([]byte("\xff\r\nMSG "), 130<<10/6)
	hdr := bytes.Repeat([]byte("H: \x80\r\n"), 70<<10/6)
	in := msg("big", "1", "", string(body)) + hmsg("big.h", "2", "", string(hdr), "body") + "PING\r\n"
	ls := streamtest.LexAll(t, newScanner(strings.NewReader(in)))

	var gotBody, gotHdr [][]byte
	for _, l := range ls {
		if l.Len() > maxPiece {
			t.Fatalf("%s at %d is %d bytes, want at most %d", l.Type(), l.Offset(), l.Len(), maxPiece)
		}
		switch l.Type() {
		case payload:
			gotBody = append(gotBody, bytesource.Bytes(l.Text()))
		case headers:
			gotHdr = append(gotHdr, bytesource.Bytes(l.Text()))
		}
	}
	if want := 3 + 1; len(gotBody) != want {
		t.Fatalf("%d PAYLOAD tokens, want %d", len(gotBody), want)
	}
	if !bytes.Equal(bytes.Join(gotBody[:3], nil), body) || string(gotBody[3]) != "body" {
		t.Fatal("the PAYLOAD tokens do not hold the payload")
	}
	if len(gotHdr) != 2 || !bytes.Equal(bytes.Join(gotHdr, nil), hdr) {
		t.Fatalf("%d HEADERS tokens, want 2 that hold the headers", len(gotHdr))
	}
	if tail := types(ls[len(ls)-3:]); tail != "CRLF OP CRLF" {
		t.Fatalf("tokens after the payload are %s, want CRLF OP CRLF", tail)
	}
}

// TestReleaseBeforeRead checks rule 1 of the driver: the active scanner is
// released before a payload is read, so a cursor whose retention is bounded
// to 4 KiB lexes a payload of 100 KiB. Without the Release, the scanner's
// checkpoints would pin the payload and the cursor would fail.
func TestReleaseBeforeRead(t *testing.T) {
	body := strings.Repeat("payload ", 100<<10/8)
	in := msg("a", "1", "", body) + "PING\r\n"
	ls := streamtest.LexRetained(t, bytesource.NewReader(strings.NewReader(in)), 4<<10, func(cur bytescan.Cursor) *driver {
		return newDriver(cur, bytescan.WithMaxTokenBytes(tokenGuard))
	})
	want := "OP SP SUBJECT SP SID SP SIZE CRLF PAYLOAD PAYLOAD CRLF OP CRLF"
	if got := types(ls); got != want {
		t.Fatalf("tokens = %s, want %s", got, want)
	}
}

// TestByteFraming checks why the example uses the byte adapter: a payload
// of valid multi-byte UTF-8 is framed by its byte count, so the next control
// line starts right after it.
func TestByteFraming(t *testing.T) {
	body := "日本語 ✓"
	ls := streamtest.LexAll(t, newScanner(strings.NewReader(msg("u", "1", "", body)+"PONG\r\n")))
	want := "OP SP SUBJECT SP SID SP SIZE CRLF PAYLOAD CRLF OP CRLF"
	if got := types(ls); got != want {
		t.Fatalf("tokens = %s, want %s", got, want)
	}
	if got := string(bytesource.Bytes(ls[8].Text())); got != body {
		t.Fatalf("PAYLOAD = %q, want %q", got, body)
	}
}

// TestLookaheadGuard checks that the longest lookahead stays under the
// per-token guard: a queue group of 1000 bytes of 0x80 or above, then blanks
// and a SID. With at most 40 blanks the field is a QUEUE_GROUP, with more it
// is a SID, and in each case Next returns no error and the next line lexes.
// It also lexes the longest lookahead of a size.
func TestLookaheadGuard(t *testing.T) {
	tests := []struct {
		high, blanks int
		first        bytescan.TokenType
	}{
		{1000, 1, queueGroup},
		{1000, 40, queueGroup},
		{1000, 41, sid},
		{1000, 1000, sid},
		{600, 1000, sid},
	}
	for _, tt := range tests {
		in := "SUB a " + strings.Repeat("\xff", tt.high) + strings.Repeat(" ", tt.blanks) + "1\r\nPING\r\n"
		ls := streamtest.LexAll(t, newScanner(strings.NewReader(in)))
		if got := ls[4].Type(); got != tt.first {
			t.Errorf("%d high bytes, %d blanks: the field is %s, want %s", tt.high, tt.blanks, got, tt.first)
		}
		if tail := types(ls[len(ls)-3:]); tail != "CRLF OP CRLF" {
			t.Errorf("%d high bytes, %d blanks: the stream ends with %s, want CRLF OP CRLF", tt.high, tt.blanks, tail)
		}
	}

	// The lookahead of sizePairRule at its bounds: 10 digits, 1000 blanks,
	// 10 digits, 1000 blanks and the CRLF.
	blanks := strings.Repeat(" ", maxRun)
	in := "HPUB a 0000000002" + blanks + "0000000003" + blanks + "\r\nabc\r\nPING\r\n"
	ls := streamtest.LexAll(t, newScanner(strings.NewReader(in)))
	want := "OP SP SUBJECT SP SIZE SP SIZE SP CRLF HEADERS PAYLOAD CRLF OP CRLF"
	if got := types(ls); got != want {
		t.Errorf("HPUB at the lookahead bounds: tokens = %s, want %s", got, want)
	}
}

// TestHeaderSegments checks that headers longer than streamkit.MaxSegment
// are read as several fields before the payload, never interleaved with it,
// for HMSG and HPUB, and that the next line lexes after them. The test lowers
// streamkit.MaxSegment so that the boundary is reached in a few bytes, as on
// a 32-bit platform with a header size above the largest int.
func TestHeaderSegments(t *testing.T) {
	defer func(n uint64) { streamkit.MaxSegment = n }(streamkit.MaxSegment)
	streamkit.MaxSegment = 5

	for _, op := range []string{"HMSG a 1", "HPUB a"} {
		in := op + " 11 14\r\nABCDEFGHIJKxyz\r\nPING\r\n"
		var hdr, body strings.Builder
		ls := streamtest.LexAll(t, newScanner(strings.NewReader(in)))
		for _, l := range ls {
			switch l.Type() {
			case headers:
				hdr.WriteString(l.Text())
			case payload:
				body.WriteString(l.Text())
			}
		}
		if hdr.String() != "ABCDEFGHIJK" || body.String() != "xyz" {
			t.Errorf("%s: HEADERS %q and PAYLOAD %q, want %q and %q", op, hdr.String(), body.String(), "ABCDEFGHIJK", "xyz")
		}
		if tail := types(ls[len(ls)-3:]); tail != "CRLF OP CRLF" {
			t.Errorf("%s: the stream ends with %s, want CRLF OP CRLF", op, tail)
		}
	}
}
