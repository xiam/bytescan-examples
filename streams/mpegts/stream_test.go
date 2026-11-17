package main

import (
	"bytes"
	"math/rand"
	"testing"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/streamtest"
)

// record returns a streamtest.RecordFunc that appends one 188-byte packet.
// Packets are of four kinds, on three PIDs: payload-only packets of a video
// and an audio stream, packets with an adaptation field (a PCR or one byte of
// flags, then stuffing) and a payload, adaptation-only packets, and null
// packets of 0xFF stuffing. The continuity counter is the low four bits of
// the record number.
//
// With clean set, no byte of a packet other than its sync byte is 0x47, so
// the only 0x47 bytes in the stream are sync bytes; without it, payload bytes
// take every value, 0x47 included.
func record(clean bool) streamtest.RecordFunc {
	randByte := func(rng *rand.Rand) byte {
		for {
			c := byte(rng.Intn(256))
			if !clean || c != syncByte {
				return c
			}
		}
	}
	return func(rng *rand.Rand, i int, dst []byte) []byte {
		counter := byte(i)
		pid := []int{pidVideo, pidAudio}[rng.Intn(2)]
		start := len(dst)
		switch kind := rng.Intn(8); {
		case kind < 4: // payload only
			dst = append(dst, header(pid, rng.Intn(4) == 0, payloadOnly, counter)...)
		case kind < 6: // adaptation field and payload
			length := rng.Intn(bodySize - 1)
			for clean && length == syncByte {
				length = rng.Intn(bodySize - 1)
			}
			dst = append(dst, header(pid, false, both, counter)...)
			dst = appendAdaptation(rng, dst, length, randByte)
		case kind < 7: // adaptation only
			dst = append(dst, header(pid, false, adaptationOnly, counter)...)
			dst = appendAdaptation(rng, dst, bodySize-1, randByte)
		default: // null packet
			dst = append(dst, header(pidNull, false, payloadOnly, counter)...)
			return append(dst, bytes.Repeat([]byte{0xFF}, packetSize-(len(dst)-start))...)
		}
		for len(dst)-start < packetSize {
			dst = append(dst, randByte(rng))
		}
		return dst
	}
}

// appendAdaptation appends an adaptation field of the given length: the
// length byte, then (for a length of at least 1) a flags byte, a 6-byte PCR
// when it fits, and 0xFF stuffing.
func appendAdaptation(rng *rand.Rand, dst []byte, length int, randByte func(*rand.Rand) byte) []byte {
	dst = append(dst, byte(length))
	end := len(dst) + length
	switch {
	case length >= 7:
		dst = append(dst, 0x10) // PCR_flag
		for i := 0; i < 6; i++ {
			dst = append(dst, randByte(rng))
		}
	case length >= 1:
		dst = append(dst, 0x00)
	}
	for len(dst) < end {
		dst = append(dst, 0xFF)
	}
	return dst
}

// TestLongRun lexes a generated stream of packets: every packet gives a SYNC
// at a multiple of 188 bytes, and no byte is UNKNOWN, although payload bytes
// take the value 0x47.
func TestLongRun(t *testing.T) {
	gen := streamtest.NewGenerator(54, record(false))
	res := streamtest.Run(t, newScanner, gen, streamtest.WithTokenFunc(func(l *bytescan.Token) {
		if l.Type() == syncType && l.Offset()%packetSize != 0 {
			t.Fatalf("SYNC at offset %d, not at a packet boundary", l.Offset())
		}
	}))
	packets := int64(gen.Records())
	for _, typ := range []bytescan.TokenType{syncType, headerType} {
		if got := res.Counts[typ]; got != packets {
			t.Errorf("%s tokens = %d, want one per packet, %d", typ, got, packets)
		}
	}
	if n := res.Counts[bytescan.TokenTypeUnknown]; n != 0 {
		t.Errorf("UNKNOWN tokens = %d, want 0", n)
	}
	if res.Counts[adaptationField] == 0 || res.Counts[payload] == 0 {
		t.Errorf("counts = %v, want ADAPTATION_FIELD and PAYLOAD tokens", res.Counts)
	}
}

// garbageAlphabet is every byte but 0x47, so a garbage run holds no sync
// byte.
var garbageAlphabet = func() string {
	var b []byte
	for c := 0; c < 256; c++ {
		if c != syncByte {
			b = append(b, byte(c))
		}
	}
	return string(b)
}()

// segment is a run of the corrupted stream that the corruption test expects
// one kind of tokens for.
type segment struct {
	start, end int64
	garbage    bool // a garbage run
	lost       bool // a packet the driver cannot confirm: all UNKNOWN
}

