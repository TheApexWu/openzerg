package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/TheApexWu/openzerg/backend/internal/events"
)

// handleSSE pushes events.Event values out as text/event-stream. Clients
// auto-reconnect with Last-Event-ID; the broker honours it to replay any
// events still in the ring beyond that sequence number.
//
// A 10-second ping is sent on idle so intermediaries (and the browser's
// EventSource) don't decide the stream is dead. Every write is checked for
// io errors so we close cleanly when the browser disconnects, avoiding the
// dangling-chunk that surfaces in DevTools as ERR_INCOMPLETE_CHUNKED_ENCODING.
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	headers := w.Header()
	headers.Set("Content-Type", "text/event-stream")
	headers.Set("Cache-Control", "no-cache, no-transform")
	headers.Set("Connection", "keep-alive")
	headers.Set("X-Accel-Buffering", "no")
	// Flush the response headers and an initial comment so the browser
	// sees the stream open before any event fires. Without this the
	// browser holds the request in "pending" until the first publish,
	// and a page navigation in that window triggers
	// ERR_INCOMPLETE_CHUNKED_ENCODING in DevTools.
	w.WriteHeader(http.StatusOK)
	if _, err := io.WriteString(w, ": ok\n\n"); err != nil {
		return
	}
	flusher.Flush()

	sinceSeq := parseLastEventID(r.Header.Get("Last-Event-ID"))
	subscriberChannel, unsubscribe := s.broker.Subscribe(sinceSeq)
	defer unsubscribe()

	// Send a hello so the client has an immediate signal the stream
	// opened. Includes the recent ring snapshot count so the UI can know
	// whether it is mid-run replay.
	helloPayload, _ := json.Marshal(map[string]any{
		"ok":         true,
		"since_seq":  sinceSeq,
		"buffer_len": len(s.broker.Recent(2000)),
	})
	if !writeSSEEvent(w, flusher, events.Event{Type: "hello", Seq: 0, Payload: helloPayload, TS: time.Now().UTC()}) {
		return
	}

	pingTicker := time.NewTicker(10 * time.Second)
	defer pingTicker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case evt, open := <-subscriberChannel:
			if !open {
				return
			}
			if !writeSSEEvent(w, flusher, evt) {
				return
			}
		case <-pingTicker.C:
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// writeSSEEvent writes one SSE record and flushes. Returns false if the
// underlying connection has been closed, so the caller can exit the loop
// instead of repeatedly writing to a dead socket (which is what produces
// ERR_INCOMPLETE_CHUNKED_ENCODING in the browser's DevTools).
func writeSSEEvent(w http.ResponseWriter, flusher http.Flusher, evt events.Event) bool {
	encoded, err := json.Marshal(evt)
	if err != nil {
		return true
	}
	if evt.Seq > 0 {
		if _, err := fmt.Fprintf(w, "id: %d\n", evt.Seq); err != nil {
			return false
		}
	}
	if evt.Type != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", evt.Type); err != nil {
			return false
		}
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", string(encoded)); err != nil {
		return false
	}
	flusher.Flush()
	return true
}

func parseLastEventID(raw string) int64 {
	if raw == "" {
		return 0
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0
	}
	return n
}
