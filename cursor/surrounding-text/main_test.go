package main

import (
	"strings"
	"testing"
)

// TestRun runs the program on fixed inputs and checks the whole output: the
// document, where the marker is found and its context read back after a
// seek, and a text without the marker.
func TestRun(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"document", document, `Searching for marker '*' in the document...
Found marker at Line 3, Column 40 (Offset: 107)
Context around marker: "the special marker * right here.\nThe res"
`},
		{"no marker", "no marker here\n", `Searching for marker '*' in the document...
Marker not found.
`},
	}
	for _, tt := range tests {
		var out strings.Builder
		if err := run(&out, strings.NewReader(tt.in)); err != nil {
			t.Fatalf("%s: run: %v", tt.name, err)
		}
		if got := out.String(); got != tt.want {
			t.Errorf("%s: output:\n%s\nwant:\n%s", tt.name, got, tt.want)
		}
	}
}
