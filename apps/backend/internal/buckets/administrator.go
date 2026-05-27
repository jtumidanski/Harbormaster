package buckets

import (
	"context"
	"fmt"

	madmin "github.com/minio/madmin-go/v3"
	miniogo "github.com/minio/minio-go/v7"
)

// applyVersioning toggles versioning on bucket. Enabled=true sends
// {Status:"Enabled"}; false sends {Status:"Suspended"} (MinIO does not
// permit a transition back to the never-versioned state once enabled, and
// "Suspended" is the closest the protocol offers).
func applyVersioning(ctx context.Context, s3 s3API, bucket string, enabled bool) error {
	status := "Suspended"
	if enabled {
		status = "Enabled"
	}
	cfg := miniogo.BucketVersioningConfiguration{Status: status}
	if err := s3.SetBucketVersioning(ctx, bucket, cfg); err != nil {
		return fmt.Errorf("buckets.applyVersioning: %w", err)
	}
	return nil
}

// applyQuota writes the operator-supplied quota to MinIO via madmin. A FIFO
// quota is currently materialised as a HardQuota on the wire — the FIFO
// eviction loop lands with the lifecycle-template handler in T3.21; until
// then we still record the hard ceiling so MinIO refuses writes once full.
//
// Pass bytes == 0 to clear the quota.
func applyQuota(ctx context.Context, adm adminAPI, bucket string, kind QuotaKind, bytes int64) error {
	q := &madmin.BucketQuota{Size: uint64(bytes)}
	if bytes > 0 {
		// madmin v3.0.66 only ships HardQuota at the wire level; FIFO
		// is implemented at the lifecycle layer (TODO(T3.21)).
		_ = kind
		q.Type = madmin.HardQuota
	}
	if err := adm.SetBucketQuota(ctx, bucket, q); err != nil {
		return fmt.Errorf("buckets.applyQuota: %w", err)
	}
	return nil
}

// applyPolicy writes a canned policy JSON, or removes the policy when
// policy is the empty string.
//
// TODO(T3.2): once internal/policies.BucketPolicyFor exists, the processor
// will call this helper with one of the three canned templates. For T3.1
// the function is wired but processor.SetPublicAccess deliberately returns
// a 501 envelope so we never hit it without the canned templates in place.
func applyPolicy(ctx context.Context, s3 s3API, bucket, policy string) error {
	if policy == "" {
		// minio-go has no explicit DeleteBucketPolicy; SetBucketPolicy
		// with an empty string is the documented way to remove it.
		if err := s3.SetBucketPolicy(ctx, bucket, ""); err != nil {
			return fmt.Errorf("buckets.applyPolicy.remove: %w", err)
		}
		return nil
	}
	if err := s3.SetBucketPolicy(ctx, bucket, policy); err != nil {
		return fmt.Errorf("buckets.applyPolicy: %w", err)
	}
	return nil
}
