# korean

An illustrative Korean word splitter built only from `rules/base`. It follows
the [example standard](../README.md) and splits a few sample
sentences into words, numbers and punctuation, as
[UAX #29](https://www.unicode.org/reports/tr29/) describes word boundaries.

**Technique:** a script with spaces whose syllables have two encodings.

```sh
go run ./korean < korean/testdata/sentences.input
```

## Words and spaces

Korean separates words with spaces, so `base.Word` splits it as it splits
English. This is the difference from the [Japanese example](../japanese),
where no spaces exist and the text splits only into script runs.

A spaced unit can carry attached particles: `언어를` is the noun `언어`
("language") and the object particle `를`. The example does not split
particles; the unit between spaces is one `WORD`.

## Two encodings

A Hangul syllable has two encodings: one precomposed syllable (NFC), or a
sequence of two or three conjoining jamo (NFD). Both are letters, so `base.Word`
reads either form as one `WORD`. The input holds `한글` in both forms; the
tokens print the same, but the NFC word is two runes and the NFD word six, as
the columns of the following newlines show. No rule normalizes the text, so the
two words are different texts.

## Tokens

| Type | Rule |
| --- | --- |
| `WORD` | `base.Word`: Hangul syllables or jamo, or Latin letters |
| `NUMBER` | `base.TextNumber(',', '.')`, ASCII digits |
| `PUNCTUATION` | `base.Punctuation`: one token per mark, so `!!!` is three |
| `WHITESPACE`, `NEWLINE` | `base.Whitespace`, `base.Newline` |
| `UNKNOWN` | one rune that no rule matches, such as `$` |

## Covered subset

Only the sample sentences of [`testdata/sentences.input`](testdata/sentences.input):
greetings and short questions. Not covered:

- Splitting particles and endings from a word.
- A Latin word attached to Hangul, such as `Go를`: `base.Word` does not stop
  where the script changes, so it is one `WORD`.
- Normalization, Hanja, spelling and grammar.

This example is illustrative; it is not a scanner for Korean.

## How invalid input lexes

The scanner never judges the text. A character that no rule matches becomes an
`UNKNOWN` token, one rune each, and lexing reaches the end of the input.

## Test data

| Case | Shows |
| --- | --- |
| `sentences` | plain words (`안녕하세요`); repeated `!` and `?` (`안녕!!!`, `정말요???`); the number in `3개`; the Latin word `Go` inside Korean text; `한글` as NFC (line 7) and NFD (line 8), each one `WORD` |
