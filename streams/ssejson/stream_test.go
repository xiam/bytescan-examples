package main

import (
	"io"
	"math/rand"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan-examples/streams/sse/ssedriver"
	"github.com/xiam/bytescan/streamtest"
)

// goroutines returns the goroutine count once it has been stable for a
// moment, so that goroutines of earlier tests that are still ending do not
// count.
func goroutines() int {
	n := runtime.NumGoroutine()
	for i := 0; i < 50; i++ {
		time.Sleep(2 * time.Millisecond)
		m := runtime.NumGoroutine()
		if m == n {
			return n
		}
		n = m
	}
	return n
}

// TestLongRun lexes a generated stream of the harness budget (512 KiB, 64
// KiB under the race detector, 1 GiB with -streamtest.long) under this
// example's ceiling. The generated documents are valid JSON, split between
// tokens, so no token of either layer is UNKNOWN, and no JSON token is
// longer than 1000 runes. The pipe starts no goroutine: the count is the
// same before and after the run.
func TestLongRun(t *testing.T) {
	before := goroutines()
	var d *driver
	newL := func(r io.Reader) *driver {
		d = newScanner(r)
		return d
	}
	src := streamtest.NewGenerator(1, eventRecord)
	res := streamtest.Run(t, newL, src, streamtest.WithCeiling(ceiling(src.Budget())), streamtest.WithTokenFunc(jsonLengthCheck(t)))
	if after := goroutines(); after != before {
		t.Errorf("%d goroutines after the run, %d before", after, before)
	}
	if n := res.Counts[unknown]; n != 0 {
		t.Errorf("%d UNKNOWN tokens, want none", n)
	}
	for _, typ := range []bytescan.TokenType{
		ssedriver.Event, ssedriver.Data, ssedriver.ID, ssedriver.Retry, ssedriver.Field, ssedriver.Colon,
		ssedriver.Space, ssedriver.Value, ssedriver.Comment, ssedriver.EOL, ssedriver.EventEnd,
		jsonBeginArray, jsonEndArray, jsonBeginObject, jsonEndObject, jsonNameSeparator, jsonValueSeparator,
		jsonTrue, jsonFalse, jsonNull, jsonNumber, jsonWhitespace, jsonEOL, jsonQuote, jsonStringText, jsonEscape,
	} {
		if res.Counts[typ] == 0 {
			t.Errorf("no %s token in the stream", typ)
		}
	}
	p := d.pipe
	t.Logf("max queue %d tokens, %d runes; %d keep-alives", p.maxTokens, p.maxRunes, p.keepAlives)
	if p.maxTokens > worstTokens || p.maxRunes > worstRunes {
		t.Errorf("queue of %d tokens or %d runes, above the worst case of %d tokens or %d runes", p.maxTokens, p.maxRunes, worstTokens, worstRunes)
	}
}

// The two kinds of garbage that TestCorruption injects between records.
const (
	// joinAlphabet makes a run with no line end, which joins the first line
	// of the record after it. That line is never a data line, and the run
	// has no letter, so it never makes one: it changes one SSE line and no
	// JSON.
	joinAlphabet = "#%&*~ :\xff\xfe"
	// junkAlphabet makes the value of an extra data line. None of its runes
	// starts a JSON token (an invalid byte reaches the rules as U+FFFD), so
	// the JSON layer lexes each one as UNKNOWN.
	junkAlphabet = "#%&*~@`;\xff"
)

