# arabic

An illustrative Arabic word splitter built from `rules/base` and `rules/core`.
It follows the [example standard](../README.md) and splits a few
sample sentences into words, numbers and punctuation, as
[UAX #29](https://www.unicode.org/reports/tr29/) describes word boundaries.

**Technique:** combining marks in right-to-left text: a vowelled word, whose
harakat are combining marks, stays one `WORD`.

```sh
go run ./arabic < arabic/testdata/sentences.input
```

## Reading order

The scanner reads runes in logical (reading) order, the order in which they are
stored, and tokens come out in that order: the first token of a line is its
first word read from the right. Display direction is not the scanner's concern;
bidirectional reordering and shaping belong to the renderer. Columns count
runes from the start of the line in logical order.

## Tokens

| Type | Rule |
| --- | --- |
| `WORD` | `base.Word`: letters and combining marks, so `مَرْحَبًا` is one word |
| `NUMBER` | `base.TextNumber('٬', '٫', base.TextDigits(...))`: the Arabic-Indic digits `٠`–`٩` (U+0660–U+0669) with the thousands separator `٬` and the decimal separator `٫` |
| `PUNCTUATION` | `base.Punctuation`: one token per mark, such as `،` `؟` `!` |
| `WHITESPACE`, `NEWLINE` | `base.Whitespace`, `base.Newline` |
| `UNKNOWN` | one rune that no rule matches, such as `$` |

## Covered subset

Only the sample sentences of [`testdata/sentences.input`](testdata/sentences.input):
greetings and short questions. Not covered:

- ASCII digits, and the Arabic percent sign `٪` (U+066A).
- Splitting clitics such as the article `ال` or the prefix `و` from a word.
- Normalization, shaping, bidirectional reordering, spelling and grammar.

This example is illustrative; it is not a scanner for Arabic.

## How invalid input lexes

The scanner never judges the text. A character that no rule matches becomes an
`UNKNOWN` token, one rune each, and lexing reaches the end of the input.

## Test data

| Case | Shows |
| --- | --- |
| `sentences` | plain words (`مرحبا`, `السلام عليكم`); the vowelled `مَرْحَبًا` as one `WORD` of nine runes; `،` and `؟` as `PUNCTUATION`; repeated `!` (`أهلا!!!`); the Arabic-Indic numbers `١٢٣` and `١٬٢٣٤٫٥٠` |
