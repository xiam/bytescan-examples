package main

import (
	"encoding/binary"
	"sync"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/bytesource"
	"github.com/xiam/bytescan/modes"
)

// driver lexes a JPEG file with three scanners, in a mode driver, and a queue
// of fields over one cursor. Its modes:
//
//   - Marker mode (newMarkerScanner) lexes SOI, EOI and the segment headers.
//     After a segment header the driver reads the two-byte, big-endian length
//     from it and reads the payload from the cursor as one SEGMENT_PAYLOAD,
//     so bytes in a payload are never taken for a marker.
//   - Scan mode (newScanScanner) follows the payload of a start-of-scan segment.
//     It lexes the entropy-coded data, and the markers as marker mode does.
//     The driver goes back to marker mode after any marker but SOS.
//   - TIFF mode (newTIFFScanner) follows an EXIF_SEGMENT_HEADER when the rest of
//     the payload can hold a TIFF header. After the TIFF header the driver
//     reads the first IFD in the byte order of the header: the bytes up to
//     the IFD offset as TIFF_DATA, the IFD_ENTRY_COUNT, each 12-byte
//     IFD_ENTRY, and the rest of the payload as TIFF_DATA. No field reaches
//     past the end of the segment. If the bytes are not a TIFF header, the
//     TIFF scanner returns one UNKNOWN byte, and the driver reads the rest of
//     the payload as SEGMENT_PAYLOAD.
//
// The driver follows the three rules of a mode driver:
//
//  1. It calls Release on the active scanner at every mode switch and before it
//     reads fields from the cursor, so an idle scanner does not pin the input.
//  2. It builds the three scanners once, in newScanner, and never drops one.
//  3. Next holds a mutex, so no two scanners on the cursor run at the same time.
//
// Every scanner returns a plain io.EOF at the end of the input. When the input
// ends inside a payload or the scan, the last token holds the bytes up to the
// end, and Next returns io.EOF after it.
type driver struct {
	mu     sync.Mutex
	modes  *modes.Driver
	marker *bytescan.Scanner
	scan   *bytescan.Scanner
	tiff   *bytescan.Scanner
	fields *bytesource.Fields

	rest int // the bytes left in the Exif segment, in TIFF mode
}

// Next returns the next queued field, or else the next token of the active
// scanner, and switches the mode or queues fields after it.
func (d *driver) Next() (*bytescan.Token, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if l, err := d.fields.NextField(); l != nil || err != nil {
		return l, err
	}
	active := d.modes.Active()
	l, err := d.modes.Next()
	if err != nil {
		return nil, err
	}
	b := bytesource.Bytes(l.Text())
	switch l.Type() {
	case segmentHeader:
		// The length counts itself and the payload (T.81, B.1.1.4).
		next := d.marker
		if b[1] == markerSOS {
			next = d.scan
		}
		d.switchTo(next)
		d.fields.Push(bytesource.Field{Type: segmentPayload, Len: be16(b[2:]) - 2})
	case exifSegmentHeader:
		// The header holds the six bytes of the Exif identifier; the rule
		// takes only lengths of 8 or more, so rest is never negative.
		rest := be16(b[2:]) - 2 - 6
		if rest >= 8 {
			d.rest = rest
			d.switchTo(d.tiff)
			break
		}
		d.switchTo(d.marker)
		d.fields.Push(bytesource.Field{Type: segmentPayload, Len: rest})
	case tiffHeader:
		d.switchTo(d.marker)
		d.queueIFD(b)
	case soi, eoi:
		d.modes.Use(d.marker)
	default:
		// ENTROPY_DATA stays in scan mode, and UNKNOWN in its mode, except
		// in TIFF mode: the bytes are not a TIFF header.
		if active == d.tiff {
			d.switchTo(d.marker)
			d.fields.Push(bytesource.Field{Type: segmentPayload, Len: d.rest - l.Len()})
		}
	}
	return l, nil
}

// queueIFD queues the fields of the first IFD after the TIFF header h, in the
// byte order of h (TIFF 6.0, section 2).
func (d *driver) queueIFD(h []byte) {
	var order binary.ByteOrder = binary.LittleEndian
	if h[0] == 'M' {
		order = binary.BigEndian
	}
	rest := d.rest - len(h)

	// The IFD offset counts from the start of the TIFF header. An offset
	// inside the header, or past the segment, leaves no room for the IFD.
	offset := int64(order.Uint32(h[4:]))
	if offset < int64(len(h)) || offset-int64(len(h)) > int64(rest-2) {
		d.fields.Push(bytesource.Field{Type: tiffData, Len: rest})
		return
	}
	gap := int(offset) - len(h)
	rest -= gap + 2
	d.fields.Push(
		bytesource.Field{Type: tiffData, Len: gap},
		bytesource.Field{Type: ifdEntryCount, Len: 2, Then: func(c []byte) {
			n := min(int(order.Uint16(c)), rest/12)
			for i := 0; i < n; i++ {
				d.fields.Push(bytesource.Field{Type: ifdEntry, Len: 12})
			}
			d.fields.Push(bytesource.Field{Type: tiffData, Len: rest - 12*n})
		}},
	)
}

// switchTo releases the active scanner, even when lx is that scanner, because the
// driver reads fields from the cursor next, and makes lx the active one. At a
// switch, Use releases the outgoing scanner again, which does nothing.
func (d *driver) switchTo(lx *bytescan.Scanner) {
	d.modes.Active().Release()
	d.modes.Use(lx)
}

func be16(b []byte) int { return int(binary.BigEndian.Uint16(b)) }
