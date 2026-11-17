package main

import (
	"encoding/binary"
	"math/rand"
	"testing"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/bytesource"
	"github.com/xiam/bytescan/streamtest"
)

// pieceHeap is the heap that one PAYLOAD piece holds while the harness holds
// the token: maxPiece bytes, each one a rune of 4 bytes. The stream tests
// that read pieces of up to maxPiece bytes raise the harness ceiling by this
// much, because the harness samples the heap while the last token is live.
const pieceHeap = 4 * maxPiece

// frameInfo is a frame that a generator wrote: where it starts, and the
// tokens the driver should give for it.
type frameInfo struct {
	offset int64
	ext    int // the bytes of EXTENDED_LENGTH: 0, 2 or 8
	masked bool
	length int64
}

// generator writes an endless stream of WebSocket messages for the harness
// and records every frame it writes, so a test can check the tokens of each
// frame as they arrive. It holds only the frames that the scanner has not
// checked yet.
type generator struct {
	pos     int64       // the stream offset of the next frame
	frames  []frameInfo // written, not checked yet
	untrack bool        // stop recording frames: nothing checks them any more
	count   int64       // frames written
	kinds   [3]int64    // frames written by length width: 7, 16 and 64 bits
	masked  int64       // masked frames written
	data    int64       // payload bytes written
}

// payloadLength returns the payload length of a data frame: a 7-bit length
// (0 to 125 bytes), or, one in five, a 16-bit length (126 to 4,096 bytes).
func payloadLength(rng *rand.Rand) int {
	switch {
	case rng.Intn(5) == 0:
		return length16 + rng.Intn(4096-length16+1)
	default:
		return rng.Intn(length16)
	}
}

// appendData appends n payload bytes: printable ASCII for a text frame, any
// byte otherwise.
func appendData(rng *rand.Rand, dst []byte, opcode byte, n int) []byte {
	for ; n > 0; n-- {
		if opcode == opText || opcode == opContinuation {
			dst = append(dst, byte(' '+rng.Intn(95)))
		} else {
			dst = append(dst, byte(rng.Intn(256)))
		}
	}
	return dst
}

// frame appends one frame of the given opcode and payload length, masked
// with a random key when masked is set, and records it.
func (g *generator) frame(rng *rand.Rand, dst []byte, fin bool, opcode byte, masked bool, n int) []byte {
	var key []byte
	if masked {
		key = binary.BigEndian.AppendUint32(nil, rng.Uint32())
	}
	start := len(dst)
	dst = appendFrame(dst, fin, opcode, key, appendData(rng, nil, opcode, n))
	f := frameInfo{offset: g.pos, masked: masked, length: int64(n)}
	switch {
	case n < length16:
		g.kinds[0]++
	case n <= 0xFFFF:
		f.ext = 2
		g.kinds[1]++
	default:
		f.ext = 8
		g.kinds[2]++
	}
	if masked {
		g.masked++
	}
	g.pos += int64(len(dst) - start)
	if !g.untrack {
		g.frames = append(g.frames, f)
	}
	g.count++
	g.data += int64(n)
	return dst
}

