package main

import (
	"strconv"
	"strings"
	"sync"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan-examples/internal/streamkit"
	"github.com/xiam/bytescan/bytesource"
	"github.com/xiam/bytescan/modes"
)

// field is the kind of one field of a control line, by its place.
type field int

const (
	fieldSubject   field = iota // SUBJECT
	fieldSID                    // SID
	fieldReplyLast              // REPLY_TO, or the only size of PUB and MSG
	fieldReplyPair              // REPLY_TO, or the header size of HPUB and HMSG
	fieldQueue                  // QUEUE_GROUP, or the SID of SUB
	fieldSize                   // SIZE
	fieldMaxMsgs                // MAX_MSGS
	fieldJSON                   // JSON
	fieldErrorText              // ERROR_TEXT
	fieldKinds                  // the number of kinds
)

// lineFields lists the fields after each operation, by the client protocol.
// An optional field (fieldReplyLast, fieldReplyPair, fieldQueue) takes the
// place of the field after it when it turns out to be that field.
var lineFields = map[string][]field{
	"INFO":    {fieldJSON},
	"CONNECT": {fieldJSON},
	"PUB":     {fieldSubject, fieldReplyLast, fieldSize},
	"HPUB":    {fieldSubject, fieldReplyPair, fieldSize, fieldSize},
	"SUB":     {fieldSubject, fieldQueue, fieldSID},
	"UNSUB":   {fieldSID, fieldMaxMsgs},
	"MSG":     {fieldSubject, fieldSID, fieldReplyLast, fieldSize},
	"HMSG":    {fieldSubject, fieldSID, fieldReplyPair, fieldSize, fieldSize},
	"PING":    nil,
	"PONG":    nil,
	"+OK":     nil,
	"-ERR":    {fieldErrorText},
}

// driver lexes the NATS client protocol with several scanners over one byte
// cursor. A control line is an operation, its fields separated by blanks, and
// a CRLF. The driver picks the scanner by the place in the line: the operation
// scanner at the start of a line, then the scanner of each field in turn. Once a
// field has a piece, the next pieces of that field come from the scanner of its
// type, so a field longer than 1000 bytes keeps its type in every piece.
//
// After the CRLF of a MSG or PUB line, the driver reads the payload of SIZE
// bytes from the cursor as PAYLOAD tokens of at most 64 KiB each; after
// HMSG or HPUB it reads the header bytes as HEADERS and the rest of the total
// size as PAYLOAD. Both are chunked fields of a bytesource.Fields queue, or
// several when a size does not fit an int (see streamkit.PushLong); the
// payload is queued when the last header segment has been read, so the
// segments never interleave. The operation scanner then lexes the CRLF after
// the payload.
// The driver reads a payload only after a line that lexed without UNKNOWN and
// with every size, so a damaged line frames nothing: lexing resumes at the
// next control line.
//
// The scanners run on a modes.Driver. Together they follow the three rules of
// a mode driver:
//
//  1. The modes.Driver calls Release on the outgoing scanner at every switch,
//     and the driver calls Release on the active scanner before it reads a
//     payload from the cursor, so an idle scanner does not pin the input.
//  2. It builds one scanner per mode when it is built, and never another one.
//  3. Next holds a mutex, so no two scanners on the cursor run at the same time.
//
// Every scanner returns a plain io.EOF at the end of the input. When the input
// ends inside a payload, the last PAYLOAD token holds the bytes up to the
// end, and Next returns io.EOF after it.
type driver struct {
	mu sync.Mutex

	// The cached scanners, one per mode.
	opLx        *bytescan.Scanner
	sepLx       *bytescan.Scanner
	longLx      *bytescan.Scanner
	fieldLx     [fieldKinds]*bytescan.Scanner // the first piece of each kind of field
	replyLx     *bytescan.Scanner             // the next pieces of a REPLY_TO
	queueLx     *bytescan.Scanner             // the next pieces of a QUEUE_GROUP
	jsonNext    *bytescan.Scanner             // the next pieces of a JSON
	errTextNext *bytescan.Scanner             // the next pieces of an ERROR_TEXT

	modes *modes.Driver // runs the scanner of the current place in the line

	// The control line. inLine is false at the start of a line.
	inLine bool
	opName string
	fields []field
	idx    int               // the field being lexed; -1 before the first
	piece  *bytescan.Scanner // the scanner of the next piece of the field, or nil
	sizes  []int64           // the sizes lexed so far
	bad    bool              // the line has an UNKNOWN token

	// The headers and the payload to read from the cursor after the line.
	queue     *bytesource.Fields
	body      int64  // the payload size, queued after the headers
	queueBody func() // queues the payload of body bytes
}

