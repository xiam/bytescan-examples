package main

import (
	"bytes"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xiam/bytescan-examples/internal/streamkit"
	"github.com/xiam/bytescan/bytesource"
)

// TestMsgSegments checks that a MSG longer than streamkit.MaxSegment is queued as
// several fields and that the frame ends only after the last of them, as on
// a 32-bit platform with a MSG_LEN above the largest int. The test lowers
// streamkit.MaxSegment so that the boundary is reached in a few bytes.
func TestMsgSegments(t *testing.T) {
	defer func(n uint64) { streamkit.MaxSegment = n }(streamkit.MaxSegment)
	streamkit.MaxSegment = 5

	m := "<1>1 - - - - - - " + strings.Repeat("ab", 11)
	input := strconv.Itoa(len(m)) + " " + m + "<1>1 - - - - - - z\n"
	d := newScanner(bytes.NewReader([]byte(input)))
	var body strings.Builder
	var pieces, frames int
	for {
		l, err := d.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		switch l.Type() {
		case msg:
			pieces++
			if l.Len() > 5 {
				assert.Failf(t, "", "MSG piece of %d bytes, want at most 5", l.Len())
			}
			body.Write(bytesource.Bytes(l.Text()))
		case pri:
			frames++
		}
	}
	{
		got, want := body.String(), strings.Repeat("ab", 11)+"z"
		assert.Equalf(t, want, got, "MSG bytes = %q, want %q", got, want)
	}
	assert.Falsef(t, pieces != 6 || frames != 2, "%d MSG pieces in %d frames, want 6 pieces (5 segments and the second frame's MSG) in 2 frames", pieces, frames)
}
