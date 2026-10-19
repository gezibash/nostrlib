package xhex

import (
	"bytes"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

// Each byte that is not a hex digit fails, in each position of 64 digits:
// the length of a key or an id, which the AVX2 decoder of xhex takes in
// one pass on amd64.
func TestDecodeRefusesEachByteThatIsNotHex(t *testing.T) {
	valid := strings.Repeat("0123456789abcdefABCDEF", 3)[:64]
	for c := range 256 {
		b := byte(c)
		if strings.IndexByte("0123456789abcdefABCDEF", b) >= 0 {
			continue
		}
		for _, at := range []int{0, 31, 32, 63} {
			src := []byte(valid)
			src[at] = b
			var dst [32]byte
			err := Decode(dst[:], src)
			var invalid hex.InvalidByteError
			if !errors.As(err, &invalid) || byte(invalid) != b {
				t.Fatalf("Decode of %q gave %v, want an invalid byte %#U", src, err, rune(b))
			}
		}
	}
}

// Valid digits of each case decode as encoding/hex decodes them.
func TestDecodeMatchesEncodingHex(t *testing.T) {
	for _, s := range []string{strings.Repeat("0123456789abcdef", 4), strings.Repeat("ABCDEF0123456789", 4), "00ff", ""} {
		want, err := hex.DecodeString(s)
		if err != nil {
			t.Fatal(err)
		}
		got := make([]byte, len(s)/2)
		if err := Decode(got, []byte(s)); err != nil || !bytes.Equal(got, want) {
			t.Fatalf("Decode(%q) = %x, %v; want %x", s, got, err, want)
		}
	}
	if err := Decode(make([]byte, 1), []byte("abc")); !errors.Is(err, ErrLength) {
		t.Fatalf("Decode of 3 digits gave %v, want ErrLength", err)
	}
}
