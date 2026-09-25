# RustFS Target Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make RustFS 1.0 a fully supported Harbormaster target: proven by the integration suite, with working bucket usage and a Prometheus-backed metrics page.

**Architecture:** The integration harness grows a second container target selected by env var. A tolerant `datausageinfo` decoder in `internal/minio` replaces the madmin `DataUsageInfo` call in both adapters. A `PrometheusSource` in `internal/metrics` implements the existing `MetricsSource` interface by running PromQL instant queries and emitting the nine tracked families under their current names, so the store, aggregator, HTTP resource and frontend are untouched. `cmd/harbormaster` picks the source from `cfg.PrometheusURL`.

**Tech Stack:** Go 1.2x, madmin-go/v4, minio-go/v7, prom2json, testcontainers-go, viper, chi. RustFS 1.0.0 container for tests.

**Spec:** `k3s/docs/superpowers/specs/2026-09-24-minio-to-rustfs-migration-design.md` §6 (pointer: `docs/tasks/task-005-rustfs-target/design.md`)

## Global Constraints

- RustFS test image is `rustfs/rustfs:1.0.0`, pinned, overridable by `HARBORMASTER_RUSTFS_IMAGE` the way `HARBORMASTER_MINIO_IMAGE` works today.
- The tracked metric family names in `internal/metrics/collector.go` (`minio_s3_requests_total` and the other eight) do not change. The frontend and the SQLite schema depend on them.
- `HARBORMASTER_PROMETHEUS_URL` defaults to empty; empty means the existing madmin metrics client is used. Nothing about the MinIO path changes.
- Package and table names stay (`internal/minio`, `minio_connections`); renames are spec §8, a later task.
- All unit tests run with `go test -race -count=1 ./...` from `apps/backend`. Integration tests run with `HARBORMASTER_INTEGRATION=1 go test -race -count=1 -tags=integration ./...`.
- `go vet -tags=integration ./...` must stay clean (PR CI runs it).
- Commit style: `type(scope): summary`, as in `git log`.
- Follow the repo's `backend-dev-guidelines` skill: no interface changes without updating every mock, and `go test ./... -count=1` plus `go build ./...` run and reported before any task is called done.

---

## File map

- Modify `apps/backend/internal/integration/helper.go`: target selector, RustFS container start.
- Modify `.forgejo/workflows/nightly.yml`: matrix entry for the rustfs target.
- Create `apps/backend/internal/minio/usage.go` and `usage_test.go`: tolerant data-usage decoder.
- Modify `apps/backend/cmd/harbormaster/audit_adapter.go` and `apps/backend/internal/integration/helper.go`: use the decoder.
- Modify `apps/backend/internal/config/config.go` and `config_test.go`: `PrometheusURL`.
- Create `apps/backend/internal/metrics/promsource.go` and `promsource_test.go`.
- Modify `apps/backend/cmd/harbormaster/audit_adapter.go` and `serve.go`: source selection.
- Modify `deploy/kubernetes/deployment.yaml`, `docs/operator/configuration.md`, `docs/observability.md`, `docs/operator/security.md`.

---

### Task 1: Integration harness target selector

**Files:**
- Modify: `apps/backend/internal/integration/helper.go:52-76, 112-160`
- Modify: `.forgejo/workflows/nightly.yml:11-19, 41-47`

**Interfaces:**
- Produces: env `HARBORMASTER_IT_TARGET` (`minio` default, `rustfs`), env `HARBORMASTER_RUSTFS_IMAGE`; `setup(t)` unchanged in signature, returns a `*TestEnv` bound to whichever target runs. Task 7 runs the whole suite through this.

- [ ] **Step 1: Add the selector constants and a RustFS starter**

Replace the block from `// envMinIOImage` through `func minioImageFor()` with:

