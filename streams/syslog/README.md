# syslog

A scanner for a stream of syslog messages
([RFC 5424, section 6](https://www.rfc-editor.org/rfc/rfc5424#section-6))
sent over TCP
([RFC 6587, section 3.4](https://www.rfc-editor.org/rfc/rfc6587#section-3.4)),
built from `rules/core` and a mode driver over the
[byte adapter](https://github.com/xiam/bytescan/tree/master/bytesource).

**Technique:** a length written as decimal text that frames the next record,
mixed with newline-ended records. An octet-counted frame is `MSG-LEN SP
SYSLOG-MSG`, where `MSG-LEN` counts the bytes of the message; any other frame
is `SYSLOG-MSG LF`. The scanner reads bytes through the byte adapter, so one rune
is one byte and a token's length is its length in bytes, the unit of
`MSG-LEN`. Rules lex the length and the header of the message. The driver uses
the length only to frame the record: it counts the header bytes and reads the
rest of the frame from the cursor as `MSG`.

```sh
go run ./streams/syslog < streams/syslog/testdata/stream.input
go test ./streams/syslog
go test -timeout 0 -run Long ./streams/syslog -streamtest.long   # 1 GiB
```

The output has one line per token: the byte offset, the type and the length
in bytes, as in the [hex view](../../binary/README.md), then the bytes as a Go
quoted string. `MSG` may hold any byte, so its line is in the hex view: its
first 16 bytes in hex.

## Tokens

| Type | Matches |
| --- | --- |
| `MSG_LEN` | 1 to 10 digits followed by SP, at a frame start |
| `SP` | one space |
| `LF` | one line feed; it ends a newline-ended frame |
| `PRI` | `<`, 1 to 3 digits, `>` |
| `VERSION` | 1 to 3 digits |
| `TIMESTAMP` | the RFC 3339 shape of section 6.2.3 (`2003-10-11T22:14:15.003Z`, `2003-08-24T05:14:15.000003-07:00`) or `-` |
| `HOSTNAME`, `APP_NAME`, `PROCID`, `MSGID` | a run of printable ASCII (`!` to `~`), or `-` |
| `STRUCTURED_DATA` | `-`, the nil value of the structured data |
| `SD_ELEMENT_OPEN`, `SD_ELEMENT_CLOSE` | `[` and `]` around an element |
| `SD_ID` | the name after `[`: printable ASCII but `=`, `]` and `"` |
| `SD_PARAM_NAME` | a name followed by `=` |
| `SD_EQUALS` | `=` |
| `SD_QUOTE` | `"` around a parameter value |
| `SD_PARAM_VALUE` | the bytes between the quotes; a `\` and the byte after it (`\"`, `\\`, `\]`) stay together |
| `MSG` | the rest of the message, BOM included |
| `UNKNOWN` | a byte that no rule of the current mode matches, or a piece of a digit run that is not a `MSG_LEN` |

A field runs to its delimiter, so a run that has lost its delimiter cannot grow
one token: `HOSTNAME`, `APP_NAME`, `PROCID`, `MSGID`, `SD_ID`,
`SD_PARAM_NAME` and a newline-ended `MSG` are pieces of at most 1000 bytes,
`SD_PARAM_VALUE` pieces of at most 500 escape units (1000 bytes), and an
octet-counted `MSG` pieces of at most 64 KiB. A longer run is several tokens
of the same type in a row. The digit fields are bounded rules (`MSG_LEN` 10
digits, `PRI` and `VERSION` 3), and `TIMESTAMP` is a bounded shape, so no rule
reads more than 1000 bytes before it decides.

## The mode driver

The fields of the header have the same shape (a run of printable ASCII) and
differ only by their position, so the driver has one scanner per mode, all over
one cursor:

| Mode | Scanner | Next mode |
| --- | --- | --- |
| frame start | `MSG_LEN`, `SP`, and a digit run | after `MSG_LEN SP`, `PRI` with the length; after a digit run, the rest of the line |
| `PRI` | `PRI`, `LF` | `VERSION` |
| `VERSION`, `TIMESTAMP`, `HOSTNAME`, `APP_NAME`, `PROCID`, `MSGID` | the field, `SP`, `LF` | `SP` moves to the next field, then to the structured data |
| structured data | `SD_ELEMENT_OPEN`, `STRUCTURED_DATA`, `SP`, `LF` | `[` to the element; `SP` to `MSG` |
| element | `SD_PARAM_NAME`, `SD_ID`, `SD_EQUALS`, `SD_QUOTE`, `SP`, `SD_ELEMENT_CLOSE`, `LF` | `"` to the value; `]` back to the structured data |
| value | `SD_PARAM_VALUE`, `SD_QUOTE`, `LF` | `"` back to the element |
| `MSG` | `MSG`, `LF` (newline-ended frames only) | |

At a frame start the driver peeks at one byte. A digit starts an octet-counted
frame; any other byte starts a newline-ended frame, whose `LF` in any mode ends
it. In an octet-counted frame the driver adds up the lengths of the header
tokens, and after the `SP` that ends the structured data it reads the rest of
the frame from the cursor as `MSG`. An `LF` there is a byte of the message, not
the end of the frame.

A scanner over a shared cursor cannot stop at the frame end. When a header token
reaches the frame end, the frame ends after it, and when it passes the end (a
frame shorter than its header), the next frame starts after that token, at the
current position. `MSG_LEN` is not trusted beyond framing: a length longer than
the stream reads the rest of the stream as `MSG` pieces of 64 KiB, so memory
follows the piece size, not the length. `MSG` is a chunked field of a
`bytesource.Fields` queue. A field length is an `int`, and a 10-digit
`MSG_LEN` does not fit one on a 32-bit platform, so a longer `MSG` is queued
as several fields, one after the other (`streamkit.PushLong`, shared with
websocket and nats).

A run of digits that is not 1 to 10 digits followed by SP cannot be a length.
The driver emits it as `UNKNOWN` pieces of at most 1000 bytes
(`Token.WithType`), reads no `MSG`
by length, and lexes the rest of the line as a newline-ended message. The next
frame starts after that line's `LF`.

The scanners run on a [`modes.Driver`](https://github.com/xiam/bytescan/tree/master/modes). Together they follow
the three rules of a mode driver:

1. `modes.Driver` calls `Release()` on the outgoing scanner at every switch, and
   the driver calls it on the active scanner before it peeks at or reads the
   cursor itself.
2. It builds its scanners once, in `newDriver`, and never builds another one.
3. `Next` holds a mutex, so two scanners never run at the same time.

## Covered subset

- Both framings of RFC 6587, mixed in one stream: octet counting (section
  3.4.1) and non-transparent framing with `LF` as the trailer (section 3.4.2).
- The RFC 5424 header, structured data and message, with nil values. The
  scanner does not check field lengths, the PRI and VERSION values, or that a
  timestamp exists, and it does not decode the BOM or the UTF-8 of `MSG`.
- Not covered: RFC 3164 (BSD) messages, other trailers than `LF`, and TLS
  (RFC 5425), whose framing is the same octet counting.

## How invalid input lexes

- A byte that does not fit the current mode is `UNKNOWN`, and the mode stays.
  After `PRI`, an `SP` still moves to the next field, so a broken field does
  not shift the fields after it.
- A garbage line resynchronises at its `LF`. Garbage that does not end with
  `LF` runs into the message after it, which then lexes as a newline-ended
  message up to the next `LF`.
- A frame shorter than its header ends inside the header; the bytes after it
  start the next frame (`testdata/short-frame`).
- An input that ends inside a frame stops there with no error
  (`testdata/truncated`, `testdata/truncated-header`).

## Stream tests

`stream_test.go` proves the example on long input with the
[stream harness](https://github.com/xiam/bytescan/tree/master/streamtest). `generator_test.go` generates an
endless stream of both framings, nil values, escaped values, a BOM and
multi-byte UTF-8 in `MSG`, and `LF` inside octet-counted messages.

- `TestLongRun` lexes the default budget, or 1 GiB with `-streamtest.long`, with no
  `UNKNOWN`, one `PRI` per message and one `MSG_LEN` or `LF` per message.
- `TestCorruption` injects garbage lines (some start with a run of digits)
  between messages: each lexes as `UNKNOWN` tokens and its `LF`, and the next
  token is the `MSG_LEN` or `PRI` of the next frame.
- `TestUndelimitedRun` lexes runs of 18 KiB with no delimiter as `MSG`,
  `HOSTNAME` and `SD_PARAM_VALUE` pieces. The `MSG` and value runs are mostly
  bytes of `0x80` or above.
- `TestDigitRun` feeds 20 KiB of digits at a frame start: `UNKNOWN` pieces,
  then the frames after its line.
- `TestCorruptLength` frames the stream with a length of 9,999,999,999 bytes.

Every scanner has a per-token guard of 1,024 bytes (`streamtest.ByteGuard`,
which counts bytes), in `newScanner` itself, so `go run` is guarded too. The
guard is a safety net: every rule is bounded, the longest token is a piece of
1000 bytes, and no input reaches the guard. `TestGuard` shows
both sides on the production constructor: a newline-ended message that never
ends keeps producing `MSG` pieces of at most 1000 bytes, with no guard error
and memory under the harness ceiling, and a rule added in the test that grows
past the guard fails `Next` with `cursor.ErrRetentionExceeded`. The first messages of a stream grow the live heap
once, by about 300 KiB, as the scanners and the cursor allocate on first use; so
the run tests lex 4 KiB of messages before the run and take the first heap
sample after them.
