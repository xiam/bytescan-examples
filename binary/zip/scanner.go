package main

import (
	"io"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/bytesource"
	"github.com/xiam/bytescan/modes"
	"github.com/xiam/bytescan/rules/octet"
)

// The token types. The record scanner emits the three record types; the driver
// reads the variable-length fields that follow each record.
const (
	localFileHeader        bytescan.TokenType = "LOCAL_FILE_HEADER"
	centralDirectoryHeader bytescan.TokenType = "CENTRAL_DIRECTORY_HEADER"
	endOfCentralDirectory  bytescan.TokenType = "END_OF_CENTRAL_DIRECTORY"
	fileName               bytescan.TokenType = "FILE_NAME"
	extraField             bytescan.TokenType = "EXTRA_FIELD"
	fileData               bytescan.TokenType = "FILE_DATA"
	fileComment            bytescan.TokenType = "FILE_COMMENT"
	zipComment             bytescan.TokenType = "ZIP_COMMENT"
)

// newScanner returns a ZIP scanner that reads r: a driver over one byte cursor,
// with a mode driver that holds the record scanner, and a queue of the fields
// that the records announce.
func newScanner(r io.Reader) *driver {
	return newDriver(bytesource.NewCursor(r))
}

// newDriver returns the driver over cur.
func newDriver(cur bytescan.Cursor) *driver {
	return &driver{
		modes:  modes.New(cur, newRecordScanner(cur)),
		fields: bytesource.NewFields(cur),
	}
}

// newRecordScanner returns the scanner for the fixed part of the three ZIP
// records of PKWARE APPNOTE 6.3.10 over cur: the 4-byte signature and the n
// fixed bytes after it, as one token (sections 4.3.7, 4.3.12 and 4.3.16):
//
//	signature[\x00-\xff]{n}
//
// A record cut by the end of the input does not match; its bytes are UNKNOWN.
//
// No two rules match the same text: each one starts with its own signature.
func newRecordScanner(cur bytescan.Cursor) *bytescan.Scanner {
	lx := bytescan.NewWithCursor(cur)
	lx.MustAddRule(localFileHeader, octet.Record([]byte("PK\x03\x04"), 26))
	lx.MustAddRule(centralDirectoryHeader, octet.Record([]byte("PK\x01\x02"), 42))
	lx.MustAddRule(endOfCentralDirectory, octet.Record([]byte("PK\x05\x06"), 18))
	return lx
}
