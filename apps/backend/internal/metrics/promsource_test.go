package metrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/prometheus/prom2json"
)

// fakeProm answers /api/v1/query with a scalar per known query and an
// empty vector otherwise.
func fakeProm(t *testing.T, answers map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		if v, ok := answers[q]; ok {
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1758700000,"` + v + `"]}]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
}

func familyValue(fams []*prom2json.Family, name string) (string, bool) {
	for _, f := range fams {
		if f.Name == name && len(f.Metrics) == 1 {
			return f.Metrics[0].(prom2json.Metric).Value, true
		}
	}
	return "", false
}

func TestPrometheusSource_MapsTrackedFamilies(t *testing.T) {
	srv := fakeProm(t, map[string]string{
		promQueries["minio_s3_requests_total"]:                   "1500",
		promQueries["minio_s3_requests_5xx_errors_total"]:        "3",
		promQueries["minio_cluster_capacity_usable_total_bytes"]: "107374182400",
		promQueries["minio_cluster_drive_online_total"]:          "1",
	})
	defer srv.Close()

	src := NewPrometheusSource(srv.URL, srv.Client())
	cluster, err := src.ClusterMetrics(context.Background())
	if err != nil {
		t.Fatalf("ClusterMetrics: %v", err)
	}
	resource, err := src.ResourceMetrics(context.Background())
	if err != nil {
		t.Fatalf("ResourceMetrics: %v", err)
	}
	all := append(cluster, resource...)

	for name, want := range map[string]string{
		"minio_s3_requests_total":                   "1500",
		"minio_s3_requests_5xx_errors_total":        "3",
		"minio_cluster_capacity_usable_total_bytes": "107374182400",
		"minio_cluster_drive_online_total":          "1",
	} {
		got, ok := familyValue(all, name)
		if !ok || got != want {
			t.Errorf("%s: want %q, got %q (present=%v)", name, want, got, ok)
		}
	}
	// A family with an empty vector is omitted, not emitted as zero.
	if _, ok := familyValue(all, "minio_s3_requests_4xx_errors_total"); ok {
		t.Error("empty vector must not produce a family")
	}
	// Every emitted family is one the collector tracks.
	for _, f := range all {
		if _, ok := trackedMetrics[f.Name]; !ok {
			t.Errorf("emitted untracked family %s", f.Name)
		}
	}
}

func TestPrometheusSource_FlattensThroughCollector(t *testing.T) {
	srv := fakeProm(t, map[string]string{promQueries["minio_s3_requests_total"]: "42"})
	defer srv.Close()
	src := NewPrometheusSource(srv.URL, srv.Client())
	c := NewCollector(func(ctx context.Context) (MetricsSource, error) { return src, nil })
	got, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got["minio_s3_requests_total"] != 42 {
		t.Errorf("want 42, got %v", got["minio_s3_requests_total"])
	}
}

func TestPrometheusSource_ErrorOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(502) }))
	defer srv.Close()
	src := NewPrometheusSource(srv.URL, srv.Client())
	if _, err := src.ClusterMetrics(context.Background()); err == nil {
		t.Error("want error on 502")
	}
}

func TestPromQueries_CoverEveryTrackedFamily(t *testing.T) {
	for name := range trackedMetrics {
		q, ok := promQueries[name]
		if !ok || strings.TrimSpace(q) == "" {
			t.Errorf("no query for tracked family %s", name)
		}
		if _, err := url.ParseQuery("query=" + url.QueryEscape(q)); err != nil {
			t.Errorf("query for %s does not encode: %v", name, err)
		}
	}
}
