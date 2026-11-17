# python

A scanner for a subset of Python 3.12, following chapter 2, "Lexical analysis",
of the [Python language reference](https://docs.python.org/3.12/reference/lexical_analysis.html).

**Technique:** a stateful filter above `Next()`. The scanner finds physical
tokens; the filter in `filter.go` applies the line structure of the reference
(sections 2.1.1 to 2.1.8). It keeps a stack of indentation levels and adds the
synthetic `INDENT` and `DEDENT` tokens, and it relabels the line break that
ends a logical line as `NEWLINE`.

```sh
go run ./python < python/testdata/program.input
```

## Tokens

| Type | Matches |
| --- | --- |
| `KEYWORD` | a hard keyword (section 2.3.1), such as `def`, `None` or `await` |
| `NAME` | an identifier (section 2.3); the soft keywords `match`, `case`, `type` and `_` are names |
| `STRING` | a string or bytes literal (section 2.4.1): an optional prefix, then a short (`'…'`, `"…"`) or a long (`'''…'''`, `"""…"""`) string; f-strings included |
| `INTEGER` | a decimal, binary (`0b`), octal (`0o`) or hexadecimal (`0x`) integer, with `_` between digits (section 2.4.5) |
| `FLOAT` | a point float (`1.5`, `1.`, `.5`) or an exponent float (`1e9`, `1.5e-3`) (section 2.4.6) |
| `IMAGINARY` | a float or a digit part followed by `j` or `J` (section 2.4.7) |
| `OPERATOR` | `+ - * ** / // % @ << >> & \| ^ ~ := < > <= >= == !=` (section 2.5) |
| `DELIMITER` | `( ) [ ] { } , : ! . ; = ->` and the augmented assignments `+=` to `**=` (section 2.6) |
| `ELLIPSIS` | `...` |
| `COMMENT` | `#` to the end of the physical line, without the line break |
| `LINE_JOIN` | a backslash and the line break after it: explicit line joining (section 2.1.5) |
| `NL` | a line break (`\n`, `\r\n` or `\r`) that does not end a logical line |
| `WHITESPACE` | a run of space, tab and formfeed |
| `UNKNOWN` | one rune that no rule matches |

The filter adds three types:

| Type | Text | Emitted |
| --- | --- | --- |
| `NEWLINE` | the line break, or empty at the end of input | at the end of each logical line |
| `INDENT` | empty | before the first token of a logical line that is deeper than the top of the stack |
| `DEDENT` | empty | before the first token of a logical line, once for each level that is deeper than the line; at the end of input, once for each open level |

A synthetic token has the position of the token after it, or of the end of
input. All layout is printed: the whitespace before the first token of a line
is what the filter measures.

## The filter

The filter reads one token at a time and keeps four things: the stack of
indentation levels, which starts as `[0]`; the number of open brackets; whether
the current logical line has a token; and the indentation of that line.

- The indentation is the width of the `WHITESPACE` at the start of the first
  physical line of a logical line. A space adds 1, a tab advances to the next
  multiple of 8, and a formfeed resets the width to 0 (section 2.1.8).
- At the first token of a logical line, other than layout, the filter compares
  the indentation with the top of the stack. A deeper line pushes its
  indentation and gets one `INDENT`. A shallower line pops every level deeper
  than itself and gets one `DEDENT` for each.
- An `NL` at bracket depth 0 that ends a line with a token becomes `NEWLINE`.
  An `NL` stays `NL` on a blank or comment-only line (section 2.1.7) and
  inside `()`, `[]` or `{}` (section 2.1.6). After `LINE_JOIN` the logical
  line continues, so the whitespace of the next physical line is not
  indentation.
- At the end of input the filter emits `NEWLINE` when the last logical line has
  a token and no line break, then one `DEDENT` for each level above 0.

The brackets are counted only to know whether a line break ends a logical line.
A closing bracket with no open one is a `DELIMITER` like any other, and the
count does not go below 0.

## Covered subset

All the tokens of chapter 2 except the ones below. Not covered:

- **f-string contents.** An f-string is one `STRING` token; its replacement
  fields are not lexed. Nesting the same quote inside a field, which Python
  allows since 3.12 (`f"{d["k"]}"`), ends the string at that quote. The
  [TypeScript example](../typescript) shows nested lexing.
- **Bad indentation is not an error.** A dedent to a column that matches no
  level on the stack pops every deeper level, with one `DEDENT` each, and
  continues at the nearest lower level; Python reports an `IndentationError`.
  The column is not pushed, so the next line with the same indentation is
  deeper than the top of the stack and gets an `INDENT`. Mixed tabs and spaces
  and unbalanced brackets are not reported either.
- **Identifiers** are `base.UnicodeIdentifier`: Unicode letters, letter
  numbers and `_`, then those and marks, decimal digits and connector
  punctuation. The `Other_ID_Start` and `Other_ID_Continue` runes are left out,
  and names are not NFKC-normalized.
- **The escape sequences** inside a string are not checked: a backslash takes
  the next rune, whatever it is. Raw strings lex the same way, as in Python,
  so `r"\""` is one string.
- **t-strings** (Python 3.14), the encoding declaration and a byte order mark
  are not covered. `$`, `?` and `` ` `` are `UNKNOWN`.
- **Tokens the filter does not add:** Python's tokenizer also emits
  `ENDMARKER` and the f-string parts; this example does not.

## How invalid input lexes

The number grammars are strict, and the longest match decides the rest:

- `007` lexes as `INTEGER "00"` and `INTEGER "7"`; `0x` as `INTEGER "0"` and
  `NAME "x"`; `1__0` as `INTEGER "1"` and `NAME "__0"`.
- `1e` lexes as `INTEGER "1"` and `NAME "e"`; `1.e` as `FLOAT "1."` and
  `NAME "e"`.
- `1if x` lexes as `INTEGER "1"` and `KEYWORD "if"`, as Python's tokenizer
  splits it; `1..2` as `FLOAT "1."` and `FLOAT ".2"`.

A short string that ends at a line break is not a `STRING`: its opening quote
is `UNKNOWN`, and lexing continues with the next rune. An unterminated long
string `'''…` lexes as the empty string `''`, an `UNKNOWN "'"` and the text
after it. A long string ends at the first triple quote, so `"""x""""` is a
`STRING` and an `UNKNOWN "\""`. A backslash that no line break follows is
`UNKNOWN`. Prefixes that Python does not allow, such as `ub` or `ur`, lex as a
`NAME` and a `STRING`.

## Test data

| File | Shows |
| --- | --- |
| `program` | a small module with every common token and nested blocks |
| `blocks` | nested blocks, a dedent of two levels and of one level, blank lines between blocks |
| `brackets` | bracketed expressions that span lines, with a comment, a blank line and any indentation inside |
| `continuation` | backslash continuation, a continuation line at column 0 inside a block, and a backslash before a space |
| `crlf` | `\r\n` line breaks: a logical line end, a continuation, a blank line and a long string that spans lines; `.gitattributes` keeps the file byte for byte |
| `blank-lines` | blank, whitespace-only and comment-only lines at several columns inside a block |
| `tabs` | a tab, eight spaces, spaces then a tab and a formfeed then a tab are one level |
| `eof-in-block` | the end of input inside three levels, with no final line break |
| `bad-dedent` | a dedent to a column between two levels |
| `string-prefixes` | every string and bytes prefix, escaped quotes, an escaped line break, and prefixes Python does not allow |
| `triple-quoted` | long strings that span lines, with quotes inside, an escaped quote before the end, the empty long string and a fourth quote |
| `fstring` | f-strings as one token, with fields, conversions, `{{`, a raw f-string and a nested quote |
| `numbers` | each form of the integer, float and imaginary grammars |
| `bad-numbers` | leading zeros, misplaced `_`, empty or wrong digits after a prefix, a missing exponent, a number before a keyword or a name |
| `names-keywords` | every hard keyword, the soft keywords, names that contain keywords, and Unicode names |
| `operators` | every operator and delimiter, and the ellipsis |
| `unterminated` | short strings cut by a line break and a long string cut by the end of input |
| `not-python` | runes outside the language and a lone backslash |

`scanner_test.go` also checks that `INDENT` and `DEDENT` are balanced in every
case, and the width of tabs and formfeeds.
