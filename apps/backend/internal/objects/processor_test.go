package objects

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	miniogo "github.com/minio/minio-go/v7"
)

// TestList_EmptyBucket asserts the happy path for a bucket that returns
// no contents and no common prefixes: the result is non-nil with both
// slices empty and NextToken == "".
func TestList_EmptyBucket(t *testing.T) {
	p, s3 := newTestProcessor(t, &stubS3{
		pages: map[string]miniogo.ListBucketV2Result{
			"": {Contents: nil, CommonPrefixes: nil, IsTruncated: false},
		},
	}, ProcessorConfig{})

	res, err := p.List(context.Background(), "photos", "", "", 0, "")
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(res.Entries) != 0 {
		t.Errorf("Entries: got %d want 0", len(res.Entries))
	}
	if len(res.Prefixes) != 0 {
		t.Errorf("Prefixes: got %d want 0", len(res.Prefixes))
	}
	if res.NextToken != "" {
		t.Errorf("NextToken: got %q want empty", res.NextToken)
	}
	if len(s3.listCalls) != 1 {
		t.Fatalf("ListObjectsV2 calls: got %d want 1", len(s3.listCalls))
	}
	if got := s3.listCalls[0]; got.Bucket != "photos" || got.ContinuationToken != "" {
		t.Errorf("call: got %+v want bucket=photos, token=\"\"", got)
	}
}

// TestList_PaginatedRoundTrip stages two pages keyed by the inbound
// continuation token, then asserts both calls decode and that the
// second call's request really did carry the token surfaced by the
// first page.
func TestList_PaginatedRoundTrip(t *testing.T) {
	first := miniogo.ListBucketV2Result{
		Contents: []miniogo.ObjectInfo{
			{Key: "a", Size: 1, LastModified: time.Unix(1, 0).UTC(), ContentType: "text/plain", ETag: "etag-a"},
		},
		IsTruncated:           true,
		NextContinuationToken: "token-page-2",
	}
	second := miniogo.ListBucketV2Result{
		Contents: []miniogo.ObjectInfo{
			{Key: "b", Size: 2, LastModified: time.Unix(2, 0).UTC(), ContentType: "text/plain", ETag: "etag-b"},
		},
		IsTruncated: false,
	}
	p, s3 := newTestProcessor(t, &stubS3{
		pages: map[string]miniogo.ListBucketV2Result{"": first, "token-page-2": second},
	}, ProcessorConfig{})

	res1, err := p.List(context.Background(), "photos", "", "", 50, "")
	if err != nil {
		t.Fatalf("first List: %v", err)
	}
	if len(res1.Entries) != 1 || res1.Entries[0].Key != "a" {
		t.Fatalf("first page: %+v", res1)
	}
	if res1.NextToken != "token-page-2" {
		t.Fatalf("NextToken: got %q want token-page-2", res1.NextToken)
	}

	res2, err := p.List(context.Background(), "photos", "", "", 50, res1.NextToken)
	if err != nil {
		t.Fatalf("second List: %v", err)
	}
	if len(res2.Entries) != 1 || res2.Entries[0].Key != "b" {
		t.Fatalf("second page: %+v", res2)
	}
	if res2.NextToken != "" {
		t.Errorf("second-page NextToken: got %q want empty", res2.NextToken)
	}

	if len(s3.listCalls) != 2 {
		t.Fatalf("ListObjectsV2 calls: got %d want 2", len(s3.listCalls))
	}
	if s3.listCalls[1].ContinuationToken != "token-page-2" {
		t.Errorf("second call token: got %q want token-page-2", s3.listCalls[1].ContinuationToken)
	}
}

