# streams/websocket

A scanner for a stream of WebSocket frames, the base framing protocol of
[RFC 6455, section 5.2](https://www.rfc-editor.org/rfc/rfc6455#section-5.2),
built from `rules/core` over the byte adapter of the
[binary examples](../../binary).

**Technique:** a payload length whose field width depends on its own first
bits, and an optional masking key. The second header byte holds a 7-bit
length: 0 to 125 is the payload length, 126 announces a 16-bit length and 127
a 64-bit length in the bytes after the header. Its first bit, `MASK`, announces
a 4-byte masking key before the payload. A driver lexes the 2-byte header with
a rule, reads these bits, and frames the extended length, the masking key and
the payload from the cursor.

```sh
go run ./streams/websocket < streams/websocket/testdata/frames.input
```

The output is the hex view of [`bytesource`](https://github.com/xiam/bytescan/tree/master/bytesource): one
line per token, `OFFSET TYPE LENGTH BYTES`, with the byte offset, the length
in bytes and the first 16 bytes in hex.

## Tokens

| Type | Matches |
| --- | --- |
| `FRAME_HEADER` | the 2-byte header: `FIN`, `RSV1` to `RSV3` and the 4-bit opcode, then `MASK` and the 7-bit payload length |
| `EXTENDED_LENGTH` | the 2-byte (7-bit length 126) or 8-byte (7-bit length 127) payload length, big-endian |
| `MASKING_KEY` | the 4-byte masking key, when `MASK` is set |
| `PAYLOAD` | the payload, still masked, in pieces of at most 64 KiB: a longer payload is several `PAYLOAD` tokens in a row; no token for a payload length of 0 |
| `UNKNOWN` | the last byte of the input when it is a frame header cut after one byte |

## The driver

`driver.go` owns one byte cursor, one scanner over it and a `bytesource.Fields`
queue:

- **At a frame start**, the header scanner (`newHeaderScanner`) lexes the 2-byte
  header as `FRAME_HEADER` (`octet.Fixed(2)`); every value of the bits is
  lexed. The driver reads the 7-bit length and `MASK` from the second byte
  and queues the fields they announce: `EXTENDED_LENGTH` of 2 or 8 bytes, then
  `MASKING_KEY` of 4 bytes.
- **In the fields**, the driver reads the queued fields from the cursor. The
  bytes of `EXTENDED_LENGTH`, big-endian, are the payload length.
- **In the payload**, the driver reads the payload from the cursor as
  `PAYLOAD` pieces of at most 64 KiB, and is at a frame start again after the
  last one. The payload is a chunked field of the queue. A field length is an
  `int`, so a payload longer than the largest `int` is queued as several
  fields, one after the other (`streamkit.PushLong`, shared with syslog and
  nats).

The lengths are used only to frame the stream. The driver does not unmask the
payload with the masking key; `MASKING_KEY` is a field like the others. The
payload length is a `uint64`, so a 64-bit length of any value, its most
significant bit included (which RFC 6455 requires to be 0), frames a payload
that runs to the end of the input, and memory follows the piece size, not the
declared length.

The driver follows the three rules of a
[mode driver](../../docs/standard.md#modes-several-scanners-over-one-cursor):

1. **Release on every switch.** The driver calls `Release()` on the header
   scanner after every token, before it reads fields or the payload from the
   cursor. `TestReleaseBeforeRead` lexes a stream over a cursor whose
   retention is bounded to 1 KiB; it fails when the release is removed.
2. **Cache one scanner per mode.** The driver builds its scanner once, in
   `newDriver`, and never builds another one. With one scanner there is no mode
   to switch, so the driver does not use a `modes.Driver`.
3. **One `Next()` at a time.** The scanner and the field reads share one cursor,
   so the driver's `Next` holds a mutex.

The scanner returns a plain `io.EOF` at the end of the input. When the input ends
inside a field or a payload, the last token holds the bytes up to the end;
when it ends after the first byte of a header, that byte is `UNKNOWN`. Lexing
ends normally.

## Covered subset

The base framing of RFC 6455, section 5.2: the header, the three widths of the
payload length, and the masking key. Every value of the header bits is lexed.
Not covered:

- **Unmasking** (section 5.3). The payload is lexed as it is on the wire.
- **The meaning of the frame.** The driver does not check the opcode, the
  reserved bits, that a control frame is final and at most 125 bytes, that a
  length is written in its shortest form, or that client frames are masked.
- **Messages.** Fragments (section 5.4) are lexed frame by frame; they are not
  reassembled, and a text frame's UTF-8 is not validated.
- **The opening handshake** (section 4), which is HTTP, and **extensions**
  (section 9), such as `permessage-deflate`, which give the `RSV` bits a
  meaning.

## How invalid input lexes

WebSocket has no sync marker: nothing in a frame shows where it starts. Every
byte at a frame start is read as a header, so the driver cannot find the next
frame after corrupt data, and it never needs `UNKNOWN` except for a header cut
by the end of the input. This is a design decision of the example.

- **Garbage between frames** is read as a frame header with the length that
  the garbage gives. The stream stays out of frame from there on: the bytes
  after it lex as `FRAME_HEADER`, `EXTENDED_LENGTH`, `MASKING_KEY` and
  `PAYLOAD` tokens in the wrong places, to the end of the input, with no
  error.
- **A length larger than the rest of the input**, up to 2^64-1, reads the rest
  of the input as `PAYLOAD` pieces of at most 64 KiB and stops normally
  (`testdata/huge-length`).
- **A truncated frame** gives its fields up to the end of the input
  (`testdata/truncated`, `testdata/truncated-length`, `testdata/truncated-key`);
  a header cut after one byte is one `UNKNOWN` byte
  (`testdata/truncated-header`).
- **Input that is not WebSocket** lexes as frames all the same
  (`testdata/not-websocket`, a line of text).

## Long-running streams

The tests use the stream harness
[`streamtest`](https://github.com/xiam/bytescan/tree/master/streamtest) (see
[Long-running streams](../../docs/streams.md)).
The generator writes one message per record, from a client (masked frames with
a random key) or from a server (unmasked frames): text, binary, ping, pong and
close frames, and one in eight a text or binary message in 2 to 4 fragments
with a ping between two of them. Payloads have 7-bit and 16-bit lengths, and
record 32 of every 256 is a binary frame with a 64-bit length of 65,536 to
66,560 bytes, so every stream of the default budget, under the race detector
too, has all three widths. The generator records each frame it writes, so a
test checks the tokens of every frame as they arrive.

The scanner has a per-token guard of 1,024 bytes, `streamtest.ByteGuard`, in
`newScanner` itself, so `go run` is guarded too. The guard counts bytes. The guard is a safety net: the only token of a rule
is the 2-byte `FRAME_HEADER`, and the fields and the payload are read by the
driver, so no rule reads more than 2 bytes before it decides, no token runs
to a delimiter, and no input reaches the guard.

The run tests raise the harness ceiling by 256 KiB, the heap of one `PAYLOAD`
piece of 64 KiB (a token holds 4 bytes per rune): the harness samples the
heap while it holds the last token, so a sample taken at a full piece holds
one piece more than a baseline taken at a small token.

| Test | Shows |
| --- | --- |
| `TestGolden` | each `testdata` input against its golden file |
| `TestGuard` | the production guard: a frame with the 64-bit length 2^64-1 followed by 256 KiB of any bytes keeps producing `PAYLOAD` pieces of at most 64 KiB, with no guard error and memory under the ceiling; a rule added in the test that grows past the guard fails `Next` with `cursor.ErrRetentionExceeded` |
| `TestInputs` | each `testdata` input equals the bytes built in `inputs_test.go` |
| `TestLongRun` | a generated stream of the default budget, or 1 GiB with `-streamtest.long`: every token matches the frame the generator wrote, no `UNKNOWN`, all three length widths, masked and unmasked frames, memory under the ceiling |
| `TestCorruption` | runs of 1 to 64 random bytes between messages. Every token before the first run matches its frame. From the first run on, the stream cannot be re-framed: the test checks that the tokens cover the stream without a gap, that each has a size the driver gives (a `PAYLOAD` piece at most 64 KiB), and that lexing reaches the end with no error under the ceiling |
| `TestCorruptionMaximalLength` | the same with garbage that is the header of a masked frame with the 64-bit length 2^64-1: after the first run, the stream is that one frame, its payload in pieces to the end |
| `TestHugeLength` | a frame with the 64-bit length 2^63-1 before a generated stream: the rest of the stream is `PAYLOAD` pieces of at most 64 KiB, and lexing stops normally under the ceiling |
| `TestReleaseBeforeRead` | rule 1 of the driver, with a cursor whose retention is bounded to 1 KiB |

```sh
go test ./streams/websocket
go test -timeout 0 -run Long ./streams/websocket -streamtest.long   # 1 GiB
```

## Test data

| Case | Shows |
| --- | --- |
| `frames` | the examples of RFC 6455, section 5.7: an unmasked and a masked `Hello` and an unmasked ping (7-bit lengths), a 256-byte unmasked and a 300-byte masked binary frame (16-bit lengths), a masked ping with no payload, and a masked close frame |
| `pieces` | a masked binary frame with a 64-bit length of 65,537 bytes, two `PAYLOAD` pieces, then a pong |
| `fragmented` | a masked text message in three fragments, with a ping between the first two; the server's pong and close frame |
| `truncated` | a frame and a second frame cut inside its payload |
| `truncated-header` | a header cut after one byte: one `UNKNOWN` byte |
| `truncated-length` | a header and a 64-bit length cut after 3 bytes |
| `truncated-key` | a masked frame cut inside its masking key |
| `huge-length` | the 64-bit length 2^64-1 before 22 bytes: the payload runs to the end |
| `empty` | no input, no tokens |
| `not-websocket` | a line of text, lexed as frames with 7-bit lengths |