// newDriver returns the driver over cur, at the start of a line. Every scanner
// is built with opts.
func newDriver(cur bytescan.Cursor, opts ...bytescan.Option) *driver {
	d := &driver{
		opLx:        newOpScanner(cur, opts...),
		sepLx:       newSepScanner(cur, opts...),
		longLx:      newLongDigitsScanner(cur, opts...),
		replyLx:     newFieldScanner(cur, replyTo, opts...),
		queueLx:     newFieldScanner(cur, queueGroup, opts...),
		jsonNext:    newTextScanner(cur, jsonArg, false, opts...),
		errTextNext: newTextScanner(cur, errorText, false, opts...),
		sizes:       make([]int64, 0, 2),
		queue:       bytesource.NewFields(cur),
	}
	d.fieldLx = [fieldKinds]*bytescan.Scanner{
		fieldSubject:   newFieldScanner(cur, subject, opts...),
		fieldSID:       newFieldScanner(cur, sid, opts...),
		fieldReplyLast: newOptionalReplyScanner(cur, true, opts...),
		fieldReplyPair: newOptionalReplyScanner(cur, false, opts...),
		fieldQueue:     newOptionalQueueScanner(cur, opts...),
		fieldSize:      newSizeScanner(cur, opts...),
		fieldMaxMsgs:   newMaxMsgsScanner(cur, opts...),
		fieldJSON:      newTextScanner(cur, jsonArg, true, opts...),
		fieldErrorText: newTextScanner(cur, errorText, true, opts...),
	}
	d.modes = modes.New(cur, d.opLx)
	d.queueBody = func() {
		if d.body > 0 {
			streamkit.PushLong(d.queue, payload, uint64(d.body), maxPiece, nil)
		}
	}
	return d
}

// Next returns the next token: a piece of a pending payload, or else the
// next token of the scanner for the current place in the line.
func (d *driver) Next() (*bytescan.Token, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if l, err := d.queue.NextField(); l != nil || err != nil {
		return l, err
	}
	d.modes.Use(d.pick())
	l, err := d.modes.Next()
	if err != nil {
		return nil, err
	}
	return d.advance(l), nil
}

// pick returns the scanner for the current place in the line.
func (d *driver) pick() *bytescan.Scanner {
	switch {
	case !d.inLine:
		return d.opLx
	case d.piece != nil:
		return d.piece
	case d.idx < 0 || d.idx >= len(d.fields):
		return d.sepLx
	}
	return d.fieldLx[d.fields[d.idx]]
}

// advance moves the place in the line past l, and returns l, or the UNKNOWN
// token that a LONG_DIGITS piece stands for.
func (d *driver) advance(l *bytescan.Token) *bytescan.Token {
	switch l.Type() {
	case op:
		d.startLine(strings.ToUpper(l.Text()))
	case crlf:
		if d.inLine {
			d.endLine()
		}
	case sp:
		switch {
		case d.idx < 0:
			d.idx = 0
		case d.piece != nil:
			d.idx++
			d.piece = nil
		}
	case bytescan.TokenTypeUnknown:
		d.bad = true
	case longDigits:
		d.bad = true
		d.piece = d.longLx
		return l.WithType(bytescan.TokenTypeUnknown)
	case size:
		if d.piece == nil {
			d.optionalTaken()
			n, _ := strconv.ParseInt(l.Text(), 10, 64) // at most 10 digits
			d.sizes = append(d.sizes, n)
		}
		d.piece = d.fieldLx[fieldSize]
	case sid:
		if d.piece == nil {
			d.optionalTaken()
		}
		d.piece = d.fieldLx[fieldSID]
	case subject:
		d.piece = d.fieldLx[fieldSubject]
	case replyTo:
		d.piece = d.replyLx
	case queueGroup:
		d.piece = d.queueLx
	case maxMsgs:
		d.piece = d.fieldLx[fieldMaxMsgs]
	case jsonArg:
		d.piece = d.jsonNext
	case errorText:
		d.piece = d.errTextNext
	}
	return l
}

// optionalTaken skips the field after an optional field that turned out to
// be that next field: the first size after a missing reply subject, or the
// SID after a missing queue group.
func (d *driver) optionalTaken() {
	if d.idx >= 0 && d.idx < len(d.fields) {
		switch d.fields[d.idx] {
		case fieldReplyLast, fieldReplyPair, fieldQueue:
			d.idx++
		}
	}
}

// startLine starts the control line of the operation name.
func (d *driver) startLine(name string) {
	d.inLine = true
	d.opName = name
	d.fields = lineFields[name]
	d.idx = -1
	d.piece = nil
	d.sizes = d.sizes[:0]
	d.bad = false
}

// endLine ends the control line at its CRLF and sets the payload that
// follows it, if any.
func (d *driver) endLine() {
	d.inLine = false
	d.piece = nil
	if d.bad {
		return
	}
	var hdr, body int64
	switch d.opName {
	case "MSG", "PUB":
		if len(d.sizes) == 1 {
			body = d.sizes[0]
		}
	case "HMSG", "HPUB":
		// The second size is the total of headers and payload. It frames the
		// message, so a header size above it is cut to it.
		if len(d.sizes) == 2 {
			hdr = min(d.sizes[0], d.sizes[1])
			body = d.sizes[1] - hdr
		}
	}
	if hdr == 0 && body == 0 {
		return
	}
	// The queue reads the cursor in the next calls to Next. PushLong queues
	// the next segment of a long field behind whatever is queued, so the
	// payload is queued only by the Done of the last header segment. When
	// the input ends inside the headers, the queue is dropped and the
	// payload is never queued.
	d.modes.Active().Release()
	d.body = body
	streamkit.PushLong(d.queue, headers, uint64(hdr), maxPiece, d.queueBody)
}
