package metrics

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/prometheus/prom2json"
)

// trackedMetrics is the set of series names the dashboard stores, mapped to
// nothing (presence = tracked); the RustFS PromQL behind each lives in
// promsource.go. Keep this list as the single source of truth for the series.
var trackedMetrics = map[string]struct{}{
	"objectstore_s3_requests_total":                   {},
	"objectstore_s3_requests_4xx_errors_total":        {},
	"objectstore_s3_requests_5xx_errors_total":        {},
	"objectstore_s3_traffic_received_bytes":           {},
	"objectstore_s3_traffic_sent_bytes":               {},
	"objectstore_cluster_capacity_usable_total_bytes": {},
	"objectstore_cluster_capacity_usable_free_bytes":  {},
	"objectstore_cluster_drive_online_total":          {},
	"objectstore_cluster_drive_offline_total":         {},
}

// counterMetrics is the subset of trackedMetrics that are counters (rates
// derived at query time). Everything else is a gauge (passed through).
var counterMetrics = map[string]struct{}{
	"objectstore_s3_requests_total":            {},
	"objectstore_s3_requests_4xx_errors_total": {},
	"objectstore_s3_requests_5xx_errors_total": {},
	"objectstore_s3_traffic_received_bytes":    {},
	"objectstore_s3_traffic_sent_bytes":        {},
}

// MetricsSource is the minimal client the collector needs (lets tests stub
// the madmin MetricsClient).
type MetricsSource interface { //nolint:revive // stutter intentional: MetricsSource is the stable cross-package name (E4/E6/E10)
	ClusterMetrics(ctx context.Context) ([]*prom2json.Family, error)
	ResourceMetrics(ctx context.Context) ([]*prom2json.Family, error)
}

// SourceGetter resolves a fresh MetricsSource per poll (rebuilt when the
// pool's credentials change).
type SourceGetter func(ctx context.Context) (MetricsSource, error)

// Collector scrapes tracked metrics into a flat (metric → value) map.
type Collector struct {
	getSource SourceGetter
}

// NewCollector returns a Collector bound to a source getter.
func NewCollector(g SourceGetter) *Collector { return &Collector{getSource: g} }

// Collect scrapes cluster + resource metrics and returns the flattened,
// tracked-only values.
func (c *Collector) Collect(ctx context.Context) (map[string]float64, error) {
	src, err := c.getSource(ctx)
	if err != nil {
		return nil, fmt.Errorf("metrics.Collect getSource: %w", err)
	}
	cluster, err := src.ClusterMetrics(ctx)
	if err != nil {
		return nil, fmt.Errorf("metrics.Collect cluster: %w", err)
	}
	resource, err := src.ResourceMetrics(ctx)
	if err != nil {
		return nil, fmt.Errorf("metrics.Collect resource: %w", err)
	}
	all := append(append([]*prom2json.Family{}, cluster...), resource...)
	return flattenFamilies(all), nil
}

// flattenFamilies sums each tracked family's Metric values into a single
// value (cluster-wide aggregate per logical series). Non-Metric elements
// (histograms/summaries) and untracked families are skipped.
func flattenFamilies(families []*prom2json.Family) map[string]float64 {
	out := map[string]float64{}
	for _, fam := range families {
		if fam == nil {
			continue
		}
		name := seriesName(fam.Name)
		if _, ok := trackedMetrics[name]; !ok {
			continue
		}
		var sum float64
		for _, el := range fam.Metrics {
			m, ok := el.(prom2json.Metric)
			if !ok {
				continue
			}
			v, err := strconv.ParseFloat(m.Value, 64)
			if err != nil {
				continue
			}
			sum += v
		}
		out[name] = sum
	}
	return out
}

// scrapedFamilyPrefix is the family-name prefix MinIO's own Prometheus
// endpoint uses. The madmin scrape path (no Prometheus URL configured)
// returns families under it; seriesName maps them onto the vendor-neutral
// series names in trackedMetrics. PromQL-sourced families already carry the
// series names and pass through unchanged.
const scrapedFamilyPrefix = "minio_"

// seriesName maps a scraped Prometheus family name to its series name.
func seriesName(family string) string {
	if rest, ok := strings.CutPrefix(family, scrapedFamilyPrefix); ok {
		return "objectstore_" + rest
	}
	return family
}
