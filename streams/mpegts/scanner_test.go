package main

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/require"
	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan-examples/internal/guardtest"
	"github.com/xiam/bytescan-examples/internal/hexdump"
	"github.com/xiam/bytescan-examples/internal/lextest"
	"github.com/xiam/bytescan/streamtest"
)

func TestGolden(t *testing.T) {
	lextest.Golden(t, hexdump.Dumper, newScanner)
}

// TestGuard checks the production guard: it is streamtest.ByteGuard;
// a stream with no sync byte, which never starts a packet, keeps producing
// UNKNOWN tokens of one byte, with no guard error and bounded memory; and a
// candidate longer than the guard fails Next with the guard error.
func TestGuard(t *testing.T) {
	require.Equalf(t, streamtest.ByteGuard, tokenGuard, "tokenGuard = %d, want streamtest.ByteGuard (%d)", tokenGuard, streamtest.ByteGuard)
	guardtest.Bounded(t, newScanner, streamtest.NewGenerator(80, guardtest.Filler(syncByte)), 1, bytescan.TokenTypeUnknown)

	// tokenGuard+1 bytes of 0xFF exceed the guard, which counts bytes, by one.
	d := newScanner(bytes.NewReader(bytes.Repeat([]byte{0xFF}, tokenGuard+1)))
	guardtest.Active(t, d.sync, tokenGuard, d.Next)
}

// TestReadError checks the Source contract when the reader fails right
// after a header that announces an adaptation field, where the driver peeks
// at the length byte: every Next returns a token or an error, never both,
// and the reader's error comes out as it is.
func TestReadError(t *testing.T) {
	errRead := errors.New("read failed")
	r := io.MultiReader(bytes.NewReader([]byte{syncByte, 0x00, 0x00, 0x20}), iotest.ErrReader(errRead))
	d := newScanner(r)
	for {
		l, err := d.Next()
		if err != nil {
			if l != nil {
				require.FailNowf(t, "", "Next returned %s and the error %v", l.Type(), err)
			}
			require.ErrorIsf(t, err, errRead, "Next: %v, want the reader's error", err)
			return
		}
		require.NotNil(t, l, "Next returned no token and no error")
	}
}
