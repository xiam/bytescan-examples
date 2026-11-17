package main

import (
	"encoding/binary"
	"testing"

	"github.com/xiam/bytescan-examples/internal/lexdump"
)

// The binary test inputs are written byte by byte here, so the
// testdata/*.input files are reproducible: TestInputs checks that each
// committed input equals the bytes built below, and go test -update rewrites
// the inputs before TestGolden rewrites the golden files.

// inputs maps the name of each test case to the bytes of its input.
var inputs = map[string][]byte{
	"document":    document(),
	"rgb24":       rgb24(),
	"core":        coreHeader(),
	"truncated":   document()[:30],
	"bad-offset":  badOffset(),
	"huge-colors": hugeColors(),
	"not-bmp":     []byte("BA\x00\x00 bm\x00\x00 BM\x01\x02\x03"),
}

func TestInputs(t *testing.T) {
	lexdump.Inputs(t, inputs)
}

var (
	le16 = binary.LittleEndian.AppendUint16
	le32 = binary.LittleEndian.AppendUint32
)

// buildFileHeader returns a BITMAPFILEHEADER for a file of size bytes whose pixel
// data starts at offset.
func buildFileHeader(size, offset int) []byte {
	b := []byte("BM")
	b = le32(b, uint32(size))
	b = le32(b, 0) // two reserved words
	return le32(b, uint32(offset))
}

// infoHeader returns a 40-byte BITMAPINFOHEADER, uncompressed (BI_RGB).
func infoHeader(width, height, bits, used int) []byte {
	b := le32(nil, 40)
	b = le32(b, uint32(width))
	b = le32(b, uint32(height))
	b = le16(b, 1) // planes
	b = le16(b, uint16(bits))
	b = le32(b, 0) // BI_RGB
	b = le32(b, 0) // image size: may be 0 for BI_RGB
	b = le32(b, 2835)
	b = le32(b, 2835) // 72 DPI
	b = le32(b, uint32(used))
	return le32(b, 0) // important colours
}

// document is a 4×2 image with 8 bits per pixel and a colour table of four
// colours (biClrUsed 4), followed by a 2-byte gap before the pixel data.
func document() []byte {
	table := []byte{
		0x00, 0x00, 0x00, 0x00, // black (blue, green, red, reserved)
		0xFF, 0xFF, 0xFF, 0x00, // white
		0x00, 0x00, 0xFF, 0x00, // red
		0xFF, 0x00, 0x00, 0x00, // blue
	}
	pixels := []byte{0, 1, 2, 3, 3, 2, 1, 0} // two rows of four indexes
	offset := 14 + 40 + len(table) + 2
	b := buildFileHeader(offset+len(pixels), offset)
	b = append(b, infoHeader(4, 2, 8, 4)...)
	b = append(b, table...)
	b = append(b, 0xAA, 0xBB) // gap
	return append(b, pixels...)
}

// rgb24 is a 2×2 image with 24 bits per pixel: no colour table, and each row
// padded to 8 bytes.
func rgb24() []byte {
	pixels := []byte{
		0x00, 0x00, 0xFF, 0xFF, 0x00, 0x00, 0x00, 0x00, // red, blue, padding
		0x0A, 0x0A, 0x0A, 0xFF, 0xFF, 0xFF, 0x00, 0x00, // grey, white, padding
	}
	b := buildFileHeader(54+len(pixels), 54)
	b = append(b, infoHeader(2, 2, 24, 0)...)
	return append(b, pixels...)
}

// coreHeader is a 1-bit image with the 12-byte BITMAPCOREHEADER of OS/2 and a
// colour table of two 3-byte entries. The driver reads colour tables only for
// DIB headers of 40 bytes or more, so this table is lexed as GAP.
func coreHeader() []byte {
	dib := le32(nil, 12)
	dib = le16(dib, 8) // width
	dib = le16(dib, 1) // height
	dib = le16(dib, 1) // planes
	dib = le16(dib, 1) // bits per pixel
	table := []byte{0x00, 0x00, 0x00, 0xFF, 0xFF, 0xFF}
	pixels := []byte{0xA5, 0x00, 0x00, 0x00}
	offset := 14 + len(dib) + len(table)
	b := buildFileHeader(offset+len(pixels), offset)
	b = append(b, dib...)
	b = append(b, table...)
	return append(b, pixels...)
}

// badOffset is the rgb24 image with a pixel offset of 20, inside the DIB
// header.
func badOffset() []byte {
	b := rgb24()
	binary.LittleEndian.PutUint32(b[10:], 20)
	return b
}

// hugeColors is the document image with a biClrUsed of 0xFFFFFFFF: the
// colour table is cut at the pixel offset, on every platform.
func hugeColors() []byte {
	b := document()
	binary.LittleEndian.PutUint32(b[14+32:], 0xFFFFFFFF)
	return b
}
