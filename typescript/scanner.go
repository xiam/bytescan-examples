package main

import (
	"bufio"
	"io"
	"unicode"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/rules/base"
	"github.com/xiam/bytescan/rules/core"
	"github.com/xiam/textreader"
)

// The token types. The names follow the ECMAScript lexical grammar where it
// names the token (Punctuator, DivPunctuator, RegularExpressionLiteral). The
// specification lexes a template as NoSubstitutionTemplate, TemplateHead,
// TemplateMiddle and TemplateTail; this example splits those into their
// delimiters and their text, so that the driver can change mode at each
// delimiter.
const (
	keyword           bytescan.TokenType = "KEYWORD"
	identifier        bytescan.TokenType = "IDENTIFIER"
	punctuator        bytescan.TokenType = "PUNCTUATOR"
	divPunctuator     bytescan.TokenType = "DIV_PUNCTUATOR"
	number            bytescan.TokenType = "NUMBER"
	stringLiteral     bytescan.TokenType = "STRING"
	regex             bytescan.TokenType = "REGEX"
	comment           bytescan.TokenType = "COMMENT"
	whitespace        bytescan.TokenType = "WHITESPACE"
	newline           bytescan.TokenType = "NEWLINE"
	templateOpen      bytescan.TokenType = "TEMPLATE_OPEN"
	templateText      bytescan.TokenType = "TEMPLATE_TEXT"
	templateExprOpen  bytescan.TokenType = "TEMPLATE_EXPR_OPEN"
	templateExprClose bytescan.TokenType = "TEMPLATE_EXPR_CLOSE"
	templateClose     bytescan.TokenType = "TEMPLATE_CLOSE"
)

// newScanner returns a TypeScript scanner that reads r: a mode driver over one
// cursor, with the code-mode scanners (where "/" is a division or starts a
// regular expression, and where "}" closes a substitution or not) and a
// template-mode scanner.
func newScanner(r io.Reader) *driver {
	return newDriver(textreader.New(bufio.NewReader(r)))
}

// keywords are the reserved words of ECMAScript 2024, section 12.7.2, the
// words reserved in strict mode (let, static, implements, interface, package,
// private, protected, public), and the common TypeScript keywords that are not
// ECMAScript words. Contextual words such as async, of, get and set are
// identifiers here.
var keywords = []string{
	// ECMAScript ReservedWord.
	"await", "break", "case", "catch", "class", "const", "continue",
	"debugger", "default", "delete", "do", "else", "enum", "export",
	"extends", "false", "finally", "for", "function", "if", "import", "in",
	"instanceof", "new", "null", "return", "super", "switch", "this",
	"throw", "true", "try", "typeof", "var", "void", "while", "with",
	"yield",
	// Reserved in strict mode.
	"let", "static", "implements", "interface", "package", "private",
	"protected", "public",
	// TypeScript.
	"abstract", "as", "declare", "keyof", "namespace", "readonly",
	"satisfies", "type",
}

// identifierStart and identifierPart approximate IdentifierStartChar and
// IdentifierPartChar (section 12.7): a Unicode letter or letter number, "$"
// and "_" start an identifier; marks, decimal digits, connector punctuation,
// ZWNJ and ZWJ may also continue it.
var (
	identifierStart = core.UnicodeTables(unicode.L, unicode.Nl).Or(core.RuneSet("$_"))
	identifierPart  = identifierStart.Or(core.UnicodeTables(unicode.Mn, unicode.Mc, unicode.Nd, unicode.Pc)).Or(core.RuneSet("‌‍"))
)

// newCodeScanner returns the scanner for code mode over cur. The ECMAScript
// specification has two goal symbols for code: InputElementDiv, where "/" and
// "/=" are DivPunctuator, and InputElementRegExp, where "/" starts a
// RegularExpressionLiteral (section 12). With k.regexGoal false the scanner
// takes the first goal, with k.regexGoal true the second. With k.exprClose
// "}" is TEMPLATE_EXPR_CLOSE, the end of a substitution (the
// InputElementTemplateTail goal), and otherwise a PUNCTUATOR. The driver in
// driver.go picks one of the code scanners before each token.
//
// One tie is broken by registration order: KEYWORD is registered before
// IDENTIFIER, because a keyword also matches the identifier rule. The longest
// match decides every other overlap: "//" and "/*" are COMMENT, not
// DIV_PUNCTUATOR "/" (a REGEX never starts with "/" or "*"), ".5" is NUMBER,
// not PUNCTUATOR ".", and "?." is one PUNCTUATOR unless a digit follows it.
func newCodeScanner(cur bytescan.Cursor, k codeKey) *bytescan.Scanner {
	lx := bytescan.NewWithCursor(cur)

	lx.MustAddRule(keyword, core.KeywordsWithGuard(identifierPart, keywords...))
	lx.MustAddRule(identifier, base.Identifier(identifierStart, identifierPart))

	lx.MustAddRule(punctuator, punctuatorRule(!k.exprClose))
	if k.exprClose {
		lx.MustAddRule(templateExprClose, core.Literal("}"))
	}
	if k.regexGoal {
		lx.MustAddRule(regex, regexRule())
	} else {
		lx.MustAddRule(divPunctuator, core.AnyLiteral("/", "/="))
	}

	lx.MustAddRule(number, numberRule())
	lx.MustAddRule(stringLiteral, core.Choice(base.Quoted('"', '\\'), base.Quoted('\'', '\\')))
	lx.MustAddRule(templateOpen, core.Literal("`"))

	lx.MustAddRule(comment, core.Choice(base.LineComment("//"), base.BlockComment("/*", "*/")))

	// WhiteSpace is tab, \v, \f, space and the Unicode space separators; this
	// example takes the ASCII ones. LineTerminator is \n, \r, LS and PS; this
	// example takes \n, \r and \r\n.
	lx.MustAddRule(whitespace, base.Whitespace())
	lx.MustAddRule(newline, base.Newline())

	return lx
}

