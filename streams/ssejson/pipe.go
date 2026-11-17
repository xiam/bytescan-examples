package main

import (
	"unicode/utf8"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan-examples/streams/sse/ssedriver"
)

// The thresholds of the pipe's counters. When the SSE tokens counted since
// the last reset reach batchTokens, or their runes reach batchRunes, the pipe
// reads no further SSE token until the next reset.
const (
	batchTokens = 64
	batchRunes  = 2000
)

// keepAlive is what the pipe returns while its counters are full and it holds
// no bytes: utf8.UTFMax line feeds. The cursor fills at least utf8.UTFMax
// bytes before it decodes a rune, so one keep-alive lets the JSON scanner
// return without a second read of the SSE layer.
var keepAlive = [utf8.UTFMax]byte{'\n', '\n', '\n', '\n'}

// lineFeed is the line feed that the pipe returns after the line end of a
// data line, as the SSE format appends one to the data buffer.
var lineFeed = [1]byte{'\n'}

// pipe is the io.Reader between the two layers. On Read it calls Next on the
// SSE driver until it gets a piece of a data value, and returns the UTF-8
// text of that piece; after the line end of a data line it returns one line
// feed. It keeps every SSE token it read in a queue, in order, for the
// combined driver to return. It starts no goroutine: the JSON layer's cursor
// calls Read from inside the JSON scanner's Next, and Read calls the SSE
// driver's Next. The two layers never share a cursor.
//
// Read returns at most one piece per call and at most len(b) bytes. When b is
// shorter than the piece, the pipe holds the rest for the next Read, and
// reads no SSE token until it has returned it. The same holds for the line
// feed and the keep-alive.
//
// The queue is bounded. The pipe counts the SSE tokens it queued since the
// last reset, and the runes of their text, except the VALUE pieces of a data
// field: those are queued but not counted, because their text goes to the
// JSON layer. When a count reaches its threshold, Read returns the bytes it
// still holds, and otherwise a keep-alive, until the combined driver resets
// the counters, which it does when it has returned every queued token.
type pipe struct {
	sse bytescan.Source

	queue []*bytescan.Token // SSE tokens read and not yet returned
	head  int               // index of the next token to return

	hold   []byte // bytes to return before the next SSE token
	inData bool   // inside a data line: after DATA, before its line end
	err    error  // the SSE driver's error, io.EOF at the end; sticky

	tokens int // SSE tokens counted since the last reset
	runes  int // runes of their text

	queued     int // runes of every token in the queue, counted or not
	maxTokens  int // the most tokens in the queue at once
	maxRunes   int // the most runes in the queue at once
	keepAlives int64

	// onKeepAlive, when set, is called with the number of bytes the pipe
	// returned before each keep-alive. Tests use it.
	onKeepAlive func(off int64)
	returned    int64 // bytes returned by Read
}

// newPipe returns a pipe that reads the SSE tokens of sse.
func newPipe(sse bytescan.Source) *pipe {
	return &pipe{sse: sse}
}

// Read returns the text of the next data piece, a line feed after a data
// line, the rest of either, or a keep-alive. It returns the SSE driver's
// error, io.EOF at the end of the stream, once it holds no more bytes.
func (p *pipe) Read(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	for len(p.hold) == 0 {
		if p.err != nil {
			return 0, p.err
		}
		if p.full() {
			if p.onKeepAlive != nil {
				p.onKeepAlive(p.returned)
			}
			p.keepAlives++
			p.hold = keepAlive[:]
			break
		}
		p.pull()
	}
	n := copy(b, p.hold)
	p.hold = p.hold[n:]
	p.returned += int64(n)
	return n, nil
}

// full reports whether a count has reached its threshold.
func (p *pipe) full() bool {
	return p.tokens >= batchTokens || p.runes >= batchRunes
}

// pull reads one SSE token, queues it, and counts it, or holds its text for
// Read when it is a data piece.
func (p *pipe) pull() {
	l, err := p.sse.Next()
	if err != nil {
		p.err = err
		return
	}
	p.queue = append(p.queue, l)
	p.queued += l.Len()
	p.maxTokens = max(p.maxTokens, len(p.queue)-p.head)
	p.maxRunes = max(p.maxRunes, p.queued)

	switch l.Type() {
	case ssedriver.Data:
		// The SSE driver emits DATA only for the field name at the start of
		// a line; a longer name is FIELD.
		p.inData = true
	case ssedriver.Value:
		if p.inData {
			// The engine allocates the text of every token, so the queued
			// token stays valid, and this copy is the pipe's own.
			p.hold = []byte(l.Text())
			return
		}
	case ssedriver.EOL:
		if p.inData {
			p.inData = false
			p.hold = lineFeed[:]
		}
	}
	p.tokens++
	p.runes += l.Len()
}

// pop returns the next queued SSE token, or nil when the queue is empty.
func (p *pipe) pop() *bytescan.Token {
	if p.head == len(p.queue) {
		return nil
	}
	l := p.queue[p.head]
	p.queue[p.head] = nil
	p.head++
	p.queued -= l.Len()
	return l
}

// reset empties the queue and resets the counters. The combined driver calls
// it when it has returned every queued token, before it calls the JSON
// driver's Next again.
func (p *pipe) reset() {
	p.queue = p.queue[:0]
	p.head = 0
	p.queued = 0
	p.tokens = 0
	p.runes = 0
}
