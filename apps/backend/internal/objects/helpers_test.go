package objects

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/url"
	"sync"
	"testing"
	"time"

	miniogo "github.com/minio/minio-go/v7"
)

// White-box test package: the object-domain s3API interface is
// unexported so callers outside the package have to go through the HTTP
// layer, but unit tests need to substitute hand-rolled fakes. Living in
// package objects gives us direct access to those types without
// leaking a test-only constructor into the public surface.

// stubS3 is a hand-rolled s3API fake. Per-method controls (return
// values, captured arguments, forced errors) follow the buckets
// package's convention so cross-package navigation is cheap.
type stubS3 struct {
	mu sync.Mutex

	// listObjectsV2 controls. Pages is a map keyed by the inbound
	// continuationToken so a multi-page round-trip can be staged: the
	// first call passes "" (the initial token) and gets pages[""]; if
	// that result's NextContinuationToken == "X" the next call passes
	// "X" and gets pages["X"]. listErr forces an error from the call.
	pages   map[string]miniogo.ListBucketV2Result
	listErr error

	listCalls []listV2Call

	// PutObject controls.
	putErr     error
	putReturn  miniogo.UploadInfo
	putCalls   []putCall
	captureBuf bytes.Buffer

	// RemoveObject controls.
	removeErr   error
	removeCalls []removeCall

	// GetObject / StatObject controls.
	getBody      []byte
	getErr       error
	statReturn   miniogo.ObjectInfo
	statErr      error
	getCalls     []getCall
	statCalls    []getCall

	// PresignedGetObject controls.
	presignErr     error
	presignReturn  *url.URL
	presignCalls   []presignCall
}

type listV2Call struct {
	Bucket            string
	Prefix            string
	StartAfter        string
	ContinuationToken string
	Delimiter         string
	MaxKeys           int
}

type putCall struct {
	Bucket      string
	Key         string
	Size        int64
	ContentType string
}

type removeCall struct {
	Bucket string
	Key    string
}

type getCall struct {
	Bucket string
	Key    string
}

type presignCall struct {
	Bucket  string
	Key     string
	Expires time.Duration
	Params  url.Values
}

func (s *stubS3) ListObjectsV2(bucket, prefix, startAfter, continuationToken, delimiter string, maxKeys int) (miniogo.ListBucketV2Result, error) {
	s.mu.Lock()
	s.listCalls = append(s.listCalls, listV2Call{
		Bucket:            bucket,
		Prefix:            prefix,
		StartAfter:        startAfter,
		ContinuationToken: continuationToken,
		Delimiter:         delimiter,
		MaxKeys:           maxKeys,
	})
	s.mu.Unlock()
	if s.listErr != nil {
		return miniogo.ListBucketV2Result{}, s.listErr
	}
	if s.pages != nil {
		if res, ok := s.pages[continuationToken]; ok {
			return res, nil
		}
	}
	return miniogo.ListBucketV2Result{}, nil
}

func (s *stubS3) PutObject(_ context.Context, bucket, key string, body io.Reader, size int64, opts miniogo.PutObjectOptions) (miniogo.UploadInfo, error) {
	// Drain the body so the multipart MaxBytesReader can fire (otherwise
	// the cap check never triggers because the bytes are never read).
	// We capture the bytes into captureBuf so tests can assert content.
	s.mu.Lock()
	n, copyErr := io.Copy(&s.captureBuf, body)
	if s.putErr == nil && copyErr != nil {
		s.mu.Unlock()
		return miniogo.UploadInfo{}, copyErr
	}
	s.putCalls = append(s.putCalls, putCall{
		Bucket:      bucket,
		Key:         key,
		Size:        n,
		ContentType: opts.ContentType,
	})
	s.mu.Unlock()
	_ = size
	if s.putErr != nil {
		return miniogo.UploadInfo{}, s.putErr
	}
	// Default UploadInfo if not pre-staged.
	if s.putReturn.Key == "" {
		return miniogo.UploadInfo{
			Bucket:       bucket,
			Key:          key,
			Size:         n,
			ETag:         "stubetag",
			LastModified: time.Unix(1700000000, 0).UTC(),
		}, nil
	}
	return s.putReturn, nil
}

