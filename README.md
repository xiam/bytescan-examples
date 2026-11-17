# bytescan examples

Example programs for [bytescan](https://github.com/xiam/bytescan), a Go
library that reads streams of data and classifies what it finds, for text
and binary alike. Each program builds a scanner with `bytescan` for one
format, reads standard input, and prints one line per token. Each one is
chosen for the lexing technique it demonstrates, not for completeness. This
page is the catalogue of examples, grouped by use case, and the standard
every example follows.

To pick a technique for your own format, start with bytescan's
[technique chooser](https://github.com/xiam/bytescan/blob/master/docs/guides/README.md):
it links one guide per use case, and each guide links the examples that show
it. This page covers only what is specific to writing an example in this
repository.

The [JSON example](json) is the reference: copy its layout when you add an
example.

The examples lived in bytescan under `internal/examples` until bytescan
v0.3.0 moved them here with their history (`git log --follow` works on every
file). Commit messages from before the move are unchanged: their `#N` and
`!N` references point at the issues and merge requests of bytescan, not of
this repository.

## Repository layout

```text
<name>/                   one text example per directory (json, go, sql, ...)
binary/<format>/          the binary file examples, with one README.md
streams/<name>/           the long-running stream examples
streams/sse/ssedriver/    the SSE mode driver, a package that ssejson imports
internal/lexdump/         the dumper and golden-file checks every example uses
internal/hexdump/         the hex view of the binary and byte-stream examples
internal/streamkit/       shared helpers of the stream examples
internal/guardtest/       the per-token guard checks of the stream examples
```

## Running the examples

`go.mod` requires the released `github.com/xiam/bytescan v0.3.0`, so the
examples build on their own:

```sh
git clone https://github.com/xiam/bytescan-examples.git
cd bytescan-examples
go test -race ./...
```

## Running the examples against a bytescan checkout

To run the examples against another bytescan revision, such as a release
candidate, point `GOWORK` at a workspace file of your own that uses both
checkouts. The repository commits no `go.work`. Relative paths in a
workspace file resolve from the directory of that file, so use absolute ones:

```text
go 1.21.1

use (
	/path/to/bytescan-examples
	/path/to/bytescan
)
```

```sh
git -C /path/to/bytescan checkout <revision>
GOWORK=/path/to/examples.go.work go vet ./...
GOWORK=/path/to/examples.go.work go test -race ./...
```

The workspace module takes the place of the required version. If `go.mod`
here requires a bytescan version that is not released yet, also add
`replace github.com/xiam/bytescan <version> => /path/to/bytescan`, because the
go command still reads the `go.mod` of a required version.

## Catalogue

Each group links the guide for its use case.

### Programming languages

Guide: [programming-language.md](https://github.com/xiam/bytescan/blob/master/docs/guides/programming-language.md).

| Example | Technique |
| --- | --- |
| [hello](hello) | the README text example: a keyword before the word rule |
| [sql](sql) | keywords that ignore case |
| [go](go) | semicolon insertion as a filter, checked against `go/scanner` |
| [python](python) | `INDENT` and `DEDENT` from a filter |
| [whitespace](whitespace) | the inverted case: layout is the only token, all visible text is comment |
| [lua](lua) | long brackets through `modes` and a counted match |
| [typescript](typescript) | regex versus division, nested template literals, through `modes` |
| [ruby](ruby) | interpolation, percent literals, heredocs, through `modes` |

### Data formats

Guide: [data-format.md](https://github.com/xiam/bytescan/blob/master/docs/guides/data-format.md).

| Example | Technique |
| --- | --- |
| [json](json) | a strict token grammar from checked escapes and number options; the reference example |

### Terminal streams

Guide: [terminal-stream.md](https://github.com/xiam/bytescan/blob/master/docs/guides/terminal-stream.md).

| Example | Technique |
| --- | --- |
| [ansi](ansi) | control data mixed into text (ECMA-48), and a strip filter |

### Human-language text

Guide: [human-text.md](https://github.com/xiam/bytescan/blob/master/docs/guides/human-text.md).

| Example | Technique |
| --- | --- |
| [words](words) | registration order between overlapping rules; skipping layout |
| [english](english) | words against punctuation, with contractions and hyphens |
| [spanish](spanish) | words against punctuation, with inverted marks and accents |
| [arabic](arabic) | combining marks in right-to-left text |
| [japanese](japanese) | script runs where words have no spaces |
| [korean](korean) | a script with spaces whose syllables have two encodings |

### Line-oriented text protocols

Guide: [line-protocol.md](https://github.com/xiam/bytescan/blob/master/docs/guides/line-protocol.md).

| Example | Technique |
| --- | --- |
| [streams/sse](streams/sse) | records separated by a blank line, through a mode driver in its own package (`ssedriver`) |
| [streams/nmea](streams/nmea) | a start character and a checksum field; resynchronisation |
| [streams/nats](streams/nats) | text control lines with byte-counted payloads, through the byte adapter and chunked fields |

### Length-prefixed protocols

Guide: [length-prefixed.md](https://github.com/xiam/bytescan/blob/master/docs/guides/length-prefixed.md).

| Example | Technique |
| --- | --- |
| [streams/mqtt](streams/mqtt) | a variable-length remaining length |
| [streams/websocket](streams/websocket) | a self-describing length width, a masking key and a 64-bit length in int-sized fields |
| [streams/syslog](streams/syslog) | a decimal length prefix (octet counting) through the byte adapter and a chunked field |

### Binary files

Guide: [length-prefixed.md](https://github.com/xiam/bytescan/blob/master/docs/guides/length-prefixed.md). The files read the
fields their headers announce as the protocols do.

| Example | Technique |
| --- | --- |
| [binary/zip](binary/zip) | bytes through the byte adapter [`bytesource`](https://github.com/xiam/bytescan/tree/master/bytesource) and the byte rules [`rules/octet`](https://github.com/xiam/bytescan/tree/master/rules/octet); length-prefixed fields |
| [binary/bmp](binary/bmp) | fixed headers and an offset-driven payload, read in pieces |
| [binary/jpeg](binary/jpeg) | marker segments, entropy-coded data, EXIF |
| [binary/wav](binary/wav) | a chunked container through the byte adapter |
| [binary/mp3](binary/mp3) | frames whose length is computed, not stored |

### Fixed-size packets with a sync marker

Guide: [sync-marker.md](https://github.com/xiam/bytescan/blob/master/docs/guides/sync-marker.md).

| Example | Technique |
| --- | --- |
| [streams/mpegts](streams/mpegts) | fixed-size packets that start with a sync byte; a look-ahead rule confirms the sync |

### Escape-framed bytes

Guide: [escape-framed.md](https://github.com/xiam/bytescan/blob/master/docs/guides/escape-framed.md).

| Example | Technique |
| --- | --- |
| [streams/slip](streams/slip) | a delimiter byte and byte stuffing, with byte rules |

### Nested formats

Guide: [nested-formats.md](https://github.com/xiam/bytescan/blob/master/docs/guides/nested-formats.md).

| Example | Technique |
| --- | --- |
| [streams/ssejson](streams/ssejson) | piped scanners: JSON inside Server-Sent Events, two cursors |

A new example adds its row to the group of its use case, and the guide of that
use case links it.

## The standard

### Lexing only

An example turns any input into a stream of tokens. It never parses, and it
never decides whether the input is valid. Input that no rule matches becomes
`UNKNOWN` tokens, one rune each; the program reaches the end of the input and
exits 0. A reviewer rejects code that checks nesting, counts brackets to judge
them, or reports an error for bad input.

### Layout

```text
<name>/
  main.go          package main: reads standard input, dumps the tokens
  scanner.go         newScanner, the rules it registers, and any helper rules
  scanner_test.go    the golden test
  README.md        technique, tokens, covered subset
  testdata/
    <case>.input   one input per case
    <case>.golden  the expected output of that input
```

A group of related examples shares one directory, such as
`binary/<format>`, with one `README.md` for the group.

- **`scanner.go`** has one constructor, `newScanner(r io.Reader)
  *bytescan.Scanner`, that creates the scanner and registers every rule. A
  rule that needs more than one line of composition gets its own function,
  named for the token (`stringRule`, `numberRule`), with a comment that gives
  its regular expression or grammar and names the specification section.
- **`main.go`** holds the package comment, `main`, and the `Dumper` when the
  example needs more than the zero value (words skips layout). JSON's `main`:

  ```go
  func main() {
      if err := (lexdump.Dumper{}).Dump(os.Stdout, newScanner(os.Stdin)); err != nil {
          log.Fatal(err)
      }
  }
  ```

- **`scanner_test.go`** runs the golden test, and any test of a filter:

  ```go
  func TestGolden(t *testing.T) {
      lexdump.Golden(t, lexdump.Dumper{}, newScanner)
  }
  ```

- **`testdata`** holds one `.input` file per case and its committed `.golden`
  file. Name a case for what it shows (`numbers`, `unterminated`). Include at
  least one realistic document, one case per token form, and the invalid cases
  of the subset: unterminated tokens, characters outside the language, and
  near-misses of each token. Every expected output is a committed file; tests
  need no interpreter and no network.

Rules come only from [`rules/core`](https://github.com/xiam/bytescan/tree/master/rules/core),
[`rules/base`](https://github.com/xiam/bytescan/tree/master/rules/base), [`rules/special`](https://github.com/xiam/bytescan/tree/master/rules/special) and
[`rules/octet`](https://github.com/xiam/bytescan/tree/master/rules/octet) (see [rules.md](https://github.com/xiam/bytescan/blob/master/docs/rules.md)). An example writes
no state function, so `git grep -n 'bytescan.State' <name>` finds nothing.
An example needs no change under bytescan's `rules/` and none in its engine.
A missing or awkward rule is reported in the merge request and as a bytescan
issue; the example composes what it can, or narrows its subset.

An example uses the standard library and bytescan only, and `go.mod` gains no
`require`.
Its tests take under 2 s with `go test -race`.

### Running

```sh
go run ./<name> < <name>/testdata/<case>.input
go test ./<name>            # compare with the golden files
go test ./<name> -update    # rewrite them, then review the diff
```

The output of `go run` on an input file is exactly its golden file.

### lexdump

[`internal/lexdump`](internal/lexdump) is the one helper every example uses:

- `Dumper.Dump(w, lx)` reads `lx` until `io.EOF` and writes one line per
  token. `lx` is any `bytescan.Source`: a `*bytescan.Scanner`, a filter
  or a mode driver.
- `Dumper.Skip` lists token types that are read but not printed, such as
  layout in [words](words).
- `Dumper.Format` turns a token into its line. The default, `lexdump.Text`,
  writes `LINE:COLUMN TYPE "TEXT"`: the 1-based line, the 0-based rune column,
  the type and the text as a Go quoted string. An example whose input is not
  text supplies its own `Format`.
- `lexdump.Golden(t, d, newScanner)` runs every `testdata/*.input` through
  `newScanner` and `d`, and compares the output with the `.golden` file of the
  same name. `go test -update` rewrites the golden files.
- `lexdump.GoldenWithout(t, newView, drop)` checks a filtered view against
  the same golden files without the lines of the dropped types, so a filter
  needs no golden files of its own ([ansi](ansi),
  [whitespace](whitespace)).
- `lexdump.Tiling(t, newScanner)` checks that the tokens of every input join
  to the input, which a mode driver can break when it switches scanners
  ([ruby](ruby)).
- `lexdump.Inputs(t, inputs)` checks that each committed binary
  `testdata/<name>.input` equals the bytes the test builds for it; `go test
  -update` rewrites the inputs first. `lexdump.Sequence(first, n)` builds
  counting bytes that show where a field is cut.

### Token types

A token type is an upper-case name with underscores, one type per token class:
`STRING`, `NUMBER`, `BEGIN_ARRAY`. Use the name the language specification
gives the token when it gives one (JSON's `NAME_SEPARATOR` for `:`). Layout
gets its own types (`WHITESPACE`, `NEWLINE`, `COMMENT`); the example prints it
unless its README says why not. `UNKNOWN` is the engine's type for a rune no
rule matches; no rule registers it. Declare the types as typed
`bytescan.TokenType` constants.

A scanner accepts each type once. To give several forms one type, combine them
with `core.Choice`.

### Registration order

The longest match wins, and a tie goes to the rule registered first (see
the [technique chooser](https://github.com/xiam/bytescan/blob/master/docs/guides/README.md#rules-that-hold-for-every-technique)).
Write a comment in `newScanner` that names each tie the order breaks, or says
that no two rules match the same text.

### The covered subset

Each example's `README.md` states what it covers. A documented subset is
enough; completeness against the specification is not a goal. The README has
these parts, in this order:

1. A title, one sentence on what the example lexes, and a link to the
   specification it follows.
2. **Technique:** the one thing the example teaches.
3. The `go run` command for one test input.
4. **Tokens:** a table of each type and what it matches.
5. **Covered subset:** what is covered and, as a list, what is left out.
6. **How invalid input lexes:** how the near-misses and broken input split.
7. **Test data:** a table of each case and what it shows.

### Filters above `Next()`

When and how to write a filter is in the
[programming-language guide](https://github.com/xiam/bytescan/blob/master/docs/guides/programming-language.md#filters-logic-on-the-token-stream).
In an example, a filter is a `bytescan.Source` in a file of its own
(`semicolons.go` in [go](go), `filter.go` in
[python](python)), so `main.go` dumps it as it dumps a
scanner. It adds, drops or relabels tokens by the lexical rules of the
language reference, and never reports that a program is invalid. Its test
uses `lexdump.GoldenWithout` when it only drops types.

### Modes: several scanners over one cursor

When a mode driver is needed is in the
[technique chooser](https://github.com/xiam/bytescan/blob/master/docs/guides/README.md); the [`modes`](https://github.com/xiam/bytescan/tree/master/modes) package
documents the driver. A driver follows three rules. Without them the output
can still look right, but the cursor retains input without bound, or tokens
come out wrong without an error:

1. **Call `Release()` on the outgoing scanner at every mode switch**, and before
   the driver reads the cursor itself.
2. **Cache one scanner per mode, per terminator or per bracket level.** Never
   build a scanner per token, and release a scanner before you drop it.
3. **Never call `Next()` on two scanners that share one cursor at the same
   time.**

`modes.Driver` applies rules 1 and 3, and `modes.Cache` applies rule 2. The
drivers of [lua](lua),
[typescript](typescript),
[ruby](ruby), the five binary examples, and the stream
examples [sse](streams/sse),
[syslog](streams/syslog),
[nats](streams/nats),
[mqtt](streams/mqtt),
[mpegts](streams/mpegts) and
[ssejson](streams/ssejson) run on it.
[websocket](streams/websocket) has one scanner and no mode
to switch, so it releases its scanner itself.

A mode-driver example adds `driver.go` to the layout. It holds the driver
type, which is a `bytescan.Source`; `newScanner` in `scanner.go` returns the
driver instead of a `*bytescan.Scanner`, and builds the scanner of each mode
with its own constructor (`newCodeScanner`, `newLevelScanner`). The stream
examples use one shape for it:

- `newScanner(r io.Reader)` builds the cursor and returns the driver with the
  example's production options, such as its per-token guard, so `go run` and
  the tests run the same scanner.
- `newDriver(cur bytescan.Cursor, opts ...bytescan.Option)` returns the
  driver over a cursor. A test passes its own cursor, such as one with a
  bounded retention (`streamtest.LexRetained`).
- Each mode scanner has its own constructor, `newXScanner(cur, ..., opts
  ...bytescan.Option)`, and `newDriver` passes `opts` to every one.
- A driver with more than one scanner runs them on a `modes.Driver`; a driver
  that reads fields by a declared length queues them in a `bytesource.Fields`
  and switches away from, or releases, its active scanner before it reads.

When another example imports the driver, the driver is a package of its own
instead of `driver.go`, with an exported `New`: the
[SSE](streams/sse) driver is
[`sse/ssedriver`](streams/sse/ssedriver), which the
[ssejson example](streams/ssejson) imports. The README has a
section on the driver, after **Tokens**, that names its modes and states how
it follows the three rules. A mode driver over text has a `lexdump.Tiling`
test.

Every scanner returns a plain `io.EOF` at the end of the input, whatever its
mode, so only the driver knows that a construct is unterminated. The driver
does not report it: it lexes the construct to the end of the input and stops
normally, as the lexing-only rule requires.

Two layers of scanners over two cursors, joined by a pull pipe, are in the
[nested-formats guide](https://github.com/xiam/bytescan/blob/master/docs/guides/nested-formats.md) and the
[ssejson example](streams/ssejson).

## Long-running streams

A stream example lexes an input that may never end, with constant memory, and
keeps lexing after corrupt data. Stream examples live under
`streams/<name>` and follow the standard above. Their long
tests use the public harness [`streamtest`](https://github.com/xiam/bytescan/tree/master/streamtest); the
[operations guide](https://github.com/xiam/bytescan/blob/master/docs/guides/operations.md) describes it, with the per-token
guard and `Release`. A stream example adds these rules:

- **Tests.** A `TestLongRun` lexes a generated stream with `streamtest.Run`,
  and a `TestCorruption` checks the tokens around each garbage run of
  `streamtest.Corrupt`. A driver that reads fields has a `TestReleaseBefore*`
  test with `streamtest.LexRetained`. A format with a largest length has a
  test that puts that record before the stream with `streamtest.Prefix`
  ([mqtt](streams/mqtt),
  [syslog](streams/syslog),
  [websocket](streams/websocket)).
- **Budget.** The default tests of a stream example take under 3 s with and
  without `-race`, at the default budget of the harness. A long run uses the
  `-streamtest.long` flag and no test timeout:

  ```sh
  go test -timeout 0 -run Long ./streams/<name> -streamtest.long
  go test -timeout 0 ./streamtest -streamtest.long   # in bytescan: the harness self-test
  ```

  Throughput is logged, not asserted.
- **Ceiling.** An example uses the default heap ceiling of `streamtest.Run`.
  An example that holds more by design, such as a queue of tokens, sets its
  own ceiling with `WithCeiling` and states why in its README.
- **Per-token guard.** Every scanner of a stream example is built with
  `bytescan.WithMaxTokenBytes` in `newScanner` itself, so `go run` is guarded
  as the tests are: through `NewWithOptions` for one scanner, through
  `NewWithCursorOptions` for each scanner of a mode driver. The guard is
  `streamtest.TextGuard` (4,096 bytes) on a UTF-8 cursor,
  `streamtest.ByteGuard` (1,024 bytes) on the byte adapter, twice that when
  the input has bytes of `0x80` or above.
- **Bounded rules.** Every rule of a stream example is bounded, so on input
  that never ends a record the example keeps producing bounded pieces and
  never reaches the guard. Every stream example except
  [streams/nmea](streams/nmea) has a `TestGuard` that shows both sides on its
  production constructor: bounded pieces with no guard error on such input,
  and the guard error for a rule that the test adds and that grows past the
  guard.

## License

MIT, as bytescan: see [LICENSE.md](LICENSE.md).