// record is the streamtest.RecordFunc of g: one message per record, from a
// client (masked frames) or from a server (unmasked frames). A message is a
// text or binary frame, a ping, a pong or a close frame, or, one in eight, a
// text or binary message in 2 to 4 fragments with a ping between two of
// them. Record 32 of every 256 is a binary frame with a 64-bit length (65,536
// to 66,560 bytes), so every stream of the default budget, also under the
// race detector, has all three length widths.
func (g *generator) record(rng *rand.Rand, i int, dst []byte) []byte {
	masked := rng.Intn(2) == 0
	switch kind := rng.Intn(16); {
	case i%256 == 32:
		return g.frame(rng, dst, true, opBinary, masked, 1<<16+rng.Intn(1<<10+1))
	case kind < 2: // a fragmented message
		op := []byte{opText, opBinary}[rng.Intn(2)]
		n := 2 + rng.Intn(3)
		ping := rng.Intn(n - 1)
		for f := 0; f < n; f++ {
			if f > 0 {
				op = opContinuation
			}
			dst = g.frame(rng, dst, f == n-1, op, masked, payloadLength(rng))
			if f == ping {
				dst = g.frame(rng, dst, true, opPing, masked, rng.Intn(length16))
			}
		}
		return dst
	case kind < 9:
		return g.frame(rng, dst, true, opText, masked, payloadLength(rng))
	case kind < 13:
		return g.frame(rng, dst, true, opBinary, masked, payloadLength(rng))
	case kind < 14:
		return g.frame(rng, dst, true, opPing, masked, rng.Intn(length16))
	case kind < 15:
		return g.frame(rng, dst, true, opPong, masked, rng.Intn(length16))
	default:
		return g.frame(rng, dst, true, opClose, masked, 2+rng.Intn(length16-2))
	}
}

// checker checks the tokens of the frames that a generator wrote, in order.
type checker struct {
	t     *testing.T
	gen   *generator
	cur   frameInfo
	next  int64 // the offset of the next token
	ext   bool  // EXTENDED_LENGTH still to come
	key   bool  // MASKING_KEY still to come
	left  int64 // payload bytes still to come
	inner bool  // inside a frame
}

// check checks l against the frame it belongs to.
func (c *checker) check(l *bytescan.Token) {
	t := c.t
	off, n := int64(l.Offset()), int64(l.Len())
	if off != c.next {
		t.Fatalf("%s at %d, want the next token at %d", l.Type(), off, c.next)
	}
	c.next = off + n
	if !c.inner {
		if len(c.gen.frames) == 0 {
			t.Fatalf("%s at %d after the last frame", l.Type(), off)
		}
		c.cur, c.gen.frames = c.gen.frames[0], c.gen.frames[1:]
		if l.Type() != frameHeader || off != c.cur.offset || n != headerSize {
			t.Fatalf("%s of %d bytes at %d, want FRAME_HEADER at %d", l.Type(), n, off, c.cur.offset)
		}
		c.ext, c.key, c.left = c.cur.ext > 0, c.cur.masked, c.cur.length
		c.inner = c.ext || c.key || c.left > 0
		return
	}
	switch {
	case c.ext:
		if l.Type() != extendedLength || n != int64(c.cur.ext) {
			t.Fatalf("%s of %d bytes at %d, want EXTENDED_LENGTH of %d", l.Type(), n, off, c.cur.ext)
		}
		c.ext = false
	case c.key:
		if l.Type() != maskingKey || n != keySize {
			t.Fatalf("%s of %d bytes at %d, want MASKING_KEY", l.Type(), n, off)
		}
		c.key = false
	default:
		if want := min(c.left, maxPiece); l.Type() != payload || n != want {
			t.Fatalf("%s of %d bytes at %d, want PAYLOAD of %d", l.Type(), n, off, want)
		}
		c.left -= n
	}
	c.inner = c.ext || c.key || c.left > 0
}

