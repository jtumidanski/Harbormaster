package objects

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	miniogo "github.com/minio/minio-go/v7"
)

// newTestRouter wires the in-memory processor stub behind a chi router
// that mounts the object Routes registrar. Tests drive the router via
// httptest so the JSON:API and action envelopes are exercised end-to-
// end rather than calling the handler closures directly.
func newTestRouter(t *testing.T, cfg ProcessorConfig, stub *stubS3) (http.Handler, *stubS3) {
	t.Helper()
	p, s3 := newTestProcessor(t, stub, cfg)
	r := chi.NewRouter()
	r.Route("/api/v1", Routes(p))
	return r, s3
}

// TestList_HTTP_ReturnsJSONAPI asserts the list endpoint emits a
// JSON:API collection document containing both object_entries and
// object_prefixes resource types, plus meta.page.next_token surfaced
// from the stub's NextContinuationToken.
func TestList_HTTP_ReturnsJSONAPI(t *testing.T) {
	stub := &stubS3{
		pages: map[string]miniogo.ListBucketV2Result{
			"": {
				Contents: []miniogo.ObjectInfo{
					{Key: "cat.jpg", Size: 1024, LastModified: time.Unix(1700000000, 0).UTC(), ContentType: "image/jpeg", ETag: "etag-cat"},
				},
				CommonPrefixes:        []miniogo.CommonPrefix{{Prefix: "albums/"}},
				IsTruncated:           true,
				NextContinuationToken: "tok-2",
			},
		},
	}
	h, _ := newTestRouter(t, ProcessorConfig{}, stub)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/buckets/photos/objects?delimiter=/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/vnd.api+json" {
		t.Errorf("Content-Type: %q", ct)
	}

	var doc struct {
		Data []struct {
			Type       string          `json:"type"`
			ID         string          `json:"id"`
			Attributes json.RawMessage `json:"attributes"`
		} `json:"data"`
		Meta struct {
			Page struct {
				NextToken string `json:"next_token"`
			} `json:"page"`
		} `json:"meta"`
		Links struct {
			Next string `json:"next"`
		} `json:"links"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v body=%s", err, rr.Body.String())
	}
	if len(doc.Data) != 2 {
		t.Fatalf("data length: got %d want 2", len(doc.Data))
	}

	types := map[string]bool{}
	for _, d := range doc.Data {
		types[d.Type] = true
	}
	if !types["object_entries"] || !types["object_prefixes"] {
		t.Errorf("types: %+v (want both object_entries and object_prefixes)", types)
	}

	if doc.Meta.Page.NextToken != "tok-2" {
		t.Errorf("meta.page.next_token: %q want tok-2", doc.Meta.Page.NextToken)
	}
	if !strings.Contains(doc.Links.Next, "page%5Btoken%5D=tok-2") && !strings.Contains(doc.Links.Next, "page[token]=tok-2") {
		t.Errorf("links.next missing token: %q", doc.Links.Next)
	}
}

// multipartUpload synthesises a multipart/form-data request body with
// the canonical Harbormaster shape (key + file). Returns the body and
// the Content-Type header value.
func multipartUpload(t *testing.T, key string, payload []byte, contentType string) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("key", key); err != nil {
		t.Fatalf("write key: %v", err)
	}
	if contentType != "" {
		if err := mw.WriteField("content_type", contentType); err != nil {
			t.Fatalf("write content_type: %v", err)
		}
	}
	fw, err := mw.CreateFormFile("file", key)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := fw.Write(payload); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close mw: %v", err)
	}
	return &buf, mw.FormDataContentType()
}

// TestUpload_OverCap_Returns413 wraps the body in MaxBytesReader at
// UploadMaxBytes=1024 and posts a payload larger than that. The
// expected envelope is 413 with code upload_too_large and
// details.limit_bytes set to the cap.
func TestUpload_OverCap_Returns413(t *testing.T) {
	const cap = int64(1024)
	h, s3 := newTestRouter(t, ProcessorConfig{UploadMaxBytes: cap}, nil)

	// 2 KiB payload — comfortably above the 1 KiB cap even after the
	// multipart envelope overhead is added.
	body, ct := multipartUpload(t, "cat.jpg", bytes.Repeat([]byte("A"), 2048), "image/jpeg")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/buckets/photos/objects", body)
	req.Header.Set("Content-Type", ct)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status: got %d want 413; body=%s", rr.Code, rr.Body.String())
	}
	// 413 is rendered action-style so the SPA can read details.limit_bytes
	// (the JSON:API errors[] envelope drops the Details map).
	var doc struct {
		Error struct {
			Code    string         `json:"code"`
			Message string         `json:"message"`
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v body=%s", err, rr.Body.String())
	}
	if doc.Error.Code != "upload_too_large" {
		t.Fatalf("error.code: got %q body=%s", doc.Error.Code, rr.Body.String())
	}
	if doc.Error.Details["limit_bytes"] != float64(cap) {
		t.Errorf("details.limit_bytes: got %v want %d; body=%s",
			doc.Error.Details["limit_bytes"], cap, rr.Body.String())
	}
	if len(s3.putCalls) != 0 {
		t.Errorf("PutObject fired despite cap rejection: %+v", s3.putCalls)
	}
}

// TestUpload_UnderCap_Returns201 posts a 100-byte file with cap=1024
// and asserts the response is 201 with a JSON:API single-resource
// document of type object_entries whose attributes.key matches.
func TestUpload_UnderCap_Returns201(t *testing.T) {
	h, s3 := newTestRouter(t, ProcessorConfig{UploadMaxBytes: 1024}, nil)

	body, ct := multipartUpload(t, "small.txt", bytes.Repeat([]byte("x"), 100), "text/plain")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/buckets/photos/objects", body)
	req.Header.Set("Content-Type", ct)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d want 201; body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/vnd.api+json" {
		t.Errorf("Content-Type: %q", ct)
	}
	var doc struct {
		Data struct {
			Type       string `json:"type"`
			ID         string `json:"id"`
			Attributes struct {
				Key         string `json:"key"`
				ContentType string `json:"content_type"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v body=%s", err, rr.Body.String())
	}
	if doc.Data.Type != "object_entries" || doc.Data.ID != "small.txt" {
		t.Errorf("data: %+v", doc.Data)
	}
	if doc.Data.Attributes.Key != "small.txt" {
		t.Errorf("attributes.key: %q", doc.Data.Attributes.Key)
	}
	if len(s3.putCalls) != 1 {
		t.Fatalf("PutObject calls: got %d want 1", len(s3.putCalls))
	}
	if s3.putCalls[0].ContentType != "text/plain" {
		t.Errorf("content type forwarded: %q", s3.putCalls[0].ContentType)
	}
}

