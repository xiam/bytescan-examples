package main

import (
	"io"
	"math/rand"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/streamtest"
)

// lineEnds are the three line endings of the stream grammar.
var lineEnds = []string{"\n", "\r\n", "\r"}

// words fill the generated values; some hold a colon or a non-ASCII rune.
var words = []string{
	"alpha", "beta", "gamma", "{\"k\":", "1}", "a: b", "€", "naïve", "😀", "x",
}

// eventRecord is the record function of an endless event stream: one event,
// or a heartbeat comment, ended by a blank line. Every line of one record ends
// with the same line ending, chosen per record, so a bare "\r" is never
// followed by a "\n" that would join it into "\r\n". A record never starts
// with a line end, so the last line end of a record and the first line of the
// next never join either.
func eventRecord(rng *rand.Rand, i int, dst []byte) []byte {
	nl := lineEnds[rng.Intn(len(lineEnds))]
	line := func(parts ...string) {
		for _, p := range parts {
			dst = append(dst, p...)
		}
		dst = append(dst, nl...)
	}
	text := func() string {
		var b strings.Builder
		for n := 1 + rng.Intn(6); n > 0; n-- {
			if b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(words[rng.Intn(len(words))])
		}
		return b.String()
	}

	if rng.Intn(5) == 0 {
		// A heartbeat: a comment line, and the blank line that ends the
		// (empty) event.
		line(": heartbeat ", strconv.Itoa(i))
		line()
		return dst
	}
	if rng.Intn(2) == 0 {
		line("event: ", words[rng.Intn(3)])
	}
	if rng.Intn(3) == 0 {
		line("id: ", strconv.Itoa(i))
	}
	if rng.Intn(20) == 0 {
		line("retry: ", strconv.Itoa(1000*(1+rng.Intn(10))))
	}
	for n := 1 + rng.Intn(4); n > 0; n-- {
		switch rng.Intn(8) {
		case 0:
			line("data") // no colon: an empty data line
		case 1:
			line("data:", text()) // no space after the colon
		case 2:
			line(": inside the event")
		case 3:
			line("x-trace: ", strconv.FormatInt(rng.Int63(), 36))
		default:
			line("data: ", text())
		}
	}
	line()
	return dst
}

// lineTableCeiling is the heap ceiling of one cursor over the generated
// stream when it is above the harness default. A cursor.Reader trims its
// per-line table only once its first 64 KiB buffer is full, and keeps two
// int slices with one entry per line, grown by doubling. The generated lines
// are short (about 14 bytes on average), so one buffer holds about 4,700
// lines, and the two slices grow to 8,192 entries each, 128 KiB, before the
// first trim. That is above the 48 KiB floor of the harness, which was fixed
// on its own longer lines. The 64 KiB stream of a race-detector run never
// passes the first trim, so this ceiling decides there (30 runs grew by at
// most 61 KiB with one cursor). A default run with one cursor (256 KiB) and
// every -streamtest.long run (16 MiB) keep the harness default.
const lineTableCeiling = 160 << 10

// ceiling returns the heap ceiling of a run with the given number of cursors
// over a generated stream of the default budget.
func ceiling(cursors int) streamtest.Option {
	return streamtest.WithCeiling(max(streamtest.DefaultCeiling(streamtest.DefaultBudget()), int64(cursors)*lineTableCeiling))
}

// TestLongRun lexes a generated event stream of the harness budget (512 KiB,
// 64 KiB under the race detector, 1 GiB with -streamtest.long) under the
// heap ceiling. No rune of the stream fits no rule, so there is no UNKNOWN
// token.
func TestLongRun(t *testing.T) {
	res := streamtest.Run(t, newScanner, streamtest.NewGenerator(1, eventRecord), ceiling(1))
	if n := res.Counts[bytescan.TokenType("UNKNOWN")]; n != 0 {
		t.Errorf("%d UNKNOWN tokens, want none", n)
	}
	for _, typ := range []bytescan.TokenType{event, data, id, retry, field, colon, space, value, comment, eol, eventEnd} {
		if res.Counts[typ] == 0 {
			t.Errorf("no %s token in the stream", typ)
		}
	}
}

// garbageAlphabet is the alphabet of the injected runs: printable runes, a
// space, a colon, and two bytes that are not UTF-8. No line end, so each run
// joins the first line of the record after it. The stream grammar takes any
// rune in a line (an invalid byte decodes to U+FFFD), so the run lexes as part
// of a field or a comment, not as UNKNOWN.
const garbageAlphabet = "#%&*~ :\xff\xfe"

