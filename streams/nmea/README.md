# streams/nmea

An NMEA 0183 scanner for the sentences that GPS receivers and marine sensors
send, as the [gpsd "NMEA Revealed"](https://gpsd.gitlab.io/gpsd/NMEA.html)
document describes them. It is a stream example: it lexes an endless stream of
sentences with constant memory, and keeps lexing after noise.

**Technique:** start-character framing with a checksum field, and
resynchronisation at the next `$` or `!` after noise. The rules are
context-free, so the scanner needs no driver: every sentence starts with a
character that no other token contains.

```sh
go run ./streams/nmea < streams/nmea/testdata/sentences.input
```

## Tokens

| Type | Matches |
| --- | --- |
| `START` | `$`, or `!` for encapsulated sentences such as AIS |
| `ADDRESS` | five upper-case letters: talker and sentence formatter (`GPGGA`, `AIVDM`), or a proprietary address of the same length (`PGRME`) |
| `FIELD` | a run of 1 to 1000 field bytes: printable ASCII (0x20 to 0x7E) except `$`, `!`, `*` and `,`: `core.RuneRunMax(fieldByte, 1000)` |
| `COMMA` | `,`, the field separator |
| `CHECKSUM` | `*` and two hexadecimal digits, in either case |
| `EOL` | `\r\n`, or `\n` alone |
| `UNKNOWN` | one rune that no rule matches |

An empty field has no token: it shows as two adjacent `COMMA` tokens.

`ADDRESS` and `FIELD` both match five upper-case letters before a delimiter.
`ADDRESS` is registered first, so it wins that tie; a longer run, or a run with
another byte, is longer as a `FIELD`, and the longest match makes it a `FIELD`.
No other two rules match the same text.

Every rule is bounded. A run of field bytes longer than 1000 runes, such as a
stream that has lost its delimiters, lexes as several `FIELD` tokens in a row,
and no rule reads more than 1000 runes before it decides. The scanner is built
with a per-token guard of 4,096 bytes (`streamtest.TextGuard`), so a rule that
scanned without bound would fail instead of holding the stream in memory. The
guard is a safety net that no input reaches: `go run` on input with no
delimiter ends normally with `FIELD` pieces of 1000 bytes.

`EOL` is `\r?\n`, not `base.Newline`, which also takes a carriage return
alone: NMEA 0183 never ends a sentence with one.

## Covered subset

Sentences of the form `$` or `!`, address, comma-separated fields, an optional
`*hh` checksum, and a line break. Not covered:

- **Checksums.** The `CHECKSUM` token is lexed as a field: the scanner never
  computes or verifies it, and a sentence without one lexes the same way up to
  its `EOL`.
- **Field meaning.** Times, coordinates and status letters are `FIELD`
  tokens; the scanner interprets none of them, and does not check the number of
  fields of a sentence type.
- **Addresses of another length.** A proprietary address of four letters
  (`PUBX`) or of more than five is a `FIELD`.
- **NMEA 4 tag blocks and escapes.** `\…\` tag blocks and `^hh` escapes are
  field bytes; the scanner gives them no token of their own.
- **Sentence length.** The 82-character limit is not checked.

## How invalid input lexes

The rules are context-free, so they do not know where a sentence starts:

- Noise made of field bytes lexes as `FIELD`, `ADDRESS` and `COMMA`, as if it
  were the fields of a sentence. A data field of exactly five upper-case
  letters lexes as `ADDRESS`, and so does a last piece of five upper-case
  letters after a 1000-rune `FIELD`.
- A byte outside every class, such as a control character, a lone `\r` or a
  byte of 0x80 or above, is one `UNKNOWN` token.
- A `*` that two hexadecimal digits do not follow is `UNKNOWN`: `*4` lexes as
  `UNKNOWN "*"` and `FIELD "4"`, and `*4F1` as `CHECKSUM "*4F"` and
  `FIELD "1"`.
- A sentence cut short by noise lexes as far as it goes.

In every case lexing is correct again from the next `START`, because no other
token contains `$` or `!`.

## Stream tests

The tests use the stream harness
([`streamtest`](https://github.com/xiam/bytescan/tree/master/streamtest)). The
generator in `generator_test.go` writes an endless stream of GGA, RMC, GSV and
VTG sentences from the GP, GL and GN talkers, with plausible values, empty
fields where receivers leave them empty, and correct checksums.

| Test | Shows |
| --- | --- |
| `TestLongRun` | a clean stream of the default budget (1 GiB with `-streamtest.long`) under the harness ceiling: one `START`, `ADDRESS`, `CHECKSUM` and `EOL` per sentence, no `UNKNOWN` |
| `TestCorruption` | noise between sentences: printable runs lex as `FIELD`, `ADDRESS` and `COMMA`, control bytes as `UNKNOWN`; no token crosses the end of the noise, and the next token is the `START` of the next sentence |
| `TestUndelimitedRun` | one 32 KiB run of field bytes with no delimiter lexes as `FIELD` pieces of at most 1000 runes |

```sh
go test ./streams/nmea
go test -timeout 0 -run Long ./streams/nmea -streamtest.long
```

## Test data

| File | Shows |
| --- | --- |
| `sentences` | GGA, RMC, GSV and VTG sentences with checksums |
| `empty-fields` | sentences of a receiver without a fix: runs of adjacent `COMMA` |
| `ais` | the `!` start of AIS sentences, with armoured payloads |
| `no-checksum` | sentences without a checksum, a proprietary address, and `EOL` as `\n` alone |
| `noise` | printable noise between sentences (`FIELD`, `ADDRESS`), non-printable noise (`UNKNOWN`), and a sentence cut by noise |
| `bad-checksum` | a checksum with one digit, a non-hexadecimal digit, lower case, a third digit, a doubled `*`, and a bare `*` |
| `long-field` | fields of more than 1000 runes, lexed in pieces; the last piece of the upper-case field is five letters, so it is an `ADDRESS` |
