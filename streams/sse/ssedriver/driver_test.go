package ssedriver

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xiam/bytescan"
)

// TestStates checks the state of the driver after each token of a line of
// every form.
func TestStates(t *testing.T) {
	cases := []struct {
		in   string
		want []state
	}{
		{"\uFEFFdata: x\n", []state{stateLine, stateName, stateColon, stateValue, stateValue, stateLine}},
		{"data:x\r\n", []state{stateName, stateColon, stateValue, stateLine}},
		{"id\r", []state{stateName, stateLine}},
		{": c\n\n", []state{stateComment, stateLine, stateLine}},
		{"a:\n", []state{stateName, stateColon, stateLine}},
	}
	for _, c := range cases {
		d := New(strings.NewReader(c.in), bytescan.WithMaxTokenBytes(4096))
		var got []state
		for {
			if _, err := d.Next(); err == io.EOF {
				break
			} else if err != nil {
				require.NoError(t, err)
			}
			got = append(got, d.state)
		}
		if len(got) != len(c.want) {
			assert.Failf(t, "", "%q: states %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				assert.Failf(t, "", "%q: states %v, want %v", c.in, got, c.want)
				break
			}
		}
	}
}
