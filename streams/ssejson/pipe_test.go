package main

import (
	"bytes"
	"io"
	"math/rand"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan-examples/streams/sse/ssedriver"
	"github.com/xiam/bytescan/streamtest"
)

// The documented worst case of the queue (README.md, "The queue"): the most
// SSE tokens, and the most runes of their text, that the pipe holds at once.
const (
	worstTokens = 67
	worstRunes  = 4000
)

// queueHeap bounds the live heap of a queue at the worst case, in bytes. The
// engine stores the text of a token as a []rune, 4 bytes per rune, so 4,000
// runes take 16,000 bytes, and each token adds its struct and span.
// TestQueueHeap measures it.
const queueHeap = 32 << 10

// queueBudget is the budget of the pipe and queue tests: the harness default
// (512 KiB, 64 KiB under the race detector), also with -streamtest.long.
func queueBudget() int64 {
	return min(streamtest.DefaultBudget(), 512<<10)
}

// ceiling returns this example's heap ceiling for a stream of budget bytes:
// the harness default plus twice the worst-case queue heap. The queue is held
// on top of everything the default ceiling already allows, and the factor of
// two is margin for heap sampling.
func ceiling(budget int64) int64 {
	return streamtest.DefaultCeiling(budget) + 2*queueHeap
}

