package metrics

import (
	"testing"

	"github.com/prometheus/prom2json"
)

func TestFlattenFamilies_SumsTrackedMetrics(t *testing.T) {
	families := []*prom2json.Family{
		{
			Name: "objectstore_s3_requests_total",
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
	if got["objectstore_s3_requests_total"] != 125 {
		t.Errorf("expected summed 125, got %v", got["objectstore_s3_requests_total"])
	}
	if _, ok := got["some_untracked_metric"]; ok {
		t.Error("untracked metric must be dropped")
	}
}

// TestFlattenFamilies_MapsScrapedFamilyNamesToSeriesNames verifies that the
// madmin scrape path's upstream family names land under the vendor-neutral
// series names the store and REST layer use.
func TestFlattenFamilies_MapsScrapedFamilyNamesToSeriesNames(t *testing.T) {
	families := []*prom2json.Family{
		{
			Name:    scrapedFamilyPrefix + "s3_requests_total",
			Type:    "COUNTER",
			Metrics: []interface{}{prom2json.Metric{Value: "7"}},
		},
		{
			Name:    scrapedFamilyPrefix + "node_untracked",
			Type:    "GAUGE",
			Metrics: []interface{}{prom2json.Metric{Value: "1"}},
		},
	}
	got := flattenFamilies(families)
	if got["objectstore_s3_requests_total"] != 7 {
		t.Errorf("expected scraped family under objectstore_s3_requests_total = 7, got %v", got)
	}
	if len(got) != 1 {
		t.Errorf("expected exactly one tracked series, got %v", got)
	}
}
