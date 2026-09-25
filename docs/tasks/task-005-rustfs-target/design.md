# task-005: RustFS as a first-class target — design

The design lives in the k3s repo, which orchestrates the whole MinIO to
RustFS migration:
`k3s/docs/superpowers/specs/2026-09-24-minio-to-rustfs-migration-design.md`,
section 6 "Phase 3: Harbormaster port". This file is the pointer the
five-phase workflow expects; `plan.md` beside it is the executable plan.

Summary of section 6:

- RustFS 1.0 serves Harbormaster's madmin routes under `/minio/admin/v3/`.
  The integration suite gains a `rustfs` target so every call is proven,
  not assumed.
- `datausageinfo` on RustFS uses snake_case keys; madmin decodes zeros. A
  tolerant decoder replaces the `DataUsageInfo` call in both adapters.
- RustFS has no `/minio/v2/metrics/*`. A Prometheus-backed `MetricsSource`
  behind `HARBORMASTER_PROMETHEUS_URL` synthesizes the nine tracked
  families from `rustfs_*` series; the madmin path stays the default.
- UI strings and the `internal/minio` package name are renamed in a later
  task (spec §8), not here.
