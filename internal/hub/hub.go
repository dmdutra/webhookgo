// Package hub fans out Redis pub/sub events to local live-stream clients.
//
// A single pattern subscription per process receives the events of every URL,
// so the number of Redis connections does not grow with the number of open
// browser tabs, and any number of app replicas can share one Redis.
package hub

import (
	"context"
	"log/slog"
	"strings"
	"sync"

	"github.com/redis/go-redis/v9"
)

const clientBuffer = 64

type Hub struct {
	prefix string
	mu     sync.Mutex
	subs   map[string]map[chan []byte]struct{}
}

func New(channelPrefix string) *Hub {
	return &Hub{prefix: channelPrefix, subs: make(map[string]map[chan []byte]struct{})}
}

// resync tells clients that events may have been missed (Redis reconnected)
// and they should reload.
var resync = []byte(`{"type":"resync","total":0}`)

// Run dispatches messages from ps until ctx is cancelled, then disconnects
// every client.
func (h *Hub) Run(ctx context.Context, ps *redis.PubSub) {
	defer ps.Close()
	defer h.closeAll()
	msgs := ps.ChannelWithSubscriptions()
	subscribed := false
	for {
		select {
		case <-ctx.Done():
			return
		case m, ok := <-msgs:
			if !ok {
				return
			}
			switch m := m.(type) {
			case *redis.Message:
				h.Broadcast(strings.TrimPrefix(m.Channel, h.prefix), []byte(m.Payload))
			case *redis.Subscription:
				// go-redis resubscribes by itself after a lost connection;
				// anything published meanwhile is gone.
				if subscribed && m.Kind == "psubscribe" {
					slog.Warn("redis pub/sub reconnected, asking clients to resync")
					h.broadcastAll(resync)
				}
				subscribed = true
			}
		}
	}
}

func (h *Hub) broadcastAll(payload []byte) {
	h.mu.Lock()
	ids := make([]string, 0, len(h.subs))
	for id := range h.subs {
		ids = append(ids, id)
	}
	h.mu.Unlock()
	for _, id := range ids {
		h.Broadcast(id, payload)
	}
}

// Subscribe registers a client for a URL. The returned channel is closed when
// the client is dropped (because it is too slow, or the hub stopped); call
// the cancel function when the client goes away.
func (h *Hub) Subscribe(tokenID string) (<-chan []byte, func()) {
	ch := make(chan []byte, clientBuffer)
	h.mu.Lock()
	if h.subs[tokenID] == nil {
		h.subs[tokenID] = make(map[chan []byte]struct{})
	}
	h.subs[tokenID][ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() { h.drop(tokenID, ch) }
}

func (h *Hub) Broadcast(tokenID string, payload []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs[tokenID] {
		select {
		case ch <- payload:
		default:
			// The client cannot keep up: disconnect it, it will reconnect
			// and resynchronise from the API.
			slog.Warn("dropping slow live client", "token", tokenID)
			h.removeLocked(tokenID, ch)
		}
	}
}

// Clients returns the number of connected live clients for a URL.
func (h *Hub) Clients(tokenID string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs[tokenID])
}

func (h *Hub) drop(tokenID string, ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.removeLocked(tokenID, ch)
}

func (h *Hub) removeLocked(tokenID string, ch chan []byte) {
	set := h.subs[tokenID]
	if _, ok := set[ch]; !ok {
		return
	}
	delete(set, ch)
	close(ch)
	if len(set) == 0 {
		delete(h.subs, tokenID)
	}
}

func (h *Hub) closeAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for tokenID, set := range h.subs {
		for ch := range set {
			close(ch)
		}
		delete(h.subs, tokenID)
	}
}