// TestLongRun lexes a generated stream of the default budget, or of 1 GiB
// with -streamtest.long, under the harness ceiling, and checks every token
// against the frame the generator wrote:
//
//	go test -timeout 0 -run Long ./streams/websocket -streamtest.long
func TestLongRun(t *testing.T) {
	g := &generator{}
	gen := streamtest.NewGenerator(56, g.record)
	c := &checker{t: t, gen: g}
	res := streamtest.Run(t, newScanner, gen,
		streamtest.WithTokenFunc(c.check),
		streamtest.WithCeiling(streamtest.DefaultCeiling(gen.Budget())+pieceHeap))
	if c.inner || len(g.frames) != 0 {
		t.Errorf("the tokens end inside a frame, or %d frames are not lexed", len(g.frames))
	}
	if got := res.Counts[frameHeader]; got != g.count {
		t.Errorf("FRAME_HEADER tokens = %d, want one per frame, %d", got, g.count)
	}
	if got, want := res.Counts[extendedLength], g.kinds[1]+g.kinds[2]; got != want {
		t.Errorf("EXTENDED_LENGTH tokens = %d, want %d", got, want)
	}
	if got := res.Counts[maskingKey]; got != g.masked {
		t.Errorf("MASKING_KEY tokens = %d, want %d", got, g.masked)
	}
	if n := res.Counts[bytescan.TokenTypeUnknown]; n != 0 {
		t.Errorf("UNKNOWN tokens = %d, want 0", n)
	}
	if g.kinds[0] == 0 || g.kinds[1] == 0 || g.kinds[2] == 0 || g.masked == 0 || g.masked == g.count {
		t.Errorf("frames by length width %v, %d of %d masked: want every width, masked and unmasked", g.kinds, g.masked, g.count)
	}
	t.Logf("%d frames (%v by width), %d masked, %d payload bytes", g.count, g.kinds, g.masked, g.data)
}

// bounded checks a token of a stream that may have lost its framing: it has
// one of the sizes that the driver gives.
func bounded(t *testing.T, l *bytescan.Token) {
	n := l.Len()
	ok := false
	switch l.Type() {
	case frameHeader:
		ok = n == headerSize
	case extendedLength:
		ok = n == 2 || n == 8
	case maskingKey:
		ok = n == keySize
	case payload:
		ok = n >= 1 && n <= maxPiece
	case bytescan.TokenTypeUnknown:
		ok = n == 1
	}
	if !ok {
		t.Fatalf("%s of %d bytes at %d", l.Type(), n, l.Offset())
	}
}

// corrupt runs the corruption test with the given garbage. Up to the first
// garbage run, every token is checked against its frame. From there on the
// stream has lost its framing: WebSocket has no sync marker, so the driver
// cannot find the next frame (see the README). The test checks what still
// holds: the tokens cover the stream without a gap, each has a size the
// driver gives (a PAYLOAD piece at most maxPiece bytes), lexing reaches the
// end of the stream with no error, and the heap stays under the ceiling. It
// returns the tokens after the first garbage run, by type.
func corrupt(t *testing.T, garbage streamtest.GarbageFunc) map[bytescan.TokenType]int64 {
	g := &generator{}
	gen := streamtest.NewGenerator(56, g.record)
	src := streamtest.Corrupt(gen, 3, 0.1, garbage)
	c := &checker{t: t, gen: g}
	var (
		first  *streamtest.Injection // the first garbage run, once reported
		framed int64                 // frames lexed before it
		after  = map[bytescan.TokenType]int64{}
	)
	res := streamtest.Run(t, newScanner, src,
		streamtest.WithCeiling(streamtest.DefaultCeiling(src.Budget())+pieceHeap),
		streamtest.WithTokenFunc(func(l *bytescan.Token) {
			if first == nil {
				if in, ok := src.NextInjection(); ok {
					first = &in
				}
			}
			if first != nil && !g.untrack {
				// The frames after the first garbage run are not checked.
				g.untrack = true
			}
			off := int64(l.Offset())
			if first == nil || off < first.Offset {
				c.check(l)
				if l.Type() == frameHeader {
					framed++
				}
				return
			}
			if off == first.Offset && c.inner {
				t.Fatalf("garbage at %d inside a frame", off)
			}
			if off != c.next {
				t.Fatalf("%s at %d, want the next token at %d", l.Type(), off, c.next)
			}
			c.next = off + int64(l.Len())
			bounded(t, l)
			after[l.Type()]++
		}))
	if first == nil {
		t.Fatal("no garbage injected")
	}
	if c.next != res.Bytes {
		t.Errorf("tokens end at %d, want the end of the stream at %d", c.next, res.Bytes)
	}
	t.Logf("%d frames before the first garbage run at %d; after it: %v", framed, first.Offset, after)
	return after
}