// TestPipeBytes reads the pipe directly, with buffers of 4,096, 1 and 3
// bytes, and compares the bytes it returns with the data buffer of the
// generated stream: each data value and a line feed after each data line,
// plus the keep-alives, at the offsets the pipe reports. The test drains the
// queue and resets the counters, as the combined driver does before it reads
// again, after every 16th Read and after each keep-alive. It samples no heap,
// so it runs in parallel, after the tests that do.
func TestPipeBytes(t *testing.T) {
	t.Parallel()
	raw, err := io.ReadAll(streamtest.NewGenerator(1, eventRecord, streamtest.WithBudget(queueBudget())))
	if err != nil {
		t.Fatal(err)
	}
	stream := string(raw)
	want := dataBuffer(stream)

	for _, size := range []int{4096, 1, 3} {
		p := newPipe(ssedriver.New(strings.NewReader(stream), bytescan.WithMaxTokenBytes(maxTokenBytes)))
		var keeps []int64
		p.onKeepAlive = func(off int64) { keeps = append(keeps, off) }

		var got bytes.Buffer
		buf := make([]byte, size)
		drained := 0
		for i := 0; ; i++ {
			n, err := p.Read(buf)
			got.Write(buf[:n])
			// Drain the queue after every 16th Read, and after a
			// keep-alive, so that the counters also fill across reads.
			if i%16 == 0 || len(keeps) > drained {
				for p.pop() != nil {
				}
				p.reset()
				drained = len(keeps)
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("buffer of %d bytes: %v", size, err)
			}
			if n == 0 {
				t.Fatalf("buffer of %d bytes: Read returned 0 bytes and no error", size)
			}
		}

		// Cut the keep-alives out, from the last one back.
		out := got.Bytes()
		for i := len(keeps) - 1; i >= 0; i-- {
			off := keeps[i]
			if off+int64(len(keepAlive)) > int64(len(out)) || !bytes.Equal(out[off:off+int64(len(keepAlive))], keepAlive[:]) {
				t.Fatalf("buffer of %d bytes: no keep-alive at byte %d", size, off)
			}
			out = append(out[:off], out[off+int64(len(keepAlive)):]...)
		}
		if string(out) != want {
			i := 0
			for i < len(out) && i < len(want) && out[i] == want[i] {
				i++
			}
			t.Fatalf("buffer of %d bytes: the pipe's bytes differ from the data buffer at byte %d of %d (got %q, want %q)",
				size, i, len(want), clip(string(out[i:]), 40), clip(want[i:], 40))
		}
		t.Logf("buffer of %d bytes: %d data bytes, %d keep-alives", size, len(want), len(keeps))
	}
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// x returns n runes of filler text.
func x(n int) string {
	return strings.Repeat("x", n)
}

// comments returns one comment line per entry of runes, each a COMMENT token
// of that many runes (at most 1000) and its line end.
func comments(runes ...int) string {
	var b strings.Builder
	for _, n := range runes {
		b.WriteString(":" + x(n-1) + "\n")
	}
	return b.String()
}

// shorts returns n comment lines of 20 four-byte runes: 2n counted tokens
// and 21n runes. A run of them reaches the token threshold well below the
// rune threshold. The lines are 82 bytes long, so the cursor's per-line
// table stays small (a run of 2-byte lines would hold a table of about
// 512 KiB before the cursor's first trim, whatever the queue holds).
func shorts(n int) string {
	return strings.Repeat(":"+strings.Repeat("\U0001F600", 19)+"\n", n)
}

// repeatRecord returns a record function that writes s for every record.
func repeatRecord(s string) streamtest.RecordFunc {
	return func(_ *rand.Rand, _ int, dst []byte) []byte {
		return append(dst, s...)
	}
}

// longNumber is one JSON number of 3,000 runes, whose three digit runs are
// longer than maxDigits: 1000 digits, a point, 1000 digits, "e", 998 digits.
var longNumber = "1" + strings.Repeat("2", 999) + "." + strings.Repeat("3", 1000) + "e" + strings.Repeat("4", 998)

// queueStreams are the streams of the queue test. Each repeats one record to
// the budget; a record holds every variant of its stream in turn. The
// comments give the run lengths in counted tokens or runes.
var queueStreams = []struct {
	name   string
	record string
}{
	// (a) Only comments and non-data fields, with the longest pieces:
	// 1000-rune COMMENT pieces and 1000-rune non-data VALUE pieces.
	{"a", comments(1000) + ":" + x(2999) + "\n" + "f:" + x(3000) + "\n" + "id: " + x(1000) + "\n\n"},

	// (b) Runs just below each threshold, 60 tokens and 998+1+997+1 = 1,997
	// runes, each ending in an empty or a 1-byte data line.
	{"b", shorts(30) + "data\n" + shorts(30) + "data:7\n" + comments(998, 997) + "data:\n" + comments(998, 997) + "data:7\n"},

	// (c) A data line of 1,996 runes (VALUE pieces of 1000 and 996 runes)
	// that carries one long JSON string, whose last piece is read by the
	// same inner Next as the run after it: a run just below the rune
	// threshold, and a run just below the token threshold with an empty data
	// line inside.
	{"c", "data:\"" + x(1994) + "\"\n" + comments(998, 998, 1000) + "data:\"" + x(1994) + "\"\n" + shorts(15) + "data\n" + shorts(16)},

	// (c, second shape) A full VALUE piece read at the start of an inner
	// Next, a counted run below the rune threshold, and the first VALUE
	// piece of the next data line, read by the look-ahead fill of the same
	// Next. This record reaches the worst case of 4,000 runes.
	{"c-two-pieces", twoPieces},

	// (d) A run just below a threshold, a data line whose value is one short
	// multi-rune JSON token, then a second run.
	{"d", comments(998, 997) + "data:true\n" + comments(998, 997, 1000) + "\n" +
		comments(998, 997) + "data:12345\n" + comments(998, 997, 1000) + "\n" +
		shorts(31) + "data:true\n" + shorts(32) + "\n" +
		shorts(31) + "data:12345\n" + shorts(32) + "\n"},

	// (e) A data line that holds one number of 3,000 runes, whose three digit
	// runs are longer than the bound, and that ends at a VALUE piece boundary,
	// then a run just below the rune threshold; without and with a run
	// before the line.
	{"e", "data:" + longNumber + "\n" + comments(998, 998, 1000) + comments(994) + "data:" + longNumber + "\n" + comments(998, 998, 1000)},
}

// twoPieces is the record of the second worst-case shape: a data line of
// 1,001 tabs (VALUE pieces of 1000 and 1 runes, one JSON_WHITESPACE of 1000
// runes), comments of 993 and 993 runes with their line ends (1,988 runes),
// and a data line that opens a string of 1,000 runes. A search over the
// prefix before the first line (0 to 3 runes), the length of the first line
// (999 to 1,005 runes), the run (1,980 to 1,999 runes), and tabs, strings,
// digits or letters in each data line found no queue above 4,000 runes.
var twoPieces = "data:" + strings.Repeat("\t", 1001) + "\n" + comments(993, 993) + "data:\"" + x(1000) + "\n"

// jsonLengthCheck returns a token function for streamtest.Run that fails the test
// when a JSON token is longer than maxRun runes.
func jsonLengthCheck(t *testing.T) func(*bytescan.Token) {
	return func(l *bytescan.Token) {
		if isJSON(l.Type()) && l.Len() > maxRun {
			t.Fatalf("%s token of %d runes, more than %d", l.Type(), l.Len(), maxRun)
		}
	}
}

func isJSON(t bytescan.TokenType) bool {
	return t == unknown || strings.HasPrefix(string(t), "JSON_")
}

// TestQueue runs the queue-test streams at the queue budget, under this
// example's ceiling, and checks that the queue never exceeds the documented
// worst case and that no JSON token is longer than 1000 runes.
func TestQueue(t *testing.T) {
	var maxLex, maxRunes int
	for _, s := range queueStreams {
		t.Run(s.name, func(t *testing.T) {
			var d *driver
			newL := func(r io.Reader) *driver {
				d = newScanner(r)
				return d
			}
			gen := streamtest.NewGenerator(0, repeatRecord(s.record), streamtest.WithBudget(queueBudget()))
			streamtest.Run(t, newL, gen, streamtest.WithCeiling(ceiling(queueBudget())), streamtest.WithTokenFunc(jsonLengthCheck(t)))
			p := d.pipe
			t.Logf("max queue %d tokens, %d runes; %d keep-alives", p.maxTokens, p.maxRunes, p.keepAlives)
			if p.maxTokens > worstTokens || p.maxRunes > worstRunes {
				t.Errorf("queue of %d tokens or %d runes, above the worst case of %d tokens or %d runes", p.maxTokens, p.maxRunes, worstTokens, worstRunes)
			}
			maxLex, maxRunes = max(maxLex, p.maxTokens), max(maxRunes, p.maxRunes)
		})
	}
	t.Logf("all streams: max queue %d tokens, %d runes", maxLex, maxRunes)
}

// TestTwoPieces checks that the record of the second shape fills the queue
// to the documented worst case of 4,000 runes, and no further.
func TestTwoPieces(t *testing.T) {
	lex, runes := lexQueue(t, strings.Repeat(twoPieces, 2))
	t.Logf("max queue %d tokens, %d runes", lex, runes)
	if runes != worstRunes {
		t.Errorf("queue of %d runes, want the worst case of %d", runes, worstRunes)
	}
}

// lexQueue lexes s to the end and returns the most tokens and runes the
// queue held.
func lexQueue(t testing.TB, s string) (int, int) {
	d := newScanner(strings.NewReader(s))
	for {
		l, err := d.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if isJSON(l.Type()) && l.Len() > maxRun {
			t.Fatalf("%s token of %d runes, more than %d", l.Type(), l.Len(), maxRun)
		}
	}
	return d.pipe.maxTokens, d.pipe.maxRunes
}

// searchStream returns one random stream for the random search: a mix of
// comments, non-data fields, blank lines, and data lines of 0 to 4,000 runes
// that hold numbers with digit runs of 1 to 1000 digits, strings, literals
// and filler.
func searchStream(rng *rand.Rand) string {
	var b strings.Builder
	for n := 3 + rng.Intn(8); n > 0; n-- {
		switch rng.Intn(6) {
		case 0:
			b.WriteString(comments(1 + rng.Intn(1000)))
		case 1:
			b.WriteString(strings.Repeat(":\n", 1+rng.Intn(40)))
		case 2:
			b.WriteString("f:" + x(rng.Intn(2000)) + "\n")
		case 3:
			b.WriteString("\n")
		default:
			var v strings.Builder
			limit := rng.Intn(4001)
			for v.Len() < limit {
				switch rng.Intn(5) {
				case 0:
					v.WriteString(jsonNumberText(rng, 1000))
				case 1:
					v.WriteString("\"" + x(rng.Intn(1500)) + "\"")
				case 2:
					v.WriteString([]string{"true", "false", "null", "[", "]", "{", "}", ",", ":"}[rng.Intn(9)])
				case 3:
					v.WriteString(strings.Repeat(" ", 1+rng.Intn(10)))
				default:
					v.WriteString(digits(rng, 1+rng.Intn(1000), true))
				}
			}
			s := v.String()
			if len(s) > limit {
				s = s[:limit]
			}
			b.WriteString("data:" + s + "\n")
		}
	}
	return b.String()
}

// searchStreams is the number of streams of the random search: 400, or 40
// under the race detector, which lexes about ten times slower.
func searchStreams() int {
	if streamtest.DefaultBudget() < 512<<10 {
		return 40
	}
	return 400
}

// TestQueueSearch lexes random streams from fixed seeds and checks that the
// queue never exceeds the documented worst case and that no JSON token is
// longer than 1000 runes. It samples no heap, so it runs in parallel, in four
// parts of a quarter of the streams each, after the tests that do.
func TestQueueSearch(t *testing.T) {
	t.Parallel()
	const parts = 4
	for part := 0; part < parts; part++ {
		part := part
		t.Run(strconv.Itoa(part), func(t *testing.T) {
			t.Parallel()
			rng := rand.New(rand.NewSource(int64(62 + part)))
			var maxLex, maxRunes, total int
			for i := 0; i < searchStreams()/parts; i++ {
				s := searchStream(rng)
				total += len(s)
				lex, runes := lexQueue(t, s)
				if lex > worstTokens || runes > worstRunes {
					t.Fatalf("stream %d: queue of %d tokens or %d runes, above the worst case of %d tokens or %d runes", i, lex, runes, worstTokens, worstRunes)
				}
				maxLex, maxRunes = max(maxLex, lex), max(maxRunes, runes)
			}
			t.Logf("%d streams, %d bytes: max queue %d tokens, %d runes", searchStreams()/parts, total, maxLex, maxRunes)
		})
	}
}

// TestQueueHeap measures the live heap of a queue at the worst case, 67 SSE
// tokens of 4,000 runes, and checks that it is within queueHeap.
func TestQueueHeap(t *testing.T) {
	// 3 COMMENT(1000) and EOL, 30 COMMENT(32) and EOL, and COMMENT(7):
	// 67 tokens, 4,000 runes.
	in := strings.Repeat(comments(1000), 3) + strings.Repeat(comments(32), 30) + ":" + x(6)
	sse := ssedriver.New(strings.NewReader(in), bytescan.WithMaxTokenBytes(maxTokenBytes))
	q := make([]*bytescan.Token, 0, worstTokens)

	heap := func() uint64 {
		var ms runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&ms)
		return ms.HeapAlloc
	}
	runes := 0
	for {
		l, err := sse.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		q = append(q, l)
		runes += l.Len()
	}
	// The heap with the queue, then without it; the driver and its cursor
	// stay live in both samples.
	n := len(q)
	with := heap()
	runtime.KeepAlive(q)
	q = nil
	without := heap()
	runtime.KeepAlive(sse)
	if n != worstTokens || runes != worstRunes {
		t.Fatalf("queue of %d tokens, %d runes; want %d and %d", n, runes, worstTokens, worstRunes)
	}
	held := int64(with) - int64(without)
	t.Logf("a queue of %d tokens and %d runes holds %d bytes of live heap (bound %d)", n, runes, held, queueHeap)
	if held > queueHeap {
		t.Errorf("the worst-case queue holds %d bytes, above queueHeap (%d)", held, queueHeap)
	}
}
