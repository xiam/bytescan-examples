package main

import (
	"bytes"
	"math/rand"
	"testing"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/streamtest"
)

// TestLongRun lexes an endless stream of both framings of the default
// budget, or of 1 GiB with -streamtest.long, under the harness ceiling, and
// checks the token counts against the messages:
//
//	go test -timeout 0 -run Long ./streams/syslog -streamtest.long
func TestLongRun(t *testing.T) {
	gen := streamtest.NewGenerator(1, syslogRecord)
	res := streamtest.Run(t, newScanner, gen)
	records := int64(gen.Records())
	if got := res.Counts[pri]; got != records {
		t.Errorf("lexed %d PRI, want one per message (%d)", got, records)
	}
	// An octet-counted frame starts with MSG_LEN, a newline-ended one ends
	// with LF; a MSG read by length holds its LF bytes.
	if got := res.Counts[msgLen] + res.Counts[lf]; got != records || res.Counts[msgLen] == 0 || res.Counts[lf] == 0 {
		t.Errorf("lexed %d MSG_LEN and %d LF, want one of them per message (%d)", res.Counts[msgLen], res.Counts[lf], records)
	}
	if got := res.Counts[unknown]; got != 0 {
		t.Errorf("lexed %d UNKNOWN in a clean stream", got)
	}
	if res.Bytes != gen.Bytes() {
		t.Errorf("Run read %d bytes, the generator wrote %d", res.Bytes, gen.Bytes())
	}
}

// garbage appends a garbage line: an optional run of digits that is not a
// MSG_LEN, bytes that start no frame and match no rule (no '<', digit, SP or
// LF), and the LF that ends the line.
func garbage(rng *rand.Rand, dst []byte) []byte {
	const junk = "!#$%&*?@^~|\x00\x01\x07\x1b\x7f\x80\xfe\xff"
	if rng.Intn(2) == 0 {
		for n := 11 + rng.Intn(30); n > 0; n-- {
			dst = append(dst, byte('0'+rng.Intn(10)))
		}
	}
	for n := 1 + rng.Intn(16); n > 0; n-- {
		dst = append(dst, junk[rng.Intn(len(junk))])
	}
	return append(dst, '\n')
}

// TestCorruption injects garbage lines between messages. Each line lexes
// as UNKNOWN tokens and the LF that ends it, and lexing is correct again
// from the next frame: a MSG_LEN or a PRI.
func TestCorruption(t *testing.T) {
	gen := streamtest.NewGenerator(3, syslogRecord)
	c := streamtest.Corrupt(gen, 4, 0.05, garbage)
	var (
		cur    streamtest.Injection
		have   bool
		inside int64 // UNKNOWN tokens inside garbage
	)
	check := func(l *bytescan.Token) {
		off := int64(l.Offset())
		end := cur.Offset + int64(cur.Len)
		if !have || off >= end {
			if have && off == end && l.Type() != msgLen && l.Type() != pri {
				t.Fatalf("after the garbage at %d: got %s %q, want MSG_LEN or PRI", cur.Offset, l.Type(), l.Text())
			}
			cur, have = c.NextInjection()
			end = cur.Offset + int64(cur.Len)
		}
		in := have && off >= cur.Offset && off < end
		switch {
		case in && off+int64(l.Len()) > end:
			t.Fatalf("at %d: %s crosses the end of the garbage at %d", off, l.Type(), end)
		case in && off+int64(l.Len()) == end:
			if l.Type() != lf {
				t.Fatalf("at %d: got %s at the end of the garbage, want LF", off, l.Type())
			}
		case in:
			if l.Type() != unknown {
				t.Fatalf("at %d: got %s %q inside the garbage, want UNKNOWN", off, l.Type(), l.Text())
			}
			inside++
		case l.Type() == unknown:
			t.Fatalf("at %d: UNKNOWN %q outside the garbage", off, l.Text())
		}
	}
	res := streamtest.Run(t, newScanner, c, streamtest.WithTokenFunc(check))
	if res.Counts[unknown] != inside || inside == 0 {
		t.Fatalf("lexed %d UNKNOWN, %d inside garbage", res.Counts[unknown], inside)
	}
	if got, want := res.Counts[pri], int64(gen.Records()); got != want {
		t.Fatalf("lexed %d PRI, want one per message (%d)", got, want)
	}
	t.Logf("%d garbage lines, %d UNKNOWN tokens", c.Injected(), inside)
}

// warmUp is the size of the messages that a test lexes before its run.
// The first messages grow the live heap once, by about 300 KiB, as the scanners
// and the cursor allocate on first use. The tests sample every warmUp bytes,
// so the baseline, the first sample, comes after the warm-up.
const warmUp = 4 << 10

// withRun returns the source of warmUp bytes of messages, run, and the
// stream of gen, and the offset of run in it.
func withRun(run []byte, gen *streamtest.Generator) (streamtest.Source, int64) {
	head := streamtest.Records(99, syslogRecord, warmUp)
	return streamtest.Prefix(gen, head, run), int64(len(head))
}

// highRun returns n bytes without LF, 15 of every 16 of them 0x80 or above.
func highRun(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		if i%16 < 15 {
			b[i] = byte(0x80 + i%0x80)
		} else {
			b[i] = byte('a' + i%26)
		}
	}
	return b
}

