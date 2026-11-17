package main

import (
	"sync"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/bytesource"
	"github.com/xiam/bytescan/modes"
)

// driver lexes a stream of MQTT control packets with three scanners over one
// byte cursor. Its modes:
//
//   - Header: the header scanner lexes one byte as FIXED_HEADER.
//   - Length: the length scanner lexes the remaining length. A REMAINING_LENGTH
//     of 1 to 4 bytes gives the number of bytes of the packet after it. For
//     PUBLISH the driver peeks at the 2-byte topic length and frames
//     VARIABLE_HEADER (the topic length, the topic and, when the QoS bits of
//     the fixed header are not 0, the 2-byte packet identifier) and PAYLOAD
//     (the rest). For every other packet type the rest is BODY. A packet with
//     a remaining length of 0 has neither. When the length is invalid, the
//     first length byte is UNKNOWN and the driver goes to the bad length.
//   - Bad length: the bad-length scanner lexes the rest of the invalid length
//     one byte at a time, as UNKNOWN, up to and including the first byte
//     without the high bit. Then a new packet starts.
//   - Fields: the driver reads VARIABLE_HEADER and then PAYLOAD or BODY from
//     the cursor through a bytesource.Fields queue, each in pieces of at most
//     maxPiece bytes, so memory follows the piece size and not the declared
//     length. Then a new packet starts.
//
// The driver uses the remaining length and the topic length only to frame the
// fields. It does not parse the topic, the payload or any other field, and it
// does not judge the flags or the packet type. MQTT has no sync marker: after
// corrupt data the driver frames packets wherever the corrupt bytes put them,
// and it cannot find the next real packet.
//
// The scanners run on a modes.Driver. Together they follow the three rules of
// a mode driver:
//
//  1. The modes.Driver calls Release on the outgoing scanner at every switch,
//     and the driver switches from the length scanner to the header scanner,
//     which is idle, before it peeks at or reads the cursor itself.
//  2. It builds its three scanners once, in newDriver, and never builds another
//     one.
//  3. Next holds a mutex, so two scanners, or a scanner and a field read, never
//     run at the same time.
//
// Every scanner returns a plain io.EOF at the end of the input. When the input
// ends inside a packet, the driver lexes what is there and stops normally.
type driver struct {
	mu  sync.Mutex
	cur bytescan.Cursor

	modes                *modes.Driver
	header, length, rest *bytescan.Scanner // the header, length and bad-length scanners

	inFields bool               // the driver reads the fields of the current packet
	first    byte               // the fixed header byte of the current packet
	fields   *bytesource.Fields // VARIABLE_HEADER and PAYLOAD or BODY of the current packet
}

// newDriver returns the driver over cur, at the start of a packet. Every
// scanner is built with opts.
func newDriver(cur bytescan.Cursor, opts ...bytescan.Option) *driver {
	d := &driver{
		cur:    cur,
		header: newHeaderScanner(cur, opts...),
		length: newLengthScanner(cur, opts...),
		rest:   newBadLengthScanner(cur, opts...),
		fields: bytesource.NewFields(cur),
	}
	d.modes = modes.New(cur, d.header)
	return d
}

// Next returns the next token of the stream.
func (d *driver) Next() (*bytescan.Token, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.lex()
}

// lex is Next without the mutex.
func (d *driver) lex() (*bytescan.Token, error) {
	if d.inFields {
		return d.readField()
	}
	l, err := d.modes.Next()
	if err != nil {
		return nil, err
	}
	switch l.Type() {
	case fixedHeader:
		d.first = bytesource.Bytes(l.Text())[0]
		d.modes.Use(d.length)
	case remainingLength:
		d.modes.Use(d.header)
		if err := d.frame(lengthValue(bytesource.Bytes(l.Text()))); err != nil {
			return nil, err
		}
	case continuation:
		d.modes.Use(d.rest)
		l = l.WithType(unknown)
	case lengthEnd:
		d.modes.Use(d.header)
		l = l.WithType(unknown)
	}
	return l, nil
}

// lengthValue returns the value of a remaining length of 1 to 4 bytes: 7 bits
// per byte, least significant group first (section 2.2.3).
func lengthValue(b []byte) int64 {
	var n int64
	for i, c := range b {
		n |= int64(c&0x7F) << (7 * i)
	}
	return n
}

// frame sets up the fields of a packet whose remaining length is n. The
// length scanner has been released.
func (d *driver) frame(n int64) error {
	if n == 0 {
		return nil
	}
	d.inFields = true
	if d.first>>4 != publish {
		d.fields.Push(bytesource.Field{Type: body, Len: int(n), MaxPiece: maxPiece})
		return nil
	}
	// PUBLISH (section 3.3.2): a topic name of 2 length bytes and that many
	// bytes, then a packet identifier of 2 bytes when QoS is not 0. The
	// variable header ends at the packet end when the packet is shorter.
	vh := n
	b, err := bytesource.Peek(d.cur, 2)
	if err != nil {
		return err
	}
	if len(b) == 2 {
		vh = 2 + (int64(b[0])<<8 | int64(b[1]))
		if d.first>>1&0x03 != 0 {
			vh += 2
		}
	}
	vh = min(vh, n)
	d.fields.Push(
		bytesource.Field{Type: variableHeader, Len: int(vh), MaxPiece: maxPiece},
		bytesource.Field{Type: payload, Len: int(n - vh), MaxPiece: maxPiece},
	)
	return nil
}

// readField reads the next piece of VARIABLE_HEADER, PAYLOAD or BODY from the
// cursor. When the queue is empty, because the packet was read or because the
// input ended inside it, a new packet starts.
func (d *driver) readField() (*bytescan.Token, error) {
	l, err := d.fields.NextField()
	if err != nil {
		return nil, err
	}
	if l == nil {
		d.inFields = false
		return d.lex()
	}
	return l, nil
}
