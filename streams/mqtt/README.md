# streams/mqtt

A scanner for a stream of MQTT control packets, as sent over one TCP
connection, following
[MQTT Version 3.1.1](https://docs.oasis-open.org/mqtt/mqtt/v3.1.1/os/mqtt-v3.1.1-os.html),
section 2 (control packet format) and section 3.3 (PUBLISH), built from
`rules/core` over the byte adapter of the [binary examples](../../binary).

**Technique:** a length encoded in 1 to 4 variable-length bytes. Each byte of
the "remaining length" holds 7 bits of the length, least significant group
first, and its high bit says that another byte follows; the same varint idea
frames Protobuf streams. A rule lexes the length bytes, and a mode driver uses
their value to frame the rest of the packet. MQTT has no sync marker, so after
corrupt data the driver cannot find the next packet.

```sh
go run ./streams/mqtt < streams/mqtt/testdata/session.input
```

The output is the hex view of [`bytesource`](https://github.com/xiam/bytescan/tree/master/bytesource): one
line per token, `OFFSET TYPE LENGTH BYTES`, with the byte offset, the length
in bytes and the first 16 bytes in hex.

## Tokens

| Type | Matches |
| --- | --- |
| `FIXED_HEADER` | the first byte of a packet: the packet type in the high four bits, the flags in the low four (section 2.2.1) |
| `REMAINING_LENGTH` | 1 to 4 bytes: at most three bytes with the high bit set, then one without (section 2.2.3) |
| `VARIABLE_HEADER` | in PUBLISH: the 2-byte topic length, the topic, and the 2-byte packet identifier when the QoS bits are not 0 (section 3.3.2); it ends at the packet end when the packet is shorter |
| `PAYLOAD` | in PUBLISH: the rest of the packet after `VARIABLE_HEADER`, in pieces of at most 64 KiB |
| `BODY` | in every other packet type: the bytes that the remaining length counts, in pieces of at most 64 KiB |
| `UNKNOWN` | one byte of an invalid remaining length: a fourth byte with the high bit still set, or a length cut by the end of the input |

`VARIABLE_HEADER`, `PAYLOAD` and `BODY` are not emitted when the remaining
length leaves no byte for them. A `VARIABLE_HEADER` longer than 64 KiB (a
topic of up to 65,535 bytes) is several pieces too.

## The mode driver

`driver.go` owns one byte cursor and three scanners over it, run by a
[`modes.Driver`](https://github.com/xiam/bytescan/tree/master/modes):

- **Header** (`newHeaderScanner`): one byte, any value, is `FIXED_HEADER`. The
  reserved packet types 0 and 15 and every flag value are lexed alike.
- **Length** (`newLengthScanner`): `REMAINING_LENGTH` is
  `[\x80-\xff]{0,3}[\x00-\x7f]`, so the rule never reads more than four bytes
  before it decides. The driver computes the length and frames the packet.
  For `PUBLISH` (type 3) it peeks at the next two bytes, the topic length, and
  frames a `VARIABLE_HEADER` of 2 + topic length bytes, plus 2 for the packet
  identifier when the QoS bits (bits 2 and 1 of the fixed header) are not 0,
  at most the remaining length; the rest is `PAYLOAD`. For any other type the
  remaining length is `BODY`. When the fourth byte still has the high bit set,
  `REMAINING_LENGTH` does not match, and the `CONTINUATION` rule matches the
  first length byte: the driver emits it as `UNKNOWN` (`Token.WithType`) and
  goes to the bad length.
- **Bad length** (`newBadLengthScanner`): one byte at a time, a byte with the
  high bit set (`CONTINUATION`) or without it (`LENGTH_END`). The driver emits
  both as `UNKNOWN`; after `LENGTH_END` a new packet starts. A run of `0xFF`
  bytes therefore lexes as one `UNKNOWN` per byte, however long it is.
- **Fields**: the driver reads `VARIABLE_HEADER`, then `PAYLOAD` or `BODY`,
  from the cursor through a `bytesource.Fields` queue, each in pieces of at
  most 64 KiB (the field's `MaxPiece`), so memory follows the piece size and
  not the declared length, which may be corrupt. Then a new packet starts.

The driver uses the remaining length and the topic length only to frame the
fields. `CONTINUATION` and `LENGTH_END` are internal types; the output never
shows them.

The driver follows the three rules of a
[mode driver](../../docs/standard.md#modes-several-scanners-over-one-cursor):

1. **Release on every switch.** `modes.Driver` calls `Release()` on the
   outgoing scanner at every switch, and after a `REMAINING_LENGTH` the driver
   switches to the idle header scanner before it peeks at the topic length or
   reads a field. `TestReleaseBeforeRead` lexes a variable header and a
   payload of more than 64 KiB over a cursor whose retention is bounded to
   1 KiB.
2. **Cache one scanner per mode.** The driver builds its three scanners once, in
   `newDriver`, and never builds a scanner per token or drops one.
3. **One `Next()` at a time.** The scanners, the peek and the field reads share
   one cursor, so the driver's `Next` holds a mutex.

Every scanner returns a plain `io.EOF` at the end of the input. When the input
ends inside a field, the field token holds the bytes up to the end. Lexing
ends normally.

## Covered subset

The fixed header and the remaining length of every MQTT 3.1.1 control packet,
and the variable header of `PUBLISH` framed by its topic length and QoS.
Not covered:

- **Session logic.** The driver does not pair `PUBLISH` with `PUBACK`, track
  packet identifiers, or know which side sent a packet.
- **The variable header and payload of other packet types.** `CONNECT` flags,
  the client identifier, `SUBSCRIBE` topic filters and the rest are one `BODY`
  token.
- **The topic and the payload.** `VARIABLE_HEADER` and `PAYLOAD` are bytes;
  the topic is not checked for UTF-8 or wildcards, and the payload is not
  decoded.
- **Validation.** Reserved packet types, flags that section 2.2.2 fixes,
  QoS 3 (taken as a QoS above 0), and remaining lengths that use more bytes
  than needed (`0x82 0x80 0x00` is the length 2 in 3 bytes) are lexed without
  judgment.
- **MQTT 5 properties.** An MQTT 5 packet has the same fixed header and
  remaining length, so it is framed, but its properties are part of `BODY`,
  `VARIABLE_HEADER` or `PAYLOAD`.
- **WebSocket transport.** The input is the raw byte stream of a TCP
  connection.

## How invalid input lexes

- **A remaining length of 5 or more bytes** (a fourth byte with the high bit
  set): the fixed header is `FIXED_HEADER`, and every byte from the first
  length byte up to and including the first byte without the high bit is one
  `UNKNOWN`. The next byte is a new `FIXED_HEADER`.
- **A run of `0xFF` bytes** where a packet starts: the first byte is a
  `FIXED_HEADER`, every other byte is `UNKNOWN`, and so is the first byte
  without the high bit after the run, even when it is the first byte of a real
  packet.
- **A truncated packet** gives its tokens up to the end of the input: a
  length cut short is `UNKNOWN` bytes, and a cut field holds the bytes that
  are left.
- **Corrupt data between packets.** MQTT has no sync marker and no delimiter,
  so the driver frames the next packet at the byte where the last one ended,
  whatever that byte is. Corrupt bytes give wrong `FIXED_HEADER` and
  `REMAINING_LENGTH` tokens, and a corrupt length of up to 268,435,455 bytes
  makes the driver read that many bytes as fields. Lexing goes on, memory stays
  bounded, and the driver finds real packets again only by chance (or when the
  corrupt bytes end an invalid length just before a packet). Re-framing needs
  the transport: a client closes the connection.

## Long-running streams

The tests use the stream harness
[`streamtest`](https://github.com/xiam/bytescan/tree/master/streamtest) (see
[Long-running streams](../../docs/streams.md)).
The generator writes one packet per record: `CONNECT`, `PUBLISH` at QoS 0, 1
and 2 (half of the packets), `PUBACK`, `SUBSCRIBE`, `PINGREQ` and `PINGRESP`.
Payloads take every byte value; one `PUBLISH` in 512 has a payload of 16 KiB
to 80 KiB. The remaining length is 1 to 4 bytes at random, padded with
continuation bytes when the value needs fewer, so every width shows up without
writing 2 MiB packets.

Every scanner has a per-token guard of 1,024 bytes, `streamtest.ByteGuard`, in
`newScanner` itself, so `go run` is guarded too. The guard is a safety net: the
longest token of a rule is a 4-byte `REMAINING_LENGTH`, and the fields are
read by the driver in pieces, so no input reaches the guard.

| Test | Shows |
| --- | --- |
| `TestGolden` | each `testdata` input against its golden file |
| `TestGuard` | the production guard: a `CONNECT` with the maximal remaining length followed by 256 KiB of any bytes keeps producing `BODY` pieces of at most 64 KiB, with no guard error and memory under the harness ceiling; a rule added in the test that grows past the guard fails `Next` with `cursor.ErrRetentionExceeded` |
| `TestInputs` | each `testdata` input equals the bytes built in `inputs_test.go` |
| `TestPieces` | a `PUBLISH` with a 65,539-byte variable header and a 70,000-byte payload: two pieces each, at most 64 KiB, then the next packet |
| `TestLengthValue` | the value of a remaining length against the examples of section 2.2.3 |
| `TestReleaseBeforeRead` | rule 1 of the driver, with a cursor whose retention is bounded to 1 KiB |
| `TestLongRun` | one `FIXED_HEADER` and one `REMAINING_LENGTH` per packet, every width from 1 to 4 bytes, one `VARIABLE_HEADER` per `PUBLISH`, one `BODY` per other packet with a length above 0, no `UNKNOWN`, the tokens without a gap, memory under the harness ceiling but at least two pieces (128 KiB) |
| `TestCorruption/invalid_length` | garbage runs that lex as an invalid remaining length: a `FIXED_HEADER` and `UNKNOWN` bytes, then every packet after it framed again, memory under the harness ceiling but at least two pieces (128 KiB) |
| `TestCorruption/any_bytes` | garbage runs of random bytes: every packet before the first run is framed; after it the driver frames packets at the wrong bytes, and lexing reaches the end of the stream without an error, without a gap and under the harness ceiling but at least two pieces (128 KiB) |
| `TestMaxLength` | a `CONNECT` and a `PUBLISH` whose remaining length is 268,435,455 bytes before a much shorter stream: the rest of the stream is read as pieces of at most 64 KiB, under the harness ceiling |
| `TestHighRun` | a run of 18 KiB of `0xFF` where a packet starts: one `FIXED_HEADER`, then one `UNKNOWN` per byte, under the harness ceiling and the guard |

```sh
go test ./streams/mqtt
go test -timeout 0 -run Long ./streams/mqtt -streamtest.long   # 1 GiB
```

## Test data

| Case | Shows |
| --- | --- |
| `session` | a realistic exchange: `CONNECT`, `CONNACK`, `SUBSCRIBE`, `SUBACK`, `PUBLISH` at QoS 0, 1 (with `RETAIN`) and 2 (with `DUP`) with text, JSON and binary payloads, `PUBACK`, `PUBREC`, `PUBREL`, `PUBCOMP`, `PINGREQ`, `PINGRESP`, `DISCONNECT` |
| `lengths` | remaining lengths of 1 byte, 2 bytes (207), and 3 and 4 bytes that encode small lengths with padding |
| `invalid-length` | remaining lengths of 5 and 6 bytes: `UNKNOWN` bytes up to the first byte without the high bit, then the next `FIXED_HEADER` |
| `ff-run` | a run of 48 `0xFF` bytes between packets: `UNKNOWN` bytes, then packets framed at the wrong bytes |
| `zero-length` | packets with a remaining length of 0: `PINGREQ`, `PINGRESP`, `DISCONNECT`, and a `PUBLISH` with no `VARIABLE_HEADER` |
| `short-publish` | `PUBLISH` packets whose remaining length ends inside the variable header |
| `truncated` | a `PUBLISH` with the maximal remaining length, 268,435,455 bytes (`ff ff ff 7f`), cut after 12 bytes |
| `truncated-length` | a packet cut inside its remaining length: the length bytes are `UNKNOWN` |
| `empty` | no input, no tokens |
