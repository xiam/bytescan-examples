package main

import (
	"bufio"
	"io"
	"unicode"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/rules/base"
	"github.com/xiam/bytescan/rules/core"
)

// The token types. The literal types are named after the productions of the
// Go specification (identifier, int_lit, float_lit, imaginary_lit, rune_lit,
// string_lit); the specification gives no names to single operators, so they
// share one type.
const (
	keyword      bytescan.TokenType = "KEYWORD"
	identifier   bytescan.TokenType = "IDENTIFIER"
	operator     bytescan.TokenType = "OPERATOR"
	semicolon    bytescan.TokenType = "SEMICOLON"
	intLit       bytescan.TokenType = "INT_LIT"
	floatLit     bytescan.TokenType = "FLOAT_LIT"
	imaginaryLit bytescan.TokenType = "IMAGINARY_LIT"
	runeLit      bytescan.TokenType = "RUNE_LIT"
	stringLit    bytescan.TokenType = "STRING_LIT"
	comment      bytescan.TokenType = "COMMENT"
	whitespace   bytescan.TokenType = "WHITESPACE"
	newline      bytescan.TokenType = "NEWLINE"
)

// keywords are the 25 keywords of the Go specification, section "Keywords".
var keywords = []string{
	"break", "case", "chan", "const", "continue",
	"default", "defer", "else", "fallthrough", "for",
	"func", "go", "goto", "if", "import",
	"interface", "map", "package", "range", "return",
	"select", "struct", "switch", "type", "var",
}

// operators are the operators and punctuation of the Go specification,
// section "Operators and punctuation", except ";", which has its own type.
var operators = []string{
	"+", "&", "+=", "&=", "&&", "==", "!=", "(", ")",
	"-", "|", "-=", "|=", "||", "<", "<=", "[", "]",
	"*", "^", "*=", "^=", "<-", ">", ">=", "{", "}",
	"/", "<<", "/=", "<<=", "++", "=", ":=", ",",
	"%", ">>", "%=", ">>=", "--", "!", "...", ".", ":",
	"&^", "&^=", "~",
}

// newScanner returns a scanner for the tokens of the Go specification, section
// "Lexical elements", that reads r. It inserts no semicolon; newSemicolons
// does that above Next.
//
// One tie is broken by registration order: a keyword is also an identifier of
// the same length, so KEYWORD is registered before IDENTIFIER. Everywhere else
// the longest match decides: "breakx" is an identifier, ".5" a float and not
// the operator ".", "1i" an imaginary literal and not the integer "1", "//" a
// comment and not the operator "/".
func newScanner(r io.Reader) *bytescan.Scanner {
	lx := bytescan.New(bufio.NewReader(r))

	lx.MustAddRule(keyword, core.Keywords(keywords...))
	lx.MustAddRule(identifier, identifierRule())

	lx.MustAddRule(operator, core.AnyLiteral(operators...))
	lx.MustAddRule(semicolon, core.Literal(";"))

	lx.MustAddRule(intLit, intRule())
	lx.MustAddRule(floatLit, floatRule())
	lx.MustAddRule(imaginaryLit, imaginaryRule())
	lx.MustAddRule(runeLit, runeRule())
	lx.MustAddRule(stringLit, stringRule())

	lx.MustAddRule(comment, commentRule())

	// The specification's white space is space, tab, carriage return and
	// newline. A newline can end a statement, so it gets its own type, and
	// "\r\n" is WHITESPACE "\r" and NEWLINE "\n", as go/scanner reads it.
	lx.MustAddRule(whitespace, core.RuneRun(core.RuneSet(" \t\r")))
	lx.MustAddRule(newline, core.Literal("\n"))

	return lx
}

