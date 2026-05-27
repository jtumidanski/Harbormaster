package policies

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// stubPolicyAdmin records every AddCannedPolicy invocation so tests can
// assert on the (name, body) tuples that hit the wire.
type stubPolicyAdmin struct {
	mu    sync.Mutex
	calls []addCannedCall
	err   error
}

type addCannedCall struct {
	Name string
	Body []byte
}

func (s *stubPolicyAdmin) AddCannedPolicy(_ context.Context, name string, body []byte) error {
	if s.err != nil {
		return s.err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]byte, len(body))
	copy(cp, body)
	s.calls = append(s.calls, addCannedCall{Name: name, Body: cp})
	return nil
}

func newMat() (*Materializer, *stubPolicyAdmin) {
	adm := &stubPolicyAdmin{}
	m := &Materializer{
		Admin: func(_ context.Context) (PolicyAdmin, error) { return adm, nil },
	}
	return m, adm
}

// TestEnsurePolicyIdempotent — two EnsurePolicy calls for the same
// template/params produce two AddCannedPolicy invocations, but each
// invocation carries the same canonical name. MinIO's AddCannedPolicy
// upserts so the on-cluster record count stays at one.
func TestEnsurePolicyIdempotent(t *testing.T) {
	m, adm := newMat()
	ctx := context.Background()

	n1, err := m.EnsurePolicy(ctx, "read-only", nil)
	if err != nil {
		t.Fatalf("first EnsurePolicy: %v", err)
	}
	n2, err := m.EnsurePolicy(ctx, "read-only", nil)
	if err != nil {
		t.Fatalf("second EnsurePolicy: %v", err)
	}
	if n1 != n2 {
		t.Errorf("non-stable name: %q vs %q", n1, n2)
	}
	if n1 != "harbormaster-read-only" {
		t.Errorf("unexpected name: %q", n1)
	}
	if len(adm.calls) != 2 {
		t.Errorf("expected 2 AddCannedPolicy calls (upsert is fine), got %d", len(adm.calls))
	}
	// Both calls must reference the same canonical name.
	for i, c := range adm.calls {
		if c.Name != n1 {
			t.Errorf("call[%d] name = %q, want %q", i, c.Name, n1)
		}
	}
}

// TestEnsurePolicyBackupTargetSameBucket — repeating the same
// (template, bucket) produces a single canonical name. Two AddCannedPolicy
// invocations are fine (each is an upsert).
func TestEnsurePolicyBackupTargetSameBucket(t *testing.T) {
	m, adm := newMat()
	ctx := context.Background()

	n1, err := m.EnsurePolicy(ctx, "backup-target", map[string]string{"bucket": "ledger"})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	n2, err := m.EnsurePolicy(ctx, "backup-target", map[string]string{"bucket": "ledger"})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if n1 != n2 {
		t.Errorf("same-bucket calls returned different names: %q vs %q", n1, n2)
	}
	// Distinct policy names on the cluster: just one.
	names := map[string]struct{}{}
	for _, c := range adm.calls {
		names[c.Name] = struct{}{}
	}
	if len(names) != 1 {
		t.Errorf("expected 1 distinct canonical name, got %d: %v", len(names), names)
	}
}

// TestEnsurePolicyBackupTargetDifferentBuckets — distinct buckets must
// produce distinct canonical names so per-bucket policies can coexist on
// the cluster.
func TestEnsurePolicyBackupTargetDifferentBuckets(t *testing.T) {
	m, adm := newMat()
	ctx := context.Background()

	if _, err := m.EnsurePolicy(ctx, "backup-target", map[string]string{"bucket": "ledger"}); err != nil {
		t.Fatalf("ledger: %v", err)
	}
	if _, err := m.EnsurePolicy(ctx, "backup-target", map[string]string{"bucket": "photos"}); err != nil {
		t.Fatalf("photos: %v", err)
	}
	names := map[string]struct{}{}
	for _, c := range adm.calls {
		names[c.Name] = struct{}{}
	}
	if len(names) != 2 {
		t.Errorf("expected 2 distinct canonical names, got %d: %v", len(names), names)
	}
}

// TestEnsurePolicyUnknownTemplate — unknown name surfaces as the sentinel
// so the REST layer can return a typed envelope.
func TestEnsurePolicyUnknownTemplate(t *testing.T) {
	m, adm := newMat()
	_, err := m.EnsurePolicy(context.Background(), "administrator", nil)
	if !errors.Is(err, ErrUnknownTemplate) {
		t.Fatalf("expected ErrUnknownTemplate, got %v", err)
	}
	if len(adm.calls) != 0 {
		t.Errorf("AddCannedPolicy invoked for unknown template: %+v", adm.calls)
	}
}

// TestEnsurePolicyBackupTargetMissingBucket — Render's missing-param error
// is surfaced verbatim, AddCannedPolicy is not called.
func TestEnsurePolicyBackupTargetMissingBucket(t *testing.T) {
	m, adm := newMat()
	_, err := m.EnsurePolicy(context.Background(), "backup-target", nil)
	if err == nil {
		t.Fatal("expected error for missing bucket param, got nil")
	}
	if len(adm.calls) != 0 {
		t.Errorf("AddCannedPolicy invoked despite Render failure: %+v", adm.calls)
	}
}
