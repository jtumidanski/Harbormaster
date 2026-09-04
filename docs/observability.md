# Observability

This document owns the deploy/runbook story: **what signals Harbormaster
actually emits, how to reach them in each of the two supported deployments,
and how to prove a deploy came up.**

Harbormaster is one self-hosted Go binary plus a static frontend, backed by a
local SQLite file. There is no service mesh, no distributed-tracing pipeline
and no bundled Grafana/Loki/Prometheus stack. Everything below is what the
source in `apps/backend` and the manifests in `deploy/` do today — read those
before extending this page.

The signal surface is four things:

| Signal | Where it comes from | Where it goes |
|---|---|---|
| Structured logs | `apps/backend/internal/observability/log` (zerolog) | the container's stderr |
| One line per HTTP request | `apps/backend/internal/observability/middleware.go` | same stream, `msg=http_request` |
| Liveness / readiness | `apps/backend/internal/server/health.go` | `GET /healthz`, `GET /readyz` |
| MinIO cluster metrics | `apps/backend/internal/metrics` | the local SQLite DB, served at `GET /api/v1/metrics` |

For the configuration knobs themselves,
[`docs/operator/configuration.md`](operator/configuration.md) is the reference
and this page does not restate it. For recovery procedures,
[`docs/operator/recovery.md`](operator/recovery.md). For what must never appear
in a log or an audit payload,
[`docs/operator/security.md`](operator/security.md).

## Log field naming

**Structured-log field keys are `snake_case`**, and the message is a short
stable event name rather than a sentence with values interpolated into it. The
request middleware is the reference shape — `request_id`, `method`, `path`,
`status`, `latency`, `bytes`, `msg=http_request`. Elsewhere in the tree the
same spelling holds: `addr`, `bucket`, `key`, `count`, `access_key`,
`deleted`. Match it. A new key spelled `bucketName` costs every future
`grep`/`jq` a second spelling to remember, and there is no normalization hook
here to rescue it — this repo has ~15 log call sites, so the convention is
cheap to hold by hand and there is no excuse for drift.

**Every emit site goes through `internal/observability/log`, or through a
logger obtained from the context.** The package doc says so, and
`golangci-lint`'s `forbidigo` rules enforce the negative half: `fmt.Print*` is
forbidden with the message *"use the logger, not fmt.Print*"* (see
`apps/backend/.golangci.yml`). The point is not tidiness — it is that a single
choke point is what makes level, format, and any future secret-scrubbing check
apply to *all* output rather than most of it.

The mechanics worth knowing:

- `log.NewWith(level, format, w)` builds the logger; `log.New` writes to
  **stderr**. Logs are not written to a file and not written to stdout — a
  collector must read stderr.
- `log.WithLogger(ctx, l)` / `log.FromCtx(ctx)` carry it. `FromCtx` on a
  context with no logger returns a **`zerolog.Nop()`** — it silently discards.
  If a code path logs nothing you expected, check that its context came from
  the request chain before assuming the code did not run.
