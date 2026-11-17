# hello

The text example of the [README](https://github.com/xiam/bytescan/blob/master/README.md#example-text) as a
program: the keyword `say` and words, built from `rules/core` and
`rules/base`. It follows the [example standard](../README.md).

**Technique:** a keyword registered before the word rule that also matches
it, and a `Dumper` that skips layout.

```sh
go run ./hello < hello/testdata/hello.input
```

## Tokens

| Type | Rule |
| --- | --- |
| `KEYWORD` | `core.Keyword("say", core.UnicodeLetter)`: `say` when no letter follows |
| `WORD` | `base.Word` |
| `WHITESPACE`, `NEWLINE` | `base.Whitespace`, `base.Newline`; not printed |
| `UNKNOWN` | one rune that no rule matches |

`say` is also a `WORD` of the same length, and the rule registered first
wins the tie. `NEWLINE` is not in the README example; it lets this program
read a file.

## Covered subset

The keyword `say` and the words of `base.Word`. Digits and punctuation are left
out.

## How invalid input lexes

Each digit, quote and punctuation mark is one `UNKNOWN` rune. `Say` is a
`WORD`, because the keyword matches case. `sayonara` and `saying` are words,
because a letter follows `say`, and `say-so` is one word, longer than the
keyword.

## Test data

| File | Shows |
| --- | --- |
| `hello` | the README input |
| `near-misses` | `say` in other case, in quotes, inside words, and digits |

To write a rule of your own as state functions, see
[docs/rule-authoring.md](https://github.com/xiam/bytescan/blob/master/docs/rule-authoring.md).
