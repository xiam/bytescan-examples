package main

import (
	"testing"

	"github.com/xiam/bytescan-examples/internal/lextest"
)

func TestGolden(t *testing.T) {
	lextest.Golden(t, dumper, newScanner)
}
