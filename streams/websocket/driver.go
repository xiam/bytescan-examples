package main

import (
	"encoding/binary"
	"sync"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan-examples/internal/streamkit"
	"github.com/xiam/bytescan/bytesource"
)

// driver lexes a stream of WebSocket frames with one scanner and a queue of
// fields over one cursor:
//
//   - At a frame start, the header scanner lexes the 2-byte header as
//     FRAME_HEADER. The driver reads its 7-bit length and its MASK bit and
//     queues the fields they announce: an EXTENDED_LENGTH of 2 bytes for the
//     length 126 or of 8 bytes for 127, then a MASKING_KEY of 4 bytes when
//     MASK is set. Any other 7-bit length is the payload length itself.
//   - The driver reads the queued fields from the cursor. The bytes of an
//     EXTENDED_LENGTH, big-endian, are the payload length.
//   - Then it reads the payload from the cursor as PAYLOAD tokens of at most
//     maxPiece bytes, still masked, and is at a frame start again. A payload
//     length of 0 gives no PAYLOAD. The payload is a chunked field of the
//     queue, or several when its length does not fit an int (see
//     streamkit.PushLong).
//
// The driver uses the lengths only to frame the stream: it does not unmask the
// payload with the masking key, and it does not check the opcode, the
// reserved bits or that a length is written in its shortest form. The payload
// length is a uint64, so a 64-bit length of any value, the most significant
// bit included, frames a payload that runs to the end of the input.
//
// It has one scanner, so it has no mode to switch and does not use a
// modes.Driver. It follows the three rules of a mode driver:
//
//  1. It calls Release on the header scanner after every token, so the scanner's
//     checkpoints do not pin the fields and the payload it reads from the
//     cursor.
//  2. It builds its scanner once, in newDriver, and never drops it.
//  3. Next holds a mutex, so the scanner and a field read never run at the same
//     time.
//
// The scanner returns a plain io.EOF at the end of the input. When the input
// ends inside a field or a payload, the last token holds the bytes up to the
// end, and Next returns io.EOF after it.
type driver struct {
	mu     sync.Mutex
	header *bytescan.Scanner
	fields *bytesource.Fields

	length uint64 // the payload length of the current frame
	queue  func() // queues the payload of length bytes after the last field
}

// newDriver returns the driver over cur. The options, such as a per-token
// bound, apply to the header scanner.
func newDriver(cur bytescan.Cursor, opts ...bytescan.Option) *driver {
	d := &driver{
		header: newHeaderScanner(cur, opts...),
		fields: bytesource.NewFields(cur),
	}
	d.queue = func() {
		streamkit.PushLong(d.fields, payload, d.length, maxPiece, nil)
	}
	return d
}

// Next returns the next queued field or payload piece, or else the next
// frame header.
func (d *driver) Next() (*bytescan.Token, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if l, err := d.fields.NextField(); l != nil || err != nil {
		return l, err
	}
	return d.nextHeader()
}

// nextHeader lexes a frame header and queues the fields it announces.
func (d *driver) nextHeader() (*bytescan.Token, error) {
	l, err := d.header.Next()
	if err != nil {
		return nil, err
	}
	d.header.Release()
	if l.Type() != frameHeader {
		// A header cut by the end of the input: its byte is UNKNOWN.
		return l, nil
	}
	b := bytesource.Bytes(l.Text())[1]
	d.length = uint64(b & lengthBits)
	switch d.length {
	case length16:
		d.fields.Push(bytesource.Field{Type: extendedLength, Len: 2, Then: func(b []byte) {
			d.length = uint64(binary.BigEndian.Uint16(b))
		}})
	case length64:
		d.fields.Push(bytesource.Field{Type: extendedLength, Len: 8, Then: func(b []byte) {
			d.length = binary.BigEndian.Uint64(b)
		}})
	}
	if b&maskBit != 0 {
		d.fields.Push(bytesource.Field{Type: maskingKey, Len: keySize})
	}
	// The payload starts after the last field, when the extended length is
	// known. When the input ends inside a field, the queue is dropped and
	// this is never called, so the driver stays at a frame start and reaches
	// the end through its scanner.
	d.fields.Push(bytesource.Field{Done: d.queue})
	return l, nil
}
