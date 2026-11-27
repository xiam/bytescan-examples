package main

import (
	"bytes"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/bytesource"
	"github.com/xiam/bytescan/streamtest"
)

// TestGuard checks the per-token guard of the scanners: the longest read of
// a rule, a size with the lookahead of sizePairRule (two sizes of
// maxSizeDigits digits, two runs of maxRun blanks and the CRLF, 2022 bytes),
// rounded up to a multiple of streamtest.ByteGuard.
func TestGuard(t *testing.T) {
	sizePair := 2*maxSizeDigits + 2*maxRun + len("\r\n")
	require.Equalf(t, 2022, sizePair, "the lookahead of sizePairRule reads %d bytes, want 2022", sizePair)
	want := (sizePair + streamtest.ByteGuard - 1) / streamtest.ByteGuard * streamtest.ByteGuard
	require.Equalf(t, want, tokenGuard, "tokenGuard = %d, want %d: %d bytes rounded up to a multiple of streamtest.ByteGuard (%d)", tokenGuard, want, sizePair, streamtest.ByteGuard)
}

// serverStream generates an endless server-to-client stream: an INFO, then
// MSG and HMSG deliveries with binary payloads, keep-alives, +OK and -ERR.
// The payloads hold CR LF pairs, MSG lines and bytes of 0x80 or above. It
// counts the operations and the payload bytes it wrote, so a test can check
// the tokens against them.
type serverStream struct {
	ops          int64
	payloadBytes int64
	headerBytes  int64
}

// payloadParts are pieces that a payload is built from: text that a rule
// would take for a control line, and bytes that are not UTF-8.
var payloadParts = []string{"\r\n", "MSG a 1 3\r\n", "PING\r\n", "\xff\xfe", "\x00", "é", "data ", "\r\n\r\n"}

// record writes record i. Record 0 is the INFO and then one line of every
// kind the stream has, so every scanner of the driver has run before the
// harness takes its baseline heap sample: a scanner holds about 36 KB more once
// it has run, whatever the input. Every other record is one line of a random
// kind, with its payload.
func (s *serverStream) record(rng *rand.Rand, i int, dst []byte) []byte {
	if i > 0 {
		return s.line(rng, rng.Intn(20), rng.Intn(3) == 0, dst)
	}
	s.ops++
	dst = append(dst, `INFO {"server_id":"NSTREAM","version":"2.10.4","max_payload":1048576}`+"\r\n"...)
	for _, k := range []int{0, 12, 16, 17, 18, 19} {
		dst = s.line(rng, k, false, dst)
		if k < 16 {
			dst = s.line(rng, k, true, dst)
		}
	}
	return dst
}

// line writes one line of kind k (0 to 11 MSG, 12 to 15 HMSG, 16 PING, 17
// PONG, 18 +OK, 19 -ERR), with a reply subject when reply is true.
func (s *serverStream) line(rng *rand.Rand, k int, reply bool, dst []byte) []byte {
	s.ops++
	body := func(maxLen int) []byte {
		n := rng.Intn(maxLen + 1)
		b := make([]byte, 0, n+16)
		for len(b) < n {
			if rng.Intn(4) == 0 {
				b = append(b, payloadParts[rng.Intn(len(payloadParts))]...)
			} else {
				b = append(b, byte(rng.Intn(256)))
			}
		}
		return b
	}
	subj := "orders." + strconv.Itoa(rng.Intn(100)) + ".created"
	sidText := strconv.Itoa(1 + rng.Intn(9))
	replyText := ""
	if reply {
		replyText = " _INBOX.x" + strconv.Itoa(rng.Intn(1000))
	}
	switch {
	case k < 12:
		b := body(1024)
		s.payloadBytes += int64(len(b))
		dst = fmt.Appendf(dst, "MSG %s %s%s %d\r\n", subj, sidText, replyText, len(b))
		dst = append(dst, b...)
		return append(dst, "\r\n"...)
	case k < 16:
		h := []byte("NATS/1.0\r\nTrace-Id: " + strconv.Itoa(rng.Int()) + "\r\n\r\n")
		b := body(512)
		s.headerBytes += int64(len(h))
		s.payloadBytes += int64(len(b))
		dst = fmt.Appendf(dst, "HMSG %s %s%s %d %d\r\n", subj, sidText, replyText, len(h), len(h)+len(b))
		dst = append(dst, h...)
		dst = append(dst, b...)
		return append(dst, "\r\n"...)
	case k == 16:
		return append(dst, "PING\r\n"...)
	case k == 17:
		return append(dst, "PONG\r\n"...)
	case k == 18:
		return append(dst, "+OK\r\n"...)
	}
	return append(dst, "-ERR 'Slow Consumer Detected'\r\n"...)
}

// payloadCounter adds up the bytes of the PAYLOAD and HEADERS tokens and
// checks that each piece is at most 64 KiB.
type payloadCounter struct {
	t               *testing.T
	payload, header int64
}

