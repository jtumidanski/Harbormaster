package metrics

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeCollector implements collectorIface for tests.
type fakeCollector struct {
	values map[string]float64
	err    error
}

func (f *fakeCollector) Collect(_ context.Context) (map[string]float64, error) {
	return f.values, f.err
}

func TestPollOnce_SuccessWritesSample(t *testing.T) {
	st := newTestStore(t)
	at := time.Now().UTC()
	c := &fakeCollector{values: map[string]float64{"minio_s3_requests_total": 42}}

	err := pollOnce(context.Background(), c, st, at)
	require.NoError(t, err)

	pts, err := st.Query(context.Background(), []string{"minio_s3_requests_total"}, at.Add(-time.Second))
	require.NoError(t, err)
	require.Len(t, pts["minio_s3_requests_total"], 1)
	require.InDelta(t, 42.0, pts["minio_s3_requests_total"][0].V, 0.001)
}

func TestPollOnce_ErrorWritesNothing(t *testing.T) {
	st := newTestStore(t)
	at := time.Now().UTC()
	c := &fakeCollector{err: errors.New("scrape failed")}

	err := pollOnce(context.Background(), c, st, at)
	require.Error(t, err)

	pts, err := st.Query(context.Background(), []string{"minio_s3_requests_total"}, at.Add(-time.Second))
	require.NoError(t, err)
	require.Empty(t, pts)
}
