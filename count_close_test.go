package nostr

import (
	"context"
	stdjson "encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/websocket"
)

// A relay can answer one COUNT more than once. Count() takes the first answer
// and ends the subscription, which closes sub.countResult, while the read loop
// may still be sending a later answer on it. Under -race this reports a data
// race, and when the buffer is full the send can panic with "send on closed
// channel" or block the read loop, so later calls on the connection time out.
func TestCountResultCloseWhileReceiving(t *testing.T) {
	ws := newWebsocketServer(func(conn *websocket.Conn) {
		for {
			var raw []stdjson.RawMessage
			if err := websocket.JSON.Receive(conn, &raw); err != nil {
				return
			}
			var typ, subid string
			if len(raw) < 2 || stdjson.Unmarshal(raw[0], &typ) != nil || typ != "COUNT" {
				continue
			}
			stdjson.Unmarshal(raw[1], &subid)
			for range 3 {
				websocket.JSON.Send(conn, []any{"COUNT", subid, map[string]any{"count": 7}})
			}
		}
	})
	defer ws.Close()

	rl := mustRelayConnect(t, ws.URL)
	defer rl.Close()

	done := make(chan error, 1)
	go func() {
		for range 20 {
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			count, _, err := rl.Count(ctx, Filter{}, SubscriptionOptions{})
			cancel()
			if err == nil && count != 7 {
				err = fmt.Errorf("count %d, want 7", count)
			}
			if err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("Count() did not return: the read loop is blocked on sub.countResult")
	}
}