// TestCorruption injects garbage between records: runs that join the first line
// of the next record, and extra data lines of junk. It checks both layers.
// The SSE layer resumes at the next line: every SSE token after the line end
// of the line that a run joins, or after the junk line, equals the token of
// the clean stream, at the same offset less the garbage before it. The JSON
// layer lexes the junk as UNKNOWN tokens and then every document as the
// clean stream does: its tokens, without the UNKNOWN ones and the JSON_EOL
// line feeds (keep-alives fall where the reads fall), equal those of the
// clean stream. A clean driver over the same stream runs in step. Each layer
// is compared in its own order, so the test holds the few clean tokens of
// one layer that come before the next token of the other.
func TestCorruption(t *testing.T) {
	const seed = 2
	var kinds []bool // per injected run: true for a junk data line
	garbage := func(rng *rand.Rand, dst []byte) []byte {
		junk := rng.Intn(2) == 0
		kinds = append(kinds, junk)
		if !junk {
			return streamtest.Garbage(joinAlphabet, 1, 24)(rng, dst)
		}
		dst = append(dst, "data: "...)
		dst = streamtest.Garbage(junkAlphabet, 1, 24)(rng, dst)
		return append(dst, '\n')
	}
	c := streamtest.Corrupt(streamtest.NewGenerator(seed, eventRecord), 3, 0.2, garbage)
	clean := newScanner(streamtest.NewGenerator(seed, eventRecord))

	var (
		inj      streamtest.Injection
		haveInj  bool
		shift    int64 // garbage bytes before the current SSE token
		skipping bool  // inside the line that a garbage run joined or made
		seen     int   // garbage runs reached
		unknowns int64 // UNKNOWN tokens of the JSON layer
		compared int64 // tokens compared with the clean stream
		maxAhead int   // the most clean tokens held for the other layer

		// The clean tokens of one layer that the clean driver returned
		// while the test looked for a token of the other layer. The
		// layers interleave by read order, which differs between the two
		// streams, so each layer is compared in its own order.
		pendingSSE, pendingJSON []*bytescan.Token
	)
	isSSE := func(l *bytescan.Token) bool { return !isJSON(l.Type()) }
	isDoc := func(l *bytescan.Token) bool { return isJSON(l.Type()) && l.Type() != jsonEOL }
	isLineEnd := func(l *bytescan.Token) bool {
		return l.Type() == ssedriver.EOL || l.Type() == ssedriver.EventEnd
	}
	// next returns the next clean token of one layer: of the SSE layer
	// when sse is set, otherwise a JSON token other than JSON_EOL.
	next := func(sse bool) *bytescan.Token {
		mine, other := &pendingJSON, &pendingSSE
		if sse {
			mine, other = &pendingSSE, &pendingJSON
		}
		for len(*mine) == 0 {
			l, err := clean.Next()
			if err != nil {
				t.Fatalf("clean stream: %v", err)
			}
			if !isSSE(l) && !isDoc(l) {
				continue
			}
			if isSSE(l) == sse {
				*mine = append(*mine, l)
			} else {
				*other = append(*other, l)
				maxAhead = max(maxAhead, len(*other))
			}
		}
		l := (*mine)[0]
		*mine = (*mine)[1:]
		return l
	}

	// Two drivers are live, each with two cursors and a queue, so each is
	// held to this example's ceiling.
	res := streamtest.Run(t, newScanner, c, streamtest.WithCeiling(2*ceiling(c.Budget())), streamtest.WithTokenFunc(func(l *bytescan.Token) {
		if t.Failed() {
			return
		}
		if isJSON(l.Type()) {
			switch {
			case l.Type() == unknown:
				if !strings.ContainsRune(junkAlphabet+"�", []rune(l.Text())[0]) {
					t.Fatalf("UNKNOWN token %q, not from the junk alphabet", l.Text())
				}
				unknowns++
			case l.Type() != jsonEOL:
				cl := next(false)
				if cl.Type() != l.Type() || cl.Text() != l.Text() {
					t.Fatalf("after %d garbage runs: JSON layer got %s %q, want %s %q", seen, l.Type(), l.Text(), cl.Type(), cl.Text())
				}
				compared++
			}
			return
		}

		if !haveInj {
			inj, haveInj = c.NextInjection()
		}
		off := int64(l.ByteOffset())
		if haveInj && off >= inj.Offset {
			if skipping {
				t.Fatalf("garbage run %d at byte %d starts before the line of the run before it ends", seen+1, inj.Offset)
			}
			if !kinds[seen] {
				// The clean stream skips the first line of the record.
				for !isLineEnd(next(true)) {
				}
			}
			shift += int64(inj.Len)
			skipping = true
			seen++
			haveInj = false
		}
		if skipping {
			if isLineEnd(l) {
				skipping = false
			}
			return
		}
		cl := next(true)
		if cl.Type() != l.Type() || cl.Text() != l.Text() || int64(cl.ByteOffset())+shift != off {
			t.Fatalf("after %d garbage runs: SSE layer got %s %q at byte %d, want %s %q at byte %d",
				seen, l.Type(), l.Text(), off, cl.Type(), cl.Text(), int64(cl.ByteOffset())+shift)
		}
		compared++
	}))

	for {
		l, err := clean.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if isSSE(l) || isDoc(l) {
			pendingSSE = append(pendingSSE, l)
		}
	}
	if n := len(pendingSSE) + len(pendingJSON); n > 0 {
		t.Errorf("the clean stream has %d more tokens than the corrupted one", n)
	}
	if seen == 0 || seen != c.Injected() {
		t.Errorf("reached %d garbage runs, %d injected", seen, c.Injected())
	}
	if unknowns == 0 {
		t.Error("no UNKNOWN token in the JSON layer")
	}
	t.Logf("%d garbage runs: %d UNKNOWN tokens in the JSON layer, %d of %d tokens equal to the clean stream; at most %d clean tokens held for the other layer",
		seen, unknowns, compared, res.Tokens, maxAhead)
}

