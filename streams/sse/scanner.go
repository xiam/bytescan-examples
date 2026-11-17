package main

import (
	"io"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan-examples/streams/sse/ssedriver"
)

// The token types, defined in ssedriver (see ../README.md, "Tokens").
const (
	bom      = ssedriver.BOM
	field    = ssedriver.Field
	event    = ssedriver.Event
	data     = ssedriver.Data
	id       = ssedriver.ID
	retry    = ssedriver.Retry
	colon    = ssedriver.Colon
	space    = ssedriver.Space
	value    = ssedriver.Value
	comment  = ssedriver.Comment
	eol      = ssedriver.EOL
	eventEnd = ssedriver.EventEnd
)

// maxRun is the most runes in one FIELD, VALUE or COMMENT token.
const maxRun = ssedriver.MaxRun

// maxTokenBytes bounds every token of every mode scanner. The largest token is
// a piece of maxRun runes of at most 4 bytes each, 4,000 bytes, so the bound
// is never reached; a rule that scanned further before it decides would fail
// Next with the limit error instead of holding the run in memory.
const maxTokenBytes = 4096

// newScanner returns a Server-Sent Events scanner that reads r: the mode driver of
// ssedriver over one cursor, with every mode scanner bounded by maxTokenBytes.
func newScanner(r io.Reader) *ssedriver.Driver {
	return ssedriver.New(r, bytescan.WithMaxTokenBytes(maxTokenBytes))
}
