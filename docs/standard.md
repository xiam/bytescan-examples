# The example standard

Every lexing example in this repository follows the rules on this page. A
reviewer checks a new example against them. The [JSON example](../json) is the
reference: copy its layout when you add an example.

The programs under [`cursor/`](../cursor) use the cursor without a scanner, so
these rules do not apply to them. Stream examples follow these rules and the
extra rules in [Long-running streams](streams.md).
Examples that lex a text that changes follow these rules with the
differences in [Text that changes](edits.md).

## Lexing only

An example turns any input into a stream of tokens. It never parses, and it
never decides whether the input is valid. Input that no rule matches becomes
`UNKNOWN` tokens, one rune each; the program reaches the end of the input and
exits 0. A reviewer rejects code that checks nesting, counts brackets to judge
them, or reports an error for bad input.

## Layout

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

## Running

```sh
go run ./<name> < <name>/testdata/<case>.input
go test ./<name>            # compare with the golden files
go test ./<name> -update    # rewrite them, then review the diff
```

The output of `go run` on an input file is exactly its golden file.

## lexdump

[`internal/lexdump`](../internal/lexdump) is the one helper every example uses:

- `Dumper.Dump(w, lx)` reads `lx` until `io.EOF` and writes one line per
  token. `lx` is any `bytescan.Source`: a `*bytescan.Scanner`, a filter
  or a mode driver.
- `Dumper.Skip` lists token types that are read but not printed, such as
  layout in [words](../words).
- `Dumper.Format` turns a token into its line. The default, `lexdump.Text`,
  writes `LINE:COLUMN TYPE "TEXT"`: the 1-based line, the 0-based rune column,
  the type and the text as a Go quoted string. An example whose input is not
  text supplies its own `Format`.
- `lexdump.Golden(t, d, newScanner)` runs every `testdata/*.input` through
  `newScanner` and `d`, and compares the output with the `.golden` file of the
  same name. `go test -update` rewrites the golden files.
- `lexdump.Compare(t, golden, got)` compares any output with one golden
  file, and `go test -update` rewrites it. An example whose output is not a
  token dump uses it ([highlight](../highlight)).
- `lexdump.GoldenWithout(t, newView, drop)` checks a filtered view against
  the same golden files without the lines of the dropped types, so a filter
  needs no golden files of its own ([ansi](../ansi),
  [whitespace](../whitespace)).
- `lexdump.Tiling(t, newScanner)` checks that the tokens of every input join
  to the input, which a mode driver can break when it switches scanners
  ([ruby](../ruby)).
- `lexdump.Inputs(t, inputs)` checks that each committed binary
  `testdata/<name>.input` equals the bytes the test builds for it; `go test
  -update` rewrites the inputs first. `lexdump.Sequence(first, n)` builds
  counting bytes that show where a field is cut.

## Token types

A token type is an upper-case name with underscores, one type per token class:
`STRING`, `NUMBER`, `BEGIN_ARRAY`. Use the name the language specification
gives the token when it gives one (JSON's `NAME_SEPARATOR` for `:`). Layout
gets its own types (`WHITESPACE`, `NEWLINE`, `COMMENT`); the example prints it
unless its README says why not. `UNKNOWN` is the engine's type for a rune no
rule matches; no rule registers it. Declare the types as typed
`bytescan.TokenType` constants.

A scanner accepts each type once. To give several forms one type, combine them
with `core.Choice`.

## Registration order

The longest match wins, and a tie goes to the rule registered first (see
the [technique chooser](https://github.com/xiam/bytescan/blob/master/docs/guides/README.md#rules-that-hold-for-every-technique)).
Write a comment in `newScanner` that names each tie the order breaks, or says
that no two rules match the same text.

## The covered subset

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

## Filters above `Next()`

When and how to write a filter is in the
[programming-language guide](https://github.com/xiam/bytescan/blob/master/docs/guides/programming-language.md#filters-logic-on-the-token-stream).
In an example, a filter is a `bytescan.Source` in a file of its own
(`semicolons.go` in [go](../go), `filter.go` in
[python](../python)), so `main.go` dumps it as it dumps a
scanner. It adds, drops or relabels tokens by the lexical rules of the
language reference, and never reports that a program is invalid. Its test
uses `lexdump.GoldenWithout` when it only drops types.

## Modes: several scanners over one cursor

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
drivers of [lua](../lua),
[typescript](../typescript),
[ruby](../ruby), the five binary examples, and the stream
examples [sse](../streams/sse),
[syslog](../streams/syslog),
[nats](../streams/nats),
[mqtt](../streams/mqtt),
[mpegts](../streams/mpegts) and
[ssejson](../streams/ssejson) run on it.
[websocket](../streams/websocket) has one scanner and no mode
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
[SSE](../streams/sse) driver is
[`sse/ssedriver`](../streams/sse/ssedriver), which the
[ssejson example](../streams/ssejson) imports. The README has a
section on the driver, after **Tokens**, that names its modes and states how
it follows the three rules. A mode driver over text has a `lexdump.Tiling`
test.

Every scanner returns a plain `io.EOF` at the end of the input, whatever its
mode, so only the driver knows that a construct is unterminated. The driver
does not report it: it lexes the construct to the end of the input and stops
normally, as the lexing-only rule requires.

Two layers of scanners over two cursors, joined by a pull pipe, are in the
[nested-formats guide](https://github.com/xiam/bytescan/blob/master/docs/guides/nested-formats.md) and the
[ssejson example](../streams/ssejson).
