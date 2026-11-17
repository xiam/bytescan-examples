package main

import (
	"strings"
	"testing"
)

// TestRun runs the program on its document and checks the whole output: the
// spans count bytes and runes apart on the lines with multi-byte runes, and
// the context comes from the retained input.
func TestRun(t *testing.T) {
	const want = `line starting at 1:0: "theme: dark"
  span: bytes 0..12, runes 0..12, 1:0>2:0
  context: "theme: dark\\nname"
  next read is at 2:0 (byte 12)

line starting at 2:0: "name: 世界"
  span: bytes 12..25, runes 12..21, 2:0>3:0
  context: "name: 世界\\nbrok"
  next read is at 3:0 (byte 25)

line starting at 3:0: "broken: 🦄"
  span: bytes 25..38, runes 21..31, 3:0>4:0
  context: "broken: 🦄\\n"
  next read is at 4:0 (byte 38)

done
`
	var out strings.Builder
	if err := run(&out, strings.NewReader(document)); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := out.String(); got != want {
		t.Errorf("output:\n%s\nwant:\n%s", got, want)
	}
}
