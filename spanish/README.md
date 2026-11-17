# spanish

An illustrative Spanish word splitter built only from `rules/base`. It follows
the [example standard](../README.md) and splits a few sample
sentences into words, numbers and punctuation. Its word rule approximates the
word boundaries of [UAX #29](https://www.unicode.org/reports/tr29/) and does not
implement it; `base.Word` lists the differences.

**Technique:** words against punctuation, with inverted marks (`¡`, `¿`) and
accented letters, using the Spanish number format `1.234,56`.

```sh
go run ./spanish < spanish/testdata/sentences.input
```

## What counts as a word

A word is a run of letters and combining marks. An apostrophe or a hyphen joins
two such parts, and it never starts or ends a word. Accented letters such as
`á`, `é`, `ñ` belong to the word.

## Tokens

| Type | Rule |
| --- | --- |
| `WORD` | `base.Word` |
| `NUMBER` | `base.TextNumber('.', ',')`: `1.234,56` |
| `PUNCTUATION` | `base.Punctuation`: one token per mark, so `!!!` is three |
| `WHITESPACE`, `NEWLINE` | `base.Whitespace`, `base.Newline` |
| `UNKNOWN` | one rune that no rule matches, such as `$` |

## Sample output

The sixth line of `sentences.input` (`¡hola! ¿cómo estás?`) lexes as:

```text
6:0 PUNCTUATION "¡"
6:1 WORD "hola"
6:5 PUNCTUATION "!"
6:6 WHITESPACE " "
6:7 PUNCTUATION "¿"
6:8 WORD "cómo"
6:12 WHITESPACE " "
6:13 WORD "estás"
6:18 PUNCTUATION "?"
6:19 NEWLINE "\n"
```

The full output is [`testdata/sentences.golden`](testdata/sentences.golden).

## Covered subset

Only the sample sentences of [`testdata/sentences.input`](testdata/sentences.input).
Not covered:

- sentence splitting;
- pairing of `¡` with `!` or `¿` with `?`;
- abbreviations, spelling, grammar and any dictionary.

This example is illustrative. It shows that the scanner can split a few sample
sentences into words; it is not a scanner for Spanish.

## How invalid input lexes

The scanner never judges the text. A character that no rule matches becomes an
`UNKNOWN` token, one rune each, and lexing reaches the end of the input.

## Test data

| Case | Shows |
| --- | --- |
| `sentences` | the owner's six cases (`hola`, `HOLAAAA`, `HOLA!!!`, `HOLA!`, `hola, como estas`, `¡hola! ¿cómo estás?`), a number with `ñ` (`El niño tiene 1.234,56 pesos.`) and an ellipsis (`bueno... ya veremos`) |
