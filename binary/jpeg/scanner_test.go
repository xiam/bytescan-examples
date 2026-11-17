package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan-examples/internal/hexdump"
	"github.com/xiam/bytescan-examples/internal/lexdump"
	"github.com/xiam/bytescan/bytesource"
)

func TestGolden(t *testing.T) {
	lexdump.Golden(t, hexdump.Dumper, newScanner)
}

// lexFile returns the tokens of a test input.
func lexFile(t *testing.T, name string) []*bytescan.Token {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name+".input"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var ls []*bytescan.Token
	d := newScanner(f)
	for {
		l, err := d.Next()
		if err == io.EOF {
			return ls
		}
		if err != nil {
			t.Fatal(err)
		}
		ls = append(ls, l)
	}
}

// TestPayloadMarkers checks that a payload that contains FF D9 is one
// SEGMENT_PAYLOAD, and that EOI appears once, at the end.
func TestPayloadMarkers(t *testing.T) {
	ls := lexFile(t, "segments")
	found := false
	for i, l := range ls {
		if l.Type() == eoi && i != len(ls)-1 {
			t.Errorf("EOI at token %d of %d", i, len(ls))
		}
		if l.Type() == segmentPayload && bytes.Contains(bytesource.Bytes(l.Text()), []byte{0xFF, 0xD9}) {
			found = true
		}
	}
	if !found {
		t.Error("no SEGMENT_PAYLOAD contains FF D9")
	}
	if last := ls[len(ls)-1]; last.Type() != eoi {
		t.Errorf("last token is %s, want EOI", last.Type())
	}
}

// TestExif checks the Exif samples of both byte orders: one TIFF header in
// that order, and one IFD_ENTRY token per entry of the first IFD.
func TestExif(t *testing.T) {
	for _, tc := range []struct {
		name    string
		order   string
		entries int
	}{
		{"document", "II", 3},
		{"exif-be", "MM", 2},
	} {
		var headers []string
		entries := 0
		for _, l := range lexFile(t, tc.name) {
			switch l.Type() {
			case tiffHeader:
				headers = append(headers, string(bytesource.Bytes(l.Text())[:2]))
			case ifdEntry:
				entries++
			}
		}
		if len(headers) != 1 || headers[0] != tc.order {
			t.Errorf("%s: TIFF headers %q, want one %q", tc.name, headers, tc.order)
		}
		if entries != tc.entries {
			t.Errorf("%s: %d IFD_ENTRY tokens, want %d", tc.name, entries, tc.entries)
		}
	}
}
