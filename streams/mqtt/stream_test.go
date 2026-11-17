package main

import (
	"bytes"
	"math/rand"
	"testing"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/streamtest"
)

// tally counts what a generator wrote.
type tally struct {
	records   int64 // packets
	publishes int64 // PUBLISH packets, each with a VARIABLE_HEADER
	bodies    int64 // other packets with a remaining length above 0
}

var (
	topics    = []string{"sensors/7/temp", "home/kitchen/light", "a", "fleet/truck-12/gps", "café/über/☕", "$SYS/broker/load"}
	filters   = []string{"sensors/+/temp", "home/#", "#", "fleet/+/gps", "café/+"}
	clientIDs = []string{"sensor-7", "", "dashboard", "truck-12", "ünïcode"}
)

// record returns the streamtest.RecordFunc of an endless MQTT stream, which
// counts its packets in c. Record i is one packet, picked at random: CONNECT,
// PUBLISH at QoS 0, 1 or 2 (half of the packets), PUBACK, SUBSCRIBE, PINGREQ
// or PINGRESP. A PUBLISH payload takes every byte value and is usually short;
// one in 512 is 16 KiB to 80 KiB, so its remaining length has 3 bytes and
// some payloads are read in two pieces. The remaining length is 1 to 4 bytes
// at random: a width above the fewest bytes pads the encoding (see
// encodeLength). No byte of the stream lexes as UNKNOWN.
func record(c *tally) streamtest.RecordFunc {
	return func(rng *rand.Rand, i int, dst []byte) []byte {
		width := 1 + rng.Intn(maxLengthBytes)
		id := u16(1 + i%0xFFFF)
		c.records++
		var p []byte
		switch kind := rng.Intn(10); {
		case kind < 5:
			qos := byte(rng.Intn(3))
			parts := [][]byte{str(topics[rng.Intn(len(topics))])}
			if qos > 0 {
				parts = append(parts, id)
			}
			n := rng.Intn(64)
			switch r := rng.Intn(512); {
			case r == 0:
				n = 16<<10 + rng.Intn(64<<10)
			case r < 64:
				n = 128 + rng.Intn(1000)
			}
			data := make([]byte, n)
			rng.Read(data)
			parts = append(parts, data)
			p = packet(publishHeader(rng.Intn(8) == 0, qos, rng.Intn(4) == 0), width, parts...)
			c.publishes++
		case kind == 5:
			p = packet(connect, width, str("MQTT"), []byte{4, 0x02}, u16(60), str(clientIDs[rng.Intn(len(clientIDs))]))
			c.bodies++
		case kind == 6:
			p = packet(puback, width, id)
			c.bodies++
		case kind == 7:
			parts := [][]byte{id}
			for n := 1 + rng.Intn(3); n > 0; n-- {
				parts = append(parts, str(filters[rng.Intn(len(filters))]), []byte{byte(rng.Intn(3))})
			}
			p = packet(subscribe, width, parts...)
			c.bodies++
		case kind == 8:
			p = packet(pingreq, width)
		default:
			p = packet(pingresp, width)
		}
		return append(dst, p...)
	}
}

// contiguous returns a token func that fails the test when a token does not
// start where the one before it ended, or when a field piece is longer than
// maxPiece, and that calls fn, when it is not nil, with each token. *pos is
// the end of the last token.
func contiguous(t *testing.T, pos *int64, fn func(*bytescan.Token)) streamtest.Option {
	return streamtest.WithTokenFunc(func(l *bytescan.Token) {
		if off := int64(l.Offset()); off != *pos {
			t.Fatalf("%s at %d, want the next token at %d", l.Type(), off, *pos)
		}
		if l.Len() > maxPiece {
			t.Fatalf("%s of %d bytes at %d, longer than a piece", l.Type(), l.Len(), l.Offset())
		}
		*pos += int64(l.Len())
		if fn != nil {
			fn(l)
		}
	})
}

