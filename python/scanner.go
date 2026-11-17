package main

import (
	"bufio"
	"io"
	"strings"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/rules/base"
	"github.com/xiam/bytescan/rules/core"
)

// The section numbers below are those of the Python language reference,
// chapter 2, "Lexical analysis", of Python 3.12.

// The token types. The scanner emits NL for every physical line break; the
// filter in filter.go relabels some of them NEWLINE and adds INDENT and
// DEDENT. NEWLINE, INDENT and DEDENT are the token names of section 2.1.
const (
	keyword    bytescan.TokenType = "KEYWORD"
	name       bytescan.TokenType = "NAME"
	stringLit  bytescan.TokenType = "STRING"
	integer    bytescan.TokenType = "INTEGER"
	float      bytescan.TokenType = "FLOAT"
	imaginary  bytescan.TokenType = "IMAGINARY"
	operator   bytescan.TokenType = "OPERATOR"
	delimiter  bytescan.TokenType = "DELIMITER"
	ellipsis   bytescan.TokenType = "ELLIPSIS"
	comment    bytescan.TokenType = "COMMENT"
	lineJoin   bytescan.TokenType = "LINE_JOIN"
	nl         bytescan.TokenType = "NL"
	whitespace bytescan.TokenType = "WHITESPACE"
	newline    bytescan.TokenType = "NEWLINE"
	indent     bytescan.TokenType = "INDENT"
	dedent     bytescan.TokenType = "DEDENT"
)

// keywords are the hard keywords (section 2.3.1). The soft keywords match,
// case, type and _ are names.
var keywords = []string{
	"False", "None", "True", "and", "as", "assert", "async", "await",
	"break", "class", "continue", "def", "del", "elif", "else", "except",
	"finally", "for", "from", "global", "if", "import", "in", "is",
	"lambda", "nonlocal", "not", "or", "pass", "raise", "return", "try",
	"while", "with", "yield",
}

