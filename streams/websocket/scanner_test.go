package main

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/xiam/bytescan-examples/internal/guardtest"
	"github.com/xiam/bytescan-examples/internal/hexdump"
	"github.com/xiam/bytescan-examples/internal/lexdump"
	"github.com/xiam/bytescan/streamtest"
)

func TestGolden(t *testing.T) {
	lexdump.Golden(t, hexdump.Dumper, newScanner)
}

// TestGuard checks the production guard: it is streamtest.ByteGuard;
// a frame whose payload never ends, a maximal 64-bit length followed by
// 256 KiB of any bytes, keeps producing PAYLOAD pieces of at most maxPiece
// bytes, with no guard error and bounded memory; and a candidate longer than
// the guard fails Next with the guard error.
func TestGuard(t *testing.T) {
	if tokenGuard != streamtest.ByteGuard {
		t.Fatalf("tokenGuard = %d, want streamtest.ByteGuard (%d)", tokenGuard, streamtest.ByteGuard)
	}
	head := binary.BigEndian.AppendUint64([]byte{0x82, length64}, 1<<64-1)
	gen := streamtest.NewGenerator(80, guardtest.Filler(), streamtest.WithBudget(4*maxPiece))
	src := streamtest.Prefix(gen, head)
	guardtest.Bounded(t, newScanner, src, maxPiece, payload,
		streamtest.WithCeiling(streamtest.DefaultCeiling(src.Budget())+pieceHeap))

	// tokenGuard+1 bytes of 0xFF exceed the guard, which counts bytes, by one.
	d := newScanner(bytes.NewReader(bytes.Repeat([]byte{0xFF}, tokenGuard+1)))
	guardtest.Active(t, d.header, tokenGuard, d.Next)
}
