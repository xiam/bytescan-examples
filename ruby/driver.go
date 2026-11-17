package main

import (
	"strings"
	"unicode/utf8"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/modes"
)

// frameKind names what a frame of the driver's stack is inside of.
type frameKind int

const (
	codeFrame    frameKind = iota // the program: the bottom of the stack
	braceFrame                    // a "{" in code
	interpFrame                   // a "#{" in a double-quoted string
	stringFrame                   // a double-quoted string
	percentFrame                  // the body of a percent literal
)

// frame is one entry of the driver's stack. A percent frame also holds the
// delimiters of its literal and the depth of the nested bracket pairs.
type frame struct {
	kind    frameKind
	percent percentKey
	depth   int
}

// percentKey names one cached percent-literal scanner: words (%w, %i) or text
// (%q, %Q), the delimiters, and whether the driver is inside a nested pair.
type percentKey struct {
	words       bool
	open, close rune
	nested      bool
}

// heredocKey names one cached heredoc scanner: the terminator, and whether it
// may be indented (<<- and <<~).
type heredocKey struct {
	id     string
	indent bool
}

// brackets maps each opening bracket of a percent literal to its closer.
var brackets = map[rune]rune{'(': ')', '[': ']', '{': '}', '<': '>'}

// driver lexes Ruby with several scanners over one cursor, on a modes.Driver.
// A stack of frames gives the mode of the next token: code, code inside "#{",
// a double-quoted string, or the body of a percent literal. A queue holds the
// heredocs opened on the current line; after the line break that ends that
// line, the driver lexes their bodies in order, and then returns to the mode
// on the stack.
//
// The modes package applies the three rules of a mode driver: the switch
// calls Release on the outgoing scanner, a modes.Cache builds one scanner per
// percent delimiter and nesting and one per heredoc terminator and never
// drops it, and Next holds a mutex, so no two scanners on the cursor run at the
// same time.
//
// Every scanner returns a plain io.EOF at the end of the input. When the input
// ends inside a string, a percent literal or a heredoc, the body runs to the
// end of the input, no closer follows, and Next returns io.EOF as it does in
// code mode.
type driver struct {
	*modes.Driver
	code     *bytescan.Scanner // code, and code inside "{"
	interp   *bytescan.Scanner // code directly inside "#{"
	str      *bytescan.Scanner // the inside of a double-quoted string
	percents *modes.Cache[percentKey]
	heredocs *modes.Cache[heredocKey]
	stack    []frame
	pending  []heredocKey // heredocs opened and not yet read, in order
	inBody   bool         // lexing the body of pending[0]
}

// newDriver returns a driver over cur in code mode.
func newDriver(cur bytescan.Cursor) *driver {
	code := newCodeScanner(cur, braceClose)
	d := &driver{
		code:     code,
		interp:   newCodeScanner(cur, interpolationClose),
		str:      newStringScanner(cur),
		percents: modes.NewCache(cur, newPercentScanner),
		heredocs: modes.NewCache(cur, newHeredocScanner),
		stack:    []frame{{kind: codeFrame}},
	}
	d.Driver = modes.New(cur, code, modes.WithSwitch(d.next))
	return d
}

// next updates the stack or the heredoc queue from the type of the token l,
// which the scanner active returned, and picks the scanner of the new mode.
func (d *driver) next(active *bytescan.Scanner, l *bytescan.Token) *bytescan.Scanner {
	top := &d.stack[len(d.stack)-1]
	switch l.Type() {
	case stringOpen:
		d.push(frame{kind: stringFrame})
	case interpolationOpen:
		d.push(frame{kind: interpFrame})
	case braceOpen:
		d.push(frame{kind: braceFrame})
	case braceClose:
		if top.kind == braceFrame {
			d.pop()
		}
	case stringClose, interpolationClose, percentClose:
		d.pop()
	case percentOpen:
		d.push(frame{kind: percentFrame, percent: percentKeyOf(l.Text())})
	case percentNestOpen:
		top.depth++
	case percentNestClose:
		top.depth--
	case heredocOpen:
		d.pending = append(d.pending, heredocKeyOf(l.Text()))
	case newline:
		// Only a line break of code starts the pending bodies; a line break
		// in a %w body does not.
		if active == d.code || active == d.interp {
			d.inBody = len(d.pending) > 0
		}
	case heredocEnd:
		d.pending = d.pending[1:]
		d.inBody = false
	}
	return d.scanner()
}

func (d *driver) push(f frame) { d.stack = append(d.stack, f) }

// pop removes the top frame, and never the code frame at the bottom.
func (d *driver) pop() {
	if len(d.stack) > 1 {
		d.stack = d.stack[:len(d.stack)-1]
	}
}

// scanner returns the scanner of the current mode.
func (d *driver) scanner() *bytescan.Scanner {
	if d.inBody {
		return d.heredocs.Get(d.pending[0])
	}
	top := d.stack[len(d.stack)-1]
	switch top.kind {
	case interpFrame:
		return d.interp
	case stringFrame:
		return d.str
	case percentFrame:
		k := top.percent
		k.nested = top.depth > 0
		return d.percents.Get(k)
	}
	return d.code
}

// percentKeyOf returns the key of the body scanner for a PERCENT_OPEN token
// such as "%w[": the kind is the rune after "%", the delimiter the last rune.
func percentKeyOf(opener string) percentKey {
	open, _ := utf8.DecodeLastRuneInString(opener)
	close, ok := brackets[open]
	if !ok {
		close = open
	}
	kind := opener[1]
	return percentKey{words: kind == 'w' || kind == 'i', open: open, close: close}
}

// heredocKeyOf returns the key of the body scanner for a HEREDOC_OPEN token
// such as "<<~'EOS'": the identifier without quotes, and whether "-" or "~"
// allows an indented terminator.
func heredocKeyOf(opener string) heredocKey {
	s := strings.TrimPrefix(opener, "<<")
	indent := strings.HasPrefix(s, "-") || strings.HasPrefix(s, "~")
	if indent {
		s = s[1:]
	}
	return heredocKey{id: strings.Trim(s, `'"`), indent: indent}
}
