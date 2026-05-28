package buckets

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	miniogo "github.com/minio/minio-go/v7"
)

// newTestRouter wires the in-memory processor stubs behind a chi router
// that mounts the bucket Routes registrar. Tests drive the router via
// httptest so the JSON:API and action envelopes are exercised end-to-end
// rather than calling the handler closures directly.
func newTestRouter(t *testing.T) (http.Handler, *stubAdmin, *stubS3) {
	t.Helper()
	p, adm, s3 := newTestProcessor(t, nil, nil)
	r := chi.NewRouter()
	r.Route("/api/v1", Routes(p, nil))
	return r, adm, s3
}

func doRequest(t *testing.T, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Buffer
	if body == nil {
		reader = bytes.NewBuffer(nil)
	} else {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewBuffer(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// TestListReturnsJSONAPICollection asserts GET /buckets renders the
// JSON:API collection envelope with the correct Content-Type and a
// non-nil data array. A single bucket is wired up so the assertion can
// also confirm the attributes block carries the bucket name.
func TestListReturnsJSONAPICollection(t *testing.T) {
	h, _, s3 := newTestRouter(t)
	s3.buckets = []miniogo.BucketInfo{
		{Name: "photos", CreationDate: time.Unix(1700000000, 0).UTC()},
	}

	rr := doRequest(t, h, http.MethodGet, "/api/v1/buckets", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/vnd.api+json" {
		t.Errorf("Content-Type: got %q want application/vnd.api+json", ct)
	}
	var doc struct {
		Data []struct {
			Type       string          `json:"type"`
			ID         string          `json:"id"`
			Attributes json.RawMessage `json:"attributes"`
		} `json:"data"`
		Meta struct {
			Page struct {
				TotalRecords int `json:"total_records"`
			} `json:"page"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode body: %v; body=%s", err, rr.Body.String())
	}
	if len(doc.Data) != 1 {
		t.Fatalf("data length: got %d want 1", len(doc.Data))
	}
	if doc.Data[0].Type != "buckets" || doc.Data[0].ID != "photos" {
		t.Errorf("resource identity wrong: %+v", doc.Data[0])
	}
	if doc.Meta.Page.TotalRecords != 1 {
		t.Errorf("meta.page.total_records: got %d want 1", doc.Meta.Page.TotalRecords)
	}
}

// TestBucketResourceMarshalsSnakeCaseAttributes pins the wire contract for the
// attributes block. Without a custom MarshalJSON the embedded Bucket serialises
// with Go field names (PascalCase: "ObjectCount"), but the API contract and the
// SPA both read snake_case ("object_count") — the mismatch renders every field
// undefined client-side and crashes the bucket list on the first non-empty load.
func TestBucketResourceMarshalsSnakeCaseAttributes(t *testing.T) {
	r := BucketResource{Bucket{
		Name:              "photos",
		CreatedAt:         time.Unix(1700000000, 0).UTC(),
		EstimatedBytes:    2048,
		ObjectCount:       7,
		VersioningEnabled: true,
		HasLifecycleRules: true,
		PublicAccess:      PublicAccessPublicRead,
		Quota:             &Quota{Kind: QuotaKindHard, Bytes: 4096, UsedBytes: 2048},
	}}
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{
		"name", "created_at", "estimated_bytes", "object_count",
		"versioning_enabled", "has_lifecycle_rules", "public_access",
	} {
		if _, ok := m[k]; !ok {
			t.Errorf("attributes missing snake_case key %q; got %s", k, raw)
		}
	}
	if _, leaked := m["ObjectCount"]; leaked {
		t.Errorf("attributes leaked PascalCase key: %s", raw)
	}
	if m["object_count"] != float64(7) {
		t.Errorf("object_count: got %v want 7", m["object_count"])
	}
	q, ok := m["quota"].(map[string]any)
	if !ok {
		t.Fatalf("quota not a nested object: %s", raw)
	}
	for _, k := range []string{"kind", "bytes", "used_bytes"} {
		if _, ok := q[k]; !ok {
			t.Errorf("quota missing snake_case key %q; got %s", k, raw)
		}
	}
}

// TestBucketResourceMarshalsNilQuotaAsNull ensures the optional quota block is
// emitted as JSON null (not omitted, not "{}") so the SPA's `quota: Quota|null`
// guard works.
func TestBucketResourceMarshalsNilQuotaAsNull(t *testing.T) {
	raw, err := json.Marshal(BucketResource{Bucket{Name: "b", PublicAccess: PublicAccessPrivate}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	v, present := m["quota"]
	if !present || v != nil {
		t.Errorf("quota: got %v (present=%v) want null", v, present)
	}
}

// TestCreateRejectsInvalidName drives the POST /buckets endpoint with a
// name MinIO's strict rules reject ("Photos" — uppercase letters). The
// handler must return 422 + the JSON:API errors[] envelope with code
// invalid_bucket_name and source.pointer pointing at the offending field.
func TestCreateRejectsInvalidName(t *testing.T) {
	h, _, _ := newTestRouter(t)

	body := map[string]any{
		"data": map[string]any{
			"type":       "buckets",
			"attributes": map[string]any{"name": "Photos"},
		},
	}
	rr := doRequest(t, h, http.MethodPost, "/api/v1/buckets", body)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status: got %d want 422; body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/vnd.api+json" {
		t.Errorf("Content-Type: got %q want application/vnd.api+json", ct)
	}
	var doc struct {
		Errors []struct {
			Status string `json:"status"`
			Code   string `json:"code"`
			Source struct {
				Pointer string `json:"pointer"`
			} `json:"source"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode body: %v; body=%s", err, rr.Body.String())
	}
	if len(doc.Errors) != 1 {
		t.Fatalf("errors length: got %d want 1; body=%s", len(doc.Errors), rr.Body.String())
	}
	if doc.Errors[0].Code != "invalid_bucket_name" {
		t.Errorf("code: got %q want invalid_bucket_name", doc.Errors[0].Code)
	}
	if doc.Errors[0].Status != "422" {
		t.Errorf("status field: got %q want 422", doc.Errors[0].Status)
	}
	if doc.Errors[0].Source.Pointer != "/data/attributes/name" {
		t.Errorf("source.pointer: got %q want /data/attributes/name", doc.Errors[0].Source.Pointer)
	}
}

// TestCreateSucceeds verifies the happy path: a valid POST returns 201
// with the JSON:API single-resource document and the bucket name in
// data.id.
func TestCreateSucceeds(t *testing.T) {
	h, _, s3 := newTestRouter(t)
	s3.buckets = []miniogo.BucketInfo{
		{Name: "backups", CreationDate: time.Unix(1700000000, 0).UTC()},
	}

	body := map[string]any{
		"data": map[string]any{
			"type":       "buckets",
			"attributes": map[string]any{"name": "backups"},
		},
	}
	rr := doRequest(t, h, http.MethodPost, "/api/v1/buckets", body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d want 201; body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/vnd.api+json" {
		t.Errorf("Content-Type: got %q want application/vnd.api+json", ct)
	}
	var doc struct {
		Data struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode body: %v; body=%s", err, rr.Body.String())
	}
	if doc.Data.Type != "buckets" || doc.Data.ID != "backups" {
		t.Errorf("data: got %+v want {type:buckets,id:backups}", doc.Data)
	}
	if len(s3.makeCalls) != 1 || s3.makeCalls[0] != "backups" {
		t.Errorf("MakeBucket calls: got %+v want [backups]", s3.makeCalls)
	}
}

// TestDeleteRejectsMismatchedConfirmName asserts the destructive-action
// guard: DELETE must return 403 with the action envelope when confirm_name
// disagrees with the path param, and must NOT have hit RemoveBucket.
func TestDeleteRejectsMismatchedConfirmName(t *testing.T) {
	h, _, s3 := newTestRouter(t)

	rr := doRequest(t, h, http.MethodDelete, "/api/v1/buckets/photos",
		map[string]string{"confirm_name": "wrong"})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status: got %d want 403; body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: got %q want application/json", ct)
	}
	var doc struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode body: %v; body=%s", err, rr.Body.String())
	}
	if doc.Error.Code != "confirm_name_mismatch" {
		t.Errorf("error.code: got %q want confirm_name_mismatch", doc.Error.Code)
	}
	if len(s3.removeCalls) != 0 {
		t.Errorf("RemoveBucket fired despite mismatch: %+v", s3.removeCalls)
	}
}

// TestSetQuotaFifoOnVersionedBucketRejects covers the FIFO/versioning
// cross-invariant via the action endpoint. The fixture configures the
// stub to report versioning Enabled for "photos"; the PUT must return
// 422 fifo_requires_versioning_off in the action envelope.
func TestSetQuotaFifoOnVersionedBucketRejects(t *testing.T) {
	h, adm, s3 := newTestRouter(t)
	s3.versioning["photos"] = miniogo.BucketVersioningConfiguration{Status: "Enabled"}

	rr := doRequest(t, h, http.MethodPut, "/api/v1/buckets/photos/quota",
		map[string]any{"kind": "fifo", "bytes": 1 << 30})
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status: got %d want 422; body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: got %q want application/json", ct)
	}
	var doc struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode body: %v; body=%s", err, rr.Body.String())
	}
	if doc.Error.Code != "fifo_requires_versioning_off" {
		t.Errorf("error.code: got %q want fifo_requires_versioning_off", doc.Error.Code)
	}
	if len(adm.setQuotaCalls) != 0 {
		t.Errorf("SetBucketQuota fired despite validation failure: %+v", adm.setQuotaCalls)
	}
}

