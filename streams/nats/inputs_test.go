package main

import (
	"strconv"
	"testing"

	"github.com/xiam/bytescan-examples/internal/lextest"
)

// The test inputs are built here, so the testdata/*.input files are
// reproducible byte for byte: they hold CR LF pairs and payload bytes of 0x80
// or above, which a line-ending conversion would change. TestInputs checks
// that each committed input equals the bytes built below, and go test -update
// rewrites the inputs before TestGolden rewrites the golden files.

// inputs maps the name of each test case to the bytes of its input.
var inputs = map[string]func() []byte{
	"session":    session,
	"payloads":   payloads,
	"headers":    headerInputs,
	"long-sizes": longSizes,
	"broken":     broken,
	"truncated":  truncated,
	"empty":      func() []byte { return nil },
}

func TestInputs(t *testing.T) {
	lextest.Inputs(t, inputs)
}

// msg returns a MSG line and its payload.
func msg(subj, sid, reply, body string) string {
	line := "MSG " + subj + " " + sid
	if reply != "" {
		line += " " + reply
	}
	return line + " " + strconv.Itoa(len(body)) + "\r\n" + body + "\r\n"
}

// hmsg returns an HMSG line, its headers and its payload.
func hmsg(subj, sid, reply, hdr, body string) string {
	line := "HMSG " + subj + " " + sid
	if reply != "" {
		line += " " + reply
	}
	return line + " " + strconv.Itoa(len(hdr)) + " " + strconv.Itoa(len(hdr)+len(body)) + "\r\n" + hdr + body + "\r\n"
}

// session is a realistic exchange with every operation, both directions in
// one stream: the server's INFO, the client's CONNECT, subscriptions,
// publications with and without a reply subject and headers, the messages
// the server delivers, and the keep-alive.
func session() []byte {
	return []byte("" +
		`INFO {"server_id":"NCXYZ","version":"2.10.4","proto":1,"headers":true,"max_payload":1048576}` + "\r\n" +
		`CONNECT {"verbose":true,"pedantic":false,"name":"orders","lang":"go","version":"1.31.0","protocol":1,"headers":true}` + "\r\n" +
		"+OK\r\n" +
		"SUB orders.> 1\r\n" +
		"SUB orders.eu workers 2\r\n" +
		"PUB orders.eu.42 11\r\nhello world\r\n" +
		"PUB orders.eu.43 _INBOX.k3Dz.1 4\r\nping\r\n" +
		"HPUB orders.eu.44 _INBOX.k3Dz.2 22 27\r\nNATS/1.0\r\nTrace: on\r\n\r\nhello\r\n" +
		msg("orders.eu.42", "1", "", "hello world") +
		msg("orders.eu.43", "2", "_INBOX.k3Dz.1", "ping") +
		hmsg("orders.eu.44", "1", "_INBOX.k3Dz.2", "NATS/1.0\r\nTrace: on\r\n\r\n", "hello") +
		"PING\r\n" +
		"PONG\r\n" +
		"UNSUB 2 10\r\n" +
		"UNSUB 1\r\n" +
		"-ERR 'Permissions Violation for Publish to orders.us'\r\n")
}

// payloads frames payloads that a rule would misread: one with a CRLF and a
// MSG line in it, one with valid multi-byte UTF-8, one with every byte value,
// a zero-length one, a line with blanks and tabs between its fields, and
// lines with blanks before the CRLF and no reply subject. The
// line after each payload lexes as a control line.
func payloads() []byte {
	all := make([]byte, 256)
	for i := range all {
		all[i] = byte(i)
	}
	return []byte("" +
		msg("a", "1", "", "x\r\nMSG b 2 3\r\nyyy") +
		msg("utf8", "2", "", "héllo, 世界 ✓") +
		"PING\r\n" +
		msg("bytes", "3", "", string(all)) +
		msg("empty", "4", "", "") +
		msg("empty.reply", "5", "_INBOX.9", "") +
		"MSG \t spaced  6 \t 3\r\nabc\r\n" +
		"PUB trailing 5 \r\nhello\r\n" +
		"MSG trailing 7 5\t \r\nhello\r\n" +
		"pong\r\n")
}

// headerInputs frames headers: headers with a payload, headers with no
// payload, a header size above the total, which the total cuts, and blanks
// before the CRLF of a line with no reply subject.
func headerInputs() []byte {
	return []byte("" +
		hmsg("h.both", "1", "", "NATS/1.0\r\nA: 1\r\n\r\n", "body") +
		hmsg("h.only", "2", "_R.1", "NATS/1.0 503\r\n\r\n", "") +
		"HMSG h.cut 3 30 8\r\nNATS/1.0\r\n" +
		"HPUB h.trailing 12 17 \r\nNATS/1.0\r\n\r\nhello\r\n" +
		"PONG\r\n")
}

// longSizes has sizes of more than 10 digits. Each lexes as UNKNOWN, the
// driver reads no payload, and the next line lexes as a control line. The
// 10-digit size is still a size; the input ends inside its payload.
func longSizes() []byte {
	return []byte("" +
		"PUB a 12345678901\r\n" +
		"MSG a 1 _R 00000000000000000003\r\n" +
		"HMSG a 1 99999999999 5\r\n" +
		"HPUB a 3 999999999999\r\n" +
		"PING\r\n" +
		"MSG a 1 9999999999\r\nend")
}

// broken has the near-misses: garbage before an operation, an unknown
// operation, an operation in lower case, a bare LF, a size that is not a
// number, a missing size, an extra field, and an operation with no blank
// after it. None frames a payload; each line after them lexes as a control
// line.
func broken() []byte {
	return []byte("" +
		"#~%PING\r\n" +
		"HELLO there\r\n" +
		"ping\r\n" +
		"PUB a 3\nabc\r\n" +
		"PUB a x3\r\nabc\r\n" +
		"MSG a 1\r\n" +
		"SUB a q 1 extra\r\n" +
		"PUBX a 1\r\nz\r\n" +
		"+OK\r\n")
}

// truncated ends inside a payload: the PAYLOAD token holds the bytes up to
// the end of the input.
func truncated() []byte {
	return []byte("INFO {}\r\nMSG a 1 100\r\nonly part of the payload")
}
