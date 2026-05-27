//go:build integration

package integration

import (
	"testing"

	"github.com/jtumidanski/Harbormaster/internal/buckets"
)

// TestBuckets_RoundTrip drives the bucket lifecycle through the live
// processor against the testcontainer MinIO:
//
//   - Create (with no extras) → List (bucket appears) → Get (returns
//     the bucket with default settings) → SetVersioning (true) →
//     SetQuota (hard, 50 MiB) → Get (confirms both) → Delete (with
//     confirmName).
//
// Each step uses real MinIO clients via the wired-up adapters in
// helper.go, so a successful run validates the full processor →
// adapter → minio-go path the production server uses.
func TestBuckets_RoundTrip(t *testing.T) {
	env, ctx := setup(t)

	const (
		bucketName = "harbormaster-it-buckets"
		actor      = "integration-test"
		sourceIP   = "127.0.0.1"
	)

	if _, err := env.Buckets.Create(ctx, bucketName, buckets.CreateOpts{}, actor, sourceIP); err != nil {
		t.Fatalf("Create: %v", err)
	}

	list, err := env.Buckets.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, b := range list {
		if b.Name == bucketName {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("created bucket %q not present in List output (%d buckets)", bucketName, len(list))
	}

	got, err := env.Buckets.Get(ctx, bucketName)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != bucketName {
		t.Fatalf("Get returned name=%q, want %q", got.Name, bucketName)
	}
	if got.VersioningEnabled {
		t.Errorf("freshly created bucket should not have versioning enabled")
	}

	if err := env.Buckets.SetVersioning(ctx, bucketName, true, actor, sourceIP); err != nil {
		t.Fatalf("SetVersioning(true): %v", err)
	}
	got, err = env.Buckets.Get(ctx, bucketName)
	if err != nil {
		t.Fatalf("Get after SetVersioning: %v", err)
	}
	if !got.VersioningEnabled {
		t.Errorf("expected VersioningEnabled=true after SetVersioning(true)")
	}

	const quotaBytes = int64(50 * 1024 * 1024)
	if err := env.Buckets.SetQuota(ctx, bucketName, buckets.QuotaKindHard, quotaBytes, actor, sourceIP); err != nil {
		t.Fatalf("SetQuota: %v", err)
	}
	got, err = env.Buckets.Get(ctx, bucketName)
	if err != nil {
		t.Fatalf("Get after SetQuota: %v", err)
	}
	if got.Quota == nil || got.Quota.Bytes != quotaBytes {
		t.Errorf("Quota mismatch: got %+v, want bytes=%d", got.Quota, quotaBytes)
	}

	if err := env.Buckets.Delete(ctx, bucketName, bucketName, actor, sourceIP); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// A second Get should now surface a typed 404.
	if _, err := env.Buckets.Get(ctx, bucketName); err == nil {
		t.Fatalf("Get after Delete: expected error, got nil")
	}
}