```go
// envTarget selects the object-store implementation the suite runs
// against: "minio" (default) or "rustfs". The same tests run unchanged
// against both; a call that works on one and fails on the other is a
// finding, not a flake.
const envTarget = "HARBORMASTER_IT_TARGET"

// envMinIOImage / envRustFSImage let the nightly matrix (or a local
// operator) override the pinned images without editing source.
const envMinIOImage = "HARBORMASTER_MINIO_IMAGE"
const envRustFSImage = "HARBORMASTER_RUSTFS_IMAGE"

// defaultMinIOImage is the pinned MinIO release. Pinning prevents a
// surprise CI failure when MinIO ships a backwards-incompatible
// admin-API tweak; bump deliberately and re-run the suite. The tag is
// one of the quay.io/minio/minio "RELEASE.<timestamp>" rolling tags.
const defaultMinIOImage = "quay.io/minio/minio:RELEASE.2025-09-07T16-13-09Z"

// defaultRustFSImage is the pinned RustFS release (1.0.0 GA, 2026-09-16).
const defaultRustFSImage = "rustfs/rustfs:1.0.0"

func imageFor(env, def string) string {
	if v := os.Getenv(env); v != "" {
		return v
	}
	return def
}

// target holds what setup needs from a started container.
type target struct {
	endpoint  string // host:port
	accessKey string
	secretKey string
	terminate func(context.Context, ...testcontainers.TerminateOption) error
}

func startMinIO(ctx context.Context) (target, error) {
	c, err := tcminio.Run(ctx, imageFor(envMinIOImage, defaultMinIOImage))
	if err != nil {
		return target{}, err
	}
	ep, err := c.ConnectionString(ctx)
	if err != nil {
		_ = c.Terminate(ctx)
		return target{}, err
	}
	return target{endpoint: ep, accessKey: c.Username, secretKey: c.Password, terminate: c.Terminate}, nil
}

// startRustFS runs the RustFS image with a fixed root pair and waits for
// its readiness endpoint. RustFS reads everything from env; there is no
// testcontainers module for it, so this is a GenericContainer.
func startRustFS(ctx context.Context) (target, error) {
	const user, pass = "rustfsadmin", "rustfsadmin-it-secret"
	req := testcontainers.ContainerRequest{
		Image:        imageFor(envRustFSImage, defaultRustFSImage),
		ExposedPorts: []string{"9000/tcp"},
		Env: map[string]string{
			"RUSTFS_ACCESS_KEY":     user,
			"RUSTFS_SECRET_KEY":     pass,
			"RUSTFS_VOLUMES":        "/data",
			"RUSTFS_ADDRESS":        ":9000",
			"RUSTFS_CONSOLE_ENABLE": "false",
			"RUSTFS_OBS_METRICS_EXPORT_ENABLED": "false",
		},
		WaitingFor: wait.ForHTTP("/health/ready").WithPort("9000/tcp").WithStartupTimeout(90 * time.Second),
	}
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true})
	if err != nil {
		return target{}, err
	}
	ep, err := c.Endpoint(ctx, "")
	if err != nil {
		_ = c.Terminate(ctx)
		return target{}, err
	}
	return target{endpoint: ep, accessKey: user, secretKey: pass, terminate: c.Terminate}, nil
}

func startTarget(ctx context.Context) (target, error) {
	switch os.Getenv(envTarget) {
	case "", "minio":
		return startMinIO(ctx)
	case "rustfs":
		return startRustFS(ctx)
	default:
		return target{}, fmt.Errorf("%s=%q: want minio or rustfs", envTarget, os.Getenv(envTarget))
	}
}
```

Add imports `"github.com/testcontainers/testcontainers-go"` and `"github.com/testcontainers/testcontainers-go/wait"`.

- [ ] **Step 2: Use it in setup**

Replace the container block in `setup` (from `container, err := tcminio.Run(...)` through `pool.Rebuild(...)`) with:

```go
	tgt, err := startTarget(ctx)
	if err != nil {
		t.Skipf("object-store testcontainer unavailable (Docker not reachable?): %v", err)
	}
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()
		_ = tgt.terminate(stopCtx)
	})

	rawURL := tgt.endpoint
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "http://" + rawURL
	}
	if _, err := url.Parse(rawURL); err != nil {
		t.Fatalf("invalid endpoint URL %q: %v", rawURL, err)
	}

	pool := hmminio.NewEmpty()
	if err := pool.Rebuild(hmminio.Credentials{
		EndpointURL: rawURL,
		AccessKey:   tgt.accessKey,
		SecretKey:   tgt.secretKey,
	}); err != nil {
		t.Fatalf("pool.Rebuild: %v", err)
	}
```

