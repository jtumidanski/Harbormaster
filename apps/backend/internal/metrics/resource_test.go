package metrics

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func newTestRouter(p *Processor) *chi.Mux {
	r := chi.NewRouter()
	Routes(p)(r)
	return r
}

// TestMetricsView_PopulatedStore asserts that a recent sample yields 200 with
// collected:true and non-empty series.
func TestMetricsView_PopulatedStore(t *testing.T) {
	st := newTestStore(t)
	p := NewProcessor(st, time.Minute)

	// Insert two samples close to now so isFresh returns true.
	now := time.Now().UTC()
	values := map[string]float64{
		"minio_s3_requests_total":                   100,
		"minio_cluster_capacity_usable_total_bytes": 1000,
	}
	require.NoError(t, st.Insert(context.Background(), now.Add(-30*time.Second), values))
	require.NoError(t, st.Insert(context.Background(), now.Add(-10*time.Second), map[string]float64{
		"minio_s3_requests_total":                   110,
		"minio_cluster_capacity_usable_total_bytes": 1000,
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics?window=1h", nil)
	newTestRouter(p).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var resp viewResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	require.Equal(t, "1h", resp.Window)
	require.True(t, resp.Collected, "expected collected:true for recent data")
	require.NotEmpty(t, resp.Series, "expected non-empty series")
}

// TestMetricsView_InvalidWindow asserts that an unknown window returns 422
// with code invalid_metrics_window.
func TestMetricsView_InvalidWindow(t *testing.T) {
	st := newTestStore(t)
	p := NewProcessor(st, time.Minute)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics?window=30d", nil)
	newTestRouter(p).ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	var doc struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&doc))
	require.Equal(t, "invalid_metrics_window", doc.Error.Code)
}

// TestMetricsView_EmptyStore asserts that an empty store returns 200 with
// collected:false.
func TestMetricsView_EmptyStore(t *testing.T) {
	st := newTestStore(t)
	p := NewProcessor(st, time.Minute)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics?window=1h", nil)
	newTestRouter(p).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp viewResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	require.False(t, resp.Collected, "expected collected:false for empty store")
}
