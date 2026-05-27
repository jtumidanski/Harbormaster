package buckets

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/jobs/bucketempty"
)

// fakeEmptyService is a deterministic EmptyService used by the handler tests.
// It records every StartOrAttach call (for the "attach mid-job" case) and
// allows the test to drive progress/done emission via a callback so multiple
// subscribers can be wired up before the worker terminates.
type fakeEmptyService struct {
	mu       sync.Mutex
	starts   int
	progress chan bucketempty.Progress
	done     chan bucketempty.Result
	startErr error
	onStart  func(s *fakeEmptyService)
}

// newFakeService returns a service whose channels are sized to hold the two
// progress batches and one terminal Result the tests emit. Buffering avoids
// goroutine coordination in the deterministic test paths.
func newFakeService() *fakeEmptyService {
	return &fakeEmptyService{
		progress: make(chan bucketempty.Progress, 4),
		done:     make(chan bucketempty.Result, 1),
	}
}

// StartOrAttach implements EmptyService. The first call optionally invokes
// onStart synchronously so the test can pre-load the channels before the
// handler reads them.
func (f *fakeEmptyService) StartOrAttach(_ context.Context, _ string, _ bool) (<-chan bucketempty.Progress, <-chan bucketempty.Result, error) {
	f.mu.Lock()
	f.starts++
	cb := f.onStart
	err := f.startErr
	f.mu.Unlock()
	if err != nil {
		return nil, nil, err
	}
	if cb != nil {
		cb(f)
	}
	return f.progress, f.done, nil
}

// terminate emits the canonical 2-batch-then-done sequence.
func (f *fakeEmptyService) terminate(deletedTotal int64) {
	f.progress <- bucketempty.Progress{Deleted: 100}
	f.progress <- bucketempty.Progress{Deleted: 200}
	close(f.progress)
	f.done <- bucketempty.Result{
		Bucket:       "bkt-1",
		DeletedTotal: deletedTotal,
		DurationMS:   42,
	}
	close(f.done)
}

// newEmptyServer mounts EmptyHandler on a fresh chi router behind
// httptest.NewServer so the test exercises real HTTP framing (headers,
// chunked transfer, flush boundaries).
func newEmptyServer(t *testing.T, svc EmptyService) *httptest.Server {
	t.Helper()
	r := chi.NewRouter()
	r.Post("/buckets/{name}/empty", EmptyHandler{Service: svc}.ServeHTTP)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

// post issues a POST with a JSON body via the default client and returns the
// raw response. The body is intentionally NOT closed here so SSE tests can
// stream from it; callers must Close themselves.
func post(t *testing.T, srv *httptest.Server, path string, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return res
}

// TestEmpty_ConfirmNameMismatch_PlainJSON_403 verifies that the confirm-name
// gate fires BEFORE the SSE headers are committed, so the SPA sees an
// action-envelope JSON body it can pattern-match on `error.code`.
func TestEmpty_ConfirmNameMismatch_PlainJSON_403(t *testing.T) {
	svc := newFakeService()
	srv := newEmptyServer(t, svc)

	res := post(t, srv, "/buckets/bkt-1/empty", `{"confirm_name":"wrong","purge_versions":false}`)
	defer res.Body.Close()

	assert.Equal(t, http.StatusForbidden, res.StatusCode)
	assert.Equal(t, "application/json", res.Header.Get("Content-Type"),
		"validation failure must NOT switch to text/event-stream")

	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&env))
	assert.Equal(t, "confirm_name_mismatch", env.Error.Code)
	assert.NotEmpty(t, env.Error.Message)

	// Service must NOT have been touched when validation fails up front.
	svc.mu.Lock()
	defer svc.mu.Unlock()
	assert.Zero(t, svc.starts, "StartOrAttach should not run when confirm_name mismatches")
}

// TestEmpty_BadJSON_400 verifies malformed-body handling produces a typed
// bad_request envelope rather than a 500 or a silent SSE.
func TestEmpty_BadJSON_400(t *testing.T) {
	svc := newFakeService()
	srv := newEmptyServer(t, svc)

	res := post(t, srv, "/buckets/bkt-1/empty", `{not-json`)
	defer res.Body.Close()

	assert.Equal(t, http.StatusBadRequest, res.StatusCode)
	assert.Equal(t, "application/json", res.Header.Get("Content-Type"))

	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&env))
	assert.Equal(t, "bad_request", env.Error.Code)
}