Update the package doc comment's first sentence to "against a real MinIO or RustFS server" and add a line: `HARBORMASTER_IT_TARGET=rustfs selects RustFS.`

- [ ] **Step 3: Tidy and compile**

Run: `cd apps/backend && go mod tidy && go vet -tags=integration ./... && go build ./...`
Expected: `testcontainers-go` moves from indirect to direct in go.mod; vet clean.

- [ ] **Step 4: Run both targets once**

Run:
```bash
cd apps/backend
HARBORMASTER_INTEGRATION=1 go test -race -count=1 -tags=integration ./internal/integration/ -run TestBuckets 2>&1 | tail -5
HARBORMASTER_INTEGRATION=1 HARBORMASTER_IT_TARGET=rustfs go test -race -count=1 -tags=integration ./internal/integration/ -run TestBuckets 2>&1 | tail -5
```
Expected: minio PASS. rustfs: the container starts and the test either passes or fails on a specific call; a failure here is Task 7's input, not a blocker for this task. A `Skipf` about the container means the image did not pull or `/health/ready` never returned 200; check `docker logs` on the container.

- [ ] **Step 5: Nightly matrix**

In `.forgejo/workflows/nightly.yml` replace the matrix and the run env with:

```yaml
      matrix:
        include:
          # MinIO floor (per docs/operator/security.md "supported floor") and
          # the rolling `latest` tag, then RustFS 1.0.0.
          - target: minio
            image: "quay.io/minio/minio:RELEASE.2025-09-07T16-13-09Z"
          - target: minio
            image: "quay.io/minio/minio:latest"
          - target: rustfs
            image: "rustfs/rustfs:1.0.0"
```
and
```yaml
        env:
          HARBORMASTER_INTEGRATION: "1"
          HARBORMASTER_IT_TARGET: ${{ matrix.target }}
          HARBORMASTER_MINIO_IMAGE: ${{ matrix.target == 'minio' && matrix.image || '' }}
          HARBORMASTER_RUSTFS_IMAGE: ${{ matrix.target == 'rustfs' && matrix.image || '' }}
```

- [ ] **Step 6: Commit**

```bash
git add apps/backend/internal/integration/helper.go apps/backend/go.mod apps/backend/go.sum .forgejo/workflows/nightly.yml
git commit -m "test(integration): HARBORMASTER_IT_TARGET selects MinIO or RustFS; nightly runs both"
```

---

### Task 2: Tolerant data-usage decoder

**Files:**
- Create: `apps/backend/internal/minio/usage.go`
- Create: `apps/backend/internal/minio/usage_test.go`
- Modify: `apps/backend/cmd/harbormaster/audit_adapter.go:72-78`
- Modify: `apps/backend/internal/integration/helper.go` (`integrationBucketAdmin.BucketUsageInfo`)

**Interfaces:**
- Produces: `func BucketUsage(ctx context.Context, adm *madmin.AdminClient, bucket string) (madmin.BucketUsageInfo, error)` and `func decodeBucketsUsage(body []byte) (map[string]madmin.BucketUsageInfo, error)` in package `minio` (import alias `hmminio`).

- [ ] **Step 1: Write the failing tests**

Create `apps/backend/internal/minio/usage_test.go`:

