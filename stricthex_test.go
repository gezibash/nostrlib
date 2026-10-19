package nostr

import "testing"

// A key or an id with a byte that is not a hex digit fails to parse. On
// amd64 with AVX2, xhex read such a byte as a digit, so "ZZ" became a key
// that nobody typed.
func TestKeysAndIDsRefuseBytesThatAreNotHex(t *testing.T) {
	good := "451d098c585bccbcdf68e2a7bf3fa542c2a2503d365c1c079ed100b743855310"
	if _, err := PubKeyFromHex(good); err != nil {
		t.Fatalf("a valid key failed: %v", err)
	}
	for _, bad := range []string{"ZZ" + good[2:], good[:31] + "g" + good[32:], good[:63] + "q", "z" + good[1:], good[:40] + "_" + good[41:]} {
		if _, err := PubKeyFromHex(bad); err == nil {
			t.Errorf("PubKeyFromHex(%q) took a byte that is not hex", bad)
		}
		if _, err := IDFromHex(bad); err == nil {
			t.Errorf("IDFromHex(%q) took a byte that is not hex", bad)
		}
	}
}
