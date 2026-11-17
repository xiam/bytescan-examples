package main

import (
	"io"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/bytesource"
	"github.com/xiam/bytescan/modes"
	"github.com/xiam/bytescan/rules/octet"
)

// The token types. The header scanner emits FILE_HEADER; the driver reads the
// other parts of the file.
const (
	fileHeader bytescan.TokenType = "FILE_HEADER"
	dibHeader  bytescan.TokenType = "DIB_HEADER"
	colorTable bytescan.TokenType = "COLOR_TABLE"
	gap        bytescan.TokenType = "GAP"
	pixelData  bytescan.TokenType = "PIXEL_DATA"
)

// newScanner returns a BMP scanner that reads r: a driver over one byte cursor,
// with a mode driver that holds the file-header scanner, and a queue of the
// parts that the headers announce.
func newScanner(r io.Reader) *driver {
	return newDriver(bytesource.NewCursor(r))
}

// newDriver returns the driver over cur.
func newDriver(cur bytescan.Cursor) *driver {
	return &driver{
		cur:    cur,
		modes:  modes.New(cur, newHeaderScanner(cur)),
		fields: bytesource.NewFields(cur),
	}
}

// newHeaderScanner returns the scanner for the 14-byte BITMAPFILEHEADER over cur:
// the signature "BM" and 12 fixed bytes (file size, two reserved words, and
// the offset of the pixel data):
//
//	BM[\x00-\xff]{12}
//
// It has one rule, so no tie arises. A header cut by the end of the input does
// not match; its bytes are UNKNOWN.
func newHeaderScanner(cur bytescan.Cursor) *bytescan.Scanner {
	lx := bytescan.NewWithCursor(cur)
	lx.MustAddRule(fileHeader, octet.Record([]byte("BM"), 12))
	return lx
}
