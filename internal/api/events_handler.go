package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	lastID := parseLastEventID(r.Header.Get("Last-Event-ID"))
	// Send buffered events first for late clients.
	for _, ev := range s.events.SnapshotSince(lastID) {
		if err := writeSSE(w, ev); err != nil {
			return
		}
	}
	flusher.Flush()

	ch, cancel := s.events.Subscribe()
	defer cancel()

	keepAlive := time.NewTicker(15 * time.Second)
	defer keepAlive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if err := writeSSE(w, ev); err != nil {
				return
			}
			flusher.Flush()
		case <-keepAlive.C:
			// SSE comment line as keep-alive.
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func parseLastEventID(v string) int64 {
	if v == "" {
		return 0
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func writeSSE(w http.ResponseWriter, ev Event) error {
	// SSE framing: https://html.spec.whatwg.org/multipage/server-sent-events.html
	if _, err := fmt.Fprintf(w, "id: %d\n", ev.ID); err != nil {
		return err
	}
	if ev.Type != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", ev.Type); err != nil {
			return err
		}
	}
	// Data must be on "data:" lines; our payload is single-line JSON.
	if _, err := fmt.Fprintf(w, "data: %s\n\n", ev.Data); err != nil {
		return err
	}
	return nil
}
