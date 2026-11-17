# go

A scanner for Go source code that follows the
[Go specification](https://go.dev/ref/spec), sections "Lexical elements" and
"Semicolons". It is built only from `rules/core` and `rules/base`, and it
follows the [example standard](../docs/standard.md).

**Technique:** a token filter above `Next()` that inserts the semicolons of
the specification, and a differential test against a reference tokenizer,
`go/scanner` from the standard library.

```sh
go run ./go < go/testdata/program.go.input
```

## Tokens

| Type | Matches |
| --- | --- |
| `KEYWORD` | the 25 keywords; registered before `IDENTIFIER`, which matches them too |
| `IDENTIFIER` | a letter (`\pL` or `_`), then letters and decimal digits (`\p{Nd}`) |
| `OPERATOR` | every operator and punctuation of the specification but `;` |
| `SEMICOLON` | `;` in the source, and each semicolon the filter inserts |
| `INT_LIT` | decimal, binary (`0b`), octal (`0o` and `0`) and hex (`0x`) integers, with `_` between digits |
| `FLOAT_LIT` | decimal floats (`1.5`, `1.`, `.5`, `1e9`) and hex floats (`0x1p-2`), with `_` between digits |
| `IMAGINARY_LIT` | a decimal digit run, an integer or a float, then `i` |
| `RUNE_LIT` | `'x'` with one character or one escape (`\a`, `\'`, `\377`, `\xff`, `ዤ`, `\U00101234`, ...) |
| `STRING_LIT` | an interpreted string `"..."` on one line, with the escapes of a rune literal but `\"` for `\'`; a raw string `` `...` `` over any number of lines |
| `COMMENT` | a line comment `// ...` up to its newline (a carriage return in it is comment text), and a general comment `/* ... */` |
| `WHITESPACE` | a run of space, tab and carriage return |
| `NEWLINE` | one line feed |
| `UNKNOWN` | one rune that no rule matches |

The literal types are named after the productions of the specification
(`int_lit`, `rune_lit`, ...). The specification gives no names to single
operators, so they share `OPERATOR`; the text tells them apart.

## The semicolon filter

`newSemicolons` wraps the scanner and implements rule 1 of the section
"Semicolons": after a line's final token, it inserts a `SEMICOLON` if that
token is an identifier, a literal, one of the keywords `break`, `continue`,
`fallthrough` and `return`, or one of `++ -- ) ] }`. A line ends at a
`NEWLINE`, at a general comment that contains a newline, and at the end of the
input. Comments, layout and `UNKNOWN` tokens are not tokens of the Go
specification, so they do not change which token is the final one.

The inserted semicolon has empty text and comes just before the token that
ends the line, at its position; at the end of the input it comes at the end of
the last token. The output marks it:

```text
1:8 IDENTIFIER "main"
1:12 SEMICOLON "" inserted
1:12 NEWLINE "\n"
```

The filter follows rule 1 only. Rule 2 lets a program leave out a semicolon
before `)` or `}`; that is for a parser, and the filter inserts nothing there.
The filter never looks at the structure of the program.

## The differential test

`TestScanner` lexes each `testdata/*.go.input` file twice: with the example
(after the filter, comments and layout removed) and with `go/scanner` (comments
skipped). The two streams must be equal: the same token type, by a table from
`token.Token` in the test, the same text, at the same byte offset. An inserted
semicolon must be at the same place in the stream; `go/scanner` puts it at the
newline, the comment or the end of the input, so its offset is not compared.
`TestScannerCoverage` checks that the Go inputs together contain every keyword,
every operator and each literal form, so the comparison covers them all.

`go/scanner` and `go/token` are used by the test only; the program does not
import them.

## Covered subset

All tokens of the section "Lexical elements" and rule 1 of "Semicolons". Not
covered:

- **Validation.** The scanner does not parse or check a program; `) ( }` lexes
  as three operators.
- **Literal values.** The shape of an escape is checked, its value is not:
  `'\400'` and `'\uD800'` are `RUNE_LIT`.
- **Carriage returns in raw strings.** `go/scanner` drops them from the
  literal; the token keeps the source text.
- **A byte order mark** at the start of the input is `UNKNOWN`; `go/scanner`
  skips it.
- **Invalid UTF-8.** Each byte that does not decode is one `UNKNOWN` token
  whose text is U+FFFD.

## How invalid input lexes

`go/scanner` reports an error and still returns a token for most invalid input.
This scanner reports nothing: the longest match takes the valid part, and a rune
that starts no token is `UNKNOWN`.

- A string, rune or raw string that does not end is not a literal: its opening
  quote is `UNKNOWN`, and the text after it lexes as code. A general comment
  that does not end is the operators `/` and `*`.
- An escape that the specification does not define, such as `"\q"`, and a rune
  literal with no character or with two (`''`, `'ab'`) split into `UNKNOWN`
  quotes and backslashes around identifiers.
- A number keeps its valid prefix: `089` is `INT_LIT "0"` and `INT_LIT "89"`,
  `0x` is `INT_LIT "0"` and `IDENTIFIER "x"`, `1_` is `INT_LIT "1"` and
  `IDENTIFIER "_"`, `0x1p` is `INT_LIT "0x1"` and `IDENTIFIER "p"`.
- `#`, `$`, `?`, `@` and `\` outside a literal are `UNKNOWN`.

An `UNKNOWN` token does not change which Go token is a line's final one, so
the filter still inserts semicolons on a broken line.

## Test data

Files named `*.go.input` are valid Go, checked against `go/scanner` and by
their golden files. Files named `*.input` only are invalid Go, checked by their
golden files only. The `.input` extension keeps the Go tool from compiling
them.

| File | Shows |
| --- | --- |
| `program.go` | a small program: a package, imports, types, methods, comments and a Unicode identifier |
| `keywords.go` | every keyword and every operator, in statements |
| `literals.go` | each form of the integer, float, imaginary, rune and string literals |
| `semicolons.go` | each kind of final token, comments at the end of a line, a carriage return inside a line comment, a general comment over two lines, `;` in the source, and an input that ends without a newline |
| `unterminated` | a string, a rune, a raw string and a general comment that do not end |
| `bad-literals` | near-misses of rune, string and number literals |
| `not-go` | runes outside the language: `#`, `$`, `?`, `@` and `\` |
