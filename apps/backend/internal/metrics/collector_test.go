package metrics

import (
	"testing"

	"github.com/prometheus/prom2json"
)

func TestFlattenFamilies_SumsTrackedMetrics(t *testing.T) {
	families := []*prom2json.Family{
		{
			Name: "minio_s3_requests_total",
			Type: "COUNTER",
			Metrics: []interface{}{
				prom2json.Metric{Labels: map[string]string{"api": "GetObject"}, Value: "100"},
				prom2json.Metric{Labels: map[string]string{"api": "PutObject"}, Value: "25"},
			},
		},
		{
			Name:    "some_untracked_metric",
			Type:    "GAUGE",
			Metrics: []interface{}{prom2json.Metric{Value: "999"}},
		},
	}
	got := flattenFamilies(families)
	if got["minio_s3_requests_total"] != 125 {
		t.Errorf("expected summed 125, got %v", got["minio_s3_requests_total"])
	}
	if _, ok := got["some_untracked_metric"]; ok {
		t.Error("untracked metric must be dropped")
	}
}
