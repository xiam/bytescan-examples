# streams/mpegts

A scanner for MPEG transport streams, the broadcast container of
[ISO/IEC 13818-1](https://www.itu.int/rec/T-REC-H.222.0), section 2.4.3
(transport packet and adaptation field layout), built from `rules/core` over
the byte adapter of the [binary examples](../../binary).

**Technique:** fixed-size packets that each start with a sync byte, and
resynchronisation on that byte after corrupt data. Every packet is 188 bytes
and starts with `0x47`. A mode driver lexes the sync byte and the header with
rules, frames the adaptation field and the payload with the header's bits, and,
after a byte that should start a packet but is not `0x47`, skips bytes as
`UNKNOWN` until a `0x47` that another `0x47` one packet later confirms.

```sh
go run ./streams/mpegts < streams/mpegts/testdata/packets.input
```

The output is the hex view of [`bytesource`](https://github.com/xiam/bytescan/tree/master/bytesource): one
line per token, `OFFSET TYPE LENGTH BYTES`, with the byte offset, the length
in bytes and the first 16 bytes in hex.

## Tokens

| Type | Matches |
| --- | --- |
| `SYNC` | the sync byte `0x47` that starts a packet |
| `HEADER` | the 3 bytes after the sync byte: transport error, payload unit start and priority bits, the 13-bit PID, scrambling control, adaptation field control and continuity counter |
| `ADAPTATION_FIELD` | the adaptation field, when bit 5 of the last header byte is set (`adaptation_field_control` `10` or `11`): its length byte and that many bytes, but at most the 184 bytes left in the packet |
| `PAYLOAD` | the rest of the packet after the header and the adaptation field; no token when nothing is left |
| `UNKNOWN` | one byte that is not part of a packet: a byte where a packet should start that is not `0x47`, a byte skipped while the driver is lost, or a byte of a header cut by the end of the input |

## The mode driver

`driver.go` owns one byte cursor, three scanners over it, run by a
[`modes.Driver`](https://github.com/xiam/bytescan/tree/master/modes), and a `bytesource.Fields` queue:

- **In sync** (`newSyncScanner`), the next byte is where a packet should start.
  The sync scanner's one rule is `octet.Set(0x47)`. A `0x47` is `SYNC`, and the
  driver switches to the header scanner. Any other byte is `UNKNOWN`, and the
  driver is lost. The driver starts in sync, so a stream whose first byte is
  `0x47` starts with a packet.
- **In the header** (`newHeaderScanner`), the header scanner lexes the 3 bytes
  after the sync byte as `HEADER` (`octet.Fixed(3)`). When the header
  announces an adaptation field, the driver peeks at its length byte L and
  queues an `ADAPTATION_FIELD` of 1 + L bytes, at most 184. Then it queues the
  `PAYLOAD`: the rest of the 184 bytes after the header. It reads both from
  the cursor and is in sync again at the next packet. The length is used only
  to frame the field: a length above 183 makes the field end at the packet
  end, with no `PAYLOAD`, so the next packet still starts at its sync byte.
- **Lost** (`newLostScanner`), the lost scanner confirms a sync byte with a rule:
  `core.FollowedBy(octet.Set(0x47), core.Sequence(octet.Fixed(187),
  octet.Set(0x47)))`, a `0x47` that another `0x47` follows one packet later.
  It is `SYNC`, and the driver is in sync again. Any other byte, and a `0x47`
  that the byte 188 bytes further does not confirm, is one `UNKNOWN` byte, and
  the driver stays lost. A `0x47` less than 188 bytes before the end of the
  input has no byte to confirm it, so it is `UNKNOWN`, and the bytes stay
  `UNKNOWN` to the end of the input.

In sync the driver takes a `0x47` without the look-ahead, so a payload byte
`0x47` is never taken for a sync byte, and a stream lexes without reading
ahead. The look-ahead is used only to leave the lost state. An earlier version
of the driver peeked 188 bytes ahead itself; the look-ahead rule replaces that
code and lexes every test input the same way.

The driver follows the three rules of a
[mode driver](../../docs/standard.md#modes-several-scanners-over-one-cursor):

1. **Release on every switch.** `modes.Driver` calls `Release()` on the
   outgoing scanner at every switch, and the driver switches from the header
   scanner to the sync scanner before it peeks at or reads bytes from the cursor.
   `TestReleaseBeforeRead` lexes a corrupted stream over a cursor whose
   retention is bounded to 1 KiB.
2. **Cache one scanner per mode.** The driver builds its three scanners once, in
   `newDriver`, and never builds a scanner per token or drops one.
3. **One `Next()` at a time.** The scanners, the peek and the field reads share
   one cursor, so the driver's `Next` holds a mutex.

Every scanner returns a plain `io.EOF` at the end of the input. When the input
ends inside a field, the field token holds the bytes up to the end; when it
ends inside a header, the header bytes are `UNKNOWN`. Lexing ends normally.

## Covered subset

Packets of 188 bytes with the 4-byte header of ISO/IEC 13818-1, section
2.4.3.2, and the adaptation field framed by its length byte (section 2.4.3.4).
Every value of the header bits is lexed. Not covered:

- **PES packets, program tables (PAT, PMT and other PSI) and codecs.** The
  payload is one token; its bytes are not parsed.
- **The adaptation field's contents.** Its flags, PCR, splice countdown,
  private data and stuffing are not split; the field is one token.
- **Continuity checking.** The continuity counter is part of `HEADER`; the
  driver does not compare it between packets.
- **The meaning of the header bits.** `adaptation_field_control` `00` is
  reserved; the driver lexes such a packet like a payload-only packet. A packet
  with `10` (adaptation field only) and a short adaptation field still gets a
  `PAYLOAD` for the rest of the packet. A set transport error indicator does
  not change the lexing.
- **192-byte (M2TS, with a 4-byte timecode) and 204-byte (with Reed-Solomon
  bytes) packet variants.** In such a stream the sync bytes are not 188 bytes
  apart, so the driver loses sync after the first packet and stays lost.

## How invalid input lexes

- **A byte where a packet should start that is not `0x47`** is `UNKNOWN`, and
  the driver is lost.
- **While the driver is lost**, every byte is one `UNKNOWN` until a `0x47` that
  is followed by another `0x47` 188 bytes later. A `0x47` inside the corrupt
  data or in the payload of a damaged packet is `UNKNOWN` unless that check
  passes by chance.
- **An adaptation field length above 183** makes `ADAPTATION_FIELD` end at the
  packet end; there is no `PAYLOAD`, and the next packet starts at its sync
  byte.
- **A truncated final packet** gives the fields up to the end of the input:
  the last field holds the bytes that are left, and a cut header is `UNKNOWN`
  bytes.
- **Input that is not a transport stream** is all `UNKNOWN`, unless it starts
  with `0x47` (the driver starts in sync and takes it as a packet) or two
  `0x47` bytes are 188 bytes apart.

## Long-running streams

The tests use the stream harness
[`streamtest`](https://github.com/xiam/bytescan/tree/master/streamtest) (see
[Long-running streams](../../docs/streams.md)).
The generator writes one 188-byte packet per record on a video, an audio and
the null PID: payload-only packets, packets with an adaptation field (a PCR or
a flags byte, then stuffing) and a payload, adaptation-only packets, and null
packets of `0xFF` stuffing.

Every scanner has a per-token guard of 1,024 bytes, `streamtest.ByteGuard`, in
`newScanner` itself, so `go run` is guarded too. The guard counts bytes. The guard is a safety net: the largest token of a
rule is the 3-byte `HEADER`, the longest read is the 189 bytes of the
confirmed sync, and the adaptation field and the payload, at most 184 bytes,
are read by the driver, so no input reaches the guard.

| Test | Shows |
| --- | --- |
| `TestGolden` | each `testdata` input against its golden file |
| `TestGuard` | the production guard: a stream with no `0x47` keeps producing one-byte `UNKNOWN` tokens, with no guard error and memory under the harness ceiling; a rule added in the test that grows past the guard fails `Next` with `cursor.ErrRetentionExceeded` |
| `TestInputs` | each `testdata` input equals the bytes built in `inputs_test.go` |
| `TestLongRun` | a generated stream whose payload bytes take every value, `0x47` included: one `SYNC` and one `HEADER` per packet, every `SYNC` at a multiple of 188 bytes, no `UNKNOWN`, memory under the harness ceiling |
| `TestCorruption` | garbage runs of 1 to 400 bytes without `0x47` between packets whose only `0x47` bytes are sync bytes. Every token is checked against the rule: garbage bytes are `UNKNOWN`; a packet after garbage is lexed as a packet when the next packet follows it directly, and is all `UNKNOWN` when garbage or the end of the stream follows it; every other packet starts with `SYNC` |
| `TestReleaseBeforeRead` | rule 1 of the driver, with a cursor whose retention is bounded to 1 KiB |

```sh
go test ./streams/mpegts
go test -timeout 0 -run Long ./streams/mpegts -streamtest.long   # 1 GiB
```

## Test data

| Case | Shows |
| --- | --- |
| `packets` | five packets: payload only (a PAT section), an adaptation field with a PCR and a payload that holds `0x47` bytes, adaptation field only (length 183), a null packet, and an adaptation field of length 0 |
| `overrun` | adaptation field lengths 183 (with a payload announced, so the payload is empty), 200 and 255 (clamped to the packet end), then a packet that starts at its sync byte |
| `truncated` | a packet cut inside its payload |
| `truncated-header` | a packet cut inside its header: the header bytes are `UNKNOWN` |
| `resync` | garbage with an unconfirmed `0x47`, two packets, a packet whose sync byte is `0x48` and whose payload holds unconfirmed `0x47` bytes (all `UNKNOWN`), and two packets after the next confirmed sync byte |
| `empty` | no input, no tokens |
| `not-ts` | text with a `0x47` (`G`) that nothing confirms: all `UNKNOWN` |
