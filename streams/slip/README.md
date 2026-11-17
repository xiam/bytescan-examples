# streams/slip

A scanner for SLIP, the Serial Line Internet Protocol of
[RFC 1055](https://www.rfc-editor.org/rfc/rfc1055), built from `rules/core`
over the byte adapter of the [binary examples](../../binary).

**Technique:** frames separated by a delimiter byte, with byte stuffing inside
binary data. Every frame ends with `END` (`0xC0`). An `END` or `ESC` (`0xDB`)
byte in the payload is sent as a two-byte escape sequence: `ESC ESC_END`
(`0xDB 0xDC`) or `ESC ESC_ESC` (`0xDB 0xDD`). The scanner has four composed
rules and no driver; it lexes the escape sequences as tokens and never
unstuffs them.

```sh
go run ./streams/slip < streams/slip/testdata/frames.input
```

The output is the hex view of [`bytesource`](https://github.com/xiam/bytescan/tree/master/bytesource): one
line per token, `OFFSET TYPE LENGTH BYTES`, with the byte offset, the length
in bytes and the first 16 bytes in hex.

## Tokens

| Type | Matches |
| --- | --- |
| `END` | one `0xC0` byte (`octet.Set(0xC0)`): the end of a frame. Consecutive `END` bytes are separate tokens; the RFC allows empty frames |
| `DATA` | a run of bytes other than `0xC0` and `0xDB`, at most 1000 bytes: `core.RuneRunMax(dataByte, 1000)`. A longer run is several `DATA` tokens in a row |
| `ESC_END` | `0xDB 0xDC` (`octet.Literal`): an escaped `END` byte of the payload |
| `ESC_ESC` | `0xDB 0xDD` (`octet.Literal`): an escaped `ESC` byte of the payload |
| `UNKNOWN` | one `0xDB` that is not followed by `0xDC` or `0xDD`: by another byte, or by the end of the input |

The rules start with different bytes, except the two escape sequences, which
differ in their second byte, so no tie arises. A `0xDC` or `0xDD` byte that
does not follow `ESC` is plain data.

## Covered subset

The framing and the byte stuffing of RFC 1055. Not covered:

- **Unstuffing.** `ESC_END` and `ESC_ESC` are tokens; the scanner does not turn
  them back into the payload bytes `0xC0` and `0xDB`, and a frame is not
  reassembled into one payload.
- **The frames' contents.** A frame usually carries an IP packet; its bytes
  are `DATA` and are not parsed.
- **Frame size limits.** RFC 1055 suggests 1006 bytes; the scanner does not
  check the length of a frame.
- **Compressed SLIP (CSLIP, RFC 1144), PPP and HDLC framing.**

## How invalid input lexes

- **An `ESC` followed by a byte other than `0xDC` or `0xDD`** is one `UNKNOWN`
  byte. The byte after it is lexed on its own: `DATA`, the start of another
  escape sequence, or `END`. So an `ESC` right before `END` does not hide the
  `END`, and the next frame starts as usual. RFC 1055 leaves this case to the
  receiver (its sample code keeps the byte); the scanner only marks it.
- **An `ESC` at the end of the input** is `UNKNOWN`.
- **Line noise between frames** lands at the start of the next frame. Its
  bytes are `DATA`, `UNKNOWN` for each `ESC` that does not start an escape
  sequence, and `END` for a `0xC0`. Lexing does not depend on what came before
  an `END`, so the frames after the next `END` lex exactly as they would
  without the noise.
- **A stream that loses its `END` bytes** is a run of `DATA` tokens of 1000
  bytes each; memory does not grow with the run.

## Long-running streams

The tests use the stream harness
[`streamtest`](https://github.com/xiam/bytescan/tree/master/streamtest) (see
[Long-running streams](../../docs/streams.md)).
The generator writes one frame per record, the way RFC 1055 sends it:
sometimes an `END` first (an empty frame), then a payload of 0 to 299 bytes,
or 1,000 to 2,999 bytes for one frame in 16, stuffed, then `END`. Payload
bytes take every value, and about one in 8 is `END` or `ESC`.

The scanner has a per-token guard of `streamtest.ByteGuard` (1,024 bytes) in
`newScanner` itself, so `go run` is guarded too. The guard counts bytes, and a
`DATA` piece of 1000 bytes is the largest token, so the guard is a safety net
that no input reaches.

| Test | Shows |
| --- | --- |
| `TestGolden` | each `testdata` input against its golden file |
| `TestGuard` | the production guard: a run with no `END` keeps producing `DATA` pieces of at most 1000 bytes, with no guard error and memory under the harness ceiling; a rule added in the test that grows past the guard fails `Next` with `cursor.ErrRetentionExceeded` |
| `TestInputs` | each `testdata` input equals the bytes built in `inputs_test.go` |
| `TestEscapes` | both escape sequences, empty frames, and each `UNKNOWN` case |
| `TestLongRun` | a generated stream of frames: the tokens cover the stream without a gap, one `END`, `ESC_END` and `ESC_ESC` token for each the generator wrote, no `UNKNOWN`, long payloads in several `DATA` pieces, memory under the harness ceiling |
| `TestCorruption` | garbage runs of invalid escapes (`ESC` and a byte that does not finish an escape sequence) between frames: `UNKNOWN` exactly at each `ESC` of the garbage and nowhere else, and the same `END`, `ESC_END` and `ESC_ESC` tokens as the generator wrote, so lexing resumes at the `END` that closes each corrupted frame |
| `TestUndelimitedRun` | one run of the harness default budget (512 KiB, 64 KiB under the race detector) with no `END` or `ESC`, half of it bytes of `0x80` and above: only `DATA` tokens of exactly 1000 bytes (the last one shorter), memory under the harness ceiling, and the 1,024-byte guard never rejects a piece |

```sh
go test ./streams/slip
go test -timeout 0 -run Long ./streams/slip -streamtest.long   # 1 GiB
```

## Test data

| Case | Shows |
| --- | --- |
| `frames` | a leading `END`, a frame with the start of an IPv4 header that holds `0xC0 0xDB` (both escape sequences), two empty frames, a frame of the bytes `0x80` to `0x9F`, a frame that ends with an escaped `END`, and a frame of one escaped `ESC` |
| `invalid` | an `ESC` before `A`, an `ESC` before `END`, an `ESC` before an escape sequence, and an `ESC` at the end of the input |
| `long-run` | a run of 2,500 data bytes without a delimiter: `DATA` pieces of 1000, 1000 and 500 bytes, then `END` |
| `empty` | no input, no tokens |
| `text` | plain text: one `DATA` token |
