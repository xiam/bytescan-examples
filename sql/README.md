# sql

A scanner for the lexical elements of standard SQL, after
[ISO/IEC 9075-2:2016](https://www.iso.org/standard/63556.html), clause 5
(5.2 `<token>` and `<separator>`, 5.3 `<literal>`). The public BNF of
[SQL-92](https://ronsavage.github.io/SQL/sql-92.bnf.html) and
[SQL:2003](https://ronsavage.github.io/SQL/sql-2003-2.bnf.html) gives the same
forms. It follows the [example standard](../docs/standard.md).

**Technique:** keywords that ignore case, competing with identifiers. A
reserved word matches in any letter case, never as the start of a longer
identifier, and wins over the identifier rule by registration order.

```sh
go run ./sql < sql/testdata/query.input
```

## Tokens

| Type | Matches |
| --- | --- |
| `KEYWORD` | a word of the reserved-word list below, in any letter case, not followed by an identifier part |
| `IDENTIFIER` | a regular identifier: a letter (`Lu Ll Lt Lm Lo Nl`), then letters, `Mn Mc Nd Pc Cf` and `·` (U+00B7) |
| `DELIMITED_IDENTIFIER` | `"…"` with `""` for a quote; not empty; may span lines |
| `STRING` | a character string literal `'…'` with `''` for a quote; may be empty and may span lines |
| `NATIONAL_STRING` | `N'…'` or `n'…'`, the body as for `STRING` |
| `BINARY_STRING` | `X'…'` or `x'…'` with pairs of hex digits, the hex form of a binary string literal |
| `EXACT_NUMERIC` | `[0-9]+(\.[0-9]*)?` or `\.[0-9]+`: `42`, `1.5`, `1.`, `.5` |
| `APPROXIMATE_NUMERIC` | an exact numeric, `E` or `e`, and a signed integer: `1e10`, `1.5E-3` |
| `COMMENT` | a simple comment `--` to the end of the line, or a bracketed comment `/* … */` |
| `OPERATOR` | `=` `<>` `<` `>` `<=` `>=` `+` `-` `*` `/` `\|\|` |
| `PUNCTUATION` | `(` `)` `,` `;` `.` |
| `WHITESPACE` | a run of space, tab, vertical tab and form feed |
| `NEWLINE` | one line break: `\r\n`, `\n` or `\r` |
| `UNKNOWN` | one rune that no rule matches |

### Reserved words

The list is a subset of the `<reserved word>` list of ISO/IEC 9075-2:2016,
5.2, written by hand. Each word is reserved in the SQL:2016 column of the
[PostgreSQL key word table](https://www.postgresql.org/docs/current/sql-keywords-appendix.html),
a public copy of the classification:

`ALL ALTER AND ANY AS BETWEEN BIGINT BOOLEAN BY CASE CAST CHAR CHARACTER CHECK
CONSTRAINT COUNT CREATE CROSS DATE DECIMAL DEFAULT DELETE DISTINCT DROP ELSE END
ESCAPE EXCEPT EXISTS FALSE FETCH FLOAT FOR FOREIGN FROM FULL GROUP HAVING IN
INNER INSERT INTEGER INTERSECT INTO IS JOIN LEFT LIKE NATURAL NOT NULL NUMERIC
OFFSET ON ONLY OR ORDER OUTER PRIMARY REFERENCES RIGHT ROWS SELECT SET SMALLINT
SUM TABLE THEN TIME TIMESTAMP TRUE UNION UNIQUE UPDATE USING VALUES VARCHAR WHEN
WHERE WITH`

A non-reserved key word of the standard, such as `ASC`, `DESC`, `KEY`,
`FIRST` or `LIMIT`, can be an identifier, so it lexes as `IDENTIFIER`.

### Registration order

`KEYWORD` is registered before `IDENTIFIER`. A reserved word is also a regular
identifier of the same length, and when two rules match the same length, the
rule registered first wins. That is the only tie: on `selection` the keyword
rule matches nothing, so the identifier rule takes the whole word. Everywhere
else the longest match decides: `N'a'` is one `NATIONAL_STRING`, not the
identifier `N` and a string; `<>` is one operator; `--` starts a comment, not
two minus signs.

The keyword rule is `core.KeywordsIgnoreCaseWithGuard`, with the reserved
words and an identifier part as the guard: a word counts only where no
identifier part follows it, and the longest such word matches. On `order` the
end after `or` is followed by `d` and does not count, and `ORDER` matches.

## Covered subset

The tokens of the table above. Not covered:

- **Validation.** The scanner does not check that a statement is SQL; `SELECT
  FROM ,` lexes as two keywords and a comma.
- **Most reserved words.** Only the list above; another reserved word lexes as
  `IDENTIFIER`.
- **Nested comments.** A bracketed comment ends at the first `*/`. The
  standard nests them; on `/* /* */ */` the comment is `/* /* */`, and the rest
  is the operators `*` and `/`.
- **String continuation.** `'a' 'b'` with a separator between the parts is one
  literal in the standard; here it is two `STRING` tokens.
- **Spaces inside a binary string**, which the standard allows between hex
  digits.
- **Other literal forms:** bit strings `B'…'` (SQL-92), Unicode strings and
  identifiers `U&'…'` and `U&"…"`, introducers `_charset'…'`, and the datetime
  and interval literals as one token: `DATE '2024-01-01'` is a keyword and a
  string.
- **Other delimiters:** `[` `]` for arrays, `::`, `->`, `=>`, `?` and the
  `:name` of host parameters.
- **Dialect extensions:** backtick identifiers, `$$` strings, `#` comments,
  `E'…'` strings, `!=`, bind parameters such as `$1` and `@v`.

Letter case uses Unicode simple case folding, as `core.LiteralIgnoreCase` does,
so `ſelect`, with a long s (U+017F), is the keyword `SELECT`.

## How invalid input lexes

- A word that starts with a reserved word is one `IDENTIFIER`: `selection`,
  `select_x`, `select1`, `orders`.
- `_x` is `UNKNOWN "_"` and the identifier `x`: an identifier starts with a
  letter. `1x` is the number `1` and the identifier `x`.
- An unterminated string, delimited identifier or bracketed comment matches
  nothing. Its first rune is `UNKNOWN` (`'`, `"`) or an operator (`/` and `*`
  of `/*`), and lexing continues with the next rune. In `'O''Brien` with no
  closing quote, `'O'` is a `STRING` and the next `'` is `UNKNOWN`.
- `""` is not an identifier: its first quote is `UNKNOWN`, and the second one
  opens a delimited identifier that ends at the next quote.
- `X'ABC'` (odd) and `X'GG'` are the identifier `X` and a `STRING`; so is
  `N 'x'`, with a space after the prefix.
- `1e` and `1e+` are the number `1`, the identifier `e` and the operator `+`.
  `1.2.3` is `1.2` and `.3`; `1..2` is `1.` and `.2`; `0x1F` is `0` and the
  identifier `x1F`. A sign is an operator: `-5` is `-` and `5`.
- `1--2` is the number `1` and the comment `--2`.
- A rune outside the language, such as `!`, `?`, `` ` ``, `$`, `@` or `#`, is
  `UNKNOWN`; `!=` is `UNKNOWN "!"` and the operator `=`.

## Test data

| File | Shows |
| --- | --- |
| `query` | a realistic script: `CREATE TABLE`, `INSERT` and a `SELECT` with a join, comments and literals |
| `keywords` | keywords in mixed case; identifiers that start with a keyword (`selection`, `select_x`, `selectä`, `select·x`); `or` against `order`; a keyword next to a quote or a bracket; non-reserved words; `ſelect` |
| `identifiers` | regular identifiers in several scripts, `_x`, delimited identifiers with `""`, a keyword in quotes, a line break inside, and the empty `""` |
| `strings` | empty, escaped and multi-line strings; national and binary strings in both cases; odd and non-hex binary strings; a space after the prefix; two adjacent strings |
| `numbers` | each exact and approximate form, and near-misses |
| `operators` | every operator and punctuation mark, operators without spaces, and near-misses |
| `layout` | tab, vertical tab, form feed, a lone `\r`, an empty line and no final line break; a `\r\n` would fail `git diff --check`, so it is left out |
| `comments` | simple and bracketed comments, `/***/`, a nested-looking comment, `---`, a comment at end of input |
| `unterminated-string` | a string with no closing quote after `''` |
| `unterminated-identifier` | a delimited identifier with no closing quote |
| `unterminated-comment` | a bracketed comment with no `*/` |
| `not-sql` | dialect forms and characters outside the language |
