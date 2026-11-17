package main

import (
	"bytes"
	"testing"

	"github.com/xiam/bytescan-examples/internal/hexdump"
	"github.com/xiam/bytescan-examples/internal/lexdump"
	"github.com/xiam/bytescan/bytesource"
	"github.com/xiam/bytescan/streamtest"
)

func TestGolden(t *testing.T) {
	lexdump.Golden(t, hexdump.Dumper, newScanner)
}

// TestPixelDataPieces checks that the pixel data, which runs to the end of the
// input, is read in pieces of at most maxPiece bytes that tile it, and that
// the header scanner is released before it: a cursor whose retention is bounded
// to 1 KiB lexes 200 KiB of pixel data.
func TestPixelDataPieces(t *testing.T) {
	const size = 200 << 10
	pixels := bytes.Repeat([]byte{0x5A}, size)
	file := append(buildFileHeader(54+size, 54), infoHeader(256, 200, 32, 0)...)
	file = append(file, pixels...)

	pieces, sum := 0, 0
	for _, l := range streamtest.LexRetained(t, bytesource.NewReader(bytes.NewReader(file)), 1<<10, newDriver) {
		if l.Type() != pixelData {
			continue
		}
		if l.Offset() != 54+sum {
			t.Errorf("piece %d at offset %d, want %d", pieces, l.Offset(), 54+sum)
		}
		if l.Len() > maxPiece {
			t.Errorf("piece %d has %d bytes, more than %d", pieces, l.Len(), maxPiece)
		}
		pieces++
		sum += l.Len()
	}
	if sum != size {
		t.Errorf("PIXEL_DATA pieces hold %d bytes, want %d", sum, size)
	}
	if want := (size + maxPiece - 1) / maxPiece; pieces != want {
		t.Errorf("%d PIXEL_DATA pieces, want %d", pieces, want)
	}
}
