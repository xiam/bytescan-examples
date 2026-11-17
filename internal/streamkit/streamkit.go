// Package streamkit holds the code that more than one stream example needs
// and that no public package covers: a field longer than an int, and the
// quoted dump format of the byte protocols with text control lines.
package streamkit

import (
	"math"
	"strconv"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/bytesource"
)

// MaxSegment is the most bytes that PushLong queues in one field. The length
// of a bytesource.Field is an int, which is 32 bits on some platforms, and a
// declared length of a stream can be larger (a 10-digit syslog MSG_LEN or
// NATS size, a 64-bit WebSocket payload length). A test lowers MaxSegment to
// reach the segment boundary in a few bytes.
var MaxSegment uint64 = math.MaxInt

// PushLong queues n bytes of type typ on f, read in pieces of at most
// maxPiece bytes: one chunked field of at most MaxSegment bytes, whose Done
// queues the next segment, until the last segment, whose Done is done (which
// may be nil). For n of 0 it queues one empty field, which gives no token
// and calls done when its turn comes.
//
// The next segment is pushed behind whatever is queued when the previous one
// ends, so a field that follows a long field must be queued by done, not
// pushed after PushLong returns. It allocates a closure only for a length
// longer than MaxSegment.
func PushLong(f *bytesource.Fields, typ bytescan.TokenType, n uint64, maxPiece int, done func()) {
	seg := min(n, MaxSegment)
	fd := bytesource.Field{Type: typ, Len: int(seg), MaxPiece: maxPiece, Done: done}
	if rest := n - seg; rest > 0 {
		fd.Done = func() { PushLong(f, typ, rest, maxPiece, done) }
	}
	f.Push(fd)
}

// Quote returns the line for l in the quoted view:
//
//	OFFSET TYPE LENGTH "BYTES"
//
// OFFSET is the byte offset of the token in the input and LENGTH its length
// in bytes, as in bytesource.Hex. BYTES are the bytes of the token as a Go
// quoted string, so text reads as text and every other byte stays on one
// line. When limit is above 0 and the token is longer, only its first limit
// bytes are shown, followed by "...".
func Quote(l *bytescan.Token, limit int) string {
	b := bytesource.Bytes(l.Text())
	more := ""
	if limit > 0 && len(b) > limit {
		b, more = b[:limit], "..."
	}
	return strconv.Itoa(l.Offset()) + " " + string(l.Type()) + " " +
		strconv.Itoa(l.Len()) + " " + strconv.Quote(string(b)) + more
}
