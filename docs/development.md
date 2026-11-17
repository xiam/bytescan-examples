# Working on the examples

This page is for people who change the examples or test them against a new
bytescan revision.

## Repository layout

```text
<name>/                   one text example per directory (json, go, sql, ...)
binary/<format>/          the binary file examples, with one README.md
streams/<name>/           the long-running stream examples
streams/sse/ssedriver/    the SSE mode driver, a package that ssejson imports
cursor/<name>/            programs that use the cursor package on its own
internal/lexdump/         the dumper and golden-file checks every example uses
internal/hexdump/         the hex view of the binary and byte-stream examples
internal/streamkit/       shared helpers of the stream examples
internal/guardtest/       the per-token guard checks of the stream examples
docs/                     the pages that go deeper than the README
```

## Running the examples

`go.mod` requires the released `github.com/xiam/bytescan v0.4.0`, so the
examples build on their own:

```sh
git clone https://github.com/xiam/bytescan-examples.git
cd bytescan-examples
go test -race ./...
```

How to run one example, and how to update its golden files, is in
[the example standard](standard.md#running).

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

## Issue references in old commits

Commits from before the examples came to this repository keep their messages.
Their `#N` and `!N` references point at the issues and merge requests of
[bytescan](https://github.com/xiam/bytescan), not of this repository.
`git log --follow` shows the full history of every file.
