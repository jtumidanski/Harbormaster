package sse_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/sse"
)

func TestWriterHeadersAndEvents(t *testing.T) {
	rec := httptest.NewRecorder()
	w, err := sse.New(rec)
	require.NoError(t, err)
	require.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
	require.Equal(t, "no", rec.Header().Get("X-Accel-Buffering"))

	require.NoError(t, w.Event("progress", map[string]int{"deleted": 1000}))
	body := rec.Body.String()
	require.True(t, strings.HasPrefix(body, "event: progress\n"))
	require.Contains(t, body, `"deleted":1000`)
	require.True(t, strings.HasSuffix(body, "\n\n"))
}

func TestHeartbeat(t *testing.T) {
	rec := httptest.NewRecorder()
	w, _ := sse.New(rec)
	require.NoError(t, w.Heartbeat())
	require.Contains(t, rec.Body.String(), ": keepalive")
}
