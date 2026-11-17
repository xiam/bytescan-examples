package main

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"testing/iotest"

	"github.com/xiam/bytescan"
	"github.com/xiam/bytescan-examples/internal/guardtest"
	"github.com/xiam/bytescan-examples/internal/hexdump"
	"github.com/xiam/bytescan-examples/internal/lexdump"
	"github.com/xiam/bytescan/streamtest"
)

func TestGolden(t *testing.T) {
	lexdump.Golden(t, hexdump.Dumper, newScanner)
}

// TestGuard checks the production guard: it is twice streamtest.ByteGuard;
// a stream with no sync byte, which never starts a packet, keeps producing
// UNKNOWN tokens of one byte, with no guard error and bounded memory; and a
// candidate longer than the guard fails Next with the guard error.
func TestGuard(t *testing.T) {
	if tokenGuard != 2*streamtest.ByteGuard {
		t.Fatalf("tokenGuard = %d, want 2*streamtest.ByteGuard (%d)", tokenGuard, 2*streamtest.ByteGuard)
	}
	guardtest.Bounded(t, newScanner, streamtest.NewGenerator(80, guardtest.Filler(syncByte)), 1, bytescan.TokenTypeUnknown)

	d := newScanner(bytes.NewReader(bytes.Repeat([]byte{0xFF}, tokenGuard)))
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
				t.Fatalf("Next returned %s and the error %v", l.Type(), err)
			}
			if !errors.Is(err, errRead) {
				t.Fatalf("Next: %v, want the reader's error", err)
			}
			return
		}
		if l == nil {
			t.Fatal("Next returned no token and no error")
		}
	}
}
