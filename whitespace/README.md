# whitespace

A scanner for [Whitespace](https://compsoc.dur.ac.uk/whitespace/), the esoteric
language whose only tokens are space, tab and line feed. It is built only from
`rules/core`, and it is part of the [example standard](../README.md).

**Technique:** the inverted case. The usual skip rule is the only token rule
here: the three layout characters are the tokens, and all visible text is
comment. `COMMENT` is the complement of the three tokens, so the rules never
overlap and the registration order decides nothing.

```sh
go run ./whitespace < whitespace/testdata/program.input
```

Add `-drop-comments` to leave the `COMMENT` tokens out. That view is a filter
above `Next()` (see `withoutComments` in `main.go`).

## Tokens

| Type | Matches |
| --- | --- |
| `SPACE` | U+0020, one token per character |
| `TAB` | U+0009, one token per character |
| `LF` | U+000A, one token per character |
| `COMMENT` | a maximal run of every other rune, including `\r` and Unicode spaces such as U+00A0 and U+3000 |

`UNKNOWN` never appears: `COMMENT` takes every rune that the three tokens do
not take, so no rune is left over.

## Covered subset

The lexical alphabet of Whitespace 0.2: the three token characters, and every
other character as comment. Not covered:

- **Parsing.** `[Space][Space]` as a push instruction, and the other
  instruction forms, are grouping of tokens. The example does not group them.
- **Execution.** The example never runs a program.

## How invalid input lexes

Whitespace has no invalid input at the lexical level. Any text is a sequence of
tokens and comments, so a program that is wrong for the language still lexes
without error and the program exits 0. Visible text is comment, not an error:
`Done.` on a line of a program is a `COMMENT`, and a carriage return before a
line feed is a `COMMENT` before the `LF`.

## Test data

| File | Shows |
| --- | --- |
| `program` | a program of only the three characters |
| `hidden` | the same kind of program inside ordinary prose; words are comment and the spaces between them are tokens |
| `crlf` | `\r\n` line ends: the `\r` is a comment and the `\n` is `LF` |
| `unicode` | U+00A0, U+3000 and U+2003 (comment), and a non-ASCII letter (comment) |
| `empty` | empty input: no tokens |

Run `go test ./whitespace`. `TestDropComments` checks the
`-drop-comments` view against the same golden files, with their `COMMENT` lines
removed.