// punctuatorRule returns a rule for Punctuator and RightBracePunctuator
// (section 12.8), and "@", which starts a decorator; with closeBrace false it
// leaves out "}". "?." is OptionalChainingPunctuator only when no decimal
// digit follows it, so "a?.5:b" is "?" and ".5":
//
//	\?\.(?![0-9])|L1|L2|...
//
// where L1, L2, ... are the literals listed below; the longest match wins.
func punctuatorRule(closeBrace bool) bytescan.Rule {
	literals := []string{
		"{", "(", ")", "[", "]", ".", "...", ";", ",", "@",
		"<", ">", "<=", ">=", "==", "!=", "===", "!==",
		"+", "-", "*", "%", "**", "++", "--",
		"<<", ">>", ">>>", "&", "|", "^", "!", "~",
		"&&", "||", "??", "?", ":",
		"=", "+=", "-=", "*=", "%=", "**=", "<<=", ">>=", ">>>=",
		"&=", "|=", "^=", "&&=", "||=", "??=", "=>",
	}
	if closeBrace {
		literals = append(literals, "}")
	}
	return core.Choice(
		core.NotFollowedBy(core.Literal("?."), core.Rune(core.ASCIIDigit)),
		core.AnyLiteral(literals...),
	)
}

// regexRule returns a rule for RegularExpressionLiteral (section 12.9.5): a
// body between two "/" and the flags. A backslash escapes the rune after it,
// and a "/" inside a class "[…]" does not end the body. No base rule has a
// class inside its body that the closer may not end, so it is composed here. The body is not
// checked against the pattern grammar, and the flags are not checked.
//
//	/(?:[^*\\/\[\n\r]|\\[^\n\r]|\[(?:[^\]\\\n\r]|\\[^\n\r])*\])(?:[^\\/\[\n\r]|\\[^\n\r]|\[(?:[^\]\\\n\r]|\\[^\n\r])*\])*/[identifierPart]*
//
// The first rune of the body is not "*" or "/", so "/*" and "//" are comments.
func regexRule() bytescan.Rule {
	lineBreak := core.RuneSet("\n\r")
	backslash := core.Sequence(core.Literal(`\`), core.Rune(lineBreak.Not()))
	class := core.Sequence(
		core.Literal("["),
		core.ZeroOrMore(core.Choice(core.Rune(core.RuneSet(`]\`).Or(lineBreak).Not()), backslash)),
		core.Literal("]"),
	)
	first := core.Choice(core.Rune(core.RuneSet(`*\/[`).Or(lineBreak).Not()), backslash, class)
	rest := core.Choice(core.Rune(core.RuneSet(`\/[`).Or(lineBreak).Not()), backslash, class)
	return core.Sequence(
		core.Literal("/"),
		first,
		core.ZeroOrMore(rest),
		core.Literal("/"),
		core.ZeroOrMore(core.Rune(identifierPart)),
	)
}

// numberRule returns a rule for NumericLiteral (section 12.9.3): a decimal
// number with an optional fraction and exponent, a binary, octal or
// hexadecimal integer, or a BigInt (an integer with the suffix "n"). A "_"
// may separate two digits.
//
//	D = [0-9](?:_?[0-9])*
//	D(?:\.D?)?(?:[eE][+-]?D)?|\.D(?:[eE][+-]?D)?
//	(?:D|0[xX]H|0[oO]O|0[bB]B)n
//	0[xX]H|0[oO]O|0[bB]B
//
// where H, O and B are the hexadecimal, octal and binary digit runs with
// separators. base.Float takes "3." and ".5" with both parts optional, and
// base.Suffixes adds the "n" of a BigInt.
func numberRule() bytescan.Rule {
	sep := base.DigitSeparator('_')
	n := base.Suffixes("", "n")
	return core.Choice(
		base.Integer(sep, n),
		base.Float(sep, base.OptionalIntegerPart(), base.OptionalFractionPart()),
		base.HexInteger(sep, n), base.OctalInteger(sep, n), base.BinaryInteger(sep, n),
	)
}

// newTemplateScanner returns the scanner for template mode over cur: the text of
// a template literal (TemplateCharacters, section 12.9.6), the "${" that opens
// a substitution, and the "`" that closes the template.
//
// The text is a run of runes other than "`", "\" and "$", of "$" when no "{"
// follows it, and of a backslash and the rune after it, line breaks included:
//
//	(?:[^`\\$]|\$(?!\{)|\\(?s:.))+
//
// So "\`" and "\${" are text. Escapes are not checked. No two rules match the
// same text: the text stops before "`" and before "${". The text is
// base.EscapedRun with the stop rune "`" and the opener "${".
func newTemplateScanner(cur bytescan.Cursor) *bytescan.Scanner {
	lx := bytescan.NewWithCursor(cur)
	lx.MustAddRule(templateText, base.EscapedRun(core.RuneSet("`"), '\\', "${"))
	lx.MustAddRule(templateExprOpen, core.Literal("${"))
	lx.MustAddRule(templateClose, core.Literal("`"))
	return lx
}
