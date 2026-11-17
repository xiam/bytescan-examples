// Package ssedriver lexes a Server-Sent Events stream (the WHATWG
// text/event-stream format) with a mode driver over one cursor. It is the
// driver of the streams/sse example, in an importable package so that the
// streams/ssejson example can pipe its data values into a second scanner. See
// ../README.md for the tokens and the covered subset.
package ssedriver

import (
	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/modes"
)

// state is the position of the driver in a line. The position decides the
// type of a token (a name at the start of a line, a value after the colon),
// and a rule cannot see it, so the driver keeps it and picks the scanner.
//
// The states group into the three modes of the stream grammar:
//
//   - line start: stateStart (the first line of the stream, which may begin
//     with the BOM) and stateLine (every other line);
//   - after the field name: stateName (before the colon) and stateColon (right
//     after it, where one space may follow);
//   - the rest of the line: stateValue and stateComment.
type state int

const (
	stateStart state = iota
	stateLine
	stateName
	stateColon
	stateValue
	stateComment
	numStates
)

// Driver lexes a text/event-stream with one scanner per state over one cursor.
// Before each token it calls the scanner of the current state; after it, the
// type of the token picks the next state:
//
//	BOM                           -> line start
//	EVENT, DATA, ID, RETRY, FIELD -> after the field name (before the colon)
//	COLON                         -> after the colon
//	SPACE, VALUE                  -> value
//	COMMENT                       -> comment
//	EOL, EVENT_END                -> line start
//
// A FIELD, VALUE or COMMENT piece keeps the state, so the next piece of a run
// longer than maxRun runes gets the same type.
//
// The driver runs its scanners on a modes.Driver. Together they follow the
// three rules of a mode driver:
//
//  1. At every switch it calls Release on the outgoing scanner, so an idle scanner
//     does not pin the input that the other scanners read.
//  2. The six scanners are built once, in New, and the driver never builds a
//     scanner per token or drops one.
//  3. Next holds a mutex, so no two scanners on the cursor run at the same time.
//
// Every scanner returns a plain io.EOF at the end of the input, so a stream that
// ends inside a line ends as one that ends after a blank line.
type Driver struct {
	modes    *modes.Driver
	scanners [numStates]*bytescan.Scanner
	state    state
}

// newDriver returns a driver over cur in stateStart. Every mode scanner is
// built with opts.
func newDriver(cur bytescan.Cursor, opts ...bytescan.Option) *Driver {
	d := &Driver{
		scanners: [numStates]*bytescan.Scanner{
			stateStart:   newLineScanner(cur, true, opts...),
			stateLine:    newLineScanner(cur, false, opts...),
			stateName:    newNameScanner(cur, opts...),
			stateColon:   newColonScanner(cur, opts...),
			stateValue:   newRunScanner(cur, Value, opts...),
			stateComment: newRunScanner(cur, Comment, opts...),
		},
		state: stateStart,
	}
	d.modes = modes.New(cur, d.scanners[stateStart], modes.WithSwitch(d.after))
	return d
}

// Next returns the next token from the scanner of the current state, and
// switches the state from the type of the token.
func (d *Driver) Next() (*bytescan.Token, error) {
	return d.modes.Next()
}

// after moves to the state that follows a token l and returns its scanner. Each
// state has its own scanner, so the scanner changes exactly when the state does.
// modes.Driver calls it with its mutex held.
func (d *Driver) after(_ *bytescan.Scanner, l *bytescan.Token) *bytescan.Scanner {
	d.state = next(d.state, l.Type())
	return d.scanners[d.state]
}

// next returns the state after a token of type t read in state s.
func next(s state, t bytescan.TokenType) state {
	switch t {
	case BOM, EOL, EventEnd:
		return stateLine
	case Event, Data, ID, Retry, Field:
		return stateName
	case Colon:
		return stateColon
	case Space, Value:
		return stateValue
	case Comment:
		return stateComment
	}
	// UNKNOWN: no scanner emits it, because every rune is a name-char, a colon,
	// or part of a line end, and each scanner covers all three. Stay.
	return s
}