// pieceCeiling is the least heap ceiling of the long-run and corruption
// tests: two pieces. The driver reads a field in pieces of up to maxPiece
// bytes, and the harness samples the heap while the last token is live, so a
// sample can hold one full piece in the cursor buffer as well as the token
// and the cursor's line table. Over the 256 KiB race budget of streamtest
// (bytescan#87) the growth measured, in 30 runs each, at most 48.2 KiB for
// the long run, 98.5 KiB for "invalid length" and 101.4 KiB for "any bytes"
// under the race detector. The 48 KiB race default of the harness held only
// while the race budget was 64 KiB, over which these tests grew at most
// 33.8 KiB. Two pieces leave about 27 KiB above the largest measurement.
// Without the race detector the budget is 512 KiB, the growth at most 48.3,
// 94.7 and 129.8 KiB, and the harness default of 256 KiB applies.
const pieceCeiling = 2 * maxPiece

// ceiling returns the heap ceiling for src: the harness default for its
// budget, but at least pieceCeiling.
func ceiling(src streamtest.Source) streamtest.Option {
	return streamtest.WithCeiling(max(streamtest.DefaultCeiling(src.Budget()), pieceCeiling))
}

// TestLongRun lexes an endless stream of the default budget, or of 1 GiB
// with -streamtest.long, under the heap ceiling of ceiling, and checks the
// tokens against the packets: one FIXED_HEADER and one REMAINING_LENGTH per
// packet, of every width from 1 to 4 bytes, one VARIABLE_HEADER per PUBLISH,
// one BODY per other packet with a remaining length above 0, and no UNKNOWN.
//
//	go test -timeout 0 -run Long ./streams/mqtt -streamtest.long
func TestLongRun(t *testing.T) {
	var c tally
	gen := streamtest.NewGenerator(57, record(&c))
	var (
		pos    int64
		widths [maxLengthBytes + 1]int64
	)
	res := streamtest.Run(t, newScanner, gen, ceiling(gen), contiguous(t, &pos, func(l *bytescan.Token) {
		if l.Type() == remainingLength {
			widths[l.Len()]++
		}
	}))
	if pos != res.Bytes || res.Bytes != gen.Bytes() {
		t.Errorf("tokens end at %d, Run read %d bytes, the generator wrote %d", pos, res.Bytes, gen.Bytes())
	}
	for _, w := range []struct {
		typ  bytescan.TokenType
		want int64
	}{
		{fixedHeader, c.records},
		{remainingLength, c.records},
		{variableHeader, c.publishes},
		{body, c.bodies},
		{unknown, 0},
	} {
		if got := res.Counts[w.typ]; got != w.want {
			t.Errorf("lexed %d %s, want %d", got, w.typ, w.want)
		}
	}
	if res.Counts[payload] == 0 {
		t.Error("lexed no PAYLOAD")
	}
	for w := 1; w <= maxLengthBytes; w++ {
		if widths[w] == 0 {
			t.Errorf("no REMAINING_LENGTH of %d bytes; widths = %v", w, widths[1:])
		}
	}
	t.Logf("%d packets, %d PUBLISH; REMAINING_LENGTH widths 1-4: %v", c.records, c.publishes, widths[1:])
}

// invalidLengthGarbage appends a garbage run that lexes as an invalid
// remaining length when it starts where a packet should: 5 to 64 bytes with
// the high bit set, then one byte without it. The first byte is a
// FIXED_HEADER, the next four make the length invalid, and the rest of the
// run, up to and including its last byte, is UNKNOWN.
func invalidLengthGarbage(rng *rand.Rand, dst []byte) []byte {
	for n := 5 + rng.Intn(60); n > 0; n-- {
		dst = append(dst, byte(0x80+rng.Intn(0x80)))
	}
	return append(dst, byte(rng.Intn(0x80)))
}

// anyBytes is every byte value.
var anyBytes = func() string {
	b := make([]byte, 256)
	for c := range b {
		b[c] = byte(c)
	}
	return string(b)
}()

