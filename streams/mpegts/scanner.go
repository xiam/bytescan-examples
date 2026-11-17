package main

import (
	"io"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan/bytesource"
	"github.com/xiam/bytescan/rules/core"
	"github.com/xiam/bytescan/rules/octet"
)

// The token types. The scanners emit SYNC and HEADER; the driver reads the
// adaptation field and the payload from the cursor.
const (
	syncType        bytescan.TokenType = "SYNC"
	headerType      bytescan.TokenType = "HEADER"
	adaptationField bytescan.TokenType = "ADAPTATION_FIELD"
	payload         bytescan.TokenType = "PAYLOAD"
)

// The packet layout of ISO/IEC 13818-1, section 2.4.3.2: every packet is 188
// bytes and starts with the sync byte 0x47, followed by 3 bytes of header.
const (
	packetSize = 188
	syncByte   = 0x47
	headerSize = 3
	// bodySize is the room for the adaptation field and the payload.
	bodySize = packetSize - 1 - headerSize
)

// tokenGuard is the per-token byte bound of every scanner: twice
// streamtest.ByteGuard, because the packets hold bytes of 0x80 and above,
// which the guard counts as 2 bytes. The longest read of a rule is the
// confirmed sync of newLostScanner, one packet and a byte (189 bytes, at most
// 378 guard bytes), so the guard never fires on any input.
const tokenGuard = 2048

// newScanner returns an MPEG transport stream scanner that reads r: a mode driver
// over one byte cursor, with a scanner for the sync byte, one for the header,
// one for the search of a sync byte after lost sync, and a queue of the
// adaptation field and payload that the header announces. Every scanner has
// the per-token guard.
func newScanner(r io.Reader) *driver {
	return newDriver(bytesource.NewCursor(r), bytescan.WithMaxTokenBytes(tokenGuard))
}

// newSyncScanner returns the scanner for the first byte of a packet over cur: the
// sync byte 0x47 (ISO/IEC 13818-1, section 2.4.3.3, sync_byte). Any other
// byte is UNKNOWN.
//
// It has one rule, so no tie arises.
func newSyncScanner(cur bytescan.Cursor, opts ...bytescan.Option) *bytescan.Scanner {
	lx := bytescan.NewWithCursorOptions(cur, opts...)
	lx.MustAddRule(syncType, octet.Set(syncByte))
	return lx
}

// newLostScanner returns the scanner of the search for a sync byte after the
// driver has lost sync, over cur. A 0x47 is SYNC only when the byte one
// packet after it is 0x47 too:
//
//	\x47(?=[\x00-\xff]{187}\x47)
//
// Any other byte, and a 0x47 that the input ends before confirming, is
// UNKNOWN, one byte at a time. It has one rule, so no tie arises.
func newLostScanner(cur bytescan.Cursor, opts ...bytescan.Option) *bytescan.Scanner {
	sync := octet.Set(syncByte)
	lx := bytescan.NewWithCursorOptions(cur, opts...)
	lx.MustAddRule(syncType, core.FollowedBy(sync, core.Sequence(octet.Fixed(packetSize-1), sync)))
	return lx
}

// newHeaderScanner returns the scanner for the 3 header bytes after the sync byte
// over cur (ISO/IEC 13818-1, section 2.4.3.2): transport_error_indicator,
// payload_unit_start_indicator, transport_priority, the 13-bit PID,
// transport_scrambling_control, adaptation_field_control and
// continuity_counter. Every value of these bits is lexed:
//
//	[\x00-\xff]{3}
//
// A header cut by the end of the input does not match; its bytes are UNKNOWN.
// It has one rule, so no tie arises.
func newHeaderScanner(cur bytescan.Cursor, opts ...bytescan.Option) *bytescan.Scanner {
	lx := bytescan.NewWithCursorOptions(cur, opts...)
	lx.MustAddRule(headerType, octet.Fixed(headerSize))
	return lx
}
