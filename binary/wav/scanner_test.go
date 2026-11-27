package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan-examples/internal/hexdump"
	"github.com/xiam/bytescan-examples/internal/lextest"
	"github.com/xiam/bytescan/bytesource"
	"github.com/xiam/bytescan/streamtest"
)

func TestGolden(t *testing.T) {
	lextest.Golden(t, hexdump.Dumper, newScanner)
}

// TestLengths checks the inputs that hold no UNKNOWN byte: the tokens tile
// the file, so their lengths sum to its size, and each payload has the length
// that its chunk header states.
func TestLengths(t *testing.T) {
	for _, name := range []string{"document", "chunks", "truncated"} {
		t.Run(name, func(t *testing.T) {
			b, err := os.ReadFile(filepath.Join("testdata", name+".input"))
			require.NoError(t, err)
			ls := streamtest.LexAll(t, newScanner(bytes.NewReader(b)))
			sum, data := 0, 0
			for i, l := range ls {
				if l.Offset() != sum {
					assert.Failf(t, "", "token %d (%s) at offset %d, want %d", i, l.Type(), l.Offset(), sum)
				}
				sum += l.Len()
				if l.Type() == bytescan.TokenTypeUnknown {
					assert.Failf(t, "", "UNKNOWN at offset %d", l.Offset())
				}
				if want, ok := payloads[l.Type()]; ok && i+1 < len(ls) {
					stated := int(binary.LittleEndian.Uint32(bytesource.Bytes(l.Text())[4:]))
					next := ls[i+1]
					if next.Type() != want {
						assert.Failf(t, "", "%s is followed by %s, want %s", l.Type(), next.Type(), want)
					}
					// Only the last payload can be cut by the end of the input.
					if next.Len() != stated && i+2 < len(ls) {
						assert.Failf(t, "", "%s at %d has %d bytes, its header states %d", next.Type(), next.Offset(), next.Len(), stated)
					}
					if want == dataChunkPayload && next.Len() == stated {
						data++
					}
				}
			}
			if sum != len(b) {
				assert.Failf(t, "", "token lengths sum to %d, the file has %d bytes", sum, len(b))
			}
			assert.Falsef(t, name != "truncated" && data != 1, "%d DATA_CHUNK_PAYLOAD tokens of the stated length, want 1", data)
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
	for _, l := range streamtest.LexRetainedBytes(t, bytes.NewReader(file), 1<<10, newDriver) {
		got = append(got, string(l.Type()))
	}
	want := "RIFF_HEADER FMT_CHUNK_HEADER FMT_CHUNK_PAYLOAD DATA_CHUNK_HEADER DATA_CHUNK_PAYLOAD CHUNK_HEADER CHUNK_PAYLOAD PAD_BYTE"
	{
		g := strings.Join(got, " ")
		assert.Equalf(t, want, g, "tokens = %s, want %s", g, want)
	}
}
