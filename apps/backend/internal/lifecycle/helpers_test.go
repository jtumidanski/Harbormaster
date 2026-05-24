package lifecycle

import (
	"context"
	"errors"
	"sync"
	"testing"

	mlifecycle "github.com/minio/minio-go/v7/pkg/lifecycle"
)

// White-box test package: the lifecycle s3API interface is unexported
// so callers outside the package have to go through the HTTP layer,
// but unit tests need to substitute hand-rolled fakes. Living in
// package lifecycle gives us direct access to those types without
// leaking a test-only constructor into the public surface.

// stubS3 is a hand-rolled s3API fake. Per-method controls (return
// values, captured arguments, forced errors) follow the buckets /
// objects packages' conventions so cross-package navigation is cheap.
type stubS3 struct {
	mu sync.Mutex

	// GetBucketLifecycle controls. getCfg is what to return on success;
	// getErr forces an error from the call (e.g. errNoSuchLifecycle to
	// simulate a bucket with no lifecycle config attached). When
	// getReturnNil is true the helper returns (nil, nil) — that
	// matches how some early MinIO builds answer the GET when no
	// config is set.
	getCfg       *mlifecycle.Configuration
	getErr       error
	getReturnNil bool
	getCalls     []string

	// SetBucketLifecycle controls. setErr forces an error; setCalls
	// captures the (bucket, deep-copied config) pairs every call sent.
	// The deep copy lets a test mutate cfg between calls without
	// corrupting earlier captures.
	setErr   error
	setCalls []setCall
}

type setCall struct {
	Bucket string
	Config *mlifecycle.Configuration
}

func (s *stubS3) GetBucketLifecycle(_ context.Context, bucket string) (*mlifecycle.Configuration, error) {
	s.mu.Lock()
	s.getCalls = append(s.getCalls, bucket)
	s.mu.Unlock()
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.getReturnNil {
		return nil, nil
	}
	return s.getCfg, nil
}

func (s *stubS3) SetBucketLifecycle(_ context.Context, bucket string, cfg *mlifecycle.Configuration) error {
	s.mu.Lock()
	// Take a shallow copy of the slice so a caller mutating cfg.Rules
	// after the call doesn't retroactively rewrite our captured row.
	captured := &mlifecycle.Configuration{Rules: make([]mlifecycle.Rule, len(cfg.Rules))}
	copy(captured.Rules, cfg.Rules)
	s.setCalls = append(s.setCalls, setCall{Bucket: bucket, Config: captured})
	s.mu.Unlock()
	return s.setErr
}

// newTestProcessor wires a Processor against an in-memory stub and
// returns both halves so tests can configure stub responses and
// inspect side effects. Pass nil to take a zero-value fake.
func newTestProcessor(t *testing.T, s3 *stubS3) (*Processor, *stubS3) {
	t.Helper()
	if s3 == nil {
		s3 = &stubS3{}
	}
	getter := func(_ context.Context) (s3API, error) { return s3, nil }
	return NewProcessor(getter), s3
}

// errNoSuchLifecycle simulates the MinIO error code returned when a
// bucket has no lifecycle configuration attached. We match the
// substring (NoSuchLifecycleConfiguration) the processor's helper
// looks for so the read-path "treat as empty" branch fires.
var errNoSuchLifecycle = errors.New("api error NoSuchLifecycleConfiguration: The lifecycle configuration does not exist")

// errFailing is a generic error sentinel used in negative-path tests.
var errFailing = errors.New("stubS3 forced failure")

// _ = errFailing keeps the symbol live even when no negative tests
// use it; it's a convenient hook for future tests.
var _ = errFailing