// TestUndelimitedRun lexes one run of 18 KiB with no delimiter in each field
// that runs to one. Each lexes as pieces of at most maxRun bytes under the
// harness ceiling and the per-token guard, and the next message lexes. The MSG
// and SD_PARAM_VALUE runs have 16.9 KiB of bytes of 0x80 or above, so the guard
// is shown not to reject a legal piece of such bytes.
func TestUndelimitedRun(t *testing.T) {
	const n = 18 << 10
	cases := []struct {
		name  string
		typ   bytescan.TokenType
		piece int // the length of a full piece
		head  string
		run   []byte
		tail  string
	}{
		{"MSG", msg, maxRun, "<1>1 - - - - - - ", highRun(n), "\n"},
		{"HOSTNAME", hostname, maxRun, "<1>1 - ", bytes.Repeat([]byte("h"), n), " - - - -\n"},
		{"SD_PARAM_VALUE", sdParamValue, maxRun / 2, `<1>1 - - - - - [x y="`, highRun(n), "\"]\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gen := streamtest.NewGenerator(5, syslogRecord, streamtest.WithBudget(2<<10))
			run := append(append([]byte(tc.head), tc.run...), tc.tail...)
			src, at := withRun(run, gen)
			at += int64(len(tc.head))
			var got int64
			check := func(l *bytescan.Token) {
				if l.Type() == unknown {
					t.Fatalf("at %d: UNKNOWN %q", l.Offset(), l.Text())
				}
				off := int64(l.Offset())
				if off < at || off >= at+n {
					return
				}
				if l.Type() != tc.typ || off != at+got || l.Len() > tc.piece || (got+int64(l.Len()) < n && l.Len() != tc.piece) {
					t.Fatalf("at %d: %s of %d bytes, want a %s piece of %d bytes at %d", off, l.Type(), l.Len(), tc.typ, tc.piece, at+got)
				}
				got += int64(l.Len())
			}
			streamtest.Run(t, newScanner, src, streamtest.WithTokenFunc(check), streamtest.WithSampleEvery(warmUp))
			if got != n {
				t.Errorf("lexed %d bytes of %s, want %d", got, tc.typ, n)
			}
		})
	}
}

// TestDigitRun feeds a run of 20 KiB of digits at a frame start. It is not a
// MSG_LEN, so it lexes as UNKNOWN pieces of at most maxRun bytes, the rest of
// its line lexes as a newline-ended message, and every frame after the line
// lexes, under the harness ceiling and the per-token guard.
func TestDigitRun(t *testing.T) {
	const n = 20 << 10
	gen := streamtest.NewGenerator(6, syslogRecord, streamtest.WithBudget(2<<10))
	run := append(bytes.Repeat([]byte("7"), n), " <1>1 - - - - - - after the digits\n"...)
	src, at := withRun(run, gen)
	var got int64
	check := func(l *bytescan.Token) {
		if l.Type() != unknown {
			return
		}
		if int64(l.Offset()) != at+got || l.Len() > maxRun {
			t.Fatalf("at %d: UNKNOWN of %d bytes, want a piece of the digit run at %d", l.Offset(), l.Len(), at+got)
		}
		got += int64(l.Len())
	}
	var after []bytescan.TokenType // the first tokens after the line of the run
	end := at + int64(len(run))
	tail := func(l *bytescan.Token) {
		check(l)
		if int64(l.Offset()) >= end && len(after) < 2 {
			after = append(after, l.Type())
		}
	}
	res := streamtest.Run(t, newScanner, src, streamtest.WithTokenFunc(tail), streamtest.WithSampleEvery(warmUp))
	if got != n {
		t.Errorf("lexed %d bytes of UNKNOWN, want the %d digits", got, n)
	}
	if len(after) == 0 || (after[0] != msgLen && after[0] != pri) {
		t.Errorf("tokens after the run: %v, want a frame start", after)
	}
	if res.Counts[msgLen] == 0 {
		t.Error("lexed no MSG_LEN")
	}
}

// TestCorruptLength frames the stream with a length of 9,999,999,999 bytes,
// longer than the stream. The driver reads the rest of the stream as MSG
// pieces of at most maxPiece bytes, so memory follows the piece size and not
// the length, under the harness ceiling.
func TestCorruptLength(t *testing.T) {
	gen := streamtest.NewGenerator(7, syslogRecord)
	frame := []byte("9999999999 <1>1 - - - - - - ")
	src, at := withRun(frame, gen)
	body := at + int64(len(frame)) // the first byte of the MSG
	var got int64
	check := func(l *bytescan.Token) {
		off := int64(l.Offset())
		if off < body {
			return
		}
		if l.Type() != msg || off != body+got || l.Len() > maxPiece {
			t.Fatalf("at %d: %s of %d bytes, want a MSG piece of at most %d bytes at %d", off, l.Type(), l.Len(), maxPiece, body+got)
		}
		got += int64(l.Len())
	}
	res := streamtest.Run(t, newScanner, src, streamtest.WithTokenFunc(check), streamtest.WithSampleEvery(warmUp))
	if want := res.Bytes - body; got != want {
		t.Errorf("lexed %d bytes of MSG after the frame, want the rest of the stream (%d)", got, want)
	}
}
