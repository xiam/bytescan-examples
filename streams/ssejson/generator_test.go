package main

import (
	"math/rand"
	"strconv"
	"strings"
)

// lineEnds are the three line endings of the SSE stream grammar.
var lineEnds = []string{"\n", "\r\n", "\r"}

var (
	eventNames = []string{"message", "chunk", "delta", "done"}
	jsonNames  = []string{"id", "model", "text", "delta", "index", "finish_reason", "usage", "naïve", "名前"}
	words      = []string{"alpha", "beta", "café", "über", "日本語", "😀", "a: b", "{not json}", "[1,2]", "data:"}
	escapes    = []string{`\"`, `\\`, `\/`, `\b`, `\f`, `\n`, `\r`, `\t`, `é`, `😀`}
	layout     = []string{"", "", "", " ", "  ", "\t", " \t "}
)

// jsonTokens appends the tokens of one random JSON document to dst. A token
// is a structural character, a literal, a number, a whole string, or a run of
// whitespace without a line feed. A document is split over data lines only
// between tokens, so no token is cut by the line feed that the pipe adds
// after a data line.
func jsonTokens(rng *rand.Rand, depth int, dst []string) []string {
	ws := func() {
		if s := layout[rng.Intn(len(layout))]; s != "" {
			dst = append(dst, s)
		}
	}
	switch k := rng.Intn(10); {
	case depth < 3 && k < 3:
		dst = append(dst, "{")
		for n := rng.Intn(4); n > 0; n-- {
			ws()
			dst = append(dst, jsonString(rng, jsonNames[rng.Intn(len(jsonNames))]))
			ws()
			dst = append(dst, ":")
			ws()
			dst = jsonTokens(rng, depth+1, dst)
			if n > 1 {
				dst = append(dst, ",")
			}
		}
		ws()
		dst = append(dst, "}")
	case depth < 3 && k < 5:
		dst = append(dst, "[")
		for n := rng.Intn(4); n > 0; n-- {
			ws()
			dst = jsonTokens(rng, depth+1, dst)
			if n > 1 {
				dst = append(dst, ",")
			}
		}
		ws()
		dst = append(dst, "]")
	case k < 7:
		dst = append(dst, jsonString(rng, randomText(rng)))
	case k < 9:
		dst = append(dst, jsonNumberText(rng, 6))
	default:
		dst = append(dst, []string{"true", "false", "null"}[rng.Intn(3)])
	}
	return dst
}

// jsonString returns s as a JSON string with some escapes mixed in. One string
// in 40 is longer than 1000 runes.
func jsonString(rng *rand.Rand, s string) string {
	var b strings.Builder
	b.WriteByte('"')
	b.WriteString(s)
	for n := rng.Intn(3); n > 0; n-- {
		b.WriteString(escapes[rng.Intn(len(escapes))])
		b.WriteString(words[rng.Intn(len(words))])
	}
	if rng.Intn(40) == 0 {
		for b.Len() < 1500+rng.Intn(1500) {
			b.WriteString(words[rng.Intn(len(words))])
			b.WriteByte(' ')
		}
	}
	b.WriteByte('"')
	return b.String()
}

// randomText returns a few words separated by spaces.
func randomText(rng *rand.Rand) string {
	var b strings.Builder
	for n := 1 + rng.Intn(4); n > 0; n-- {
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(words[rng.Intn(len(words))])
	}
	return b.String()
}

// digits returns n random digits, the first one not 0 when lead is set.
func digits(rng *rand.Rand, n int, lead bool) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte('0' + rng.Intn(10))
	}
	if lead && n > 0 && b[0] == '0' {
		b[0] = '1'
	}
	return string(b)
}

