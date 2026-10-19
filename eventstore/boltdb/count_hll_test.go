package boltdb

import (
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip45/hyperloglog"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

// CountEventsHLL counts the events that match the full filter and adds the
// pubkey of each one to the hyperloglog. It returns also when the index keeps
// only the first 8 bytes of a value that another value shares.
func TestCountEventsHLL(t *testing.T) {
	// a and b share their first 8 bytes (16 hex characters), then differ
	a := "0123456789abcdef" + strings.Repeat("a", 48)
	b := "0123456789abcdef" + strings.Repeat("b", 48)
	pubkey := func(s string) nostr.PubKey {
		var pk nostr.PubKey
		_, err := hex.Decode(pk[:], []byte(s))
		require.NoError(t, err)
		return pk
	}
	// pkA and pkB also share their first 8 bytes
	pkA, pkB := pubkey(a), pubkey(b)
	pkC, pkD, pkE := pubkey(strings.Repeat("c", 64)), pubkey(strings.Repeat("d", 64)), pubkey(strings.Repeat("e", 64))

	var n byte
	event := func(pk nostr.PubKey, kind nostr.Kind, tags ...nostr.Tag) nostr.Event {
		n++
		evt := nostr.Event{PubKey: pk, Kind: kind, CreatedAt: nostr.Timestamp(1000 + int(n)), Tags: tags, Content: "x"}
		evt.ID[0] = n
		evt.ID[16] = n
		return evt
	}

	// the "e" keys are the first keys of the 32-byte tag index
	followA := event(pkA, 3, nostr.Tag{"p", a})
	followB := event(pkB, 3, nostr.Tag{"p", b})
	reactA := event(pkC, 7, nostr.Tag{"e", a})
	reactB := event(pkD, 7, nostr.Tag{"e", b})
	noteA := event(pkE, 1, nostr.Tag{"p", a})

	open := func(t *testing.T, events ...nostr.Event) *BoltBackend {
		db := &BoltBackend{Path: filepath.Join(t.TempDir(), "bolt")}
		require.NoError(t, db.Init())
		for _, evt := range events {
			require.NoError(t, db.SaveEvent(evt))
		}
		return db
	}
	noCache := func(nostr.Kind) (bool, bool) { return false, false }

	const offset = 8
	check := func(t *testing.T, db *BoltBackend, filter nostr.Filter, want ...nostr.Event) {
		t.Helper()
		type result struct {
			count uint32
			hll   *hyperloglog.HyperLogLog
			err   error
		}
		done := make(chan result, 1)
		go func() {
			count, hll, err := db.CountEventsHLL(filter, offset)
			done <- result{count, hll, err}
		}()

		var res result
		select {
		case res = <-done:
		case <-time.After(5 * time.Second):
			// the read transaction stays open, so the store is not closed
			t.Fatalf("CountEventsHLL(%v) did not return", filter)
		}
		require.NoError(t, res.err)
		require.Equal(t, uint32(len(want)), res.count, "count")

		expected := hyperloglog.New(offset)
		for _, evt := range want {
			expected.Add(evt.PubKey)
		}
		require.Equal(t, expected.GetRegisters(), res.hll.GetRegisters(), "hyperloglog")
		db.Close()
	}

	for _, tc := range []struct {
		name   string
		filter nostr.Filter
		want   []nostr.Event
	}{
		{"p tag and kind", nostr.Filter{Kinds: []nostr.Kind{3}, Tags: nostr.TagMap{"p": {a}}}, []nostr.Event{followA}},
		{"other p tag and kind", nostr.Filter{Kinds: []nostr.Kind{3}, Tags: nostr.TagMap{"p": {b}}}, []nostr.Event{followB}},
		{"e tag and kind", nostr.Filter{Kinds: []nostr.Kind{7}, Tags: nostr.TagMap{"e": {a}}}, []nostr.Event{reactA}},
		{"other e tag and kind", nostr.Filter{Kinds: []nostr.Kind{7}, Tags: nostr.TagMap{"e": {b}}}, []nostr.Event{reactB}},
		// the pubkey-kind index, with no extra kinds or tags to check
		{"author and kind", nostr.Filter{Kinds: []nostr.Kind{3}, Authors: []nostr.PubKey{pkB}}, []nostr.Event{followB}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := open(t, followA, followB, reactA, reactB, noteA)
			db.EnableHLLCacheFor = noCache
			check(t, db, tc.filter, tc.want...)
		})
	}

	t.Run("index entry with no event", func(t *testing.T) {
		lost := event(pkE, 3, nostr.Tag{"p", a})
		db := open(t, followA, followB, lost)
		db.EnableHLLCacheFor = noCache
		require.NoError(t, db.DB.Update(func(txn *bbolt.Tx) error {
			return txn.Bucket(rawEventStore).Delete(lost.ID[16:24])
		}))
		check(t, db, nostr.Filter{Kinds: []nostr.Kind{3}, Tags: nostr.TagMap{"p": {a}}}, followA)
	})

	// a store with no EnableHLLCacheFor, and a filter with no kinds
	t.Run("default store", func(t *testing.T) {
		db := open(t, followA, followB, reactA, reactB, noteA)
		check(t, db, nostr.Filter{Tags: nostr.TagMap{"p": {a}}}, followA, noteA)
	})
}
