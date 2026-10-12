package nostr

import (
	"io"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/websocket"
)

// Events are dispatched in their own goroutines. When the subscription ends,
// sub.Events is closed while some of those goroutines may still be sending.
// Under -race this reports a data race between the send and the close, and
// without -race it can panic with "send on closed channel".
func TestSubscriptionCloseWhileDispatching(t *testing.T) {
	ws := newWebsocketServer(func(conn *websocket.Conn) {
		io.Copy(io.Discard, conn)
	})
	defer ws.Close()

	rl := mustRelayConnect(t, ws.URL)
	defer rl.Close()

	for range 200 {
		sub, err := rl.Subscribe(t.Context(), Filter{}, SubscriptionOptions{MaxWaitForEOSE: time.Duration(math.MaxInt64)})
		require.NoError(t, err)

		for range 8 {
			sub.dispatchEvent(Event{})
		}
		sub.Unsub()

		for range sub.Events {
		}
	}
}
