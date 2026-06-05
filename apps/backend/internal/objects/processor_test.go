package objects

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	miniogo "github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
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
		bytes.NewBufferString("payload"), "image/jpeg", "", "")
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

	if err := p.Delete(context.Background(), "photos", "cat.jpg", "", ""); err != nil {
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

	_, err := p.MintShareLink(context.Background(), "photos", "cat.jpg", 10, "", "")
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
	_, err := p.MintShareLink(context.Background(), "photos", "cat.jpg", requested, "", "")
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

	if _, err := p.MintShareLink(context.Background(), "photos", "albums/2025/cat.jpg", 300, "", ""); err != nil {
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

// ---------------------------------------------------------------------------
// Version operations (B4)
// ---------------------------------------------------------------------------

// buildVersions creates a slice of ObjectInfo for version tests. The
// order matches newest-first S3 semantics (index 0 is the latest).
func buildVersions() []miniogo.ObjectInfo {
	t0 := time.Unix(1700000000, 0).UTC()
	return []miniogo.ObjectInfo{
		// v3: latest delete marker (no size)
		{Key: "k", VersionID: "vdm", IsLatest: true, IsDeleteMarker: true, LastModified: t0.Add(3 * time.Second)},
		// v2: regular version
		{Key: "k", VersionID: "v2", Size: 200, ContentType: "text/plain", LastModified: t0.Add(2 * time.Second)},
		// v1: regular version
		{Key: "k", VersionID: "v1", Size: 100, ContentType: "text/plain", LastModified: t0.Add(1 * time.Second)},
		// sibling key — must be filtered out
		{Key: "k-other", VersionID: "vs", Size: 10, LastModified: t0},
	}
}

// TestListVersions_WindowingAndDeleteMarker stages four entries (3 for
// "k", 1 sibling), requests page size 2 and asserts:
//   - page 1 = [vdm, v2]; vdm.IsDeleteMarker && vdm.Size==nil; NextToken non-empty
//   - page 2 with that token = [v1]; NextToken==""
func TestListVersions_WindowingAndDeleteMarker(t *testing.T) {
	p, _ := newTestProcessor(t, &stubS3{versions: buildVersions()}, ProcessorConfig{})

	res1, err := p.ListVersions(context.Background(), "b", "k", 2, "")
	require.NoError(t, err)
	require.Len(t, res1.Versions, 2)
	require.Equal(t, "vdm", res1.Versions[0].VersionID)
	require.True(t, res1.Versions[0].IsDeleteMarker)
	require.Nil(t, res1.Versions[0].Size, "delete marker size must be nil")
	require.Equal(t, "v2", res1.Versions[1].VersionID)
	require.NotEmpty(t, res1.NextToken)

	res2, err := p.ListVersions(context.Background(), "b", "k", 2, res1.NextToken)
	require.NoError(t, err)
	require.Len(t, res2.Versions, 1)
	require.Equal(t, "v1", res2.Versions[0].VersionID)
	require.Empty(t, res2.NextToken)
}

// TestRestoreVersion_RejectsDeleteMarker asserts that calling
// RestoreVersion with a delete-marker versionID returns a 422 with
// code cannot_restore_delete_marker and does NOT invoke CopyObject.
func TestRestoreVersion_RejectsDeleteMarker(t *testing.T) {
	stub := &stubS3{versions: buildVersions()}
	p, _ := newTestProcessor(t, stub, ProcessorConfig{})

	_, err := p.RestoreVersion(context.Background(), "b", "k", "vdm", "op", "127.0.0.1")
	require.Error(t, err)
	var ae *apierror.Error
	require.ErrorAs(t, err, &ae)
	require.Equal(t, "cannot_restore_delete_marker", ae.Code)
	require.Equal(t, "", stub.copyCalledSrc, "CopyObject must not be called for delete markers")
}

// TestRestoreVersion_CopiesVersion asserts that RestoreVersion on a
// regular version calls CopyObject with that versionID and returns the
// newly-created current version (VersionID == "new-current").
func TestRestoreVersion_CopiesVersion(t *testing.T) {
	stub := &stubS3{versions: buildVersions()}
	p, _ := newTestProcessor(t, stub, ProcessorConfig{})

	v, err := p.RestoreVersion(context.Background(), "b", "k", "v2", "op", "127.0.0.1")
	require.NoError(t, err)
	require.Equal(t, "v2", stub.copyCalledSrc)
	require.Equal(t, "new-current", v.VersionID)
}

// TestDeleteVersion_RequiresConfirm asserts that calling DeleteVersion
// without confirm=true returns an error and does not call RemoveObject.
func TestDeleteVersion_RequiresConfirm(t *testing.T) {
	stub := &stubS3{versions: buildVersions()}
	p, _ := newTestProcessor(t, stub, ProcessorConfig{})

	err := p.DeleteVersion(context.Background(), "b", "k", "v1", false, "op", "127.0.0.1")
	require.Error(t, err)
	var ae *apierror.Error
	require.ErrorAs(t, err, &ae)
	require.Equal(t, 0, len(stub.removedVerIDs), "RemoveObject must not fire without confirm")
}

// TestDeleteVersion_WithConfirm asserts that confirm=true causes
// RemoveObject to be called with the supplied versionID.
func TestDeleteVersion_WithConfirm(t *testing.T) {
	stub := &stubS3{versions: buildVersions()}
	p, _ := newTestProcessor(t, stub, ProcessorConfig{})

	err := p.DeleteVersion(context.Background(), "b", "k", "v1", true, "op", "127.0.0.1")
	require.NoError(t, err)
	require.Equal(t, []string{"v1"}, stub.removedVerIDs)
}

// TestUndelete_RejectsNonDeleteMarker asserts that Undelete returns 422
// not_delete_marked when the latest version is a regular object.
func TestUndelete_RejectsNonDeleteMarker(t *testing.T) {
	// Use versions without a delete marker as latest.
	stub := &stubS3{versions: []miniogo.ObjectInfo{
		{Key: "k", VersionID: "v1", IsLatest: true, IsDeleteMarker: false, Size: 100},
	}}
	p, _ := newTestProcessor(t, stub, ProcessorConfig{})

	_, err := p.Undelete(context.Background(), "b", "k", "op", "127.0.0.1")
	require.Error(t, err)
	var ae *apierror.Error
	require.ErrorAs(t, err, &ae)
	require.Equal(t, "not_delete_marked", ae.Code)
}

// TestUndelete_RemovesDeleteMarkerAndReturnsExposed asserts that when
// the latest version IS a delete marker, Undelete removes it and
// returns the previously-hidden version.
func TestUndelete_RemovesDeleteMarkerAndReturnsExposed(t *testing.T) {
	// versions: latest = delete marker (vdm), then v1.
	stub := &stubS3{versions: []miniogo.ObjectInfo{
		{Key: "k", VersionID: "vdm", IsLatest: true, IsDeleteMarker: true},
		{Key: "k", VersionID: "v1", Size: 100, ContentType: "text/plain"},
	}}
	p, _ := newTestProcessor(t, stub, ProcessorConfig{})

	v, err := p.Undelete(context.Background(), "b", "k", "op", "127.0.0.1")
	require.NoError(t, err)
	require.Equal(t, []string{"vdm"}, stub.removedVerIDs)
	require.Equal(t, "v1", v.VersionID)
}
