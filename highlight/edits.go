package main

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/xiam/bytescan/statelex"
)

// readEdits reads a list of edits, one per line:
//
//	START END "TEXT"
//
// The bytes [START, END) of the text before the edit become TEXT. START and
// END are byte offsets. TEXT is a Go quoted string, and a deletion leaves it
// out. A blank line and a line that starts with # are skipped.
func readEdits(r io.Reader) ([]statelex.Edit, error) {
	var edits []statelex.Edit
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		e, err := parseEdit(line)
		if err != nil {
			return nil, fmt.Errorf("edits line %d: %w", n, err)
		}
		edits = append(edits, e)
	}
	return edits, sc.Err()
}

func parseEdit(line string) (statelex.Edit, error) {
	start, rest, _ := strings.Cut(line, " ")
	end, quoted, _ := strings.Cut(strings.TrimSpace(rest), " ")
	var e statelex.Edit
	var err error
	if e.Start, err = strconv.Atoi(start); err != nil {
		return e, fmt.Errorf("start: %w", err)
	}
	if e.End, err = strconv.Atoi(end); err != nil {
		return e, fmt.Errorf("end: %w", err)
	}
	if quoted = strings.TrimSpace(quoted); quoted != "" {
		text, err := strconv.Unquote(quoted)
		if err != nil {
			return e, fmt.Errorf("text %s: %w", quoted, err)
		}
		e.Text = []byte(text)
	}
	return e, nil
}

// describe returns an edit in words: "insert TEXT at N", "delete [S,E)" or
// "replace [S,E) with TEXT".
func describe(e statelex.Edit) string {
	switch {
	case e.Start == e.End:
		return fmt.Sprintf("insert %q at %d", e.Text, e.Start)
	case len(e.Text) == 0:
		return fmt.Sprintf("delete [%d,%d)", e.Start, e.End)
	default:
		return fmt.Sprintf("replace [%d,%d) with %q", e.Start, e.End, e.Text)
	}
}
