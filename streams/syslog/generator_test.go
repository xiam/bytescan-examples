package main

import (
	"math/rand"
	"strconv"
	"time"
)

var (
	hostnames = []string{"mymachine.example.com", "192.0.2.1", "2001:db8::17", "router-7", "-"}
	appNames  = []string{"su", "sshd", "evntslog", "kernel", "cron", "-"}
	msgIDs    = []string{"ID47", "TCPIN", "AUTH", "-"}
	sdIDs     = []string{"exampleSDID@32473", "origin", "meta", "timeQuality"}
	sdNames   = []string{"iut", "eventSource", "eventID", "ip", "sequenceId", "language"}
	sdValues  = []string{"3", "Application", "1011", "192.0.2.1", `say \"hi\"`, `C:\\temp`, `[a\]`, "ñandú", "日本語", ""}
	words     = []string{"An", "application", "event", "log", "entry", "su", "root", "failed", "for", "lonvick",
		"on", "/dev/pts/8", "café", "☕", "über", "ошибка", "错误", "%%", "'quoted'"}
)

// epoch is the time of record 0 of a generated stream.
var epoch = time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)

// syslogRecord is the streamtest.RecordFunc of an endless syslog stream.
// Record i is one RFC 5424 message at second i after epoch, picked at random:
// octet-counted or newline-ended, with nil values, structured data with
// escaped values, and a MSG that may start with a BOM and holds multi-byte
// UTF-8. A MSG of an octet-counted frame may hold LF. No byte of the stream
// lexes as UNKNOWN.
func syslogRecord(rng *rand.Rand, i int, dst []byte) []byte {
	counted := rng.Intn(2) == 0
	start := len(dst)
	dst = append(dst, '<')
	dst = strconv.AppendInt(dst, int64(rng.Intn(192)), 10)
	dst = append(dst, ">1 "...)
	dst = appendTimestamp(rng, i, dst)
	dst = append(dst, ' ')
	dst = append(dst, pick(rng, hostnames)...)
	dst = append(dst, ' ')
	dst = append(dst, pick(rng, appNames)...)
	dst = append(dst, ' ')
	if rng.Intn(4) == 0 {
		dst = append(dst, '-')
	} else {
		dst = strconv.AppendInt(dst, int64(1+rng.Intn(65535)), 10)
	}
	dst = append(dst, ' ')
	dst = append(dst, pick(rng, msgIDs)...)
	dst = append(dst, ' ')
	dst = appendSD(rng, dst)
	if rng.Intn(5) > 0 {
		dst = append(dst, ' ')
		if rng.Intn(3) == 0 {
			dst = append(dst, "\xef\xbb\xbf"...)
		}
		for n := 1 + rng.Intn(12); n > 0; n-- {
			if n > 1 && counted && rng.Intn(20) == 0 {
				dst = append(dst, '\n')
			}
			dst = append(dst, pick(rng, words)...)
			dst = append(dst, ' ')
		}
		dst = dst[:len(dst)-1]
	}
	if !counted {
		return append(dst, '\n')
	}
	// Prefix the message with its length and SP: shift it right.
	n := len(dst) - start
	prefix := strconv.AppendInt(nil, int64(n), 10)
	prefix = append(prefix, ' ')
	dst = append(dst, prefix...)
	copy(dst[start+len(prefix):], dst[start:start+n])
	copy(dst[start:], prefix)
	return dst
}

// appendTimestamp appends the time of record i in one of the shapes of RFC
// 5424 section 6.2.3, or the nil value.
func appendTimestamp(rng *rand.Rand, i int, dst []byte) []byte {
	ts := epoch.Add(time.Duration(i) * time.Second)
	switch rng.Intn(5) {
	case 0:
		return append(dst, '-')
	case 1:
		return ts.AppendFormat(dst, "2006-01-02T15:04:05Z")
	case 2:
		return ts.Add(time.Duration(rng.Intn(1000))*time.Millisecond).AppendFormat(dst, "2006-01-02T15:04:05.000Z")
	case 3:
		return ts.In(time.FixedZone("", -7*3600)).Add(time.Duration(rng.Intn(1e6))*time.Microsecond).
			AppendFormat(dst, "2006-01-02T15:04:05.000000-07:00")
	default:
		return ts.In(time.FixedZone("", 5*3600+1800)).AppendFormat(dst, "2006-01-02T15:04:05-07:00")
	}
}

// appendSD appends STRUCTURED-DATA: the nil value or one to three elements of
// up to three parameters each.
func appendSD(rng *rand.Rand, dst []byte) []byte {
	if rng.Intn(3) == 0 {
		return append(dst, '-')
	}
	for e := 1 + rng.Intn(3); e > 0; e-- {
		dst = append(dst, '[')
		dst = append(dst, pick(rng, sdIDs)...)
		for p := rng.Intn(4); p > 0; p-- {
			dst = append(dst, ' ')
			dst = append(dst, pick(rng, sdNames)...)
			dst = append(dst, `="`...)
			dst = append(dst, pick(rng, sdValues)...)
			dst = append(dst, '"')
		}
		dst = append(dst, ']')
	}
	return dst
}

func pick(rng *rand.Rand, s []string) string {
	return s[rng.Intn(len(s))]
}