// TestCorruption injects garbage before records and checks that only the line the
// garbage joins lexes differently: from the line end of that line on, every
// token is the token of the clean stream, at the same offset less the
// garbage before it. A clean driver over the same stream without garbage runs
// in step, so the test holds no more than the two drivers.
func TestCorruption(t *testing.T) {
	const seed = 2
	c := streamtest.Corrupt(streamtest.NewGenerator(seed, eventRecord), 3, 0.2, streamtest.Garbage(garbageAlphabet, 1, 24))
	clean := newScanner(streamtest.NewGenerator(seed, eventRecord))

	var (
		inj       streamtest.Injection
		haveInj   bool
		shift     int64 // garbage bytes before the current token
		skipping  bool  // inside the line that a garbage run joined
		seen      int   // garbage runs reached
		corrupted int64 // tokens of the joined lines
		compared  int64 // tokens compared with the clean stream
	)
	cleanNext := func() *bytescan.Token {
		l, err := clean.Next()
		if err != nil {
			t.Fatalf("clean stream: %v", err)
		}
		return l
	}
	isLineEnd := func(l *bytescan.Token) bool {
		return l.Type() == eol || l.Type() == eventEnd
	}

	// Two cursors are live: the corrupted stream's and the clean driver's.
	res := streamtest.Run(t, newScanner, c, ceiling(2), streamtest.WithTokenFunc(func(l *bytescan.Token) {
		if t.Failed() {
			return
		}
		if l.Type() == "UNKNOWN" {
			t.Errorf("UNKNOWN token %q at byte %d", l.Text(), l.ByteOffset())
			return
		}
		if !haveInj {
			inj, haveInj = c.NextInjection()
		}
		off := int64(l.ByteOffset())
		if haveInj && off >= inj.Offset {
			if skipping {
				t.Fatalf("garbage run %d at byte %d starts before the joined line of the run before it ends", seen+1, inj.Offset)
			}
			// The clean stream skips the first line of the record.
			target := inj.Offset - shift
			for {
				cl := cleanNext()
				if int64(cl.ByteOffset()) < target {
					t.Fatalf("clean token %s %q at byte %d is before the record at byte %d", cl.Type(), cl.Text(), cl.ByteOffset(), target)
				}
				if isLineEnd(cl) {
					break
				}
			}
			shift += int64(inj.Len)
			skipping = true
			seen++
			haveInj = false
		}
		if skipping {
			corrupted++
			if isLineEnd(l) {
				skipping = false
			}
			return
		}
		cl := cleanNext()
		if cl.Type() != l.Type() || cl.Text() != l.Text() || int64(cl.ByteOffset())+shift != off {
			t.Fatalf("after %d garbage runs: got %s %q at byte %d, want %s %q at byte %d",
				seen, l.Type(), l.Text(), off, cl.Type(), cl.Text(), int64(cl.ByteOffset())+shift)
		}
		compared++
	}))

	if _, err := clean.Next(); err != io.EOF {
		t.Errorf("the clean stream has more tokens than the corrupted one (err %v)", err)
	}
	if seen == 0 || seen != c.Injected() {
		t.Errorf("reached %d garbage runs, %d injected", seen, c.Injected())
	}
	t.Logf("%d garbage runs: %d tokens in the joined lines, %d of %d tokens equal to the clean stream", seen, corrupted, compared, res.Tokens)
}

// TestUndelimited lexes one run with no line end and no colon, of at least
// 16 KiB, as a field name, a value and a comment. Each run lexes as pieces of
// at most maxRun runes of one type, under the heap ceiling and the per-token
// guard. The run mixes one-, two-, three- and four-byte runes.
func TestUndelimited(t *testing.T) {
	const (
		budget = 20 << 10
		chunk  = "abc é € 😀 "
	)
	cases := []struct {
		name   string
		prefix string
		typ    bytescan.TokenType
		before int // tokens of the prefix
	}{
		{"name", "", field, 0},
		{"value", "data: ", value, 3},
		{"comment", ":", comment, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			record := func(rng *rand.Rand, i int, dst []byte) []byte {
				if i == 0 {
					dst = append(dst, c.prefix...)
				}
				for len(dst) < 1<<10 {
					dst = append(dst, chunk...)
				}
				return dst
			}
			var (
				n      int
				pieces int
				runLen int64
			)
			streamtest.Run(t, newScanner, streamtest.NewGenerator(0, record, streamtest.WithBudget(budget)), streamtest.WithTokenFunc(func(l *bytescan.Token) {
				n++
				if n <= c.before {
					return
				}
				if l.Type() != c.typ {
					t.Fatalf("token %d is %s %.20q, want %s", n, l.Type(), l.Text(), c.typ)
				}
				if r := utf8.RuneCountInString(l.Text()); r > maxRun {
					t.Fatalf("token %d has %d runes, more than %d", n, r, maxRun)
				}
				pieces++
				runLen += int64(len(l.Text()))
			}))
			if runLen < 16<<10 {
				t.Fatalf("the run is %d bytes, want at least 16 KiB", runLen)
			}
			t.Logf("a %d-byte %s run in %d pieces", runLen, c.typ, pieces)
		})
	}
}
