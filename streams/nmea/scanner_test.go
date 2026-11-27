package main

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xiam/bytescan-examples/internal/lexdump"
	"github.com/xiam/bytescan-examples/internal/lextest"
	"github.com/xiam/bytescan/streamtest"
)

func TestGolden(t *testing.T) {
	lextest.Golden(t, lexdump.Dumper{}, newScanner)
}

// TestTokenGuard checks that the scanner's per-token guard is the harness value
// for a UTF-8 cursor.
func TestTokenGuard(t *testing.T) {
	require.Equalf(t, streamtest.TextGuard, tokenGuard, "tokenGuard is %d, want streamtest.TextGuard (%d)", tokenGuard, streamtest.TextGuard)
}
