// Package hexdump holds the lexdump.Dumper of the binary and stream examples
// that read bytes through the public bytesource package.
package hexdump

import (
	"github.com/xiam/bytescan-examples/internal/lexdump"
	"github.com/xiam/bytescan/bytesource"
)

// Dumper is the lexdump.Dumper of the binary examples: it writes every token
// in the hex view of bytesource.Hex.
var Dumper = lexdump.Dumper{Format: bytesource.Hex}
