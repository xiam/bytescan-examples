package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xiam/bytescan-examples/internal/hexdump"
	"github.com/xiam/bytescan-examples/internal/lexdump"
	"github.com/xiam/bytescan/bytesource"
	"github.com/xiam/bytescan/streamtest"
)

func TestGolden(t *testing.T) {
	lexdump.Golden(t, hexdump.Dumper, newScanner)
}

// TestFrames checks the samples of the subset: each FRAME_HEADER is followed
// by a FRAME_BODY of the length the helper wrote, the number of frames is the
// number the helper wrote, and the tokens tile the file. In truncated, the
// helper records the length of the last frame up to the cut.
func TestFrames(t *testing.T) {
	for _, name := range []string{"document", "mpeg2", "tag-mismatch", "truncated"} {
		t.Run(name, func(t *testing.T) {
			s := samples[name]
			ls := streamtest.LexAll(t, newScanner(bytes.NewReader(s.b)))
			var frames []int
			sum := 0
			for i, l := range ls {
				if l.Offset() != sum {
					t.Errorf("token %d (%s) at offset %d, want %d", i, l.Type(), l.Offset(), sum)
				}
				sum += l.Len()
				if l.Type() != frameHeader {
					continue
				}
				if i+1 == len(ls) || ls[i+1].Type() != frameBody {
					t.Fatalf("FRAME_HEADER at %d has no FRAME_BODY", l.Offset())
				}
				frames = append(frames, l.Len()+ls[i+1].Len())
			}
			if sum != len(s.b) {
				t.Errorf("token lengths sum to %d, the file has %d bytes", sum, len(s.b))
			}
			if len(frames) != len(s.frames) {
				t.Fatalf("%d frames, the helper wrote %d", len(frames), len(s.frames))
			}
			for i := range frames {
				if frames[i] != s.frames[i] {
					t.Errorf("frame %d is %d bytes, the helper wrote %d", i, frames[i], s.frames[i])
				}
			}
		})
	}
}

// TestTagSkipped checks that the document's tag is read by its synchsafe
// size: its private frame body holds FF FB 14 C4, which stays in one
// ID3_FRAME_BODY, and no FRAME_HEADER starts inside the tag.
func TestTagSkipped(t *testing.T) {
	s := samples["document"]
	tagEnd := 10 + synchsafe(s.b[6:])
	found := false
	for _, l := range streamtest.LexAll(t, newScanner(bytes.NewReader(s.b))) {
		if l.Type() == frameHeader && l.Offset() < tagEnd {
			t.Errorf("FRAME_HEADER at %d, inside the tag that ends at %d", l.Offset(), tagEnd)
		}
		if l.Type() == id3FrameBody && bytes.Contains(bytesource.Bytes(l.Text()), mpeg1At32k[:]) {
			found = true
		}
	}
	if !found {
		t.Error("no ID3_FRAME_BODY holds FF FB 14 C4")
	}
}

// TestReleaseBeforeBody checks rule 1 of the driver: the active scanner is
// released before a body is read from the cursor, so a cursor whose retention
// is bounded to 1 KiB lexes a tag frame of 64 KiB and audio frames of
// 1440 bytes. Without the Release, the scanner's checkpoints would pin the body
// and the cursor would fail.
func TestReleaseBeforeBody(t *testing.T) {
	s := sample{b: tag(4, 0, tagFrame(4, "PRIV", make([]byte, 64<<10)))}
	for i := 0; i < 4; i++ {
		s.frame(mpeg1At320k, nil)
	}
	var got []string
	for _, l := range streamtest.LexRetained(t, bytesource.NewReader(bytes.NewReader(s.b)), 1<<10, newDriver) {
		got = append(got, string(l.Type()))
	}
	want := "ID3_HEADER ID3_FRAME_HEADER ID3_FRAME_BODY" + strings.Repeat(" FRAME_HEADER FRAME_BODY", 4)
	if g := strings.Join(got, " "); g != want {
		t.Errorf("tokens = %s, want %s", g, want)
	}
}

// TestFrameLength checks frameLength against lengths computed by hand.
func TestFrameLength(t *testing.T) {
	for _, tc := range []struct {
		h    [4]byte
		want int
	}{
		{mpeg1At32k, 96},                       // 144 * 32000 / 48000
		{mpeg1At320k, 1440},                    // 144 * 320000 / 32000
		{mpeg2At8k, 26},                        // 72 * 8000 / 22050 = 26.1
		{mpeg2At8kPadded, 27},                  // and the padding byte
		{[4]byte{0xFF, 0xFB, 0x90, 0x64}, 417}, // 128 kbit/s, 44 100 Hz: 417.96
		{[4]byte{0xFF, 0xFA, 0x92, 0x64}, 418}, // with CRC and padding
	} {
		if got := frameLength(tc.h[:]); got != tc.want {
			t.Errorf("frameLength(% x) = %d, want %d", tc.h, got, tc.want)
		}
	}
}
