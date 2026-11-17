package main

import (
	"strconv"
	"sync"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan-examples/internal/streamkit"
	"github.com/xiam/bytescan/bytesource"
	"github.com/xiam/bytescan/modes"
)

// mode is what the driver lexes next.
type mode int

const (
	modeFrame     mode = iota // the start of a frame
	modeLen                   // the SP after MSG_LEN
	modeBadLen                // the rest of a digit run that is not a MSG_LEN
	modePRI                   // PRI
	modeVersion               // VERSION and its SP
	modeTimestamp             // TIMESTAMP and its SP
	modeHostname              // HOSTNAME and its SP
	modeAppName               // APP_NAME and its SP
	modeProcID                // PROCID and its SP
	modeMsgID                 // MSGID and its SP
	modeSD                    // STRUCTURED-DATA between elements
	modeElement               // inside an SD-ELEMENT
	modeValue                 // inside a PARAM-VALUE
	modeMsg                   // MSG
)

// driver lexes a syslog stream with one scanner per mode over one byte cursor.
//
// A frame is octet-counted when it starts with a digit (RFC 6587 section
// 3.4.1): MSG_LEN, SP, and a message of MSG_LEN bytes. Any other frame is
// newline-ended (section 3.4.2): a message and LF. In both, the driver lexes
// the header of the message (RFC 5424 section 6) field by field: each field
// has its own scanner, and SP moves to the next field.
//
// In an octet-counted frame the driver counts the bytes of the header tokens.
// After the SP that ends STRUCTURED-DATA it reads the rest of the frame from
// the cursor as MSG, through a bytesource.Fields queue, in pieces of at most
// maxPiece bytes. A scanner over a
// shared cursor cannot stop at the frame end, so when a header token reaches
// or passes the frame end, the frame ends after that token: there is no MSG,
// and the next frame starts at the current position. In a newline-ended frame
// MSG is lexed by rules up to LF, and LF in any mode ends the frame.
//
// A digit run that is not 1 to 10 digits followed by SP is not a length: the
// driver emits it as UNKNOWN pieces of at most maxRun bytes, reads no MSG by
// length, and lexes the rest of the line as a newline-ended message.
//
// The scanners run on a modes.Driver. Together they follow the three rules of
// a mode driver:
//
//  1. The modes.Driver calls Release on the outgoing scanner at every switch,
//     and the driver calls Release on the active scanner before it peeks at or
//     reads the cursor itself.
//  2. It builds one scanner per mode, once, and never builds another one.
//  3. Next holds a mutex, so two scanners never run at the same time.
//
// Every scanner returns a plain io.EOF at the end of the input. When the input
// ends inside a frame, the driver lexes what is there and stops normally.
type driver struct {
	mu  sync.Mutex
	cur bytescan.Cursor

	modes    *modes.Driver
	scanners [modeMsg + 1]*bytescan.Scanner // by mode; modeLen and modeBadLen use the frame scanner

	mode    mode
	counted bool               // the frame is octet-counted
	size    int64              // MSG_LEN of an octet-counted frame
	read    int64              // bytes of the message read so far
	msg     *bytesource.Fields // MSG of an octet-counted frame
	msgDone func()             // ends the frame after the last piece of MSG
}