// jsonNumberText returns a valid JSON number whose digit runs have 1 to max
// digits.
func jsonNumberText(rng *rand.Rand, max int) string {
	var b strings.Builder
	if rng.Intn(4) == 0 {
		b.WriteByte('-')
	}
	if rng.Intn(5) == 0 {
		b.WriteByte('0')
	} else {
		b.WriteString(digits(rng, 1+rng.Intn(max), true))
	}
	if rng.Intn(2) == 0 {
		b.WriteByte('.')
		b.WriteString(digits(rng, 1+rng.Intn(max), false))
	}
	if rng.Intn(3) == 0 {
		b.WriteString([]string{"e", "E", "e+", "e-", "E-"}[rng.Intn(5)])
		b.WriteString(digits(rng, 1+rng.Intn(max), false))
	}
	return b.String()
}

// eventRecord is the record function of an endless SSE stream whose data
// lines carry JSON documents, the shape of a streaming HTTP API. A record is
// one event ended by a blank line. Its first line is never a data line (an
// event name, an id, or a comment), so a garbage run that joins the first
// line of a record never takes a piece of a document. Most events carry one
// JSON document, split between JSON tokens over one to four data lines, with
// comments and other fields between the data lines; some carry no data, and
// some an empty data line. A data line may be longer than 1000 runes, and a
// string may be too. Every line of one record ends with the same line ending,
// chosen per record, so a bare "\r" is never joined to a "\n".
func eventRecord(rng *rand.Rand, i int, dst []byte) []byte {
	nl := lineEnds[rng.Intn(len(lineEnds))]
	line := func(parts ...string) {
		for _, p := range parts {
			dst = append(dst, p...)
		}
		dst = append(dst, nl...)
	}

	switch rng.Intn(3) {
	case 0:
		line("event: ", eventNames[rng.Intn(len(eventNames))])
	case 1:
		line("id: ", strconv.Itoa(i))
	default:
		line(": ", randomText(rng))
	}

	switch rng.Intn(10) {
	case 0:
		// An event with no data: a retry field or another comment.
		switch rng.Intn(3) {
		case 0:
			line("retry: ", strconv.Itoa(1000*(1+rng.Intn(10))))
		case 1:
			line(": keep-alive ", strconv.Itoa(i))
		default:
			// A long comment: COMMENT pieces that fill the pipe's
			// counters, so that the pipe returns keep-alives.
			line(":", strings.Repeat(randomText(rng)+" ", 300))
		}
	case 1:
		// An empty data value, with and without the colon.
		line([]string{"data", "data:", "data: "}[rng.Intn(3)])
	default:
		tokens := jsonTokens(rng, 0, nil)
		parts := 1 + rng.Intn(4)
		for p := 0; p < parts && len(tokens) > 0; p++ {
			n := len(tokens)
			if p < parts-1 {
				n = 1 + rng.Intn(len(tokens))
			}
			if p > 0 {
				switch rng.Intn(4) {
				case 0:
					line(": between the data lines")
				case 1:
					line("x-trace: ", strconv.FormatInt(rng.Int63(), 36))
				}
			}
			sep := []string{"data: ", "data:"}[rng.Intn(2)]
			line(sep, strings.Join(tokens[:n], ""))
			tokens = tokens[n:]
		}
	}
	line()
	return dst
}

// dataBuffer returns the text that a pipe must return for an SSE stream,
// without keep-alives: for each data line, its value and a line feed. It is
// a separate reading of the WHATWG rules ("Interpreting an event stream"),
// with no scanner: a line ends at "\r\n", "\r" or "\n"; the field name runs to
// the first colon; one space after the colon is dropped.
func dataBuffer(stream string) string {
	var b strings.Builder
	for len(stream) > 0 {
		i := strings.IndexAny(stream, "\r\n")
		if i < 0 {
			i = len(stream)
		}
		line := stream[:i]
		stream = stream[i:]
		switch {
		case strings.HasPrefix(stream, "\r\n"):
			stream = stream[2:]
		case stream != "":
			stream = stream[1:]
		}
		name, val, colon := strings.Cut(line, ":")
		if name != "data" {
			continue
		}
		if colon {
			val = strings.TrimPrefix(val, " ")
		}
		b.WriteString(val)
		b.WriteByte('\n')
	}
	return b.String()
}