// TestCorruption injects runs of 1 to 64 random bytes between messages.
// After the first run the driver reads the garbage as a frame header, with a
// length that the garbage gives, and the stream stays out of frame.
func TestCorruption(t *testing.T) {
	alphabet := make([]byte, 256)
	for i := range alphabet {
		alphabet[i] = byte(i)
	}
	corrupt(t, streamtest.Garbage(string(alphabet), 1, 64))
}

// maximal is a garbage run of the header of a masked binary frame whose 64-bit
// length is the largest value, 2^64-1, and its masking key.
func maximal(rng *rand.Rand, dst []byte) []byte {
	dst = append(dst, 0x82, maskBit|length64)
	dst = binary.BigEndian.AppendUint64(dst, ^uint64(0))
	return binary.BigEndian.AppendUint32(dst, rng.Uint32())
}

// TestCorruptionMaximalLength injects garbage that declares a maximal 64-bit
// length. The first run frames a payload that runs to the end of the stream:
// after it, the stream lexes as that one frame, its payload in pieces of at
// most maxPiece bytes, under the harness ceiling.
func TestCorruptionMaximalLength(t *testing.T) {
	after := corrupt(t, maximal)
	if after[frameHeader] != 1 || after[extendedLength] != 1 || after[maskingKey] != 1 || after[bytescan.TokenTypeUnknown] != 0 {
		t.Errorf("after the first garbage run: %v, want one frame whose payload runs to the end", after)
	}
}

// TestHugeLength lexes a frame whose 64-bit length, 2^63-1, is larger than
// the rest of the stream, a generated stream of the default budget. The
// driver reads the rest of the stream as PAYLOAD pieces of maxPiece bytes and
// stops normally at its end, under the harness ceiling.
func TestHugeLength(t *testing.T) {
	g := &generator{}
	gen := streamtest.NewGenerator(7, g.record)
	head := []byte{0x82, length64}
	head = binary.BigEndian.AppendUint64(head, 1<<63-1)
	src := streamtest.Prefix(gen, head)
	body := int64(len(head))
	var got int64
	res := streamtest.Run(t, newScanner, src,
		streamtest.WithCeiling(streamtest.DefaultCeiling(src.Budget())+pieceHeap),
		streamtest.WithTokenFunc(func(l *bytescan.Token) {
			off := int64(l.Offset())
			if off < body {
				return
			}
			if l.Type() != payload || off != body+got || l.Len() > maxPiece {
				t.Fatalf("at %d: %s of %d bytes, want a PAYLOAD piece of at most %d bytes at %d", off, l.Type(), l.Len(), maxPiece, body+got)
			}
			got += int64(l.Len())
		}))
	if want := res.Bytes - body; got != want {
		t.Errorf("lexed %d bytes of PAYLOAD, want the rest of the stream (%d)", got, want)
	}
	if res.Counts[frameHeader] != 1 || res.Counts[extendedLength] != 1 {
		t.Errorf("counts = %v, want one frame", res.Counts)
	}
}

// TestReleaseBeforeRead checks rule 1 of the driver: the header scanner is
// released before the driver reads fields and payloads, so a cursor whose
// retention is bounded to 1 KiB lexes a stream of frames.
func TestReleaseBeforeRead(t *testing.T) {
	g := &generator{}
	gen := streamtest.NewGenerator(1, g.record, streamtest.WithBudget(64<<10))
	ls := streamtest.LexRetained(t, bytesource.NewReader(gen), 1<<10, func(cur bytescan.Cursor) *driver {
		return newDriver(cur, bytescan.WithMaxTokenBytes(tokenGuard))
	})
	n := int64(0)
	for _, l := range ls {
		if l.Type() == frameHeader {
			n++
		}
	}
	if n != g.count {
		t.Fatalf("lexed %d frame headers, want %d", n, g.count)
	}
}