// TestCorruption inserts garbage runs of 1 to 400 bytes (no 0x47) between
// packets whose only 0x47 bytes are sync bytes, and checks every token of the
// stream against what the resynchronisation rule predicts:
//
//   - every garbage byte is one UNKNOWN token;
//   - a packet that follows garbage is lexed as a packet when the packet after
//     it starts right after it, because its sync byte is then confirmed; it is
//     all UNKNOWN when garbage or the end of the stream follows it;
//   - every other packet is lexed as a packet: SYNC at its first byte, then
//     HEADER, ADAPTATION_FIELD and PAYLOAD inside it.
//
// The tokens cover the stream without a gap, and a token never crosses from
// one segment into the next.
func TestCorruption(t *testing.T) {
	gen := streamtest.NewGenerator(54, record(true))
	src := streamtest.Corrupt(gen, 7, 0.1, streamtest.Garbage(garbageAlphabet, 1, 400))

	var (
		pending                       []streamtest.Injection // injections not yet laid out
		seg                           segment                // the current segment
		rec                           int                    // the record of the next packet segment
		afterGarbage                  bool                   // the next packet follows garbage
		pos                           int64                  // the offset of the next token
		packets, lostPackets, garbage int64
	)
	drain := func() {
		for {
			in, ok := src.NextInjection()
			if !ok {
				return
			}
			pending = append(pending, in)
		}
	}
	// layout makes the segment after seg the current one.
	layout := func() {
		drain()
		start := seg.end
		if !afterGarbage && len(pending) > 0 && pending[0].Record == rec {
			in := pending[0]
			pending = pending[1:]
			if in.Offset != start {
				t.Fatalf("garbage at %d, want it at %d, before record %d", in.Offset, start, rec)
			}
			seg = segment{start: start, end: start + int64(in.Len), garbage: true}
			afterGarbage = true
			garbage++
			return
		}
		// The driver has peeked one packet past a packet that follows
		// garbage, so the generator has made the next record by now, or
		// has ended.
		nextGarbage := len(pending) > 0 && pending[0].Record == rec+1
		last := gen.Records() == rec+1
		seg = segment{start: start, end: start + packetSize, lost: afterGarbage && (nextGarbage || last)}
		afterGarbage = false
		rec++
		packets++
		if seg.lost {
			lostPackets++
		}
	}
	res := streamtest.Run(t, newScanner, src, streamtest.WithTokenFunc(func(l *bytescan.Token) {
		off, end := int64(l.Offset()), int64(l.Offset()+l.Len())
		if off != pos {
			t.Fatalf("%s at %d, want the next token at %d", l.Type(), off, pos)
		}
		pos = end
		for off >= seg.end {
			layout()
		}
		if end > seg.end {
			t.Fatalf("%s at %d-%d crosses the segment end %d", l.Type(), off, end, seg.end)
		}
		switch {
		case seg.garbage || seg.lost:
			if l.Type() != bytescan.TokenTypeUnknown || l.Len() != 1 {
				t.Fatalf("%s of %d bytes at %d, want one UNKNOWN byte (segment %+v)", l.Type(), l.Len(), off, seg)
			}
		case off == seg.start:
			if l.Type() != syncType {
				t.Fatalf("%s at %d, want SYNC at the packet start", l.Type(), off)
			}
		default:
			if l.Type() != headerType && l.Type() != adaptationField && l.Type() != payload {
				t.Fatalf("%s at %d, inside packet %d-%d", l.Type(), off, seg.start, seg.end)
			}
		}
	}))
	if pos != res.Bytes {
		t.Errorf("tokens end at %d, want the end of the stream at %d", pos, res.Bytes)
	}
	if got, want := res.Counts[syncType], packets-lostPackets; got != want {
		t.Errorf("SYNC tokens = %d, want %d", got, want)
	}
	if garbage == 0 || lostPackets == 0 {
		t.Errorf("%d garbage runs and %d unconfirmed packets, want some of each", garbage, lostPackets)
	}
	t.Logf("%d packets, %d garbage runs, %d packets unconfirmed (garbage on both sides)", packets, garbage, lostPackets)
}

// TestReleaseBeforeRead checks rule 1 of the driver: the scanners are released
// before the driver peeks or reads fields, so a cursor whose retention is
// bounded to 1 KiB lexes a corrupted stream.
func TestReleaseBeforeRead(t *testing.T) {
	gen := streamtest.NewGenerator(1, record(false), streamtest.WithBudget(64<<10))
	src := streamtest.Corrupt(gen, 2, 0.2, streamtest.Garbage(garbageAlphabet, 1, 400))
	ls := streamtest.LexRetainedBytes(t, src, 1<<10, func(cur bytescan.Cursor) *driver {
		return newDriver(cur, bytescan.WithMaxTokenBytes(tokenGuard))
	})
	if len(ls) == 0 {
		t.Fatal("no tokens")
	}
}
