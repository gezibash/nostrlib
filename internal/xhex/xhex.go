// Package xhex is github.com/templexxx/xhex with a check of each byte. On
// amd64 with AVX2, the decoder of xhex reads any byte as a hex digit, so
// "ZZ" decodes to a value and a key that nobody typed passes. Decode here
// refuses each byte that is not 0-9, a-f or A-F before it decodes.
package xhex

import (
	"encoding/hex"

	upstream "github.com/templexxx/xhex"
)

// ErrLength is the error of an odd number of hex digits.
var ErrLength = upstream.ErrLength

// Encode writes the hex digits of src to dst.
func Encode(dst, src []byte) { upstream.Encode(dst, src) }

// Decode decodes src into len(src)/2 bytes of dst. It returns an
// encoding/hex.InvalidByteError for the first byte that is not a hex digit.
func Decode(dst, src []byte) error {
	for _, c := range src {
		if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F') {
			return hex.InvalidByteError(c)
		}
	}
	return upstream.Decode(dst, src)
}
