package main

import (
	"bufio"
	"io"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/rules/core"
)

// The token types, named after the controls of ECMA-48: TEXT is a run of
// runes that are not controls, and C0 any other control of the C0 set.
const (
	text   bytescan.TokenType = "TEXT"
	bel    bytescan.TokenType = "BEL"
	bs     bytescan.TokenType = "BS"
	tab    bytescan.TokenType = "TAB"
	lf     bytescan.TokenType = "LF"
	cr     bytescan.TokenType = "CR"
	c0     bytescan.TokenType = "C0"
	csi    bytescan.TokenType = "CSI"
	osc    bytescan.TokenType = "OSC"
	escSeq bytescan.TokenType = "ESC_SEQ"
)

// The byte classes of ECMA-48 (5th edition), written as runes: the C0 set of
// section 5.2, and the bytes of a control sequence of section 5.4.
var (
	// control is the C0 set, 0x00 to 0x1F, and DEL, 0x7F. Neither is text.
	control = core.RuneRange(0x00, 0x1F).Or(core.RuneSet("\x7F"))

	// textRune is every rune that is not a control: printable ASCII and every
	// rune above it.
	textRune = control.Not()

	parameter    = core.RuneRange(0x30, 0x3F) // 0123456789:;<=>?
	intermediate = core.RuneRange(0x20, 0x2F) // space and !"#$%&'()*+,-./
	csiFinal     = core.RuneRange(0x40, 0x7E) // @ A-Z [\]^_` a-z {|}~
	escFinal     = core.RuneRange(0x30, 0x7E) // 0-9 :;<=>? @ A-Z ... ~
)

// newScanner returns a scanner for text mixed with ECMA-48 control functions, read
// from r. It lexes the 7-bit forms only: an escape sequence starts with ESC.
//
// No two rules match the same text, so the registration order decides no tie.
// TEXT never takes a control. Each single-control rule takes a different
// control, and C0 takes the rest of the C0 set except ESC. The ESC rules differ
// in their second rune: CSI takes ESC [, OSC takes ESC ], and ESC_SEQ takes
// neither.
func newScanner(r io.Reader) *bytescan.Scanner {
	lx := bytescan.New(bufio.NewReader(r))

	lx.MustAddRule(text, core.RuneRun(textRune))

	lx.MustAddRule(bel, core.Literal("\a"))
	lx.MustAddRule(bs, core.Literal("\b"))
	lx.MustAddRule(tab, core.Literal("\t"))
	lx.MustAddRule(lf, core.Literal("\n"))
	lx.MustAddRule(cr, core.Literal("\r"))
	// ESC is left out: it only starts an escape sequence, so an ESC that
	// starts none is UNKNOWN.
	lx.MustAddRule(c0, core.Rune(core.RuneRange(0x00, 0x1F).And(core.RuneSet("\a\b\t\n\r\x1B").Not())))

	lx.MustAddRule(csi, csiRule())
	lx.MustAddRule(osc, oscRule())
	lx.MustAddRule(escSeq, escapeRule())

	return lx
}

// csiRule returns a rule for a control sequence (ECMA-48, section 5.4):
//
//	\x1B\[[\x30-\x3F]*[\x20-\x2F]*[\x40-\x7E]
//
// ESC [ is the 7-bit form of CSI. The parameter bytes are not split into
// parameters or checked against the final byte: ESC[1;31m, ESC[?25l and
// ESC[;;;H are all one CSI token.
func csiRule() bytescan.Rule {
	return core.Sequence(
		core.Literal("\x1B["),
		core.ZeroOrMore(core.Rune(parameter)),
		core.ZeroOrMore(core.Rune(intermediate)),
		core.Rune(csiFinal),
	)
}

// oscRule returns a rule for an operating system command (ECMA-48, sections
// 8.3.89 and 5.6), ended by BEL as xterm also allows, or by ST:
//
//	\x1B\][^\x00-\x1F\x7F]*(?:\x07|\x1B\\)
//
// ESC ] is the 7-bit form of OSC and ESC \ the 7-bit form of ST. The command
// string is a run of text runes, so it can hold UTF-8 (xterm takes UTF-8
// window titles), and any other control ends the match without a terminator.
func oscRule() bytescan.Rule {
	return core.Sequence(
		core.Literal("\x1B]"),
		core.ZeroOrMore(core.Rune(textRune)),
		core.Choice(core.Literal("\a"), core.Literal("\x1B\\")),
	)
}

// escapeRule returns a rule for any other escape sequence (ECMA-35, clause 13,
// and ECMA-48, sections 5.3 and 5.5):
//
//	\x1B(?:[\x20-\x2F]+[\x30-\x7E]|[\x30-\x5A\x5C\x5E-\x7E])
//
// It takes ESC 7, ESC c, ESC ( B and ESC \ (ST alone). A final byte right after
// ESC is never [ or ]: those open a CSI or an OSC, so a cut-off CSI or OSC is
// not an ESC_SEQ.
func escapeRule() bytescan.Rule {
	return core.Sequence(
		core.Literal("\x1B"),
		core.Choice(
			core.Sequence(core.RuneRun(intermediate), core.Rune(escFinal)),
			core.Rune(escFinal.And(core.RuneSet("[]").Not())),
		),
	)
}