func (c *payloadCounter) add(l *bytescan.Token) {
	switch l.Type() {
	case payload:
		c.payload += int64(l.Len())
	case headers:
		c.header += int64(l.Len())
	}
	if l.Len() > maxPiece {
		require.FailNowf(c.t, "", "%s at %d is %d bytes, want at most %d", l.Type(), l.Offset(), l.Len(), maxPiece)
	}
}

// minCeiling is the least heap ceiling of the long-run and corruption tests,
// above the harness floor of 48 KiB. The cursor keeps a line table of 16
// bytes per line for its 64 KiB buffer, and through the byte adapter every
// 0x0A byte of a payload starts a line. The generated payloads hold CR LF
// pairs, about one 0x0A byte in 20, so the table grows by an estimated
// 50 KiB as the first buffer fills, and a run under the race detector
// (64 KiB, with the baseline after 4 KiB) measured 71 to 78 KiB of growth.
// The growth does not follow the length of the stream: with the same sampling
// and no race detector it is 72 KiB after 128 KiB and after 256 KiB.
const minCeiling = 128 << 10

// ceiling returns the heap ceiling for a stream of budget bytes: the harness
// default, but at least minCeiling.
func ceiling(budget int64) streamtest.Option {
	return streamtest.WithCeiling(max(streamtest.DefaultCeiling(budget), minCeiling))
}

// TestLongRun lexes a generated server stream of the harness default budget, or
// of 1 GiB with -streamtest.long, under the harness ceiling. Every operation
// lexes as one OP, every payload byte is in a PAYLOAD or HEADERS token, and
// no byte is UNKNOWN.
func TestLongRun(t *testing.T) {
	s := &serverStream{}
	gen := streamtest.NewGenerator(1, s.record)
	c := &payloadCounter{t: t}
	res := streamtest.Run(t, newScanner, gen, ceiling(gen.Budget()), streamtest.WithTokenFunc(c.add))
	{
		got := res.Counts[op]
		require.Equalf(t, s.ops, got, "lexed %d OP, want one per line (%d)", got, s.ops)
	}
	{
		got := res.Counts[bytescan.TokenTypeUnknown]
		require.Zerof(t, got, "lexed %d UNKNOWN in a clean stream", got)
	}
	if c.payload != s.payloadBytes || c.header != s.headerBytes {
		require.FailNowf(t, "", "PAYLOAD and HEADERS hold %d and %d bytes, the stream has %d and %d",
			c.payload, c.header, s.payloadBytes, s.headerBytes)
	}
	if res.Bytes != gen.Bytes() {
		require.FailNowf(t, "", "Run read %d bytes, the generator wrote %d", res.Bytes, gen.Bytes())
	}
}

// garbage is the alphabet of the injected runs: no operation starts with any
// of its bytes, and it has no CR or LF.
const garbage = "#%&*~\x80\xfe"

// TestCorruption lexes a server stream with garbage runs between its records.
// As the tokens arrive, it checks that each run lexes as UNKNOWN tokens,
// that the next token is the OP of the record after it, and that no token
// outside a run is UNKNOWN: every payload after a run is framed again.
func TestCorruption(t *testing.T) {
	s := &serverStream{}
	c := streamtest.Corrupt(streamtest.NewGenerator(3, s.record), 4, 0.1, streamtest.Garbage(garbage, 1, 64))
	var (
		cur     streamtest.Injection
		have    bool
		unknown int64
		resumed int64
	)
	check := func(l *bytescan.Token) {
		off := int64(l.Offset())
		if !have || off >= cur.Offset+int64(cur.Len) {
			if have && off == cur.Offset+int64(cur.Len) {
				if l.Type() != op {
					require.FailNowf(t, "", "after the garbage at %d: got %s %q, want OP", cur.Offset, l.Type(), l.Text())
				}
				resumed++
			}
			cur, have = c.NextInjection()
		}
		inside := have && off >= cur.Offset && off < cur.Offset+int64(cur.Len)
		if inside != (l.Type() == bytescan.TokenTypeUnknown) {
			require.FailNowf(t, "", "at %d: got %s %q, garbage run here: %v", off, l.Type(), l.Text(), inside)
		}
		if inside {
			unknown++
		}
	}
	res := streamtest.Run(t, newScanner, c, ceiling(c.Budget()), streamtest.WithTokenFunc(check))
	require.Falsef(t, res.Counts[bytescan.TokenTypeUnknown] != unknown || unknown == 0, "lexed %d UNKNOWN, %d inside garbage runs", res.Counts[bytescan.TokenTypeUnknown], unknown)
	if resumed != int64(c.Injected()) {
		require.FailNowf(t, "", "resumed at an OP after %d of %d garbage runs", resumed, c.Injected())
	}
	{
		got := res.Counts[op]
		require.Equalf(t, s.ops, got, "lexed %d OP, want one per line (%d)", got, s.ops)
	}
	t.Logf("%d garbage runs, %d UNKNOWN tokens", c.Injected(), unknown)
}

