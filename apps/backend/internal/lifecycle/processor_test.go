package lifecycle

import (
	"context"
	"errors"
	"net/http"
	"testing"

	mlifecycle "github.com/minio/minio-go/v7/pkg/lifecycle"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
)

// TestProcessorList_NoLifecycleConfig confirms the read path silently
// treats the typed "NoSuchLifecycleConfiguration" error MinIO returns
// for a bucket with no lifecycle as an empty rule set; the SPA renders
// "no rules" without an error banner.
func TestProcessorList_NoLifecycleConfig(t *testing.T) {
	t.Parallel()
	p, _ := newTestProcessor(t, &stubS3{getErr: errNoSuchLifecycle})
	rules, err := p.List(context.Background(), "b")
	if err != nil {
		t.Fatalf("List(): unexpected err %v", err)
	}
	if len(rules) != 0 {
		t.Errorf("rules = %v; want empty", rules)
	}
}

// TestProcessorList_NilConfig covers the alternative "no config"
// answer some MinIO builds give: a nil *Configuration with a nil
// error. Same expected outcome — empty rule slice, nil error.
func TestProcessorList_NilConfig(t *testing.T) {
	t.Parallel()
	p, _ := newTestProcessor(t, &stubS3{getReturnNil: true})
	rules, err := p.List(context.Background(), "b")
	if err != nil {
		t.Fatalf("List(): unexpected err %v", err)
	}
	if len(rules) != 0 {
		t.Errorf("rules = %v; want empty", rules)
	}
}

