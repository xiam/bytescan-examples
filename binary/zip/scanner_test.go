package main

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xiam/bytescan-examples/internal/hexdump"
	"github.com/xiam/bytescan-examples/internal/lextest"
	"github.com/xiam/bytescan/bytesource"
	"github.com/xiam/bytescan/streamtest"
)

func TestGolden(t *testing.T) {
	lextest.Golden(t, hexdump.Dumper, newScanner)
}

// TestNames checks the document archive against archive/zip: the FILE_NAME
// tokens are the names that archive/zip reports, once for the local file
// headers and once for the central directory, and every file reads back.
func TestNames(t *testing.T) {
	path := filepath.Join("testdata", "document.input")
	zr, err := zip.OpenReader(path)
	require.NoError(t, err)
	defer zr.Close()
	var want []string
	for _, f := range zr.File {
		want = append(want, f.Name)
		rc, err := f.Open()
		require.NoError(t, err)
		{
			_, err := io.Copy(io.Discard, rc)
			require.NoErrorf(t, err, "%s: %v", f.Name, err)
		}
		rc.Close()
	}
	if len(want) != 2 {
		require.FailNowf(t, "", "archive/zip reports %d files, want 2", len(want))
	}
	want = append(want, want...)

	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()
	var got []string
	d := newScanner(f)
	for {
		l, err := d.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		if l.Type() == fileName {
			got = append(got, string(bytesource.Bytes(l.Text())))
		}
	}
	require.Equalf(t, len(want), len(got), "FILE_NAME tokens = %q, want %q", got, want)
	for i := range got {
		assert.Equalf(t, want[i], got[i], "FILE_NAME %d = %q, want %q", i, got[i], want[i])
	}
}

// TestReleaseBeforeData checks rule 1 of the driver: the record scanner is
// released before the file data is read, so a cursor whose retention is
// bounded to 1 KiB lexes a file of 64 KiB. Without the Release, the scanner's
// checkpoints would pin the file data and the cursor would fail.
func TestReleaseBeforeData(t *testing.T) {
	data := bytes.Repeat([]byte("PK\x05\x06\xff"), 64<<10/5)
	archive := writeRaw(t, "", rawFile{name: "big", method: zip.Store, data: data, plain: data})
	var got []string
	for _, l := range streamtest.LexRetainedBytes(t, bytes.NewReader(archive), 1<<10, newDriver) {
		got = append(got, string(l.Type()))
	}
	want := "LOCAL_FILE_HEADER FILE_NAME FILE_DATA CENTRAL_DIRECTORY_HEADER FILE_NAME END_OF_CENTRAL_DIRECTORY"
	{
		g := strings.Join(got, " ")
		assert.Equalf(t, want, g, "tokens = %s, want %s", g, want)
	}
}

// TestFileDataPieces checks that file data longer than maxPiece is read in
// pieces of at most maxPiece bytes that tile it.
func TestFileDataPieces(t *testing.T) {
	data := bytes.Repeat([]byte{0x5A}, 150<<10)
	archive := writeRaw(t, "", rawFile{name: "big", method: zip.Store, data: data, plain: data})
	pieces, sum := 0, 0
	for _, l := range streamtest.LexAll(t, newScanner(bytes.NewReader(archive))) {
		if l.Type() != fileData {
			continue
		}
		if l.Len() > maxPiece {
			assert.Failf(t, "", "piece %d has %d bytes, more than %d", pieces, l.Len(), maxPiece)
		}
		pieces++
		sum += l.Len()
	}
	if sum != len(data) || pieces != 3 {
		assert.Failf(t, "", "%d FILE_DATA pieces of %d bytes, want 3 of %d", pieces, sum, len(data))
	}
}
