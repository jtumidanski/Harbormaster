//go:build integration

package integration

import (
	"testing"

	miniogo "github.com/minio/minio-go/v7"
)

// TestLifecycle_RoundTrip drives the lifecycle processor end-to-end:
// Create (expiration, 30 days, no prefix) → List (returns one managed
// rule) → Delete by ID → List (empty).
func TestLifecycle_RoundTrip(t *testing.T) {
	env, ctx := setup(t)

	const (
		bucketName = "harbormaster-it-lifecycle"
		actor      = "integration-test"
		sourceIP   = "127.0.0.1"
	)

	if err := env.MC.MakeBucket(ctx, bucketName, miniogo.MakeBucketOptions{}); err != nil {
		t.Fatalf("MakeBucket: %v", err)
	}
	t.Cleanup(func() { _ = env.MC.RemoveBucket(ctx, bucketName) })

	rule, err := env.Lifecycle.Create(ctx, bucketName, 30, "", actor, sourceIP)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !rule.Managed {
		t.Errorf("Create returned Managed=false, want true")
	}
	if rule.Days != 30 {
		t.Errorf("Create returned Days=%d, want 30", rule.Days)
	}
	if rule.Kind != "expiration" {
		t.Errorf("Create returned Kind=%q, want %q", rule.Kind, "expiration")
	}

	listed, err := env.Lifecycle.List(ctx, bucketName)
	if err != nil {
		t.Fatalf("List after Create: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("List after Create: got %d rules, want 1; rules=%+v", len(listed), listed)
	}
	if listed[0].ID != rule.ID {
		t.Errorf("List after Create returned ID=%q, want %q", listed[0].ID, rule.ID)
	}

	if err := env.Lifecycle.Delete(ctx, bucketName, rule.ID, actor, sourceIP); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	listed, err = env.Lifecycle.List(ctx, bucketName)
	if err != nil {
		t.Fatalf("List after Delete: %v", err)
	}
	if len(listed) != 0 {
		t.Errorf("List after Delete: got %d rules, want 0; rules=%+v", len(listed), listed)
	}
}
