package objects

import (
	"context"
	"strconv"
	"testing"
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