```go
package minio

import "testing"

const minioShape = `{
  "lastUpdate": "2026-09-24T10:00:00Z",
  "objectsCount": 12,
  "objectsTotalSize": 4096,
  "bucketsCount": 1,
  "bucketsUsage": {
    "zot": {"size": 4096, "objectsCount": 12, "objectsPendingReplicationTotalSize": 0}
  }
}`

const rustfsShape = `{
  "last_update": "2026-09-24T10:00:00Z",
  "objects_count": 7,
  "objects_total_size": 2048,
  "buckets_count": 1,
  "buckets_usage": {
    "zot": {"size": 2048, "objects_count": 7}
  }
}`

func TestDecodeBucketsUsage_MinIOCamelCase(t *testing.T) {
	got, err := decodeBucketsUsage([]byte(minioShape))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	u := got["zot"]
	if u.Size != 4096 || u.ObjectsCount != 12 {
		t.Errorf("want size 4096 / objects 12, got %+v", u)
	}
}

func TestDecodeBucketsUsage_RustFSSnakeCase(t *testing.T) {
	got, err := decodeBucketsUsage([]byte(rustfsShape))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	u := got["zot"]
	if u.Size != 2048 || u.ObjectsCount != 7 {
		t.Errorf("want size 2048 / objects 7, got %+v", u)
	}
}

func TestDecodeBucketsUsage_MissingBucketIsZero(t *testing.T) {
	got, err := decodeBucketsUsage([]byte(minioShape))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if u := got["nope"]; u.Size != 0 || u.ObjectsCount != 0 {
		t.Errorf("missing bucket must be zero value, got %+v", u)
	}
}

func TestDecodeBucketsUsage_BadJSON(t *testing.T) {
	if _, err := decodeBucketsUsage([]byte(`{`)); err == nil {
		t.Error("want error on malformed body")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd apps/backend && go test ./internal/minio/ -run TestDecodeBucketsUsage -v`
Expected: FAIL, `undefined: decodeBucketsUsage`.

- [ ] **Step 3: Implement**

Create `apps/backend/internal/minio/usage.go`:

```go
package minio

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	madmin "github.com/minio/madmin-go/v4"
)

// usageRow accepts both MinIO's camelCase and RustFS's snake_case field
// names for a bucket's usage row (rustfs/rustfs#7985). Each pair is
// merged by preferring whichever is non-zero.
type usageRow struct {
	Size          uint64 `json:"size"`
	ObjectsCount  uint64 `json:"objectsCount"`
	ObjectsCount2 uint64 `json:"objects_count"`
}

type usageBody struct {
	BucketsUsage  map[string]usageRow `json:"bucketsUsage"`
	BucketsUsage2 map[string]usageRow `json:"buckets_usage"`
}

// decodeBucketsUsage parses a datausageinfo response body from either
// server flavour into madmin rows keyed by bucket.
func decodeBucketsUsage(body []byte) (map[string]madmin.BucketUsageInfo, error) {
	var b usageBody
	if err := json.Unmarshal(body, &b); err != nil {
		return nil, fmt.Errorf("datausageinfo: decode: %w", err)
	}
	src := b.BucketsUsage
	if len(src) == 0 {
		src = b.BucketsUsage2
	}
	out := make(map[string]madmin.BucketUsageInfo, len(src))
	for name, r := range src {
		count := r.ObjectsCount
		if count == 0 {
			count = r.ObjectsCount2
		}
		out[name] = madmin.BucketUsageInfo{Size: r.Size, ObjectsCount: count}
	}
	return out, nil
}

// BucketUsage fetches the scanner's usage census through the signed admin
// client and returns the row for bucket. A bucket the scanner has not seen
// yet is the zero value with a nil error, matching the previous
// DataUsageInfo-based adapters.
func BucketUsage(ctx context.Context, adm *madmin.AdminClient, bucket string) (madmin.BucketUsageInfo, error) {
	resp, err := adm.ExecuteMethod(ctx, http.MethodGet, madmin.RequestData{RelPath: "/v3/datausageinfo"})
	if err != nil {
		return madmin.BucketUsageInfo{}, fmt.Errorf("datausageinfo: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return madmin.BucketUsageInfo{}, fmt.Errorf("datausageinfo: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return madmin.BucketUsageInfo{}, fmt.Errorf("datausageinfo: read: %w", err)
	}
	rows, err := decodeBucketsUsage(body)
	if err != nil {
		return madmin.BucketUsageInfo{}, err
	}
	return rows[bucket], nil
}
```

madmin's `RelPath` is relative to `/minio/admin`; its own `DataUsageInfo` uses `adminAPIPrefix + "/datausageinfo"` with `adminAPIPrefix = "/v3"` (madmin-go v4.10.5, `info-commands.go:254`), so this is the same URL.

- [ ] **Step 4: Run to verify pass**

Run: `cd apps/backend && go test ./internal/minio/ -run TestDecodeBucketsUsage -v`
Expected: 4 PASS.

- [ ] **Step 5: Use it in both adapters**

In `cmd/harbormaster/audit_adapter.go` replace the body of `bucketAdminAdapter.BucketUsageInfo`:

```go
func (a bucketAdminAdapter) BucketUsageInfo(ctx context.Context, bucket string) (madmin.BucketUsageInfo, error) {
	return hmminio.BucketUsage(ctx, a.AdminClient, bucket)
}
```
and update its doc comment: "Delegates to hmminio.BucketUsage, which tolerates both MinIO's camelCase and RustFS's snake_case census."

In `internal/integration/helper.go` do the same for `integrationBucketAdmin.BucketUsageInfo` (`return hmminio.BucketUsage(ctx, a.AdminClient, bucket)`).

- [ ] **Step 6: Full unit run and commit**

Run: `cd apps/backend && go test -race -count=1 ./... && go vet -tags=integration ./...`
Expected: PASS, vet clean.

```bash
git add apps/backend/internal/minio/usage.go apps/backend/internal/minio/usage_test.go apps/backend/cmd/harbormaster/audit_adapter.go apps/backend/internal/integration/helper.go
git commit -m "fix(buckets): decode datausageinfo from MinIO and RustFS field names"
```

---

### Task 3: `HARBORMASTER_PROMETHEUS_URL` config

**Files:**
- Modify: `apps/backend/internal/config/config.go:17-42, 66-93, 121-152`
- Modify: `apps/backend/internal/config/config_test.go`
- Modify: `docs/operator/configuration.md:46-51`

**Interfaces:**
- Produces: `Config.PrometheusURL string`, empty by default; when set, an absolute `http://` or `https://` URL with no trailing slash.

- [ ] **Step 1: Write the failing tests**

Append to `apps/backend/internal/config/config_test.go` (match the file's existing helper for setting env; if it uses `t.Setenv`, do the same):

```go
func TestLoad_PrometheusURL_DefaultEmpty(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PrometheusURL != "" {
		t.Errorf("want empty default, got %q", cfg.PrometheusURL)
	}
}

func TestLoad_PrometheusURL_TrimsTrailingSlash(t *testing.T) {
	t.Setenv("HARBORMASTER_PROMETHEUS_URL", "http://prometheus.observability.svc.cluster.local:9090/")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PrometheusURL != "http://prometheus.observability.svc.cluster.local:9090" {
		t.Errorf("got %q", cfg.PrometheusURL)
	}
}

func TestLoad_PrometheusURL_RejectsRelative(t *testing.T) {
	t.Setenv("HARBORMASTER_PROMETHEUS_URL", "prometheus:9090")
	if _, err := Load(); err == nil {
		t.Error("want error for URL without scheme")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd apps/backend && go test ./internal/config/ -run PrometheusURL -v`
Expected: FAIL, `cfg.PrometheusURL undefined`.

- [ ] **Step 3: Implement**

In `Config` add after `MetricsRetention`:
```go
	// PrometheusURL, when set, makes the dashboard read its series from a
	// Prometheus server instead of the target's /minio/v2/metrics endpoint.
	// Required for RustFS, which exports OTLP only.
	PrometheusURL string
```
In `Load`'s struct literal add `PrometheusURL: strings.TrimRight(v.GetString("PROMETHEUS_URL"), "/"),`. In `defaults` add `v.SetDefault("PROMETHEUS_URL", "")`. In `validate` add before the trusted-proxies loop:
```go
	if c.PrometheusURL != "" {
		u, err := url.Parse(c.PrometheusURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("HARBORMASTER_PROMETHEUS_URL must be an absolute http(s) URL (got %q)", c.PrometheusURL)
		}
	}
```
Add `"net/url"` to imports.

- [ ] **Step 4: Run to verify pass**

Run: `cd apps/backend && go test ./internal/config/ -v`
Expected: all PASS.

- [ ] **Step 5: Document**

In `docs/operator/configuration.md` add after the `HARBORMASTER_METRICS_LISTEN_ADDR` row:
```
| `HARBORMASTER_PROMETHEUS_URL`              | (empty)                          | URL                   | When set, the dashboard's request/capacity series are read from this Prometheus (PromQL over `rustfs_*` metrics) instead of the target's `/minio/v2/metrics` endpoint. Required for RustFS targets. Empty keeps the MinIO scrape path. |
```
Add `prometheus_url: ""` to the config-file example under the metrics keys.

- [ ] **Step 6: Commit**

```bash
git add apps/backend/internal/config/config.go apps/backend/internal/config/config_test.go docs/operator/configuration.md
git commit -m "feat(config): HARBORMASTER_PROMETHEUS_URL for Prometheus-backed dashboard metrics"
```

---

### Task 4: Prometheus metrics source

**Files:**
- Create: `apps/backend/internal/metrics/promsource.go`
- Create: `apps/backend/internal/metrics/promsource_test.go`

**Interfaces:**
- Consumes: `MetricsSource` interface in `collector.go` (`ClusterMetrics(ctx) ([]*prom2json.Family, error)`, `ResourceMetrics(ctx) ([]*prom2json.Family, error)`), `trackedMetrics` and `counterMetrics` maps.
- Produces: `func NewPrometheusSource(baseURL string, client *http.Client) *PrometheusSource`; `*PrometheusSource` satisfies `MetricsSource`. Task 5 wires it.

- [ ] **Step 1: Write the failing tests**

Create `apps/backend/internal/metrics/promsource_test.go`:

```go
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
```

- [ ] **Step 2: Run to verify failure**

Run: `cd apps/backend && go test ./internal/metrics/ -run PrometheusSource -v`
Expected: FAIL, `undefined: promQueries`, `undefined: NewPrometheusSource`.

- [ ] **Step 3: Implement**

Create `apps/backend/internal/metrics/promsource.go`:

```go
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
var promQueries = map[string]string{
	"minio_s3_requests_total":                   `sum(rustfs_http_server_requests_total)`,
	"minio_s3_requests_4xx_errors_total":        `sum(rustfs_http_server_request_duration_seconds_count{status_class="4xx"})`,
	"minio_s3_requests_5xx_errors_total":        `sum(rustfs_http_server_request_duration_seconds_count{status_class="5xx"})`,
	"minio_s3_traffic_received_bytes":           `sum(rustfs_http_server_request_body_bytes_total)`,
	"minio_s3_traffic_sent_bytes":               `sum(rustfs_http_server_response_body_bytes_total)`,
	"minio_cluster_capacity_usable_total_bytes": `sum(rustfs_cluster_drive_total_bytes)`,
	"minio_cluster_capacity_usable_free_bytes":  `sum(rustfs_cluster_drive_free_bytes)`,
	"minio_cluster_drive_online_total":          `sum(rustfs_cluster_drive_runtime_state{state="online"})`,
	"minio_cluster_drive_offline_total":         `sum(rustfs_cluster_drive_runtime_state{state="offline"})`,
}

// clusterFamilies are served by ClusterMetrics; the rest by ResourceMetrics.
// The split only mirrors the madmin client's two calls; Collect concatenates.
var clusterFamilies = map[string]bool{
	"minio_cluster_capacity_usable_total_bytes": true,
	"minio_cluster_capacity_usable_free_bytes":  true,
	"minio_cluster_drive_online_total":          true,
	"minio_cluster_drive_offline_total":         true,
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
```

- [ ] **Step 4: Run to verify pass**

Run: `cd apps/backend && go test -race ./internal/metrics/ -v`
Expected: all PASS including the four new tests.

- [ ] **Step 5: Commit**

```bash
git add apps/backend/internal/metrics/promsource.go apps/backend/internal/metrics/promsource_test.go
git commit -m "feat(metrics): Prometheus-backed MetricsSource mapping rustfs_* series to the tracked families"
```

---

### Task 5: Wire the source, deploy env, docs

**Files:**
- Modify: `apps/backend/cmd/harbormaster/audit_adapter.go:354-358`
- Modify: `apps/backend/cmd/harbormaster/serve.go:99`
- Modify: `deploy/kubernetes/deployment.yaml:68-75`
- Modify: `docs/observability.md:176-190`

**Interfaces:**
- Consumes: `cfg.PrometheusURL` (Task 3), `metrics.NewPrometheusSource` (Task 4).

- [ ] **Step 1: Source getter**

Replace `newMetricsSourceGetter` in `audit_adapter.go`:

```go
// newMetricsSourceGetter picks the dashboard's series source. With a
// Prometheus URL configured the pool is not consulted at all: the series
// come from PromQL over the target's exported metrics (RustFS pushes OTLP
// and has no scrape endpoint). Otherwise the madmin metrics client scrapes
// /minio/v2/metrics on the live connection, as before.
func newMetricsSourceGetter(pool *hmminio.Pool, prometheusURL string) metrics.SourceGetter {
	if prometheusURL != "" {
		src := metrics.NewPrometheusSource(prometheusURL, nil)
		return func(ctx context.Context) (metrics.MetricsSource, error) { return src, nil }
	}
	return func(ctx context.Context) (metrics.MetricsSource, error) {
		return pool.NewMetricsClient(ctx)
	}
}
```

In `serve.go` change the call to `metrics.NewCollector(newMetricsSourceGetter(pool, cfg.PrometheusURL))`. If a startup log line lists the effective config, add `prometheus_url` to it.

- [ ] **Step 2: Build and test**

Run: `cd apps/backend && go build ./... && go test -race -count=1 ./... && go vet -tags=integration ./...`
Expected: clean.

- [ ] **Step 3: Deployment env**

In `deploy/kubernetes/deployment.yaml` after the `HARBORMASTER_LOG_LEVEL` entry add:
```yaml
            # Dashboard series come from the cluster Prometheus (RustFS pushes
            # OTLP there; there is no /minio/v2/metrics to scrape).
            - name: HARBORMASTER_PROMETHEUS_URL
              value: "http://prometheus.observability.svc.cluster.local:9090"
```

- [ ] **Step 4: Observability doc**

In `docs/observability.md` at the paragraph beginning "`HARBORMASTER_METRICS_POLL_INTERVAL` (default 30s) → `collector.go` calls the MinIO admin client's cluster and resource metrics" add after it:

```markdown
With `HARBORMASTER_PROMETHEUS_URL` set, `promsource.go` replaces the admin
client: one PromQL instant query per tracked family (table in that file)
against the configured Prometheus, results emitted under the same
`minio_*` family names so nothing downstream changes. This is the path for
RustFS targets, which export OTLP to Prometheus and expose no scrape
endpoint. An empty vector for a family produces no sample for that poll.
```

- [ ] **Step 5: Commit**

```bash
git add apps/backend/cmd/harbormaster/audit_adapter.go apps/backend/cmd/harbormaster/serve.go deploy/kubernetes/deployment.yaml docs/observability.md
git commit -m "feat(metrics): select the Prometheus source from HARBORMASTER_PROMETHEUS_URL; deploy against the cluster Prometheus"
```

---

### Task 6: Version banner and connection probe on RustFS

**Files:**
- Modify: `apps/backend/internal/connection/probe.go:158-166`
- Modify: `apps/backend/internal/connection/probe_test.go`

- [ ] **Step 1: Write the failing test**

Append to `probe_test.go`:

```go
func TestServerVersion_BareSemver(t *testing.T) {
	info := madmin.InfoMessage{Servers: []madmin.ServerProperties{{Version: "1.0.0"}}}
	if got := serverVersion(info); got != "1.0.0" {
		t.Errorf("want 1.0.0, got %q", got)
	}
}

func TestServerVersion_NoServersFallsBackToMode(t *testing.T) {
	info := madmin.InfoMessage{Mode: "online"}
	if got := serverVersion(info); got != "online" {
		t.Errorf("want mode fallback, got %q", got)
	}
}

func TestServerVersion_Empty(t *testing.T) {
	if got := serverVersion(madmin.InfoMessage{}); got != "unknown" {
		t.Errorf("want \"unknown\" for an empty banner, got %q", got)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd apps/backend && go test ./internal/connection/ -run TestServerVersion -v`
Expected: first two PASS already (passthrough), `TestServerVersion_Empty` FAIL (got "").

- [ ] **Step 3: Implement**

```go
// serverVersion picks the most useful version banner available in the
// madmin v3 InfoMessage. MinIO reports RELEASE.YYYY-… per server, RustFS a
// bare semver such as 1.0.0; both pass through. Fall back to Mode, then
// to "unknown" so the wizard never shows an empty banner.
func serverVersion(info madmin.InfoMessage) string {
	if len(info.Servers) > 0 && info.Servers[0].Version != "" {
		return info.Servers[0].Version
	}
	if info.Mode != "" {
		return info.Mode
	}
	return "unknown"
}
```

- [ ] **Step 4: Run to verify pass, commit**

Run: `cd apps/backend && go test -race ./internal/connection/`
Expected: PASS.
```bash
git add apps/backend/internal/connection/probe.go apps/backend/internal/connection/probe_test.go
git commit -m "fix(connection): never show an empty version banner; RustFS reports bare semver"
```

---

### Task 7: Run the compatibility suite against RustFS and close the gaps

**Files:**
- Modify: whichever processor or adapter a failing call points at (each fix is its own commit)
- Modify: `docs/operator/security.md` (supported targets)

**Interfaces:**
- Consumes: Task 1's target selector, Task 2's decoder.

- [ ] **Step 1: Full run, both targets**

```bash
cd apps/backend
HARBORMASTER_INTEGRATION=1 go test -race -count=1 -tags=integration ./... 2>&1 | tee /tmp/it-minio.log | tail -20
HARBORMASTER_INTEGRATION=1 HARBORMASTER_IT_TARGET=rustfs go test -race -count=1 -tags=integration ./... 2>&1 | tee /tmp/it-rustfs.log | tail -20
grep -E '^(--- FAIL|FAIL|ok)' /tmp/it-rustfs.log
```
Expected: minio all `ok`. rustfs: a list of `--- FAIL` lines or all `ok`.

- [ ] **Step 2: Triage each RustFS failure**

For each `--- FAIL`, read the assertion and the HTTP error. Classify:

1. **Response shape differs** (like the usage census): add a tolerant decode at the adapter boundary in `cmd/harbormaster/audit_adapter.go` or `internal/minio`, unit-tested with both fixtures, same pattern as Task 2. Commit `fix(<pkg>): tolerate RustFS <field> shape`.
2. **Route returns 404 or 501 on RustFS**: in the processor, map the madmin error to a typed `apierror` with code `backend_unsupported` and a message naming the operation, so the UI shows "not supported by this target" rather than a 500. Add a unit test with a stub client returning that error. Commit `feat(<pkg>): typed backend_unsupported error for <operation>`. Record the operation in Step 4's table.
3. **Semantic difference** (e.g. a policy attach that succeeds but `GetUserInfo` does not reflect it immediately): add a bounded retry only if the integration test proves eventual consistency; otherwise treat as class 2.

Re-run the rustfs suite after each fix until Step 1's grep shows only `ok`.

- [ ] **Step 3: Live check against the cluster RustFS**

With the phase-2 RustFS up (k3s plan Task 3 done) and a local Harbormaster build pointed at it through a port-forward, with `HARBORMASTER_PROMETHEUS_URL` pointed at a port-forward of the cluster Prometheus:

```bash
kubectl -n rustfs port-forward svc/rustfs 19001:9000 &
kubectl -n observability port-forward svc/prometheus 19090:9090 &
cd apps/backend && HARBORMASTER_DATA_DIR=/tmp/hm-rustfs HARBORMASTER_SESSION_COOKIE_SECURE=false HARBORMASTER_PROMETHEUS_URL=http://localhost:19090 go run ./cmd/harbormaster serve
```
In the UI: complete setup against `http://localhost:19001` with the RustFS root pair. Check: Users lists zot, atlas-data, atlas-renders, myfleet with policies; Policies lists zot-rw and myfleet-media-rw; Buckets shows six with non-zero size; Dashboard shows version `1.0.0` and 1 drive online; Metrics page fills after two poll intervals. Record results in `docs/tasks/task-005-rustfs-target/context.md`.

- [ ] **Step 4: Document supported targets**

In `docs/operator/security.md` under the "supported floor" text add:

```markdown
### Supported targets

| Target | Version floor | Notes |
|---|---|---|
| MinIO | RELEASE.2025-09-07T16-13-09Z | Dashboard metrics scraped from `/minio/v2/metrics`. |
| RustFS | 1.0.0 | Set `HARBORMASTER_PROMETHEUS_URL`; RustFS exports OTLP only. Unsupported operations (if any) return `backend_unsupported`: (list from Task 7 Step 2, or "none"). |

Both run in the nightly integration matrix.
```

- [ ] **Step 5: Commit and open the PR**

```bash
git add -A docs/operator/security.md docs/tasks/task-005-rustfs-target
git commit -m "docs(operator): RustFS is a supported target; record the compatibility run"
git push -u origin task-005-rustfs-target
```
Open the PR. PR CI runs unit tests and `go vet -tags=integration`; the nightly runs the three-target matrix.
