# cursor/surrounding-text

A program that uses [`bytescan/cursor`](https://github.com/xiam/bytescan/tree/master/cursor)
on its own, without a scanner, as an `io.RuneReader`, an `io.Seeker` and an
`io.Reader`.

**Technique:** read rune by rune to a marker, take its line, column and offset
from `Pos`, then `Seek` back 20 bytes and `Read` the text around the marker.

```sh
go run ./cursor/surrounding-text
```

The program searches its document for `*` and prints where the marker is and
the 40 bytes around it. When the marker is closer than 20 bytes to the start,
it reads from the start instead. A text without the marker prints `Marker not
found.`

`TestRun` runs the program on its document and on a text without the marker,
and checks the whole output.
