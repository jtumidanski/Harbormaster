package objects

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
)

func TestCountExpansion_ExplicitPlusPrefixes(t *testing.T) {
	s3 := &stubS3{
		bulkListing: map[string][]string{
			"photos/": {"photos/a.jpg", "photos/b.jpg", "photos/c.jpg"},
			"logs/":   {"logs/x.log"},
		},
	}
	count, truncated, err := countExpansion(context.Background(), s3, "b",
		[]string{"notes.txt", "readme.md"}, []string{"photos/", "logs/"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if truncated {
		t.Fatalf("did not expect truncation")
	}
	// 2 explicit keys + 3 under photos/ + 1 under logs/ = 6.
	if count != 6 {
		t.Fatalf("count = %d, want 6", count)
	}
}

func TestCountExpansion_Ceiling(t *testing.T) {
	keys := make([]string, 0, 10001)
	for i := 0; i < 10001; i++ {
		keys = append(keys, "big/"+strconv.Itoa(i))
	}
	s3 := &stubS3{bulkListing: map[string][]string{"big/": keys}}
	count, truncated, err := countExpansion(context.Background(), s3, "b", nil, []string{"big/"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != bulkDeleteCeiling {
		t.Fatalf("count = %d, want %d", count, bulkDeleteCeiling)
	}
	if !truncated {
		t.Fatalf("expected truncated=true at the ceiling")
	}
}

func TestCountExpansion_ListError(t *testing.T) {
	s3 := &stubS3{bulkListErr: errFailing}
	_, _, err := countExpansion(context.Background(), s3, "b", nil, []string{"photos/"})
	if err == nil {
		t.Fatalf("expected a listing error")
	}
}

func TestDeleteExpansion_BatchesAndDeletes(t *testing.T) {
	s3 := &stubS3{
		bulkListing: map[string][]string{"photos/": {"photos/a.jpg", "photos/b.jpg"}},
	}
	deleted, failures, err := deleteExpansion(context.Background(), s3, "b",
		[]string{"notes.txt"}, []string{"photos/"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(failures) != 0 {
		t.Fatalf("unexpected failures: %v", failures)
	}
	if deleted != 3 {
		t.Fatalf("deleted = %d, want 3", deleted)
	}
	if len(s3.removeSubmitted) != 3 {
		t.Fatalf("submitted %d keys, want 3", len(s3.removeSubmitted))
	}
}

func TestDeleteExpansion_PartialFailure(t *testing.T) {
	s3 := &stubS3{
		bulkListing:    map[string][]string{"logs/": {"logs/ok.log", "logs/locked.bin"}},
		removeFailKeys: map[string]string{"logs/locked.bin": "object is WORM-locked"},
	}
	deleted, failures, err := deleteExpansion(context.Background(), s3, "b", nil, []string{"logs/"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	if len(failures) != 1 || failures[0].Key != "logs/locked.bin" {
		t.Fatalf("failures = %+v, want one for logs/locked.bin", failures)
	}
}

func TestDeleteExpansion_ListError(t *testing.T) {
	s3 := &stubS3{bulkListErr: errFailing}
	_, _, err := deleteExpansion(context.Background(), s3, "b", nil, []string{"photos/"})
	if err == nil {
		t.Fatalf("expected a listing error to abort the delete")
	}
}

func TestBulkDelete_EmptyRequest_400(t *testing.T) {
	p, _ := newTestProcessor(t, nil, ProcessorConfig{})
	_, err := p.BulkDelete(context.Background(), "b", nil, nil, true, "alice", "1.2.3.4")
	requireAPIError(t, err, 400, "bad_request")
}

func TestBulkDelete_EmptyPrefix_400(t *testing.T) {
	p, _ := newTestProcessor(t, nil, ProcessorConfig{})
	_, err := p.BulkDelete(context.Background(), "b", nil, []string{""}, false, "alice", "1.2.3.4")
	requireAPIError(t, err, 400, "bad_request")
}

func TestBulkDelete_SlashPrefix_400(t *testing.T) {
	p, _ := newTestProcessor(t, nil, ProcessorConfig{})
	_, err := p.BulkDelete(context.Background(), "b", nil, []string{"/"}, false, "alice", "1.2.3.4")
	requireAPIError(t, err, 400, "bad_request")
}

func TestBulkDelete_InvalidKey_400(t *testing.T) {
	p, _ := newTestProcessor(t, nil, ProcessorConfig{})
	_, err := p.BulkDelete(context.Background(), "b", []string{""}, nil, false, "alice", "1.2.3.4")
	requireAPIError(t, err, 400, "object_invalid_key")
}

func TestBulkDelete_DryRun_CountsWithoutDeleting(t *testing.T) {
	s3 := &stubS3{bulkListing: map[string][]string{"photos/": {"photos/a", "photos/b"}}}
	p, stub := newTestProcessor(t, s3, ProcessorConfig{})
	res, err := p.BulkDelete(context.Background(), "b", []string{"notes.txt"}, []string{"photos/"}, true, "alice", "1.2.3.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ObjectCount != 3 || res.Truncated {
		t.Fatalf("got count=%d truncated=%v, want 3/false", res.ObjectCount, res.Truncated)
	}
	if len(stub.removeSubmitted) != 0 {
		t.Fatalf("dry-run must not delete; submitted %d keys", len(stub.removeSubmitted))
	}
}

func TestBulkDelete_Delete_AggregatesFailures(t *testing.T) {
	s3 := &stubS3{
		bulkListing:    map[string][]string{"logs/": {"logs/ok", "logs/bad"}},
		removeFailKeys: map[string]string{"logs/bad": "boom"},
	}
	p, _ := newTestProcessor(t, s3, ProcessorConfig{})
	res, err := p.BulkDelete(context.Background(), "b", nil, []string{"logs/"}, false, "alice", "1.2.3.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.DeletedCount != 1 {
		t.Fatalf("deleted = %d, want 1", res.DeletedCount)
	}
	if len(res.Failures) != 1 || res.Failures[0].Key != "logs/bad" {
		t.Fatalf("failures = %+v, want one for logs/bad", res.Failures)
	}
}

func TestBulkDelete_ListError_502(t *testing.T) {
	s3 := &stubS3{bulkListErr: errFailing}
	p, _ := newTestProcessor(t, s3, ProcessorConfig{})
	_, err := p.BulkDelete(context.Background(), "b", nil, []string{"photos/"}, false, "alice", "1.2.3.4")
	requireAPIError(t, err, 502, "minio_error")
}

func TestBulkDelete_DryRun_NoDeleteCalls(t *testing.T) {
	s3 := &stubS3{bulkListing: map[string][]string{"a/": {"a/1"}}}
	p, stub := newTestProcessor(t, s3, ProcessorConfig{})
	if _, err := p.BulkDelete(context.Background(), "b", nil, []string{"a/"}, true, "alice", "1.2.3.4"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stub.removeSubmitted) != 0 {
		t.Fatalf("dry-run submitted %d keys, want 0", len(stub.removeSubmitted))
	}
}

// requireAPIError fails the test unless err is an *apierror.Error with the
// given HTTP status and code.
func requireAPIError(t *testing.T, err error, status int, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error with status %d code %q, got nil", status, code)
	}
	var ae *apierror.Error
	if !errors.As(err, &ae) {
		t.Fatalf("error is not *apierror.Error: %v", err)
	}
	if ae.HTTPStatus != status || ae.Code != code {
		t.Fatalf("got status=%d code=%q, want status=%d code=%q", ae.HTTPStatus, ae.Code, status, code)
	}
}
