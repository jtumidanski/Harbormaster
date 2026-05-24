package buckets

import (
	"context"
	"errors"
	"sync"
	"testing"

	madmin "github.com/minio/madmin-go/v3"
	miniogo "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/lifecycle"
)

// White-box test package: the bucket-domain interfaces (adminAPI, s3API,
// ClientGetter) are intentionally unexported so callers outside the
// package have to go through the HTTP layer, but the unit tests need to
// substitute hand-rolled fakes for both clients. Living in package
// buckets gives us direct access to those types without leaking a
// test-only constructor into the public surface.

// stubAdmin is a hand-rolled adminAPI fake.
type stubAdmin struct {
	mu sync.Mutex

	usageInfo map[string]madmin.BucketUsageInfo
	quota     map[string]madmin.BucketQuota
	usageErr  error
	quotaErr  error

	setQuotaCalls []setQuotaCall
	setQuotaErr   error
}

type setQuotaCall struct {
	Bucket string
	Quota  madmin.BucketQuota
}

func (s *stubAdmin) BucketUsageInfo(_ context.Context, bucket string) (madmin.BucketUsageInfo, error) {
	if s.usageErr != nil {
		return madmin.BucketUsageInfo{}, s.usageErr
	}
	return s.usageInfo[bucket], nil
}

func (s *stubAdmin) GetBucketQuota(_ context.Context, bucket string) (madmin.BucketQuota, error) {
	if s.quotaErr != nil {
		return madmin.BucketQuota{}, s.quotaErr
	}
	return s.quota[bucket], nil
}

func (s *stubAdmin) SetBucketQuota(_ context.Context, bucket string, q *madmin.BucketQuota) error {
	if s.setQuotaErr != nil {
		return s.setQuotaErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setQuotaCalls = append(s.setQuotaCalls, setQuotaCall{Bucket: bucket, Quota: *q})
	return nil
}

// stubS3 is a hand-rolled s3API fake.
type stubS3 struct {
	mu sync.Mutex

	buckets       []miniogo.BucketInfo
	listErr       error
	versioning    map[string]miniogo.BucketVersioningConfiguration
	versioningErr error
	policy        map[string]string
	policyErr     error
	lifecycle     map[string]*lifecycle.Configuration
	lifecycleErr  error

	// ListObjects controls
	listObjectsReturn map[string][]miniogo.ObjectInfo

	// Side-effect tracking
	makeCalls          []string
	makeErr            error
	removeCalls        []string
	removeErr          error
	setPolicyCalls     []setPolicyCall
	setPolicyErr       error
	setVersioningCalls []setVersioningCall
	setVersioningErr   error
}

type setPolicyCall struct {
	Bucket string
	Policy string
}
type setVersioningCall struct {
	Bucket string
	Config miniogo.BucketVersioningConfiguration
}

func (s *stubS3) ListBuckets(_ context.Context) ([]miniogo.BucketInfo, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.buckets, nil
}

func (s *stubS3) MakeBucket(_ context.Context, bucket string, _ miniogo.MakeBucketOptions) error {
	if s.makeErr != nil {
		return s.makeErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.makeCalls = append(s.makeCalls, bucket)
	return nil
}

func (s *stubS3) RemoveBucket(_ context.Context, bucket string) error {
	if s.removeErr != nil {
		return s.removeErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeCalls = append(s.removeCalls, bucket)
	return nil
}

func (s *stubS3) GetBucketPolicy(_ context.Context, bucket string) (string, error) {
	if s.policyErr != nil {
		return "", s.policyErr
	}
	return s.policy[bucket], nil
}

func (s *stubS3) SetBucketPolicy(_ context.Context, bucket, policy string) error {
	if s.setPolicyErr != nil {
		return s.setPolicyErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setPolicyCalls = append(s.setPolicyCalls, setPolicyCall{Bucket: bucket, Policy: policy})
	return nil
}

func (s *stubS3) GetBucketVersioning(_ context.Context, bucket string) (miniogo.BucketVersioningConfiguration, error) {
	if s.versioningErr != nil {
		return miniogo.BucketVersioningConfiguration{}, s.versioningErr
	}
	return s.versioning[bucket], nil
}

func (s *stubS3) SetBucketVersioning(_ context.Context, bucket string, cfg miniogo.BucketVersioningConfiguration) error {
	if s.setVersioningErr != nil {
		return s.setVersioningErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setVersioningCalls = append(s.setVersioningCalls, setVersioningCall{Bucket: bucket, Config: cfg})
	return nil
}

func (s *stubS3) GetBucketLifecycle(_ context.Context, bucket string) (*lifecycle.Configuration, error) {
	if s.lifecycleErr != nil {
		return nil, s.lifecycleErr
	}
	cfg, ok := s.lifecycle[bucket]
	if !ok {
		return nil, errors.New("no lifecycle")
	}
	return cfg, nil
}

func (s *stubS3) ListObjects(_ context.Context, bucket string, _ miniogo.ListObjectsOptions) <-chan miniogo.ObjectInfo {
	ch := make(chan miniogo.ObjectInfo, len(s.listObjectsReturn[bucket])+1)
	go func() {
		defer close(ch)
		for _, obj := range s.listObjectsReturn[bucket] {
			ch <- obj
		}
	}()
	return ch
}

// newTestProcessor wires a Processor against in-memory stubs and returns
// both halves so tests can configure stub responses and inspect side
// effects. Pass nil receivers to take zero-value fakes.
func newTestProcessor(t *testing.T, adm *stubAdmin, s3 *stubS3) (*Processor, *stubAdmin, *stubS3) {
	t.Helper()
	if adm == nil {
		adm = &stubAdmin{
			usageInfo: map[string]madmin.BucketUsageInfo{},
			quota:     map[string]madmin.BucketQuota{},
		}
	}
	if s3 == nil {
		s3 = &stubS3{
			versioning:        map[string]miniogo.BucketVersioningConfiguration{},
			policy:            map[string]string{},
			lifecycle:         map[string]*lifecycle.Configuration{},
			listObjectsReturn: map[string][]miniogo.ObjectInfo{},
		}
	}
	getter := func(_ context.Context) (adminAPI, s3API, error) {
		return adm, s3, nil
	}
	return NewProcessor(getter), adm, s3
}
