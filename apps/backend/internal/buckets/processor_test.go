package buckets

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	miniogo "github.com/minio/minio-go/v7"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
)

// TestSetQuotaRejectsFifoWithVersioning verifies the cross-domain
// invariant: a FIFO quota requires versioning to be off. The stub s3API
// reports versioning as Enabled; the call must return the typed
// fifo_requires_versioning_off envelope without invoking SetBucketQuota.
func TestSetQuotaRejectsFifoWithVersioning(t *testing.T) {
	p, adm, s3 := newTestProcessor(t, nil, nil)
	s3.versioning["photos"] = miniogo.BucketVersioningConfiguration{Status: "Enabled"}

	err := p.SetQuota(context.Background(), "photos", QuotaKindFifo, 1<<30)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ae *apierror.Error
	if !errors.As(err, &ae) {
		t.Fatalf("expected *apierror.Error, got %T: %v", err, err)
	}
	if ae.Code != "fifo_requires_versioning_off" {
		t.Errorf("wrong code: got %q want fifo_requires_versioning_off", ae.Code)
	}
	if ae.HTTPStatus != http.StatusUnprocessableEntity {
		t.Errorf("wrong status: got %d want %d", ae.HTTPStatus, http.StatusUnprocessableEntity)
	}
	if len(adm.setQuotaCalls) != 0 {
		t.Errorf("SetBucketQuota was called despite validation failure: %+v", adm.setQuotaCalls)
	}
}

// TestSetQuotaPersistsHardQuota verifies the happy path for the hard
// quota branch: the call should reach SetBucketQuota with the
// HardQuota type and the operator-supplied byte ceiling.
func TestSetQuotaPersistsHardQuota(t *testing.T) {
	p, adm, _ := newTestProcessor(t, nil, nil)

	const want = int64(5 * 1 << 30)
	if err := p.SetQuota(context.Background(), "photos", QuotaKindHard, want); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(adm.setQuotaCalls) != 1 {
		t.Fatalf("expected one SetBucketQuota call, got %d", len(adm.setQuotaCalls))
	}
	c := adm.setQuotaCalls[0]
	if c.Bucket != "photos" {
		t.Errorf("wrong bucket: %q", c.Bucket)
	}
	if int64(c.Quota.Size) != want {
		t.Errorf("wrong size: got %d want %d", c.Quota.Size, want)
	}
}

// TestDeleteReturnsConflictOnNonEmpty verifies the emptiness re-check:
// when ListObjects yields any entry, Delete must return apierror(409,
// "bucket_not_empty") without invoking RemoveBucket.
func TestDeleteReturnsConflictOnNonEmpty(t *testing.T) {
	p, _, s3 := newTestProcessor(t, nil, nil)
	s3.listObjectsReturn["photos"] = []miniogo.ObjectInfo{
		{Key: "ignored.jpg"},
	}

	err := p.Delete(context.Background(), "photos", "photos")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ae *apierror.Error
	if !errors.As(err, &ae) {
		t.Fatalf("expected *apierror.Error, got %T: %v", err, err)
	}
	if ae.Code != "bucket_not_empty" {
		t.Errorf("wrong code: got %q want bucket_not_empty", ae.Code)
	}
	if ae.HTTPStatus != http.StatusConflict {
		t.Errorf("wrong status: got %d want %d", ae.HTTPStatus, http.StatusConflict)
	}
	if len(s3.removeCalls) != 0 {
		t.Errorf("RemoveBucket was called for a non-empty bucket: %+v", s3.removeCalls)
	}
}

// TestDeleteRequiresConfirmName: the destructive-action guard.
func TestDeleteRequiresConfirmName(t *testing.T) {
	p, _, s3 := newTestProcessor(t, nil, nil)

	err := p.Delete(context.Background(), "photos", "wrong")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ae *apierror.Error
	if !errors.As(err, &ae) || ae.Code != "confirm_name_mismatch" {
		t.Fatalf("expected confirm_name_mismatch, got %v", err)
	}
	if len(s3.removeCalls) != 0 {
		t.Errorf("RemoveBucket should not have been called")
	}
}

