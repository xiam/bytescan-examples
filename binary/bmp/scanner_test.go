package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/xiam/bytescan-examples/internal/hexdump"
	"github.com/xiam/bytescan-examples/internal/lextest"
	"github.com/xiam/bytescan/streamtest"
)

func TestGolden(t *testing.T) {
	lextest.Golden(t, hexdump.Dumper, newScanner)
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
	for _, l := range streamtest.LexRetainedBytes(t, bytes.NewReader(file), 1<<10, newDriver) {
		if l.Type() != pixelData {
			continue
		}
		if l.Offset() != 54+sum {
			assert.Failf(t, "", "piece %d at offset %d, want %d", pieces, l.Offset(), 54+sum)
		}
		if l.Len() > maxPiece {
			assert.Failf(t, "", "piece %d has %d bytes, more than %d", pieces, l.Len(), maxPiece)
		}
		pieces++
		sum += l.Len()
	}
	assert.Equalf(t, size, sum, "PIXEL_DATA pieces hold %d bytes, want %d", sum, size)
	{
		want := (size + maxPiece - 1) / maxPiece
		assert.Equalf(t, want, pieces, "%d PIXEL_DATA pieces, want %d", pieces, want)
	}
}