// TestCorruptSize lexes a MSG line whose size field is damaged into a large
// size, then a stream that ends long before it. The driver reads the payload
// in pieces of at most 64 KiB up to the end of the input, under the harness
// ceiling, without holding the declared size.
func TestCorruptSize(t *testing.T) {
	const body = 2*maxPiece + 1000
	once := func(rng *rand.Rand, i int, dst []byte) []byte {
		dst = append(dst, "MSG a 1 9999999999\r\n"...)
		for len(dst) < body {
			dst = append(dst, byte(rng.Intn(256)))
		}
		return dst
	}
	gen := streamtest.NewGenerator(5, once, streamtest.WithBudget(1))
	c := &payloadCounter{t: t}
	res := streamtest.Run(t, newScanner, gen, streamtest.WithTokenFunc(c.add))
	{
		want := int64(body - len("MSG a 1 9999999999\r\n"))
		require.Equalf(t, want, c.payload, "PAYLOAD holds %d bytes, want %d", c.payload, want)
	}
	{
		got, want := res.Counts[payload], int64((body+maxPiece-1)/maxPiece)
		require.Equalf(t, want, got, "lexed %d PAYLOAD, want %d", got, want)
	}
}

// warmUp returns control lines that run every scanner of the driver, padded
// with PING lines to 5 KiB. A scanner holds about 36 KB more once it has run,
// whatever the input, so a stream that starts with warmUp takes its baseline
// heap sample after the driver has reached its steady size.
func warmUp() []byte {
	b := append(append(session(), payloads()...), headerInputs()...)
	b = append(b, "PUB a 12345678901\r\nHPUB a 3 999999999999\r\n"...)
	for len(b) < 5<<10 {
		b = append(b, "PING\r\n"...)
	}
	return b
}

// TestUndelimited lexes, for each field that runs to a delimiter, one run of
// at least 16 KiB with no delimiter in it, under the harness ceiling and the
// per-token guard. The run is emitted as pieces of at most 1000 bytes of one
// type that together hold the run, and the next control line lexes as usual.
// The stream starts with warmUp, so the heap ceiling sees the run and not the
// first use of a scanner. The runs of the text fields are bytes of 0x80 or
// above, so the guard is proven not to reject a legal piece of 1000 such
// bytes.
func TestUndelimited(t *testing.T) {
	const n = 16<<10 + 123
	high := func(i int) byte { return 0x80 + byte(i%0x80) }
	run := func(gen func(int) byte) []byte {
		b := make([]byte, n)
		for i := range b {
			b[i] = gen(i)
		}
		return b
	}
	tests := []struct {
		name   string
		before string
		run    []byte
		typ    bytescan.TokenType
		after  string
	}{
		{"json", "INFO ", run(high), jsonArg, "\r\n"},
		{"subject", "SUB ", run(high), subject, " 1\r\n"},
		{"sid", "SUB a ", run(high), sid, "\r\n"},
		{"reply", "PUB a ", run(high), replyTo, " 0\r\n\r\n"},
		{"digits", "PUB a ", run(func(i int) byte { return '0' + byte(i%10) }), bytescan.TokenTypeUnknown, "\r\n"},
		{"size digits", "PUB a b ", run(func(i int) byte { return '9' }), bytescan.TokenTypeUnknown, "\r\n"},
		{"blanks", "PUB", run(func(i int) byte { return " \t"[i%2] }), sp, "a 0\r\n\r\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warm := warmUp()
			start := len(warm) + len(tt.before)
			in := append(append(append(append(warm, tt.before...), tt.run...), tt.after...), "PING\r\n"...)
			record := func(rng *rand.Rand, i int, dst []byte) []byte { return append(dst, in...) }
			var (
				pieces  int
				got     []byte
				ls      []string
				started bool
				ended   bool
			)
			check := func(l *bytescan.Token) {
				ls = append(ls, string(l.Type()))
				if len(ls) > 64 {
					ls = ls[1:]
				}
				inRun := l.Offset() >= start && l.Offset() < start+n
				if !inRun {
					if started {
						ended = true
					}
					return
				}
				started = true
				if ended || l.Type() != tt.typ {
					require.FailNowf(t, "", "at %d: got %s, want a %s piece of the run", l.Offset(), l.Type(), tt.typ)
				}
				if l.Len() > maxRun {
					require.FailNowf(t, "", "at %d: %s piece of %d bytes, want at most %d", l.Offset(), l.Type(), l.Len(), maxRun)
				}
				pieces++
				got = append(got, bytesource.Bytes(l.Text())...)
			}
			streamtest.Run(t, newScanner, streamtest.NewGenerator(1, record, streamtest.WithBudget(1)), streamtest.WithTokenFunc(check))
			if !bytes.Equal(got, tt.run) {
				require.FailNowf(t, "", "the pieces hold %d bytes, want the run of %d", len(got), n)
			}
			require.GreaterOrEqualf(t, pieces, n/maxRun, "%d pieces, want at least %d", pieces, n/maxRun)
			{
				tail := strings.Join(ls[len(ls)-3:], " ")
				require.Equalf(t, "CRLF OP CRLF", tail, "the stream ends with %s, want CRLF OP CRLF", tail)
			}
		})
	}
}