// commentRule returns a rule for a comment (section "Comments"): a line
// comment from "//" to the end of the line, without the newline, or a general
// comment from "/*" to the first "*/":
//
//	//[^\n]*|/\*(?s:.*?)\*/
//
// A line comment ends only at a newline, so a carriage return inside it is
// comment text. base.LineComment also stops at "\r", so the line comment is
// composed from core; the general comment is base.BlockComment.
func commentRule() bytescan.Rule {
	line := core.Sequence(core.Literal("//"), core.ZeroOrMore(core.Rune(core.RuneSet("\n").Not())))
	return core.Choice(line, base.BlockComment("/*", "*/"))
}

// identifierRule returns a rule for an identifier (section "Identifiers"):
//
//	identifier = letter { letter | unicode_digit } .
//	letter     = unicode_letter | "_" .
//
// unicode_letter is the category L and unicode_digit the category Nd.
// base.UnicodeIdentifier also takes Nl, Mn, Mc and Pc runes, so the Go form is
// composed from base.Identifier.
func identifierRule() bytescan.Rule {
	letter := core.UnicodeLetter.Or(core.RuneSet("_"))
	return base.Identifier(letter, letter.Or(core.UnicodeTables(unicode.Nd)))
}

var (
	// separator is the "_" that Go allows between two digits, and
	// afterPrefix also allows it after a radix prefix ("0x_FF").
	separator   = base.DigitSeparator('_')
	afterPrefix = base.SeparatorAfterPrefix()
	// optionalParts lets a decimal or hex float leave out the digits before
	// or after the point ("1.", ".5", "0x.8p1").
	optionalParts = []base.NumberOption{base.OptionalIntegerPart(), base.OptionalFractionPart()}
)

// intRule returns a rule for an integer literal (section "Integer literals"):
//
//	int_lit     = decimal_lit | binary_lit | octal_lit | hex_lit .
//	decimal_lit = "0" | ( "1" … "9" ) [ [ "_" ] decimal_digits ] .
//	binary_lit  = "0" ( "b" | "B" ) [ "_" ] binary_digits .
//	octal_lit   = "0" [ "o" | "O" ] [ "_" ] octal_digits .
//	hex_lit     = "0" ( "x" | "X" ) [ "_" ] hex_digits .
//
// base.Integer with base.NoLeadingZeros is decimal_lit, and the prefixed base
// rules with base.SeparatorAfterPrefix are the other forms with a prefix.
func intRule() bytescan.Rule {
	return core.Choice(base.Integer(separator, base.NoLeadingZeros()), legacyOctalRule(), prefixedRule())
}

// prefixedRule returns the binary, octal and hexadecimal integers with a
// radix prefix, with the options in opts added.
func prefixedRule(opts ...base.NumberOption) bytescan.Rule {
	opts = append([]base.NumberOption{separator, afterPrefix}, opts...)
	return core.Choice(base.BinaryInteger(opts...), base.OctalInteger(opts...), base.HexInteger(opts...))
}

// legacyOctalRule returns a rule for the octal_lit without "o" or "O", such
// as "017" or "0_17": "0" [ "_" ] octal_digits. It is composed here because
// base.OctalInteger takes only the 0o prefix.
func legacyOctalRule() bytescan.Rule {
	digits := core.RuneRun(core.ASCIIOctalDigit)
	return core.Sequence(
		core.Literal("0"), core.Optional(core.Literal("_")),
		digits, core.ZeroOrMore(core.Sequence(core.Literal("_"), digits)),
	)
}

// floatRule returns a rule for a floating-point literal (section
// "Floating-point literals"):
//
//	float_lit         = decimal_float_lit | hex_float_lit .
//	decimal_float_lit = decimal_digits "." [ decimal_digits ] [ decimal_exponent ] |
//	                    decimal_digits decimal_exponent |
//	                    "." decimal_digits [ decimal_exponent ] .
//	decimal_exponent  = ( "e" | "E" ) [ "+" | "-" ] decimal_digits .
//	hex_float_lit     = "0" ( "x" | "X" ) hex_mantissa hex_exponent .
//	hex_mantissa      = [ "_" ] hex_digits "." [ hex_digits ] |
//	                    [ "_" ] hex_digits |
//	                    "." hex_digits .
//	hex_exponent      = ( "p" | "P" ) [ "+" | "-" ] decimal_digits .
//
// base.Float and base.HexFloat with both parts optional are the two forms.
// The options in opts are added, for the suffix of imaginaryRule.
func floatRule(opts ...base.NumberOption) bytescan.Rule {
	decimal := append([]base.NumberOption{separator}, optionalParts...)
	hex := append([]base.NumberOption{separator, afterPrefix}, optionalParts...)
	return core.Choice(
		base.Float(append(decimal, opts...)...),
		base.HexFloat(append(hex, opts...)...),
	)
}

