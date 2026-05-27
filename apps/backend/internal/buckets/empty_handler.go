// Package buckets — empty-bucket SSE handler.
//
// EmptyHandler streams progress for an in-flight bulk-delete operation as a
// Server-Sent Events feed. The handler is intentionally a thin wrapper around
// the bucketempty.Service: validation gates run first (and surface as plain
// JSON action-envelope errors so the SPA can read `error.code` directly),
// then a 200 OK with the SSE headers is committed and progress events are
// pumped until the worker terminates.
//
// Wire-protocol notes (see api-contracts.md §POST /buckets/{name}/empty):
//   - `event: progress` frames carry {deleted, estimated_total} per batch.
//   - `event: done` is the terminal success frame with {deleted_total, duration_ms}.
//   - `event: error` is the terminal failure frame with {message}.
//   - A `: keepalive` comment is written every 15s to defeat proxy buffering.
//
// The handler is decoupled from `*bucketempty.Service` via the EmptyService
// interface so unit tests can supply a deterministic fake without spinning up
// the full single-flight worker (which requires a GORM DB + live MinIO pool).
package buckets

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
	"github.com/jtumidanski/Harbormaster/internal/jobs/bucketempty"
	"github.com/jtumidanski/Harbormaster/internal/sse"
)

// EmptyService is the subset of bucketempty.Service the handler depends on.
// Defining it here keeps the handler testable in isolation: production wiring
// passes a real `*bucketempty.Service`, tests pass a deterministic fake.
type EmptyService interface {
	StartOrAttach(ctx context.Context, bucket string, purgeVersions bool) (<-chan bucketempty.Progress, <-chan bucketempty.Result, error)
}

// EmptyHandler is the SSE handler for POST /api/v1/buckets/{name}/empty.
// Construct with the live bucketempty.Service in production wiring.
type EmptyHandler struct {
	Service EmptyService
}

// heartbeatInterval is the cadence at which the SSE writer emits a comment
// frame to defeat reverse-proxy idle buffering. 15s mirrors the design doc
// and the nginx config in deploy/.
const heartbeatInterval = 15 * time.Second

// terminalDrainTimeout bounds how long the handler will wait for the terminal
// Result after the progress channel closes. The service contract guarantees a
// Result will arrive promptly, but a bounded wait turns any future regression
// into a visible `event: error` frame rather than a hung connection.
const terminalDrainTimeout = time.Minute

// ServeHTTP validates the confirm-name gate, opens an SSE stream, attaches
// (or starts) the empty-bucket job, and pumps progress events until the
// worker terminates or the client disconnects.
//
// Important ordering invariant: validation errors are written BEFORE
// sse.New is called, so the response stays as a plain JSON action envelope
// (Content-Type: application/json). Once sse.New runs the headers flip to
// text/event-stream and the status is committed to 200 OK — at that point
// any further failures surface as `event: error` frames in the stream
// itself, never as JSON envelopes.
func (h EmptyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var body struct {
		ConfirmName   string `json:"confirm_name"`
		PurgeVersions bool   `json:"purge_versions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.StyleAction, apierror.New(http.StatusBadRequest,
			"bad_request", "Invalid JSON body"))
		return
	}
	if body.ConfirmName != name {
		apierror.Write(w, apierror.StyleAction, apierror.New(http.StatusForbidden,
			"confirm_name_mismatch", "Provided confirm_name does not match bucket name"))
		return
	}

	sw, err := sse.New(w)
	if err != nil {
		apierror.Write(w, apierror.StyleAction, apierror.Internal("SSE not supported"))
		return
	}

	progress, done, err := h.Service.StartOrAttach(r.Context(), name, body.PurgeVersions)
	if err != nil {
		// Headers/status are already committed to 200 + text/event-stream;
		// surface the failure in-band so the client's event reader handles it
		// via the same `error` listener as worker-side failures.
		_ = sw.Event("error", map[string]string{"message": err.Error()})
		return
	}

	// Heartbeat lifetime is tied to ServeHTTP via `stop`. Closing on return
	// ensures the goroutine exits before the response writer goes away.
	stop := make(chan struct{})
	defer close(stop)
	go sw.StartHeartbeat(stop, heartbeatInterval)

	for {
		select {
		case p, ok := <-progress:
			if !ok {
				// Progress channel closed by terminate(); the terminal Result
				// is either already buffered on `done` (cap=1) or arriving
				// imminently. Bound the wait so a future contract regression
				// becomes an error frame rather than a hung request.
				select {
				case res := <-done:
					if res.ErrorMessage != "" {
						_ = sw.Event("error", map[string]string{"message": res.ErrorMessage})
					} else {
						_ = sw.Event("done", map[string]any{
							"deleted_total": res.DeletedTotal,
							"duration_ms":   res.DurationMS,
						})
					}
					return
				case <-time.After(terminalDrainTimeout):
					_ = sw.Event("error", map[string]string{"message": "terminal state lost"})
					return
				}
			}
			_ = sw.Event("progress", p)
		case <-r.Context().Done():
			// Client disconnected; the worker keeps running on its own
			// context (see service.StartOrAttach) — we just stop pumping.
			return
		}
	}
}
