package main

import (
	"bufio"
	"io"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/cursor"
	"github.com/xiam/bytescan/rules/base"
	"github.com/xiam/bytescan/rules/core"
)

// The token types. The code-mode scanners emit the tokens of code and the
// openers that the driver in driver.go switches on; the string, percent and
// heredoc scanners emit the bodies and the closers.
const (
	keyword            bytescan.TokenType = "KEYWORD"
	identifier         bytescan.TokenType = "IDENTIFIER"
	constant           bytescan.TokenType = "CONSTANT"
	label              bytescan.TokenType = "LABEL"
	instanceVariable   bytescan.TokenType = "INSTANCE_VARIABLE"
	classVariable      bytescan.TokenType = "CLASS_VARIABLE"
	globalVariable     bytescan.TokenType = "GLOBAL_VARIABLE"
	symbol             bytescan.TokenType = "SYMBOL"
	operator           bytescan.TokenType = "OPERATOR"
	punctuation        bytescan.TokenType = "PUNCTUATION"
	integer            bytescan.TokenType = "INTEGER"
	float              bytescan.TokenType = "FLOAT"
	stringLit          bytescan.TokenType = "STRING"
	comment            bytescan.TokenType = "COMMENT"
	whitespace         bytescan.TokenType = "WHITESPACE"
	braceOpen          bytescan.TokenType = "BRACE_OPEN"
	braceClose         bytescan.TokenType = "BRACE_CLOSE"
	newline            bytescan.TokenType = "NEWLINE"
	stringOpen         bytescan.TokenType = "STRING_OPEN"
	stringText         bytescan.TokenType = "STRING_TEXT"
	stringClose        bytescan.TokenType = "STRING_CLOSE"
	interpolationOpen  bytescan.TokenType = "INTERPOLATION_OPEN"
	interpolationClose bytescan.TokenType = "INTERPOLATION_CLOSE"
	percentOpen        bytescan.TokenType = "PERCENT_OPEN"
	percentWord        bytescan.TokenType = "PERCENT_WORD"
	percentText        bytescan.TokenType = "PERCENT_TEXT"
	percentNestOpen    bytescan.TokenType = "PERCENT_NEST_OPEN"
	percentNestClose   bytescan.TokenType = "PERCENT_NEST_CLOSE"
	percentClose       bytescan.TokenType = "PERCENT_CLOSE"
	heredocOpen        bytescan.TokenType = "HEREDOC_OPEN"
	heredocBody        bytescan.TokenType = "HEREDOC_BODY"
	heredocEnd         bytescan.TokenType = "HEREDOC_END"
)

// newScanner returns a Ruby scanner that reads r: a mode driver over one cursor,
// with the scanners of code mode and interpolation mode, the scanner of a
// double-quoted string, and a scanner per percent-literal delimiter and per
// heredoc terminator, built when the driver first needs it.
func newScanner(r io.Reader) *driver {
	return newDriver(cursor.New(bufio.NewReader(r)))
}

// keywords are the 41 reserved words of the Ruby documentation, "Keywords"
// (doc/syntax/keywords.rdoc).
var keywords = []string{
	"__ENCODING__", "__LINE__", "__FILE__", "BEGIN", "END",
	"alias", "and", "begin", "break", "case", "class", "def", "defined?",
	"do", "else", "elsif", "end", "ensure", "false", "for", "if", "in",
	"module", "next", "nil", "not", "or", "redo", "rescue", "retry",
	"return", "self", "super", "then", "true", "undef", "unless", "until",
	"when", "while", "yield",
}

