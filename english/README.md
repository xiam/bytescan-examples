# english

An illustrative English word splitter built only from `rules/base`. It follows
the [example standard](../README.md) and splits a few sample
sentences into words, numbers and punctuation. Its word rule approximates the
word boundaries of [UAX #29](https://www.unicode.org/reports/tr29/) and does not
implement it; `base.Word` lists the differences.

**Technique:** words against punctuation, with contractions, hyphenated words,
English number format `1,234.56` and repeated punctuation.

```sh
go run ./english < english/testdata/sentences.input
```

## What counts as a word

A word is a run of letters and combining marks. An apostrophe or a hyphen joins
two such parts, and it never starts or ends a word, so `don't` and
`well-known` are one word each, and `'quoted'` is `quoted` with the marks as
`PUNCTUATION`.

## Tokens

| Type | Rule |
| --- | --- |
| `WORD` | `base.Word` |
| `NUMBER` | `base.TextNumber(',', '.')`: `1,234.56` |
| `PUNCTUATION` | `base.Punctuation`: one token per mark, so `?!` is two |
| `WHITESPACE`, `NEWLINE` | `base.Whitespace`, `base.Newline` |
| `UNKNOWN` | one rune that no rule matches, such as `$` |

## Sample output

The fifth line of `sentences.input` (`What?!`) lexes as:

```text
5:0 WORD "What"
5:4 PUNCTUATION "?"
5:5 PUNCTUATION "!"
5:6 NEWLINE "\n"
```

The full output is [`testdata/sentences.golden`](testdata/sentences.golden).

## Covered subset

Only the sample sentences of [`testdata/sentences.input`](testdata/sentences.input).
Not covered:

- sentence splitting;
- quotation pairing;
- abbreviations, spelling, grammar and any dictionary.

This example is illustrative. It shows that the scanner can split a few sample
sentences into words; it is not a scanner for English.

## How invalid input lexes

The scanner never judges the text. A character that no rule matches becomes an
`UNKNOWN` token, one rune each, and lexing reaches the end of the input.

## Test data

| Case | Shows |
| --- | --- |
| `sentences` | a contraction (`don't`, `isn't`, `Tom's`), a hyphenated word (`well-known`, `Rock-and-roll`), a number (`1,234.56`), quoted speech, and repeated punctuation (`What?!`, `Wait...`) |
