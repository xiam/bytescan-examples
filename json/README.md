# json

A JSON scanner for [RFC 8259](https://www.rfc-editor.org/rfc/rfc8259), built
from `rules/core` and the string and number rules of `rules/base`. It is the
reference example of the [example standard](../docs/standard.md).

**Technique:** a strict token grammar from a string with checked escapes
(`base.QuotedWith`) and base number options, and letting the longest match
split input that the grammar does not allow.

```sh
go run ./json < json/testdata/document.input
```

## Tokens

| Type | Matches |
| --- | --- |
| `BEGIN_ARRAY`, `END_ARRAY` | `[` and `]` |
| `BEGIN_OBJECT`, `END_OBJECT` | `{` and `}` |
| `NAME_SEPARATOR`, `VALUE_SEPARATOR` | `:` and `,` |
| `TRUE`, `FALSE`, `NULL` | the literal names, case-sensitive |
| `STRING` | `"…"` with the escapes `\" \\ \/ \b \f \n \r \t \uXXXX`; no control character (U+0000 to U+001F) inside |
| `NUMBER` | `-?(0\|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?` |
| `WHITESPACE` | a run of space, tab, line feed and carriage return |
| `UNKNOWN` | one rune that no rule matches |

## Covered subset

All JSON tokens. Not covered:

- **Validation.** The scanner does not check nesting, commas, names or duplicate
  names; `]{,` lexes as three structural tokens.
- **Surrogate pairing.** `\uD800` alone is a `STRING`: the shape of `\uXXXX`
  is checked, not whether a high surrogate is followed by a low one.
- **A byte order mark** is `UNKNOWN`.

## How invalid input lexes

The number grammar is strict and the longest match decides the rest:

- `01` lexes as `NUMBER "0"` and `NUMBER "1"`.
- `1.` lexes as `NUMBER "1"` and `UNKNOWN "."`; `1e` as `NUMBER "1"` and
  `UNKNOWN "e"`.
- A lone `-`, and the `+` of `+1`, are `UNKNOWN`.

A string that does not end on its line is not a `STRING`: its opening quote is
`UNKNOWN`, and lexing continues with the next rune. So `"\x"` lexes as `UNKNOWN`
tokens for `"`, `\` and `x`, and the closing quote can then open a new string.

## Test data

| File | Shows |
| --- | --- |
| `document` | a small document with every token type |
| `escapes` | every escape, `\uXXXX` in both cases, a surrogate pair, a lone surrogate, non-ASCII text and malformed escapes |
| `numbers` | each form of the number grammar |
| `bad-numbers` | leading zeros, a missing fraction or exponent, a lone sign, a leading `+`, hex |
| `unterminated` | a string cut by a line break, a raw tab, and the end of input |
| `not-json` | single quotes, wrong case, bare names, a comment and other characters |
