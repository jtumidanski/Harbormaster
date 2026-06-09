//go:build integration

package integration

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	miniogo "github.com/minio/minio-go/v7"
)

// TestObjects_RoundTrip exercises the object-domain happy path:
// upload → list → download (proxy) → mint share-link → fetch the
// presigned URL → delete. The bucket is created via the live S3 client
// (not the bucket processor) so this test stays focused on the object
// domain.
func TestObjects_RoundTrip(t *testing.T) {
	env, ctx := setup(t)

	const (
		bucketName  = "harbormaster-it-objects"
		key         = "greeting.txt"
		contentType = "text/plain"
		body        = "hello, integration\n"
		actor       = "integration-test"
		sourceIP    = "127.0.0.1"
	)

	// Create the bucket directly so a bug in the bucket-processor
	// happy-path does not mask the object-domain assertions.
	if err := env.MC.MakeBucket(ctx, bucketName, miniogo.MakeBucketOptions{}); err != nil {
		t.Fatalf("MakeBucket: %v", err)
	}

	entry, err := env.Objects.Upload(ctx, bucketName, key, strings.NewReader(body), contentType, actor, sourceIP)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if entry.Key != key {
		t.Errorf("Upload returned key=%q, want %q", entry.Key, key)
	}
	if entry.Size != int64(len(body)) {
		t.Errorf("Upload returned size=%d, want %d", entry.Size, len(body))
	}

	listRes, err := env.Objects.List(ctx, bucketName, "", "", 100, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, e := range listRes.Entries {
		if e.Key == key {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("uploaded object %q not present in List output (%d entries)", key, len(listRes.Entries))
	}

	rc, dlEntry, err := env.Objects.Download(ctx, bucketName, key, "", actor, sourceIP)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer rc.Close()
	gotBody, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read download stream: %v", err)
	}
	if string(gotBody) != body {
		t.Errorf("download body: got %q, want %q", gotBody, body)
	}
	if dlEntry.Size != int64(len(body)) {
		t.Errorf("download stat size=%d, want %d", dlEntry.Size, len(body))
	}

	share, err := env.Objects.MintShareLink(ctx, bucketName, key, 300, actor, sourceIP)
	if err != nil {
		t.Fatalf("MintShareLink: %v", err)
	}
	if share.URL == "" {
		t.Fatalf("MintShareLink returned empty URL")
	}
	resp, err := http.Get(share.URL)
	if err != nil {
		t.Fatalf("GET share URL: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("share URL status: got %d, want 200", resp.StatusCode)
	}
	sharedBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read share body: %v", err)
	}
	if !bytes.Equal(sharedBody, []byte(body)) {
		t.Errorf("share body: got %q, want %q", sharedBody, body)
	}

	if err := env.Objects.Delete(ctx, bucketName, key, actor, sourceIP); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Confirm the key really is gone via the underlying client.
	_, err = env.MC.StatObject(ctx, bucketName, key, miniogo.StatObjectOptions{})
	if err == nil {
		t.Errorf("StatObject after Delete: expected error, got nil")
	}

	// Cleanup: remove the bucket so a re-run of the integration suite
	// against the same MinIO does not leak state. Failure here is
	// non-fatal; the testcontainer is destroyed when the test ends.
	_ = env.MC.RemoveBucket(ctx, bucketName)
}
