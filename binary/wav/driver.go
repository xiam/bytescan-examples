package main

import (
	"encoding/binary"
	"math"
	"sync"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/bytesource"
	"github.com/xiam/bytescan/modes"
)

// driver lexes a WAV file with two scanners, in a mode driver, and a queue of
// fields over one cursor. Its modes:
//
//   - File mode (newFileScanner) lexes the RIFF header. The driver switches to
//     chunk mode after it, and stays there.
//   - Chunk mode (newChunkScanner) lexes the chunk headers. After a chunk header
//     the driver reads the length from it and reads the payload from the
//     cursor as one token of the type of its chunk, so bytes in a payload are
//     never taken for a chunk header. After a payload of odd length it reads
//     the pad byte that follows it as PAD_BYTE.
//
// The size in the RIFF header is not used: chunks are lexed up to the end of
// the input. All numbers are little-endian.
//
// The driver follows the three rules of a mode driver:
//
//  1. The mode driver calls Release on the file scanner at the switch to chunk
//     mode, and the driver calls Release on the chunk scanner before it reads
//     a payload from the cursor, so an idle scanner does not pin the payload.
//  2. It builds the two scanners once, in newDriver, and never drops one.
//  3. Next holds a mutex, so the scanners and the field reads never run at the
//     same time.
//
// Every scanner returns a plain io.EOF at the end of the input. When the input
// ends inside a payload, the payload token holds the bytes up to the end, and
// Next returns io.EOF after it.
type driver struct {
	mu     sync.Mutex
	modes  *modes.Driver
	fields *bytesource.Fields
}

// payloads maps the type of a chunk header to the type of its payload.
var payloads = map[bytescan.TokenType]bytescan.TokenType{
	fmtChunkHeader:  fmtChunkPayload,
	dataChunkHeader: dataChunkPayload,
	chunkHeader:     chunkPayload,
}

// Next returns the next queued field, or else the next token of the active
// scanner, and queues the payload after a chunk header.
func (d *driver) Next() (*bytescan.Token, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if l, err := d.fields.NextField(); l != nil || err != nil {
		return l, err
	}
	l, err := d.modes.Next()
	if err != nil {
		return nil, err
	}
	if typ, ok := payloads[l.Type()]; ok {
		// The length at 4 counts the payload, not the pad byte.
		n := u32(bytesource.Bytes(l.Text())[4:])
		d.modes.Active().Release()
		d.fields.Push(
			bytesource.Field{Type: typ, Len: n},
			bytesource.Field{Type: padByte, Len: n % 2},
		)
	}
	return l, nil
}

// u32 returns a 4-byte length, at most math.MaxInt32 so that it fits an int
// on every platform.
func u32(b []byte) int {
	return int(min(binary.LittleEndian.Uint32(b), math.MaxInt32))
}
