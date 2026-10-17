package khatru

import (
	"context"
	"fmt"
	"iter"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip42"
	"fiatjaf.com/nostr/nip77/negentropy"
	"fiatjaf.com/nostr/nip77/negentropy/storage/vector"
	"github.com/fasthttp/websocket"
	"github.com/stretchr/testify/require"
)

// The relay handles each message of a connection in its own goroutine. So an
// AUTH changes the authenticated keys while a REQ or a NEG-OPEN of the same
// connection reads them.
func TestAuthWhileReadingAuthedKeys(t *testing.T) {
	relay := NewRelay()
	relay.Negentropy = true
	// three keys and two places: an AUTH appends, replaces in place, or swaps
	relay.MaxAuthenticatedClients = 2

	challenges := make(chan string, 1)
	relay.OnConnect = func(ctx context.Context) {
		challenges <- GetConnection(ctx).Challenge
	}
	read := func(ctx context.Context) {
		for _, pk := range GetAllAuthed(ctx) {
			IsAuthed(ctx, pk)
		}
		GetAuthed(ctx)
	}
	relay.OnRequest = func(ctx context.Context, filter nostr.Filter) (bool, string) {
		read(ctx)
		return false, ""
	}
	relay.QueryStored = func(ctx context.Context, filter nostr.Filter) iter.Seq[nostr.Event] {
		read(ctx)
		return func(yield func(nostr.Event) bool) {}
	}

	server := httptest.NewServer(relay)
	defer server.Close()
	url := "ws" + server.URL[4:]

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	defer conn.Close()

	var challenge string
	select {
	case challenge = <-challenges:
	case <-time.After(5 * time.Second):
		t.Fatal("no connection")
	}

	auths := make([][]byte, 3)
	for i := range auths {
		sk := nostr.Generate()
		event := nip42.CreateUnsignedAuthEvent(challenge, nostr.GetPublicKey(sk), url)
		require.NoError(t, event.Sign(sk))
		auths[i], err = nostr.AuthEnvelope{Event: event}.MarshalJSON()
		require.NoError(t, err)
	}
	vec := vector.New()
	vec.Seal()
	start := negentropy.New(vec, 1024*1024, false, false).Start()

	const rounds = 200
	go func() {
		for range rounds * 3 {
			relay.ListClients()
		}
	}()
	for i := range rounds {
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, auths[i%len(auths)]))
		req := fmt.Sprintf(`["REQ","r%d",{"kinds":[1]}]`, i)
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(req)))
		neg := fmt.Sprintf(`["NEG-OPEN","n%d",{"kinds":[1]},"%s"]`, i, start)
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(neg)))
	}

	// one reply for each message: OK, EOSE and NEG-MSG
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	accepted := 0
	for range rounds * 3 {
		_, msg, err := conn.ReadMessage()
		require.NoError(t, err)
		if !strings.HasPrefix(string(msg), `["OK",`) {
			continue
		}
		var ok nostr.OKEnvelope
		require.NoError(t, ok.FromJSON(string(msg)))
		require.True(t, ok.OK, ok.Reason)
		accepted++
	}
	require.Equal(t, rounds, accepted)
}
