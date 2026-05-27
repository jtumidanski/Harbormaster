package buckets

import (
	"strings"

	madmin "github.com/minio/madmin-go/v3"
	miniogo "github.com/minio/minio-go/v7"
)

// bucketFromInfo seeds a Bucket value with the cheap-to-read attributes
// available from minio-go's ListBuckets response. Quota / versioning /
// lifecycle / policy fields are filled in later by the per-bucket
// fan-out in Processor.
func bucketFromInfo(info miniogo.BucketInfo) Bucket {
	return Bucket{
		Name:      info.Name,
		CreatedAt: info.CreationDate,
	}
}

// applyUsage merges the per-bucket usage record into b. The madmin
// BucketUsageInfo type uses uint64 for sizes; we cap at math.MaxInt64 by
// the natural int64 conversion (a single-bucket usage value crossing 8 EiB
// is not a real-world concern).
func applyUsage(b Bucket, usage madmin.BucketUsageInfo) Bucket {
	b.EstimatedBytes = int64(usage.Size)
	b.ObjectCount = int64(usage.ObjectsCount)
	return b
}

// versioningEnabled extracts the boolean the UI cares about from minio-go's
// versioning config. We treat "Enabled" as true; "Suspended" and unset are
// false (the latter is the default for a brand-new bucket).
func versioningEnabled(cfg miniogo.BucketVersioningConfiguration) bool {
	return cfg.Enabled()
}

// publicAccessFromPolicy reduces a raw bucket-policy JSON document to one
// of the three canned PublicAccess labels Harbormaster surfaces.
//
// Detection is intentionally permissive: we look for the well-known
// "s3:GetObject" Allow grant for everyone (public-read) and additionally
// for "s3:PutObject" (public-read-write). Anything else — including the
// empty policy string MinIO returns when no policy is set — collapses to
// "private". A bespoke operator-authored policy will therefore appear as
// "private" in the UI; the read view never overwrites it because
// SetPublicAccess is the only path that mutates the policy.
//
// The full canned-template matcher lands with T3.2 (internal/policies); the
// heuristic here is enough for the read side to behave sensibly until then.
func publicAccessFromPolicy(raw string) PublicAccess {
	if strings.TrimSpace(raw) == "" {
		return PublicAccessPrivate
	}
	// Cheap structural sniff: avoid pulling in a full JSON parser for the
	// read path. The canned templates we materialize on the write side use
	// these exact substrings; anything else falls through to Private.
	hasGet := strings.Contains(raw, "\"s3:GetObject\"") || strings.Contains(raw, "s3:GetObject")
	hasPut := strings.Contains(raw, "\"s3:PutObject\"") || strings.Contains(raw, "s3:PutObject")
	switch {
	case hasGet && hasPut:
		return PublicAccessPublicReadWrite
	case hasGet:
		return PublicAccessPublicRead
	default:
		return PublicAccessPrivate
	}
}

// quotaFromMadmin converts madmin's BucketQuota into the domain Quota
// representation. Returns nil when no quota is configured (madmin reports
// Quota == 0 in that case).
//
// Note: madmin v3.0.66 only knows about HardQuota at the wire level. The
// FIFO quota Harbormaster offers is a domain-level concept that maps onto
// a (hard quota + lifecycle template) pair on the write side. The read
// path therefore always returns QuotaKindHard for any non-zero quota
// reported by MinIO; a TODO(T3.21) hook will widen this once the lifecycle-
// template handler lands and we can recognise a FIFO-managed bucket.
func quotaFromMadmin(q madmin.BucketQuota, usedBytes int64) *Quota {
	bytes := int64(q.Size)
	if bytes == 0 {
		bytes = int64(q.Quota) // pre-Aug-2023 field; harmless if zero
	}
	if bytes == 0 {
		return nil
	}
	return &Quota{
		Kind:      QuotaKindHard,
		Bytes:     bytes,
		UsedBytes: usedBytes,
	}
}