// TestDeleteSucceedsOnEmptyBucket: end-to-end happy path through the
// emptiness check and into RemoveBucket.
func TestDeleteSucceedsOnEmptyBucket(t *testing.T) {
	p, _, s3 := newTestProcessor(t, nil, nil)

	if err := p.Delete(context.Background(), "photos", "photos"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := s3.removeCalls; len(got) != 1 || got[0] != "photos" {
		t.Errorf("RemoveBucket calls: got %+v want [photos]", got)
	}
}

// TestCreateAppliesOptionalSettings verifies that MakeBucket runs and the
// versioning + quota side-effects fire when the corresponding opts are
// set. Public-access is left empty because T3.1 returns 501 for that.
func TestCreateAppliesOptionalSettings(t *testing.T) {
	p, adm, s3 := newTestProcessor(t, nil, nil)
	s3.buckets = []miniogo.BucketInfo{
		{Name: "photos", CreationDate: time.Unix(1700000000, 0).UTC()},
	}

	const want = int64(2 << 30)
	_, err := p.Create(context.Background(), "photos", CreateOpts{
		VersioningEnabled: true,
		Quota:             &Quota{Kind: QuotaKindHard, Bytes: want},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := s3.makeCalls; len(got) != 1 || got[0] != "photos" {
		t.Errorf("MakeBucket calls: got %+v want [photos]", got)
	}
	if len(s3.setVersioningCalls) != 1 || s3.setVersioningCalls[0].Config.Status != "Enabled" {
		t.Errorf("versioning not enabled: %+v", s3.setVersioningCalls)
	}
	if len(adm.setQuotaCalls) != 1 || int64(adm.setQuotaCalls[0].Quota.Size) != want {
		t.Errorf("quota not applied: %+v", adm.setQuotaCalls)
	}
}

// TestCreatePublicAccessIsNotYetImplemented documents the T3.1 boundary:
// public-access modes other than "private" return 501 until T3.2 lands.
func TestCreatePublicAccessIsNotYetImplemented(t *testing.T) {
	p, _, s3 := newTestProcessor(t, nil, nil)

	_, err := p.Create(context.Background(), "photos", CreateOpts{
		PublicAccess: PublicAccessPublicRead,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ae *apierror.Error
	if !errors.As(err, &ae) || ae.HTTPStatus != http.StatusNotImplemented {
		t.Fatalf("expected 501 not_implemented, got %v", err)
	}
	if len(s3.makeCalls) != 1 {
		t.Errorf("MakeBucket should still have run before the policy step: %+v", s3.makeCalls)
	}
}

// TestSetPublicAccessIsNotYetImplemented documents the T3.1 boundary for
// the action endpoint.
func TestSetPublicAccessIsNotYetImplemented(t *testing.T) {
	p, _, _ := newTestProcessor(t, nil, nil)

	err := p.SetPublicAccess(context.Background(), "photos", "public-read", "photos")
	if err == nil {
		t.Fatal("expected error")
	}
	var ae *apierror.Error
	if !errors.As(err, &ae) || ae.HTTPStatus != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %v", err)
	}
}

// TestSetVersioningHappyPath asserts the action endpoint flips the
// versioning status without touching other state.
func TestSetVersioningHappyPath(t *testing.T) {
	p, _, s3 := newTestProcessor(t, nil, nil)

	if err := p.SetVersioning(context.Background(), "photos", true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s3.setVersioningCalls) != 1 {
		t.Fatalf("expected one SetBucketVersioning call, got %d", len(s3.setVersioningCalls))
	}
	if s3.setVersioningCalls[0].Config.Status != "Enabled" {
		t.Errorf("wrong status: %q", s3.setVersioningCalls[0].Config.Status)
	}
}
