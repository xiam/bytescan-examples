package main

import (
	"strings"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/modes"
)

// levelKey names one cached level scanner: a long string or a long comment, and
// the level of its bracket.
type levelKey struct {
	comment bool
	level   int
}

// driver lexes Lua with several scanners over one cursor, on a modes.Driver.
// Code mode lexes every token and the opener of a long bracket. After an
// opener the driver switches to the scanner for that kind and level, which lexes
// the body and the closer; after the closer it switches back to code mode.
//
// The modes package applies the three rules of a mode driver: the switch
// calls Release on the outgoing scanner, the modes.Cache builds one scanner per
// kind and level and never drops it, and Next holds a mutex, so no two scanners
// on the cursor run at the same time.
//
// Every scanner returns a plain io.EOF at the end of the input. When the input
// ends inside a long bracket, the body runs to the end of the input, no closer
// follows, and Next returns io.EOF as it does in code mode.
type driver struct {
	*modes.Driver
	code   *bytescan.Scanner
	levels *modes.Cache[levelKey]
}

// newDriver returns a driver over cur in code mode.
func newDriver(cur bytescan.Cursor, code *bytescan.Scanner) *driver {
	d := &driver{
		code:   code,
		levels: modes.NewCache(cur, buildLevelScanner),
	}
	d.Driver = modes.New(cur, code, modes.WithSwitch(d.next))
	return d
}

// next picks the scanner after the token l: a level scanner after an opener, the
// code scanner after a closer, and no switch otherwise.
func (d *driver) next(_ *bytescan.Scanner, l *bytescan.Token) *bytescan.Scanner {
	switch l.Type() {
	case longStringOpen:
		return d.levels.Get(levelKey{comment: false, level: strings.Count(l.Text(), "=")})
	case longCommentOpen:
		return d.levels.Get(levelKey{comment: true, level: strings.Count(l.Text(), "=")})
	case longStringClose, longCommentClose:
		return d.code
	}
	return nil
}

// buildLevelScanner builds the scanner for k over cur.
func buildLevelScanner(cur bytescan.Cursor, k levelKey) *bytescan.Scanner {
	body, close := longString, longStringClose
	if k.comment {
		body, close = longComment, longCommentClose
	}
	return newLevelScanner(cur, body, close, k.level)
}
