package main

import (
	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/modes"
)

// frame is one entry of the driver's mode stack. A template frame lexes the
// text of a template literal. A code frame lexes code; the code frame that a
// "${" pushes counts the "{" and "}" inside the substitution in depth, only to
// find the "}" that closes it.
type frame struct {
	template bool
	depth    int
}

// codeKey names one code-mode scanner: the goal symbol, where "/" starts a
// REGEX or is a DIV_PUNCTUATOR, and whether a "}" closes a substitution.
type codeKey struct {
	regexGoal bool
	exprClose bool
}

// driver lexes TypeScript with several scanners over one cursor, on a
// modes.Driver: up to four for code mode, which differ only in what "/"
// starts and in what "}" is, and one for template mode.
//
// After each token the driver changes the stack. TEMPLATE_OPEN pushes a
// template frame and TEMPLATE_CLOSE pops it; TEMPLATE_EXPR_OPEN pushes a code
// frame and TEMPLATE_EXPR_CLOSE pops it. In that code frame "{" adds one to
// its depth and "}" takes one away. At depth 0 the driver picks a code scanner
// in which "}" is TEMPLATE_EXPR_CLOSE: the lexical rule of the specification
// (section 12, the InputElementTemplateTail goal). The count never judges
// whether the braces balance. Templates nest to any depth.
//
// Then the driver picks the scanner of the next token. In a template frame it
// picks the template scanner. In a code frame it picks a scanner in which "/" is a
// DIV_PUNCTUATOR when the previous significant token can end an expression
// (see divAfter), and otherwise one in which "/" starts a REGEX. Whitespace,
// line breaks and comments are not significant.
//
// The modes package applies the three rules of a mode driver: the switch
// calls Release on the outgoing scanner, a modes.Cache builds each code scanner
// once and never drops it, and Next holds a mutex, so no two scanners on the
// cursor run at the same time.
//
// Every scanner returns a plain io.EOF at the end of the input. When the input
// ends inside a template or a substitution, the stack is not empty, and Next
// returns io.EOF as it does at the top level.
type driver struct {
	*modes.Driver
	code     *modes.Cache[codeKey]
	template *bytescan.Scanner

	// stack holds the modes; stack[0] is the top-level code frame.
	stack []frame
	// regexOK reports whether "/" starts a REGEX after the previous
	// significant token.
	regexOK bool
}

// newDriver returns a driver over cur at the top level, where "/" starts a
// REGEX.
func newDriver(cur bytescan.Cursor) *driver {
	d := &driver{
		code:     modes.NewCache(cur, newCodeScanner),
		template: newTemplateScanner(cur),
		stack:    []frame{{}},
		regexOK:  true,
	}
	d.Driver = modes.New(cur, d.pick(), modes.WithSwitch(d.next))
	return d
}

// next updates the stack and the previous significant token from l, and
// picks the scanner of the next token.
func (d *driver) next(_ *bytescan.Scanner, l *bytescan.Token) *bytescan.Scanner {
	top := &d.stack[len(d.stack)-1]
	switch l.Type() {
	case templateOpen:
		d.stack = append(d.stack, frame{template: true})
	case templateExprOpen:
		d.stack = append(d.stack, frame{})
	case templateClose, templateExprClose:
		d.pop()
	case punctuator:
		if len(d.stack) == 1 {
			break // the top-level frame does not count braces
		}
		switch l.Text() {
		case "{":
			top.depth++
		case "}":
			top.depth--
		}
	}

	switch l.Type() {
	case whitespace, newline, comment:
	default:
		d.regexOK = !divAfter(l)
	}
	return d.pick()
}

// divAfter reports whether a "/" after l is a DIV_PUNCTUATOR, because l can
// end an expression: an identifier, a literal, ")", "]", "}", or one of the
// keywords this, super, true, false and null. After any other token, such as
// "=", "(", "," or the keyword return, "/" starts a REGEX.
//
// The rule looks only at l. It is wrong where the token before "/" does not
// tell; README.md lists those cases.
func divAfter(l *bytescan.Token) bool {
	switch l.Type() {
	case identifier, number, stringLiteral, regex, templateClose:
		return true
	case punctuator:
		switch l.Text() {
		case ")", "]", "}":
			return true
		}
	case keyword:
		switch l.Text() {
		case "this", "super", "true", "false", "null":
			return true
		}
	}
	return false
}

// pick returns the scanner for the current mode.
func (d *driver) pick() *bytescan.Scanner {
	top := d.stack[len(d.stack)-1]
	if top.template {
		return d.template
	}
	return d.code.Get(codeKey{regexGoal: d.regexOK, exprClose: len(d.stack) > 1 && top.depth == 0})
}

// pop removes the top frame. The top-level frame stays.
func (d *driver) pop() {
	if len(d.stack) > 1 {
		d.stack = d.stack[:len(d.stack)-1]
	}
}
