# ansi

A scanner for text mixed with terminal escape sequences, the control functions
of [ECMA-48](https://ecma-international.org/publications-and-standards/standards/ecma-48/)
(5th edition) in the 7-bit forms that
[xterm](https://invisible-island.net/xterm/ctlseqs/ctlseqs.html) and most
terminals use. It is built only from `rules/core`, and it is part of the
[example standard](../docs/standard.md).

**Technique:** control data mixed into text. Every escape sequence starts with
the one control rune ESC, so `TEXT` is the complement of the controls and the
sequences are composed from `core.Sequence`, `core.ZeroOrMore` and
`core.RuneRange` byte classes. A filter above `Next()` then strips them.

```sh
go run ./ansi < ansi/testdata/colours.input
```

The dump shows each control in the escaped form of a Go string: ESC is `\x1b`,
BEL is `\a`.

Add `-strip` to write the plain text instead of the dump: a tool that strips
colours from a log. It is the filter `stripped` in `main.go`, which keeps only
`TEXT`, `TAB`, `LF` and `CR` and drops every other token, `UNKNOWN` included:

```sh
go run ./ansi -strip < ansi/testdata/colours.input
```

## Tokens

In the patterns, a byte stands for the rune of the same value.

| Type | Matches |
| --- | --- |
| `TEXT` | a run of runes that are not controls: not C0 (`0x00`–`0x1F`) and not DEL (`0x7F`) |
| `BEL`, `BS`, `TAB`, `LF`, `CR` | one C0 control each: `0x07`, `0x08`, `0x09`, `0x0A`, `0x0D` |
| `C0` | one other C0 control, such as NUL, VT or FF; ESC (`0x1B`) is not one |
| `CSI` | `ESC [`, parameter bytes `0x30`–`0x3F`, intermediate bytes `0x20`–`0x2F`, one final byte `0x40`–`0x7E`: `ESC[1;31m`, `ESC[2K`, `ESC[?25l` |
| `OSC` | `ESC ]`, a string of `TEXT` runes, then `BEL` or ST (`ESC \`): window titles, OSC 8 hyperlinks |
| `ESC_SEQ` | `ESC`, intermediate bytes `0x20`–`0x2F`, one final byte `0x30`–`0x7E`; with no intermediate byte the final is not `[` or `]`: `ESC 7`, `ESC c`, `ESC ( B`, `ESC \` |
| `UNKNOWN` | an ESC that starts no complete sequence, and DEL |

No two rules match the same text, so the registration order decides nothing.

## Covered subset

The 7-bit C0 controls, and the escape sequences, control sequences and
operating system commands that start with ESC. Not covered:

- **Interpretation.** The scanner does not decode what a sequence does. It keeps
  no screen, cursor or colour state, and it does not split the parameters of a
  `CSI` or check them against the final byte: `ESC[;;;H` and `ESC[99999m` are
  `CSI` tokens.
- **8-bit C1 controls.** U+0080 to U+009F are `TEXT`, so `U+009B` is not a
  CSI. A raw byte `0x9B` is not valid UTF-8, so it reaches the scanner as
  U+FFFD and is `TEXT` too.
- **Other control strings.** `DCS` (`ESC P`), `SOS` (`ESC X`), `PM` (`ESC ^`)
  and `APC` (`ESC _`) are two-rune `ESC_SEQ` tokens; the string after them is
  `TEXT`.
- **OSC strings with controls.** ECMA-48 allows `0x08`–`0x0D` in a command
  string; here any control other than the terminator ends the `OSC` match.

## How invalid input lexes

A sequence is one token only when it is complete. When it is not, its ESC is
`UNKNOWN` and lexing goes on with the next rune, so the rest of the sequence
lexes as what it is on its own, usually `TEXT`:

- `ESC[1;3` cut by a line break or by the end of input: `UNKNOWN "\x1b"`,
  then `TEXT "[1;3"`.
- `ESC[ 1m`, an intermediate byte before a parameter byte: `UNKNOWN`, then
  `TEXT "[ 1m"`.
- An `OSC` with no terminator, or with ESC but no `\` at the end of input:
  `UNKNOWN`, then `TEXT` for the string, then `UNKNOWN` for the last ESC.
- ESC followed by DEL or by a line break: `UNKNOWN`, and DEL is `UNKNOWN` too.

`ESC`, a space and `e` is a complete `ESC_SEQ`: the space is an intermediate
byte and `e` a final byte.

## Test data

| File | Shows |
| --- | --- |
| `colours` | SGR with one, several and no parameters, and a `TAB`; `colours.txt` is its plain text |
| `progress` | a progress line that rewrites itself with `CR` and erase-line, then cursor movement |
| `private-mode` | private-mode sequences `ESC[?25l` and `ESC[?1049h`, and the `>` and `=` private markers |
| `osc-title` | an OSC window title ended by `BEL`, and an empty one |
| `hyperlink` | an OSC 8 hyperlink opened and closed, each ended by `ESC \` |
| `utf8` | UTF-8 text between sequences, and a UTF-8 window title |
| `esc-seq` | `ESC_SEQ` forms: save and restore cursor, reset, character-set designation, keypad modes, ST alone |
| `c0` | each single-control type, other C0 controls and DEL |
| `near-misses` | broken sequences, a lone ESC, ESC before DEL, C1 runes and an invalid UTF-8 byte |
| `truncated-csi`, `truncated-osc`, `truncated-esc`, `truncated-st` | a sequence cut off by the end of the input |

The inputs hold raw control bytes. `testdata/.gitattributes` marks them
`-text -whitespace`, so git keeps them byte for byte.

Run `go test ./ansi`. `TestStrip` checks that the filter turns
`colours.input` into `colours.txt`; `TestStripKeepsText` checks the filter on
every input against the `TEXT`, `TAB`, `LF` and `CR` lines of its golden file.
