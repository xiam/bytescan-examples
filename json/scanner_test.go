package main

import (
	"testing"

	"github.com/xiam/bytescan-examples/internal/lexdump"
)

func TestGolden(t *testing.T) {
	lexdump.Golden(t, lexdump.Dumper{}, newScanner)
}
