// Package sse writes Server-Sent Events with reverse-proxy-friendly headers
// and periodic heartbeats. One Writer per HTTP response.
package sse

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Writer streams SSE frames over an HTTP response.
// Heartbeat must be called periodically by the caller (or via a paired goroutine)
// to defeat proxy buffering.
type Writer struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

// New initializes the response headers and returns a ready Writer.
func New(w http.ResponseWriter) (*Writer, error) {
	f, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("response writer does not support flushing")
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	return &Writer{w: w, flusher: f}, nil
}

// Event writes an `event: <name>\ndata: <json>\n\n` frame and flushes.
func (sw *Writer) Event(name string, data any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(sw.w, "event: %s\ndata: %s\n\n", name, b); err != nil {
		return err
	}
	sw.flusher.Flush()
	return nil
}

// Heartbeat writes a comment-only frame to keep the connection alive.
func (sw *Writer) Heartbeat() error {
	if _, err := fmt.Fprint(sw.w, ": keepalive\n\n"); err != nil {
		return err
	}
	sw.flusher.Flush()
	return nil
}

// StartHeartbeat fires a heartbeat every interval until ctx is done. Run in a
// goroutine alongside the main producer.
func (sw *Writer) StartHeartbeat(stop <-chan struct{}, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			_ = sw.Heartbeat()
		}
	}
}
