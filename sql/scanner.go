package main

import (
	"bufio"
	"io"
	"unicode"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/rules/base"
	"github.com/xiam/bytescan/rules/core"
)

// The token types, named after the tokens of ISO/IEC 9075-2:2016, clause 5.
const (
	keyword             bytescan.TokenType = "KEYWORD"
	identifier          bytescan.TokenType = "IDENTIFIER"
	delimitedIdentifier bytescan.TokenType = "DELIMITED_IDENTIFIER"
	stringLit           bytescan.TokenType = "STRING"
	nationalString      bytescan.TokenType = "NATIONAL_STRING"
	binaryString        bytescan.TokenType = "BINARY_STRING"
	exactNumeric        bytescan.TokenType = "EXACT_NUMERIC"
	approximateNumeric  bytescan.TokenType = "APPROXIMATE_NUMERIC"
	comment             bytescan.TokenType = "COMMENT"
	operator            bytescan.TokenType = "OPERATOR"
	punctuation         bytescan.TokenType = "PUNCTUATION"
	whitespace          bytescan.TokenType = "WHITESPACE"
	newline             bytescan.TokenType = "NEWLINE"
)

// reservedWords is the subset of the <reserved word> list of ISO/IEC
// 9075-2:2016, 5.2 that the scanner knows. A word that is not here, such as the
// non-reserved ASC, DESC, KEY and LIMIT, lexes as an IDENTIFIER, as the
// standard allows.
var reservedWords = []string{
	"ALL", "ALTER", "AND", "ANY", "AS",
	"BETWEEN", "BIGINT", "BOOLEAN", "BY",
	"CASE", "CAST", "CHAR", "CHARACTER", "CHECK", "CONSTRAINT", "COUNT", "CREATE", "CROSS",
	"DATE", "DECIMAL", "DEFAULT", "DELETE", "DISTINCT", "DROP",
	"ELSE", "END", "ESCAPE", "EXCEPT", "EXISTS",
	"FALSE", "FETCH", "FLOAT", "FOR", "FOREIGN", "FROM", "FULL",
	"GROUP", "HAVING",
	"IN", "INNER", "INSERT", "INTEGER", "INTERSECT", "INTO", "IS",
	"JOIN", "LEFT", "LIKE",
	"NATURAL", "NOT", "NULL", "NUMERIC",
	"OFFSET", "ON", "ONLY", "OR", "ORDER", "OUTER",
	"PRIMARY", "REFERENCES", "RIGHT", "ROWS",
	"SELECT", "SET", "SMALLINT", "SUM",
	"TABLE", "THEN", "TIME", "TIMESTAMP", "TRUE",
	"UNION", "UNIQUE", "UPDATE", "USING",
	"VALUES", "VARCHAR", "WHEN", "WHERE", "WITH",
}

// The runes of a regular identifier (ISO/IEC 9075-2:2016, 5.2):
//
//	<identifier start>  Lu Ll Lt Lm Lo Nl
//	<identifier extend> U+00B7 Mn Mc Nd Pc Cf
//
// An identifier part is a start or an extend rune.
var (
	identStart = core.UnicodeTables(unicode.L, unicode.Nl)
	identPart  = identStart.
			Or(core.UnicodeTables(unicode.Mn, unicode.Mc, unicode.Nd, unicode.Pc, unicode.Cf)).
			Or(core.RuneSet("·"))
)