- The HTTP middleware derives a per-request child logger carrying
  `request_id` (chi's request ID) and puts it on the context, so anything
  downstream that uses `FromCtx` is automatically correlated. Use it; do not
  thread your own ID.
- Every line carries a timestamp and a caller. `HARBORMASTER_LOG_FORMAT=json`
  (the default, and what both deploy manifests set) is what you want for
  anything that gets grepped; `console` is for a human at a terminal.

Secrets — S3 access keys, passwords, session material — never go in a log
field or an audit payload. That rule and its enforcement belong to
[`docs/operator/security.md`](operator/security.md); it is repeated here only
because the log call is
where the violation would be written.

## Diagnosing a runtime failure

**For a wedged deploy or a runtime failure, read the service logs before you
change any code.** They usually name the root cause directly, and a fix
written before reading them is a guess. The two commands, one per supported
deployment:

```sh
docker compose -f deploy/docker/docker-compose.yml logs harbormaster
kubectl logs deploy/harbormaster
```

**Read the logs for the workload you name, never a whole-namespace listing.**
`kubectl get pods -n <ns>` and friends are almost entirely metadata; when you
already know the workload is `harbormaster`, a namespace-wide listing pays for
every other pod's status block to learn one pod's name. Name the workload, or
filter the listing.

A corollary specific to this repo's manifests: **the pod label is
`app.kubernetes.io/name=harbormaster`, not `app=harbormaster`.** A selector
written `-l app=harbormaster` matches nothing and returns silently — no error,
zero rows, which reads exactly like "the service logged nothing." Check the
selector before believing an empty result. `deploy/kubernetes/deployment.yaml`
is the authority on the label set.

**A large log payload gets sliced, not read whole.** `grep` for the event name
or the request ID first (`| grep http_request`, `| grep '"status":5'`), then
widen with `-A`/`-B` around the hit. Reading a whole log dump into context to
"see it properly" is the cost mistake [`docs/slice-first.md`](slice-first.md)
describes, and log
output is its most expensive instance.

### Reading `/readyz` correctly

`GET /readyz` reports **only whether the local database answers a ping.** It is
deliberately *not* coupled to MinIO reachability: readiness gates Service
membership, so wiring MinIO into it would let a MinIO outage withdraw the pod
and 503 every route — including the login page an operator needs in order to
fix a bad connection. See the comment on `dbReadiness` in
`apps/backend/cmd/harbormaster/serve.go`.

The diagnostic consequences:

- `/readyz` returning 503 with `{"error":{"code":"not_ready", ...}}` means the
  **local DB**, not MinIO. Look at the data volume and the DB file.
- `/readyz` returning 200 while every bucket listing fails means **MinIO**.
  Look for `connection: failed to hydrate minio pool at boot` in the logs — a
  bad or undecryptable stored connection is logged as a warning and is
  deliberately non-fatal, so the process starts healthy and only the MinIO-backed
  routes fail.
- `/healthz` is unconditional. It proves the process is serving HTTP and
  nothing else. It cannot tell you anything about state.

## Deploy smoke test

A deploy is not verified because the container is running. It is verified when
something served a request. Run the check for the path you deployed.

### Docker Compose

```sh
docker compose -f deploy/docker/docker-compose.yml up -d
docker compose -f deploy/docker/docker-compose.yml ps
curl -fsS localhost:8080/healthz   # {"status":"ok"}
curl -fsS localhost:8080/readyz    # {"status":"ok"}; 503 means the DB
docker compose -f deploy/docker/docker-compose.yml logs harbormaster \
  | grep 'harbormaster started'
```

The last two lines are the ones that matter. **The compose file's own
`healthcheck` runs `harbormaster version`** — that proves the binary is
executable inside the image and nothing more; it never touches the HTTP
server, so a container can sit `healthy` while the listener is dead. A
healthcheck that does not exercise the serving path is not a smoke test.
Substitute the port if you overrode `HARBORMASTER_PORT`.

### Kubernetes

```sh
kubectl rollout status deploy/harbormaster --timeout=120s
kubectl get pods -l app.kubernetes.io/name=harbormaster
kubectl logs deploy/harbormaster | grep 'harbormaster started'
kubectl port-forward deploy/harbormaster 8080:8080
# in another shell:
curl -fsS localhost:8080/readyz
```

`rollout status` is the load-bearing one: the manifest wires
`readinessProbe: GET /readyz` and `livenessProbe: GET /healthz`, so a rollout
that reports complete has already had `/readyz` answer 200 from inside the
cluster. A rollout that hangs is the probe failing — go straight to
`kubectl logs`, not to the manifest.

Two properties of this manifest change what a stuck rollout means:
`replicas: 1` with `strategy: Recreate`, and a single `ReadWriteOnce` PVC
(`deploy/kubernetes/pvc.yaml`). The old pod must fully terminate and release
the volume before the new one binds, so a rollout stalled at
`ContainerCreating` is usually the PVC still attached to the terminating pod,
not the application. Single-replica operation is a deliberate constraint, not
an oversight — [`docs/operator/security.md`](operator/security.md) explains
why.

## Metrics

`apps/backend/internal/metrics` is a self-contained subsystem: it **polls
MinIO's own Prometheus endpoint and stores the result locally.** Harbormaster
does not expose a Prometheus scrape endpoint of its own, and does not
instrument itself.

The pipeline is six files, in order: `poller.go` ticks every
`HARBORMASTER_METRICS_POLL_INTERVAL` (default 30s) → `collector.go` calls the
MinIO admin client's cluster and resource metrics and flattens the result →
`store.go` writes one row per `(metric, value)` per poll into SQLite →
`processor.go` is the read path, joining `store.Query` to `aggregator.go`'s
`Aggregate` and computing data freshness → `aggregator.go` downsamples on
read → `resource.go` serves `GET /api/v1/metrics?window=<1h|6h|24h|7d>`. A
second goroutine
(`StartRetentionSweeper`) deletes samples older than
`HARBORMASTER_METRICS_RETENTION` (default 8 days) once a day.

**What it collects is an explicit allowlist**, `trackedMetrics` in
`collector.go` — nine families, S3 request counts, 4xx/5xx counts, traffic
bytes, cluster usable/free capacity, and drive online/offline counts. Anything
else MinIO exposes is discarded at flatten time.

**That allowlist is the design, not a stub, and it is the local form of a
cardinality budget.** Every tracked family is summed to one cluster-wide value
per poll, so the row count is (families × polls) and nothing multiplies it.
Adding a per-bucket or per-object series instead would multiply stored rows by
an operator-controlled, unbounded factor — into a SQLite file on a homelab
volume, on a fixed retention. So: **before adding a metric, bound its
cardinality; enumerate what you keep rather than storing whatever the source
emits.** "Keep everything MinIO sends" is not an acceptable change to
`trackedMetrics`.

**What it does not collect:** anything about Harbormaster itself. No request
latency histogram, no Go runtime metrics, no per-bucket or per-object series,
and no histograms or summaries at all (`flattenFamilies` skips any family
element that is not a plain `prom2json.Metric`). Harbormaster's own request
timings exist only as the `latency` field on each `http_request` log line.

### Reading the numbers without being misled

`aggregator.go` derives counter rates at read time, and three of its choices
change what a chart means:

- **The poll interval is the resolution floor.** The window's step is fixed by
  `model.go` (`1h`→60s, `6h`/`24h`→300s, `7d`→2400s, each chosen to keep a
  series under ~300 points). Polling less often than the step leaves buckets
  empty; polling much more often just discards samples at downsample time.
- **Rates divide by the step, not by the actual sample spacing**, and a gap in
  the samples resets rate continuity rather than interpolating across it. A
  restart, or a stretch where MinIO was unreachable, shows as a hole — not as
  a spike on either side of one.
- **A negative delta is clamped to zero**, because it means the MinIO counter
  reset. A MinIO restart therefore reads as a flat zero interval, not as a
  negative rate. Do not read that zero as "no traffic."

`Collected: false` in the response means no samples matched the window at all
— usually metrics have simply not been polled yet, or MinIO was never
reachable. It is not an error.

### Three knobs that are read but not wired

`HARBORMASTER_METRICS_ENABLED`, `HARBORMASTER_METRICS_LISTEN_ADDR` and
`HARBORMASTER_OTEL_EXPORTER_OTLP_ENDPOINT` are parsed and validated by
`internal/config`, but as of this writing nothing else in `apps/backend` reads
them: there is exactly one `http.Server` in the tree
(`internal/server/server.go`, bound to `HARBORMASTER_LISTEN_ADDR`), and no
OpenTelemetry symbol is imported outside `go.mod`. Setting them changes
nothing. The general rule this is an instance of: **a config key existing is
not evidence that the behaviour behind it exists — grep for the field's
consumer before writing a runbook step that depends on it.**

## What is deliberately absent

Stated so nobody re-adds them by reflex, and so nobody reads this document as
an unfinished port of a bigger one:

- **No tracing pipeline.** No tracer provider, no spans, no OTLP exporter, no
  Tempo. Correlate with `request_id` across log lines instead.
- **No Grafana, Prometheus server or Loki in this repo.** `deploy/` contains a
  Dockerfile, a Compose file, two reverse-proxy examples and five Kubernetes
  manifests. There are no dashboards to edit and no datasource to provision;
  the metrics UI is the frontend reading `GET /api/v1/metrics`.
- **No per-environment labelling.** There is one deployment per namespace, and
  no ephemeral per-PR environment. Scope by workload name, which the commands
  above already do.

Anything you want to add here that assumes one of those exists needs the
infrastructure first, and belongs in a design document before it belongs in a
runbook.
