package main

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/streamtest"
)

// TestLongRun lexes an endless sentence stream of the default budget, or
// of 1 GiB with -streamtest.long, under the harness ceiling, and checks the
// token counts against the sentences:
//
//	go test -timeout 0 -run Long ./streams/nmea -streamtest.long
func TestLongRun(t *testing.T) {
	gen := streamtest.NewGenerator(1, sentenceRecord)
	res := streamtest.Run(t, newScanner, gen)
	records := int64(gen.Records())
	for _, typ := range []bytescan.TokenType{start, address, checksum, eol} {
		{
			got := res.Counts[typ]
			assert.Equalf(t, records, got, "lexed %d %s, want one per sentence (%d)", got, typ, records)
		}
	}
	{
		got := res.Counts[bytescan.TokenTypeUnknown]
		assert.Zerof(t, got, "lexed %d UNKNOWN in a clean stream", got)
	}
	if res.Bytes != gen.Bytes() {
		assert.Failf(t, "", "Run read %d bytes, the generator wrote %d", res.Bytes, gen.Bytes())
	}
}

// injectionChecker follows the garbage runs of a Corrupter while the tokens
// arrive. For each token it reports whether the token starts inside a run,
// and it fails the test when a token crosses the end of a run, or when the
// first token after a run is not the START of the next sentence.
type injectionChecker struct {
	t    *testing.T
	c    *streamtest.Corrupter
	cur  streamtest.Injection
	have bool
}

func (k *injectionChecker) check(l *bytescan.Token) bool {
	off := int64(l.ByteOffset())
	end := k.cur.Offset + int64(k.cur.Len)
	if !k.have || off >= end {
		if k.have && off == end && l.Type() != start {
			require.FailNowf(k.t, "", "after the garbage at %d: got %s %q, want START", k.cur.Offset, l.Type(), l.Text())
		}
		k.cur, k.have = k.c.NextInjection()
		end = k.cur.Offset + int64(k.cur.Len)
	}
	in := k.have && off >= k.cur.Offset && off < end
	if in {
		if off+int64(len(l.Text())) > end {
			require.FailNowf(k.t, "", "at %d: %s %q crosses the end of the garbage at %d", off, l.Type(), l.Text(), end)
		}
	}
	return in
}

// TestCorruption injects noise between sentences: runs of printable field
// bytes and commas, and of non-printable bytes. The rules are context-free, so
// printable noise lexes as FIELD, ADDRESS and COMMA tokens, and each
// non-printable byte as one UNKNOWN token. No token crosses the end of a
// run, and lexing is correct again from the START of the next sentence.
func TestCorruption(t *testing.T) {
	gen := streamtest.NewGenerator(3, sentenceRecord)
	c := streamtest.Corrupt(gen, 4, 0.05, noise)
	k := &injectionChecker{t: t, c: c}
	seen := map[bytescan.TokenType]int64{}
	check := func(l *bytescan.Token) {
		if !k.check(l) {
			if l.Type() == bytescan.TokenTypeUnknown {
				require.FailNowf(t, "", "at %d: UNKNOWN %q outside the noise", l.ByteOffset(), l.Text())
			}
			return
		}
		switch l.Type() {
		case bytescan.TokenTypeUnknown, field, address, comma:
			seen[l.Type()]++
		default:
			require.FailNowf(t, "", "at %d: got %s %q inside the noise", l.ByteOffset(), l.Type(), l.Text())
		}
	}
	res := streamtest.Run(t, newScanner, c, streamtest.WithTokenFunc(check))
	for _, typ := range []bytescan.TokenType{bytescan.TokenTypeUnknown, field, address, comma} {
		assert.NotZerof(t, seen[typ], "no %s token inside the noise: %v", typ, seen)
	}
	{
		got, want := res.Counts[bytescan.TokenTypeUnknown], seen[bytescan.TokenTypeUnknown]
		assert.Equalf(t, want, got, "lexed %d UNKNOWN, %d of them inside the noise", got, want)
	}
	{
		got, want := res.Counts[start], int64(gen.Records())
		assert.Equalf(t, want, got, "lexed %d START, want one per sentence (%d)", got, want)
	}
	t.Logf("%d noise runs lexed as %v", c.Injected(), seen)
}

// noise is the streamtest.GarbageFunc of TestCorruption. It appends one to
// four pieces, each followed by a comma or not: five upper-case letters (which
// lex as ADDRESS), a word of other field bytes (FIELD), or a run of
// non-printable bytes (UNKNOWN). It never writes a start character, an
// asterisk or a line break.
func noise(rng *rand.Rand, dst []byte) []byte {
	const (
		upper   = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
		field   = "abcxyz0189 .-#/"
		control = "\x00\x01\x02\x07\x0b\x1b\x7f"
	)
	for n := 1 + rng.Intn(4); n > 0; n-- {
		switch rng.Intn(3) {
		case 0:
			for j := 0; j < 5; j++ {
				dst = append(dst, upper[rng.Intn(len(upper))])
			}
		case 1:
			for m := 1 + rng.Intn(12); m > 0; m-- {
				dst = append(dst, field[rng.Intn(len(field))])
			}
		default:
			for m := 1 + rng.Intn(4); m > 0; m-- {
				dst = append(dst, control[rng.Intn(len(control))])
			}
		}
		if rng.Intn(3) > 0 {
			dst = append(dst, ',')
		}
	}
	return dst
}

// TestUndelimitedRun lexes one run of 32 KiB of field bytes with no delimiter,
// as a stream that has lost its delimiters. It lexes as FIELD pieces of at most
// 1000 runes, under the harness ceiling and the per-token guard.
func TestUndelimitedRun(t *testing.T) {
	const budget = 32 << 10
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789. "
	run := func(rng *rand.Rand, _ int, dst []byte) []byte {
		for j := 0; j < 1024; j++ {
			dst = append(dst, letters[rng.Intn(len(letters))])
		}
		return dst
	}
	gen := streamtest.NewGenerator(7, run, streamtest.WithBudget(budget))
	var full int64
	check := func(l *bytescan.Token) {
		if l.Type() != field || l.Len() > maxRun {
			require.FailNowf(t, "", "at %d: got %s of %d runes, want FIELD of at most %d", l.ByteOffset(), l.Type(), l.Len(), maxRun)
		}
		if l.Len() == maxRun {
			full++
		}
	}
	res := streamtest.Run(t, newScanner, gen, streamtest.WithTokenFunc(check))
	require.GreaterOrEqualf(t, res.Bytes, int64(16<<10), "lexed %d bytes, want at least 16 KiB", res.Bytes)
	want := res.Bytes / maxRun
	pieces := want
	if res.Bytes%maxRun != 0 {
		pieces++
	}
	if full != want || res.Tokens != pieces {
		require.FailNowf(t, "", "lexed %d tokens, %d of %d runes, from %d bytes; want %d tokens, %d of them full",
			res.Tokens, full, maxRun, res.Bytes, pieces, want)
	}
}
