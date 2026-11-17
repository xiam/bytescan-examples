# words

A multilingual word scanner built only from `rules/base` and `rules/special`. It
follows the [example standard](../README.md).

**Technique:** registration order between overlapping rules, and a `Dumper`
that skips layout.

```sh
go run ./words < words/testdata/words.input
```

## Tokens

| Type | Rule |
| --- | --- |
| `URL` | `special.URL` |
| `EMOJI` | `special.Emoji` |
| `HAN` | `base.ScriptRun("Han")` |
| `WORD` | `base.Word` |
| `PUNCTUATION` | `base.Punctuation` |
| `WHITESPACE`, `NEWLINE` | `base.Whitespace`, `base.Newline`; not printed |
| `UNKNOWN` | one rune that no rule matches |

The URL, the emoji and the Han run are registered before `base.Word`. Each
matches text that `base.Word` also matches, and when two rules match the same
text, the first one registered wins.

## Covered subset

English, French, Greek, a Han run, an emoji and a URL. Not covered:

- `base.Word` approximates UAX #29 and does not stop where the script changes,
  so the input sets the Han run apart from Latin words with spaces.
- No normalization: the NFC and NFD forms of `café` are each one word, but
  different texts.
