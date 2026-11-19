# highlight

A syntax highlighter for a text that changes. It colors a small language of
words, numbers, operators, strings and block comments. Then it applies a list
of edits, and after each edit it repaints only the tokens that the edit
changed. The language is rule set A of bytescan's
[resumable lexing](https://github.com/xiam/bytescan/blob/master/docs/specs/resumable-lexing.md#rule-set-a)
page.

**Technique:** the lexer with state,
[`statelex`](https://pkg.go.dev/github.com/xiam/bytescan/statelex). You tell
it which bytes changed. It tells you which tokens changed. The highlighter
keeps its own list of painted tokens and updates it from that answer alone.
The guide [Lexing text that changes](https://github.com/xiam/bytescan/blob/master/docs/guides/text-that-changes.md)
explains the lexer with state.

```sh
go run ./highlight -edits highlight/testdata/backspaces.edits highlight/testdata/backspaces.input
```

Add `-color` to see the colors in a terminal:

```sh
go run ./highlight -color < highlight/testdata/document.input
```

## Tokens

| Type | Matches |
| --- | --- |
| `WORD` | one or more ASCII letters |
| `NUMBER` | one or more digits, then optionally `.` and one or more digits |
| `OP` | `==`, or one of `=`, `+`, `*`, `/`, `.` |
| `STRING` | `"`, then any characters except `"` and a line break, then `"` |
| `COMMENT` | `/*`, then anything up to the next `*/`, across lines |
| `WS` | one or more spaces |
| `NL` | one line break |
| `UNKNOWN` | one character that no rule matches |

## Edits

The `-edits` file lists one edit per line: a start offset, an end offset, and
the text that replaces the bytes between them, as a Go quoted string. The
offsets count bytes in the text as it is before that edit. Leave out the text
to delete. Lines that start with `#` are comments.

```text
# type an opening quote at offset 4
4 4 "\""
# delete the bytes from offset 12 up to offset 13: a backspace
12 13
# replace the first 12 bytes
0 12 "say \"hi\"\n"
```

## Output

First the program loads the whole text, which is one edit of an empty
lexer. Then it applies each edit. For each step it prints:

- the change: the old range and the new range of the changed bytes, and how
  far the rest of the text moved;
- a `-` line for each token the edit removed, and a `+` line for each token it
  painted;
- the painted text.

The backspace that turns `alpha` into `alph`:

```text
edit 1: delete [12,13)
change [8,13) -> [8,12), shift -1
- WORD [8,13) "alpha"
+ WORD [8,12) "alph"
  total = alph + beta
  Wwwww O Wwww O Wwww
```

Without `-color`, a ruler under each line shows the paint. Each character has
the letter of its token type: `w` word, `n` number, `o` operator, `s` string,
`c` comment, `?` unknown. The letter is upper case on the first character of
a token, so you can see where each token starts. Spaces and line breaks are
not painted. With `-color`, the text is printed in ANSI colors instead.

## How the highlighter works

The highlighter in the package [paint](paint/paint.go) holds a list of
tokens: a type and a byte range each. The [repl](../repl) example paints its
line with the same package. For each change from the lexer it:

1. removes the tokens in the changed range;
2. adds the new tokens;
3. moves every later token by the shift.

It reads nothing else from the lexer. It never sees a rule.

## Covered subset

All of rule set A. Not covered:

- **A user interface.** The edits come from a file, not from the keyboard.
- **Painting only the visible part** of a long text.
- **Escapes in strings.** A string has none, and it ends on its line.

## How invalid input lexes

- A `"` that is not closed on its line is `UNKNOWN`, and the text after it
  lexes as usual. When the closing quote arrives, the whole string is
  repainted as one `STRING`.
- A `/*` with no `*/` after it is two `OP` tokens, `/` and `*`. When a `*/`
  arrives, everything between is repainted as one `COMMENT`.
- `1.` is `NUMBER "1"` and `OP "."`.
- A character outside the language, such as `@` or a tab, is `UNKNOWN`.

## Test data

Each case is a text, `<case>.input`, and a list of edits, `<case>.edits`. The
expected output is `<case>.golden`.

| Case | Shows |
| --- | --- |
| `document` | every token type, a comment over two lines and broken input; no edits |
| `backspaces` | ten backspaces, one after another |
| `selection` | a selection deleted in the middle of a line |
| `chunk` | a chunk deleted at the beginning of the text |
| `paste` | select all, then paste |
| `string` | a string that opens, closes, and opens again |
| `comment` | a comment that opens, closes across a line, and opens again |

`document.color.golden` holds the `-color` output of `document`.

The tests also check that, after every edit, the highlighter's tokens are the
same as those of a new highlighter that paints the final text from the start
(`TestRepaint`). `TestRepaintGenerated` does the same for 300 generated edits.