func (s *stubS3) RemoveObject(_ context.Context, bucket, key string, _ miniogo.RemoveObjectOptions) error {
	s.mu.Lock()
	s.removeCalls = append(s.removeCalls, removeCall{Bucket: bucket, Key: key})
	s.mu.Unlock()
	return s.removeErr
}

func (s *stubS3) GetObject(_ context.Context, bucket, key string, _ miniogo.GetObjectOptions) (io.ReadCloser, error) {
	s.mu.Lock()
	s.getCalls = append(s.getCalls, getCall{Bucket: bucket, Key: key})
	s.mu.Unlock()
	if s.getErr != nil {
		return nil, s.getErr
	}
	return io.NopCloser(bytes.NewReader(s.getBody)), nil
}

func (s *stubS3) StatObject(_ context.Context, bucket, key string, _ miniogo.StatObjectOptions) (miniogo.ObjectInfo, error) {
	s.mu.Lock()
	s.statCalls = append(s.statCalls, getCall{Bucket: bucket, Key: key})
	s.mu.Unlock()
	if s.statErr != nil {
		return miniogo.ObjectInfo{}, s.statErr
	}
	if s.statReturn.Key == "" {
		// Reasonable default: a 0-byte octet-stream entry the proxy
		// download handler can render without crashing.
		return miniogo.ObjectInfo{Key: key, Size: int64(len(s.getBody)), ContentType: "application/octet-stream"}, nil
	}
	return s.statReturn, nil
}

func (s *stubS3) PresignedGetObject(_ context.Context, bucket, key string, expires time.Duration, params url.Values) (*url.URL, error) {
	s.mu.Lock()
	s.presignCalls = append(s.presignCalls, presignCall{Bucket: bucket, Key: key, Expires: expires, Params: params})
	s.mu.Unlock()
	if s.presignErr != nil {
		return nil, s.presignErr
	}
	if s.presignReturn != nil {
		return s.presignReturn, nil
	}
	// Default presigned URL so the test surface stays trivial.
	u, _ := url.Parse("https://minio.example/" + bucket + "/" + key + "?X-Amz-Expires=" + durSeconds(expires))
	return u, nil
}

func durSeconds(d time.Duration) string {
	// Hand-rolled to avoid pulling fmt into the helpers; tests don't
	// inspect this value byte-for-byte.
	s := int64(d / time.Second)
	digits := []byte{}
	if s == 0 {
		return "0"
	}
	for s > 0 {
		digits = append([]byte{byte('0' + s%10)}, digits...)
		s /= 10
	}
	return string(digits)
}

// newTestProcessor wires a Processor against an in-memory stub and
// returns both halves so tests can configure stub responses and
// inspect side effects. Pass nil to take a zero-value fake.
func newTestProcessor(t *testing.T, s3 *stubS3, cfg ProcessorConfig) (*Processor, *stubS3) {
	t.Helper()
	if s3 == nil {
		s3 = &stubS3{}
	}
	getter := func(_ context.Context) (s3API, error) { return s3, nil }
	if cfg.UploadMaxBytes == 0 {
		cfg.UploadMaxBytes = 1 << 20
	}
	if cfg.ShareLinkMaxTTL == 0 {
		cfg.ShareLinkMaxTTL = 7 * 24 * time.Hour
	}
	if cfg.DownloadProxyMode == "" {
		cfg.DownloadProxyMode = "proxy"
	}
	return NewProcessor(getter, cfg), s3
}

// errFailing is a generic error sentinel used in negative-path tests.
var errFailing = errors.New("stubS3 forced failure")

// _ = errFailing keeps the symbol live even when no negative tests use
// it; it's a convenient hook for future tests.
var _ = errFailing
