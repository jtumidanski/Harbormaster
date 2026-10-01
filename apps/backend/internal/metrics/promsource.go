package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/prometheus/prom2json"
)

// promQueries maps each tracked family to the PromQL instant query that
// yields its cluster-wide value from RustFS's rustfs_* series (spec §6.3).
// Counters stay cumulative: Aggregate derives rates at read time, exactly
// as it does for values scraped from MinIO.
//
// Label values confirmed against the cluster Prometheus scrape of RustFS
// 1.0.0: status_class lives on the request-duration histogram count (the
// plain requests counter only carries method); drive runtime state is a
// per-state gauge. The family list is the contract.
//
// The offline count counts every drive not in the "online" state, so it also
// picks up "returning", "suspect", and "unknown" — not just "offline".
var promQueries = map[string]string{
	"objectstore_s3_requests_total":                   `sum(rustfs_http_server_requests_total)`,
	"objectstore_s3_requests_4xx_errors_total":        `sum(rustfs_http_server_request_duration_seconds_count{status_class="4xx"})`,
	"objectstore_s3_requests_5xx_errors_total":        `sum(rustfs_http_server_request_duration_seconds_count{status_class="5xx"})`,
	"objectstore_s3_traffic_received_bytes":           `sum(rustfs_http_server_request_body_bytes_total)`,
	"objectstore_s3_traffic_sent_bytes":               `sum(rustfs_http_server_response_body_bytes_total)`,
	"objectstore_cluster_capacity_usable_total_bytes": `sum(rustfs_cluster_drive_total_bytes)`,
	"objectstore_cluster_capacity_usable_free_bytes":  `sum(rustfs_cluster_drive_free_bytes)`,
	"objectstore_cluster_drive_online_total":          `sum(rustfs_cluster_drive_runtime_state{state="online"})`,
	"objectstore_cluster_drive_offline_total":         `sum(rustfs_cluster_drive_runtime_state{state!="online"})`,
}

// clusterFamilies are served by ClusterMetrics; the rest by ResourceMetrics.
// The split only mirrors the madmin client's two calls; Collect concatenates.
var clusterFamilies = map[string]bool{
	"objectstore_cluster_capacity_usable_total_bytes": true,
	"objectstore_cluster_capacity_usable_free_bytes":  true,
	"objectstore_cluster_drive_online_total":          true,
	"objectstore_cluster_drive_offline_total":         true,
}

// PrometheusSource is a MetricsSource that reads from a Prometheus HTTP
// API instead of the target's /minio/v2/metrics endpoint. Used for RustFS,
// which pushes OTLP metrics to Prometheus and exposes no scrape endpoint.
type PrometheusSource struct {
	base   string
	client *http.Client
}

// NewPrometheusSource binds to baseURL (no trailing slash, e.g.
// http://prometheus:9090). A nil client gets a 5s-timeout default.
func NewPrometheusSource(baseURL string, client *http.Client) *PrometheusSource {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &PrometheusSource{base: strings.TrimRight(baseURL, "/"), client: client}
}

// ClusterMetrics returns the capacity and drive families.
func (p *PrometheusSource) ClusterMetrics(ctx context.Context) ([]*prom2json.Family, error) {
	return p.families(ctx, true)
}

// ResourceMetrics returns the request and traffic families.
func (p *PrometheusSource) ResourceMetrics(ctx context.Context) ([]*prom2json.Family, error) {
	return p.families(ctx, false)
}

func (p *PrometheusSource) families(ctx context.Context, cluster bool) ([]*prom2json.Family, error) {
	var out []*prom2json.Family
	for name, q := range promQueries {
		if clusterFamilies[name] != cluster {
			continue
		}
		v, ok, err := p.query(ctx, q)
		if err != nil {
			return nil, fmt.Errorf("prometheus %s: %w", name, err)
		}
		if !ok {
			continue
		}
		typ := "GAUGE"
		if _, isCounter := counterMetrics[name]; isCounter {
			typ = "COUNTER"
		}
		out = append(out, &prom2json.Family{
			Name:    name,
			Type:    typ,
			Metrics: []any{prom2json.Metric{Value: v}},
		})
	}
	return out, nil
}

type promResponse struct {
	Status string `json:"status"`
	Data   struct {
		Result []struct {
			Value []any `json:"value"` // [unix_ts, "string value"]
		} `json:"result"`
	} `json:"data"`
}

// query runs one instant query. ok is false for an empty vector.
func (p *PrometheusSource) query(ctx context.Context, q string) (value string, ok bool, err error) {
	u := p.base + "/api/v1/query?query=" + url.QueryEscape(q)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", false, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", false, err
	}
	var pr promResponse
	if err := json.Unmarshal(body, &pr); err != nil {
		return "", false, err
	}
	if pr.Status != "success" {
		return "", false, fmt.Errorf("status %q", pr.Status)
	}
	if len(pr.Data.Result) == 0 || len(pr.Data.Result[0].Value) != 2 {
		return "", false, nil
	}
	s, isStr := pr.Data.Result[0].Value[1].(string)
	if !isStr {
		return "", false, fmt.Errorf("unexpected value type %T", pr.Data.Result[0].Value[1])
	}
	return s, true, nil
}

var _ MetricsSource = (*PrometheusSource)(nil)
