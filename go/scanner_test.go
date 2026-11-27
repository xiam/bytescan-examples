package main

import (
	"fmt"
	"go/scanner"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan-examples/internal/lextest"
	"github.com/xiam/bytescan/streamtest"
)

func TestGolden(t *testing.T) {
	lextest.Golden(t, dumper, func(r io.Reader) *semicolons {
		return newSemicolons(newScanner(r))
	})
}

// tokenTypes maps each token of go/scanner to the token type of the example.
// Keywords and operators are added by init, from go/token.
var tokenTypes = map[token.Token]bytescan.TokenType{
	token.IDENT:     identifier,
	token.INT:       intLit,
	token.FLOAT:     floatLit,
	token.IMAG:      imaginaryLit,
	token.CHAR:      runeLit,
	token.STRING:    stringLit,
	token.SEMICOLON: semicolon,
}

func init() {
	for tok := token.ILLEGAL; tok <= token.TILDE; tok++ {
		switch {
		case tok.IsKeyword():
			tokenTypes[tok] = keyword
		case tok.IsOperator() && tok != token.SEMICOLON:
			tokenTypes[tok] = operator
		}
	}
}

// item is one token of a stream, in a form both streams share. An inserted
// semicolon has the text "\n" in both: go/scanner gives it that literal.
type item struct {
	typ    bytescan.TokenType
	text   string
	offset int // byte offset; -1 for an inserted semicolon
}

func (i item) String() string {
	return fmt.Sprintf("%d %s %q", i.offset, i.typ, i.text)
}

// exampleStream returns the tokens of src from the example: the filter's
// output without comments and layout.
func exampleStream(t *testing.T, src []byte) []item {
	t.Helper()
	f := bytescan.Skip(newSemicolons(newScanner(strings.NewReader(string(src)))), comment, whitespace, newline)
	var items []item
	for _, l := range streamtest.LexAll(t, f) {
		if isInserted(l) {
			items = append(items, item{semicolon, "\n", -1})
		} else {
			items = append(items, item{l.Type(), l.Text(), l.ByteOffset()})
		}
	}
	return items
}

// scannerStream returns the tokens of src from go/scanner, with comments
// skipped. It fails the test on a scan error: the differential inputs are
// valid Go.
func scannerStream(t *testing.T, name string, src []byte) []item {
	t.Helper()
	fset := token.NewFileSet()
	file := fset.AddFile(name, -1, len(src))
	var s scanner.Scanner
	s.Init(file, src, func(pos token.Position, msg string) { assert.Failf(t, "", "%s: %s", pos, msg) }, 0)
	var items []item
	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			return items
		}
		typ, ok := tokenTypes[tok]
		if !ok {
			require.FailNowf(t, "", "%s: no token type for %s", fset.Position(pos), tok)
		}
		if lit == "" {
			lit = tok.String()
		}
		offset := file.Offset(pos)
		if tok == token.SEMICOLON && lit == "\n" {
			offset = -1
		}
		items = append(items, item{typ, lit, offset})
	}
}

// goInputs returns the differential inputs: testdata/*.go.input.
func goInputs(t *testing.T) []string {
	t.Helper()
	inputs, err := filepath.Glob(filepath.Join("testdata", "*.go.input"))
	require.NoError(t, err)
	if len(inputs) < 3 {
		require.FailNowf(t, "", "found %d testdata/*.go.input files, want at least 3", len(inputs))
	}
	return inputs
}

// TestScanner checks the example against go/scanner: for each Go input, both
// give the same tokens, with the same text at the same byte offset, and the
// same inserted semicolons.
func TestScanner(t *testing.T) {
	for _, input := range goInputs(t) {
		t.Run(filepath.Base(input), func(t *testing.T) {
			src, err := os.ReadFile(input)
			require.NoError(t, err)
			got, want := exampleStream(t, src), scannerStream(t, input, src)
			for i := 0; i < len(got) || i < len(want); i++ {
				switch {
				case i >= len(got):
					require.FailNowf(t, "", "token %d: missing, want %v", i, want[i])
				case i >= len(want):
					require.FailNowf(t, "", "token %d: got %v, want the end of the stream", i, got[i])
				case got[i] != want[i]:
					require.FailNowf(t, "", "token %d: got %v, want %v", i, got[i], want[i])
				}
			}
		})
	}
}

// literalForms are the literal forms the Go inputs must contain, each as a
// test on the token text.
var literalForms = []struct {
	name string
	tok  token.Token
	is   func(lit string) bool
}{
	{"decimal integer", token.INT, func(s string) bool { return s[0] != '0' && !strings.Contains(s, "_") }},
	{"integer with _", token.INT, func(s string) bool { return s[0] != '0' && strings.Contains(s, "_") }},
	{"binary integer", token.INT, hasPrefix("0b", "0B")},
	{"octal integer, 0o", token.INT, hasPrefix("0o", "0O")},
	{"octal integer, 0", token.INT, func(s string) bool { return len(s) > 1 && s[0] == '0' && s[1] >= '0' && s[1] <= '9' }},
	{"hex integer", token.INT, hasPrefix("0x", "0X")},
	{"decimal float", token.FLOAT, func(s string) bool { return !hasPrefix("0x", "0X")(s) }},
	{"decimal float with exponent", token.FLOAT, func(s string) bool { return strings.ContainsAny(s, "eE") && !hasPrefix("0x", "0X")(s) }},
	{"hex float", token.FLOAT, hasPrefix("0x", "0X")},
	{"imaginary", token.IMAG, func(string) bool { return true }},
	{"rune", token.CHAR, func(s string) bool { return !strings.Contains(s, `\`) }},
	{"rune with escape", token.CHAR, func(s string) bool { return strings.Contains(s, `\`) }},
	{"interpreted string", token.STRING, hasPrefix(`"`)},
	{"raw string", token.STRING, hasPrefix("`")},
	{"raw string over lines", token.STRING, func(s string) bool { return s[0] == '`' && strings.Contains(s, "\n") }},
}

func hasPrefix(prefixes ...string) func(string) bool {
	return func(s string) bool {
		for _, p := range prefixes {
			if strings.HasPrefix(s, p) {
				return true
			}
		}
		return false
	}
}

// TestScannerCoverage checks that the Go inputs together contain every
// keyword, every operator and each literal form, so that TestScanner covers
// them.
func TestScannerCoverage(t *testing.T) {
	seen := map[token.Token]bool{}
	forms := map[string]bool{}
	for _, input := range goInputs(t) {
		src, err := os.ReadFile(input)
		require.NoError(t, err)
		for _, it := range scannerStream(t, input, src) {
			for tok, typ := range tokenTypes {
				if typ == it.typ && (typ == keyword || typ == operator) && tok.String() == it.text {
					seen[tok] = true
				}
			}
			if it.typ == semicolon && it.offset >= 0 {
				seen[token.SEMICOLON] = true
			}
			for _, f := range literalForms {
				if tokenTypes[f.tok] == it.typ && f.is(it.text) {
					forms[f.name] = true
				}
			}
		}
	}
	for tok, typ := range tokenTypes {
		assert.Falsef(t, (typ == keyword || typ == operator || tok == token.SEMICOLON) && !seen[tok], "no Go input contains %s %q", typ, tok)
	}
	for _, f := range literalForms {
		assert.Truef(t, forms[f.name], "no Go input contains a %s", f.name)
	}
}
