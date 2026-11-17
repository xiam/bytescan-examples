package main

import (
	"math/rand"
	"testing"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/streamtest"
)

// counts is what a generated stream holds, as the record function wrote it.
type counts struct {
	ends, escEnds, escEscs int64
}

// record returns a streamtest.RecordFunc that appends one frame the way RFC
// 1055 sends it: sometimes an END first (an empty frame on the wire), then the
// stuffed payload, then END. A payload is 0 to 299 bytes, or 1,000 to 2,999
// bytes for one frame in 16, so long runs are lexed in several DATA pieces.
// Its bytes take every value, and about one in 8 is END or ESC, which the
// record stuffs. The record function adds what it wrote to c.
func record(c *counts) streamtest.RecordFunc {
	var payload []byte
	return func(rng *rand.Rand, i int, dst []byte) []byte {
		if rng.Intn(8) == 0 {
			dst = append(dst, end)
			c.ends++
		}
		n := rng.Intn(300)
		if rng.Intn(16) == 0 {
			n = 1000 + rng.Intn(2000)
		}
		payload = payload[:0]
		for j := 0; j < n; j++ {
			switch rng.Intn(16) {
			case 0:
				payload = append(payload, end)
				c.escEnds++
			case 1:
				payload = append(payload, esc)
				c.escEscs++
			default:
				payload = append(payload, byte(rng.Intn(256)))
				switch payload[len(payload)-1] {
				case end:
					c.escEnds++
				case esc:
					c.escEscs++
				}
			}
		}
		c.ends++
		return append(stuff(dst, payload), end)
	}
}

// checkToken fails t when l is not a token the scanner can emit: an END or an
// UNKNOWN of 1 byte, an escape sequence of 2 bytes, or DATA of 1 to maxRun
// bytes.
func checkToken(t *testing.T, l *bytescan.Token) {
	t.Helper()
	ok := false
	switch l.Type() {
	case endType, bytescan.TokenTypeUnknown:
		ok = l.Len() == 1
	case escEndType, escEscType:
		ok = l.Len() == 2
	case dataType:
		ok = l.Len() >= 1 && l.Len() <= maxRun
	}
	if !ok {
		t.Fatalf("%s of %d bytes at %d", l.Type(), l.Len(), l.Offset())
	}
}

// TestLongRun lexes a generated stream of frames: the tokens cover the
// stream without a gap, there is one END, ESC_END and ESC_ESC token for each
// that the generator wrote, no UNKNOWN, and memory stays under the harness
// ceiling.
func TestLongRun(t *testing.T) {
	var want counts
	gen := streamtest.NewGenerator(55, record(&want))
	var pos int64
	res := streamtest.Run(t, newScanner, gen, streamtest.WithTokenFunc(func(l *bytescan.Token) {
		checkToken(t, l)
		if int64(l.Offset()) != pos {
			t.Fatalf("%s at %d, want the next token at %d", l.Type(), l.Offset(), pos)
		}
		pos += int64(l.Len())
	}))
	if pos != res.Bytes {
		t.Errorf("tokens end at %d, want the end of the stream at %d", pos, res.Bytes)
	}
	got := counts{res.Counts[endType], res.Counts[escEndType], res.Counts[escEscType]}
	if got != want {
		t.Errorf("END, ESC_END, ESC_ESC tokens = %+v, want %+v", got, want)
	}
	if n := res.Counts[bytescan.TokenTypeUnknown]; n != 0 {
		t.Errorf("UNKNOWN tokens = %d, want 0", n)
	}
	if res.Counts[dataType] <= int64(gen.Records()) {
		t.Errorf("DATA tokens = %d for %d frames, want long payloads in several pieces", res.Counts[dataType], gen.Records())
	}
}

// garbage appends 1 to 50 pairs of bytes: an ESC, then a byte that does not
// finish an escape sequence and is not END or ESC. Each ESC of a pair is one
// UNKNOWN token; the byte after it is DATA.
func garbage(rng *rand.Rand, dst []byte) []byte {
	for n := 1 + rng.Intn(50); n > 0; n-- {
		c := byte(rng.Intn(256))
		for c == end || c == esc || c == escEnd || c == escEsc {
			c = byte(rng.Intn(256))
		}
		dst = append(dst, esc, c)
	}
	return dst
}

// TestCorruption inserts garbage runs of invalid escapes between frames, so a
// run lands at the start of the frame after it, as line noise does on a
// serial line. Every UNKNOWN token is an ESC of a garbage run, at an even
// offset in the run, and every ESC of a run is UNKNOWN. The tokens cover the
// stream without a gap, and there is one END, ESC_END and ESC_ESC token for
// each that the generator wrote: the END that closes a corrupted frame is
// lexed as END, and lexing goes on unchanged from there.
func TestCorruption(t *testing.T) {
	var want counts
	gen := streamtest.NewGenerator(55, record(&want))
	src := streamtest.Corrupt(gen, 7, 0.1, garbage)

	var (
		pending  []streamtest.Injection // injections the tokens have not passed
		pos      int64
		unknowns int64 // UNKNOWN tokens the injections hold
		runs     int
	)
	drain := func() {
		for {
			in, ok := src.NextInjection()
			if !ok {
				return
			}
			pending = append(pending, in)
			unknowns += int64(in.Len / 2)
			runs++
		}
	}
	res := streamtest.Run(t, newScanner, src, streamtest.WithTokenFunc(func(l *bytescan.Token) {
		checkToken(t, l)
		off := int64(l.Offset())
		if off != pos {
			t.Fatalf("%s at %d, want the next token at %d", l.Type(), off, pos)
		}
		pos += int64(l.Len())
		drain()
		for len(pending) > 0 && pending[0].Offset+int64(pending[0].Len) <= off {
			pending = pending[1:]
		}
		inGarbage := len(pending) > 0 && off >= pending[0].Offset
		escInGarbage := inGarbage && (off-pending[0].Offset)%2 == 0
		if (l.Type() == bytescan.TokenTypeUnknown) != escInGarbage {
			t.Fatalf("%s at %d, garbage %+v: want UNKNOWN exactly at each ESC of a garbage run", l.Type(), off, pending)
		}
	}))
	if pos != res.Bytes {
		t.Errorf("tokens end at %d, want the end of the stream at %d", pos, res.Bytes)
	}
	if n := res.Counts[bytescan.TokenTypeUnknown]; n != unknowns || n == 0 {
		t.Errorf("UNKNOWN tokens = %d, want %d, more than 0", n, unknowns)
	}
	got := counts{res.Counts[endType], res.Counts[escEndType], res.Counts[escEscType]}
	if got != want {
		t.Errorf("END, ESC_END, ESC_ESC tokens = %+v, want %+v", got, want)
	}
	t.Logf("%d frames, %d garbage runs, %d UNKNOWN", gen.Records(), runs, unknowns)
}
