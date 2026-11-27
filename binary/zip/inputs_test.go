package main

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"hash/crc32"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xiam/bytescan-examples/internal/lextest"
)

// The binary test inputs are built here, so the testdata/*.input files are
// reproducible: TestInputs checks that each committed input equals the bytes
// built below, and go test -update rewrites the inputs before TestGolden
// rewrites the golden files.

// notes is the text of the deflated file of the document archive, and
// notesDeflate its raw DEFLATE stream (RFC 1951), as compress/flate wrote it.
// The stream is a literal, so the archive does not change when the compressor
// of a later Go release writes other bytes; TestNames checks that it inflates
// to notes.
const notes = "Notes\n=====\n\nThe lexer reads runes, one byte each.\n"

var notesDeflate = mustHex("f2cb2f492de6b205012eae908c54859cd48ad42285a2d4c4946285a2d2bcd4621d85fcbc5485a4ca925485d4c4e40c3d2e40000000ffff")

// inputs maps the name of each test case to the bytes of its input.
var inputs = map[string]func(t *testing.T) []byte{
	"document":   document,
	"empty":      empty,
	"descriptor": descriptor,
	"zip64":      zip64,
	"truncated":  truncated,
	"not-zip":    notZip,
}

func TestInputs(t *testing.T) {
	lextest.Inputs(t, inputs)
}

// rawFile is one file that archive/zip writes with CreateRaw: no data
// descriptor, so the sizes and the CRC are in the local file header.
type rawFile struct {
	name, comment string
	extra         []byte
	method        uint16
	data          []byte // as stored in the archive
	plain         []byte // after decompression
}

func writeRaw(t *testing.T, comment string, files ...rawFile) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		w, err := zw.CreateRaw(&zip.FileHeader{
			Name:               f.name,
			Comment:            f.comment,
			Extra:              f.extra,
			Method:             f.method,
			ModifiedDate:       0x5b6e, // 2025-11-14
			ModifiedTime:       0x6000, // 12:00:00
			CRC32:              crc32.ChecksumIEEE(f.plain),
			CompressedSize64:   uint64(len(f.data)),
			UncompressedSize64: uint64(len(f.plain)),
		})
		require.NoError(t, err)
		_, err = w.Write(f.data)
		require.NoError(t, err)
	}
	require.NoError(t, zw.SetComment(comment))
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// document is an archive with a stored file and a deflated file, an extra
// field, a file comment and an archive comment.
func document(t *testing.T) []byte {
	hello := []byte("Hello, ZIP!\n")
	// An extra field block (APPNOTE 4.5.1): header ID 0xCAFE, data size 4.
	extra := []byte{0xFE, 0xCA, 0x04, 0x00, 0xDE, 0xAD, 0xBE, 0xEF}
	return writeRaw(t, "two files",
		rawFile{name: "hello.txt", comment: "stored", method: zip.Store, data: hello, plain: hello},
		rawFile{name: "docs/notes.md", extra: extra, method: zip.Deflate, data: notesDeflate, plain: []byte(notes)},
	)
}

// empty is an archive with no file: only the end of central directory record.
func empty(t *testing.T) []byte {
	return writeRaw(t, "")
}

// descriptor is an archive written by zip.Writer.CreateHeader, which sets bit 3 of
// the general-purpose flag and writes a data descriptor after the file data.
// That is out of the subset: the local file header has a compressed size of 0.
func descriptor(t *testing.T) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.CreateHeader(&zip.FileHeader{Name: "a.txt", Method: zip.Store, ModifiedDate: 0x5b6e, ModifiedTime: 0x6000})
	require.NoError(t, err)
	_, err = w.Write([]byte("PK\x01\x02 in data\n"))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// zip64 is a one-file archive in the ZIP64 form (APPNOTE 4.4.8, 4.3.14 and
// 4.3.15), written byte by byte: the sizes in the headers are 0xFFFFFFFF and
// the real sizes are in a ZIP64 extra field.
func zip64(t *testing.T) []byte {
	data := []byte("big\n")
	crc := crc32.ChecksumIEEE(data)
	le16 := binary.LittleEndian.AppendUint16
	le32 := binary.LittleEndian.AppendUint32
	le64 := binary.LittleEndian.AppendUint64

	var extra []byte // ZIP64 extended information extra field
	extra = le16(extra, 0x0001)
	extra = le16(extra, 16)
	extra = le64(extra, uint64(len(data)))
	extra = le64(extra, uint64(len(data)))

	var b []byte
	b = append(b, "PK\x03\x04"...)
	b = le16(b, 45) // version needed: 4.5
	b = le16(b, 0)  // flags
	b = le16(b, 0)  // stored
	b = le16(b, 0x6000)
	b = le16(b, 0x5b6e)
	b = le32(b, crc)
	b = le32(b, 0xFFFFFFFF)
	b = le32(b, 0xFFFFFFFF)
	b = le16(b, 5)
	b = le16(b, uint16(len(extra)))
	b = append(b, "b.txt"...)
	b = append(b, extra...)
	b = append(b, data...)

	cd := len(b)
	b = append(b, "PK\x01\x02"...)
	b = le16(b, 45)
	b = le16(b, 45)
	b = le16(b, 0)
	b = le16(b, 0)
	b = le16(b, 0x6000)
	b = le16(b, 0x5b6e)
	b = le32(b, crc)
	b = le32(b, 0xFFFFFFFF)
	b = le32(b, 0xFFFFFFFF)
	b = le16(b, 5)
	b = le16(b, uint16(len(extra)))
	b = le16(b, 0) // comment length
	b = le16(b, 0) // disk
	b = le16(b, 0) // internal attributes
	b = le32(b, 0) // external attributes
	b = le32(b, 0) // local header offset
	b = append(b, "b.txt"...)
	b = append(b, extra...)
	cdSize := len(b) - cd

	eocd64 := len(b)
	b = append(b, "PK\x06\x06"...)
	b = le64(b, 44)
	b = le16(b, 45)
	b = le16(b, 45)
	b = le32(b, 0)
	b = le32(b, 0)
	b = le64(b, 1)
	b = le64(b, 1)
	b = le64(b, uint64(cdSize))
	b = le64(b, uint64(cd))

	b = append(b, "PK\x06\x07"...)
	b = le32(b, 0)
	b = le64(b, uint64(eocd64))
	b = le32(b, 1)

	b = append(b, "PK\x05\x06"...)
	b = le16(b, 0xFFFF)
	b = le16(b, 0xFFFF)
	b = le16(b, 0xFFFF)
	b = le16(b, 0xFFFF)
	b = le32(b, 0xFFFFFFFF)
	b = le32(b, 0xFFFFFFFF)
	b = le16(b, 0)
	return b
}

// truncated is the document archive cut inside the file data of its second
// file.
func truncated(t *testing.T) []byte {
	b := document(t)
	i := bytes.Index(b, notesDeflate)
	return b[:i+10]
}

// notZip holds near-misses of the three signatures, a record cut by the end
// of the input, and text.
func notZip(t *testing.T) []byte {
	return []byte("PK\x03\x03 PK\x01\x01 PK\x05\x05 pk\x03\x04 PK\x05\x06\x00\x00")
}

func mustHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}
