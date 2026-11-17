package main

import (
	"strings"
	"testing"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan-examples/internal/lexdump"
	"github.com/xiam/bytescan/streamtest"
)

func TestGolden(t *testing.T) {
	lexdump.Golden(t, lexdump.Dumper{}, newScanner)
}

// TestDeepNesting checks that templates nest to any depth: a template inside
// the substitution of a template, 200 levels deep, ends with the stack back at
// the top-level frame and every "}" of a substitution as TEMPLATE_EXPR_CLOSE.
func TestDeepNesting(t *testing.T) {
	const depth = 200
	src := strings.Repeat("`a${ {b: 1}.b + ", depth) + "x" + strings.Repeat(" }c`", depth) + " / 2"
	d := newScanner(strings.NewReader(src))
	count := map[bytescan.TokenType]int{}
	for _, l := range streamtest.LexAll(t, d) {
		count[l.Type()]++
	}
	for _, typ := range []bytescan.TokenType{templateOpen, templateExprOpen, templateExprClose, templateClose} {
		if count[typ] != depth {
			t.Errorf("%s tokens = %d, want %d", typ, count[typ], depth)
		}
	}
	if count[divPunctuator] != 1 || count[bytescan.TokenTypeUnknown] != 0 {
		t.Errorf("DIV_PUNCTUATOR = %d, UNKNOWN = %d, want 1 and 0", count[divPunctuator], count[bytescan.TokenTypeUnknown])
	}
	if len(d.stack) != 1 {
		t.Errorf("stack depth at the end = %d, want 1", len(d.stack))
	}
}

// TestTiling checks that the tokens of every test input, joined, are the
// input: no mode switch loses or repeats text.
func TestTiling(t *testing.T) {
	lexdump.Tiling(t, newScanner)
}

// TestRelease checks rule 1 of the driver: with a switch on almost every
// token, a cursor that retains at most 64 bytes is enough, because the cursor
// keeps only the window of the active scanner, not the input since the last
// token of an idle scanner.
func TestRelease(t *testing.T) {
	src := "x = `t${ {a: 1}.a }` / 2;" + strings.Repeat(" a = b / c / d + /re/g.source;", 200)
	streamtest.LexRetained(t, strings.NewReader(src), 64, newDriver)
}
