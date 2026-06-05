package lifecycle

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// newTestRouter wires a Processor with an in-memory stub behind a chi
// router that mounts the lifecycle Routes. Tests drive the router via
// httptest so the JSON:API codec and error envelopes are exercised end-
// to-end rather than calling handler closures directly.
func newTestRouter(t *testing.T, s3 *stubS3) http.Handler {
	t.Helper()
	p, _ := newTestProcessor(t, s3)
	r := chi.NewRouter()
	r.Route("/api/v1", Routes(p))
	return r
}

// doRequest is a thin helper that marshals body (or sends an empty
// request when body is nil) and records the response.
func doRequest(t *testing.T, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf *bytes.Buffer
	if body == nil {
		buf = bytes.NewBuffer(nil)
	} else {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		buf = bytes.NewBuffer(raw)
	}
	req := httptest.NewRequest(method, path, buf)
	req.Header.Set("Content-Type", "application/vnd.api+json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// TestCreateHandlerUnsupportedKind asserts that POSTing a lifecycle rule
// with an unknown kind returns 422 with the "unsupported_lifecycle_kind"
// error code and the source pointer aimed at /data/attributes/kind so
// the SPA can attach the message to the correct form field.
func TestCreateHandlerUnsupportedKind(t *testing.T) {
	t.Parallel()
	h := newTestRouter(t, &stubS3{getErr: errNoSuchLifecycle})
	body := map[string]any{
		"data": map[string]any{
			"type": "lifecycle_rules",
			"attributes": map[string]any{
				"kind": "transition",
				"days": 30,
			},
		},
	}
	rr := doRequest(t, h, http.MethodPost, "/api/v1/buckets/my-bucket/lifecycle-rules", body)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status: got %d want 422; body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/vnd.api+json" {
		t.Errorf("Content-Type: got %q want application/vnd.api+json", ct)
	}
	var doc struct {
		Errors []struct {
			Code   string `json:"code"`
			Source struct {
				Pointer string `json:"pointer"`
			} `json:"source"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode body: %v; body=%s", err, rr.Body.String())
	}
	if len(doc.Errors) == 0 {
		t.Fatalf("errors array is empty; body=%s", rr.Body.String())
	}
	if doc.Errors[0].Code != "unsupported_lifecycle_kind" {
		t.Errorf("errors[0].code = %q; want unsupported_lifecycle_kind", doc.Errors[0].Code)
	}
	const wantPointer = "/data/attributes/kind"
	if doc.Errors[0].Source.Pointer != wantPointer {
		t.Errorf("errors[0].source.pointer = %q; want %q", doc.Errors[0].Source.Pointer, wantPointer)
	}
}

// TestCreateHandlerEmptyKindRejects asserts that POSTing without a
// kind field (empty string) is also rejected with 422
// "unsupported_lifecycle_kind". Empty kind is not treated as a
// backward-compat alias for expiration because the existing
// kind="expiration" wire contract always included an explicit kind
// field; no client ever omitted it.
func TestCreateHandlerEmptyKindRejects(t *testing.T) {
	t.Parallel()
	h := newTestRouter(t, &stubS3{getErr: errNoSuchLifecycle})
	body := map[string]any{
		"data": map[string]any{
			"type": "lifecycle_rules",
			"attributes": map[string]any{
				"days": 30,
			},
		},
	}
	rr := doRequest(t, h, http.MethodPost, "/api/v1/buckets/my-bucket/lifecycle-rules", body)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status: got %d want 422; body=%s", rr.Code, rr.Body.String())
	}
}

// TestCreateHandlerExpirationKind is a smoke test confirming that the
// "expiration" path through the handler switch still reaches the
// processor and returns 201 on success. It guards against accidentally
// breaking the existing expiration contract while adding the switch.
func TestCreateHandlerExpirationKind(t *testing.T) {
	t.Parallel()
	h := newTestRouter(t, &stubS3{getErr: errNoSuchLifecycle})
	body := map[string]any{
		"data": map[string]any{
			"type": "lifecycle_rules",
			"attributes": map[string]any{
				"kind": KindExpiration,
				"days": 14,
			},
		},
	}
	rr := doRequest(t, h, http.MethodPost, "/api/v1/buckets/my-bucket/lifecycle-rules", body)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d want 201; body=%s", rr.Code, rr.Body.String())
	}
}

// TestCreateHandlerNoncurrentKind confirms the "noncurrent-expiration"
// path routes to CreateNoncurrent and returns 201.
func TestCreateHandlerNoncurrentKind(t *testing.T) {
	t.Parallel()
	h := newTestRouter(t, &stubS3{getErr: errNoSuchLifecycle})
	body := map[string]any{
		"data": map[string]any{
			"type": "lifecycle_rules",
			"attributes": map[string]any{
				"kind":            KindNoncurrentExpiration,
				"noncurrent_days": 30,
			},
		},
	}
	rr := doRequest(t, h, http.MethodPost, "/api/v1/buckets/my-bucket/lifecycle-rules", body)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d want 201; body=%s", rr.Code, rr.Body.String())
	}
}

// TestCreateHandlerAbortMPUKind confirms the "abort-incomplete-multipart"
// path routes to CreateAbortMPU and returns 201.
func TestCreateHandlerAbortMPUKind(t *testing.T) {
	t.Parallel()
	h := newTestRouter(t, &stubS3{getErr: errNoSuchLifecycle})
	body := map[string]any{
		"data": map[string]any{
			"type": "lifecycle_rules",
			"attributes": map[string]any{
				"kind":                  KindAbortIncompleteMPU,
				"days_after_initiation": 7,
			},
		},
	}
	rr := doRequest(t, h, http.MethodPost, "/api/v1/buckets/my-bucket/lifecycle-rules", body)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d want 201; body=%s", rr.Code, rr.Body.String())
	}
}
