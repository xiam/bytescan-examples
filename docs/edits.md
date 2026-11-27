# Text that changes

Some examples lex a text that changes: they read a text and a list of edits,
or keys, and they update their result after each edit without lexing the
whole text again. They use bytescan's lexer with state,
[`statelex`](https://pkg.go.dev/github.com/xiam/bytescan/statelex). The guide
[Lexing text that changes](https://github.com/xiam/bytescan/blob/master/docs/guides/text-that-changes.md)
explains it. The [highlight](../highlight) example is the reference.

These examples follow the [example standard](standard.md), with the
differences below.

## Input

- The program reads the text from the file named on the command line, or
  from standard input when there is no name.
- It reads the edits from the file of the `-edits` flag. Without the flag,
  it only loads the text.
- One edit is one line: `START END "TEXT"`. The bytes `[START, END)` of the
  text before the edit become `TEXT`, a Go quoted string. A deletion leaves
  `TEXT` out. A blank line and a line that starts with `#` are skipped.
- Loading the text is one edit of an empty lexer: an insertion of the whole
  text at offset 0.

## Layout

```text
<name>/
  main.go            package main: reads the text and the edits, runs them
  scanner.go         newModeSet, the token types and the rules
  edits.go           readEdits, the reader of the edits file
  <consumer>.go      the code that uses the changes
  <name>_test.go     the golden test and the repaint test
  README.md
  testdata/
    <case>.input     the text of one case
    <case>.edits     its edits; a case without this file only loads the text
    <case>.golden    the expected output of the case
```

- **The consumer** is in a file of its own. When another example uses it, it
  is a package of its own instead: the highlighter is
  [`highlight/paint`](../highlight/paint), which [repl](../repl) imports.
- **`scanner.go`** has `newModeSet() *modes.ModeSet` in place of
  `newScanner`. The lexer with state takes a
  [`modes.ModeSet`](https://pkg.go.dev/github.com/xiam/bytescan/modes#ModeSet).
  A language without modes is a set with one mode.
- **The rules** come only from the `rules/` packages of bytescan, as for
  every example. They must be pure: a rule keeps its progress only in the
  rules it returns. The rules of `rules/core` and `rules/base` are pure. The
  guide says why
  ([Pure rules](https://github.com/xiam/bytescan/blob/master/docs/guides/text-that-changes.md#pure-rules)).

## Keys instead of an edits file

The [repl](../repl) example reads its edits from the keyboard, so its input
differs:

- It reads standard input as a list of keys. Each key that changes the entry
  is one edit at the end of the entry. Enter asks the program to run the
  entry or to wait for one more line.
- Each case in `testdata` is one file of keys, `<case>.input`, and its
  expected output, `<case>.golden`. There is no `.edits` file. The test
  builds the keys and checks them against the committed files
  (`lextest.Inputs`), because a backspace is a control byte.
- Besides the change, the program asks the lexer two questions on Enter:
  `Lexer.Unfinished` and `Lexer.ModeAtEnd`. It has no code of its own that
  looks for an open string or comment.

## The consumer

The consumer is the code that keeps the program's own result, such as the
painted tokens of a highlighter.

- It knows token types and byte positions only. It holds no rule, no mode
  and no lexer state.
- It takes one `statelex.Change` for each edit, and that change is the only
  thing it reads from the lexer to update its result. The program may read
  the text from the lexer (`Lexer.Text`) to print it.
- It has no lexing code of its own. If `statelex` lacks something the
  example needs, report it in the merge request and as a bytescan issue, as
  for a missing rule.

## Tests

- **Golden test.** Each case in `testdata` runs through `run`, and its output
  is compared with `<case>.golden` through `lextest.Compare`. `go test
  -update` rewrites the golden files.
- **Repaint test.** After each edit of each case, the consumer's result
  equals the result of a new consumer that processes the final text from the
  start. A second test does the same for many generated edits from a fixed
  seed.
- **Cases.** Name a case for the edit it shows. Cover the edits of the
  [edit cases](https://github.com/xiam/bytescan/blob/master/docs/specs/resumable-lexing.md#edit-cases)
  that the example's language can show: typing and backspaces, a deleted
  selection, a deleted chunk at the start, a paste over everything, and a
  string or a comment that opens and closes.

The tests need no terminal and no network, and they take under 2 s with
`go test -race`.

## README

The README has the parts of the [standard](standard.md#the-covered-subset),
and three more after **Tokens**:

- **Edits:** the format of the edits file, with an example.
- **Output:** what the program prints for each edit, with an example.
- **How the consumer works:** what it does with each change.
