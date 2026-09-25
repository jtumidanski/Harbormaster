# Task 7 — RustFS compatibility run: results

Records the outcome of task-005 Task 7 (`.superpowers/sdd/plan/task-7-brief.md`):
running Harbormaster's integration suite against RustFS, closing any gaps
found, and a live check against the phase-2 k3s cluster's RustFS
deployment. No secrets are recorded below.

## Step 1 — Integration suite

Environment note: `quay.io/minio/minio` pulls return 401 (anonymous pull
blocked) in this environment, so only the RustFS leg of Step 1 could run;
the MinIO leg is exercised by the nightly matrix in CI, not here.

```
HARBORMASTER_INTEGRATION=1 HARBORMASTER_IT_TARGET=rustfs go test -race -count=1 -tags=integration ./...
```

Result: every package `ok`, no `--- FAIL` lines. The `internal/integration`
package (the RustFS-vs-MinIO parity suite) passed in 61.7s. See the Step 2
triage below — no test-suite failures were found, but the live check (Step
3) surfaced one gap the ephemeral testcontainer suite does not exercise.

## Step 2 — Triage

No `--- FAIL` lines from Step 1, so there was nothing from the integration
suite itself to triage. One gap was found later, by the live check against
the real cluster (Step 3), not by the testcontainer suite: RustFS's admin
`info` endpoint (`GET /minio/admin/v3/info`, what `madmin.ServerInfo`
calls) responds `200 OK` but with neither a `version` nor a `servers[]`
entry — only bucket/object/usage counts. This is a response-shape gap
(triage class 1: "response shape differs"), not a 404/501, so it does not
produce a `backend_unsupported` error; the dashboard fan-out call itself
never fails.

Fix: `cmd/harbormaster/audit_adapter.go`'s `dashboardPoolAdapter.ServerInfo`
now falls back to the target's public, unauthenticated `/health/ready`
endpoint when the admin `info` call comes back with no version and no
servers. `/health/ready`'s body was captured live from the cluster
(`{"version":"1.0.0","ready":true,"details":{"storage":{"ready":true}}}`,
trimmed) and is decoded by a new `internal/minio/health.go` (`ServerHealth`
+ `decodeHealthReady`), unit-tested with that fixture. The adapter
synthesises a single-node `NodeStatus` from the readiness flags (state
`online`/`offline`, one drive healthy/unhealthy) so the dashboard shows a
version and a drive-online count instead of going blank. MinIO always
populates `info.Servers`, so this fallback is a no-op for MinIO targets.

Commit: `fix(dashboard): fall back to /health/ready for RustFS's version + node status`.

No route on RustFS returned 404/501 during this run, so the "Unsupported
operations" cell in `docs/operator/security.md`'s Supported targets table
reads **none**.

## Step 3 — Live check against the cluster RustFS

Cluster: phase-2 k3s RustFS (`rustfs` namespace) + cluster Prometheus
(`observability` namespace), reached via port-forwards on local ports
19011/19091 (19001/19090 were already in use). Harbormaster ran locally
(`go run ./cmd/harbormaster serve`) against a clean `/tmp/hm-rustfs` data
dir, `HARBORMASTER_PROMETHEUS_URL=http://localhost:19091`,
`HARBORMASTER_METRICS_POLL_INTERVAL=15s`. Driven with `curl` + a cookie jar
against the JSON API (not the UI) per the brief's contract
(`docs/tasks/task-001-harbormaster-mvp-v1/api-contracts.md`).

Root credentials were read from `kubectl -n rustfs get secret
rustfs-root-creds` into shell variables only; never echoed or logged.

Setup (`POST /api/v1/setup`) and login (`POST /api/v1/auth/login`) are
public, unauthenticated routes per the contract and `cmd/harbormaster/serve.go`
(CSRF is only required on the protected route group), so no CSRF header
was needed for those two calls.

| Endpoint | Status | Result |
|---|---|---|
| `POST /api/v1/setup` | 201 | `{"initialized":true}` against `http://localhost:19011` with the RustFS root pair. |
| `POST /api/v1/auth/login` | 204 | Session + CSRF cookies set. |
| `GET /api/v1/connection` | 200 | `endpoint_url` correct, `access_key_masked` present, no secret echoed. |
| `GET /api/v1/users` | 200 | 4 users: `atlas-data`, `atlas-renders` (both `other_policies: [readwrite]`), `myfleet` (`attached_policies: [myfleet-media-rw]`), `zot` (`attached_policies: [zot-rw]`) — matches expectation. |
| `GET /api/v1/policies` | 200 | 10 policies including `zot-rw` and `myfleet-media-rw` among the MinIO/RustFS built-ins and KMS policies — matches expectation. |
| `GET /api/v1/buckets` | 200 | 6 buckets (`atlas-assets`, `atlas-canonical`, `atlas-renders`, `atlas-wz`, `myfleet-media`, `zot`), all with non-zero `estimated_bytes`/`object_count` on the **first** try — RustFS's census had already caught up, no 60s retry needed. |
| `GET /api/v1/dashboard` (before fix) | 200 | `server.version=""`, `nodes=[]` — the gap described in Step 2. |
| `GET /api/v1/dashboard` (after fix) | 200 | `server.version="1.0.0"`, `nodes=[{"endpoint":"localhost:19011","state":"online","drives":{"total":1,"healthy":1,"unhealthy":0}}]`, `totals.buckets=6` — matches expectation. |
| `GET /api/v1/metrics?window=1h` (after ~60s / one poll cycle) | 200 | `collected:true`, all 9 tracked series present incl. `minio_cluster_drive_online_total=1`, sourced via PromQL over the cluster Prometheus. |

Server and both port-forwards were stopped at the end of the check; ports
18080/19011/19091 confirmed free afterward.

## Step 4 — Documentation

`docs/operator/security.md`: added a "Supported targets" section (MinIO /
RustFS version floors, notes, unsupported-operations = none) directly
after the document's intro, before "Threat model (summary)" — the file had
no pre-existing "supported floor" heading to anchor under.