// TestUndelimited lexes one data line of at least 16 KiB that opens a JSON
// string and never closes it. The SSE layer emits VALUE pieces and the JSON
// layer JSON_STRING_TEXT pieces, each of at most 1000 runes, under this
// example's ceiling and the per-token guard. The line mixes one-, two-,
// three- and four-byte runes.
func TestUndelimited(t *testing.T) {
	const (
		budget = 20 << 10
		chunk  = "abc é € 😀 "
	)
	record := func(_ *rand.Rand, i int, dst []byte) []byte {
		if i == 0 {
			dst = append(dst, "data: \""...)
		}
		for len(dst) < 1<<10 {
			dst = append(dst, chunk...)
		}
		return dst
	}
	var values, texts int64
	var textBytes int64
	streamtest.Run(t, newScanner, streamtest.NewGenerator(0, record, streamtest.WithBudget(budget)),
		streamtest.WithCeiling(ceiling(budget)), streamtest.WithTokenFunc(func(l *bytescan.Token) {
			if l.Len() > maxRun {
				t.Fatalf("%s token of %d runes, more than %d", l.Type(), l.Len(), maxRun)
			}
			switch l.Type() {
			case ssedriver.Value:
				values++
			case jsonStringText:
				texts++
				textBytes += int64(len(l.Text()))
			case ssedriver.Data, ssedriver.Colon, ssedriver.Space, jsonQuote:
			default:
				t.Fatalf("unexpected %s token %.20q", l.Type(), l.Text())
			}
		}))
	if textBytes < 16<<10 {
		t.Fatalf("the string is %d bytes, want at least 16 KiB", textBytes)
	}
	t.Logf("a %d-byte string in %d VALUE pieces and %d JSON_STRING_TEXT pieces", textBytes, values, texts)
}

// TestLongNumber checks that a number longer than the bound comes out as
// several JSON_NUMBER tokens in a row, none longer than 1000 runes.
func TestLongNumber(t *testing.T) {
	cases := []struct {
		in   string
		want []int // rune lengths of the JSON_NUMBER tokens
	}{
		{"data:" + strings.Repeat("7", 1000) + "\n", []int{330, 330, 330, 10}},
		{"data:" + longNumber + "\n", []int{330, 330, 330, 341}},
		{"data:-1." + strings.Repeat("5", 330) + "e+" + strings.Repeat("9", 330) + "\n", []int{665}},
	}
	for _, c := range cases {
		d := newScanner(strings.NewReader(c.in))
		var got []int
		var types []string
		for {
			l, err := d.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			if !isJSON(l.Type()) || l.Type() == jsonEOL {
				continue
			}
			if l.Len() > maxRun {
				t.Fatalf("%s token of %d runes, more than %d", l.Type(), l.Len(), maxRun)
			}
			types = append(types, string(l.Type()))
			if l.Type() == jsonNumber {
				got = append(got, l.Len())
			}
		}
		if len(got) < len(c.want) || !equalInts(got[:len(c.want)], c.want) {
			t.Errorf("%.30q: JSON_NUMBER lengths %v, want %v first (types %v)", c.in, got, c.want, types)
		}
		if len(types) < len(c.want) {
			t.Errorf("%.30q: tokens %v", c.in, types)
			continue
		}
		for i := range c.want {
			if types[i] != string(jsonNumber) {
				t.Errorf("%.30q: token %d is %s, want JSON_NUMBER (types %v)", c.in, i, types[i], types)
			}
		}
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
