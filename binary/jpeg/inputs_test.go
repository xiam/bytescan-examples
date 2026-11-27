package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The binary test inputs are built here, so the testdata/*.input files are
// reproducible: go test -update rewrites the inputs before TestGolden
// rewrites the golden files. TestInputs checks that each committed input
// equals the bytes built below. The document input holds the output of
// image/jpeg, which a later Go release may change, so for it TestInputs
// checks only that image/jpeg decodes the committed file.

// inputs maps the name of each test case to the bytes of its input.
var inputs = map[string]func(t *testing.T) []byte{
	"document":    document,
	"exif-be":     exifBE,
	"segments":    segments,
	"scan":        scan,
	"truncated":   truncated,
	"near-misses": nearMisses,
	"short-exif":  shortExif,
}

// encoded names the inputs that hold encoder output.
var encoded = map[string]bool{"document": true, "truncated": true}

func TestInputs(t *testing.T) {
	update := flag.Lookup("update").Value.String() == "true"
	for name, build := range inputs {
		path := filepath.Join("testdata", name+".input")
		got := build(t)
		if update {
			require.NoError(t, os.WriteFile(path, got, 0o644))
		}
		want, err := os.ReadFile(path)
		require.NoError(t, err)
		if encoded[name] {
			continue
		}
		assert.Truef(t, bytes.Equal(got, want), "%s differs from the bytes built in inputs_test.go; run go test -update", path)
	}
	f, err := os.Open(filepath.Join("testdata", "document.input"))
	require.NoError(t, err)
	defer f.Close()
	{
		_, err := jpeg.Decode(f)
		assert.NoErrorf(t, err, "image/jpeg does not decode document.input: %v", err)
	}
}

// segment returns a marker segment: FF, the marker, the big-endian length
// and the payload.
func segment(marker byte, payload []byte) []byte {
	b := []byte{0xFF, marker}
	b = binary.BigEndian.AppendUint16(b, uint16(len(payload)+2))
	return append(b, payload...)
}

// entry is one IFD entry: tag, type, count, and value or offset.
type entry struct {
	tag, typ uint16
	count    uint32
	value    uint32
}

// exifPayload returns the payload of an Exif APP1 segment: the identifier,
// a TIFF header in the given byte order, gap bytes before the first IFD, the
// IFD with its entries and a next-IFD offset of 0, and the value area.
func exifPayload(order binary.AppendByteOrder, gap int, entries []entry, values []byte) []byte {
	b := []byte("Exif\x00\x00")
	tiff := []byte("II\x2a\x00")
	if order == binary.AppendByteOrder(binary.BigEndian) {
		tiff = []byte("MM\x00\x2a")
	}
	tiff = order.AppendUint32(tiff, uint32(8+gap))
	tiff = append(tiff, make([]byte, gap)...)
	tiff = order.AppendUint16(tiff, uint16(len(entries)))
	for _, e := range entries {
		tiff = order.AppendUint16(tiff, e.tag)
		tiff = order.AppendUint16(tiff, e.typ)
		tiff = order.AppendUint32(tiff, e.count)
		tiff = order.AppendUint32(tiff, e.value)
	}
	tiff = order.AppendUint32(tiff, 0)
	tiff = append(tiff, values...)
	return append(b, tiff...)
}

// The TIFF field types used in the samples (TIFF 6.0, section 2).
const (
	typeASCII = 2
	typeShort = 3
)

// document is a 16×8 image that image/jpeg encodes, with a little-endian
// Exif segment of three entries after SOI: Make (an offset into the value
// area), Orientation and YCbCrPositioning.
func document(t *testing.T) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 16, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 16; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 16), uint8(y * 32), 0x80, 0xFF})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 50}))
	// The value area starts after the header (8), the count (2), three
	// entries (36) and the next-IFD offset (4).
	exif := exifPayload(binary.LittleEndian, 0, []entry{
		{0x010F, typeASCII, 6, 50}, // Make: "lexer\x00" at offset 50
		{0x0112, typeShort, 1, 1},  // Orientation: top-left
		{0x0213, typeShort, 1, 1},  // YCbCrPositioning: centred
	}, []byte("lexer\x00"))
	enc := buf.Bytes()
	out := append([]byte{}, enc[:2]...) // SOI
	out = append(out, segment(0xE1, exif)...)
	return append(out, enc[2:]...)
}

// exifBE is a JPEG skeleton with a big-endian Exif segment of two entries,
// whose first IFD starts 2 bytes after the TIFF header.
func exifBE(t *testing.T) []byte {
	exif := exifPayload(binary.BigEndian, 2, []entry{
		{0x0112, typeShort, 1, 6 << 16}, // Orientation: a SHORT in the high half
		{0x011A, 5, 1, 38},              // XResolution: a RATIONAL at offset 38
	}, []byte{0, 0, 0, 72, 0, 0, 0, 1})
	b := []byte{0xFF, 0xD8}
	b = append(b, segment(0xE1, exif)...)
	return append(b, 0xFF, 0xD9)
}

// segments holds segments whose payloads look like markers: a COM segment
// that contains FF D9 and FF D8, an APP1 segment that is not Exif (XMP), an
// APP0 JFIF segment, and an empty COM segment.
func segments(t *testing.T) []byte {
	b := []byte{0xFF, 0xD8}
	b = append(b, segment(0xE0, []byte("JFIF\x00\x01\x02\x00\x00\x01\x00\x01\x00\x00"))...)
	b = append(b, segment(0xE1, []byte("http://ns.adobe.com/xap/1.0/\x00<x/>"))...)
	b = append(b, segment(0xFE, []byte("end \xff\xd9 start \xff\xd8"))...)
	b = append(b, segment(0xFE, nil)...)
	return append(b, 0xFF, 0xD9)
}

// scan holds two scans whose entropy-coded data contains stuffed bytes
// (FF 00) and restart markers (FF D0, FF D1), a DHT segment between the
// scans, and fill bytes (FF FF) before EOI.
func scan(t *testing.T) []byte {
	sos := []byte{0x01, 0x01, 0x00, 0x00, 0x3F, 0x00}
	b := []byte{0xFF, 0xD8}
	b = append(b, segment(0xDA, sos)...)
	b = append(b, 0x12, 0xFF, 0x00, 0x34, 0xFF, 0xD0, 0x56, 0xFF, 0xD1, 0x78)
	b = append(b, segment(0xC4, []byte{0x00, 0x01, 0x02})...)
	b = append(b, segment(0xDA, sos)...)
	b = append(b, 0x9A, 0xBC, 0xFF, 0xFF, 0xD9)
	return b
}

// truncated is the document cut inside its entropy-coded data.
func truncated(t *testing.T) []byte {
	b := document(t)
	return b[:len(b)-10]
}

// nearMisses holds a restart marker and a stuffed byte outside a scan, a lone
// FF, an Exif segment with the byte order "XX", an Exif segment too short for
// a TIFF header, and an Exif segment whose IFD offset points past its end.
func nearMisses(t *testing.T) []byte {
	b := []byte{0xFF, 0xD8, 0xFF, 0xD0, 0xFF, 0x00, 0xFF}
	b = append(b, segment(0xE1, []byte("Exif\x00\x00XX\x2a\x00\x08\x00\x00\x00"))...)
	b = append(b, segment(0xE1, []byte("Exif\x00\x00II"))...)
	b = append(b, segment(0xE1, []byte("Exif\x00\x00II\x2a\x00\xff\x00\x00\x00\x01\x00"))...)
	return append(b, 0xFF, 0xD9)
}

// shortExif holds APP1 segments of the lengths 2 to 8, each followed by the
// Exif identifier. Only the segment of length 8 holds the whole identifier;
// in the others, the identifier bytes after the end of the segment are
// outside it.
func shortExif(t *testing.T) []byte {
	b := []byte{0xFF, 0xD8}
	for n := 2; n <= 8; n++ {
		b = append(b, 0xFF, 0xE1, 0x00, byte(n))
		b = append(b, "Exif\x00\x00"...)
	}
	return append(b, 0xFF, 0xD9)
}
