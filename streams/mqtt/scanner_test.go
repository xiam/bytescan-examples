package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan-examples/internal/guardtest"
	"github.com/xiam/bytescan-examples/internal/hexdump"
	"github.com/xiam/bytescan-examples/internal/lextest"
	"github.com/xiam/bytescan/streamtest"
)

func TestGolden(t *testing.T) {
	lextest.Golden(t, hexdump.Dumper, newScanner)
}

// TestGuard checks the production guard: it is streamtest.ByteGuard; a
// packet that never ends, a CONNECT with the maximal remaining length
// followed by 256 KiB of any bytes, keeps producing BODY pieces of at most
// maxPiece bytes, with no guard error and bounded memory; and a candidate
// longer than the guard fails Next with the guard error.
func TestGuard(t *testing.T) {
	require.Equalf(t, streamtest.ByteGuard, tokenGuard, "tokenGuard = %d, want streamtest.ByteGuard (%d)", tokenGuard, streamtest.ByteGuard)
	gen := streamtest.NewGenerator(80, guardtest.Filler(), streamtest.WithBudget(4*maxPiece))
	src := streamtest.Prefix(gen, []byte{connect, 0xFF, 0xFF, 0xFF, 0x7F})
	guardtest.Bounded(t, newScanner, src, maxPiece, body,
		streamtest.WithCeiling(streamtest.DefaultCeiling(src.Budget())+4*maxPiece))

	d := newScanner(bytes.NewReader(bytes.Repeat([]byte{'a'}, 2*tokenGuard)))
	guardtest.Active(t, d.header, tokenGuard, d.Next)
}

// bigPublish is a PUBLISH at QoS 1 with a topic of 65,535 bytes and a
// payload of 70,000 bytes, then a PINGREQ. Its variable header is 65,539
// bytes and its remaining length 135,539, three bytes long.
func bigPublish() []byte {
	topic := string(bytes.Repeat([]byte("t"), 0xFFFF))
	b := packet(publishHeader(false, 1, false), 0, str(topic), u16(7), bytes.Repeat([]byte{0xA5}, 70000))
	return append(b, packet(pingreq, 1)...)
}

// TestPieces checks that VARIABLE_HEADER and PAYLOAD, framed by declared
// lengths, are read in pieces of at most maxPiece bytes, and that the next
// packet starts after them.
func TestPieces(t *testing.T) {
	type piece struct {
		typ bytescan.TokenType
		n   int
	}
	var got []piece
	for _, l := range streamtest.LexAll(t, newScanner(bytes.NewReader(bigPublish()))) {
		got = append(got, piece{l.Type(), l.Len()})
	}
	want := []piece{
		{fixedHeader, 1}, {remainingLength, 3},
		{variableHeader, maxPiece}, {variableHeader, 65539 - maxPiece},
		{payload, maxPiece}, {payload, 70000 - maxPiece},
		{fixedHeader, 1}, {remainingLength, 1},
	}
	require.Equalf(t, len(want), len(got), "tokens = %v, want %v", got, want)
	for i := range want {
		require.Equalf(t, want[i], got[i], "tokens = %v, want %v", got, want)
	}
}

// TestLengthValue checks the value of a remaining length against the
// examples of section 2.2.3, table 2.4.
func TestLengthValue(t *testing.T) {
	cases := []struct {
		b []byte
		n int64
	}{
		{[]byte{0x00}, 0},
		{[]byte{0x7F}, 127},
		{[]byte{0x80, 0x01}, 128},
		{[]byte{0xFF, 0x7F}, 16383},
		{[]byte{0x80, 0x80, 0x01}, 16384},
		{[]byte{0xFF, 0xFF, 0x7F}, 2097151},
		{[]byte{0x80, 0x80, 0x80, 0x01}, 2097152},
		{[]byte{0xFF, 0xFF, 0xFF, 0x7F}, 268435455},
	}
	for _, tc := range cases {
		{
			got := lengthValue(tc.b)
			assert.Equalf(t, tc.n, got, "lengthValue(% x) = %d, want %d", tc.b, got, tc.n)
		}
		{
			got := encodeLength(int(tc.n), 0)
			assert.Truef(t, bytes.Equal(got, tc.b), "encodeLength(%d) = % x, want % x", tc.n, got, tc.b)
		}
	}
}

// TestReleaseBeforeRead checks rule 1 of the driver: the scanners are released
// before the driver peeks or reads fields, so a cursor whose retention is
// bounded to 1 KiB lexes a variable header and a payload of more than 64 KiB.
// Without the Release, the checkpoint of the length scanner would pin them and
// the cursor would fail.
func TestReleaseBeforeRead(t *testing.T) {
	ls := streamtest.LexRetainedBytes(t, bytes.NewReader(bigPublish()), 1<<10, func(cur bytescan.Cursor) *driver {
		return newDriver(cur, bytescan.WithMaxTokenBytes(tokenGuard))
	})
	{
		n := len(ls)
		assert.Equalf(t, 8, n, "lexed %d tokens, want 8", n)
	}
}
