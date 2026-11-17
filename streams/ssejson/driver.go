package main

import (
	"sync"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/modes"
)

// newJSONDriver returns the mode driver of the JSON layer over cur, the
// cursor that reads the pipe, with every scanner built with opts. It starts in
// value mode: structure, literals and numbers. JSON_QUOTE switches between
// value mode and string mode; in string mode, JSON_EOL also returns to value
// mode, because a JSON string cannot hold a line feed, so a line feed ends an
// unterminated string.
//
// modes.Driver follows the three rules of a mode driver on this cursor: it
// calls Release on the outgoing scanner at every switch, the two scanners are
// built once, here, and its Next holds a mutex.
func newJSONDriver(cur bytescan.Cursor, opts ...bytescan.Option) *modes.Driver {
	value := newValueScanner(cur, opts...)
	str := newStringScanner(cur, opts...)
	return modes.New(cur, value, modes.WithSwitch(func(active *bytescan.Scanner, l *bytescan.Token) *bytescan.Scanner {
		switch {
		case l.Type() == jsonQuote && active == value:
			return str
		case l.Type() == jsonQuote, l.Type() == jsonEOL:
			return value
		}
		return nil
	}))
}

// driver is the piped scanner: the SSE layer, the pipe, and the JSON layer.
// Its Next returns the SSE tokens that the pipe queued, in order, and then
// the JSON token whose read made the pipe queue them. So the output is one
// sequence that shows both layers, in read order. The JSON layer's cursor
// reads ahead (it fills at least 4 bytes before it decodes a rune, and the
// engine reads one rune past a token), so the SSE layer runs ahead of the
// JSON layer: the SSE tokens of the next event can come before the last
// JSON token of a document.
//
// The JSON scanner is inside Next while the pipe calls the SSE driver's Next.
// That is allowed, because the two scanners do not share a cursor. The driver's
// own Next holds a mutex, so no two calls interleave.
type driver struct {
	mu    sync.Mutex
	pipe  *pipe
	inner *modes.Driver

	held    *bytescan.Token // the JSON token to return after the queue
	heldErr error           // or the JSON layer's error
	holding bool
}

// Next returns the next token of either layer, in read order.
func (d *driver) Next() (*bytescan.Token, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if l := d.pipe.pop(); l != nil {
		return l, nil
	}
	if d.holding {
		l, err := d.held, d.heldErr
		d.held, d.heldErr, d.holding = nil, nil, false
		return l, err
	}
	// Every queued token has been returned: the counters start again.
	d.pipe.reset()
	l, err := d.inner.Next()
	if q := d.pipe.pop(); q != nil {
		d.held, d.heldErr, d.holding = l, err, true
		return q, nil
	}
	return l, err
}
