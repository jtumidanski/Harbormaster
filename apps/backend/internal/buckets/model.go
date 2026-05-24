// Package buckets owns the bucket-domain view of MinIO: listing, creating,
// inspecting, and mutating buckets via the configured admin + S3 clients.
//
// Unlike most Harbormaster domains, buckets are NOT persisted in the local
// SQLite — the source of truth is MinIO itself. The Bucket model in this
// package is assembled from madmin (BucketUsageInfo, BucketQuota) and
// minio-go (ListBuckets, BucketVersioningConfiguration, BucketPolicy,
// LifecycleConfiguration) responses at read time.
package buckets

import "time"

// Bucket is the immutable read view of a single MinIO bucket combined with
// the auxiliary settings the UI surfaces (versioning, lifecycle presence,
// canned public-access mode, and the operator-set quota).
type Bucket struct {
	Name              string
	CreatedAt         time.Time
	EstimatedBytes    int64
	ObjectCount       int64
	VersioningEnabled bool
	HasLifecycleRules bool
	PublicAccess      PublicAccess
	Quota             *Quota
}

// PublicAccess is the canned bucket-policy mode rendered for operators.
// MinIO's underlying BucketPolicy JSON is reduced to one of these labels by
// matching against the three canned templates Harbormaster supports.
type PublicAccess string

const (
	PublicAccessPrivate         PublicAccess = "private"
	PublicAccessPublicRead      PublicAccess = "public-read"
	PublicAccessPublicReadWrite PublicAccess = "public-read-write"
)

// Quota describes an operator-set bucket quota. Bytes is the configured
// ceiling; UsedBytes is the most-recently reported consumption (sourced from
// the BucketUsageInfo entry, not the quota record itself).
type Quota struct {
	Kind      QuotaKind // "hard" | "fifo"
	Bytes     int64
	UsedBytes int64
}

// QuotaKind discriminates the two operator-facing quota behaviours:
//   - hard: writes are rejected once UsedBytes >= Bytes (native MinIO quota).
//   - fifo: oldest objects are deleted to keep UsedBytes < Bytes. The FIFO
//     mode requires versioning to be disabled because the eviction loop
//     would otherwise pile up delete markers indefinitely.
type QuotaKind string

const (
	QuotaKindHard QuotaKind = "hard"
	QuotaKindFifo QuotaKind = "fifo"
)