// TestProcessorList_MixedRules feeds both a managed-shape rule and
// an unmanaged rule through the read path and asserts the classifier
// keys produce the right per-rule Managed flag without re-running the
// classifier's full test grid.
func TestProcessorList_MixedRules(t *testing.T) {
	t.Parallel()
	cfg := mlifecycle.NewConfiguration()
	cfg.Rules = []mlifecycle.Rule{
		{
			ID:     "harbormaster-expire-7d",
			Status: "Enabled",
			Expiration: mlifecycle.Expiration{
				Days: mlifecycle.ExpirationDays(7),
			},
		},
		{
			ID:     "rule-from-mc-abc",
			Status: "Enabled",
			Transition: mlifecycle.Transition{
				Days:         mlifecycle.ExpirationDays(30),
				StorageClass: "GLACIER",
			},
		},
	}
	p, _ := newTestProcessor(t, &stubS3{getCfg: cfg})
	rules, err := p.List(context.Background(), "b")
	if err != nil {
		t.Fatalf("List(): unexpected err %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("len(rules) = %d; want 2", len(rules))
	}
	if !rules[0].Managed {
		t.Errorf("rules[0].Managed = false; want true (managed shape)")
	}
	if rules[1].Managed {
		t.Errorf("rules[1].Managed = true; want false (transition present)")
	}
}

// TestProcessorCreate_MergesIntoExisting asserts the read-modify-write
// contract: a Create call must preserve every pre-existing rule and
// append (or replace) the deterministic-ID rule generated from
// (days, prefix). The SetBucketLifecycle round-trip is the only place
// the merged config is observable from outside the processor.
func TestProcessorCreate_MergesIntoExisting(t *testing.T) {
	t.Parallel()
	existing := mlifecycle.NewConfiguration()
	existing.Rules = []mlifecycle.Rule{
		{ID: "pre-existing-rule", Status: "Enabled"},
	}
	p, s := newTestProcessor(t, &stubS3{getCfg: existing})

	rule, err := p.Create(context.Background(), "b", 14, "uploads/", "", "")
	if err != nil {
		t.Fatalf("Create(): unexpected err %v", err)
	}
	if !rule.Managed || rule.Days != 14 || rule.Prefix != "uploads/" {
		t.Errorf("returned rule = %#v; want managed/14d/uploads/", rule)
	}
	if rule.ID != "harbormaster-expire-14d-uploads" {
		t.Errorf("rule.ID = %q; want harbormaster-expire-14d-uploads", rule.ID)
	}
	if len(s.setCalls) != 1 {
		t.Fatalf("SetBucketLifecycle calls = %d; want 1", len(s.setCalls))
	}
	sent := s.setCalls[0].Config.Rules
	if len(sent) != 2 {
		t.Fatalf("merged rules = %d; want 2 (pre-existing + new). Got: %#v", len(sent), sent)
	}
	if sent[0].ID != "pre-existing-rule" {
		t.Errorf("merged rules[0].ID = %q; pre-existing rule was lost", sent[0].ID)
	}
	if sent[1].ID != "harbormaster-expire-14d-uploads" {
		t.Errorf("merged rules[1].ID = %q; want harbormaster-expire-14d-uploads", sent[1].ID)
	}
}

// TestProcessorCreate_ReplacesByID confirms a second Create with the
// same (days, prefix) is idempotent at the rule-list level: instead
// of growing the slice with a duplicate, it overwrites the existing
// row. This matters because the SPA's "create" affordance must not
// produce silent duplicates if the operator double-clicks.
func TestProcessorCreate_ReplacesByID(t *testing.T) {
	t.Parallel()
	existing := mlifecycle.NewConfiguration()
	existing.Rules = []mlifecycle.Rule{
		{
			ID:     "harbormaster-expire-7d",
			Status: "Enabled",
			Expiration: mlifecycle.Expiration{
				Days: mlifecycle.ExpirationDays(7),
			},
		},
	}
	p, s := newTestProcessor(t, &stubS3{getCfg: existing})
	if _, err := p.Create(context.Background(), "b", 7, "", "", ""); err != nil {
		t.Fatalf("Create(): unexpected err %v", err)
	}
	if len(s.setCalls) != 1 {
		t.Fatalf("SetBucketLifecycle calls = %d; want 1", len(s.setCalls))
	}
	sent := s.setCalls[0].Config.Rules
	if len(sent) != 1 {
		t.Fatalf("merged rules = %d; want 1 (in-place replace, not append). Got: %#v", len(sent), sent)
	}
}

// TestProcessorCreate_NoExistingConfig confirms a Create against a
// bucket that has no lifecycle config bootstraps a fresh
// Configuration containing just the new rule. The NoSuchLifecycle
// error from GetBucketLifecycle must NOT block the write.
func TestProcessorCreate_NoExistingConfig(t *testing.T) {
	t.Parallel()
	p, s := newTestProcessor(t, &stubS3{getErr: errNoSuchLifecycle})
	if _, err := p.Create(context.Background(), "b", 5, "", "", ""); err != nil {
		t.Fatalf("Create(): unexpected err %v", err)
	}
	if len(s.setCalls) != 1 {
		t.Fatalf("SetBucketLifecycle calls = %d; want 1", len(s.setCalls))
	}
	sent := s.setCalls[0].Config.Rules
	if len(sent) != 1 || sent[0].ID != "harbormaster-expire-5d" {
		t.Errorf("rules = %#v; want a single harbormaster-expire-5d row", sent)
	}
}

// TestProcessorCreate_InvalidDays surfaces the operator-facing
// validation: days must be > 0. The error must be the typed 422
// envelope so the SPA can attach the message to the form field.
func TestProcessorCreate_InvalidDays(t *testing.T) {
	t.Parallel()
	p, s := newTestProcessor(t, nil)
	_, err := p.Create(context.Background(), "b", 0, "", "", "")
	if err == nil {
		t.Fatal("Create(days=0): want error; got nil")
	}
	var ae *apierror.Error
	if !errors.As(err, &ae) {
		t.Fatalf("Create(days=0): want *apierror.Error; got %T", err)
	}
	if ae.HTTPStatus != http.StatusUnprocessableEntity {
		t.Errorf("status = %d; want 422", ae.HTTPStatus)
	}
	if len(s.setCalls) != 0 {
		t.Errorf("SetBucketLifecycle was called %d times; want 0 on validation failure", len(s.setCalls))
	}
}

// TestProcessorDelete_RemovesByID confirms the rule with the given ID
// is dropped from the merged config that goes back to MinIO; every
// other rule is preserved.
func TestProcessorDelete_RemovesByID(t *testing.T) {
	t.Parallel()
	cfg := mlifecycle.NewConfiguration()
	cfg.Rules = []mlifecycle.Rule{
		{ID: "keep-me", Status: "Enabled"},
		{ID: "drop-me", Status: "Enabled"},
	}
	p, s := newTestProcessor(t, &stubS3{getCfg: cfg})
	if err := p.Delete(context.Background(), "b", "drop-me", "", ""); err != nil {
		t.Fatalf("Delete(): unexpected err %v", err)
	}
	if len(s.setCalls) != 1 {
		t.Fatalf("SetBucketLifecycle calls = %d; want 1", len(s.setCalls))
	}
	sent := s.setCalls[0].Config.Rules
	if len(sent) != 1 || sent[0].ID != "keep-me" {
		t.Errorf("post-delete rules = %#v; want [keep-me]", sent)
	}
}

// TestProcessorDelete_MissingRuleIsNoop locks in the "double-DELETE
// is harmless" contract: when the requested rule ID is not present,
// the processor returns nil and skips the SetBucketLifecycle round-
// trip so the operation stays cheap.
func TestProcessorDelete_MissingRuleIsNoop(t *testing.T) {
	t.Parallel()
	cfg := mlifecycle.NewConfiguration()
	cfg.Rules = []mlifecycle.Rule{
		{ID: "keep-me", Status: "Enabled"},
	}
	p, s := newTestProcessor(t, &stubS3{getCfg: cfg})
	if err := p.Delete(context.Background(), "b", "ghost", "", ""); err != nil {
		t.Fatalf("Delete(): unexpected err %v", err)
	}
	if len(s.setCalls) != 0 {
		t.Errorf("SetBucketLifecycle called %d times; want 0 (no-op delete)", len(s.setCalls))
	}
}

// TestProcessorDelete_NoLifecycleConfig confirms a Delete against a
// bucket with no lifecycle config is also a no-op (no SetBucketLifecycle
// call, no error surfaced). Mirrors the "double-delete is harmless"
// stance even when the predecessor lifecycle config never existed.
func TestProcessorDelete_NoLifecycleConfig(t *testing.T) {
	t.Parallel()
	p, s := newTestProcessor(t, &stubS3{getErr: errNoSuchLifecycle})
	if err := p.Delete(context.Background(), "b", "anything", "", ""); err != nil {
		t.Fatalf("Delete(): unexpected err %v", err)
	}
	if len(s.setCalls) != 0 {
		t.Errorf("SetBucketLifecycle called %d times; want 0", len(s.setCalls))
	}
}

// TestCreateNoncurrentBuildsRule asserts that CreateNoncurrent generates
// the deterministic minio rule shape for a noncurrent-version-expiration
// rule: the returned Rule has the right Kind/NoncurrentDays/NewerNoncurrentVersions
// values and the NoncurrentVersionExpiration config that lands in MinIO
// is correct.
func TestCreateNoncurrentBuildsRule(t *testing.T) {
	t.Parallel()
	p, s := newTestProcessor(t, &stubS3{getErr: errNoSuchLifecycle})
	rule, err := p.CreateNoncurrent(context.Background(), "b", 30, 3, "uploads/", "actor", "ip")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rule.Kind != KindNoncurrentExpiration {
		t.Errorf("rule.Kind = %q; want %q", rule.Kind, KindNoncurrentExpiration)
	}
	if rule.NoncurrentDays != 30 {
		t.Errorf("rule.NoncurrentDays = %d; want 30", rule.NoncurrentDays)
	}
	if rule.NewerNoncurrentVersions != 3 {
		t.Errorf("rule.NewerNoncurrentVersions = %d; want 3", rule.NewerNoncurrentVersions)
	}
	if len(s.setCalls) != 1 {
		t.Fatalf("SetBucketLifecycle calls = %d; want 1", len(s.setCalls))
	}
	saved := s.setCalls[0].Config.Rules[0]
	if int(saved.NoncurrentVersionExpiration.NoncurrentDays) != 30 {
		t.Errorf("minio rule noncurrent days = %d; want 30", int(saved.NoncurrentVersionExpiration.NoncurrentDays))
	}
	if saved.NoncurrentVersionExpiration.NewerNoncurrentVersions != 3 {
		t.Errorf("minio rule newer_noncurrent_versions = %d; want 3", saved.NoncurrentVersionExpiration.NewerNoncurrentVersions)
	}
}

// TestCreateNoncurrentMergesIntoExisting confirms the read-modify-write
// contract: an existing rule is preserved and the new noncurrent rule is
// appended.
func TestCreateNoncurrentMergesIntoExisting(t *testing.T) {
	t.Parallel()
	existing := mlifecycle.NewConfiguration()
	existing.Rules = []mlifecycle.Rule{
		{ID: "pre-existing-rule", Status: "Enabled"},
	}
	p, s := newTestProcessor(t, &stubS3{getCfg: existing})
	if _, err := p.CreateNoncurrent(context.Background(), "b", 10, 0, "", "actor", "ip"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s.setCalls) != 1 {
		t.Fatalf("SetBucketLifecycle calls = %d; want 1", len(s.setCalls))
	}
	sent := s.setCalls[0].Config.Rules
	if len(sent) != 2 {
		t.Fatalf("merged rules = %d; want 2 (pre-existing + new). Got: %#v", len(sent), sent)
	}
	if sent[0].ID != "pre-existing-rule" {
		t.Errorf("pre-existing rule was lost; rules[0].ID = %q", sent[0].ID)
	}
}

// TestCreateNoncurrentInvalidDays confirms validation returns a 422 error
// when noncurrent_days <= 0 and does not call SetBucketLifecycle.
func TestCreateNoncurrentInvalidDays(t *testing.T) {
	t.Parallel()
	p, s := newTestProcessor(t, nil)
	_, err := p.CreateNoncurrent(context.Background(), "b", 0, 0, "", "actor", "ip")
	if err == nil {
		t.Fatal("want error for noncurrent_days=0; got nil")
	}
	var ae *apierror.Error
	if !errors.As(err, &ae) {
		t.Fatalf("want *apierror.Error; got %T", err)
	}
	if ae.HTTPStatus != http.StatusUnprocessableEntity {
		t.Errorf("status = %d; want 422", ae.HTTPStatus)
	}
	if len(s.setCalls) != 0 {
		t.Errorf("SetBucketLifecycle called %d times; want 0 on validation failure", len(s.setCalls))
	}
}

// TestCreateNoncurrentInvalidNewerNoncurrent confirms that when noncurrent_days
// is valid but newer_noncurrent_versions is negative, the returned *apierror.Error
// points at /data/attributes/newer_noncurrent_versions — not at noncurrent_days —
// so the SPA highlights the correct form field.
func TestCreateNoncurrentInvalidNewerNoncurrent(t *testing.T) {
	t.Parallel()
	p, s := newTestProcessor(t, nil)
	_, err := p.CreateNoncurrent(context.Background(), "b", 30, -1, "", "actor", "ip")
	if err == nil {
		t.Fatal("want error for newer_noncurrent_versions=-1; got nil")
	}
	var ae *apierror.Error
	if !errors.As(err, &ae) {
		t.Fatalf("want *apierror.Error; got %T", err)
	}
	if ae.HTTPStatus != http.StatusUnprocessableEntity {
		t.Errorf("status = %d; want 422", ae.HTTPStatus)
	}
	const wantPointer = "/data/attributes/newer_noncurrent_versions"
	if ae.Pointer != wantPointer {
		t.Errorf("pointer = %q; want %q", ae.Pointer, wantPointer)
	}
	if len(s.setCalls) != 0 {
		t.Errorf("SetBucketLifecycle called %d times; want 0 on validation failure", len(s.setCalls))
	}
}

// TestCreateAbortMPUBuildsRule asserts that CreateAbortMPU generates the
// correct minio AbortIncompleteMultipartUpload rule shape and returns a
// Rule with Kind == KindAbortIncompleteMPU and the right DaysAfterInitiation.
func TestCreateAbortMPUBuildsRule(t *testing.T) {
	t.Parallel()
	p, s := newTestProcessor(t, &stubS3{getErr: errNoSuchLifecycle})
	rule, err := p.CreateAbortMPU(context.Background(), "b", 7, "", "actor", "ip")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rule.Kind != KindAbortIncompleteMPU {
		t.Errorf("rule.Kind = %q; want %q", rule.Kind, KindAbortIncompleteMPU)
	}
	if rule.DaysAfterInitiation != 7 {
		t.Errorf("rule.DaysAfterInitiation = %d; want 7", rule.DaysAfterInitiation)
	}
	if len(s.setCalls) != 1 {
		t.Fatalf("SetBucketLifecycle calls = %d; want 1", len(s.setCalls))
	}
	saved := s.setCalls[0].Config.Rules[0]
	if int(saved.AbortIncompleteMultipartUpload.DaysAfterInitiation) != 7 {
		t.Errorf("minio rule days_after_initiation = %d; want 7",
			int(saved.AbortIncompleteMultipartUpload.DaysAfterInitiation))
	}
}

// TestCreateAbortMPUMergesIntoExisting confirms the read-modify-write
// contract for abort-mpu: an existing rule is preserved.
func TestCreateAbortMPUMergesIntoExisting(t *testing.T) {
	t.Parallel()
	existing := mlifecycle.NewConfiguration()
	existing.Rules = []mlifecycle.Rule{
		{ID: "pre-existing-rule", Status: "Enabled"},
	}
	p, s := newTestProcessor(t, &stubS3{getCfg: existing})
	if _, err := p.CreateAbortMPU(context.Background(), "b", 14, "imgs/", "actor", "ip"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s.setCalls) != 1 {
		t.Fatalf("SetBucketLifecycle calls = %d; want 1", len(s.setCalls))
	}
	sent := s.setCalls[0].Config.Rules
	if len(sent) != 2 {
		t.Fatalf("merged rules = %d; want 2 (pre-existing + new). Got: %#v", len(sent), sent)
	}
	if sent[0].ID != "pre-existing-rule" {
		t.Errorf("pre-existing rule was lost; rules[0].ID = %q", sent[0].ID)
	}
}

// TestCreateAbortMPUInvalidDays confirms validation returns a 422 error
// when days_after_initiation <= 0 and does not call SetBucketLifecycle.
func TestCreateAbortMPUInvalidDays(t *testing.T) {
	t.Parallel()
	p, s := newTestProcessor(t, nil)
	_, err := p.CreateAbortMPU(context.Background(), "b", 0, "", "actor", "ip")
	if err == nil {
		t.Fatal("want error for days_after_initiation=0; got nil")
	}
	var ae *apierror.Error
	if !errors.As(err, &ae) {
		t.Fatalf("want *apierror.Error; got %T", err)
	}
	if ae.HTTPStatus != http.StatusUnprocessableEntity {
		t.Errorf("status = %d; want 422", ae.HTTPStatus)
	}
	if len(s.setCalls) != 0 {
		t.Errorf("SetBucketLifecycle called %d times; want 0 on validation failure", len(s.setCalls))
	}
}
