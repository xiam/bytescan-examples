# cursor/positioned-cursor

A program that uses [`bytescan/cursor`](https://github.com/xiam/bytescan/tree/master/cursor)
on its own, without a scanner. It reads a small document line by line the way
a line-oriented rule does: it holds a checkpoint, reads ahead to the newline,
and only then decides.

**Technique:** a `Checkpoint` held across a read-ahead, so the span of the line
stays retained after the cursor has moved past it; `SpanText` and `Context`
read that span back without moving the cursor.

```sh
go run ./cursor/positioned-cursor
```

For each line the program prints the cursor position where it starts, the
line's text, its span in bytes and in runes, the context around it (up to 4
bytes on each side, clamped to the retained input: the lines before it are
released, so the context starts at the line), and the position of the next
read. The document has multi-byte
runes (`世界`, `🦄`), so the byte and rune offsets of the later lines differ.
After each line the checkpoint is released and the reader can drop the bytes
behind the cursor.

`TestRun` runs the program on its document and checks the whole output.
