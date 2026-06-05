package metrics

import (
	"context"
	"fmt"
	"strconv"

	"github.com/prometheus/prom2json"
)

// trackedMetrics is the set of Prometheus family names the dashboard stores,
// mapped to nothing (presence = tracked). Names verified against MinIO's
// cluster/resource subsystems (design §5.1); confirmed in the integration
// test. Keep this list as the single source of truth for the series.
var trackedMetrics = map[string]struct{}{
	"minio_s3_requests_total":            {},
	"minio_s3_requests_4xx_errors_total": {},
	"minio_s3_requests_5xx_errors_total": {},
	"minio_s3_traffic_received_bytes":    {},
	"minio_s3_traffic_sent_bytes":        {},
	"minio_cluster_capacity_usable_total_bytes": {},
	"minio_cluster_capacity_usable_free_bytes":  {},
	"minio_cluster_drive_online_total":          {},
	"minio_cluster_drive_offline_total":         {},
}

// counterMetrics is the subset of trackedMetrics that are counters (rates
// derived at query time). Everything else is a gauge (passed through).
var counterMetrics = map[string]struct{}{
	"minio_s3_requests_total":            {},
	"minio_s3_requests_4xx_errors_total": {},
	"minio_s3_requests_5xx_errors_total": {},
	"minio_s3_traffic_received_bytes":    {},
	"minio_s3_traffic_sent_bytes":        {},
}

// MetricsSource is the minimal client the collector needs (lets tests stub
// the madmin MetricsClient).
type MetricsSource interface {
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
		if _, ok := trackedMetrics[fam.Name]; !ok {
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
		out[fam.Name] = sum
	}
	return out
}