// TestSetPublicAccessPublicReadSucceeds asserts the public-read happy
// path: 204 No Content and a captured SetBucketPolicy call whose policy
// body contains the canned s3:GetObject statement from
// internal/policies.BucketPolicyFor.
func TestSetPublicAccessPublicReadSucceeds(t *testing.T) {
	h, _, s3 := newTestRouter(t)

	rr := doRequest(t, h, http.MethodPut, "/api/v1/buckets/photos/public-access",
		map[string]any{"mode": "public-read"})
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204; body=%s", rr.Code, rr.Body.String())
	}
	if len(s3.setPolicyCalls) != 1 {
		t.Fatalf("SetBucketPolicy calls: got %d want 1", len(s3.setPolicyCalls))
	}
	got := s3.setPolicyCalls[0]
	if got.Bucket != "photos" {
		t.Errorf("policy bucket: got %q want photos", got.Bucket)
	}
	if !strings.Contains(got.Policy, "s3:GetObject") {
		t.Errorf("policy missing s3:GetObject: %s", got.Policy)
	}
	if !strings.Contains(got.Policy, "arn:aws:s3:::photos") {
		t.Errorf("policy missing bucket ARN: %s", got.Policy)
	}
}

// TestEmptyReturnsNotImplemented documents the T3.3/T3.5 boundary:
// POST /buckets/{name}/empty validates the body but returns 501 until the
// SSE handler lands in T3.5.
func TestEmptyReturnsNotImplemented(t *testing.T) {
	h, _, _ := newTestRouter(t)

	rr := doRequest(t, h, http.MethodPost, "/api/v1/buckets/photos/empty",
		map[string]any{"confirm_name": "photos", "purge_versions": false})
	if rr.Code != http.StatusNotImplemented {
		t.Fatalf("status: got %d want 501; body=%s", rr.Code, rr.Body.String())
	}
	var doc struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode body: %v; body=%s", err, rr.Body.String())
	}
	if doc.Error.Code != "not_implemented" {
		t.Errorf("error.code: got %q want not_implemented", doc.Error.Code)
	}
}

// Compile-time sanity: ensure newTestProcessor and Routes interoperate.
var _ = context.Background
