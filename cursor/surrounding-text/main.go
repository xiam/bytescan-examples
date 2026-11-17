// Command surrounding-text shows the io.Reader and io.Seeker side of
// bytescan/cursor: it reads a document rune by rune until a marker, reports
// the marker's line, column and offset, then seeks back and reads the text
// around it.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/xiam/bytescan/cursor"
)

const contextBytes = 20

const document = `This is a simple document.
It has multiple lines of standard text.
Our goal is to find the special marker * right here.
The rest of the document follows.`

// run reads r until the first '*' and writes its position and the
// contextBytes bytes on each side of it to w.
func run(w io.Writer, r io.Reader) error {
	reader := cursor.New(r)

	fmt.Fprintln(w, "Searching for marker '*' in the document...")

	// Read rune by rune until we find our marker or reach the end.
	for {
		r, _, err := reader.ReadRune()
		if err != nil {
			if err == io.EOF {
				fmt.Fprintln(w, "Marker not found.")
				return nil
			}
			return fmt.Errorf("reading rune: %w", err)
		}

		// We found the marker.
		if r == '*' {
			markerPos := reader.Pos()
			fmt.Fprintf(w, "Found marker at Line %d, Column %d (Offset: %d)\n",
				markerPos.Line(), markerPos.Column(), markerPos.Offset())

			_, err := reader.Seek(-contextBytes, io.SeekCurrent)
			if err != nil {
				// If the marker is too close to the start, seek might fail.
				// In that case, we just go to the very beginning of the
				// stream.
				_, _ = reader.Seek(0, io.SeekStart)
			}

			contextBuffer := make([]byte, contextBytes*2)
			n, _ := reader.Read(contextBuffer)

			fmt.Fprintf(w, "Context around marker: %q\n", contextBuffer[:n])
			return nil
		}
	}
}

func main() {
	if err := run(os.Stdout, strings.NewReader(document)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