// newScanner returns a scanner for a subset of Python that reads r. Its NL
// tokens are physical line breaks; the filter in filter.go turns the one
// that ends a logical line into NEWLINE and adds INDENT and DEDENT.
//
// One tie is broken by the order: a keyword is also a name, so KEYWORD is
// registered before NAME. Every other overlap is decided by the longest
// match: "classy" is a NAME, "rb'x'" a STRING, ".5" a FLOAT, "..." an
// ELLIPSIS and "**=" a DELIMITER.
func newScanner(r io.Reader) *bytescan.Scanner {
	lx := bytescan.New(bufio.NewReader(r))
	lx.MustAddRule(keyword, core.Keywords(keywords...))
	// Section 2.3: id_start and id_continue, without Other_ID_Start,
	// Other_ID_Continue and NFKC normalization.
	lx.MustAddRule(name, base.UnicodeIdentifier())
	lx.MustAddRule(stringLit, stringRule())
	lx.MustAddRule(integer, integerRule())
	lx.MustAddRule(float, floatRule())
	lx.MustAddRule(imaginary, imaginaryRule())
	lx.MustAddRule(operator, operatorRule())
	lx.MustAddRule(delimiter, delimiterRule())
	// Section 2.6: three periods are the ellipsis literal.
	lx.MustAddRule(ellipsis, core.Literal("..."))
	// Section 2.1.3: a comment runs to the end of the physical line.
	lx.MustAddRule(comment, base.LineComment("#"))
	// Section 2.1.5: explicit line joining, a backslash at the end of a
	// physical line.
	lx.MustAddRule(lineJoin, core.Sequence(core.Literal(`\`), base.Newline()))
	// Section 2.1.2: a physical line ends with \n, \r\n or \r.
	lx.MustAddRule(nl, base.Newline())
	// Section 2.1.9: space, tab and formfeed separate tokens.
	lx.MustAddRule(whitespace, core.RuneRun(core.RuneSet(" \t\f")))
	return lx
}

// stringRule returns a rule for a string or bytes literal (section 2.4.1):
//
//	stringliteral ::= [prefix] (shortstring | longstring)
//	shortstring   ::= "'" shortitem* "'" | '"' shortitem* '"'
//	longstring    ::= "'''" longitem* "'''" | '"""' longitem* '"""'
//
// where prefix is one of r, u, f, b, fr, rf, br and rb in any case. A short
// string holds no unescaped line break; a long string ends at the first
// unescaped triple quote. A backslash escapes the next character in every
// form, raw strings included, so r"\"" is one string. An f-string is one
// token: its replacement fields are not lexed.
func stringRule() bytescan.Rule {
	return core.Sequence(
		core.Optional(core.AnyLiteralIgnoreCase("r", "u", "f", "b", "fr", "rf", "br", "rb")),
		core.Choice(
			shortString('\''), shortString('"'),
			longString('\''), longString('"'),
		),
	)
}

// shortString returns the rule for a short string between quotes q:
//
//	q(?:[^\\\n\rq]|\\(?:\r\n|(?s:.)))*q
func shortString(q rune) bytescan.Rule {
	escape := core.Sequence(core.Literal(`\`), core.Choice(core.Literal("\r\n"), core.Rune(core.AnyRune)))
	return base.QuotedWith(q, core.RuneSet(string(q)+"\\\n\r").Not(), escape)
}

// longString returns the rule for a long string between triple quotes q:
//
//	qqq(?:q{0,2}(?:[^\\q]|\\(?s:.)))*qqq
//
// One or two quotes inside the string are content when a character other
// than a quote follows them, so the string ends at the first triple quote.
// No base rule has a closer of three quotes that may also hold one or two, so
// the rule is composed here.
func longString(q rune) bytescan.Rule {
	one := string(q)
	plain := core.Choice(
		core.Rune(core.RuneSet(one+`\`).Not()),
		core.Sequence(core.Literal(`\`), core.Rune(core.AnyRune)),
	)
	item := core.Sequence(core.Repeat(core.Literal(one), 0, 2), plain)
	return core.Sequence(core.Literal(one+one+one), core.ZeroOrMore(item), core.Literal(one+one+one))
}

// digitSeparator is the "_" that Python allows between digits and after a
// radix prefix (section 2.4.5).
var digitSeparator = base.DigitSeparator('_')

// integerRule returns a rule for an integer literal (section 2.4.5):
//
//	decinteger ::= nonzerodigit (["_"] digit)* | "0"+ (["_"] "0")*
//	bininteger ::= "0" ("b" | "B") (["_"] bindigit)+
//	octinteger ::= "0" ("o" | "O") (["_"] octdigit)+
//	hexinteger ::= "0" ("x" | "X") (["_"] hexdigit)+
//
// base.Integer with base.NoLeadingZeros is the first form of decinteger, and
// the prefixed rules with base.SeparatorAfterPrefix are the other three. The
// run of zeros has no base rule and is composed here. "007" lexes as the
// integers "00" and "7".
func integerRule() bytescan.Rule {
	afterPrefix := base.SeparatorAfterPrefix()
	zeros := core.RuneRun(core.RuneSet("0"))
	return core.Choice(
		base.Integer(digitSeparator, base.NoLeadingZeros()),
		core.Sequence(zeros, core.ZeroOrMore(core.Sequence(core.Literal("_"), zeros))),
		base.BinaryInteger(digitSeparator, afterPrefix),
		base.OctalInteger(digitSeparator, afterPrefix),
		base.HexInteger(digitSeparator, afterPrefix),
	)
}

// floatRule returns a rule for a floating-point literal (section 2.4.6):
//
//	floatnumber   ::= pointfloat | exponentfloat
//	pointfloat    ::= [digitpart] fraction | digitpart "."
//	exponentfloat ::= (digitpart | pointfloat) exponent
//	digitpart     ::= digit (["_"] digit)*
//	fraction      ::= "." digitpart
//	exponent      ::= ("e" | "E") ["+" | "-"] digitpart
//
// Python takes "1." and ".5", so base.Float takes both optional parts. The
// options in opts are added, for the suffix of imaginaryRule.
func floatRule(opts ...base.NumberOption) bytescan.Rule {
	return base.Float(append([]base.NumberOption{
		digitSeparator, base.OptionalIntegerPart(), base.OptionalFractionPart(),
	}, opts...)...)
}

// imaginaryRule returns a rule for an imaginary literal (section 2.4.7):
//
//	imagnumber ::= (floatnumber | digitpart) ("j" | "J")
//
// A digitpart takes leading zeros, so it is base.Integer without
// base.NoLeadingZeros: "09j" is one imaginary literal.
func imaginaryRule() bytescan.Rule {
	j := base.Suffixes("j", "J")
	return core.Choice(floatRule(j), base.Integer(digitSeparator, j))
}

// operatorRule returns a rule for an operator (section 2.5).
func operatorRule() bytescan.Rule {
	return core.AnyLiteral(strings.Fields(`
		+ - * ** / // % @ << >> & | ^ ~ :=
		< > <= >= == !=
	`)...)
}

// delimiterRule returns a rule for a delimiter (section 2.6). The filter
// tracks the brackets ( ) [ ] { } by their text.
func delimiterRule() bytescan.Rule {
	return core.AnyLiteral(strings.Fields(`
		( ) [ ] { } , : ! . ; = ->
		+= -= *= /= //= %= @= &= |= ^= >>= <<= **=
	`)...)
}
