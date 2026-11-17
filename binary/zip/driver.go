package main

import (
	"encoding/binary"
	"math"
	"sync"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/bytesource"
	"github.com/xiam/bytescan/modes"
)

// driver lexes a ZIP archive with one scanner, in a mode driver, and a queue of
// fields over one cursor. The record scanner lexes the fixed part of a record.
// After a record the driver reads the lengths of the fields that follow it
// from the record (little-endian, as in every ZIP field) and reads those
// fields from the cursor, one token per field, and FILE_DATA in pieces of at
// most maxPiece bytes. Then it goes back to the record scanner.
//
// The driver follows the three rules of a mode driver:
//
//  1. Before it reads fields from the cursor it calls Release on the record
//     scanner, so the scanner's checkpoints do not pin the file data.
//  2. It has one scanner and never builds another one.
//  3. Next holds a mutex, so the scanner and the field reads never run at the
//     same time.
//
// The scanner returns a plain io.EOF at the end of the input. When the input
// ends inside a field, the field token holds the bytes up to the end, and Next
// returns io.EOF after it.
type driver struct {
	mu     sync.Mutex
	modes  *modes.Driver
	fields *bytesource.Fields
}

// Next returns the next queued field, or else the next token of the record
// scanner, and queues the fields that a record announces.
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
	b := bytesource.Bytes(l.Text())
	switch l.Type() {
	case localFileHeader:
		// APPNOTE 4.3.7: compressed size at 18, name length at 26, extra
		// field length at 28.
		d.queue(
			bytesource.Field{Type: fileName, Len: u16(b[26:])},
			bytesource.Field{Type: extraField, Len: u16(b[28:])},
			bytesource.Field{Type: fileData, Len: u32(b[18:]), MaxPiece: maxPiece},
		)
	case centralDirectoryHeader:
		// APPNOTE 4.3.12: name length at 28, extra field length at 30,
		// comment length at 32.
		d.queue(
			bytesource.Field{Type: fileName, Len: u16(b[28:])},
			bytesource.Field{Type: extraField, Len: u16(b[30:])},
			bytesource.Field{Type: fileComment, Len: u16(b[32:])},
		)
	case endOfCentralDirectory:
		// APPNOTE 4.3.16: comment length at 20.
		d.queue(bytesource.Field{Type: zipComment, Len: u16(b[20:])})
	}
	return l, nil
}

// maxPiece is the largest FILE_DATA token, in bytes: the file data is read in
// pieces, so memory follows the piece size and not the size of the file.
const maxPiece = 64 << 10

// queue releases the record scanner and queues fields to read from the cursor.
func (d *driver) queue(fields ...bytesource.Field) {
	d.modes.Active().Release()
	d.fields.Push(fields...)
}

func u16(b []byte) int { return int(binary.LittleEndian.Uint16(b)) }

// u32 returns a 4-byte length, at most math.MaxInt32 so that it fits an int
// on every platform. The ZIP64 size 0xFFFFFFFF is not replaced by the size in
// the ZIP64 extra field, so such a field runs to the end of the input, in
// pieces of at most maxPiece bytes.
func u32(b []byte) int {
	return int(min(binary.LittleEndian.Uint32(b), math.MaxInt32))
}
