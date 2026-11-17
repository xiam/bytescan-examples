package main

import (
	"io"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/bytesource"
	"github.com/xiam/bytescan/modes"
	"github.com/xiam/bytescan/rules/core"
	"github.com/xiam/bytescan/rules/octet"
)

// The token types. The marker and scan scanners emit the markers, the segment
// headers and the entropy-coded data; the TIFF scanner emits the TIFF header.
// The driver reads the other types from the cursor.
const (
	soi               bytescan.TokenType = "SOI"
	eoi               bytescan.TokenType = "EOI"
	segmentHeader     bytescan.TokenType = "SEGMENT_HEADER"
	exifSegmentHeader bytescan.TokenType = "EXIF_SEGMENT_HEADER"
	segmentPayload    bytescan.TokenType = "SEGMENT_PAYLOAD"
	entropyData       bytescan.TokenType = "ENTROPY_DATA"
	tiffHeader        bytescan.TokenType = "TIFF_HEADER"
	tiffData          bytescan.TokenType = "TIFF_DATA"
	ifdEntryCount     bytescan.TokenType = "IFD_ENTRY_COUNT"
	ifdEntry          bytescan.TokenType = "IFD_ENTRY"
)

// The markers of ITU T.81, table B.1, that the driver tells apart.
const (
	markerSOS = 0xDA // start of scan
)

// newScanner returns a JPEG scanner that reads r: a mode driver over one byte
// cursor, with a scanner for the markers, one for the scan and one for the TIFF
// header of an Exif segment, and a queue of the fields that they announce.
func newScanner(r io.Reader) *driver {
	cur := bytesource.NewCursor(r)
	marker := newMarkerScanner(cur)
	return &driver{
		modes:  modes.New(cur, marker),
		marker: marker,
		scan:   newScanScanner(cur),
		tiff:   newTIFFScanner(cur),
		fields: bytesource.NewFields(cur),
	}
}

// newMarkerScanner returns the scanner for the markers between the segments of
// ITU T.81, Annex B, over cur. Bytes that are not a marker are UNKNOWN.
func newMarkerScanner(cur bytescan.Cursor) *bytescan.Scanner {
	lx := bytescan.NewWithCursor(cur)
	addMarkerRules(lx)
	return lx
}

// newScanScanner returns the scanner for the bytes after a start-of-scan segment
// over cur: the entropy-coded data, and the markers that end it.
//
// ENTROPY_DATA never matches FF followed by a marker byte, and every marker
// rule starts with FF and a marker byte, so no two rules match the same text.
func newScanScanner(cur bytescan.Cursor) *bytescan.Scanner {
	lx := bytescan.NewWithCursor(cur)
	addMarkerRules(lx)
	lx.MustAddRule(entropyData, entropyDataRule())
	return lx
}

// addMarkerRules registers the markers of T.81, section B.1.1.3: SOI, EOI and
// the header of a marker segment, and the header of an Exif APP1 segment.
//
// SEGMENT_HEADER and EXIF_SEGMENT_HEADER both match the first four bytes of
// an Exif segment. The longest match decides: EXIF_SEGMENT_HEADER is ten bytes
// long. SOI and EOI are not segment markers, so no other rules overlap.
func addMarkerRules(lx *bytescan.Scanner) {
	lx.MustAddRule(soi, octet.Literal([]byte{0xFF, 0xD8}))
	lx.MustAddRule(eoi, octet.Literal([]byte{0xFF, 0xD9}))
	lx.MustAddRule(segmentHeader, segmentHeaderRule())
	lx.MustAddRule(exifSegmentHeader, exifSegmentHeaderRule())
}

// segmentHeaderRule returns a rule for the header of a marker segment (T.81,
// section B.1.1.4): FF, a marker byte, and the two-byte length of the segment.
// The marker bytes are C0 to FE except the restart markers D0 to D7 and SOI
// and EOI, which have no length:
//
//	\xff[\xc0-\xcf\xda-\xfe][\x00-\xff]{2}
func segmentHeaderRule() bytescan.Rule {
	return core.Sequence(
		octet.Literal([]byte{0xFF}),
		core.Choice(octet.Range(0xC0, 0xCF), octet.Range(0xDA, 0xFE)),
		octet.Fixed(2),
	)
}

// exifSegmentHeaderRule returns a rule for the header of an APP1 segment and
// the Exif identifier that starts its payload (Exif 2.3, section 4.5). The
// length must be 8 or more, so that the segment holds the six bytes of the
// identifier; a shorter APP1 segment is a SEGMENT_HEADER:
//
//	\xff\xe1([\x01-\xff][\x00-\xff]|\x00[\x08-\xff])Exif\x00\x00
func exifSegmentHeaderRule() bytescan.Rule {
	return core.Sequence(
		octet.Literal([]byte{0xFF, 0xE1}),
		core.Choice(
			core.Sequence(octet.Range(0x01, 0xFF), octet.Any()),
			core.Sequence(octet.Set(0x00), octet.Range(0x08, 0xFF)),
		),
		octet.Literal([]byte("Exif\x00\x00")),
	)
}

// entropyDataRule returns a rule for entropy-coded data (T.81, section
// B.1.1.5): any byte but FF, or FF followed by a stuffed 00 or by a restart
// marker byte, D0 to D7:
//
//	([^\xff]|\xff[\x00\xd0-\xd7])+
//
// The restart markers are part of the data. The data ends before FF and any
// other byte.
func entropyDataRule() bytescan.Rule {
	return core.OneOrMore(core.Choice(
		octet.Range(0x00, 0xFE),
		core.Sequence(
			octet.Literal([]byte{0xFF}),
			core.Choice(octet.Set(0x00), octet.Range(0xD0, 0xD7)),
		),
	))
}

// newTIFFScanner returns the scanner for the TIFF header at the start of the TIFF
// structure in an Exif segment (TIFF 6.0, section 2; Exif 2.3, section 4.5):
// the byte order "II" or "MM", the number 42 in that order, and the four-byte
// offset of the first IFD:
//
//	II\x2a\x00[\x00-\xff]{4}|MM\x00\x2a[\x00-\xff]{4}
//
// It has one rule, so no tie arises.
func newTIFFScanner(cur bytescan.Cursor) *bytescan.Scanner {
	lx := bytescan.NewWithCursor(cur)
	lx.MustAddRule(tiffHeader, core.Choice(
		octet.Record([]byte("II\x2a\x00"), 4),
		octet.Record([]byte("MM\x00\x2a"), 4),
	))
	return lx
}