// imaginaryRule returns a rule for an imaginary literal (section "Imaginary
// literals"):
//
//	imaginary_lit = (decimal_digits | int_lit | float_lit) "i" .
//
// decimal_digits takes leading zeros, so it is base.Integer without
// base.NoLeadingZeros, and it also covers the decimal_lit and the octal_lit
// without a prefix: "0123i" is one imaginary literal.
func imaginaryRule() bytescan.Rule {
	i := base.Suffixes("i")
	return core.Choice(base.Integer(separator, i), prefixedRule(i), floatRule(i))
}

// escape returns a rule for the escapes of section "Rune literals" that a rune
// or a string literal takes, with quote as the one quote that escaped_char
// allows ("'" in a rune literal, `"` in a string literal):
//
//	byte_value       = octal_byte_value | hex_byte_value .
//	octal_byte_value = `\` octal_digit octal_digit octal_digit .
//	hex_byte_value   = `\` "x" hex_digit hex_digit .
//	little_u_value   = `\` "u" hex_digit hex_digit hex_digit hex_digit .
//	big_u_value      = `\` "U" hex_digit hex_digit hex_digit hex_digit
//	                           hex_digit hex_digit hex_digit hex_digit .
//	escaped_char     = `\` ( "a" | "b" | "f" | "n" | "r" | "t" | "v" | `\` | quote ) .
//
// The value is not checked: "\400" and "\U00110000" match.
func escape(quote string) bytescan.Rule {
	hex := func(n int) bytescan.Rule { return core.Repeat(core.Rune(core.ASCIIHexDigit), n, n) }
	return core.Sequence(
		core.Literal(`\`),
		core.Choice(
			core.Repeat(core.Rune(core.ASCIIOctalDigit), 3, 3),
			core.Sequence(core.Literal("x"), hex(2)),
			core.Sequence(core.Literal("u"), hex(4)),
			core.Sequence(core.Literal("U"), hex(8)),
			core.Rune(core.RuneSet(`abfnrtv\`+quote)),
		),
	)
}

// runeRule returns a rule for a rune literal (section "Rune literals"):
//
//	rune_lit      = "'" ( unicode_value | byte_value ) "'" .
//	unicode_value = unicode_char | little_u_value | big_u_value | escaped_char .
//
// unicode_char is any rune but a newline; an unescaped "'" or `\` would end or
// start something else, so neither is a unicode_char here. base.QuotedWith
// takes any number of items and a rune literal holds one, so the rule is
// composed here.
func runeRule() bytescan.Rule {
	char := core.Rune(core.RuneSet("'\\\n").Not())
	return core.Sequence(core.Literal("'"), core.Choice(char, escape("'")), core.Literal("'"))
}

// stringRule returns a rule for a string literal (section "String literals"):
//
//	string_lit             = raw_string_lit | interpreted_string_lit .
//	raw_string_lit         = "`" { unicode_char | newline } "`" .
//	interpreted_string_lit = `"` { unicode_value | byte_value } `"` .
//
// An interpreted string stops at a newline; a raw string spans lines and has
// no escapes.
func stringRule() bytescan.Rule {
	interpreted := base.QuotedWith('"', core.RuneSet("\"\\\n").Not(), escape(`"`))
	return core.Choice(base.RawMultiline("`", "`"), interpreted)
}
