# streams/sse

A scanner for Server-Sent Events, the `text/event-stream` format of the
[WHATWG HTML standard, "Server-sent events", "Parsing an event stream"](https://html.spec.whatwg.org/multipage/server-sent-events.html#parsing-an-event-stream),
built from `rules/core`. It lexes an endless stream with constant memory.

**Technique:** records made of `field: value` lines, ended by a blank line.
The type of a token depends on where it stands in its line: `data` at the
start of a line is a field name, after the colon it is part of a value. A rule
cannot see its place in the line, so a mode driver keeps it and picks the
scanner for each place, over one cursor.

```sh
go run ./streams/sse < streams/sse/testdata/document.input
```

## Tokens

| Type | Matches |
| --- | --- |
| `BOM` | U+FEFF as the first rune of the stream |
| `EVENT`, `DATA`, `ID`, `RETRY` | the field names `event`, `data`, `id` and `retry` at the start of a line; case matters |
| `FIELD` | any other field name: the runes up to the first `:` or line end; in pieces of at most 1000 runes |
| `COLON` | the `:` after a field name |
| `SPACE` | the one optional space right after that colon |
| `VALUE` | the rest of the line after the colon and the optional space; in pieces of at most 1000 runes |
| `COMMENT` | a line that starts with `:`, from the `:` to the line end; in pieces of at most 1000 runes |
| `EOL` | the line end of a field or comment line: `\r\n`, `\n` or `\r` |
| `EVENT_END` | a blank line: a line end at the start of a line, which ends the event |

The stream grammar of the specification takes every rune in a line, so every
rune of any input lexes as one of these types, and no token is `UNKNOWN`.

`FIELD`, `VALUE` and `COMMENT` run to a delimiter. A stream that has lost its
line ends would grow one such token without bound, so each is emitted in
pieces of at most 1000 runes: a longer run is several tokens of the same type
in a row, and no rule reads more than 1000 runes before it decides. Every mode
scanner is built with `bytescan.WithMaxTokenBytes(4096)`, a little above the
largest piece (1000 runes of at most 4 bytes), so a rule that scanned further
would fail `Next` with the limit error.

## The mode driver

The driver and the mode scanners live in the importable package
[`ssedriver`](ssedriver), so that [`streams/ssejson`](../ssejson) can pipe the
`data` values of a stream into a second scanner. `ssedriver.New(r, opts...)`
builds the driver, with `opts` applied to every mode scanner; this example passes
`bytescan.WithMaxTokenBytes(4096)`.

`ssedriver/driver.go` owns one cursor and six scanners over it, one per state.
The states group into the three modes of a line:

- **Line start.** `newLineScanner` lexes a field name, a comment, or a blank line
  (`EVENT_END`). The scanner for the first line of the stream also takes the
  `BOM`; there the first rune of a field name is not U+FEFF, so the two never
  match the same text. On any later line U+FEFF is a rune of the field name.
- **After the field name.** `newNameScanner` lexes the `COLON`, the line end of
  a field with no colon, or the next `FIELD` piece of a long name.
  `newColonScanner`, right after the colon, lexes the one optional `SPACE`, the
  first `VALUE` piece (which does not start with a space), or the line end of
  an empty value.
- **The rest of the line.** `newRunScanner` lexes the next `VALUE` piece, or the
  next `COMMENT` piece, and the line end.

Before each token the driver calls the scanner of its state; after it, the type
of the token picks the next state. A name goes to after the field name,
`COLON` to after the colon, `SPACE` and `VALUE` to value, `COMMENT` to comment,
and every line end back to line start. A `FIELD`, `VALUE` or `COMMENT` piece
keeps the state, so the next piece of a long run gets the same type: a name
1003 runes long that ends in `data` is two `FIELD` tokens, not a `FIELD` and a
`DATA`.

The driver keeps the state and runs the six scanners on a
[`modes.Driver`](https://github.com/xiam/bytescan/tree/master/modes), the public mode driver. Together they
follow the three rules of a mode driver:

1. **Release on every switch.** `modes.Driver` calls `Release()` on the
   outgoing scanner at every switch, so an idle scanner does not pin the input
   that the others read.
2. **Cache one scanner per state.** The driver builds the six scanners once and
   never builds one per token or drops one. The rules are built once and shared
   by the scanners.
3. **One `Next()` at a time.** The `Next` of `modes.Driver` holds a mutex.

Every scanner returns a plain `io.EOF` at the end of the input, so a stream that
ends inside a line ends as one that ends after a blank line.

## Covered subset

Every line form of the stream grammar: the BOM, comments, fields with and
without a colon, with and without the space, empty values, and the three line
ends. Not covered:

- **Event dispatch.** The scanner does not join `data` lines, set the event type
  or the last event ID, or dispatch anything at `EVENT_END`.
- **Field values.** `retry` is not checked for digits, an `id` that holds NUL
  is not ignored, and unknown field names are lexed, not dropped.
- **Decoding.** The cursor decodes UTF-8; an invalid byte reaches the rules as
  U+FFFD, as the UTF-8 decode that the specification requires makes it.
  Token texts are not changed otherwise.

## How invalid input lexes

The stream grammar has no invalid line: any text is a field, a comment, or a
blank line. Text that is not an event stream lexes as field names and values
(see `not-sse`). A line that lost its line end joins the next line, and lexing
is correct again from the next line end; the corruption test shows it on a
generated stream. A run with no delimiter lexes as pieces of 1000 runes.

## Test data

| Case | Shows |
| --- | --- |
| `document` | a realistic stream: comments, events with names and ids, multi-line data, an empty `data`, a field with no colon |
| `bom` | the BOM at the start of the stream |
| `bom-twice` | a second U+FEFF at the start, and U+FEFF on a later line: both runes of a field name |
| `fields` | a field with no colon, an empty value, a value after two spaces, a value with no space, `data: a: b`, `data::` |
| `names` | the four named fields, other names, a name with a space, `datax`, `DATA`, `events` |
| `comments` | comments with and without text, a blank line after a comment, a comment inside an event |
| `line-endings` | `\n`, `\r\n` and a bare `\r`, mixed in one stream |
| `unterminated` | a stream that ends inside a value |
| `not-sse` | HTML, JSON, invalid UTF-8 bytes, a NUL and a tab: all field names and values |
| `pieces` | a 1003-rune field name that ends in `data`, a 2100-rune value of one- and three-byte runes, a 1201-rune comment |

The stream tests (`stream_test.go`) use the harness in
[`streamtest`](https://github.com/xiam/bytescan/tree/master/streamtest):

- **`TestLongRun`** lexes a generated stream of events: event names, ids,
  `retry`, multi-line `data` with and without the space, unknown fields,
  comments inside events, heartbeat comments, and all three line ends (one per
  event, so a bare `\r` is never joined to a `\n`). It runs on the harness
  budget, under the heap ceiling, and checks that every type but `BOM` appears
  and that no token is `UNKNOWN`.
- **`TestCorruption`** injects runs of garbage (printable runes, a space, a colon
  and invalid UTF-8 bytes, no line end) before events. A clean driver over the
  same stream without garbage runs in step: every token after the line end of
  the line that a run joins equals the clean token, at the same offset less
  the garbage before it.
- **`TestUndelimited`** lexes one run of at least 16 KiB with no line end and
  no colon, as a field name, a value and a comment, of one- to four-byte runes,
  and checks that it comes out as pieces of at most 1000 runes of one type.

**Ceiling.** The generated lines are short (about 14 bytes), and a
`textreader` cursor trims its per-line table only once its first 64 KiB buffer
is full. Before that trim the table holds about 4,700 lines in two slices
grown to 128 KiB, above the harness's 48 KiB floor, and a race-detector run
(64 KiB) never reaches the trim. So the long-run and corruption tests set a
ceiling of 160 KiB per cursor when that is above the harness default: 160 KiB
for the long run under the race detector, and 320 KiB for the corruption test,
which has two cursors. The default run with one cursor keeps the harness
ceiling of 256 KiB, and the `-streamtest.long` run keeps 16 MiB.

```sh
go test -timeout 0 -run Long ./streams/sse -streamtest.long
```