// TestCorruption inserts garbage runs between packets. MQTT has no sync
// marker, so the driver cannot look for the next packet after corrupt data:
// it frames a packet at the byte where the last one ended.
//
//   - "invalid length": every run lexes as an invalid remaining length (see
//     invalidLengthGarbage), which ends at the last byte of the run. The test
//     checks every token: in a run, a FIXED_HEADER and then UNKNOWN bytes;
//     outside the runs, every packet framed and no UNKNOWN.
//   - "any bytes": runs of 1 to 64 random bytes. Every packet before the
//     first run is framed. After it the driver frames packets at whatever
//     bytes follow, and a corrupt remaining length, up to 268,435,455 bytes,
//     may read the rest of the stream as fields. The test asserts that lexing
//     reaches the end of the stream without an error and under the heap
//     ceiling of ceiling, and that the tokens cover the stream without a gap.
func TestCorruption(t *testing.T) {
	t.Run("invalid length", func(t *testing.T) {
		var c tally
		gen := streamtest.NewGenerator(57, record(&c))
		src := streamtest.Corrupt(gen, 7, 0.05, invalidLengthGarbage)
		var (
			pos     int64
			in      streamtest.Injection
			have    bool
			garbage int64 // UNKNOWN tokens inside garbage
		)
		res := streamtest.Run(t, newScanner, src, ceiling(src), contiguous(t, &pos, func(l *bytescan.Token) {
			off := int64(l.Offset())
			if !have || off >= in.Offset+int64(in.Len) {
				in, have = src.NextInjection()
			}
			switch {
			case !have || off < in.Offset:
				if l.Type() == unknown {
					t.Fatalf("UNKNOWN at %d, outside the garbage", off)
				}
			case off == in.Offset:
				if l.Type() != fixedHeader {
					t.Fatalf("%s at the garbage start %d, want FIXED_HEADER", l.Type(), off)
				}
			default:
				if l.Type() != unknown || l.Len() != 1 {
					t.Fatalf("%s of %d bytes at %d inside the garbage (%d bytes at %d), want one UNKNOWN byte", l.Type(), l.Len(), off, in.Len, in.Offset)
				}
				garbage++
			}
		}))
		if pos != res.Bytes {
			t.Errorf("tokens end at %d, want the end of the stream at %d", pos, res.Bytes)
		}
		runs := int64(src.Injected())
		if runs == 0 {
			t.Fatal("no garbage runs")
		}
		if got, want := res.Counts[fixedHeader], c.records+runs; got != want {
			t.Errorf("lexed %d FIXED_HEADER, want one per packet and one per garbage run (%d)", got, want)
		}
		if got := res.Counts[unknown]; got != garbage {
			t.Errorf("lexed %d UNKNOWN, %d of them inside garbage", got, garbage)
		}
		t.Logf("%d packets, %d garbage runs, %d UNKNOWN", c.records, runs, garbage)
	})

	t.Run("any bytes", func(t *testing.T) {
		var c tally
		gen := streamtest.NewGenerator(57, record(&c))
		src := streamtest.Corrupt(gen, 8, 0.02, streamtest.Garbage(anyBytes, 1, 64))
		var (
			pos     int64
			first   streamtest.Injection
			have    bool
			headers int64 // FIXED_HEADER tokens before the first garbage run
			after   int64 // tokens from the first garbage run on
		)
		res := streamtest.Run(t, newScanner, src, ceiling(src), contiguous(t, &pos, func(l *bytescan.Token) {
			if !have {
				first, have = src.NextInjection()
			}
			off := int64(l.Offset())
			if have && off >= first.Offset {
				after++
				return
			}
			if l.Type() == unknown {
				t.Fatalf("UNKNOWN at %d, before the first garbage run", off)
			}
			if l.Type() == fixedHeader {
				headers++
			}
		}))
		if pos != res.Bytes {
			t.Errorf("tokens end at %d, want the end of the stream at %d", pos, res.Bytes)
		}
		if !have {
			t.Fatal("no garbage runs")
		}
		if headers != int64(first.Record) {
			t.Errorf("lexed %d FIXED_HEADER before the first garbage run, want one per packet (%d)", headers, first.Record)
		}
		t.Logf("%d packets, %d garbage runs; the first at %d after %d packets; %d tokens from it to the end of the stream at %d, %d UNKNOWN",
			c.records, src.Injected(), first.Offset, first.Record, after, res.Bytes, res.Counts[unknown])
	})
}

