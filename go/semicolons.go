package main

import (
	"io"
	"strings"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/cursor"
)

// semicolons is a filter above the Next method of a scanner. It inserts a
// SEMICOLON token where rule 1 of the Go specification, section "Semicolons",
// requires one:
//
//	When the input is broken into tokens, a semicolon is automatically
//	inserted into the token stream immediately after a line's final token if
//	that token is
//	  - an identifier
//	  - an integer, floating-point, imaginary, rune, or string literal
//	  - one of the keywords break, continue, fallthrough, or return
//	  - one of the operators and punctuation ++, --, ), ], or }
//
// A line ends at a NEWLINE, at a general comment that contains a newline (the
// specification reads such a comment as a newline), and at the end of the
// input. The inserted token comes just before the token that ends the line,
// at its position; at the end of the input it comes at the end of the last
// token.
//
// An inserted token is a bytescan.NewEmptyToken, and its empty text is how
// a reader tells it apart: the scanner never returns an empty token. The filter follows rule 1 only.
// Rule 2, which lets a program leave out a semicolon before ")" or "}", is for
// the parser, and the filter inserts nothing there. The filter never looks at
// the structure of the program. Comments, layout and UNKNOWN tokens are not
// tokens, so they do not change what the line's final token is.
type semicolons struct {
	lx bytescan.Source

	// insert reports whether the last token read ends a line with a semicolon.
	insert bool
	// end is the end of the last token read.
	end cursor.Pos
	// pending is a token read from lx and held back behind an inserted
	// semicolon.
	pending *bytescan.Token
}

// newSemicolons returns the filter over lx.
func newSemicolons(lx bytescan.Source) *semicolons {
	return &semicolons{lx: lx}
}

// Next returns the next token of the scanner, or an inserted SEMICOLON before
// it.
func (f *semicolons) Next() (*bytescan.Token, error) {
	if l := f.pending; l != nil {
		f.pending = nil
		return l, nil
	}
	l, err := f.lx.Next()
	if err == io.EOF && f.insert {
		f.insert = false
		return inserted(f.end), nil
	}
	if err != nil {
		return nil, err
	}
	f.end = l.Span().End()

	switch {
	case endsLine(l):
		if f.insert {
			f.insert = false
			f.pending = l
			return inserted(l.Span().Start()), nil
		}
	case isToken(l.Type()):
		f.insert = endsStatement(l)
	}
	return l, nil
}

// endsLine reports whether l ends a line.
func endsLine(l *bytescan.Token) bool {
	switch l.Type() {
	case newline:
		return true
	case comment:
		return strings.HasPrefix(l.Text(), "/*") && strings.Contains(l.Text(), "\n")
	}
	return false
}

// isToken reports whether tokens of type t are tokens of the Go specification.
// Comments, white space and runes no rule matches are not.
func isToken(t bytescan.TokenType) bool {
	switch t {
	case comment, whitespace, newline, bytescan.TokenTypeUnknown:
		return false
	}
	return true
}

// endsStatement reports whether a semicolon follows the token l when l is the
// final token of a line.
func endsStatement(l *bytescan.Token) bool {
	switch l.Type() {
	case identifier, intLit, floatLit, imaginaryLit, runeLit, stringLit:
		return true
	case keyword:
		switch l.Text() {
		case "break", "continue", "fallthrough", "return":
			return true
		}
	case operator:
		switch l.Text() {
		case "++", "--", ")", "]", "}":
			return true
		}
	}
	return false
}

// inserted returns an inserted SEMICOLON at p.
func inserted(p cursor.Pos) *bytescan.Token {
	return bytescan.NewEmptyToken(semicolon, p)
}

// isInserted reports whether l is a SEMICOLON that the filter inserted.
func isInserted(l *bytescan.Token) bool {
	return l.Type() == semicolon && l.Len() == 0
}
