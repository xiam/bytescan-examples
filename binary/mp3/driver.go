package main

import (
	"encoding/binary"
	"sync"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/bytesource"
	"github.com/xiam/bytescan/modes"
)

// driver lexes an MP3 file with three scanners, in a mode driver, and a queue
// of fields over one cursor. Its modes:
//
//   - Start mode (newStartScanner) lexes the first token of the file. After an
//     ID3_HEADER the driver takes the tag size from it and switches to tag
//     mode; after anything else it switches to audio mode.
//   - Tag mode (newTagScanner) lexes the frame headers of the tag. After a frame
//     header the driver reads the body from the cursor as one ID3_FRAME_BODY,
//     with the size the header states: a 32-bit big-endian number in
//     ID3v2.3, a synchsafe number in ID3v2.4. When fewer than 10 bytes of the
//     tag are left, or the next byte is 00, the rest of the tag is
//     ID3_PADDING. No field reaches past the end of the tag, so the bytes of
//     the tag are never taken for an audio frame. After the tag the driver
//     switches to audio mode.
//   - Audio mode (newAudioScanner) lexes the audio frame headers. After a frame
//     header the driver computes the length of the frame from its bitrate,
//     sampling rate and padding bit, and reads the rest of the frame from the
//     cursor as one FRAME_BODY.
//
// The driver follows the three rules of a mode driver:
//
//  1. It calls Release on the active scanner at every mode switch and before it
//     reads a body or the padding from the cursor, so an idle scanner does not
//     pin them.
//  2. It builds the three scanners once, in newDriver, and never drops one.
//  3. Next holds a mutex, so no two scanners on the cursor, and no scanner and a
//     field read, run at the same time.
//
// Every scanner returns a plain io.EOF at the end of the input. When the input
// ends inside a body or the tag, the last token holds the bytes up to the
// end, and Next returns io.EOF after it.
type driver struct {
	mu     sync.Mutex
	cur    bytescan.Cursor
	modes  *modes.Driver
	start  *bytescan.Scanner
	tag    *bytescan.Scanner
	audio  *bytescan.Scanner
	fields *bytesource.Fields

	version int // the major version of the ID3v2 tag: 3 or 4
	rest    int // the bytes left in the tag, in tag mode
}

// Next returns the next queued field, or else the next token of the active
// scanner, and switches the mode or queues the body after it.
func (d *driver) Next() (*bytescan.Token, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if l, err := d.fields.NextField(); l != nil || err != nil {
		return l, err
	}
	if d.modes.Active() == d.tag {
		end, err := d.tagEnds()
		if err != nil {
			return nil, err
		}
		if end {
			d.switchTo(d.audio)
			d.fields.Push(bytesource.Field{Type: id3Padding, Len: d.rest})
			d.rest = 0
			if l, err := d.fields.NextField(); l != nil || err != nil {
				return l, err
			}
		}
	}
	active := d.modes.Active()
	l, err := d.modes.Next()
	if err != nil {
		return nil, err
	}
	b := bytesource.Bytes(l.Text())
	switch l.Type() {
	case id3Header:
		d.version = int(b[3])
		d.rest = synchsafe(b[6:])
		d.switchTo(d.tag)
	case id3FrameHeader:
		size := int(binary.BigEndian.Uint32(b[4:]) & 0x7FFFFFFF)
		if d.version == 4 {
			size = synchsafe(b[4:])
		}
		d.rest -= len(b)
		size = min(size, d.rest)
		d.rest -= size
		d.switchTo(d.tag)
		d.fields.Push(bytesource.Field{Type: id3FrameBody, Len: size})
	case frameHeader:
		d.switchTo(d.audio)
		d.fields.Push(bytesource.Field{Type: frameBody, Len: frameLength(b) - len(b)})
	default:
		// UNKNOWN: in tag mode it is a byte of the tag; in start mode the
		// file has no tag.
		switch active {
		case d.tag:
			d.rest -= l.Len()
		case d.start:
			d.switchTo(d.audio)
		}
	}
	return l, nil
}

// tagEnds reports whether the rest of the tag is padding: fewer bytes than a
// frame header are left, or the next byte is 00, which no frame id starts
// with (ID3v2.3, section 3.3).
func (d *driver) tagEnds() (bool, error) {
	if d.rest < 10 {
		return true, nil
	}
	b, err := bytesource.Peek(d.cur, 1)
	if err != nil {
		return false, err
	}
	return len(b) == 0 || b[0] == 0, nil
}

// switchTo releases the active scanner, even when lx is that scanner, because the
// driver reads a body from the cursor next, and makes lx the active one. At a
// switch, Use releases the outgoing scanner again, which does nothing.
func (d *driver) switchTo(lx *bytescan.Scanner) {
	d.modes.Active().Release()
	d.modes.Use(lx)
}

// synchsafe returns the 28-bit synchsafe integer in the first four bytes of
// b: seven bits from each byte, most significant first (ID3v2.4 structure,
// section 6.2). The high bit of each byte is ignored.
func synchsafe(b []byte) int {
	n := 0
	for _, c := range b[:4] {
		n = n<<7 | int(c&0x7F)
	}
	return n
}

// The bitrates in kbit/s by bitrate index, and the sampling rates in Hz by
// sampling-rate index, of Layer III (ISO/IEC 11172-3, section 2.4.2.3;
// ISO/IEC 13818-3, section 2.4.2.3). Index 0 of the bitrates is free format,
// which the header rule does not match.
var (
	bitrates = map[byte][15]int{
		mpeg1: {0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320},
		mpeg2: {0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160},
	}
	samplingRates = map[byte][3]int{
		mpeg1: {44100, 48000, 32000},
		mpeg2: {22050, 24000, 16000},
	}
)

// The version bits of the second header byte.
const (
	mpeg1 = 3
	mpeg2 = 2
)

// frameLength returns the length in bytes of the Layer III frame whose header
// is h, the header included. A frame holds 1152 samples in MPEG-1 and 576 in
// MPEG-2, so its length is 144 (or 72) times the bitrate over the sampling
// rate, rounded down, plus one byte when the padding bit is set.
func frameLength(h []byte) int {
	version := h[1] >> 3 & 3
	bitrate := bitrates[version][h[2]>>4] * 1000
	rate := samplingRates[version][h[2]>>2&3]
	padding := int(h[2] >> 1 & 1)
	factor := 144
	if version == mpeg2 {
		factor = 72
	}
	return factor*bitrate/rate + padding
}