// warmUp is the size of the packets that a test lexes before a fixed run. The
// first packets grow the live heap once, as the scanners and the cursor
// allocate on first use. The tests sample every warmUp bytes, so the
// baseline, the first sample, comes after the warm-up.
const warmUp = 4 << 10

// withRun returns the source of warmUp bytes of packets, run, and the stream
// of gen, and the offset of run in it.
func withRun(run []byte, gen *streamtest.Generator) (streamtest.Source, int64) {
	head := streamtest.Records(99, record(&tally{}), warmUp)
	return streamtest.Prefix(gen, head, run), int64(len(head))
}

// TestMaxLength puts a packet whose remaining length is the maximum,
// 268,435,455 bytes (0xFF 0xFF 0xFF 0x7F), before a generated stream of the
// default budget, which is far shorter. The driver reads the rest of the
// stream as pieces of at most maxPiece bytes: BODY for a CONNECT, and
// VARIABLE_HEADER then PAYLOAD for a PUBLISH, whose topic length is the first
// two bytes of the next packet. Lexing reaches the end without an error, and
// memory follows the piece size, not the length, under the harness ceiling.
func TestMaxLength(t *testing.T) {
	for _, tc := range []struct {
		name   string
		header byte
		types  []bytescan.TokenType // the types after the length, in order
	}{
		{"CONNECT", connect, []bytescan.TokenType{body}},
		{"PUBLISH", publishHeader(false, 1, false), []bytescan.TokenType{variableHeader, payload}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var c tally
			budget := min(streamtest.DefaultBudget(), 512<<10)
			gen := streamtest.NewGenerator(58, record(&c), streamtest.WithBudget(budget))
			src, at := withRun([]byte{tc.header, 0xFF, 0xFF, 0xFF, 0x7F}, gen)
			fields := at + 5 // the first byte after the length
			var (
				pos  int64
				next int // index of the type expected now in tc.types
			)
			res := streamtest.Run(t, newScanner, src, contiguous(t, &pos, func(l *bytescan.Token) {
				off := int64(l.Offset())
				switch {
				case off == at && l.Type() != fixedHeader, off == at+1 && l.Type() != remainingLength:
					t.Fatalf("%s at %d, want the packet with the maximal length", l.Type(), off)
				case off < fields:
					return
				}
				for next < len(tc.types) && l.Type() != tc.types[next] {
					next++
				}
				if next == len(tc.types) {
					t.Fatalf("%s at %d, want a piece of %v", l.Type(), off, tc.types)
				}
			}), streamtest.WithSampleEvery(warmUp))
			if pos != res.Bytes {
				t.Errorf("tokens end at %d, want the end of the stream at %d", pos, res.Bytes)
			}
		})
	}
}

// TestHighRun lexes a run of 18 KiB of 0xFF where a packet starts: the first
// byte is a FIXED_HEADER and every other byte of the run is one UNKNOWN, under
// the harness ceiling and the per-token guard. The bad-length scanner reads one
// byte per token, so the run never grows one token.
func TestHighRun(t *testing.T) {
	const n = 18 << 10
	var c tally
	gen := streamtest.NewGenerator(59, record(&c), streamtest.WithBudget(2<<10))
	src, at := withRun(bytes.Repeat([]byte{0xFF}, n), gen)
	var (
		pos  int64
		seen int64
	)
	streamtest.Run(t, newScanner, src, contiguous(t, &pos, func(l *bytescan.Token) {
		off := int64(l.Offset())
		switch {
		case off == at:
			if l.Type() != fixedHeader {
				t.Fatalf("%s at the run start %d, want FIXED_HEADER", l.Type(), off)
			}
		case off > at && off < at+n:
			if l.Type() != unknown || l.Len() != 1 {
				t.Fatalf("%s of %d bytes at %d in the run, want one UNKNOWN byte", l.Type(), l.Len(), off)
			}
			seen++
		}
	}), streamtest.WithSampleEvery(warmUp))
	if seen != n-1 {
		t.Errorf("lexed %d UNKNOWN in the run, want %d", seen, n-1)
	}
}