// TestDownload_ProxyMode_StreamsBytes asserts the proxy-mode handler
// streams the object body verbatim and sets Content-Disposition with
// the key's basename.
func TestDownload_ProxyMode_StreamsBytes(t *testing.T) {
	payload := []byte("hello bytes")
	stub := &stubS3{
		getBody: payload,
		statReturn: miniogo.ObjectInfo{
			Key:         "cat.jpg",
			Size:        int64(len(payload)),
			ContentType: "image/jpeg",
		},
	}
	h, _ := newTestRouter(t, ProcessorConfig{DownloadProxyMode: "proxy"}, stub)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/buckets/photos/objects/download?key=albums/cat.jpg", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	if !bytes.Equal(rr.Body.Bytes(), payload) {
		t.Errorf("body bytes mismatch: got %q want %q", rr.Body.String(), payload)
	}
	disp := rr.Header().Get("Content-Disposition")
	if !strings.Contains(disp, "attachment") || !strings.Contains(disp, "cat.jpg") {
		t.Errorf("Content-Disposition: %q", disp)
	}
}

// TestDownload_DirectMode_307Redirect asserts the direct-mode handler
// emits a 307 with the Location header set to the presigned URL the
// stub returned.
func TestDownload_DirectMode_307Redirect(t *testing.T) {
	stubURL, _ := url.Parse("https://minio.example/photos/cat.jpg?X-Amz-Signature=abc")
	stub := &stubS3{presignReturn: stubURL}
	h, _ := newTestRouter(t, ProcessorConfig{DownloadProxyMode: "direct"}, stub)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/buckets/photos/objects/download?key=cat.jpg", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status: got %d want 307; body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Location"); got != stubURL.String() {
		t.Errorf("Location: got %q want %q", got, stubURL.String())
	}
}

// TestShareLink_Returns201 posts a JSON body with key + expires_seconds
// and asserts the response is a 201 JSON:API single-resource document
// whose attributes carry url and expires_at.
func TestShareLink_Returns201(t *testing.T) {
	h, _ := newTestRouter(t, ProcessorConfig{ShareLinkMaxTTL: time.Hour}, nil)

	body := bytes.NewBufferString(`{"key":"cat.jpg","expires_seconds":300}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/buckets/photos/objects/share-links", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d want 201; body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/vnd.api+json" {
		t.Errorf("Content-Type: %q", ct)
	}
	var doc struct {
		Data struct {
			Type       string `json:"type"`
			Attributes struct {
				URL       string    `json:"url"`
				ExpiresAt time.Time `json:"expires_at"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v body=%s", err, rr.Body.String())
	}
	if doc.Data.Type != "object_share_links" {
		t.Errorf("data.type: %q", doc.Data.Type)
	}
	if doc.Data.Attributes.URL == "" {
		t.Errorf("attributes.url empty")
	}
	if doc.Data.Attributes.ExpiresAt.IsZero() {
		t.Errorf("attributes.expires_at zero")
	}
}

// TestDelete_HTTP_Returns204 verifies the delete handler returns 204
// with an empty body and that RemoveObject saw the right key.
func TestDelete_HTTP_Returns204(t *testing.T) {
	h, s3 := newTestRouter(t, ProcessorConfig{}, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/buckets/photos/objects?key=cat.jpg", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204; body=%s", rr.Code, rr.Body.String())
	}
	if rr.Body.Len() != 0 {
		t.Errorf("body should be empty: %q", rr.Body.String())
	}
	if len(s3.removeCalls) != 1 || s3.removeCalls[0].Key != "cat.jpg" {
		t.Errorf("RemoveObject: %+v", s3.removeCalls)
	}
}

// Compile-time sanity: ensure newTestProcessor and Routes interoperate.
var _ = context.Background
