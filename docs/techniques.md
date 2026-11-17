# Examples by technique

This page lists every example with the lexing technique it shows, grouped by
use case. Each example is chosen for its technique, not for completeness. To
pick a technique for your own format, start with bytescan's
[technique chooser](https://github.com/xiam/bytescan/blob/master/docs/guides/README.md):
it links one guide per use case, and each guide links the examples that show
it.

Each group links the guide for its use case.

## Programming languages

Guide: [programming-language.md](https://github.com/xiam/bytescan/blob/master/docs/guides/programming-language.md).

| Example | Technique |
| --- | --- |
| [hello](../hello) | the [README](https://github.com/xiam/bytescan/blob/master/README.md#example-text) text example: a keyword before the word rule |
| [sql](../sql) | keywords that ignore case |
| [go](../go) | semicolon insertion as a filter, checked against `go/scanner` |
| [python](../python) | `INDENT` and `DEDENT` from a filter |
| [whitespace](../whitespace) | the inverted case: layout is the only token, all visible text is comment |
| [lua](../lua) | long brackets through `modes` and a counted match |
| [typescript](../typescript) | regex versus division, nested template literals, through `modes` |
| [ruby](../ruby) | interpolation, percent literals, heredocs, through `modes` |

## Data formats

Guide: [data-format.md](https://github.com/xiam/bytescan/blob/master/docs/guides/data-format.md).

| Example | Technique |
| --- | --- |
| [json](../json) | a strict token grammar from checked escapes and number options; the reference example |

## Terminal streams

Guide: [terminal-stream.md](https://github.com/xiam/bytescan/blob/master/docs/guides/terminal-stream.md).

| Example | Technique |
| --- | --- |
| [ansi](../ansi) | control data mixed into text (ECMA-48), and a strip filter |

## Human-language text

Guide: [human-text.md](https://github.com/xiam/bytescan/blob/master/docs/guides/human-text.md).

| Example | Technique |
| --- | --- |
| [words](../words) | registration order between overlapping rules; skipping layout |
| [english](../english) | words against punctuation, with contractions and hyphens |
| [spanish](../spanish) | words against punctuation, with inverted marks and accents |
| [arabic](../arabic) | combining marks in right-to-left text |
| [japanese](../japanese) | script runs where words have no spaces |
| [korean](../korean) | a script with spaces whose syllables have two encodings |

## Line-oriented text protocols

Guide: [line-protocol.md](https://github.com/xiam/bytescan/blob/master/docs/guides/line-protocol.md).

| Example | Technique |
| --- | --- |
| [streams/sse](../streams/sse) | records separated by a blank line, through a mode driver in its own package (`ssedriver`) |
| [streams/nmea](../streams/nmea) | a start character and a checksum field; resynchronisation |
| [streams/nats](../streams/nats) | text control lines with byte-counted payloads, through the byte adapter and chunked fields |

## Length-prefixed protocols

Guide: [length-prefixed.md](https://github.com/xiam/bytescan/blob/master/docs/guides/length-prefixed.md).

| Example | Technique |
| --- | --- |
| [streams/mqtt](../streams/mqtt) | a variable-length remaining length |
| [streams/websocket](../streams/websocket) | a self-describing length width, a masking key and a 64-bit length in int-sized fields |
| [streams/syslog](../streams/syslog) | a decimal length prefix (octet counting) through the byte adapter and a chunked field |

## Binary files

Guide: [length-prefixed.md](https://github.com/xiam/bytescan/blob/master/docs/guides/length-prefixed.md). The files read the
fields their headers announce as the protocols do.

| Example | Technique |
| --- | --- |
| [binary/zip](../binary/zip) | bytes through the byte adapter [`bytesource`](https://github.com/xiam/bytescan/tree/master/bytesource) and the byte rules [`rules/octet`](https://github.com/xiam/bytescan/tree/master/rules/octet); length-prefixed fields |
| [binary/bmp](../binary/bmp) | fixed headers and an offset-driven payload, read in pieces |
| [binary/jpeg](../binary/jpeg) | marker segments, entropy-coded data, EXIF |
| [binary/wav](../binary/wav) | a chunked container through the byte adapter |
| [binary/mp3](../binary/mp3) | frames whose length is computed, not stored |

## Fixed-size packets with a sync marker

Guide: [sync-marker.md](https://github.com/xiam/bytescan/blob/master/docs/guides/sync-marker.md).

| Example | Technique |
| --- | --- |
| [streams/mpegts](../streams/mpegts) | fixed-size packets that start with a sync byte; a look-ahead rule confirms the sync |

## Escape-framed bytes

Guide: [escape-framed.md](https://github.com/xiam/bytescan/blob/master/docs/guides/escape-framed.md).

| Example | Technique |
| --- | --- |
| [streams/slip](../streams/slip) | a delimiter byte and byte stuffing, with byte rules |

## Nested formats

Guide: [nested-formats.md](https://github.com/xiam/bytescan/blob/master/docs/guides/nested-formats.md).

| Example | Technique |
| --- | --- |
| [streams/ssejson](../streams/ssejson) | piped scanners: JSON inside Server-Sent Events, two cursors |

## The cursor on its own

Package: [`bytescan/cursor`](https://github.com/xiam/bytescan/tree/master/cursor).
These programs use the cursor without a scanner, so the [example standard](standard.md)
does not apply to them; each has a test that runs it on a fixed input.

| Example | Technique |
| --- | --- |
| [cursor/positioned-cursor](../cursor/positioned-cursor) | a checkpoint held across a read-ahead; span text and context from the retained input |
| [cursor/surrounding-text](../cursor/surrounding-text) | read to a marker, then seek back and read the text around it |

A new example adds its row to the group of its use case, on this page and in
the [README catalogue](../README.md#the-catalogue), and the guide of that
use case links it.
