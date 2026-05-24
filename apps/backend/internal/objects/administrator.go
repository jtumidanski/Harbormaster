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

// statObject reads the object metadata in isolation. The proxy-mode
// download path uses this to set Content-Length / Content-Type headers
// before opening the body stream.
func statObject(ctx context.Context, s3 s3API, bucket, key string) (miniogo.ObjectInfo, error) {
	info, err := s3.StatObject(ctx, bucket, key, miniogo.StatObjectOptions{})
	if err != nil {
		return miniogo.ObjectInfo{}, fmt.Errorf("objects.statObject: %w", err)
	}
	return info, nil
}

// getObject opens a streaming reader against the object body. The
// caller owns the returned ReadCloser and must Close it. The proxy-mode
// download handler is the only call site today.
func getObject(ctx context.Context, s3 s3API, bucket, key string) (io.ReadCloser, error) {
	rc, err := s3.GetObject(ctx, bucket, key, miniogo.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("objects.getObject: %w", err)
	}
	return rc, nil
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
