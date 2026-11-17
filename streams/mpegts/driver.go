package main

import (
	"sync"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/bytesource"
	"github.com/xiam/bytescan/modes"
)

// driver lexes an MPEG transport stream with three scanners and a queue of
// fields over one cursor. Its modes:
//
//   - In sync, the sync scanner lexes the byte where a packet should start. A
//     0x47 is SYNC, and the driver switches to the header scanner. Any other
//     byte is UNKNOWN, and the driver has lost sync.
//   - In the header, the header scanner lexes the 3 header bytes as HEADER.
//     Bit 5 of the last one (adaptation_field_control 10 or 11) announces an
//     adaptation field. The driver peeks at its length byte and queues the
//     ADAPTATION_FIELD (the length byte and that many bytes, but at most the
//     184 bytes left in the packet) and then the PAYLOAD (the rest of the
//     packet, when any is left). It reads them from the cursor and goes back
//     in sync at the next packet. A header cut by the end of the input is
//     UNKNOWN, and the driver has lost sync.
//   - Lost, the lost scanner lexes a 0x47 as SYNC only when the byte 188 bytes
//     further is 0x47 too, and the driver switches to the header scanner. Any
//     other byte, and a 0x47 that is not confirmed, is UNKNOWN.
//
// The driver uses the adaptation field length only to frame the field; it
// does not parse the field or the payload.
//
// The scanners run on a modes.Driver. Together they follow the three rules of
// a mode driver:
//
//  1. The modes.Driver calls Release on the outgoing scanner at every switch,
//     and the driver switches away from the header scanner before it peeks at
//     or reads bytes from the cursor, so an idle scanner does not pin the
//     packets that follow.
//  2. It builds the three scanners once, in newDriver, and never drops one.
//  3. Next holds a mutex, so no two scanners on the cursor, and no scanner and a
//     field read, run at the same time.
//
// The scanners return a plain io.EOF at the end of the input. When the input
// ends inside a field, the field token holds the bytes up to the end, and
// Next returns io.EOF after it.
type driver struct {
	mu     sync.Mutex
	cur    bytescan.Cursor
	modes  *modes.Driver
	sync   *bytescan.Scanner
	header *bytescan.Scanner
	lost   *bytescan.Scanner
	fields *bytesource.Fields
}

// newDriver returns the driver over cur, in sync. Every scanner is built with
// opts.
func newDriver(cur bytescan.Cursor, opts ...bytescan.Option) *driver {
	d := &driver{
		cur:    cur,
		sync:   newSyncScanner(cur, opts...),
		header: newHeaderScanner(cur, opts...),
		lost:   newLostScanner(cur, opts...),
		fields: bytesource.NewFields(cur),
	}
	d.modes = modes.New(cur, d.sync)
	return d
}

// Next returns the next queued field, or else the next token of the mode.
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
	switch l.Type() {
	case syncType:
		d.modes.Use(d.header)
	case headerType:
		d.modes.Use(d.sync)
		if err := d.queue(l); err != nil {
			return nil, err
		}
	default:
		d.modes.Use(d.lost)
	}
	return l, nil
}

// queue queues the adaptation field and the payload that the header h
// announces. The header scanner has been released.
func (d *driver) queue(h *bytescan.Token) error {
	rest := bodySize
	// adaptation_field_control is bits 5 and 4 of the last header byte; bit 5
	// set (10 or 11) means an adaptation field follows the header.
	if bytesource.Bytes(h.Text())[2]&0x20 != 0 {
		b, err := bytesource.Peek(d.cur, 1)
		if err != nil || len(b) == 0 {
			return err
		}
		n := min(1+int(b[0]), rest)
		d.fields.Push(bytesource.Field{Type: adaptationField, Len: n})
		rest -= n
	}
	d.fields.Push(bytesource.Field{Type: payload, Len: rest})
	return nil
}
