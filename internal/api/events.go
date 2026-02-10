package api

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"
)

type Event struct {
	ID   int64
	Type string
	At   time.Time
	Data []byte // JSON payload
}

// EventHub is an in-memory pub/sub with a small ring buffer for late clients.
type EventHub struct {
	nextID atomic.Int64

	mu    sync.Mutex
	ring  []Event
	start int
	size  int

	subs      map[int]chan Event
	nextSubID int
}

func NewEventHub(capacity int) *EventHub {
	if capacity <= 0 {
		capacity = 1
	}
	return &EventHub{
		ring: make([]Event, capacity),
		subs: make(map[int]chan Event),
	}
}

func (h *EventHub) Publish(eventType string, data any) {
	id := h.nextID.Add(1)

	payload := []byte("{}")
	if data != nil {
		if b, err := json.Marshal(data); err == nil {
			payload = b
		}
	}

	ev := Event{
		ID:   id,
		Type: eventType,
		At:   time.Now().UTC(),
		Data: payload,
	}

	h.mu.Lock()
	h.pushLocked(ev)
	for _, ch := range h.subs {
		// Don't let slow clients block producers.
		select {
		case ch <- ev:
		default:
		}
	}
	h.mu.Unlock()
}

func (h *EventHub) Subscribe() (<-chan Event, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()

	id := h.nextSubID
	h.nextSubID++
	ch := make(chan Event, 32)
	h.subs[id] = ch

	cancel := func() {
		h.mu.Lock()
		if c, ok := h.subs[id]; ok {
			delete(h.subs, id)
			close(c)
		}
		h.mu.Unlock()
	}

	return ch, cancel
}

// SnapshotSince returns buffered events with ID > lastID, oldest-first.
// If lastID is 0, the full ring buffer snapshot is returned.
func (h *EventHub) SnapshotSince(lastID int64) []Event {
	h.mu.Lock()
	defer h.mu.Unlock()

	out := make([]Event, 0, h.size)
	for i := 0; i < h.size; i++ {
		ev := h.ring[(h.start+i)%len(h.ring)]
		if lastID == 0 || ev.ID > lastID {
			out = append(out, ev)
		}
	}
	return out
}

func (h *EventHub) pushLocked(ev Event) {
	capacity := len(h.ring)
	if capacity == 0 {
		return
	}

	if h.size < capacity {
		idx := (h.start + h.size) % capacity
		h.ring[idx] = ev
		h.size++
		return
	}

	// Overwrite oldest.
	h.ring[h.start] = ev
	h.start = (h.start + 1) % capacity
}