// newCodeScanner returns a scanner for code over cur: every token of the subset
// and the openers of the string, percent and heredoc modes. The closing brace
// gets the type rbrace: BRACE_CLOSE in code mode, and INTERPOLATION_CLOSE in
// the scanner that the driver uses directly inside "#{", where that brace ends
// the interpolation.
//
// Two ties are broken by registration order: KEYWORD is registered before
// IDENTIFIER and CONSTANT, because a reserved word ("end", "defined?",
// "BEGIN") also matches the name rules. The longest match decides every other
// overlap: "<<~A" is HEREDOC_OPEN, not OPERATOR "<<"; "%w[" is PERCENT_OPEN,
// not OPERATOR "%"; "empty?" is one IDENTIFIER; "if:" is LABEL, not KEYWORD;
// ":a" is SYMBOL and "::" is OPERATOR, not two ":"; "1.5" is FLOAT, not
// INTEGER "1"; "..." and "**=" are one OPERATOR.
func newCodeScanner(cur bytescan.Cursor, rbrace bytescan.TokenType) *bytescan.Scanner {
	lx := bytescan.NewWithCursor(cur)

	lx.MustAddRule(keyword, core.Keywords(keywords...))
	lx.MustAddRule(identifier, methodNameRule(core.ASCIILower.Or(core.RuneSet("_"))))
	lx.MustAddRule(constant, base.Identifier(core.ASCIIUpper, core.ASCIIWord))
	lx.MustAddRule(label, core.Sequence(
		base.ASCIIIdentifier(),
		core.NotFollowedBy(core.Literal(":"), core.Literal(":")),
	))
	lx.MustAddRule(instanceVariable, core.Sequence(core.Literal("@"), base.ASCIIIdentifier()))
	lx.MustAddRule(classVariable, core.Sequence(core.Literal("@@"), base.ASCIIIdentifier()))
	lx.MustAddRule(globalVariable, core.Sequence(core.Literal("$"), base.ASCIIIdentifier()))
	lx.MustAddRule(symbol, core.Sequence(core.Literal(":"), methodNameRule(core.ASCIIIdentifierStart)))

	lx.MustAddRule(operator, core.AnyLiteral(
		"!", "!=", "!~", "&&", "||", "&&=", "||=",
		"+", "-", "*", "**", "/", "%", "+=", "-=", "*=", "**=", "/=", "%=",
		"&", "|", "^", "~", "<<", ">>", "&=", "|=", "^=", "<<=", ">>=",
		"=", "==", "===", "=~", "<", "<=", ">", ">=", "<=>",
		"..", "...", "?", ":", "::", "->", "=>", "&.",
	))
	lx.MustAddRule(punctuation, core.AnyLiteral("(", ")", "[", "]", ",", ".", ";"))
	lx.MustAddRule(braceOpen, core.Literal("{"))
	lx.MustAddRule(rbrace, core.Literal("}"))

	digits := base.DigitSeparator('_')
	lx.MustAddRule(integer, core.Choice(
		base.Integer(digits), base.HexInteger(digits),
		base.BinaryInteger(digits), base.OctalInteger(digits),
	))
	lx.MustAddRule(float, base.Float(digits))

	lx.MustAddRule(stringLit, base.QuotedMultiline('\'', '\\'))
	lx.MustAddRule(stringOpen, core.Literal(`"`))
	lx.MustAddRule(percentOpen, percentOpenRule())
	lx.MustAddRule(heredocOpen, heredocOpenRule())

	lx.MustAddRule(comment, base.LineComment("#"))
	lx.MustAddRule(whitespace, base.Whitespace())
	lx.MustAddRule(newline, base.Newline())

	return lx
}

// methodNameRule returns a rule for a name that starts with a rune of start,
// with the optional "?" or "!" of a method name (doc/syntax/methods.rdoc):
//
//	[start][A-Za-z0-9_]*(?:[?!](?!=))?
//
// IDENTIFIER, a local variable or method name, starts with [a-z_]; the name of
// a SYMBOL starts with [A-Za-z_]. The suffix is not taken before "=", so "a!=b"
// is "a", "!=" and "b".
func methodNameRule(start core.Class) bytescan.Rule {
	return core.Sequence(
		base.Identifier(start, core.ASCIIWord),
		core.Optional(core.NotFollowedBy(core.Rune(core.RuneSet("?!")), core.Literal("="))),
	)
}

