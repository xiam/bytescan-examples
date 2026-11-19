# repl

A prompt loop, or REPL, that follows the line as you type it. Each key
changes the line, and the program shows which tokens changed and paints the
line. When you press Enter, it runs the line, or it waits for one more line
when a string or a comment is still open. The language has words, numbers,
operators, block comments and strings that can span lines. It follows the
REPL of bytescan's
[resumable lexing](https://github.com/xiam/bytescan/blob/master/docs/specs/resumable-lexing.md#the-repl)
page.

**Technique:** the lexer with state,
[`statelex`](https://pkg.go.dev/github.com/xiam/bytescan/statelex), as a
REPL. Each key is one edit, and the lexer answers with one change. On Enter,
two questions to the lexer decide between "run" and "continue". The REPL has
no code of its own that looks for an open string or comment. The guide
[Lexing text that changes](https://github.com/xiam/bytescan/blob/master/docs/guides/text-that-changes.md#an-open-string-or-comment-at-the-end)
explains the two questions.

```sh
go run ./repl < repl/testdata/string.input
```

Add `-color` to paint the line with ANSI colors instead of a ruler.

## Type key by key in a terminal

A terminal normally sends a whole line at once, after Enter. The standard
`stty` command makes it send each key as you press it, and stops it from
showing the keys, because the program shows them:

```sh
stty -icanon -echo; go run ./repl -color; stty sane
```

Press Ctrl-C to stop. `stty sane` gives the terminal back its usual
settings. If the terminal still does not show what you type, type
`stty sane` and press Enter.

## Tokens

| Type | Matches |
| --- | --- |
| `WORD` | one or more ASCII letters |
| `NUMBER` | one or more digits, then optionally `.` and one or more digits |
| `OP` | `==`, or one of `=`, `+`, `*`, `/`, `.` |
| `COMMENT` | `/*`, then anything up to the next `*/`, across lines |
| `QUOTE` | `"`, which opens or closes a string |
| `TEXT` | the characters of a string, up to a `"` or a line break |
| `WS` | one or more spaces |
| `NL` | one line break, inside or outside a string |
| `UNKNOWN` | one character that no rule matches |

## Modes

The language has two modes. Outside a string, the mode is `code`. A `QUOTE`
opens the mode `str`, and the next `QUOTE` closes it. In `str`, the
characters are `TEXT`, and a line break is its own `NL` token. So a line that
ends inside a string ends in the mode `str`.

## Keys

The program reads standard input as a list of keys:

| Key | What it does |
| --- | --- |
| a printable character | one edit: type it at the end of the entry |
| backspace, the byte `0x7F` or `0x08` | one edit: delete the last character of the entry |
| Enter, a line break | one edit: type the line break; then run or continue |
| anything else, such as a tab or an escape | skipped |

The entry is everything typed since the last run. A backspace deletes a
whole character, even one of several bytes, such as `é`. On an empty entry,
it does nothing.

## Output

For each key the program prints the key and its edit, the change of the
tokens, and the painted entry, as the [highlight](../highlight) example
does. A session that opens a string on one line and closes it on the next,
with some keys left out (the case `string` in `testdata`):

```text
>
key 's': insert "s" at 0
change [0,0) -> [0,1), shift +1
+ WORD [0,1) "s"
  s
  W
(9 more keys)
key enter: insert "\n" at 10
change [10,10) -> [10,11), shift +1
+ NL [10,11) "\n"
  say "hello
  Www QSssss
continue: the text ends in the mode str
...
key 'w': insert "w" at 11
(5 more keys)
key enter: insert "\n" at 17
change [17,17) -> [17,18), shift +1
+ NL [17,18) "\n"
  say "hello
  Www QSssss
  world"
  SssssQ
run
1:0 WORD "say"
1:3 WS " "
1:4 QUOTE "\""
1:5 TEXT "hello"
1:10 NL "\n"
2:0 TEXT "world"
2:5 QUOTE "\""
2:6 NL "\n"
>
end of input
```

- `>` waits for a new entry, and `...` waits for one more line of the same
  entry.
- `continue:` says why the entry is not complete.
- `run` prints the tokens of the entry, one per line, as the other examples
  do: the line, the column, the type and the text.
- In the ruler, each character has the letter of its token type: `w` word,
  `n` number, `o` operator, `q` quote, `s` text, `c` comment, `?` unknown.

When the input ends inside an entry, the program prints why the entry was
not finished, then its tokens.

## How the REPL decides

On Enter, the REPL asks the lexer two questions
([repl.go](repl.go), `open`):

1. Is there a token that needs more input? `Lexer.Unfinished` answers. A
   comment with no `*/` is one: its rule read to the end of the text.
2. Is the mode at the end of the text the mode of an empty entry?
   `Lexer.ModeAtEnd` answers. After `say "`, the mode is `str`: the string
   is open.

If either answer says "open", the REPL continues. Otherwise it runs. The
line `say "` shows why the second question is needed: its three tokens are
complete, so the first question says nothing is open.

The paint of the entry comes from the highlighter of the
[highlight](../highlight) example, in the package
[`highlight/paint`](../highlight/paint). It updates the paint from each
change and reads nothing else from the lexer.

## Covered subset

The language above, typed at the end of the line. Not covered:

- **Moving the cursor** inside the line. Every key types or deletes at the
  end.
- **History**, and any key that is not in the table of keys.
- **Running the entry.** The REPL prints its tokens. It does not evaluate
  them.
- **Checking that an entry is complete as a statement**, such as balanced
  brackets. That needs a parser.
- **Escapes in strings.**

## How invalid input lexes

- A `/*` with no `*/` after it is two `OP` tokens, `/` and `*`, until a `*/`
  arrives. Then everything between is one `COMMENT`.
- `1.` is `NUMBER "1"` and `OP "."`.
- A character outside the language, such as `@` or `é`, is `UNKNOWN`.

## Test data

Each case is a list of keys, `<case>.input`, and the expected output,
`<case>.golden`. The keys are built in [repl_test.go](repl_test.go), where a
backspace can be read.

| Case | Shows |
| --- | --- |
| `backspaces` | ten backspaces at the end of a line: ten small changes |
| `string` | a string opened on one line and closed on the next: continue, then run |
| `quote` | the line `say "`: three complete tokens, continue because of the mode |
| `comment` | a comment that is not closed: continue because a token needs more input |
| `eof` | the end of input inside an entry |
| `keys` | two entries, a backspace on an empty entry, the byte `0x08`, a character of two bytes, and skipped keys |

`string.color.golden` holds the `-color` output of `string`.

The tests also check that, after every key, the paint is the same as the
paint of the entry painted from the start (`TestRepaint`).
`TestRepaintGenerated` does the same for 300 keys chosen from a fixed seed.
