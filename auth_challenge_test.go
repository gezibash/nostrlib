//go:build !js

package nostr

import (
	"context"
	stdjson "encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/websocket"
)

// A relay sends a new challenge with each "auth-required" refusal, so a new
// challenge can arrive while Auth waits for the OK of its AUTH event. Auth
// must then finish, and it must not corrupt its own state.
func TestAuthWhileANewChallengeArrives(t *testing.T) {
	ws := newWebsocketServer(func(conn *websocket.Conn) {
		require.NoError(t, websocket.JSON.Send(conn, []any{"AUTH", "first"}))

		var raw []stdjson.RawMessage
		require.NoError(t, websocket.JSON.Receive(conn, &raw))
		var label string
		require.NoError(t, stdjson.Unmarshal(raw[0], &label))
		require.Equal(t, "AUTH", label)
		var auth Event
		require.NoError(t, stdjson.Unmarshal(raw[1], &auth))
		require.Equal(t, "first", auth.Tags.Find("challenge")[1])

		// The client reads messages in order: it gets the new challenge
		// before the OK that ends its authentication.
		require.NoError(t, websocket.JSON.Send(conn, []any{"AUTH", "second"}))
		require.NoError(t, websocket.JSON.Send(conn, []any{"OK", auth.ID.Hex(), true, ""}))

		// Keep the connection open until the client closes it.
		_ = websocket.JSON.Receive(conn, &raw)
	})
	defer ws.Close()

	rl := mustRelayConnect(t, ws.URL)
	defer rl.Close()

	priv := Generate()
	sign := func(ctx context.Context, evt *Event) error { return evt.Sign(priv) }
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	var err error
	for {
		if err = rl.Auth(ctx, sign); err == nil || !strings.Contains(err.Error(), "no challenge") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.NoError(t, err)
}
