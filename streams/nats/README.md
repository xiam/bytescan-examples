# streams/nats

A scanner for a stream of the NATS client protocol, in both directions (the
operations of the
[client protocol](https://docs.nats.io/reference/reference-protocols/nats-protocol)),
built from `rules/core` over the byte adapter of
[`bytesource`](https://github.com/xiam/bytescan/tree/master/bytesource). It is a long-running stream
example (see
[Long-running streams](../../docs/streams.md)).

**Technique:** text control lines mixed with byte-counted binary payloads on
one stream. Rules lex each control line; its `SIZE` fields tell the driver how
many bytes of payload follow, and the driver reads them from the cursor. So a
payload that holds `\r\nMSG …` is never taken for a control line. A size
counts bytes, so the cursor is the byte adapter: on a UTF-8 cursor a
multi-byte sequence in a payload would count as one rune and misframe the
stream.

```sh
go run ./streams/nats < streams/nats/testdata/session.input
```

Each line of the output is `OFFSET TYPE LENGTH "BYTES"`: the byte offset and
length of the token, and its first 48 bytes as a Go quoted string, followed by
`...` when it is longer. Control lines read as text, and the bytes of a payload
stay on one line.

## Tokens

| Type | Matches |
| --- | --- |
| `OP` | an operation, in any case: `INFO`, `CONNECT`, `PUB`, `HPUB`, `SUB`, `UNSUB`, `MSG`, `HMSG`, `PING`, `PONG`, `+OK`, `-ERR` |
| `SUBJECT` | the subject of `PUB`, `HPUB`, `SUB`, `MSG` and `HMSG`: `[^ \t\r\n]{1,1000}` |
| `SID` | the subscription ID of `SUB`, `UNSUB`, `MSG` and `HMSG`: `[^ \t\r\n]{1,1000}` |
| `REPLY_TO` | the optional reply subject of `PUB`, `HPUB`, `MSG` and `HMSG`: `[^ \t\r\n]{1,1000}` |
| `QUEUE_GROUP` | the optional queue group of `SUB`: `[^ \t\r\n]{1,1000}` |
| `SIZE` | a byte count, at most 10 digits: `[0-9]{1,10}`; the header size and the total size of `HMSG` and `HPUB` are both `SIZE` |
| `MAX_MSGS` | the optional message count of `UNSUB`: `[0-9]{1,1000}` |
| `JSON` | the argument of `INFO` and `CONNECT`, to the end of the line: `[^ \t\r\n][^\r\n]{0,999}`, then `[^\r\n]{1,1000}` |
| `ERROR_TEXT` | the text of `-ERR`, to the end of the line, in the same pieces as `JSON` |
| `SP` | a run of spaces and tabs: `[ \t]{1,1000}` |
| `CRLF` | the end of a control line, and the `\r\n` after a payload |
| `HEADERS` | the header bytes of `HMSG` and `HPUB`, read by the driver, at most 64 KiB each |
| `PAYLOAD` | the payload bytes of `MSG`, `PUB`, `HMSG` and `HPUB`, read by the driver, at most 64 KiB each |
| `UNKNOWN` | a byte that no rule of the current mode matches, or a piece of a digit run that is too long for a size |

`QUEUE_GROUP` and `MAX_MSGS` are not in the ticket's list of tokens; they are
the other two optional fields of `SUB` and `UNSUB`, and without them those
lines would lex as `UNKNOWN`.

**Bounded runs.** Every token that runs to a delimiter is at most 1000 bytes:
a longer subject, JSON, error text or run of blanks is emitted as several
tokens of the same type in a row. A digit run of more than 10 digits in the
place of a size is emitted as `UNKNOWN` pieces of at most 1000 digits, and the
driver reads no payload after that line (the driver relabels them with
`Token.WithType`). Every scanner is built with a per-token guard of 2,048 bytes
(`WithMaxTokenBytes`), which `newScanner` passes to `newDriver`: the guard counts
bytes, and it also counts what a rule reads ahead. The longest read is a size
with the lookahead of the size pair: 10 digits, 1000 blanks, 10 digits, 1000
blanks and the CRLF, 2,022 bytes in all. A field of 1000 bytes with the
lookahead of the queue group reads at most 40 blanks and one more byte after
it, 1,042 bytes. `TestLookaheadGuard` checks both reads.

## The mode driver

`driver.go` owns one byte cursor and these scanners over it, each built once:

| Mode | Scanner | Rules |
| --- | --- | --- |
| start of a line | `newOpScanner` | `OP`, `CRLF` |
| after the operation, after the last field | `newSepScanner` | `SP`, `CRLF` |
| a subject, a SID, the next pieces of a reply subject or a queue group | `newFieldScanner` | the field, `SP`, `CRLF` |
| a size | `newSizeScanner` | `SIZE`, a longer digit run, `SP`, `CRLF` |
| the rest of a digit run that is too long | `newLongDigitsScanner` | a digit run, `SP`, `CRLF` |
| the count of `UNSUB` | `newMaxMsgsScanner` | `MAX_MSGS`, `SP`, `CRLF` |
| the optional reply subject, or the first size | `newOptionalReplyScanner` | `SIZE` confirmed by the rest of the line, a longer digit run, `REPLY_TO`, `SP`, `CRLF` |
| the optional queue group, or the SID | `newOptionalQueueScanner` | `QUEUE_GROUP` confirmed by at most 40 blanks and another field after it, `SID`, `SP`, `CRLF` |
| the argument of `INFO`, `CONNECT`, `-ERR` | `newTextScanner` | the first piece, `SP`, `CRLF`; then the next pieces, `CRLF` |

The driver picks the scanner by the place in the line. It knows the fields of
each operation, and it moves to the next field after the blanks that follow a
field. Once a field has a piece, its next pieces come from the scanner of its
type, so a field longer than 1000 bytes has one type in every piece.

Two fields are optional and come before a field of another type, so the place
alone does not tell them apart. A rule with lookahead (`core.FollowedBy`)
decides from the rest of the line:

- In `PUB` and `MSG`, a run of at most 10 digits that the `CRLF` follows is the
  size, and any other field is the reply subject. In `HPUB` and `HMSG`, the
  run must be followed by blanks, a second size and the `CRLF`. Up to 1000
  blanks before the `CRLF` are allowed, as the server allows them.
- In `SUB`, a field that at most 40 blanks and another field follow is the
  queue group; any other is the SID.

After the `CRLF` of a `MSG` or `PUB` line, the driver reads `SIZE` bytes from
the cursor as `PAYLOAD` tokens of at most 64 KiB each. After `HMSG` or `HPUB`
it reads the header size as `HEADERS` and the rest of the total size as
`PAYLOAD`. The operation scanner then lexes the `CRLF` after the payload. A
payload of 0 bytes gives no token. A declared size may be corrupt, so memory
is bounded by the 64 KiB piece, never by the size. The headers and the payload
are chunked fields of a `bytesource.Fields` queue. A field length is an `int`,
and a 10-digit size does not fit one on a 32-bit platform, so a longer one is
queued as several fields, one after the other (`streamkit.PushLong`, shared
with syslog and websocket).

The scanners run on a [`modes.Driver`](https://github.com/xiam/bytescan/tree/master/modes). Together they follow
the three rules of a mode driver:

1. **Release on every switch.** `modes.Driver` calls `Release()` on the
   outgoing scanner when the driver switches scanners, and the driver calls it on
   the active scanner before it reads a payload from the cursor, so an idle
   scanner's checkpoints do not pin the payload.
   `TestReleaseBeforeRead` lexes a 100 KiB payload through a cursor that
   retains at most 4 KiB.
2. **Cache one scanner per mode.** The driver builds its 16 scanners when it is
   built and never builds another one. The rules are package variables shared
   by every scanner: a rule keeps no state between tokens, and a `core.Repeat`
   of up to 1000 is about 120 KB to build (a run of one class is
   `core.RuneRunMax`, whose cost does not grow with the count).
3. **One `Next()` at a time.** The driver's `Next` holds a mutex.

## Covered subset

Every operation of the client protocol, both directions on one stream. Not
covered:

- **No subscription logic.** The driver does not match a `MSG` to a `SUB`, or
  count messages after `UNSUB`.
- **Headers are one token.** The `NATS/1.0` line and the header fields are
  not lexed; they are the bytes of `HEADERS`.
- **JSON is one token.** The argument of `INFO` and `CONNECT` is not lexed as
  JSON; it is the bytes to the end of the line.
- **A reply subject made of digits.** In `PUB` and `MSG`, a reply subject of
  more than 10 digits only is taken for a size that is too long, so the line
  frames no payload. A reply subject of at most 10 digits is lexed right,
  because the rest of the line decides.
- **A total size of `HPUB` or `HMSG` of more than 10 digits** makes the rule
  for the header size fail its lookahead, so a header size before it is lexed
  as `REPLY_TO`. The long total is `UNKNOWN`, and the line frames nothing.
- **A queue group longer than 1000 bytes** cannot be confirmed by the
  lookahead, which sees one piece of the field. It is lexed as `SID` pieces,
  and the real SID after it is `UNKNOWN`.
- **More than 1000 blanks before the `CRLF`** of a `PUB`, `MSG`, `HPUB` or
  `HMSG` line with no reply subject: the size lookahead reads at most 1000
  blanks, so the size is lexed as `REPLY_TO` and no payload is framed. The
  payload is then lexed as control lines. Up to 1000 blanks frame correctly.
- **A queue group followed by more than 40 blanks** is lexed as the SID, so
  that the lookahead of the queue-group rule stays under the per-token guard.
  The real SID after it is then `UNKNOWN`. `SUB` frames no payload, so the
  next line is not affected.
- **The line ends with `\r\n` only.** A bare `\n` is `UNKNOWN`, and the line
  goes on until the next `\r\n`.

## How invalid input lexes

- **Garbage before an operation** is `UNKNOWN`, one byte each, up to the next
  operation. A line that does not start with an operation is `UNKNOWN` up to
  its `CRLF`.
- **A damaged control line frames no payload.** The driver reads a payload
  only after a line that lexed without `UNKNOWN` and with every size it needs.
  After any other line, the bytes that follow are lexed as control lines: the
  first `CRLF` ends the damage, and lexing resumes at the next control line.
- **A size of more than 10 digits** is `UNKNOWN`, in pieces of at most 1000
  digits, and the line frames no payload.
- **An extra field** is `UNKNOWN`, one byte each, up to the `CRLF`.
- **A header size above the total size** is cut to the total: the driver reads
  the total as `HEADERS` and no `PAYLOAD`.
- **The input may end inside a payload.** The last `PAYLOAD` holds the bytes up
  to the end, and lexing ends normally.

## Stream tests

`stream_test.go` runs the example through the stream harness
(`streamtest`):

| Test | Shows |
| --- | --- |
| `TestLongRun` | a generated server stream (`INFO`, then `MSG` and `HMSG` with binary payloads that hold `\r\n` and `MSG` lines, `PING`, `PONG`, `+OK`, `-ERR`) of 512 KiB, 64 KiB under `-race`, or 1 GiB with `-streamtest.long`: one `OP` per line, every payload byte in `PAYLOAD` or `HEADERS`, no `UNKNOWN` |
| `TestCorruption` | the same stream with garbage runs between lines: each run is `UNKNOWN`, the next token is the `OP` of the next line, and no other token is `UNKNOWN` |
| `TestCorruptSize` | a size damaged to `9999999999` before a stream that ends long before it: `PAYLOAD` pieces of 64 KiB up to the end of the input |
| `TestUndelimited` | a run of more than 16 KiB with no delimiter in a JSON, a subject, a SID, a reply subject, two size places and the blanks: pieces of at most 1000 bytes of one type, then the next control line. The text runs are bytes of `0x80` or above |

```sh
go test -timeout 0 -run Long ./streams/nats -streamtest.long
```

**Ceiling.** `TestLongRun` and `TestCorruption` raise the harness ceiling to at
least 128 KiB (`minCeiling`), so the ceiling is 128 KiB under `-race` and stays
256 KiB by default and 16 MiB with `-streamtest.long`. The cursor keeps a line table of
16 bytes per line for its 64 KiB buffer, and through the byte adapter every
`0x0A` byte of a payload starts a line. The generated payloads hold `\r\n`
pairs, so a run of 64 KiB under `-race`, with its baseline after 4 KiB,
measured 71 to 78 KiB of growth as the first buffer fills. The growth does not
follow the stream: it is 72 KiB after 128 KiB and after 256 KiB. A payload made
only of `0x0A` bytes holds about 1 MiB in the table, the size of a full buffer
of lines, and no more after 4 MiB.

**Warm-up.** A scanner holds about 36 KB more once it has run, whatever the
input, so a driver holds about 0.6 MB once all its modes have run. The first
record of the generated stream has one line of every kind, and
`TestUndelimited` starts with lines of every kind, so the baseline heap sample
is taken after every scanner has run.

## Test data

| Case | Shows |
| --- | --- |
| `session` | every operation in a realistic exchange: `INFO`, `CONNECT`, `+OK`, `SUB` with and without a queue group, `PUB` with and without a reply subject, `HPUB`, `MSG`, `HMSG`, `PING`, `PONG`, `UNSUB` with and without a count, `-ERR` |
| `payloads` | a payload with `\r\nMSG` in it, a payload of valid multi-byte UTF-8 followed by the next control line, every byte value, zero-length payloads, blanks and tabs between fields, blanks before the `CRLF`, an operation in lower case |
| `headers` | headers with a payload, headers with no payload, a header size above the total, blanks before the `CRLF` |
| `long-sizes` | sizes of more than 10 digits in each place, then a 10-digit size whose payload the end of the input cuts |
| `broken` | garbage before an operation, an unknown operation, a bare `\n`, a size that is not a number, a missing size, an extra field, an operation with no blank after it |
| `truncated` | the input ends inside a payload |
| `empty` | no input |

The inputs hold `\r\n` pairs and bytes of `0x80` or above, so they are built
in `inputs_test.go`; `TestInputs` checks each committed input against the bytes
built there, and `go test -update` rewrites them.
