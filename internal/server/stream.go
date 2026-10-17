package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const keepAlive = 20 * time.Second

// stream pushes a URL's events to the browser with Server-Sent Events.
func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	t, ok := s.loadToken(w, r)
	if !ok {
		return
	}
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache, no-transform")
	h.Set("X-Accel-Buffering", "no") // disable proxy buffering (nginx)
	w.WriteHeader(http.StatusOK)

	events, cancel := s.hub.Subscribe(t.ID)
	defer cancel()

	n, _ := s.store.CountRequests(r.Context(), t.ID)
	hello, _ := json.Marshal(event{Type: "connected", Total: n})
	fmt.Fprintf(w, "retry: 3000\ndata: %s\n\n", hello)
	if rc.Flush() != nil {
		return
	}

	ping := time.NewTicker(keepAlive)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.shutdown:
			return
		case msg, ok := <-events:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", msg)
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
		}
		if rc.Flush() != nil {
			return
		}
	}
}
