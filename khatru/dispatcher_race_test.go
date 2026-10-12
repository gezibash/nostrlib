package khatru

import (
	"sync"
	"testing"

	"fiatjaf.com/nostr"
)

// The relay adds and removes subscriptions under clientsMutex, and it
// notifies listeners without it. So candidates runs at the same time as
// addSubscription and removeSubscription.
func TestDispatcherAddWhileNotifying(t *testing.T) {
	d := newDispatcher()
	author := nostr.MustPubKeyFromHex("87f5650744bed197fcb170ae05fd8d1948a24b2aac34cedf7bdb1c47d6d93273")
	event := nostr.Event{
		PubKey:    author,
		Kind:      1,
		CreatedAt: nostr.Now(),
		Tags:      nostr.Tags{{"t", "x"}},
	}
	filters := []nostr.Filter{
		{Kinds: []nostr.Kind{1}},
		{Authors: []nostr.PubKey{author}},
		{Tags: nostr.TagMap{"t": []string{"x"}}},
		{},
	}

	var clients sync.Mutex // stands in for rl.clientsMutex
	var wg sync.WaitGroup
	done := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
			}
			for range d.candidates(event) {
			}
		}
	}()

	for i := range 2000 {
		clients.Lock()
		ssid := d.addSubscription(subscription{id: "s", filter: filters[i%len(filters)]})
		if i%3 == 0 {
			d.removeSubscription(ssid)
		}
		clients.Unlock()
	}
	close(done)
	wg.Wait()
}
