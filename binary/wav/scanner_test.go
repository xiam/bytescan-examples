package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan-examples/internal/hexdump"
	"github.com/xiam/bytescan-examples/internal/lexdump"
	"github.com/xiam/bytescan/bytesource"
	"github.com/xiam/bytescan/streamtest"
)

func TestGolden(t *testing.T) {
	lexdump.Golden(t, hexdump.Dumper, newScanner)
}

// TestLengths checks the inputs that hold no UNKNOWN byte: the tokens tile
// the file, so their lengths sum to its size, and each payload has the length
// that its chunk header states.
func TestLengths(t *testing.T) {
	for _, name := range []string{"document", "chunks", "truncated"} {
		t.Run(name, func(t *testing.T) {
			b, err := os.ReadFile(filepath.Join("testdata", name+".input"))
			if err != nil {
				t.Fatal(err)
			}
			ls := streamtest.LexAll(t, newScanner(bytes.NewReader(b)))
			sum, data := 0, 0
			for i, l := range ls {
				if l.Offset() != sum {
					t.Errorf("token %d (%s) at offset %d, want %d", i, l.Type(), l.Offset(), sum)
				}
				sum += l.Len()
				if l.Type() == bytescan.TokenTypeUnknown {
					t.Errorf("UNKNOWN at offset %d", l.Offset())
				}
				if want, ok := payloads[l.Type()]; ok && i+1 < len(ls) {
					stated := int(binary.LittleEndian.Uint32(bytesource.Bytes(l.Text())[4:]))
					next := ls[i+1]
					if next.Type() != want {
						t.Errorf("%s is followed by %s, want %s", l.Type(), next.Type(), want)
					}
					// Only the last payload can be cut by the end of the input.
					if next.Len() != stated && i+2 < len(ls) {
						t.Errorf("%s at %d has %d bytes, its header states %d", next.Type(), next.Offset(), next.Len(), stated)
					}
					if want == dataChunkPayload && next.Len() == stated {
						data++
					}
				}
			}
			if sum != len(b) {
				t.Errorf("token lengths sum to %d, the file has %d bytes", sum, len(b))
			}
			if name != "truncated" && data != 1 {
				t.Errorf("%d DATA_CHUNK_PAYLOAD tokens of the stated length, want 1", data)
			}
		})
	}
}

// TestReleaseBeforePayload checks rule 1 of the driver: the chunk scanner is
// released before a payload is read, so a cursor whose retention is bounded
// to 1 KiB lexes a data chunk of 64 KiB. Without the Release, the scanner's
// checkpoints would pin the payload and the cursor would fail.
func TestReleaseBeforePayload(t *testing.T) {
	file := riff(chunk("fmt ", pcmFormat(1, 8000, 8)), chunk("data", samples(64<<10)), chunk("note", []byte("end")))
	var got []string
	for _, l := range streamtest.LexRetained(t, bytesource.NewReader(bytes.NewReader(file)), 1<<10, newDriver) {
		got = append(got, string(l.Type()))
	}
	want := "RIFF_HEADER FMT_CHUNK_HEADER FMT_CHUNK_PAYLOAD DATA_CHUNK_HEADER DATA_CHUNK_PAYLOAD CHUNK_HEADER CHUNK_PAYLOAD PAD_BYTE"
	if g := strings.Join(got, " "); g != want {
		t.Errorf("tokens = %s, want %s", g, want)
	}
}
