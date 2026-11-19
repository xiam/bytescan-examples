# bytescan examples

Small programs that show what [bytescan](https://github.com/xiam/bytescan) can
do. bytescan is a Go library that reads text or bytes and tells you what each
piece is. Each program here reads one format: a programming language, a
human language, a binary file or a network protocol. It prints every piece it
finds, one per line.

Every program is short, complete and tested. Copy the one closest to your own
format and change it.

## Run one

You need [Go](https://go.dev/dl/). Clone this repository, then pipe some text
into the `hello` example:

```sh
git clone https://github.com/xiam/bytescan-examples.git
cd bytescan-examples
echo 'say hello to the world' | go run ./hello
```

Output:

```text
1:0 KEYWORD "say"
1:4 WORD "hello"
1:10 WORD "to"
1:13 WORD "the"
1:17 WORD "world"
```

Each line gives the line number, the column, the kind of piece and its text.
`say` is a keyword. The other words are plain words.

Every example has sample inputs in its `testdata` folder. To try one, run
`go run ./json < json/testdata/document.input`. To run all the tests, run
`go test ./...`.

## The catalogue

### Text

| Example | What it shows |
| --- | --- |
| [hello](hello) | The smallest example: a keyword and words. |
| [json](json) | JSON, with strict strings and numbers. Copy this one to start a new example. |
| [sql](sql) | SQL keywords, which ignore upper and lower case. |
| [go](go) | Go, including the semicolons that Go adds for you. |
| [python](python) | Python, where indentation opens and closes blocks. |
| [lua](lua) | Lua, with its long brackets such as `[==[ ... ]==]`. |
| [typescript](typescript) | TypeScript, where `/` can start a pattern or divide numbers. |
| [ruby](ruby) | Ruby, with code inside strings and multi-line strings. |
| [whitespace](whitespace) | A language made only of spaces, tabs and line breaks. |
| [ansi](ansi) | Terminal colour codes mixed into text, and how to remove them. |
| [words](words) | Words in many languages, when more than one rule could match. |
| [english](english) | English words and punctuation, with "don't" and "well-known". |
| [spanish](spanish) | Spanish words and punctuation, with `¿`, `¡` and accents. |
| [arabic](arabic) | Arabic, written right to left, with its vowel marks. |
| [japanese](japanese) | Japanese, where words have no spaces between them. |
| [korean](korean) | Korean syllables, which Unicode can store in two ways. |

### Binary files

| Example | What it shows |
| --- | --- |
| [binary/zip](binary/zip) | The parts of a ZIP archive. |
| [binary/bmp](binary/bmp) | A BMP image: its headers, then its pixels. |
| [binary/jpeg](binary/jpeg) | A JPEG image: its segments, its image data and its EXIF data. |
| [binary/wav](binary/wav) | A WAV sound file and its chunks. |
| [binary/mp3](binary/mp3) | An MP3 file, frame by frame. |

### Streams

These programs read a stream that may never end, in constant memory. They
keep going after corrupt data.

| Example | What it shows |
| --- | --- |
| [streams/sse](streams/sse) | Server-Sent Events, the events a web server pushes to a browser. |
| [streams/ssejson](streams/ssejson) | JSON inside Server-Sent Events: one format nested in another. |
| [streams/nmea](streams/nmea) | GPS sentences, with their checksums. |
| [streams/nats](streams/nats) | The NATS messaging protocol: text commands with binary payloads. |
| [streams/mqtt](streams/mqtt) | MQTT packets, the protocol of many small devices. |
| [streams/websocket](streams/websocket) | WebSocket frames. |
| [streams/syslog](streams/syslog) | Syslog messages sent over TCP, ended by a newline or led by their length. |
| [streams/mpegts](streams/mpegts) | MPEG transport stream, the fixed-size packets of digital TV. |
| [streams/slip](streams/slip) | SLIP frames, packets sent over a serial line. |

### Text that changes

These programs follow a text as it changes. After each edit they update only
what the edit changed.

| Example | What it shows |
| --- | --- |
| [highlight](highlight) | A syntax highlighter that repaints only the tokens an edit changed. |
| [repl](repl) | A prompt loop that follows each key, and waits for one more line while a string or a comment is open. |

### The cursor on its own

bytescan reads its input through a cursor. You can also use the cursor
without the rest of bytescan.

| Example | What it shows |
| --- | --- |
| [cursor/positioned-cursor](cursor/positioned-cursor) | Hold a place, read ahead, then read back the text you passed. |
| [cursor/surrounding-text](cursor/surrounding-text) | Find a marker, then read the text around it. |

## Learn more

About bytescan:

- [bytescan's README](https://github.com/xiam/bytescan#readme): what bytescan
  is, with a text example and a binary example.
- [bytescan's documentation](https://github.com/xiam/bytescan/tree/master/docs):
  how to build a scanner, the rules you can use, and the guides. The
  [technique chooser](https://github.com/xiam/bytescan/blob/master/docs/guides/README.md)
  helps you pick the approach for your format.
- [bytescan's specifications](https://github.com/xiam/bytescan/tree/master/docs/specs):
  the exact behaviour, for when you rely on an edge case.

About these examples:

- [Examples by technique](docs/techniques.md): each example with the lexing
  technique it shows and the bytescan guide that explains it.
- [The example standard](docs/standard.md): the rules every example follows.
  Read it before you add or change an example.
- [Long-running streams](docs/streams.md): the extra rules for the stream
  examples.
- [Text that changes](docs/edits.md): the extra rules for the examples that
  lex a text that changes.
- [Working on the examples](docs/development.md): the repository layout, and
  how to test the examples against a bytescan checkout.

## License

MIT, as bytescan: see [LICENSE.md](LICENSE.md).