// percentOpenRule returns a rule for the opener of a percent literal
// (doc/syntax/literals.rdoc, "Percent Literals"): "%", the kind, and the
// delimiter, any ASCII punctuation or symbol rune except the backslash:
//
//	%[wiqQ][!-/:-@\[\]^`{-~]
//
// The backslash is left out because it escapes the rune after it in the body,
// so it could never close the literal. The driver reads the kind and the
// delimiter from the opener and switches to the scanner built for them.
func percentOpenRule() bytescan.Rule {
	delimiter := core.RuneRange('!', '~').And(core.ASCIIWord.Not()).And(core.RuneSet(`\`).Not())
	return core.Sequence(core.Literal("%"), core.Rune(core.RuneSet("wiqQ")), core.Rune(delimiter))
}

// heredocOpenRule returns a rule for the opener of a here document
// (doc/syntax/literals.rdoc, "Here Documents"): "<<", an optional "-" or "~",
// and the identifier, bare or in single or double quotes:
//
//	<<[-~]?(?:ID|'ID'|"ID")    where ID is [A-Za-z_][A-Za-z0-9_]*
func heredocOpenRule() bytescan.Rule {
	id := base.ASCIIIdentifier()
	return core.Sequence(
		core.Literal("<<"),
		core.Optional(core.Rune(core.RuneSet("-~"))),
		core.Choice(
			id,
			core.Sequence(core.Literal("'"), id, core.Literal("'")),
			core.Sequence(core.Literal(`"`), id, core.Literal(`"`)),
		),
	)
}

// newStringScanner returns the scanner for the inside of a double-quoted string
// over cur (doc/syntax/literals.rdoc, "Strings"). It emits:
//
//	STRING_TEXT         (?:[^"\\#]|\\(?s:.)|#(?!\{))+
//	INTERPOLATION_OPEN  #\{
//	STRING_CLOSE        "
//
// The text may span lines. An escaped "#" does not open an interpolation, and
// a "#" that no "{" follows is text: the text is base.EscapedRun with the stop
// rune `"` and the opener "#{". No two rules match the same text.
func newStringScanner(cur bytescan.Cursor) *bytescan.Scanner {
	lx := bytescan.NewWithCursor(cur)
	lx.MustAddRule(stringText, base.EscapedRun(core.RuneSet(`"`), '\\', "#{"))
	lx.MustAddRule(interpolationOpen, core.Literal("#{"))
	lx.MustAddRule(stringClose, core.Literal(`"`))
	return lx
}

// newPercentScanner returns the scanner for the body of a percent literal with the
// opening delimiter k.open over cur. For %w and %i (k.words) the body is words
// split by layout; for %q and %Q it is one text. A backslash escapes the rune
// after it, so an escaped delimiter or space is part of the word or text.
//
// For a bracket pair, such as "[" and "]", an inner opening bracket is
// PERCENT_NEST_OPEN, and the driver counts it to find the closing bracket of
// the literal. At depth 0 (k.nested false) the closing bracket is
// PERCENT_CLOSE; inside a nested pair (k.nested true) it is
// PERCENT_NEST_CLOSE. For any other delimiter the literal closes at the next
// occurrence of it, and nothing nests. The rules are, with D the delimiters:
//
//	PERCENT_WORD   (?:[^D\\\s]|\\(?s:.))+        %w and %i
//	PERCENT_TEXT   (?:[^D\\]|\\(?s:.))+          %q and %Q
//	PERCENT_NEST_OPEN, PERCENT_NEST_CLOSE, PERCENT_CLOSE   one delimiter
//
// No two rules match the same text.
func newPercentScanner(cur bytescan.Cursor, k percentKey) *bytescan.Scanner {
	delims := string(k.open)
	if k.close != k.open {
		delims += string(k.close)
	}
	stop := core.RuneSet(delims)

	lx := bytescan.NewWithCursor(cur)
	if k.words {
		layout := core.RuneSet(" \t\v\f\r\n")
		lx.MustAddRule(percentWord, base.EscapedRun(stop.Or(layout), '\\'))
		lx.MustAddRule(whitespace, base.Whitespace())
		lx.MustAddRule(newline, base.Newline())
	} else {
		lx.MustAddRule(percentText, base.EscapedRun(stop, '\\'))
	}
	if k.close != k.open {
		lx.MustAddRule(percentNestOpen, core.Literal(string(k.open)))
	}
	if k.nested {
		lx.MustAddRule(percentNestClose, core.Literal(string(k.close)))
	} else {
		lx.MustAddRule(percentClose, core.Literal(string(k.close)))
	}
	return lx
}

// newHeredocScanner returns the scanner for the body of a here document with the
// terminator k.id over cur. The driver switches to it only after a line break,
// so it always starts at the beginning of a line. It emits:
//
//	HEREDOC_BODY  (?:L?\n)*L?(?!.)   lines L that are not the terminator line
//	HEREDOC_END   W?ID(?![^\r\n])    the terminator line, without its line break
//
// where \n is any line break (\r\n, \n or \r), W is a run of spaces and tabs,
// allowed only for <<- and <<~ (k.indent), and L is the text of one line
// that is not W?ID. The body stops before the terminator line, so the two rules
// never match the same text. When no terminator line follows, the body runs
// to the end of the input. No base rule stops a run at a line that the input
// names, so the body and the terminator are composed here.
func newHeredocScanner(cur bytescan.Cursor, k heredocKey) *bytescan.Scanner {
	nl := base.Newline()
	line := notTerminator(k)

	end := []bytescan.Rule{core.Literal(k.id)}
	if k.indent {
		end = append([]bytescan.Rule{core.Optional(core.RuneRun(core.RuneSet(" \t")))}, end...)
	}

	lx := bytescan.NewWithCursor(cur)
	lx.MustAddRule(heredocBody, core.Sequence(
		core.ZeroOrMore(core.Sequence(core.Optional(line), nl)),
		core.Optional(core.NotFollowedBy(line, core.Rune(core.AnyRune))),
	))
	lx.MustAddRule(heredocEnd, core.NotFollowedBy(
		core.Sequence(end...),
		core.Rune(core.RuneSet("\r\n").Not()),
	))
	return lx
}

// notTerminator returns a rule for the text of one line, without its line
// break, that is not the terminator line W?ID of k. The rule is the
// complement of W?ID among the nonempty runs of [^\r\n], written as the ways a
// line can differ from it. With R the line after its leading run of spaces
// and tabs (k.indent), or the whole line (not k.indent):
//
//   - R is empty: the line is only spaces and tabs;
//   - R is a nonempty proper prefix of ID;
//   - R starts with ID[:j] and then a rune other than ID[j], for some j;
//   - R is ID followed by more text.
//
// With k.indent, R starts with a rune that is not a space or a tab, so no
// split of the leading run lets "  ID" count as a different line.
func notTerminator(k heredocKey) bytescan.Rule {
	text := core.RuneSet("\r\n").Not()
	rest := core.ZeroOrMore(core.Rune(text))
	blank := core.RuneSet(" \t")
	id := []rune(k.id)

	var ways []bytescan.Rule
	for j := range id {
		first := text.And(core.RuneSet(string(id[j])).Not())
		if j == 0 && k.indent {
			first = first.And(blank.Not())
		}
		differ := core.Sequence(core.Rune(first), rest)
		if j > 0 {
			ways = append(ways, core.Literal(string(id[:j])))
			differ = core.Sequence(core.Literal(string(id[:j])), differ)
		}
		ways = append(ways, differ)
	}
	ways = append(ways, core.Sequence(core.Literal(k.id), core.RuneRun(text)))
	r := core.Choice(ways...)

	if !k.indent {
		return r
	}
	return core.Choice(
		core.RuneRun(blank),
		core.Sequence(core.Optional(core.RuneRun(blank)), r),
	)
}
