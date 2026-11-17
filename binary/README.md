# binary

Scanners for five binary file formats, built from `rules/octet`, `rules/core`,
`bytesource` and `modes`: ZIP archives
([PKWARE APPNOTE 6.3.10](https://pkware.cachefly.net/webdocs/casestudies/APPNOTE.TXT)),
BMP images (the BMP file header and `BITMAPINFOHEADER`,
[Microsoft documentation](https://learn.microsoft.com/en-us/windows/win32/gdi/bitmap-storage)),
and JPEG images with their Exif segment
([ITU T.81, Annex B](https://www.w3.org/Graphics/JPEG/itu-t81.pdf);
[Exif 2.3, section 4.5](https://www.cipa.jp/std/documents/e/DC-008-2012_E.pdf);
[TIFF 6.0, section 2](https://www.itu.int/itudoc/itu-t/com16/tiff-fx/docs/tiff6.pdf)),
WAV audio files (the RIFF `WAVE` form,
[Multimedia Programming Interface and Data Specifications 1.0](https://www.mmsp.ece.mcgill.ca/Documents/AudioFormats/WAVE/Docs/riffmci.pdf)),
and MP3 audio files with an ID3v2 tag
([ID3v2.3](https://id3.org/id3v2.3.0);
[ID3v2.4 structure](https://id3.org/id3v2.4.0-structure);
the Layer III frame header of ISO/IEC 11172-3 and ISO/IEC 13818-3, section
2.4.2.3).

**Technique:** byte-oriented input through a byte-to-rune adapter, byte rules,
and length-prefixed fields read by a driver. The scanner reads runes, so an
adapter returns each byte as one rune. Rules from `rules/octet` match the fixed
parts of a format: its signatures, markers and fixed-size headers. Where a
field's length is stored in the data, the driver reads the length from the
header token and reads that many bytes from the shared cursor as one token,
so bytes in a payload are never taken for a marker. A field that runs to the
end of the input is read in pieces of at most 64 KiB.

```sh
go run ./binary/zip < binary/zip/testdata/document.input
go run ./binary/bmp < binary/bmp/testdata/document.input
go run ./binary/jpeg < binary/jpeg/testdata/document.input
go run ./binary/wav < binary/wav/testdata/document.input
go run ./binary/mp3 < binary/mp3/testdata/document.input
```

## The byte adapter and the hex view

The public package [`bytesource`](https://github.com/xiam/bytescan/tree/master/bytesource) is shared by the five
examples:

| Name | Does |
| --- | --- |
| `NewReader(r)` | an `io.RuneReader` that returns each byte of `r` as one rune, U+0000 to U+00FF |
| `NewCursor(r)` | a byte-mode `cursor.Reader` over `r`, for `bytescan.NewWithCursor` |
| `Text(s)` | the rule text for the bytes of `s`: `core.Literal(bytesource.Text("\xff\xd8"))` matches the bytes `FF D8` |
| `Bytes(text)` | the bytes of a token's text, the inverse of `Text` |
| `AnyByte` | the class of every byte, for compositions of core classes |
| `Read(cur, typ, n)` | reads up to `n` bytes from the cursor as one token of type `typ` |
| `Peek(cur, n)` | the next `n` bytes, without moving the cursor |
| `Fields` | a queue of fields (`Field{Type, Len, MaxPiece, Then, Done}`) that a driver steps with `NextField`, in order |
| `Hex` | the hex view |

[`hexdump.Dumper`](../internal/hexdump) is the `lexdump.Dumper` that writes the hex view.

The rules use the public package [`rules/octet`](https://github.com/xiam/bytescan/tree/master/rules/octet):
`Literal` for signatures and markers, `Fixed(n)` and `Record(sig, n)` for
fixed-size fields, and `Any`, `Set` and `Range` for one byte. A union of byte
ranges, such as the JPEG marker bytes, is a `core.Choice` of `octet.Range`
rules. The octet rules take byte values, so they need no `Text`. A core rule
that matches bytes uses `Text` for every byte of 0x80 or above. A Go string
such as `"\xff\xd8"` is not valid UTF-8: `core.Literal("\xff\xd8")` decodes it
to U+FFFD and never matches the byte runes, and it does not fail.

The hex view writes one line per token:

```text
OFFSET TYPE LENGTH BYTES
```

`OFFSET` is the byte offset of the token in the input and `LENGTH` its length
in bytes, in decimal. `BYTES` are its first 16 bytes in hex, followed by `...`
when the token is longer. The view prints byte offsets, not the
`LINE:COLUMN` of the text view: the cursor counts a line at every 0x0A byte.
Because each byte is one rune, `Token.Offset()` and `Token.Len()` are byte
offsets and byte lengths. The cursor counts bytes, so `Token.ByteOffset()` and
`Span.Bytes()` are the same file offsets and lengths.

## The drivers

Each example has a driver in `driver.go` that owns one byte cursor, a
[`modes.Driver`](https://github.com/xiam/bytescan/tree/master/modes) that holds the scanners over it, and a
`bytesource.Fields` queue. Its `Next` returns the next queued field, or else
the next token of the active scanner. After a header token, the
driver reads the lengths from the header's bytes and pushes the fields that
follow it. A field cut by the end of the input holds the bytes up to the end;
the rest of the queue is dropped. A field of length 0 gives no token.

| Example | Scanners (modes) | Fields read from the cursor |
| --- | --- | --- |
| zip | records | `FILE_NAME`, `EXTRA_FIELD`, `FILE_DATA`, `FILE_COMMENT`, `ZIP_COMMENT` |
| bmp | file header | `DIB_HEADER`, `COLOR_TABLE`, `GAP`, `PIXEL_DATA` |
| jpeg | marker, scan, TIFF | `SEGMENT_PAYLOAD`, `TIFF_DATA`, `IFD_ENTRY_COUNT`, `IFD_ENTRY` |
| wav | file, chunk | `FMT_CHUNK_PAYLOAD`, `DATA_CHUNK_PAYLOAD`, `CHUNK_PAYLOAD`, `PAD_BYTE` |
| mp3 | start, tag, audio | `ID3_FRAME_BODY`, `ID3_PADDING`, `FRAME_BODY` |

Every driver follows the three rules of a
[mode driver](../docs/standard.md#modes-several-scanners-over-one-cursor):

1. **Release on every switch.** `modes.Driver.Use` calls `Release()` on the
   outgoing scanner at every mode switch, and the driver calls it on the active
   scanner before it reads fields from the cursor. An
   idle scanner keeps checkpoints that pin all later input; a payload read
   without the release would be held in memory, and a cursor with a retention
   bound would fail. `TestReleaseBeforeData` in zip,
   `TestReleaseBeforePayload` in wav and `TestReleaseBeforeBody` in mp3 check
   this with a 1 KiB bound and a payload of 64 KiB; each test fails when the
   release before the payload read is removed. `TestPixelDataPieces` in bmp
   checks the same bound for pixel data read in pieces.
2. **Cache one scanner per mode.** Each driver builds its scanners once, in
   `newScanner`, and never builds a scanner per token or drops one.
3. **One `Next()` at a time.** The scanners and the field reads share one
   cursor, so the driver's `Next` holds a mutex. The mutex of `modes.Driver`
   covers only its scanners, not the field reads.

Every scanner returns a plain `io.EOF` at the end of the input. So only the
driver knows that the input ended inside a field or a scan. It does not report
that: the last token holds the bytes up to the end, and lexing ends normally.

## zip

**Tokens:**

| Type | Matches |
| --- | --- |
| `LOCAL_FILE_HEADER` | `PK\x03\x04` and the 26 fixed bytes after it (APPNOTE 4.3.7) |
| `CENTRAL_DIRECTORY_HEADER` | `PK\x01\x02` and the 42 fixed bytes after it (4.3.12) |
| `END_OF_CENTRAL_DIRECTORY` | `PK\x05\x06` and the 18 fixed bytes after it (4.3.16) |
| `FILE_NAME` | the file name after a local or central header; its length is in the header |
| `EXTRA_FIELD` | the extra field after the file name |
| `FILE_DATA` | the file data after a local header: the compressed size of the header, in pieces of at most 64 KiB |
| `FILE_COMMENT` | the file comment after a central header |
| `ZIP_COMMENT` | the archive comment after the end of central directory record |
| `UNKNOWN` | one byte where no record starts |

**Covered subset:** archives whose local file headers hold the sizes of the
data, with stored or deflated files (the method does not matter, as the data
is not decompressed). Not covered:

- **Data descriptors.** When bit 3 of the general-purpose flag is set, the
  local file header has a compressed size of 0 and the sizes follow the data
  in a data descriptor. The driver reads no file data, so the data and the
  descriptor are `UNKNOWN` bytes, or are taken for a record when they contain
  a signature (see `descriptor` below).
- **ZIP64.** A size of `0xFFFFFFFF` is read as it is, not from the ZIP64
  extra field, so the file data runs to the end of the input. The ZIP64 end
  of central directory record and locator are not lexed.
- **Offsets.** The central directory is lexed where it is, not found through
  the end of central directory record. Encryption headers, archive extra data
  and digital signatures are not lexed.
- **No decoding.** No decompression, no CRC check.

**How invalid input lexes:** a near-miss of a signature, such as `PK\x03\x03`
or `pk\x03\x04`, is `UNKNOWN` bytes. A record cut by the end of the input does
not match: its bytes are `UNKNOWN`. A file cut inside a field gives that field
up to the end.

**Test data:**

| File | Shows |
| --- | --- |
| `document` | a stored and a deflated file, an extra field, a file comment and an archive comment; `TestNames` checks the file names against `archive/zip` |
| `empty` | an archive with no file: one end of central directory record |
| `descriptor` | an archive written by `zip.Writer.CreateHeader`, with a data descriptor (out of the subset); its data holds `PK\x01\x02`, taken for a central header |
| `zip64` | a ZIP64 archive (out of the subset): the file data runs to the end |
| `truncated` | the document cut inside the data of its second file |
| `not-zip` | near-misses of the three signatures and a cut record |

## bmp

**Tokens:**

| Type | Matches |
| --- | --- |
| `FILE_HEADER` | `BM` and the 12 bytes after it: file size, two reserved words, pixel offset |
| `DIB_HEADER` | the DIB header: as many bytes as its first four bytes give (at least 4) |
| `COLOR_TABLE` | the colour table after a DIB header of 40 bytes or more |
| `GAP` | the bytes from the end of the colour table to the pixel offset |
| `PIXEL_DATA` | the bytes from the pixel offset to the end of the input, in pieces of at most 64 KiB |
| `UNKNOWN` | one byte before a file header |

**Covered subset:** a file header and any DIB header. The colour table is
`biClrUsed` entries of 4 bytes, or 2 to the power of `biBitCount` when
`biClrUsed` is 0 and `biBitCount` is 1 to 8, read for a DIB header of 40
bytes or more (`BITMAPINFOHEADER` and its V4 and V5 forms). The colour table
ends at the pixel offset at the latest. Not covered:

- **The 12-byte `BITMAPCOREHEADER`** of OS/2 has 3-byte colour entries; its
  colour table is lexed as `GAP`.
- **Bit masks** after a `BITMAPINFOHEADER` with `BI_BITFIELDS` are part of
  `GAP`.
- **The file size** in the file header is not used. Data after the pixel
  data, such as an ICC profile, is part of `PIXEL_DATA`.
- **No decoding.** Pixel data is not decoded or decompressed.

**How invalid input lexes:** bytes before `BM`, such as the near-misses `BA`
and `bm`, are `UNKNOWN`. A file header cut by the end of the input is
`UNKNOWN` bytes. A DIB header cut by the end of the input gives a short
`DIB_HEADER`. If the pixel offset lies inside the headers, there is no colour
table and no gap, and the pixel data starts after the DIB header.

**Test data:**

| File | Shows |
| --- | --- |
| `document` | 4×2, 8 bits per pixel, a colour table of four entries and a gap of two bytes |
| `rgb24` | 2×2, 24 bits per pixel, no colour table, padded rows |
| `core` | a `BITMAPCOREHEADER` whose colour table is lexed as `GAP` |
| `truncated` | the document cut inside the DIB header |
| `bad-offset` | a pixel offset that lies inside the DIB header |
| `huge-colors` | a `biClrUsed` of `0xFFFFFFFF`: the colour table is cut at the pixel offset |
| `not-bmp` | the near-misses `BA` and `bm`, and a cut file header |

## jpeg

**Tokens:**

| Type | Matches |
| --- | --- |
| `SOI` | `FF D8` |
| `EOI` | `FF D9` |
| `SEGMENT_HEADER` | `FF`, a marker byte `C0`–`CF` or `DA`–`FE`, and the two-byte length of the segment (T.81, B.1.1.4) |
| `SEGMENT_PAYLOAD` | the payload of a segment: the length minus 2 |
| `ENTROPY_DATA` | the data after a start-of-scan segment: any byte but `FF`, and `FF` only before `00` or a restart marker `D0`–`D7` |
| `EXIF_SEGMENT_HEADER` | the header of an APP1 segment (`FF E1` and a length of 8 or more) and the identifier `Exif\0\0` |
| `TIFF_HEADER` | `II*\0` or `MM\0*` and the four-byte offset of the first IFD |
| `TIFF_DATA` | bytes of the TIFF structure that are not a header, a count or an entry: the gap before the IFD, the next-IFD offset and the values |
| `IFD_ENTRY_COUNT` | the two-byte number of entries of the first IFD |
| `IFD_ENTRY` | one 12-byte entry of the first IFD: tag, type, count, value or offset |
| `UNKNOWN` | one byte that no rule of the current mode matches |

The modes: **marker** mode lexes the markers between segments. **Scan** mode
follows a start-of-scan (`SOS`, `FF DA`) segment: it lexes `ENTROPY_DATA` and
the markers, and the driver goes back to marker mode after any marker but
`SOS`. **TIFF** mode follows an `EXIF_SEGMENT_HEADER` whose segment can hold a
TIFF header. After the TIFF header the driver reads the count and the entries
of the first IFD from the cursor, in the byte order of the header. No Exif
field reaches past the end of its segment.

**Covered subset:** the segment structure of T.81 Annex B, and the TIFF
header and the first IFD of an Exif segment. Not covered:

- **Fill bytes.** `FF` bytes before a marker are `UNKNOWN`.
- **Restart markers outside a scan**, `TEM` and the reserved markers `02`–`BF`
  are `UNKNOWN` bytes.
- **Other IFDs.** The next IFD, the Exif IFD and the GPS IFD are not followed:
  they are part of `TIFF_DATA`. The values of the entries are not
  interpreted.
- **No decoding.** No Huffman or DCT decoding; the length of a segment is not
  checked against its marker.

**How invalid input lexes:** a restart marker, a stuffed byte or a lone `FF`
outside a scan is `UNKNOWN` bytes. An APP1 segment whose length is below 8
cannot hold the Exif identifier, so it is a `SEGMENT_HEADER` and its short
payload, and the identifier bytes after the segment are `UNKNOWN`. In an Exif segment, a byte order other
than `II` or `MM` gives one `UNKNOWN` byte and the rest of the payload as
`SEGMENT_PAYLOAD`; a segment too short for a TIFF header gives its payload as
`SEGMENT_PAYLOAD`; an IFD offset outside the segment gives the rest of the
segment as `TIFF_DATA`. A file cut inside a payload or a scan gives that
token up to the end.

**Test data:**

| File | Shows |
| --- | --- |
| `document` | a 16×8 image from `image/jpeg` with a little-endian Exif segment of three entries |
| `exif-be` | a big-endian Exif segment of two entries whose IFD starts after a 2-byte gap |
| `segments` | a COM payload that holds `FF D9` and `FF D8`, a non-Exif APP1 (XMP), JFIF, an empty segment |
| `scan` | two scans with stuffed bytes and restart markers, a DHT segment between them, a fill byte before `EOI` |
| `truncated` | the document cut inside its entropy-coded data |
| `short-exif` | APP1 segments of the lengths 2 to 8, each followed by `Exif\0\0`: only length 8 is an `EXIF_SEGMENT_HEADER` |
| `near-misses` | a restart marker, a stuffed byte and a lone `FF` outside a scan; Exif segments with a bad byte order, too short, and an IFD offset past the end |

`TestPayloadMarkers` checks that the `FF D9` in the COM payload stays in one
`SEGMENT_PAYLOAD` and that `EOI` appears once, at the end. `TestExif` checks
the TIFF header and the number of `IFD_ENTRY` tokens of both byte orders.

## wav

**Tokens:**

| Type | Matches |
| --- | --- |
| `RIFF_HEADER` | `RIFF`, the four-byte size, and the form type `WAVE` |
| `FMT_CHUNK_HEADER` | `fmt ` and the four-byte length of its payload |
| `FMT_CHUNK_PAYLOAD` | the payload of a `fmt ` chunk: the format tag, channels, sampling rate and the rest |
| `DATA_CHUNK_HEADER` | `data` and the four-byte length of its payload |
| `DATA_CHUNK_PAYLOAD` | the payload of a `data` chunk: the samples |
| `CHUNK_HEADER` | any other chunk id of four printable ASCII bytes, and the length |
| `CHUNK_PAYLOAD` | the payload of any other chunk |
| `PAD_BYTE` | the byte after a payload of odd length |
| `UNKNOWN` | one byte before the RIFF header, or where no chunk header starts |

The modes: **file** mode lexes the RIFF header; the driver switches to
**chunk** mode after it. Chunk mode lexes the chunk headers. After each one the
driver reads the length (little-endian) and reads that many bytes as the
payload, then one `PAD_BYTE` when the length is odd. So the bytes `data` in
the payload of another chunk are not taken for a chunk header.

**Covered subset:** a RIFF `WAVE` file as a flat sequence of chunks. Not
covered:

- **The RIFF size.** The size in the RIFF header is not used: chunks are
  lexed up to the end of the input, and bytes after the RIFF form are lexed as
  chunks.
- **Nested chunks.** A `LIST` chunk is one `CHUNK_HEADER` and one
  `CHUNK_PAYLOAD`; its sub-chunks are not lexed.
- **Other forms.** `RIFX` (big-endian), `RF64` and other form types than
  `WAVE` are `UNKNOWN` bytes.
- **No decoding.** The fields of the `fmt ` chunk are not interpreted and the
  samples are not decoded. A `data` chunk before the `fmt ` chunk, or a missing
  `fmt ` chunk, is lexed as it is.

**How invalid input lexes:** bytes before a RIFF header, such as the
near-misses `RIFX`, `riff` and a form type of `WAVX`, are `UNKNOWN`. After the
RIFF header, a chunk id with a byte outside printable ASCII is not a chunk
header: its bytes are `UNKNOWN`, one at a time, and a chunk header can start at
any of the bytes after it. A chunk header cut by the end of the input is
`UNKNOWN` bytes. A file cut inside a payload gives that payload up to the end.

**Test data:**

| File | Shows |
| --- | --- |
| `document` | mono, 8 bits at 8000 Hz: a `fmt ` chunk, a `note` chunk of 5 bytes that holds `data` and its pad byte, a `data` chunk of 200 samples |
| `chunks` | stereo, 16 bits: a `data` chunk of 3 bytes and its pad byte, a `LIST` chunk, an empty chunk |
| `truncated` | the document cut inside its `data` payload |
| `not-wav` | near-misses of the RIFF header, then near-misses of a chunk id and a cut chunk header |

`TestLengths` checks that the tokens of `document`, `chunks` and `truncated`
tile the file, so their lengths sum to its size, and that each payload has the
length its header states.

## mp3

**Tokens:**

| Type | Matches |
| --- | --- |
| `ID3_HEADER` | `ID3`, the major version 3 or 4, the revision, the flags and the tag size as a 28-bit synchsafe integer: 10 bytes |
| `ID3_FRAME_HEADER` | a tag frame id of four upper-case letters or digits, the four-byte size and two bytes of flags |
| `ID3_FRAME_BODY` | the body of a tag frame: the size its header states, up to the end of the tag |
| `ID3_PADDING` | the rest of the tag after its last frame |
| `FRAME_HEADER` | the 4-byte header of an MPEG-1 or MPEG-2 Layer III frame: the sync word, the version, the layer, a bitrate index of 1 to 14 and a sampling-rate index of 0 to 2 |
| `FRAME_BODY` | the rest of the frame, whose length the driver computes from the header |
| `UNKNOWN` | one byte where no header of the current mode starts |

The modes: **start** mode lexes the first token of the file, an
`ID3_HEADER` or a `FRAME_HEADER`. After an `ID3_HEADER` the driver switches to
**tag** mode, which lexes the tag frame headers. The driver counts the bytes
left in the tag. After a frame header it reads the body by the size the header
states: a plain 32-bit big-endian number in ID3v2.3, a synchsafe number in
ID3v2.4. When fewer than 10 bytes are left, or the next byte is `00`, the rest
of the tag is `ID3_PADDING`. So the bytes `FF FB` inside the tag are never
taken for an audio frame. After the tag, or after any first token but an
`ID3_HEADER`, the driver switches to **audio** mode, which lexes the frame
headers. A frame stores no length: after a frame header the driver computes it
as 144 × bitrate / sampling rate (MPEG-1) or 72 × bitrate / sampling rate
(MPEG-2), rounded down, plus 1 when the padding bit is set, and reads the rest
of the frame as `FRAME_BODY`.

**Covered subset:** an ID3v2.3 or v2.4 tag at the start of the file, and
MPEG-1 and MPEG-2 Layer III frames. Not covered:

- **Other tags.** ID3v2.2 (three-character frame ids), a tag that is not at
  the start of the file, the ID3v2.4 footer and ID3v1 (`TAG` at the end) are
  not lexed as tags: their bytes are `UNKNOWN`, and bytes in them that look
  like a frame header are taken for one (see `id3v22` below).
- **Tag details.** The extended header is lexed as bytes of the tag
  (`UNKNOWN` bytes or a frame header); unsynchronisation, compression and
  encryption are not undone. Frame bodies, such as the text of `TIT2`, are not
  interpreted.
- **Other frames.** Free-format frames (bitrate index 0), Layer I and II, and
  MPEG 2.5 are not frame headers: their bytes are `UNKNOWN`.
- **No decoding.** No Huffman or MDCT decoding, no CRC check; the CRC after a
  header with the protection bit 0 is part of `FRAME_BODY`. A Xing or Info
  frame lexes as an ordinary frame. Frames are not checked to follow one
  another.

**How invalid input lexes:** in audio mode, bytes that are not a frame header
are `UNKNOWN`, one at a time, until the next frame header. The near-misses of
a frame header (bitrate index 0 or 15, sampling-rate index 3, Layer I, MPEG
2.5) and a frame header cut by the end of the input are `UNKNOWN` bytes. An ID3
header with a size byte of `80` or above is not synchsafe, so it is `UNKNOWN`
bytes, and the file has no tag. In the tag, bytes that are not a frame header
are `UNKNOWN`, one at a time, and count against the tag; a frame whose size
runs past the end of the tag is cut at the end of the tag. A file cut inside a
body gives that body up to the end.

**Test data:**

| File | Shows |
| --- | --- |
| `document` | an ID3v2.4 tag with a title, an artist, a private frame that holds `FF FB 14 C4`, and padding; three MPEG-1 frames of 96 bytes, the second of which holds a frame header in its body |
| `mpeg2` | an ID3v2.3 tag whose comment size `00 00 00 82` is a plain number, 130; four MPEG-2 frames of 26 and 27 (padded) bytes |
| `tag-mismatch` | an ID3v2.3 tag with bytes that are not a frame id and a frame whose size runs past the tag |
| `id3v22` | an ID3v2.2 tag (out of the subset): its `FF FB 14 C4` is taken for a frame header |
| `near-misses` | a non-synchsafe ID3 header, bytes between frames, the near-misses of a frame header, an ID3 header after the start and a cut frame header |
| `truncated` | the document cut inside its second frame |

`TestFrames` checks that each `FRAME_HEADER` is followed by a `FRAME_BODY`,
that each frame has the length that `inputs_test.go` wrote, and that the number
of frames is the number it wrote. `TestTagSkipped` checks that the
`FF FB 14 C4` of the document's tag stays in one `ID3_FRAME_BODY` and that no
`FRAME_HEADER` starts inside the tag. `TestFrameLength` checks the computed
lengths against lengths worked out by hand.

## Test inputs

The inputs are binary files, built by `inputs_test.go` in each example and
marked `binary` in the repository's `.gitattributes`. `TestInputs` checks that
each committed input equals the bytes it builds, except the two JPEG inputs
that hold `image/jpeg` output (`document`, and `truncated`, cut from it),
which a later Go release may change; it checks that `image/jpeg` decodes the
committed `document` instead. `go test ./binary/... -update`
rewrites the inputs, then the golden files.
