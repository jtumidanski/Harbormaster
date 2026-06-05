package objects

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	miniogo "github.com/minio/minio-go/v7"
)

// listObjects performs a single ListObjectsV2 round-trip with explicit
// pagination control. The minio-go Client.ListObjects helper returns a
// channel and hides the continuation token, which makes UI-style "page N
// of M" navigation impossible. The Core wrapper exposes the raw
// continuation-token interface we need.
//
// pageSize defaults to MinIO's server cap of 1000 when zero.
func listObjects(_ context.Context, s3 s3API, bucket, prefix, delimiter, continuationToken string, pageSize int) (miniogo.ListBucketV2Result, error) {
	res, err := s3.ListObjectsV2(bucket, prefix, "", continuationToken, delimiter, pageSize)
	if err != nil {
		return miniogo.ListBucketV2Result{}, fmt.Errorf("objects.listObjects: %w", err)
	}
	return res, nil
}

// putObject uploads body to bucket/key. size==-1 lets minio-go pick a
// part size and stream the body in multipart chunks; that's the
// expected path because the REST layer has already wrapped the body in
// http.MaxBytesReader to enforce the configured ceiling.
func putObject(ctx context.Context, s3 s3API, bucket, key string, body io.Reader, size int64, contentType string) (miniogo.UploadInfo, error) {
	opts := miniogo.PutObjectOptions{ContentType: contentType}
	info, err := s3.PutObject(ctx, bucket, key, body, size, opts)
	if err != nil {
		return miniogo.UploadInfo{}, fmt.Errorf("objects.putObject: %w", err)
	}
	return info, nil
}

// removeObject deletes a single object. The non-versioned RemoveObject
// path is sufficient for T3.9 — version-aware deletion is deferred to a
// later task once the version-listing surface lands.
func removeObject(ctx context.Context, s3 s3API, bucket, key string) error {
	if err := s3.RemoveObject(ctx, bucket, key, miniogo.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("objects.removeObject: %w", err)
	}
	return nil
}

// presignedGet mints a presigned GET URL valid for ttl. Used by both
// the direct-mode download handler (5-minute fixed TTL) and the share-
// link minting endpoint (operator-supplied TTL clamped at the
// configured ceiling).
//
// reqParams is forwarded as additional query string parameters on the
// presigned URL (e.g. response-content-disposition); pass nil when no
// extra parameters are needed.
func presignedGet(ctx context.Context, s3 s3API, bucket, key string, ttl time.Duration, reqParams url.Values) (*url.URL, error) {
	u, err := s3.PresignedGetObject(ctx, bucket, key, ttl, reqParams)
	if err != nil {
		return nil, fmt.Errorf("objects.presignedGet: %w", err)
	}
	return u, nil
}

// maxVersionScan caps how many version entries listObjectVersions will
// drain from the channel for a single key. The version browser is scoped
// to one key whose cardinality is normally tens; the cap bounds a
// pathological key and flips VersionListResult.Truncated when hit.
const maxVersionScan = 10_000

// listObjectVersions returns all versions+delete-markers for exactly key
// (the SDK's prefix listing can match siblings, so the caller filters to
// exact-key matches). The bool return is "truncated" — true when the scan
// stopped at maxVersionScan before the channel closed.
func listObjectVersions(ctx context.Context, s3 s3API, bucket, key string) ([]miniogo.ObjectInfo, bool, error) {
	infos, truncated, err := s3.ListObjectVersions(ctx, bucket, key, maxVersionScan)
	if err != nil {
		return nil, false, fmt.Errorf("objects.listObjectVersions: %w", err)
	}
	out := infos[:0]
	for _, info := range infos {
		if info.Key == key {
			out = append(out, info)
		}
	}
	return out, truncated, nil
}

// copyObjectVersion server-side copies srcVersionID of bucket/key back onto
// the same bucket/key, creating a new current version (the restore op).
func copyObjectVersion(ctx context.Context, s3 s3API, bucket, key, srcVersionID string) (miniogo.UploadInfo, error) {
	info, err := s3.CopyObject(ctx,
		miniogo.CopyDestOptions{Bucket: bucket, Object: key},
		miniogo.CopySrcOptions{Bucket: bucket, Object: key, VersionID: srcVersionID},
	)
	if err != nil {
		return miniogo.UploadInfo{}, fmt.Errorf("objects.copyObjectVersion: %w", err)
	}
	return info, nil
}

// removeObjectVersion permanently deletes a single version id of bucket/key.
func removeObjectVersion(ctx context.Context, s3 s3API, bucket, key, versionID string) error {
	if err := s3.RemoveObject(ctx, bucket, key, miniogo.RemoveObjectOptions{VersionID: versionID}); err != nil {
		return fmt.Errorf("objects.removeObjectVersion: %w", err)
	}
	return nil
}

// statObjectVersion stats a specific version.
func statObjectVersion(ctx context.Context, s3 s3API, bucket, key, versionID string) (miniogo.ObjectInfo, error) {
	info, err := s3.StatObject(ctx, bucket, key, miniogo.StatObjectOptions{VersionID: versionID})
	if err != nil {
		return miniogo.ObjectInfo{}, fmt.Errorf("objects.statObjectVersion: %w", err)
	}
	return info, nil
}

// getObjectVersion opens a reader against a specific version body.
func getObjectVersion(ctx context.Context, s3 s3API, bucket, key, versionID string) (io.ReadCloser, error) {
	rc, err := s3.GetObject(ctx, bucket, key, miniogo.GetObjectOptions{VersionID: versionID})
	if err != nil {
		return nil, fmt.Errorf("objects.getObjectVersion: %w", err)
	}
	return rc, nil
}