// newDriver returns the driver over cur, at the start of a frame. Every scanner
// is built with opts.
func newDriver(cur bytescan.Cursor, opts ...bytescan.Option) *driver {
	d := &driver{cur: cur, msg: bytesource.NewFields(cur)}
	d.msgDone = func() { d.mode = modeFrame }
	frame := newFrameScanner(cur, opts...)
	d.scanners[modeFrame] = frame
	d.scanners[modeLen] = frame
	d.scanners[modeBadLen] = frame
	d.scanners[modePRI] = newPRIScanner(cur, opts...)
	d.scanners[modeVersion] = newFieldScanner(cur, version, versionRule(), opts...)
	d.scanners[modeTimestamp] = newFieldScanner(cur, timestamp, timestampRule(), opts...)
	d.scanners[modeHostname] = newFieldScanner(cur, hostname, tokenRule(), opts...)
	d.scanners[modeAppName] = newFieldScanner(cur, appName, tokenRule(), opts...)
	d.scanners[modeProcID] = newFieldScanner(cur, procID, tokenRule(), opts...)
	d.scanners[modeMsgID] = newFieldScanner(cur, msgID, tokenRule(), opts...)
	d.scanners[modeSD] = newSDScanner(cur, opts...)
	d.scanners[modeElement] = newElementScanner(cur, opts...)
	d.scanners[modeValue] = newValueScanner(cur, opts...)
	d.scanners[modeMsg] = newMsgScanner(cur, opts...)
	d.modes = modes.New(cur, frame)
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
	switch d.mode {
	case modeFrame, modeBadLen:
		b, err := d.peek()
		if err != nil {
			return nil, err
		}
		if b < 0 {
			return d.next() // io.EOF from the scanner
		}
		if !isDigit(b) && !(d.mode == modeBadLen && b == ' ') {
			// A newline-ended frame, or the rest of the line after a digit
			// run that is not a length.
			d.start(false, 0)
			return d.lex()
		}
		l, err := d.next()
		if err != nil {
			return nil, err
		}
		switch {
		case l.Type() == sp:
			d.start(false, 0) // after a bad digit run
		case l.Type() == msgLen && d.mode == modeFrame:
			n, err := strconv.ParseInt(l.Text(), 10, 64)
			if err != nil {
				return nil, err // cannot happen: at most 10 digits
			}
			d.size = n
			d.mode = modeLen
		default:
			// A digit run that is not a length, or a piece of it.
			d.mode = modeBadLen
			l = l.WithType(unknown)
		}
		return l, nil

	case modeLen:
		l, err := d.next() // the SP that MSG_LEN is followed by
		if err != nil {
			return nil, err
		}
		d.start(true, d.size)
		return l, nil

	case modeMsg:
		if d.counted {
			return d.readMsg()
		}
	}

	l, err := d.next()
	if err != nil {
		return nil, err
	}
	d.advance(l.Type())
	if d.counted {
		d.read += int64(l.Len())
		switch {
		case d.read >= d.size:
			d.mode = modeFrame
		case d.mode == modeMsg:
			// MSG_LEN has up to 10 digits, more than an int holds on a
			// 32-bit platform, so PushLong may queue MSG as several fields.
			streamkit.PushLong(d.msg, msg, uint64(d.size-d.read), maxPiece, d.msgDone)
		}
	}
	return l, nil
}

// start starts the message of a frame: octet-counted with size bytes, or
// newline-ended.
func (d *driver) start(counted bool, size int64) {
	d.counted, d.size, d.read = counted, size, 0
	d.mode = modePRI
	if counted && size == 0 {
		d.mode = modeFrame
	}
}

// advance moves to the mode after a token of type typ.
func (d *driver) advance(typ bytescan.TokenType) {
	switch {
	case typ == lf && !d.counted:
		d.mode = modeFrame
	case typ == pri && d.mode == modePRI:
		d.mode = modeVersion
	case typ == sp && d.mode >= modeVersion && d.mode <= modeMsgID:
		d.mode++
	case typ == sdElementOpen:
		d.mode = modeElement
	case typ == sdElementClose:
		d.mode = modeSD
	case typ == sdQuote && d.mode == modeElement:
		d.mode = modeValue
	case typ == sdQuote && d.mode == modeValue:
		d.mode = modeElement
	case typ == sp && d.mode == modeSD:
		d.mode = modeMsg
	}
}

// readMsg reads the next piece of MSG of an octet-counted frame from the
// cursor. The Done of the field ends the frame after its last piece. When the
// queue is empty, because the input ended inside the frame, the next frame
// starts and finds the end of the input.
func (d *driver) readMsg() (*bytescan.Token, error) {
	d.modes.Active().Release()
	l, err := d.msg.NextField()
	if err != nil {
		return nil, err
	}
	if l == nil {
		d.mode = modeFrame
		return d.lex()
	}
	return l, nil
}

// next returns the next token of the scanner of the current mode. The
// modes.Driver releases the scanner that ran before it when that is another
// one.
func (d *driver) next() (*bytescan.Token, error) {
	d.modes.Use(d.scanners[d.mode])
	return d.modes.Next()
}

// peek returns the next byte of the cursor without consuming it, or -1 at
// the end of the input.
func (d *driver) peek() (int, error) {
	d.modes.Active().Release()
	b, err := bytesource.Peek(d.cur, 1)
	if err != nil || len(b) == 0 {
		return -1, err
	}
	return int(b[0]), nil
}

func isDigit(b int) bool { return b >= '0' && b <= '9' }
