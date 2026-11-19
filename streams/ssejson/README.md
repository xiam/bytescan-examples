# streams/ssejson

A piped scanner for JSON inside Server-Sent Events: the shape of a streaming
HTTP API that sends JSON documents in `text/event-stream`. Two layers of
scanners run at the same time over one endless stream, with constant memory.
The outer layer lexes the Server-Sent Events of
[WHATWG HTML, "Server-sent events"](https://html.spec.whatwg.org/multipage/server-sent-events.html#parsing-an-event-stream).
The text of its `data` values forms a second stream, and the inner layer lexes
that stream as JSON ([RFC 8259](https://www.rfc-editor.org/rfc/rfc8259)).

**Technique:** two cursors joined by a pull pipe. The JSON layer reads its
cursor from a pipe, an `io.Reader` whose `Read` calls `Next()` on the SSE
layer. No goroutine runs, and no token of either layer is held longer than
the pipe's bounded queue. A JSON token can cross the line and piece
boundaries of the SSE layer.

```sh
go run ./streams/ssejson < streams/ssejson/testdata/document.input
```

## Tokens

The SSE tokens keep the types of [`streams/sse`](../sse): `BOM`, `EVENT`,
`DATA`, `ID`, `RETRY`, `FIELD`, `COLON`, `SPACE`, `VALUE`, `COMMENT`, `EOL`
and `EVENT_END`. The JSON tokens have the `JSON_` prefix:

| Mode | Type | Matches |
| --- | --- | --- |
| value | `JSON_BEGIN_ARRAY`, `JSON_END_ARRAY` | `[`, `]` |
| value | `JSON_BEGIN_OBJECT`, `JSON_END_OBJECT` | `{`, `}` |
| value | `JSON_NAME_SEPARATOR`, `JSON_VALUE_SEPARATOR` | `:`, `,` |
| value | `JSON_TRUE`, `JSON_FALSE`, `JSON_NULL` | `true`, `false`, `null` |
| value | `JSON_NUMBER` | `-?(0\|[1-9][0-9]{0,329})(\.[0-9]{1,330})?([eE][+-]?[0-9]{1,330})?` |
| value | `JSON_WHITESPACE` | space, tab and `\r`; in pieces of at most 1000 runes |
| both | `JSON_EOL` | one `\n`; in string mode it ends the string |
| both | `JSON_QUOTE` | `"`: opens a string in value mode, closes it in string mode |
| string | `JSON_STRING_TEXT` | unescaped characters (not `"`, `\` or U+0000 to U+001F); in pieces of at most 1000 runes |
| string | `JSON_ESCAPE` | one escape of RFC 8259 section 7: `\"`, `\\`, `\/`, `\b`, `\f`, `\n`, `\r`, `\t`, or `\u` and four hex digits |
| both | `UNKNOWN` | one rune that fits no rule of the mode |

The line feed is not JSON whitespace here, because the pipe ends every `data`
line with one (see below) and a line feed is where an unterminated string
ends.

**Bounded runs.** Every JSON token is at most 1000 runes. `JSON_STRING_TEXT`
and `JSON_WHITESPACE` are emitted in pieces of 1000 runes. `JSON_NUMBER` is
bounded as a whole token: each of its three digit runs (integer, fraction,
exponent) takes at most 330 digits, so the longest number, `-`, 330 digits,
`.`, 330 digits, `e+`, 330 digits, is 994 runes. A longer number is several
`JSON_NUMBER` tokens in a row: 1000 digits are numbers of 330, 330, 330 and
10 digits. A bound on each digit run alone at 1000 would allow one number of
about 3,000 runes, and the queue bound below rests on 1000-rune tokens.
Every scanner of both layers is built with `bytescan.WithMaxTokenBytes(4096)`,
a little above 1000 runes of at most 4 bytes, so a rule that scanned further
would fail `Next` with the limit error.

## The two layers

`newScanner` in `scanner.go` builds three parts:

- **The SSE layer** is the mode driver of
  [`ssedriver`](../sse/ssedriver), the driver of `streams/sse` moved into an
  importable package. It owns the first cursor, over the input.
- **The pipe** (`pipe.go`) is an `io.Reader`. On `Read` it calls `Next()` on
  the SSE driver until it gets a `VALUE` piece of a `data` field, and returns
  the UTF-8 text of that piece. After the `EOL` of a `data` line it returns
  one `\n`, as the SSE format appends a line feed to the data buffer. A `data`
  line with no colon or an empty value gives the `\n` alone. It returns at
  most one piece per `Read` and at most `len(p)` bytes: the cursor reads into
  the free tail of its buffer, which can be shorter than a piece, so the pipe
  keeps the rest for the next `Read` and reads no SSE token until it has
  returned it. It keeps every SSE token it read in a queue, in order. At the
  end of the SSE stream it returns `io.EOF`.
- **The JSON layer** (`driver.go`, `newJSONDriver`) is a
  [`modes.Driver`](https://github.com/xiam/bytescan/tree/master/modes) over the second cursor,
  `cursor.New(pipe)`, with a value mode and a string mode. `JSON_QUOTE`
  switches between them; in string mode `JSON_EOL` also returns to value
  mode.

The **combined driver** (`driver.go`, `driver`) implements `bytescan.Source`.
Its `Next()` returns the queued SSE tokens first, in order, and then the JSON
token whose read caused them. So the output is one deterministic sequence
that shows both layers.

**Driver rules.** Each cursor follows the three rules of a mode driver
separately. Each driver releases the outgoing scanner at every mode switch,
builds its scanners once, and holds a mutex in `Next`. A scanner of the JSON
cursor is inside `Next()` while a scanner of the SSE cursor runs, because the
JSON cursor's read calls the pipe. That is allowed: the two scanners do not
share a cursor, and each scanner locks its own `Next`. The combined driver's
`Next` holds its own mutex. The engine allocates a new rune array for the
text of every token (`nextToken` in `bytescan.go`, inline for a short
token and its own array for a longer one), so a queued SSE token stays valid
after later `Next()` calls, and the queue stores no copy.

**Two streams, two offsets.** The line, column and offsets of an SSE token
count in the input. Those of a JSON token count in the piped stream: the
`data` values, a line feed after each `data` line, and the keep-alives. So
JSON line 1 is the first `data` line, and each `data` line, and each
keep-alive line feed, starts a new JSON line.

**Read order.** The layers interleave by read order, not by source position,
and the SSE layer runs ahead of the JSON layer. The JSON cursor reads the pipe
only when it holds no whole rune. The engine reads one rune past a token only
while a rule could still go on: after a number, or a run of whitespace or
string text. So the SSE tokens that lead to a piece of a `data` value, or to
the line feed after it, come before the JSON token whose read pulled them.
The `EOL` of a `data` line comes before the `JSON_EOL` of its line feed, and
the SSE tokens of the next event come before the first JSON token of its
`data` (see `read-order`).

## The queue

A stream can carry little or no `data` for a long time: heartbeat comments,
other fields, garbage. The queue must not grow with it. So the pipe counts the
SSE tokens it queued since the last reset, and the runes of their text,
except the `VALUE` pieces of a `data` field. Those are queued but not counted,
because the pipe returns their text to the JSON layer. So a keep-alive never
comes between two `VALUE` pieces of one `data` line, and the JSON layer never
sees a line feed inside a `data` value that the stream does not have. The
`DATA`, `COLON`, `SPACE` and `EOL` of a `data` line are counted.

When a count has reached **64 tokens or 2,000 runes**, whichever comes first,
the pipe reads no further SSE token until the reset. Each `Read` first
returns bytes that it still holds (the rest of a piece, a pending line feed),
and otherwise returns a **keep-alive**: one line feed. The JSON scanner
returns it as a `JSON_EOL` without another read, so the combined driver can
return the queue and reset the counters. A keep-alive only adds a `JSON_EOL`
at the start of a JSON line, where a line feed already ended any string.

**Reset rule.** The counters reset when the combined driver has returned
every queued token, that is, before it calls the JSON driver's `Next()`
again. They do not reset when the pipe returns bytes. So one JSON `Next()`
queues at most one counted batch, by construction. Two earlier rules failed in
review:

- A reset after any returned byte let an empty or 1-byte `data` line end a
  run just below the threshold, and one JSON `Next()` then read 4 to 5
  batches.
- A reset after 4 returned bytes let a short `data` line such as `data:true`
  do the same for two batches, because its JSON token still needs more bytes.

**Worst case: 65 tokens and 3,000 runes.** The queue holds the SSE tokens
that one JSON `Next()` pulled. No JSON rule goes on past a line feed, and
none reads more than 1000 runes from where its token starts: a run stops at
1000 runes without a look-ahead. So one JSON `Next()` reads at most one
`VALUE` piece that it does not count, and the line end after it. The pipe
pulls a token only while both counts are below their thresholds, so the
counted tokens before that piece are at most 63 tokens and 1,999 runes. Two
shapes reach the bound:

1. **A batch that crosses a threshold.** A counted run just below a
   threshold, plus the one token that crosses it (up to 1000 runes): at most
   64 tokens and 2,999 runes.
2. **A batch under the thresholds, then one piece.** A counted run just
   under both thresholds, then the `VALUE` piece of a `data` line and its
   `EOL`. The JSON layer reads at most 1000 runes of them, but the `EOL` of a
   `\r\n` line end is 2 runes, so together they are up to 1,001 runes: at
   most 65 tokens and 3,000 runes. The record of queue-test stream `c-worst`
   builds it: 63 counted tokens of 1,999 runes, then a `data` line of 999
   tabs that ends in `\r\n`. The `JSON_WHITESPACE` of 999 tabs reads the line
   feed after it, so the queue holds 65 tokens of 3,000 runes.

**Measured.** The tests assert the stated bound, 65 tokens and 3,000 runes,
on every stream. The measured maxima (go1.25.4, linux/amd64):

| Streams | Tokens | Runes |
| --- | --- | --- |
| queue test, streams (a) to (e), `c-worst` and `c-mixed` | 65 | 3,000 |
| random search, 400 streams | 64 | 2,997 |
| long run, 512 KiB | 38 | 2,182 |

A queue at the worst case, 65 tokens of 3,000 runes, holds about **24,400
bytes** of live heap (22,392 bytes under the race detector; `TestQueueHeap`).
The engine stores the text of a token as a `[]rune`, 4 bytes per rune, so
3,000 runes take 12,000 bytes, and each token adds its struct and span. The tests
bound it at 32 KiB (`queueHeap`).

**Ceiling.** The queue is held on top of everything that the harness default
ceiling already allows. So this example sets its own ceiling with
`streamtest.WithCeiling`: the default ceiling plus twice `queueHeap`, as
margin for heap sampling. That is 320 KiB at the default 512 KiB budget,
112 KiB under the race detector (64 KiB), and 16 MiB + 64 KiB with `-streamtest.long`.
The corruption test runs a clean driver beside the corrupted one, so it allows
twice this ceiling.

## Covered subset

Every line form of the SSE grammar, as in `streams/sse`, and the JSON tokens
of RFC 8259 in the text of the `data` values. Not covered:

- **JSON parsing.** No nesting state, no check that brackets balance or that a
  document is complete, no decoding of escapes. A `\u` escape is checked for
  four hex digits; a surrogate is not checked for its pair.
- **Event dispatch.** No `event`, `id` or `retry` interpretation, and no
  reassembly of the data buffer beyond the line feed after each `data` line.
  A blank line does not end a document for the JSON layer.
- **A document split inside a token.** The pipe adds a line feed after every
  `data` line, as the format does. So a document split over `data` lines
  lexes as the same tokens only when it is split between tokens. A string cut
  by a line end is an unterminated string (see `unterminated`).

## How invalid input lexes

In value mode, a rune that starts no token is `UNKNOWN`: `<html>` is six
`UNKNOWN` tokens, `1.` is `JSON_NUMBER` `1` and `UNKNOWN` `.`, and `01` is the
numbers `0` and `1`. In string mode, a `\` that starts no escape is `UNKNOWN`
and the text after it is string text, and a raw control character is
`UNKNOWN`. A line feed ends an unterminated string, so lexing is correct
again from the next `data` line (see `not-json`, `unterminated`). The SSE
layer has no invalid input; a garbage run that joins a line changes that line
only.

## Test data

| Case | Shows |
| --- | --- |
| `document` | a realistic chat-completion stream: events, ids, `retry`, a keep-alive comment, and `data: [DONE]` |
| `split` | a document split between JSON tokens over four `data` lines, with a comment and a non-`data` field between them |
| `read-order` | the `EOL` of a `data` line before its `JSON_EOL`, and the SSE tokens of the next event before the first JSON token of its `data` |
| `piece-boundary` | a number and a literal that cross a 1000-rune `VALUE` piece boundary inside one `data` line, each one token |
| `long-string` | a string of more than 1000 runes, in two `JSON_STRING_TEXT` pieces |
| `unterminated` | a string ended by the line end of its `data` line, then a correct document |
| `no-data` | an event with no `data`, a comment, and `data` fields with an empty value, with and without the colon and the space |
| `line-endings` | `\r\n` and a bare `\r`: the pipe returns `\n` for each |
| `not-json` | text that is not JSON, near-misses of numbers and literals, bad escapes and a raw control character in a string |

The tests use the harness in [`streamtest`](https://github.com/xiam/bytescan/tree/master/streamtest). Every
scanner of both layers is built with the 4,096-byte per-token guard in every
test.

- **`TestPipeBytes`** reads the pipe of a generated stream with buffers of
  4,096, 1 and 3 bytes, and compares its bytes with the data buffer that a
  separate reading of the WHATWG rules gives: each `data` value, a `\n` after
  each `data` line, and the keep-alives at the offsets the pipe reports.
- **`TestQueue`** runs the queue-test streams (a) to (e), `c-worst` and `c-mixed`,
  each at the harness default budget under this example's ceiling, and
  asserts the worst case and the 1000-rune bound on every JSON token.
  `TestWorstCase` checks that the worst-case record reaches 65 tokens and
  3,000 runes with a `\r\n` line end, and 2,999 runes with `\n` or `\r`.
  **`TestQueueSearch`** lexes 400 random streams from fixed seeds (40 under
  the race detector) that mix comments, non-`data` fields and `data` lines of
  0 to 4,000 runes, with numbers whose digit runs have 1 to 1000 digits.
  **`TestQueueHeap`** measures the heap of the worst-case queue.
- **`TestLongRun`** lexes a generated stream of JSON documents in events:
  documents split between tokens over one to four `data` lines, comments and
  other fields between them, long strings and long `data` lines, long
  comments (keep-alives), events with no `data`, empty `data` values, and all
  three line ends. It checks that every type appears, that no token is
  `UNKNOWN`, that no JSON token is longer than 1000 runes, that the queue
  stays within the worst case, and that the goroutine count is the same before
  and after.
- **`TestCorruption`** injects garbage between events: runs that join the first
  line of the next event, and extra `data` lines of junk. A clean driver runs
  in step. The SSE layer resumes at the next line; the JSON layer lexes the
  junk as `UNKNOWN` tokens and then every document as the clean stream does.
- **`TestUndelimited`** lexes one `data` line of 20 KiB that opens a string
  and never closes it: `VALUE` pieces and `JSON_STRING_TEXT` pieces of at most
  1000 runes, under the ceiling. **`TestLongNumber`** shows that a number
  longer than the bound is several `JSON_NUMBER` tokens in a row.

The default tests take about 3 s, and about 5 s under the race detector: the
pipe, queue and search tests add about 2 s to the long-run, corruption and
undelimited tests that every stream example has. The tests that sample no
heap run in parallel, after the ones that do.

```sh
go test -timeout 0 -run Long ./streams/ssejson -streamtest.long
```
