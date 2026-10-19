package boltdb

import (
	"encoding/hex"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"fiatjaf.com/nostr"
	"github.com/stretchr/testify/require"
)

// The indexes keep only the first 8 bytes of a pubkey, of a 64-hex tag value
// and of the pubkey in an "a" tag. Two values that agree on these bytes must
// still not match each other's filters.
func TestPrefixCollisions(t *testing.T) {
	// a and b share their first 8 bytes (16 hex characters), then differ
	a := "0123456789abcdef" + strings.Repeat("a", 48)
	b := "0123456789abcdef" + strings.Repeat("b", 48)
	// the store does not check that a pubkey is a point on the curve
	var pkA, pkB nostr.PubKey
	_, err := hex.Decode(pkA[:], []byte(a))
	require.NoError(t, err)
	_, err = hex.Decode(pkB[:], []byte(b))
	require.NoError(t, err)

	addrA := "30023:" + a + ":post"
	addrB := "30023:" + b + ":post"

	other := nostr.MustPubKeyFromHex(strings.Repeat("c", 64))

	var n byte
	event := func(pubkey nostr.PubKey, kind nostr.Kind, tags ...nostr.Tag) nostr.Event {
		n++
		evt := nostr.Event{PubKey: pubkey, Kind: kind, CreatedAt: nostr.Timestamp(1000 + int(n)), Tags: tags, Content: "x"}
		evt.ID[0] = n
		evt.ID[16] = n
		return evt
	}

	wrapA := event(other, 1059, nostr.Tag{"p", a})
	wrapB := event(other, 1059, nostr.Tag{"p", b})
	replyA := event(other, 1, nostr.Tag{"e", a})
	replyB := event(other, 1, nostr.Tag{"e", b})
	noteA := event(pkA, 1)
	noteB := event(pkB, 1)
	deleteA := event(other, 5, nostr.Tag{"a", addrA})
	deleteB := event(other, 5, nostr.Tag{"a", addrB})
	// matches the first two tags of the filter below, but not the third
	threeTags := event(other, 1, nostr.Tag{"e", strings.Repeat("e", 64)}, nostr.Tag{"t", "one"}, nostr.Tag{"x", "other"})

	all := []nostr.Event{threeTags, deleteB, deleteA, noteB, noteA, replyB, replyA, wrapB, wrapA}

	db := &BoltBackend{Path: filepath.Join(t.TempDir(), "bolt")}
	require.NoError(t, db.Init())
	defer db.Close()
	for _, evt := range all {
		require.NoError(t, db.SaveEvent(evt))
	}

	for _, tc := range []struct {
		name   string
		filter nostr.Filter
		want   []nostr.Event
	}{
		{"p tag", nostr.Filter{Tags: nostr.TagMap{"p": {a}}}, []nostr.Event{wrapA}},
		{"p tag and kind", nostr.Filter{Kinds: []nostr.Kind{1059}, Tags: nostr.TagMap{"p": {b}}}, []nostr.Event{wrapB}},
		{"e tag", nostr.Filter{Tags: nostr.TagMap{"e": {a}}}, []nostr.Event{replyA}},
		{"a tag", nostr.Filter{Tags: nostr.TagMap{"a": {addrB}}}, []nostr.Event{deleteB}},
		{"author", nostr.Filter{Authors: []nostr.PubKey{pkA}}, []nostr.Event{noteA}},
		{"author and kind", nostr.Filter{Authors: []nostr.PubKey{pkB}, Kinds: []nostr.Kind{1}}, []nostr.Event{noteB}},
		{"three tags", nostr.Filter{Tags: nostr.TagMap{"e": {strings.Repeat("e", 64)}, "t": {"one"}, "x": {"wanted"}}}, nil},
		{"kind and two other tags", nostr.Filter{Kinds: []nostr.Kind{1}, Tags: nostr.TagMap{"x": {"other"}, "y": {"none"}}}, nil},
		{"kind only", nostr.Filter{Kinds: []nostr.Kind{1059}}, []nostr.Event{wrapB, wrapA}},
		{"id", nostr.Filter{IDs: []nostr.ID{wrapA.ID}}, []nostr.Event{wrapA}},
		{"empty ids list", nostr.Filter{IDs: []nostr.ID{}}, nil},
		// an empty list of authors or kinds selects no index, so it does not narrow the result
		{"empty authors list", nostr.Filter{Authors: []nostr.PubKey{}, Kinds: []nostr.Kind{1059}}, []nostr.Event{wrapB, wrapA}},
		{"empty kinds list", nostr.Filter{Kinds: []nostr.Kind{}}, all},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := slices.Collect(db.QueryEvents(tc.filter, 500))
			require.Equal(t, ids(tc.want), ids(got), "query")

			count, err := db.CountEvents(tc.filter)
			require.NoError(t, err)
			require.Equal(t, uint32(len(tc.want)), count, "count")
		})
	}

	t.Run("id that shares the raw store key", func(t *testing.T) {
		// shares the 8 bytes of the raw store key with wrapA, and nothing else
		id := wrapA.ID
		id[0] = 0xff
		got := slices.Collect(db.QueryEvents(nostr.Filter{IDs: []nostr.ID{id}}, 500))
		require.Empty(t, ids(got))
	})
}

func ids(events []nostr.Event) []nostr.ID {
	out := make([]nostr.ID, len(events))
	for i, evt := range events {
		out[i] = evt.ID
	}
	return out
}

// A replaceable event replaces only an older event of its own author, also
// when another author shares the first 8 bytes of its pubkey.
func TestReplaceKeepsTheEventOfAnotherAuthor(t *testing.T) {
	var pkA, pkB nostr.PubKey
	_, err := hex.Decode(pkA[:], []byte("0123456789abcdef"+strings.Repeat("a", 48)))
	require.NoError(t, err)
	_, err = hex.Decode(pkB[:], []byte("0123456789abcdef"+strings.Repeat("b", 48)))
	require.NoError(t, err)

	newerA := nostr.Event{PubKey: pkA, Kind: 10002, CreatedAt: 2000, Content: "a"}
	newerA.ID[16] = 1
	olderB := nostr.Event{PubKey: pkB, Kind: 10002, CreatedAt: 1000, Content: "b"}
	olderB.ID[16] = 2
	newerB := nostr.Event{PubKey: pkB, Kind: 10002, CreatedAt: 3000, Content: "b2"}
	newerB.ID[16] = 3

	db := &BoltBackend{Path: filepath.Join(t.TempDir(), "bolt")}
	require.NoError(t, db.Init())
	defer db.Close()

	deleted, err := db.ReplaceEvent(newerA)
	require.NoError(t, err)
	require.Empty(t, ids(deleted))

	// a newer event of another author must not keep b's older one out
	deleted, err = db.ReplaceEvent(olderB)
	require.NoError(t, err)
	require.Empty(t, ids(deleted))

	// b's newer event replaces b's older one, and leaves a's alone
	deleted, err = db.ReplaceEvent(newerB)
	require.NoError(t, err)
	require.Equal(t, ids([]nostr.Event{olderB}), ids(deleted))

	got := slices.Collect(db.QueryEvents(nostr.Filter{Kinds: []nostr.Kind{10002}}, 500))
	require.Equal(t, ids([]nostr.Event{newerB, newerA}), ids(got))
}