// TestList_DelimiterReturnsPrefixes asserts that a delimiter-style
// response populates the prefixes[] slice and that the delimiter is
// forwarded to the SDK.
func TestList_DelimiterReturnsPrefixes(t *testing.T) {
	p, s3 := newTestProcessor(t, &stubS3{
		pages: map[string]miniogo.ListBucketV2Result{
			"": {
				CommonPrefixes: []miniogo.CommonPrefix{
					{Prefix: "photos/2025/"},
					{Prefix: "photos/2026/"},
				},
			},
		},
	}, ProcessorConfig{})

	res, err := p.List(context.Background(), "photos", "photos/", "/", 0, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(res.Prefixes) != 2 {
		t.Fatalf("Prefixes: got %d want 2", len(res.Prefixes))
	}
	if res.Prefixes[0].Name != "photos/2025/" {
		t.Errorf("first prefix: %q", res.Prefixes[0].Name)
	}
	if got := s3.listCalls[0].Delimiter; got != "/" {
		t.Errorf("delimiter forwarded: got %q want /", got)
	}
}

// TestUpload_ReturnsEntry asserts the Upload return value matches the
// stub-supplied UploadInfo (key, size, etag, last-modified) with the
// content-type threaded through from the request.
func TestUpload_ReturnsEntry(t *testing.T) {
	stub := &stubS3{
		putReturn: miniogo.UploadInfo{
			Bucket:       "photos",
			Key:          "cat.jpg",
			Size:         42,
			ETag:         "etag-cat",
			LastModified: time.Unix(1700000000, 0).UTC(),
		},
	}
	p, _ := newTestProcessor(t, stub, ProcessorConfig{})

	entry, err := p.Upload(context.Background(), "photos", "cat.jpg",
		bytes.NewBufferString("payload"), "image/jpeg")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if entry.Key != "cat.jpg" {
		t.Errorf("Key: %q", entry.Key)
	}
	if entry.Size != 42 {
		t.Errorf("Size: %d", entry.Size)
	}
	if entry.ETag != "etag-cat" {
		t.Errorf("ETag: %q", entry.ETag)
	}
	if entry.ContentType != "image/jpeg" {
		t.Errorf("ContentType: %q", entry.ContentType)
	}
}

// TestDelete_CallsRemove verifies the stub captured the bucket + key
// pair RemoveObject was invoked with.
func TestDelete_CallsRemove(t *testing.T) {
	p, s3 := newTestProcessor(t, nil, ProcessorConfig{})

	if err := p.Delete(context.Background(), "photos", "cat.jpg"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(s3.removeCalls) != 1 {
		t.Fatalf("RemoveObject calls: got %d want 1", len(s3.removeCalls))
	}
	if got := s3.removeCalls[0]; got.Bucket != "photos" || got.Key != "cat.jpg" {
		t.Errorf("call: %+v", got)
	}
}

// TestMintShareLink_ClampsLowerBound feeds a tiny expires_seconds value
// and asserts the presigned expiry passed to the SDK is clamped up to
// ShareLinkMinTTL (30 s).
func TestMintShareLink_ClampsLowerBound(t *testing.T) {
	p, s3 := newTestProcessor(t, nil, ProcessorConfig{ShareLinkMaxTTL: time.Hour})

	_, err := p.MintShareLink(context.Background(), "photos", "cat.jpg", 10)
	if err != nil {
		t.Fatalf("MintShareLink: %v", err)
	}
	if len(s3.presignCalls) != 1 {
		t.Fatalf("PresignedGetObject calls: got %d want 1", len(s3.presignCalls))
	}
	if got := s3.presignCalls[0].Expires; got != ShareLinkMinTTL {
		t.Errorf("Expires: got %v want %v", got, ShareLinkMinTTL)
	}
}

// TestMintShareLink_ClampsUpperBound feeds an expires_seconds that
// exceeds the configured ShareLinkMaxTTL and asserts the presigned
// expiry passed to the SDK is clamped down to the max.
func TestMintShareLink_ClampsUpperBound(t *testing.T) {
	max := 2 * time.Hour
	p, s3 := newTestProcessor(t, nil, ProcessorConfig{ShareLinkMaxTTL: max})

	requested := int(max.Seconds()) + 1000
	_, err := p.MintShareLink(context.Background(), "photos", "cat.jpg", requested)
	if err != nil {
		t.Fatalf("MintShareLink: %v", err)
	}
	if got := s3.presignCalls[0].Expires; got != max {
		t.Errorf("Expires: got %v want %v", got, max)
	}
}

// TestMintShareLink_ResponseHasContentDisposition asserts the
// share-link presigned URL carries the response-content-disposition
// override so a browser opening the link downloads (rather than
// renders) the file. This also indirectly verifies the basename is
// extracted from the key.
func TestMintShareLink_ResponseHasContentDisposition(t *testing.T) {
	p, s3 := newTestProcessor(t, nil, ProcessorConfig{ShareLinkMaxTTL: time.Hour})

	if _, err := p.MintShareLink(context.Background(), "photos", "albums/2025/cat.jpg", 300); err != nil {
		t.Fatalf("MintShareLink: %v", err)
	}
	disp := s3.presignCalls[0].Params.Get("response-content-disposition")
	if !strings.Contains(disp, "attachment") {
		t.Errorf("response-content-disposition: %q", disp)
	}
	if !strings.Contains(disp, "cat.jpg") {
		t.Errorf("response-content-disposition missing basename: %q", disp)
	}
}

// TestMintShareLink_AuditPayloadHasNoURL is a placeholder for the
// T3.23 audit-wiring task: once MintShareLink records an audit row,
// this test must assert the recorded PayloadSummary contains
// {bucket, key, expires_seconds} only and NEVER the minted URL.
//
// TODO(T3.23): unstub once audit.Processor is wired in.
func TestMintShareLink_AuditPayloadHasNoURL(t *testing.T) {
	t.Skip("deferred: audit wiring lands in T3.23")
}
