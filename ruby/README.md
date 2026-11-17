# ruby

A Ruby scanner for a documented subset of the
[Ruby syntax documentation](https://docs.ruby-lang.org/en/3.3/syntax_rdoc.html),
mainly [literals](https://docs.ruby-lang.org/en/3.3/syntax/literals_rdoc.html)
(strings, percent literals, here documents), built from `rules/core` and
`rules/base`. It is a mode-driver example (see
[Modes](../docs/standard.md#modes-several-scanners-over-one-cursor)), with
the [Lua example](../lua) as its reference.

**Technique:** three constructs that no single rule set can lex, each lexed by
a scanner of its own over the same cursor:

- **Interpolation.** `#{` in a double-quoted string switches to code, and the
  `}` that balances it switches back, to any depth.
- **Delimiters chosen by the input.** The delimiter of a percent literal,
  `%w[ … ]` or `%q| … |`, is named by its opener, so its body scanner is built
  from that delimiter.
- **A token whose body starts later in the text.** A heredoc opener `<<~EOS`
  is followed by the rest of its line; its body starts on the next line and
  ends at the terminator line `EOS`.

```sh
go run ./ruby < ruby/testdata/document.input
```

## Tokens

| Type | Matches |
| --- | --- |
| `KEYWORD` | the 41 reserved words, `__ENCODING__` to `yield`, with `defined?` |
| `IDENTIFIER` | `[a-z_][A-Za-z0-9_]*`, with an optional `?` or `!` that no `=` follows: `empty?`, `save!` |
| `CONSTANT` | `[A-Z][A-Za-z0-9_]*` |
| `LABEL` | a name and `:`, when no second `:` follows: `key:` in `{key: 1}` |
| `INSTANCE_VARIABLE` | `@` and a name: `@name` |
| `CLASS_VARIABLE` | `@@` and a name: `@@count` |
| `GLOBAL_VARIABLE` | `$` and a name: `$stdout` |
| `SYMBOL` | `:` and a name, with an optional `?` or `!`: `:name`, `:Sym`, `:empty?` |
| `OPERATOR` | `! != !~ && \|\| &&= \|\|= + - * ** / % += -= *= **= /= %= & \| ^ ~ << >> &= \|= ^= <<= >>= = == === =~ < <= > >= <=> .. ... ? : :: -> => &.` |
| `PUNCTUATION` | `( ) [ ] , . ;` |
| `BRACE_OPEN`, `BRACE_CLOSE` | `{` and `}` in code |
| `INTEGER` | a decimal, `0x`, `0b` or `0o` integer, with `_` between digits: `1_000`, `0x1F` |
| `FLOAT` | digits on both sides of the point, an exponent, or both: `1.5`, `1e9`, `3_000.25` |
| `STRING` | a single-quoted string, which may span lines; a backslash escapes the rune after it |
| `STRING_OPEN`, `STRING_CLOSE` | the `"` that opens and closes a double-quoted string |
| `STRING_TEXT` | text of a double-quoted string, line breaks and escapes included |
| `INTERPOLATION_OPEN`, `INTERPOLATION_CLOSE` | `#{` in a double-quoted string, and the `}` that balances it |
| `PERCENT_OPEN` | `%w`, `%i`, `%q` or `%Q` and the delimiter: `%w[`, `%q(`, `%w\|` |
| `PERCENT_WORD` | a word of a `%w` or `%i` body: a run of runes other than layout and the delimiters; a backslash escapes the rune after it |
| `PERCENT_TEXT` | text of a `%q` or `%Q` body, line breaks and escapes included |
| `PERCENT_NEST_OPEN`, `PERCENT_NEST_CLOSE` | an inner bracket pair of a bracket-delimited body, such as the `[` and `]` of `%w[a [b]]` |
| `PERCENT_CLOSE` | the delimiter that closes a percent literal |
| `HEREDOC_OPEN` | `<<`, an optional `-` or `~`, and the identifier, bare or quoted: `<<EOS`, `<<-EOS`, `<<~'EOS'`, `<<"EOS"` |
| `HEREDOC_BODY` | the lines of a heredoc before its terminator line, line breaks included |
| `HEREDOC_END` | the terminator line without its line break: the identifier, after spaces and tabs for `<<-` and `<<~` |
| `COMMENT` | `#` and the rest of the line, in code |
| `WHITESPACE` | a run of space, tab, `\v` and `\f` |
| `NEWLINE` | one line break: `\n`, `\r\n` or `\r` |
| `UNKNOWN` | one rune that no rule of the current mode matches |

## The mode driver

`driver.go` runs on a `modes.Driver` over one cursor, with a stack of modes:

- **Code mode** (`newCodeScanner`) lexes every code token and the openers of the
  other modes. `{` pushes a brace frame and `}` pops it, so the driver knows
  which `}` ends an interpolation.
- **Interpolation mode** is code directly inside `#{`. Its scanner is a second
  `newCodeScanner`, in which `}` is `INTERPOLATION_CLOSE`. A `{` inside the
  interpolation pushes a brace frame, and code mode lexes up to its `}`.
- **String mode** (`newStringScanner`) lexes the inside of a double-quoted
  string: text, `#{` and the closing `"`.
- **Percent mode** (`newPercentScanner`) lexes the body of a percent literal.
  The driver reads the kind and the delimiter from `PERCENT_OPEN`. A bracket
  `(`, `[`, `{` or `<` closes at the matching bracket, and the driver counts
  the inner pairs: inside one, the closing bracket is `PERCENT_NEST_CLOSE`,
  so there is one scanner for depth 0 and one for the inner pairs. Any other
  delimiter closes at its next occurrence.
- **Heredoc mode** (`newHeredocScanner`) lexes one heredoc body and its
  terminator line. `HEREDOC_OPEN` adds the terminator to a queue and code goes
  on. At the next `NEWLINE` of code, the driver switches to the scanner of the
  first terminator in the queue. After `HEREDOC_END` it returns to code, whose
  `NEWLINE` ends the terminator line; if the queue still holds a terminator,
  its body follows. So several heredocs opened on one line are read in order.

Bracket counting in percent literals and brace counting in interpolation only
find the closing delimiter, as the Ruby scanner does. Nothing checks that the
program balances its brackets. The driver runs on a `modes.Driver`, and the
`modes` package applies the three rules of a mode driver:

1. **Release on every switch.** `modes.Driver` calls `Release()` on the
   outgoing scanner at every switch. An idle scanner that holds its checkpoints
   pins all later input in the shared cursor. `TestRelease` lexes 4 KiB of
   code after every mode over a cursor that retains at most 1 KiB.
2. **Cache one scanner per mode, delimiter and terminator.** The code, string
   and interpolation scanners are built once. A `modes.Cache` builds a percent
   scanner the first time the driver meets its kind, delimiter and depth (0 or
   inner), and a heredoc scanner the first time it meets its terminator and
   indent, and returns the same scanner afterwards. It never builds a scanner per
   token and never drops a scanner.
3. **One `Next()` at a time.** The scanners share one cursor, so no two of them
   may run at the same time. The `Next` of `modes.Driver` holds a mutex.

Every scanner returns a plain `io.EOF` at the end of the input, whatever its
mode. So only the driver's stack and queue know that the input ended inside a
string, a percent literal or a heredoc. The driver does not report that: the
construct is lexed to the end of the input, no closer follows, and lexing ends
normally.

## Covered subset

The tokens above. Not covered:

- **Regular expressions.** `/` is always `OPERATOR`; the TypeScript example
  shows the technique for a regular expression against a division.
- **Character literals** such as `?a`: `?` is `OPERATOR`.
- **Interpolation outside double-quoted strings.** `#{` is text in heredoc
  bodies and in `%Q` bodies, and `#@var` and `#$var` are text everywhere.
- **Other literals.** Backtick commands, `%x`, `%r`, `%s`, `%W`, `%I` and the
  bare `%( … )`; `:"quoted"` and operator symbols such as `:+`; quoted
  labels (`"a": 1`); rational and imaginary numbers (`3r`, `2i`); special
  globals such as `$0`, `$!` and `$1`.
- **Heredoc forms.** Backtick identifiers, and quoted identifiers that are not
  a plain name (`<<"A B"`). A terminator line with trailing spaces is a body
  line.
- **Values.** The body of `<<~` keeps its indentation, which Ruby removes, and
  escapes are not checked or converted.
- **`=begin`/`=end` comments and `__END__`.** They lex as code.
- **Names are ASCII.** A letter outside ASCII is `UNKNOWN`.
- **Context.** The Ruby scanner reads `a ?b :c`, `x <<y` and `a %w[b]` by the
  token before them; here the longest match decides. So `<<` directly followed
  by a name always opens a heredoc, `%w[` always opens a percent literal, and
  `:c` is always a symbol. `<< A`, with a space, is the operator.
- **A line break inside a token on a heredoc line.** The body starts after the
  first `NEWLINE` of code. When a string or a percent literal on the opener's
  line spans lines, the body starts after the line where that token ends.
- **No parsing.** Keywords used as method names (`x.class`) are `KEYWORD`;
  nothing checks that `do`, `end` or brackets balance.

## How invalid input lexes

- **An unterminated double-quoted string** is lexed to the end of the input
  in string mode, interpolations included; no `STRING_CLOSE` follows.
- **An unterminated percent literal** gives its body to the end of the input:
  in `%w[a [b c]` the line after it is lexed as words.
- **An unterminated heredoc** gives its body to the end of the input, with no
  `HEREDOC_END`. A second heredoc on the same line is still read after the
  first one ends.
- **Near-misses of a terminator** are body lines: `EO`, `EOSX`, `EOS ` (with a
  trailing space), `eos`, and ` EOS` (indented) for a bare `<<EOS`.
- **An unterminated single-quoted string** is not a `STRING`: its quote is
  `UNKNOWN`, and lexing goes on with the next rune.
- **Near-misses of a percent literal** are code: `% w[a]` is `OPERATOR "%"`,
  `IDENTIFIER "w"` and the brackets; `%x(ls)` is `OPERATOR "%"` and code.
  A backslash is not a delimiter, because it escapes the rune after it in a
  body: `%q\a\` is `OPERATOR "%"`, `IDENTIFIER "q"`, `UNKNOWN "\\"`,
  `IDENTIFIER "a"` and `UNKNOWN "\\"`.
- **Numbers take the longest form the grammar allows,** and the next rule
  takes the rest: `1__0` is `INTEGER "1"` and `IDENTIFIER "__0"`, `0b2` is
  `INTEGER "0"` and `IDENTIFIER "b2"`, `1.e3` is `INTEGER "1"`,
  `PUNCTUATION "."` and `IDENTIFIER "e3"`.
- **Characters outside the subset**, such as `` ` ``, `\`, a lone `@` or `$`,
  and non-ASCII letters, are `UNKNOWN`.

## Test data

| File | Shows |
| --- | --- |
| `document` | a small module with every technique: interpolation, `%w` and `%i`, a `<<~` heredoc with code after the opener |
| `tokens` | the 41 reserved words, names that contain one, every operator and punctuation mark |
| `names` | identifiers with `?` and `!`, `!=` after a name, constants, variables, symbols, labels, the ternary |
| `numbers` | integers with separators and prefixes, floats, `1.times` and ranges, near-misses |
| `strings` | single-quoted strings: escapes, no interpolation, across lines, unterminated |
| `interpolation` | `"a #{ "b #{c}" } d"`, empty and adjacent interpolations, braces and blocks inside, escapes, a string across lines, a comment inside |
| `percent` | `%w[a b [c] d]`, `%q(it's (nested))`, `%w\|a b\|`, `%i<…>` and `%Q{…}` with inner pairs, escaped delimiters and spaces, a body across lines |
| `heredocs` | a `<<~` heredoc with code after the opener, two heredocs on one line, quoted identifiers, an empty body |
| `terminators` | near-misses of a terminator, a tab-indented `<<-` terminator, blank body lines |
| `heredoc-at-end` | a terminator at the end of the input, with no line break |
| `unterminated-heredoc` | a heredoc that the input ends inside |
| `unterminated-second-heredoc` | two heredocs on one line, and the input ends inside the second |
| `unterminated-string` | a double-quoted string and an interpolation that the input ends inside |
| `unterminated-percent` | a percent literal that the input ends inside |
| `comments` | comments after code, without a space, and `#{` in a comment |
| `layout` | `\r\n` line breaks, also in a heredoc; tab, `\v` and `\f` |
| `not-ruby` | backticks, a regular expression, `?a`, special globals, `=begin`, other percent literals and a backslash delimiter, near-misses of a heredoc opener, `__END__` |