// newScanner returns a scanner for standard SQL that reads r. The token types
// follow the token names of ISO/IEC 9075-2:2016, clause 5.
//
// The registration order breaks one tie: a reserved word is also a regular
// identifier of the same length, so KEYWORD is registered before IDENTIFIER
// and wins. No other two rules match the same text; the longest match decides
// the rest, so N'a' is one NATIONAL_STRING rather than the identifier N, "<>"
// is one operator, and "--" starts a comment rather than two minus signs.
func newScanner(r io.Reader) *bytescan.Scanner {
	lx := bytescan.New(bufio.NewReader(r))

	// A reserved word matches without regard to case and only when no
	// identifier part follows (ISO/IEC 9075-2:2016, 5.2, <reserved word>):
	// (?i:SELECT|FROM|...)(?![identifier part]). The longest word wins, so
	// "ORDER" wins over "OR" on "order"; on "orders" and "selection" an
	// identifier part follows every word, and the rule matches nothing.
	lx.MustAddRule(keyword, core.KeywordsIgnoreCaseWithGuard(identPart, reservedWords...))
	lx.MustAddRule(identifier, base.Identifier(identStart, identPart))
	lx.MustAddRule(delimitedIdentifier, delimitedIdentifierRule())

	lx.MustAddRule(stringLit, base.QuotedMultiline('\'', '\''))
	lx.MustAddRule(nationalString, core.Sequence(core.Rune(core.RuneSet("Nn")), base.QuotedMultiline('\'', '\'')))
	lx.MustAddRule(binaryString, binaryStringRule())

	lx.MustAddRule(exactNumeric, exactNumericRule())
	lx.MustAddRule(approximateNumeric, approximateNumericRule())

	// <simple comment> and <bracketed comment> are both a <comment>.
	lx.MustAddRule(comment, core.Choice(base.LineComment("--"), base.BlockComment("/*", "*/")))

	lx.MustAddRule(operator, core.AnyLiteral("<>", "<=", ">=", "||", "=", "<", ">", "+", "-", "*", "/"))
	lx.MustAddRule(punctuation, core.AnyLiteral("(", ")", ",", ";", "."))

	lx.MustAddRule(whitespace, base.Whitespace())
	lx.MustAddRule(newline, base.Newline())

	return lx
}

// delimitedIdentifierRule returns a rule for a delimited identifier
// (ISO/IEC 9075-2:2016, 5.2):
//
//	"(?:""|[^"])+"
//
// A doubled quote stands for one quote, and the body may span lines. The body
// is not empty, so "" is not an identifier, and base.QuotedMultiline, which
// takes an empty body, does not fit.
func delimitedIdentifierRule() bytescan.Rule {
	quote := core.Literal(`"`)
	part := core.Choice(core.Literal(`""`), core.Rune(core.RuneSet(`"`).Not()))
	return core.Sequence(quote, core.OneOrMore(part), quote)
}

// binaryStringRule returns a rule for a binary string literal in its hex form
// (ISO/IEC 9075-2:2016, 5.3):
//
//	[Xx]'(?:[0-9A-Fa-f]{2})*'
//
// The hex digits come in pairs. The standard also allows spaces between the
// digits; this rule does not. No base rule counts the runes of a body in pairs,
// so the rule is composed here.
func binaryStringRule() bytescan.Rule {
	quote := core.Literal("'")
	pair := core.Repeat(core.Rune(core.ASCIIHexDigit), 2, 2)
	return core.Sequence(core.Rune(core.RuneSet("Xx")), quote, core.ZeroOrMore(pair), quote)
}

// exactNumericRule returns a rule for an exact numeric literal
// (ISO/IEC 9075-2:2016, 5.3):
//
//	[0-9]+(?:\.[0-9]*)?|\.[0-9]+
//
// "1." and ".5" are exact numerics, so base.Decimal takes both parts as
// optional.
func exactNumericRule() bytescan.Rule {
	return core.Choice(base.Integer(), base.Decimal(base.OptionalIntegerPart(), base.OptionalFractionPart()))
}

// approximateNumericRule returns a rule for an approximate numeric literal
// (ISO/IEC 9075-2:2016, 5.3): an exact numeric mantissa, then E and a signed
// integer exponent:
//
//	(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)[Ee][+-]?[0-9]+
func approximateNumericRule() bytescan.Rule {
	return core.Sequence(exactNumericRule(), base.Exponent("Ee"))
}
