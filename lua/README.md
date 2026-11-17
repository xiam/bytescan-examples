# lua

A Lua scanner for the lexical conventions of the
[Lua 5.4 reference manual, section 3.1](https://www.lua.org/manual/5.4/manual.html#3.1),
built from `rules/core` and `rules/base`. It is the first mode-driver example
(see [Modes](../docs/standard.md#modes-several-scanners-over-one-cursor)).

**Technique:** a counted match. A long bracket `[==[ … ]==]` closes only at the
level of its opener, so no single rule set can lex every level. A mode driver
switches to a scanner built for the level of the opener, over the same cursor.

```sh
go run ./lua < lua/testdata/document.input
```

## Tokens

| Type | Matches |
| --- | --- |
| `KEYWORD` | the 22 reserved words: `and break do else elseif end false for function goto if in local nil not or repeat return then true until while` |
| `NAME` | `[A-Za-z_][A-Za-z0-9_]*` that is not a reserved word |
| `OPERATOR` | `+ - * / // % ^ # & ~ \| << >> == ~= <= >= < > ..` |
| `DELIMITER` | `= ( ) { } [ ] :: ; : , . ...` |
| `NUMERAL` | a decimal or hexadecimal integer or float: `3`, `0xff`, `3.`, `.5`, `314.16e-2`, `0xA23p-4`, `0x.8` |
| `STRING` | a short string in `"…"` or `'…'` on one line; a backslash escapes the rune after it |
| `COMMENT` | a short comment: `--` and the rest of the line, when no opening long bracket follows `--` |
| `LONG_STRING_OPEN` | an opening long bracket: `[`, n times `=`, `[` |
| `LONG_STRING` | the body of a long string, line breaks included |
| `LONG_STRING_CLOSE` | the closing long bracket of the opener's level: `]`, n times `=`, `]` |
| `LONG_COMMENT_OPEN` | `--` and an opening long bracket |
| `LONG_COMMENT` | the body of a long comment |
| `LONG_COMMENT_CLOSE` | the closing long bracket of the opener's level |
| `WHITESPACE` | a run of space, tab, `\v` and `\f` |
| `NEWLINE` | one line break: `\n`, `\r\n` or `\r` |
| `UNKNOWN` | one rune that no rule of the current mode matches |

## The mode driver

`driver.go` runs the scanners over one cursor on a
[`modes.Driver`](https://github.com/xiam/bytescan/tree/master/modes), the public mode driver:

- **Code mode** (`newCodeScanner`) lexes every token above and the two openers.
- **Level mode** (`newLevelScanner`) lexes the inside of one long bracket: one
  body token and the closer. Its closer is the literal `]`, n times `=`, `]`,
  so the driver builds one level scanner for each kind (string or comment) and
  level it meets.

After `LONG_STRING_OPEN` or `LONG_COMMENT_OPEN`, the driver counts the `=` of
the opener and switches to the level scanner for that kind and level. After the
closer it switches back to code mode. The `modes` package applies the three
rules of a mode driver:

1. **Release on every switch.** `modes.Driver` calls `Release()` on the
   outgoing scanner at every switch. An idle scanner that holds its checkpoints
   pins all later input in the shared cursor.
2. **Cache one scanner per level.** A `modes.Cache` builds a level scanner the
   first time the driver meets a kind and level, and returns the same scanner
   afterwards. It never builds a scanner per token and never drops a scanner; a
   scanner that is dropped must be released first.
3. **One `Next()` at a time.** The scanners share one cursor, so no two of them
   may run at the same time. The `Next` of `modes.Driver` holds a mutex.

Every scanner returns a plain `io.EOF` at the end of the input, whatever its
mode. So only the driver knows that the input ended inside a long bracket. It
does not report that: the body runs to the end of the input, no closer
follows, and lexing ends normally.

## Covered subset

All tokens of section 3.1. Not covered:

- **Escapes are not checked.** In a short string a backslash takes the rune
  after it, whatever it is: `"\q"` is a `STRING`. `\x`, `\u{…}` and decimal
  escapes are not checked for their digits.
- **An escaped line break and `\z` across lines.** Lua allows `\` followed by a
  line break, and `\z` followed by line breaks, inside a short string. Here a
  short string is one line, so such a string does not match (see below).
- **Values.** The first line break after an opener is part of the body, as it
  is in the text; Lua drops it from the value. Numerals are not converted.
- **Names are ASCII.** A letter outside ASCII is `UNKNOWN`.
- **A first line that starts with `#`** (a shebang) is lexed as code.
- **No parsing.** `goto`, labels and attributes such as `<const>` are lexed as
  their tokens (`KEYWORD`, `DELIMITER "::"`, `OPERATOR "<"`); nothing checks
  how they are used.

## How invalid input lexes

- **A short string that does not end on its line** is not a `STRING`: its
  quote is `UNKNOWN`, and lexing goes on with the next rune. So
  `"unterminated` lexes as `UNKNOWN "\""` and `NAME "unterminated"`.
- **An unterminated long bracket** gives the opener and a body to the end of
  the input, with no closer.
- **A closer of another level** is part of the body: `[==[ ]] ]=] ]===] ]==]`
  is one body `" ]] ]=] ]===] "` and the closer `]==]`.
- **`--[` without a full opener** is a short comment: `--[ x`, `--[= x` and
  `--[==x` are each one `COMMENT`.
- **Near-misses of a long bracket** in code mode are delimiters: `[=b` is
  `DELIMITER "["`, `DELIMITER "="` and `NAME "b"`; `]]` is two `DELIMITER "]"`.
- **Numerals take the longest form the grammar allows,** and the next rule
  takes the rest. Lua rejects these as malformed numbers; here `1..2` lexes as
  `NUMERAL "1."` and `NUMERAL ".2"`, `3x` as `NUMERAL "3"` and `NAME "x"`,
  `0x` as `NUMERAL "0"` and `NAME "x"`, and `1e+` as `NUMERAL "1"`,
  `NAME "e"` and `OPERATOR "+"`.
- **Characters outside Lua**, such as `!`, `$`, `@`, `` ` `` and `?`, are
  `UNKNOWN`. `!=` is `UNKNOWN "!"` and `DELIMITER "="`.

## Test data

| File | Shows |
| --- | --- |
| `document` | a small module with every kind of token |
| `tokens` | the 22 reserved words, every operator and delimiter, names that contain a reserved word |
| `numerals` | each numeral form: decimal and hex, integer and float, with exponents, `3.`, `.5`, `0x.8`, `0xA.` |
| `bad-numerals` | `1..2`, `0x`, `0xg`, `3e`, `1e+`, `0x1p`, `3x`, `.e1`, `08` |
| `strings` | short strings in both quotes, escapes, non-ASCII text |
| `bad-strings` | short strings cut by a line break, and an escaped line break |
| `long-strings` | long strings at levels 0, 1 and 3, an empty one, a second level 0 from the cached scanner, one that spans lines |
| `closers` | long strings that contain closers of a lower and of a higher level, and an opener of another level |
| `long-comments` | long comments on one line, across lines, empty, and after code |
| `short-comments` | `--[`, `--[=`, `--[==x` and `-- [[` as short comments; an empty comment |
| `unterminated-string` | a long string that the input ends inside |
| `unterminated-comment` | a long comment that the input ends inside |
| `layout` | `\r\n` line breaks, also inside a long string; tab, `\v` and `\f` |
| `not-lua` | a shebang, `!=`, `&&`, a non-ASCII name, characters outside Lua, near-misses of a long bracket |
