# japanese

An illustrative splitter of Japanese text into script runs, built from
`rules/base` and `rules/core`. It follows the
[example standard](../README.md) and uses the Unicode
[Script property](https://www.unicode.org/reports/tr24/).

**Technique:** script runs where words have no spaces.

```sh
go run ./japanese < japanese/testdata/sentences.input
```

## A script run is not a word

Japanese writes no spaces between words, and true word boundaries need a
dictionary, which is out of scope. The example splits text into script runs,
the nearest lexical unit: a run of Han (kanji), hiragana, katakana or Latin.

A script run is not a word. In `書きます` ("write"), one word, the stem `書` is
Han and the ending `きます` is hiragana, so it is two runs:

```text
2:15 HAN "書"
2:16 HIRAGANA "きます"
```

The other way round, `とひらがなと` is one hiragana run that holds the particle
`と`, the word `ひらがな` and `と` again.

## Tokens

| Type | Rule |
| --- | --- |
| `HAN` | `base.ScriptRun("Han")` |
| `HIRAGANA` | `base.ScriptRun("Hiragana")` |
| `KATAKANA` | `base.ScriptRun("Katakana", core.RuneSet("ー"))`: `\p{Katakana}[\p{Katakana}\pMー]*` |
| `LATIN` | `base.ScriptRun("Latin")` |
| `NUMBER` | `base.TextNumber(',', '.')`, ASCII digits |
| `PUNCTUATION` | `base.Punctuation`: one token per mark, such as `、` `。` `！` `？` |
| `WHITESPACE`, `NEWLINE` | `base.Whitespace`, `base.Newline` |
| `UNKNOWN` | one rune that no rule matches |

The prolonged sound mark `ー` (U+30FC) is in the Common script, so
`base.ScriptRun("Katakana")` would end the run before it. The extra class
`core.RuneSet("ー")` lets it continue a katakana run, so `コーヒー` is one
`KATAKANA` token. An extra class never starts a run.

## Covered subset

Only the sample sentences of [`testdata/sentences.input`](testdata/sentences.input).
Not covered:

- Word segmentation: no dictionary and no statistical model.
- `ー` after hiragana, and `ー` at the start of a run: each is `UNKNOWN`.
- Full-width digits and Latin letters, the ideographic space U+3000, and
  half-width katakana.
- Readings, morphology and normalization.

This example is illustrative; it is not a scanner for Japanese.

## How invalid input lexes

The scanner never judges the text. A character that no rule matches becomes an
`UNKNOWN` token, one rune each, and lexing reaches the end of the input.

## Test data

| Case | Shows |
| --- | --- |
| `sentences` | a sentence that mixes the three scripts (`私はカタカナとひらがなと漢字を書きます。`); `ー` inside the katakana runs `コーヒー`, `タワー`, `スーパー` and `ラーメン`; the Latin word `Go` inside Japanese text; a number (`3時`); `、` `。` `！` `？` as `PUNCTUATION` |
