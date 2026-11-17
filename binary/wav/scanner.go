package main

import (
	"io"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/bytesource"
	"github.com/xiam/bytescan/modes"
	"github.com/xiam/bytescan/rules/core"
	"github.com/xiam/bytescan/rules/octet"
)

// The token types. The file scanner emits RIFF_HEADER and the chunk scanner the
// three chunk headers; the driver reads the payloads and the pad bytes.
const (
	riffHeader       bytescan.TokenType = "RIFF_HEADER"
	fmtChunkHeader   bytescan.TokenType = "FMT_CHUNK_HEADER"
	fmtChunkPayload  bytescan.TokenType = "FMT_CHUNK_PAYLOAD"
	dataChunkHeader  bytescan.TokenType = "DATA_CHUNK_HEADER"
	dataChunkPayload bytescan.TokenType = "DATA_CHUNK_PAYLOAD"
	chunkHeader      bytescan.TokenType = "CHUNK_HEADER"
	chunkPayload     bytescan.TokenType = "CHUNK_PAYLOAD"
	padByte          bytescan.TokenType = "PAD_BYTE"
)

// newScanner returns a WAV scanner that reads r: a mode driver over one byte
// cursor, with a scanner for the RIFF header, one for the chunk headers, and a
// queue of the payloads that the chunk headers announce.
func newScanner(r io.Reader) *driver {
	return newDriver(bytesource.NewCursor(r))
}

// newDriver returns the driver over cur. The mode driver starts in file mode
// and switches to chunk mode after the RIFF header.
func newDriver(cur bytescan.Cursor) *driver {
	chunks := newChunkScanner(cur)
	toChunks := func(_ *bytescan.Scanner, l *bytescan.Token) *bytescan.Scanner {
		if l.Type() == riffHeader {
			return chunks
		}
		return nil
	}
	return &driver{
		modes:  modes.New(cur, newFileScanner(cur), modes.WithSwitch(toChunks)),
		fields: bytesource.NewFields(cur),
	}
}

// newFileScanner returns the scanner for the RIFF header at the start of a WAV
// file over cur: the chunk id "RIFF", the four-byte size of the file after
// the size field, and the form type "WAVE" (RIFF, "WAVE Form Type"):
//
//	RIFF[\x00-\xff]{4}WAVE
//
// It has one rule, so no tie arises. Bytes before a RIFF header are UNKNOWN.
func newFileScanner(cur bytescan.Cursor) *bytescan.Scanner {
	lx := bytescan.NewWithCursor(cur)
	lx.MustAddRule(riffHeader, core.Sequence(octet.Record([]byte("RIFF"), 4), octet.Literal([]byte("WAVE"))))
	return lx
}

// newChunkScanner returns the scanner for the chunk headers after the RIFF header
// over cur: a four-character chunk id and the four-byte, little-endian length
// of the payload. A chunk id is a FOURCC, four ASCII characters padded with
// spaces (RIFF, "Chunks"), so a generic id is four printable ASCII bytes:
//
//	[\x20-\x7e]{4}[\x00-\xff]{4}
//
// A header cut by the end of the input does not match; its bytes are UNKNOWN.
//
// FMT_CHUNK_HEADER and DATA_CHUNK_HEADER match the same eight bytes as
// CHUNK_HEADER, so they are registered first and win the tie.
func newChunkScanner(cur bytescan.Cursor) *bytescan.Scanner {
	lx := bytescan.NewWithCursor(cur)
	lx.MustAddRule(fmtChunkHeader, octet.Record([]byte("fmt "), 4))
	lx.MustAddRule(dataChunkHeader, octet.Record([]byte("data"), 4))
	lx.MustAddRule(chunkHeader, core.Sequence(core.Repeat(octet.Range(0x20, 0x7E), 4, 4), octet.Fixed(4)))
	return lx
}