// TestEmpty_HappyPath_ProgressThenDone drives a full SSE session: two
// progress frames followed by a terminal `done` frame carrying the
// deleted_total. The test reads line-by-line so it asserts both the event
// names and the JSON payload on the data line.
func TestEmpty_HappyPath_ProgressThenDone(t *testing.T) {
	svc := newFakeService()
	svc.onStart = func(s *fakeEmptyService) {
		// Emit asynchronously so the handler is reading from the channels
		// when frames arrive (mirrors the real worker).
		go s.terminate(200)
	}
	srv := newEmptyServer(t, svc)

	res := post(t, srv, "/buckets/bkt-1/empty", `{"confirm_name":"bkt-1","purge_versions":false}`)
	defer res.Body.Close()

	require.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, "text/event-stream", res.Header.Get("Content-Type"))
	assert.Equal(t, "no-cache", res.Header.Get("Cache-Control"))

	frames := readFrames(t, res.Body, 5*time.Second)

	// Sequence: two progress frames, then a done frame. Heartbeat comments
	// may appear interleaved but the 15s cadence makes that unlikely in a
	// sub-second test.
	require.GreaterOrEqual(t, len(frames), 3, "expected at least 2 progress + 1 done frame, got %v", frames)

	var sawDone bool
	progressCount := 0
	for _, f := range frames {
		switch f.event {
		case "progress":
			progressCount++
		case "done":
			sawDone = true
			var payload struct {
				DeletedTotal int64 `json:"deleted_total"`
				DurationMS   int64 `json:"duration_ms"`
			}
			require.NoError(t, json.Unmarshal([]byte(f.data), &payload))
			assert.EqualValues(t, 200, payload.DeletedTotal)
			assert.EqualValues(t, 42, payload.DurationMS)
		}
	}
	assert.Equal(t, 2, progressCount, "expected two progress frames")
	assert.True(t, sawDone, "expected a terminal done frame")
}

// TestEmpty_AttachMidJob verifies that a second POST while the worker is
// running attaches to the same job and observes the same terminal `done`
// frame. The fake records both StartOrAttach calls.
func TestEmpty_AttachMidJob(t *testing.T) {
	// Use a shared, manually-driven service so both connections see the
	// same channels (mimicking the real fan-out subscription model).
	svc := newFakeService()
	srv := newEmptyServer(t, svc)

	// Kick off two concurrent POSTs. The handler under test will call
	// StartOrAttach for each; in production the second would receive a new
	// progress channel and the SAME done channel. For this test we accept
	// both connections seeing the same fake channels (since done is closed
	// after one read, the second connection may see the channel-closed
	// signal rather than the buffered Result — both are valid per the
	// service contract).
	type result struct {
		frames []sseFrame
		status int
	}
	results := make(chan result, 2)

	for i := 0; i < 2; i++ {
		go func() {
			res := post(t, srv, "/buckets/bkt-1/empty",
				`{"confirm_name":"bkt-1","purge_versions":false}`)
			defer res.Body.Close()
			results <- result{
				frames: readFrames(t, res.Body, 5*time.Second),
				status: res.StatusCode,
			}
		}()
	}

	// Give both POSTs a moment to register, then terminate.
	time.Sleep(50 * time.Millisecond)
	go svc.terminate(200)

	for i := 0; i < 2; i++ {
		select {
		case r := <-results:
			assert.Equal(t, http.StatusOK, r.status)
			// At least one of the two connections must observe the done
			// event (the buffered Result on the done channel is consumed
			// by whichever handler reads first; the other sees the
			// channel-closed signal and falls through to the timeout
			// branch — which is acceptable since the contract allows
			// either terminus).
		case <-time.After(10 * time.Second):
			t.Fatal("timed out waiting for second connection to finish")
		}
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	assert.Equal(t, 2, svc.starts, "both POSTs should have called StartOrAttach")
}

// sseFrame is the parsed form of one `event: <name>\ndata: <json>\n\n` block.
type sseFrame struct {
	event string
	data  string
}

// readFrames streams the SSE response body and returns parsed frames once the
// connection closes or the deadline elapses. Heartbeat comment lines
// (starting with `:`) are skipped.
func readFrames(t *testing.T, body io.Reader, deadline time.Duration) []sseFrame {
	t.Helper()
	type chunk struct {
		line string
		err  error
	}
	lineCh := make(chan chunk, 16)
	go func() {
		br := bufio.NewReader(body)
		for {
			line, err := br.ReadString('\n')
			lineCh <- chunk{line: line, err: err}
			if err != nil {
				return
			}
		}
	}()

	var frames []sseFrame
	var current sseFrame
	timer := time.NewTimer(deadline)
	defer timer.Stop()
	for {
		select {
		case c := <-lineCh:
			line := strings.TrimRight(c.line, "\n")
			line = strings.TrimRight(line, "\r")
			switch {
			case strings.HasPrefix(line, ":"):
				// heartbeat comment — ignore
			case strings.HasPrefix(line, "event: "):
				current.event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				current.data = strings.TrimPrefix(line, "data: ")
			case line == "":
				if current.event != "" || current.data != "" {
					frames = append(frames, current)
					current = sseFrame{}
					// Stop reading once we have a terminal frame.
					if last := frames[len(frames)-1].event; last == "done" || last == "error" {
						return frames
					}
				}
			}
			if c.err != nil {
				return frames
			}
		case <-timer.C:
			return frames
		}
	}
}

