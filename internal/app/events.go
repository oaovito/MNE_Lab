package app

import (
	"encoding/json"
	"sync"
)

// Event is pushed to the interface (and, filtered, to paired phones).
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
}

// Hub fans events out to subscribers.
type Hub struct {
	mu   sync.Mutex
	subs map[chan []byte]bool
}

func newHub() *Hub { return &Hub{subs: map[chan []byte]bool{}} }

// Subscribe returns a channel of encoded events and a cancel function.
func (h *Hub) Subscribe() (<-chan []byte, func()) {
	ch := make(chan []byte, 64)
	h.mu.Lock()
	h.subs[ch] = true
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		if h.subs[ch] {
			delete(h.subs, ch)
			close(ch)
		}
		h.mu.Unlock()
	}
}

// Publish sends an event; slow subscribers drop events instead of
// blocking the application.
func (h *Hub) Publish(typ string, data any) {
	b, err := json.Marshal(Event{Type: typ, Data: data})
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- b:
		default:
		}
	}
}

// Close ends every subscription.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		close(ch)
		delete(h.subs, ch)
	}
}
