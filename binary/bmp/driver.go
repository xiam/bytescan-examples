package main

import (
	"encoding/binary"
	"math"
	"sync"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/bytesource"
	"github.com/xiam/bytescan/modes"
)

// driver lexes a BMP file with one scanner, in a mode driver, and a queue of
// fields over one cursor. The header scanner lexes the file header. The driver
// takes the offset of the pixel data from it, and reads the rest of the file
// from the cursor:
//
//   - DIB_HEADER: the driver peeks at its first four bytes, which give its
//     size, and reads that many bytes.
//   - COLOR_TABLE: for a DIB header of 40 bytes or more, biClrUsed times 4
//     bytes; when biClrUsed is 0 and biBitCount is 1 to 8, 2 to the power of
//     biBitCount times 4 bytes. Other DIB headers have no colour table.
//   - GAP: every byte from the end of the colour table to the pixel offset.
//   - PIXEL_DATA: every byte from the pixel offset to the end of the input,
//     in pieces of at most 64 KiB, so memory follows the piece size and not
//     the size of the image.
//
// Only PIXEL_DATA reaches past the pixel offset. If the pixel offset lies
// inside the headers, the colour table and the gap are empty and the pixel
// data starts after the DIB header. All numbers are little-endian. A part of
// length 0 gives no token.
//
// The driver follows the three rules of a mode driver:
//
//  1. Before it reads from the cursor it calls Release on the header scanner,
//     so the scanner's checkpoints do not pin the pixel data.
//  2. It has one scanner and never builds another one.
//  3. Next holds a mutex, so the scanner and the field reads never run at the
//     same time.
//
// The scanner returns a plain io.EOF at the end of the input. When the input
// ends inside a part, the part holds the bytes up to the end, and Next
// returns io.EOF after it.
type driver struct {
	mu     sync.Mutex
	cur    bytescan.Cursor
	modes  *modes.Driver
	fields *bytesource.Fields

	pixelOffset int // from the file header
	pos         int // the offset after the last part queued
}

// Next returns the next queued part, or else the next token of the header
// scanner, and queues the parts after a file header.
func (d *driver) Next() (*bytescan.Token, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if l, err := d.fields.NextField(); l != nil || err != nil {
		return l, err
	}
	l, err := d.modes.Next()
	if err != nil {
		return nil, err
	}
	if l.Type() == fileHeader {
		d.modes.Active().Release()
		d.pixelOffset = u32(bytesource.Bytes(l.Text())[10:])
		d.pos = l.Offset() + l.Len()

		// The DIB header starts with its own size. A size below 4 is read as
		// 4, the size field alone.
		b, err := bytesource.Peek(d.cur, 4)
		if err != nil {
			return nil, err
		}
		size := 4
		if len(b) == 4 {
			size = max(u32(b), 4)
		}
		d.fields.Push(bytesource.Field{Type: dibHeader, Len: size, Then: d.afterDIBHeader})
	}
	return l, nil
}

// afterDIBHeader queues the colour table, the gap and the pixel data after
// the DIB header b.
func (d *driver) afterDIBHeader(b []byte) {
	d.pos += len(b)
	table := 0
	if len(b) >= 40 {
		// BITMAPINFOHEADER: biBitCount at 14, biClrUsed at 32.
		bits, used := int(binary.LittleEndian.Uint16(b[14:])), u32(b[32:])
		switch {
		case used > 0:
			// In int64, so that a huge biClrUsed does not wrap on a 32-bit
			// int; the table is cut at the pixel offset below.
			table = int(min(int64(used)*4, math.MaxInt32))
		case bits >= 1 && bits <= 8:
			table = (1 << bits) * 4
		}
	}
	table = max(min(table, d.pixelOffset-d.pos), 0)
	gapLen := max(d.pixelOffset-d.pos-table, 0)
	d.fields.Push(
		bytesource.Field{Type: colorTable, Len: table},
		bytesource.Field{Type: gap, Len: gapLen},
		// The pixel data has no stated length: a chunked field of the
		// largest length reads it to the end of the input, piece by piece.
		bytesource.Field{Type: pixelData, Len: math.MaxInt, MaxPiece: maxPiece},
	)
}

// maxPiece is the largest PIXEL_DATA token, in bytes.
const maxPiece = 64 << 10

// u32 returns a 4-byte number, at most math.MaxInt32 so that it fits an int
// on every platform.
func u32(b []byte) int {
	return int(min(binary.LittleEndian.Uint32(b), math.MaxInt32))
}
