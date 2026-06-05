# P0 UI Feature Parity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Read `context.md` (same folder) first — it carries the verified SDK facts and conventions every task assumes.

**Goal:** Close the four P0 MinIO-Console parity gaps — object version browser + restore, noncurrent/abort-mpu lifecycle rules, custom IAM policy editor, and a Prometheus-backed metrics dashboard — as one coordinated release, preserving every v1 convention.

**Architecture:** Four independent backend tracks under `apps/backend/internal/{objects,lifecycle,policies,metrics,users}`, each following the per-domain DDD layering (`model→builder→provider→administrator→processor→resource→rest`) with processors depending on small unexported interfaces injected via `ClientGetter`. MinIO stays the source of truth for versions/lifecycle/policies; the only new persistence is a `metrics_samples` SQLite table fed by a background poller. The React frontend adds a versions drawer, a lifecycle kind selector, a full Policies page + custom-policy user attachment, and a Metrics charts page.

**Tech Stack:** Go 1.x, chi, gorm/SQLite, minio-go v7.0.74, madmin-go v3.0.66, prom2json v1.3.3; React/TypeScript, Vite, TanStack Query, react-hook-form + Zod, shadcn/ui, Recharts, Vitest.

**Build order:** A (lifecycle) → B (objects versions) → C (policies CRUD) → D (users custom-policy attachment, depends on C) → E (metrics) → F (nav/route integration + final verification). Tracks A/B/E are independent; D depends on C.

**Done bar (every track must keep green):** Backend `go test -race -count=1 ./...`, `go vet ./...`, `golangci-lint run`, `CGO_ENABLED=0 go build ./...`; Frontend `npm run lint`, `npm run format`, `npm test`, `npm run build`. Commands run from `apps/backend` and `apps/frontend` respectively.

---

## Track A — Lifecycle Extensions

Extends `internal/lifecycle` from expiration-only to three managed kinds: `expiration` (unchanged), `noncurrent-expiration`, `abort-incomplete-multipart`. Preserves the "managed iff Harbormaster-shaped AND no foreign attributes" invariant.

### Task A1: Extend the lifecycle domain model with kind-specific fields

**Files:**
- Modify: `apps/backend/internal/lifecycle/model.go`

- [ ] **Step 1: Add the new fields and kind constants to `model.go`**

Add these constants above the `Rule` struct (after the package doc comment block):

```go
// Managed lifecycle kinds. A managed Rule's Kind is exactly one of these.
const (
	KindExpiration            = "expiration"
	KindNoncurrentExpiration  = "noncurrent-expiration"
	KindAbortIncompleteMPU    = "abort-incomplete-multipart"
)
```

Add these fields to the `Rule` struct (after the existing `Prefix` field, before `Summary`):

```go
	// NoncurrentDays is the age (days) after which a noncurrent version
	// expires. Non-zero only for Kind == KindNoncurrentExpiration.
	NoncurrentDays int

	// NewerNoncurrentVersions optionally retains this many newest
	// noncurrent versions before expiring older ones. Zero means "no
	// retention floor". Only meaningful for KindNoncurrentExpiration.
	NewerNoncurrentVersions int

	// DaysAfterInitiation is the age (days) after which an incomplete
	// multipart upload is aborted. Non-zero only for
	// Kind == KindAbortIncompleteMPU.
	DaysAfterInitiation int
```

Update the `Kind` field doc comment to read: `// Kind is one of {KindExpiration, KindNoncurrentExpiration, KindAbortIncompleteMPU} for managed rules.`

- [ ] **Step 2: Verify the package still builds**

Run: `cd apps/backend && CGO_ENABLED=0 go build ./internal/lifecycle/...`
Expected: builds clean (no test changes yet — fields are additive).

- [ ] **Step 3: Commit**

```bash
git add apps/backend/internal/lifecycle/model.go
git commit -m "feat(lifecycle): add kind constants and noncurrent/abort-mpu model fields"
```

### Task A2: Deterministic IDs and validators for the new kinds

**Files:**
- Modify: `apps/backend/internal/lifecycle/builder.go`
- Test: `apps/backend/internal/lifecycle/builder_test.go` (create)

- [ ] **Step 1: Write the failing test**

Create `apps/backend/internal/lifecycle/builder_test.go`:

```go
package lifecycle

import "testing"

func TestGenerateNoncurrentRuleID(t *testing.T) {
	cases := []struct {
		days   int
		prefix string
		want   string
	}{
		{30, "", "harbormaster-noncurrent-all-30d"},
		{7, "uploads/", "harbormaster-noncurrent-uploads-7d"},
	}
	for _, c := range cases {
		if got := generateNoncurrentRuleID(c.days, c.prefix); got != c.want {
			t.Errorf("generateNoncurrentRuleID(%d,%q)=%q want %q", c.days, c.prefix, got, c.want)
		}
	}
}

func TestGenerateAbortMPURuleID(t *testing.T) {
	if got := generateAbortMPURuleID(7, ""); got != "harbormaster-abortmpu-all-7d" {
		t.Errorf("got %q", got)
	}
	if got := generateAbortMPURuleID(3, "tmp/"); got != "harbormaster-abortmpu-tmp-3d" {
		t.Errorf("got %q", got)
	}
}

func TestValidateNoncurrent(t *testing.T) {
	if err := validateNoncurrent(0, 0); err == nil {
		t.Error("expected error for noncurrent_days=0")
	}
	if err := validateNoncurrent(1, -1); err == nil {
		t.Error("expected error for negative newer_noncurrent_versions")
	}
	if err := validateNoncurrent(30, 3); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateDaysAfterInitiation(t *testing.T) {
	if err := validateDaysAfterInitiation(0); err == nil {
		t.Error("expected error for 0")
	}
	if err := validateDaysAfterInitiation(7); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/backend && go test ./internal/lifecycle/ -run 'GenerateNoncurrent|GenerateAbortMPU|ValidateNoncurrent|ValidateDaysAfterInitiation' -v`
Expected: FAIL — undefined `generateNoncurrentRuleID`, etc.

- [ ] **Step 3: Implement the generators and validators**

Append to `apps/backend/internal/lifecycle/builder.go`:

```go
// idSlug returns the prefix slug used in a managed rule ID, or "all" when
// the prefix is empty (whole-bucket scope). Reuses slugifyPrefix so the
// charset stays in lock-step with the classifier regex.
func idSlug(prefix string) string {
	if prefix == "" {
		return "all"
	}
	slug := slugifyPrefix(prefix)
	if slug == "" {
		return "all"
	}
	return slug
}

// generateNoncurrentRuleID returns the deterministic ID for a managed
// noncurrent-version-expiration rule: "harbormaster-noncurrent-<slug>-<days>d".
func generateNoncurrentRuleID(days int, prefix string) string {
	id := fmt.Sprintf("harbormaster-noncurrent-%s-%dd", idSlug(prefix), days)
	return clampRuleID(id)
}

// generateAbortMPURuleID returns the deterministic ID for a managed
// abort-incomplete-multipart rule: "harbormaster-abortmpu-<slug>-<days>d".
func generateAbortMPURuleID(days int, prefix string) string {
	id := fmt.Sprintf("harbormaster-abortmpu-%s-%dd", idSlug(prefix), days)
	return clampRuleID(id)
}

// clampRuleID truncates an over-long generated ID and trims a trailing
// delimiter so it still satisfies the classifier regex.
func clampRuleID(id string) string {
	if len(id) > MaxRuleIDLen {
		id = strings.TrimRight(id[:MaxRuleIDLen], "-.")
	}
	return id
}

// validateNoncurrent enforces the operator-facing contract for a
// noncurrent-expiration rule: noncurrent_days >= 1 and (optional)
// newer_noncurrent_versions >= 0.
func validateNoncurrent(noncurrentDays, newerNoncurrent int) error {
	if noncurrentDays <= 0 {
		return errors.New("noncurrent_days must be > 0")
	}
	if newerNoncurrent < 0 {
		return errors.New("newer_noncurrent_versions must be >= 0")
	}
	return nil
}

// validateDaysAfterInitiation enforces days_after_initiation >= 1 for an
// abort-incomplete-multipart rule.
func validateDaysAfterInitiation(days int) error {
	if days <= 0 {
		return errors.New("days_after_initiation must be > 0")
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd apps/backend && go test ./internal/lifecycle/ -run 'GenerateNoncurrent|GenerateAbortMPU|ValidateNoncurrent|ValidateDaysAfterInitiation' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/backend/internal/lifecycle/builder.go apps/backend/internal/lifecycle/builder_test.go
git commit -m "feat(lifecycle): deterministic IDs + validators for noncurrent/abort-mpu kinds"
```

### Task A3: Generalize the classifier to three managed families

**Files:**
- Modify: `apps/backend/internal/lifecycle/classifier.go`
- Test: `apps/backend/internal/lifecycle/classifier_test.go` (extend)

- [ ] **Step 1: Write the failing tests**

Append to `apps/backend/internal/lifecycle/classifier_test.go`:

```go
func TestClassifyNoncurrentManaged(t *testing.T) {
	r := mlifecycle.Rule{
		ID:     "harbormaster-noncurrent-uploads-30d",
		Status: "Enabled",
		NoncurrentVersionExpiration: mlifecycle.NoncurrentVersionExpiration{
			NoncurrentDays:          mlifecycle.ExpirationDays(30),
			NewerNoncurrentVersions: 3,
		},
		RuleFilter: mlifecycle.Filter{Prefix: "uploads/"},
	}
	got := classify(r)
	if !got.Managed || got.Kind != KindNoncurrentExpiration {
		t.Fatalf("expected managed noncurrent, got %+v", got)
	}
	if got.NoncurrentDays != 30 || got.NewerNoncurrentVersions != 3 || got.Prefix != "uploads/" {
		t.Errorf("fields wrong: %+v", got)
	}
}

func TestClassifyAbortMPUManaged(t *testing.T) {
	r := mlifecycle.Rule{
		ID:     "harbormaster-abortmpu-all-7d",
		Status: "Enabled",
		AbortIncompleteMultipartUpload: mlifecycle.AbortIncompleteMultipartUpload{
			DaysAfterInitiation: mlifecycle.ExpirationDays(7),
		},
	}
	got := classify(r)
	if !got.Managed || got.Kind != KindAbortIncompleteMPU || got.DaysAfterInitiation != 7 {
		t.Fatalf("expected managed abort-mpu(7), got %+v", got)
	}
}

func TestClassifyNoncurrentWithForeignActionIsUnmanaged(t *testing.T) {
	// A noncurrent-ID rule that ALSO carries an expiration action is foreign-shaped.
	r := mlifecycle.Rule{
		ID:                          "harbormaster-noncurrent-all-30d",
		Status:                      "Enabled",
		NoncurrentVersionExpiration: mlifecycle.NoncurrentVersionExpiration{NoncurrentDays: mlifecycle.ExpirationDays(30)},
		Expiration:                  mlifecycle.Expiration{Days: mlifecycle.ExpirationDays(5)},
	}
	if got := classify(r); got.Managed {
		t.Fatalf("expected unmanaged, got %+v", got)
	}
}
```

(Confirm `classifier_test.go` already imports `mlifecycle "github.com/minio/minio-go/v7/pkg/lifecycle"`; if not, add it.)

- [ ] **Step 2: Run to verify failure**

Run: `cd apps/backend && go test ./internal/lifecycle/ -run 'ClassifyNoncurrent|ClassifyAbortMPU' -v`
Expected: FAIL — these rules currently classify as unmanaged.

- [ ] **Step 3: Rewrite the classifier**

Replace the `managedIDRE` var and the `classify` function in `classifier.go` with:

```go
// Managed-ID regexes, one per family. A rule's ID must match exactly one
// of these as a precondition of the managed path; the per-family shape
// check below gates the actual Managed verdict.
var (
	expireIDRE      = regexp.MustCompile(`^harbormaster-expire-\d+d(-[a-z0-9.-]+)?$`)
	noncurrentIDRE  = regexp.MustCompile(`^harbormaster-noncurrent-[a-z0-9.-]+-\d+d$`)
	abortMPUIDRE    = regexp.MustCompile(`^harbormaster-abortmpu-[a-z0-9.-]+-\d+d$`)
)

// classify maps an upstream lifecycle.Rule into the domain Rule shape. A
// rule is "managed" iff its ID matches exactly one managed family AND its
// server-side config is exactly that family's action with nothing foreign
// (no other actions, no tag filters). Any drift flips it to "unmanaged".
func classify(r mlifecycle.Rule) Rule {
	switch {
	case expireIDRE.MatchString(r.ID) && isExpirationShaped(r):
		return Rule{
			ID: r.ID, Managed: true, Kind: KindExpiration,
			Days: int(r.Expiration.Days), Prefix: r.RuleFilter.Prefix,
		}
	case noncurrentIDRE.MatchString(r.ID) && isNoncurrentShaped(r):
		return Rule{
			ID: r.ID, Managed: true, Kind: KindNoncurrentExpiration,
			NoncurrentDays:          int(r.NoncurrentVersionExpiration.NoncurrentDays),
			NewerNoncurrentVersions: int(r.NoncurrentVersionExpiration.NewerNoncurrentVersions),
			Prefix:                  r.RuleFilter.Prefix,
		}
	case abortMPUIDRE.MatchString(r.ID) && isAbortMPUShaped(r):
		return Rule{
			ID: r.ID, Managed: true, Kind: KindAbortIncompleteMPU,
			DaysAfterInitiation: int(r.AbortIncompleteMultipartUpload.DaysAfterInitiation),
			Prefix:              r.RuleFilter.Prefix,
		}
	}
	return Rule{ID: r.ID, Managed: false, Summary: summarize(r)}
}

// isExpirationShaped is true iff r carries exactly one Expiration action
// (positive days), no other actions, and no tag filters.
func isExpirationShaped(r mlifecycle.Rule) bool {
	return r.Transition.IsNull() &&
		r.NoncurrentVersionTransition.StorageClass == "" &&
		r.NoncurrentVersionExpiration.IsDaysNull() &&
		r.NoncurrentVersionExpiration.NewerNoncurrentVersions == 0 &&
		r.AbortIncompleteMultipartUpload.DaysAfterInitiation == 0 &&
		r.DelMarkerExpiration.Days == 0 &&
		!r.Expiration.IsDaysNull() && int(r.Expiration.Days) > 0 &&
		hasNoTagFilters(r)
}

// isNoncurrentShaped is true iff r carries exactly one
// NoncurrentVersionExpiration action (positive NoncurrentDays), no other
// actions, and no tag filters.
func isNoncurrentShaped(r mlifecycle.Rule) bool {
	return r.Transition.IsNull() &&
		r.NoncurrentVersionTransition.StorageClass == "" &&
		r.Expiration.IsDaysNull() &&
		r.AbortIncompleteMultipartUpload.DaysAfterInitiation == 0 &&
		r.DelMarkerExpiration.Days == 0 &&
		!r.NoncurrentVersionExpiration.IsDaysNull() &&
		int(r.NoncurrentVersionExpiration.NoncurrentDays) > 0 &&
		hasNoTagFilters(r)
}

// isAbortMPUShaped is true iff r carries exactly one
// AbortIncompleteMultipartUpload action (positive DaysAfterInitiation), no
// other actions, and no tag filters.
func isAbortMPUShaped(r mlifecycle.Rule) bool {
	return r.Transition.IsNull() &&
		r.NoncurrentVersionTransition.StorageClass == "" &&
		r.Expiration.IsDaysNull() &&
		r.NoncurrentVersionExpiration.IsDaysNull() &&
		r.NoncurrentVersionExpiration.NewerNoncurrentVersions == 0 &&
		r.DelMarkerExpiration.Days == 0 &&
		r.AbortIncompleteMultipartUpload.DaysAfterInitiation > 0 &&
		hasNoTagFilters(r)
}
```

Note: `AbortIncompleteMultipartUpload.DaysAfterInitiation` is an `int` in minio-go (not `ExpirationDays`); the test above wraps it in `ExpirationDays` — **adjust the test and the `Rule` build to match the real field type.** Verify with `grep -n "DaysAfterInitiation" $(go env GOMODCACHE)/github.com/minio/minio-go/v7@v7.0.74/pkg/lifecycle/lifecycle.go` and use the exact type in both test and classifier.

- [ ] **Step 4: Run to verify pass (and no regression)**

Run: `cd apps/backend && go test ./internal/lifecycle/ -v`
Expected: PASS (existing expiration classifier tests still pass; new ones pass).

- [ ] **Step 5: Commit**

```bash
git add apps/backend/internal/lifecycle/classifier.go apps/backend/internal/lifecycle/classifier_test.go
git commit -m "feat(lifecycle): classify noncurrent-expiration and abort-mpu managed families"
```

### Task A4: Processor builds the right minio-go rule per kind

**Files:**
- Modify: `apps/backend/internal/lifecycle/processor.go`
- Test: `apps/backend/internal/lifecycle/processor_test.go` (extend)

- [ ] **Step 1: Write the failing test**

Append to `apps/backend/internal/lifecycle/processor_test.go` (reuse the existing stub `s3API` in that file — inspect it first; it likely captures the last `SetBucketLifecycle` config):

```go
func TestCreateNoncurrentBuildsRule(t *testing.T) {
	stub := newStubS3() // existing helper in processor_test.go; adapt name if different
	p := NewProcessor(func(ctx context.Context) (s3API, error) { return stub, nil })
	rule, err := p.CreateNoncurrent(context.Background(), "b", 30, 3, "uploads/", "actor", "ip")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if rule.Kind != KindNoncurrentExpiration || rule.NoncurrentDays != 30 || rule.NewerNoncurrentVersions != 3 {
		t.Fatalf("rule wrong: %+v", rule)
	}
	saved := stub.lastConfig.Rules[0] // adapt accessor to the stub's capture field
	if int(saved.NoncurrentVersionExpiration.NoncurrentDays) != 30 {
		t.Errorf("minio rule noncurrent days wrong: %+v", saved)
	}
}

func TestCreateAbortMPUBuildsRule(t *testing.T) {
	stub := newStubS3()
	p := NewProcessor(func(ctx context.Context) (s3API, error) { return stub, nil })
	rule, err := p.CreateAbortMPU(context.Background(), "b", 7, "", "actor", "ip")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if rule.Kind != KindAbortIncompleteMPU || rule.DaysAfterInitiation != 7 {
		t.Fatalf("rule wrong: %+v", rule)
	}
}
```

If the existing test file constructs the processor and stub differently, mirror that exact construction — do not invent helpers that don't exist.

- [ ] **Step 2: Run to verify failure**

Run: `cd apps/backend && go test ./internal/lifecycle/ -run 'CreateNoncurrent|CreateAbortMPU' -v`
Expected: FAIL — undefined `CreateNoncurrent`/`CreateAbortMPU`.

- [ ] **Step 3: Implement the two new processor methods**

Add to `processor.go` (the existing `Create` stays as the expiration path). Both reuse the read-modify-write `upsertRule` + `SetBucketLifecycle` flow from `Create`:

```go
// CreateNoncurrent builds and upserts a managed noncurrent-version-expiration
// rule. newerNoncurrent is optional (0 = no retention floor).
func (p *Processor) CreateNoncurrent(ctx context.Context, bucket string, noncurrentDays, newerNoncurrent int, prefix, actor, sourceIP string) (Rule, error) {
	id := generateNoncurrentRuleID(noncurrentDays, prefix)
	payload := map[string]any{
		"bucket": bucket, "rule_id": id, "kind": KindNoncurrentExpiration,
		"noncurrent_days": noncurrentDays, "newer_noncurrent_versions": newerNoncurrent, "prefix": prefix,
	}
	failAudit := p.lifecycleFailAudit(ctx, bucket, payload)
	if err := validateNoncurrent(noncurrentDays, newerNoncurrent); err != nil {
		return Rule{}, failAudit(apierror.New(http.StatusUnprocessableEntity,
			"invalid_lifecycle_rule", err.Error()).WithPointer("/data/attributes/noncurrent_days"))
	}
	newRule := mlifecycle.Rule{
		ID:     id,
		Status: "Enabled",
		NoncurrentVersionExpiration: mlifecycle.NoncurrentVersionExpiration{
			NoncurrentDays:          mlifecycle.ExpirationDays(noncurrentDays),
			NewerNoncurrentVersions: newerNoncurrent,
		},
		RuleFilter: mlifecycle.Filter{Prefix: prefix},
	}
	return p.upsertManaged(ctx, bucket, newRule, payload, failAudit)
}

// CreateAbortMPU builds and upserts a managed abort-incomplete-multipart rule.
func (p *Processor) CreateAbortMPU(ctx context.Context, bucket string, days int, prefix, actor, sourceIP string) (Rule, error) {
	id := generateAbortMPURuleID(days, prefix)
	payload := map[string]any{
		"bucket": bucket, "rule_id": id, "kind": KindAbortIncompleteMPU,
		"days_after_initiation": days, "prefix": prefix,
	}
	failAudit := p.lifecycleFailAudit(ctx, bucket, payload)
	if err := validateDaysAfterInitiation(days); err != nil {
		return Rule{}, failAudit(apierror.New(http.StatusUnprocessableEntity,
			"invalid_lifecycle_rule", err.Error()).WithPointer("/data/attributes/days_after_initiation"))
	}
	newRule := mlifecycle.Rule{
		ID:     id,
		Status: "Enabled",
		AbortIncompleteMultipartUpload: mlifecycle.AbortIncompleteMultipartUpload{
			DaysAfterInitiation: days, // verify type with go doc; wrap if it is ExpirationDays
		},
		RuleFilter: mlifecycle.Filter{Prefix: prefix},
	}
	return p.upsertManaged(ctx, bucket, newRule, payload, failAudit)
}

// lifecycleFailAudit returns the standard failure-audit closure for a
// lifecycle.rule.create attempt with the given payload.
func (p *Processor) lifecycleFailAudit(ctx context.Context, bucket string, payload map[string]any) func(error) error {
	return func(err error) error {
		p.recordAudit(ctx, audit.Event{
			Action: audit.ActionLifecycleRuleCreate, TargetType: "bucket", TargetID: bucket,
			Outcome: audit.OutcomeFailure, ErrorMessage: err.Error(), PayloadSummary: payload,
		})
		return err
	}
}

// upsertManaged performs the read-modify-write upsert shared by every
// managed Create*, then records a success audit row and returns the
// classified rule.
func (p *Processor) upsertManaged(ctx context.Context, bucket string, newRule mlifecycle.Rule, payload map[string]any, failAudit func(error) error) (Rule, error) {
	s3, err := p.clients(ctx)
	if err != nil {
		return Rule{}, failAudit(err)
	}
	cfg, err := s3.GetBucketLifecycle(ctx, bucket)
	if err != nil && !isNoSuchLifecycleConfiguration(err) {
		return Rule{}, failAudit(mapClientError(err, "failed to load lifecycle configuration"))
	}
	if cfg == nil {
		cfg = mlifecycle.NewConfiguration()
	}
	cfg.Rules = upsertRule(cfg.Rules, newRule)
	if err := s3.SetBucketLifecycle(ctx, bucket, cfg); err != nil {
		return Rule{}, failAudit(mapClientError(err, "failed to save lifecycle configuration"))
	}
	p.recordAudit(ctx, audit.Event{
		Action: audit.ActionLifecycleRuleCreate, TargetType: "bucket", TargetID: bucket,
		Outcome: audit.OutcomeSuccess, PayloadSummary: payload,
	})
	return classify(newRule), nil
}
```

Optionally refactor the existing `Create` to delegate to `upsertManaged` (keeps DRY); only do so if the existing `Create` tests still pass unchanged.

- [ ] **Step 4: Run to verify pass**

Run: `cd apps/backend && go test ./internal/lifecycle/ -v`
Expected: PASS (all lifecycle tests).

- [ ] **Step 5: Commit**

```bash
git add apps/backend/internal/lifecycle/processor.go apps/backend/internal/lifecycle/processor_test.go
git commit -m "feat(lifecycle): processor Create paths for noncurrent + abort-mpu kinds"
```

### Task A5: REST decoding by kind + discriminated managed attributes

**Files:**
- Modify: `apps/backend/internal/lifecycle/rest.go`
- Modify: `apps/backend/internal/lifecycle/resource.go`
- Test: `apps/backend/internal/lifecycle/golden_test.go` (extend) and an HTTP-level test if one exists.

- [ ] **Step 1: Extend `CreateRequest` and the managed `MarshalJSON`**

In `rest.go`, replace `CreateRequest` with the superset that carries every kind's fields:

```go
// CreateRequest is the attributes block accepted by
// POST /api/v1/buckets/{name}/lifecycle-rules. kind discriminates which
// other fields are read.
type CreateRequest struct {
	Kind                    string `json:"kind"`
	Days                    int    `json:"days"`                      // expiration
	NoncurrentDays          int    `json:"noncurrent_days"`           // noncurrent-expiration
	NewerNoncurrentVersions int    `json:"newer_noncurrent_versions"` // noncurrent-expiration (optional)
	DaysAfterInitiation     int    `json:"days_after_initiation"`     // abort-incomplete-multipart
	Prefix                  string `json:"prefix"`
}
```

Replace the managed branch of `RuleResource.MarshalJSON` so each kind emits only its own fields:

```go
func (r RuleResource) MarshalJSON() ([]byte, error) {
	if !r.Managed {
		return json.Marshal(struct {
			Managed bool   `json:"managed"`
			Summary string `json:"summary"`
		}{Managed: false, Summary: r.Summary})
	}
	switch r.Kind {
	case KindNoncurrentExpiration:
		return json.Marshal(struct {
			Managed                 bool   `json:"managed"`
			Kind                    string `json:"kind"`
			NoncurrentDays          int    `json:"noncurrent_days"`
			NewerNoncurrentVersions int    `json:"newer_noncurrent_versions"`
			Prefix                  string `json:"prefix"`
		}{true, r.Kind, r.NoncurrentDays, r.NewerNoncurrentVersions, r.Prefix})
	case KindAbortIncompleteMPU:
		return json.Marshal(struct {
			Managed             bool   `json:"managed"`
			Kind                string `json:"kind"`
			DaysAfterInitiation int    `json:"days_after_initiation"`
			Prefix              string `json:"prefix"`
		}{true, r.Kind, r.DaysAfterInitiation, r.Prefix})
	default: // KindExpiration
		return json.Marshal(struct {
			Managed bool   `json:"managed"`
			Kind    string `json:"kind"`
			Days    int    `json:"days"`
			Prefix  string `json:"prefix"`
		}{true, r.Kind, r.Days, r.Prefix})
	}
}
```

- [ ] **Step 2: Switch the create handler on kind**

In `resource.go`, replace the `create` handler's kind check with a dispatch:

```go
func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "name")
	var attrs CreateRequest
	if err := h.dec.Single(r.Body, &attrs); err != nil {
		apierror.Write(w, apierror.StyleJSONAPI, apierror.New(http.StatusBadRequest,
			"bad_request", "Invalid JSON:API request body"))
		return
	}
	actor, ip := actorFromRequest(r)
	var (
		rule Rule
		err  error
	)
	switch attrs.Kind {
	case KindExpiration:
		rule, err = h.p.Create(r.Context(), bucket, attrs.Days, attrs.Prefix, actor, ip)
	case KindNoncurrentExpiration:
		rule, err = h.p.CreateNoncurrent(r.Context(), bucket, attrs.NoncurrentDays, attrs.NewerNoncurrentVersions, attrs.Prefix, actor, ip)
	case KindAbortIncompleteMPU:
		rule, err = h.p.CreateAbortMPU(r.Context(), bucket, attrs.DaysAfterInitiation, attrs.Prefix, actor, ip)
	default:
		apierror.Write(w, apierror.StyleJSONAPI, apierror.New(http.StatusUnprocessableEntity,
			"unsupported_lifecycle_kind",
			"only expiration, noncurrent-expiration, and abort-incomplete-multipart are supported").
			WithPointer("/data/attributes/kind"))
		return
	}
	if err != nil {
		apierror.Write(w, apierror.StyleJSONAPI, err)
		return
	}
	writeSingle(w, h.enc, http.StatusCreated, RuleResource{Rule: rule})
}
```

- [ ] **Step 3: Add golden coverage**

In `golden_test.go`, add cases marshaling a `RuleResource` for each managed kind and assert the JSON matches the discriminated shapes above (follow the existing golden-test structure in that file). Also add an HTTP test asserting `kind:"transition"` → 422 `unsupported_lifecycle_kind` (mirror any existing handler test in the package).

- [ ] **Step 4: Run tests**

Run: `cd apps/backend && go test ./internal/lifecycle/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/backend/internal/lifecycle/rest.go apps/backend/internal/lifecycle/resource.go apps/backend/internal/lifecycle/golden_test.go
git commit -m "feat(lifecycle): kind-discriminated create decoding + managed attribute shapes"
```

### Task A6: Frontend — lifecycle kind selector

**Files:**
- Modify: `apps/frontend/src/features/lifecycle/api.ts`
- Modify: `apps/frontend/src/features/lifecycle/CreateRuleDialog.tsx`
- Modify: `apps/frontend/src/features/lifecycle/LifecycleRulesTab.tsx` (kind badges)
- Test: `apps/frontend/src/features/lifecycle/CreateRuleDialog.test.tsx` (create or extend)

- [ ] **Step 1: Extend the API client and types**

In `api.ts`, extend `LifecycleRule` and add per-kind create helpers:

```ts
export type LifecycleRule = {
  id: string;
  managed: boolean;
  kind?: string;
  days?: number;
  noncurrent_days?: number;
  newer_noncurrent_versions?: number;
  days_after_initiation?: number;
  prefix?: string;
  summary?: string;
};
```

Replace `createRule` with a kind-aware creator (keep the JSON:API envelope shape):

```ts
export type CreateRuleAttrs =
  | { kind: "expiration"; days: number; prefix: string }
  | { kind: "noncurrent-expiration"; noncurrent_days: number; newer_noncurrent_versions: number; prefix: string }
  | { kind: "abort-incomplete-multipart"; days_after_initiation: number; prefix: string };

export async function createRule(bucket: string, attributes: CreateRuleAttrs): Promise<LifecycleRule> {
  const res = await api.post<LifecycleSingleResponse>(
    `/api/v1/buckets/${encodeURIComponent(bucket)}/lifecycle-rules`,
    { data: { type: "lifecycle_rules", attributes } },
  );
  return { ...res.data.attributes, id: res.data.id };
}
```

- [ ] **Step 2: Write the failing test**

In `CreateRuleDialog.test.tsx`, add a test that selecting "Noncurrent versions" reveals a `noncurrent_days` field and submits `{kind:"noncurrent-expiration", noncurrent_days, newer_noncurrent_versions, prefix}`. Mock `createRule`. (Mirror the existing dialog test setup in the file / `UploadDialog.test.tsx`.)

Run: `cd apps/frontend && npm test -- CreateRuleDialog`
Expected: FAIL — no kind selector yet.

- [ ] **Step 3: Implement the discriminated form**

Rewrite `CreateRuleDialog.tsx` to use a `kind` `Select` (shadcn `components/ui/select`) and a Zod discriminated union:

```ts
const ruleSchema = z.discriminatedUnion("kind", [
  z.object({
    kind: z.literal("expiration"),
    days: z.coerce.number().int().min(1).max(10_000),
    prefix: z.string().max(1024),
  }),
  z.object({
    kind: z.literal("noncurrent-expiration"),
    noncurrent_days: z.coerce.number().int().min(1).max(10_000),
    newer_noncurrent_versions: z.coerce.number().int().min(0).max(1000),
    prefix: z.string().max(1024),
  }),
  z.object({
    kind: z.literal("abort-incomplete-multipart"),
    days_after_initiation: z.coerce.number().int().min(1).max(10_000),
    prefix: z.string().max(1024),
  }),
]);
type FormValues = z.infer<typeof ruleSchema>;
```

Watch `kind` and render only the relevant fields. On submit, call `createRule(bucket, values)`. For `noncurrent-expiration`, show a non-blocking inline warning when the bucket's `versioning_enabled` is false (pass a `versioningEnabled?: boolean` prop from the parent tab, which already has the bucket resource). Map error `pointer` (`/data/attributes/noncurrent_days` etc.) to the right field message via `form.setError` in `onError`.

- [ ] **Step 4: Kind badges in the list**

In `LifecycleRulesTab.tsx`, render a badge per managed rule keyed on `kind` ("Expiration" / "Noncurrent" / "Abort MPU") and show the kind-specific summary line (e.g. "Keep 3 newest, expire after 30d" for noncurrent). Unmanaged rows keep the existing `summary` display.

- [ ] **Step 5: Run tests + lint/format**

Run: `cd apps/frontend && npm test -- CreateRuleDialog && npm run lint && npm run format`
Expected: PASS / clean.

- [ ] **Step 6: Commit**

```bash
git add apps/frontend/src/features/lifecycle/
git commit -m "feat(lifecycle-ui): kind selector for noncurrent + abort-mpu rules"
```

---

## Track B — Object Version Browser + Restore

Extends `internal/objects` with per-key version listing (in-memory windowing over the high-level channel API), version-aware download/preview, and restore/delete-version/undelete actions. See design §2; read `context.md` for the verified minio-go facts.

### Task B1: New audit verbs for version mutations

**Files:**
- Modify: `apps/backend/internal/audit/model.go`

- [ ] **Step 1: Add the constants and register them**

In the `const (...)` action block add:

```go
	ActionObjectVersionRestore = "object.version.restore"
	ActionObjectVersionDelete  = "object.version.delete"
	ActionObjectUndelete       = "object.undelete"
```

Add the same three to the slice returned by `AllActions()`.

- [ ] **Step 2: Run the audit action-coverage test**

Run: `cd apps/backend && go test ./internal/audit/ -v`
Expected: PASS (the test that enumerates `AllActions()` stays consistent).

- [ ] **Step 3: Commit**

```bash
git add apps/backend/internal/audit/model.go
git commit -m "feat(audit): add object version restore/delete/undelete verbs"
```

### Task B2: Version domain model + provider mapping

**Files:**
- Modify: `apps/backend/internal/objects/model.go`
- Modify: `apps/backend/internal/objects/provider.go`
- Test: `apps/backend/internal/objects/provider_test.go` (create)

- [ ] **Step 1: Add the model types**

In `model.go` add:

```go
// ObjectVersion is the immutable read view of a single object version or
// delete marker. Size is a pointer so delete markers serialise as null.
type ObjectVersion struct {
	Key            string
	VersionID      string
	Size           *int64
	LastModified   time.Time
	ETag           string
	ContentType    string
	IsLatest       bool
	IsDeleteMarker bool
}

// VersionListResult is one page of an object's version history. NextToken
// is an opaque base64 offset cursor; Truncated is set when the safety cap
// stopped the scan before exhaustion.
type VersionListResult struct {
	Versions  []ObjectVersion
	NextToken string
	Truncated bool
}
```

- [ ] **Step 2: Write the failing provider test**

Create `apps/backend/internal/objects/provider_test.go`:

```go
package objects

import (
	"testing"

	miniogo "github.com/minio/minio-go/v7"
)

func TestVersionFromObjectInfo_DeleteMarker(t *testing.T) {
	v := versionFromObjectInfo(miniogo.ObjectInfo{
		Key: "k", VersionID: "v1", IsDeleteMarker: true, IsLatest: true,
	})
	if v.Size != nil {
		t.Errorf("delete marker Size must be nil, got %v", *v.Size)
	}
	if v.ContentType != "" {
		t.Errorf("delete marker ContentType must be empty, got %q", v.ContentType)
	}
	if !v.IsDeleteMarker || !v.IsLatest {
		t.Errorf("flags wrong: %+v", v)
	}
}

func TestVersionFromObjectInfo_Regular(t *testing.T) {
	v := versionFromObjectInfo(miniogo.ObjectInfo{
		Key: "k", VersionID: "v2", Size: 42, ContentType: "image/jpeg", IsLatest: true,
	})
	if v.Size == nil || *v.Size != 42 {
		t.Fatalf("size wrong: %+v", v)
	}
	if v.ContentType != "image/jpeg" {
		t.Errorf("content type wrong: %q", v.ContentType)
	}
}
```

Run: `cd apps/backend && go test ./internal/objects/ -run VersionFromObjectInfo -v`
Expected: FAIL — undefined `versionFromObjectInfo`.

- [ ] **Step 3: Implement the mapper**

Append to `provider.go`:

```go
// versionFromObjectInfo maps a minio-go ObjectInfo (from a WithVersions
// listing) into the domain ObjectVersion. Delete markers carry no size or
// content-type, so Size is left nil and ContentType empty.
func versionFromObjectInfo(info miniogo.ObjectInfo) ObjectVersion {
	v := ObjectVersion{
		Key:            info.Key,
		VersionID:      info.VersionID,
		LastModified:   info.LastModified,
		ETag:           info.ETag,
		IsLatest:       info.IsLatest,
		IsDeleteMarker: info.IsDeleteMarker,
	}
	if !info.IsDeleteMarker {
		size := info.Size
		v.Size = &size
		v.ContentType = info.ContentType
	}
	return v
}
```

Run: `cd apps/backend && go test ./internal/objects/ -run VersionFromObjectInfo -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add apps/backend/internal/objects/model.go apps/backend/internal/objects/provider.go apps/backend/internal/objects/provider_test.go
git commit -m "feat(objects): ObjectVersion model + provider mapping"
```

### Task B3: Interface split — add the high-level version-list client + copy/remove-with-opts

**Files:**
- Modify: `apps/backend/internal/objects/processor.go`
- Modify: `apps/backend/internal/objects/administrator.go`

The processor currently has one `s3API` (backed by `Core` for `ListObjectsV2`). Version listing needs the **high-level** `Client.ListObjects` channel; restore needs `CopyObject`; delete-version needs `RemoveObject` with a `VersionID`. `RemoveObject(ctx, bucket, key, RemoveObjectOptions)` is already on `s3API`, so reuse it. Add the two missing capabilities.

- [ ] **Step 1: Extend the unexported `s3API` and exported `S3Client` interfaces**

In `processor.go`, add these methods to **both** `s3API` and `S3Client` (keep them identical):

```go
	// ListObjectVersions drains the high-level WithVersions channel for a
	// single key into a slice (newest-first per S3 semantics). Implemented
	// in the wiring adapter via *miniogo.Client.ListObjects.
	ListObjectVersions(ctx context.Context, bucket, key string, maxScan int) ([]miniogo.ObjectInfo, bool, error)
	// CopyObject performs a server-side copy (used by restore).
	CopyObject(ctx context.Context, dst miniogo.CopyDestOptions, src miniogo.CopySrcOptions) (miniogo.UploadInfo, error)
```

`RemoveObject(ctx, bucket, key, miniogo.RemoveObjectOptions)`, `GetObject`, `StatObject`, `PresignedGetObject` already exist on both interfaces — version-id is passed through their existing `opts`.

- [ ] **Step 2: Add administrator helpers**

Append to `administrator.go`:

```go
// maxVersionScan caps how many version entries listObjectVersions will
// drain from the channel for a single key. The version browser is scoped
// to one key whose cardinality is normally tens; the cap bounds a
// pathological key and flips VersionListResult.Truncated when hit.
const maxVersionScan = 10_000

// listObjectVersions returns all versions+delete-markers for exactly key
// (the SDK's prefix listing can match siblings, so the caller filters to
// exact-key matches). The bool return is "truncated" — true when the scan
// stopped at maxVersionScan before the channel closed.
func listObjectVersions(ctx context.Context, s3 s3API, bucket, key string) ([]miniogo.ObjectInfo, bool, error) {
	infos, truncated, err := s3.ListObjectVersions(ctx, bucket, key, maxVersionScan)
	if err != nil {
		return nil, false, fmt.Errorf("objects.listObjectVersions: %w", err)
	}
	out := infos[:0]
	for _, info := range infos {
		if info.Key == key {
			out = append(out, info)
		}
	}
	return out, truncated, nil
}

// copyObjectVersion server-side copies srcVersionID of bucket/key back onto
// the same bucket/key, creating a new current version (the restore op).
func copyObjectVersion(ctx context.Context, s3 s3API, bucket, key, srcVersionID string) (miniogo.UploadInfo, error) {
	info, err := s3.CopyObject(ctx,
		miniogo.CopyDestOptions{Bucket: bucket, Object: key},
		miniogo.CopySrcOptions{Bucket: bucket, Object: key, VersionID: srcVersionID},
	)
	if err != nil {
		return miniogo.UploadInfo{}, fmt.Errorf("objects.copyObjectVersion: %w", err)
	}
	return info, nil
}

// removeObjectVersion permanently deletes a single version id of bucket/key.
func removeObjectVersion(ctx context.Context, s3 s3API, bucket, key, versionID string) error {
	if err := s3.RemoveObject(ctx, bucket, key, miniogo.RemoveObjectOptions{VersionID: versionID}); err != nil {
		return fmt.Errorf("objects.removeObjectVersion: %w", err)
	}
	return nil
}
```

- [ ] **Step 3: Make version-aware variants of stat/get/presign**

Still in `administrator.go`, add `versionID`-carrying variants (leave the existing zero-version helpers for the non-version paths):

```go
// statObjectVersion stats a specific version.
func statObjectVersion(ctx context.Context, s3 s3API, bucket, key, versionID string) (miniogo.ObjectInfo, error) {
	info, err := s3.StatObject(ctx, bucket, key, miniogo.StatObjectOptions{VersionID: versionID})
	if err != nil {
		return miniogo.ObjectInfo{}, fmt.Errorf("objects.statObjectVersion: %w", err)
	}
	return info, nil
}

// getObjectVersion opens a reader against a specific version body.
func getObjectVersion(ctx context.Context, s3 s3API, bucket, key, versionID string) (io.ReadCloser, error) {
	rc, err := s3.GetObject(ctx, bucket, key, miniogo.GetObjectOptions{VersionID: versionID})
	if err != nil {
		return nil, fmt.Errorf("objects.getObjectVersion: %w", err)
	}
	return rc, nil
}
```

For presigned version downloads, pass `versionId` as a query param: in `presignedGet`'s callers add `reqParams.Set("versionId", versionID)` when non-empty (handled in the processor in Task B5).

- [ ] **Step 4: Build the package**

Run: `cd apps/backend && CGO_ENABLED=0 go build ./internal/objects/...`
Expected: FAIL — the existing in-package test stub for `s3API` doesn't implement the two new methods yet. That's fixed in Task B4; if the build only fails in `_test.go`, proceed.

- [ ] **Step 5: Commit**

```bash
git add apps/backend/internal/objects/processor.go apps/backend/internal/objects/administrator.go
git commit -m "feat(objects): s3API gains version-list + copy; admin version helpers"
```

### Task B4: Processor version operations (list/restore/delete/undelete) with TDD

**Files:**
- Modify: `apps/backend/internal/objects/processor.go`
- Test: `apps/backend/internal/objects/processor_test.go` (extend; update the in-package stub)

- [ ] **Step 1: Extend the test stub, then write failing tests**

Find the existing `s3API` stub in `processor_test.go`. Add fields + methods so it implements the two new interface methods and can be programmed per test:

```go
// add to the stub struct:
//   versions      []miniogo.ObjectInfo
//   versTruncated bool
//   copyCalledSrc string            // captures CopySrcOptions.VersionID
//   removedVerIDs []string          // captures RemoveObject VersionIDs

func (s *stubS3) ListObjectVersions(_ context.Context, _, _ string, _ int) ([]miniogo.ObjectInfo, bool, error) {
	return s.versions, s.versTruncated, nil
}
func (s *stubS3) CopyObject(_ context.Context, _ miniogo.CopyDestOptions, src miniogo.CopySrcOptions) (miniogo.UploadInfo, error) {
	s.copyCalledSrc = src.VersionID
	return miniogo.UploadInfo{Key: src.Object, VersionID: "new-current"}, nil
}
// RemoveObject already exists on the stub; make it append opts.VersionID to s.removedVerIDs.
```

(Adapt names to the actual stub. If the stub is shared with other test files, keep the additions backward-compatible.)

Add the behavior tests:

```go
func TestListVersions_WindowAndDeleteMarker(t *testing.T) {
	sz := int64(10)
	stub := &stubS3{versions: []miniogo.ObjectInfo{
		{Key: "k", VersionID: "v3", IsLatest: true, IsDeleteMarker: true},
		{Key: "k", VersionID: "v2", Size: sz},
		{Key: "k", VersionID: "v1", Size: sz},
		{Key: "other", VersionID: "x"}, // sibling: must be filtered out
	}}
	p := newTestProcessor(stub) // mirror existing helper
	res, err := p.ListVersions(context.Background(), "b", "k", 2, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Versions) != 2 || res.Versions[0].VersionID != "v3" {
		t.Fatalf("first page wrong: %+v", res.Versions)
	}
	if !res.Versions[0].IsDeleteMarker || res.Versions[0].Size != nil {
		t.Errorf("delete marker not mapped: %+v", res.Versions[0])
	}
	if res.NextToken == "" {
		t.Fatal("expected a next token (3 matching versions, page size 2)")
	}
	// second page
	res2, err := p.ListVersions(context.Background(), "b", "k", 2, res.NextToken)
	if err != nil {
		t.Fatal(err)
	}
	if len(res2.Versions) != 1 || res2.Versions[0].VersionID != "v1" || res2.NextToken != "" {
		t.Fatalf("second page wrong: %+v token=%q", res2.Versions, res2.NextToken)
	}
}

func TestRestoreVersion_RejectsDeleteMarker(t *testing.T) {
	stub := &stubS3{versions: []miniogo.ObjectInfo{
		{Key: "k", VersionID: "vdm", IsDeleteMarker: true},
	}}
	p := newTestProcessor(stub)
	_, err := p.RestoreVersion(context.Background(), "b", "k", "vdm", "actor", "ip")
	assertAPIError(t, err, 422, "cannot_restore_delete_marker") // mirror existing assert helper
	if stub.copyCalledSrc != "" {
		t.Error("CopyObject must not be called for a delete marker")
	}
}

func TestRestoreVersion_CopiesWithVersionID(t *testing.T) {
	stub := &stubS3{versions: []miniogo.ObjectInfo{
		{Key: "k", VersionID: "v2", Size: 1},
		{Key: "k", VersionID: "v1", Size: 1},
	}}
	p := newTestProcessor(stub)
	out, err := p.RestoreVersion(context.Background(), "b", "k", "v2", "actor", "ip")
	if err != nil {
		t.Fatal(err)
	}
	if stub.copyCalledSrc != "v2" {
		t.Errorf("expected copy from v2, got %q", stub.copyCalledSrc)
	}
	if out.VersionID != "new-current" {
		t.Errorf("expected new current version, got %+v", out)
	}
}

func TestDeleteVersion_ConfirmGate(t *testing.T) {
	stub := &stubS3{}
	p := newTestProcessor(stub)
	if err := p.DeleteVersion(context.Background(), "b", "k", "v1", false, "a", "ip"); err == nil {
		t.Fatal("expected 422 when confirm=false")
	}
	if len(stub.removedVerIDs) != 0 {
		t.Error("RemoveObject must not be called without confirm")
	}
	if err := p.DeleteVersion(context.Background(), "b", "k", "v1", true, "a", "ip"); err != nil {
		t.Fatal(err)
	}
	if len(stub.removedVerIDs) != 1 || stub.removedVerIDs[0] != "v1" {
		t.Errorf("removed wrong: %+v", stub.removedVerIDs)
	}
}

func TestUndelete_RejectsNonDeleteMarked(t *testing.T) {
	stub := &stubS3{versions: []miniogo.ObjectInfo{
		{Key: "k", VersionID: "v2", IsLatest: true, Size: 1}, // latest is NOT a delete marker
	}}
	p := newTestProcessor(stub)
	_, err := p.Undelete(context.Background(), "b", "k", "a", "ip")
	assertAPIError(t, err, 422, "not_delete_marked")
}

func TestUndelete_RemovesLatestDeleteMarker(t *testing.T) {
	stub := &stubS3{versions: []miniogo.ObjectInfo{
		{Key: "k", VersionID: "vdm", IsLatest: true, IsDeleteMarker: true},
		{Key: "k", VersionID: "v1", Size: 1},
	}}
	p := newTestProcessor(stub)
	out, err := p.Undelete(context.Background(), "b", "k", "a", "ip")
	if err != nil {
		t.Fatal(err)
	}
	if len(stub.removedVerIDs) != 1 || stub.removedVerIDs[0] != "vdm" {
		t.Errorf("must remove the delete marker, got %+v", stub.removedVerIDs)
	}
	if out.VersionID != "v1" {
		t.Errorf("undelete should report the newly-exposed version v1, got %q", out.VersionID)
	}
}
```

Run: `cd apps/backend && go test ./internal/objects/ -run 'Versions|Restore|DeleteVersion|Undelete' -v`
Expected: FAIL — undefined methods.

- [ ] **Step 2: Implement the cursor helpers + processor methods**

Append to `processor.go`:

```go
// encodeVersionToken / decodeVersionToken implement the opaque base64
// offset cursor for the in-memory version window. The token's only
// meaning is "resume at this index"; callers must treat it as opaque.
func encodeVersionToken(offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(offset)))
}

func decodeVersionToken(tok string) (int, error) {
	if tok == "" {
		return 0, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(tok)
	if err != nil {
		return 0, apierror.New(http.StatusBadRequest, "bad_request", "invalid page token")
	}
	n, err := strconv.Atoi(string(b))
	if err != nil || n < 0 {
		return 0, apierror.New(http.StatusBadRequest, "bad_request", "invalid page token")
	}
	return n, nil
}

// ListVersions returns one window of the version history for bucket/key,
// newest-first. pageToken is the opaque offset cursor from a prior call.
func (p *Processor) ListVersions(ctx context.Context, bucket, key string, pageSize int, pageToken string) (VersionListResult, error) {
	if err := ValidateObjectKey(key); err != nil {
		return VersionListResult{}, apierror.New(http.StatusBadRequest, "object_invalid_key", err.Error())
	}
	pageSize = clampPageSize(pageSize)
	offset, err := decodeVersionToken(pageToken)
	if err != nil {
		return VersionListResult{}, err
	}
	s3, err := p.clients(ctx)
	if err != nil {
		return VersionListResult{}, err
	}
	infos, truncated, err := listObjectVersions(ctx, s3, bucket, key)
	if err != nil {
		return VersionListResult{}, mapClientError(err, "failed to list object versions")
	}
	all := make([]ObjectVersion, 0, len(infos))
	for _, info := range infos {
		all = append(all, versionFromObjectInfo(info))
	}
	if offset > len(all) {
		offset = len(all)
	}
	end := offset + pageSize
	next := ""
	if end < len(all) {
		next = encodeVersionToken(end)
	} else {
		end = len(all)
	}
	return VersionListResult{Versions: all[offset:end], NextToken: next, Truncated: truncated}, nil
}

// RestoreVersion server-side copies versionID back onto key, creating a new
// current version. Rejects a delete-marker source up front.
func (p *Processor) RestoreVersion(ctx context.Context, bucket, key, versionID, actor, sourceIP string) (ObjectVersion, error) {
	payload := map[string]any{"bucket": bucket, "key": key, "version_id": versionID}
	failAudit := p.objectFailAudit(ctx, audit.ActionObjectVersionRestore, bucket, key, payload)
	if err := ValidateObjectKey(key); err != nil {
		return ObjectVersion{}, failAudit(apierror.New(http.StatusBadRequest, "object_invalid_key", err.Error()))
	}
	s3, err := p.clients(ctx)
	if err != nil {
		return ObjectVersion{}, failAudit(err)
	}
	infos, _, err := listObjectVersions(ctx, s3, bucket, key)
	if err != nil {
		return ObjectVersion{}, failAudit(mapClientError(err, "failed to resolve version"))
	}
	src, ok := findVersion(infos, versionID)
	if !ok {
		return ObjectVersion{}, failAudit(apierror.NotFound("version"))
	}
	if src.IsDeleteMarker {
		return ObjectVersion{}, failAudit(apierror.New(http.StatusUnprocessableEntity,
			"cannot_restore_delete_marker", "cannot restore a delete marker"))
	}
	if _, err := copyObjectVersion(ctx, s3, bucket, key, versionID); err != nil {
		return ObjectVersion{}, failAudit(mapClientError(err, "failed to restore version"))
	}
	info, err := statObject(ctx, s3, bucket, key)
	if err != nil {
		return ObjectVersion{}, failAudit(mapClientError(err, "restored but failed to stat new current version"))
	}
	p.recordAudit(ctx, audit.Event{
		Actor: actor, SourceIP: sourceIP, Action: audit.ActionObjectVersionRestore,
		TargetType: "object", TargetID: bucket + "/" + key,
		Outcome: audit.OutcomeSuccess, PayloadSummary: payload,
	})
	return versionFromObjectInfo(info), nil
}

// DeleteVersion permanently removes a single version. confirm must be true.
func (p *Processor) DeleteVersion(ctx context.Context, bucket, key, versionID string, confirm bool, actor, sourceIP string) error {
	payload := map[string]any{"bucket": bucket, "key": key, "version_id": versionID}
	failAudit := p.objectFailAudit(ctx, audit.ActionObjectVersionDelete, bucket, key, payload)
	if err := ValidateObjectKey(key); err != nil {
		return failAudit(apierror.New(http.StatusBadRequest, "object_invalid_key", err.Error()))
	}
	if !confirm {
		return failAudit(apierror.New(http.StatusUnprocessableEntity, "bad_request",
			"permanent version delete requires confirm:true"))
	}
	s3, err := p.clients(ctx)
	if err != nil {
		return failAudit(err)
	}
	if err := removeObjectVersion(ctx, s3, bucket, key, versionID); err != nil {
		return failAudit(mapClientError(err, "failed to delete version"))
	}
	p.recordAudit(ctx, audit.Event{
		Actor: actor, SourceIP: sourceIP, Action: audit.ActionObjectVersionDelete,
		TargetType: "object", TargetID: bucket + "/" + key,
		Outcome: audit.OutcomeSuccess, PayloadSummary: payload,
	})
	return nil
}

// Undelete removes the latest delete marker for key (re-exposing the prior
// version). Rejects when the latest version is not a delete marker.
func (p *Processor) Undelete(ctx context.Context, bucket, key, actor, sourceIP string) (ObjectVersion, error) {
	payload := map[string]any{"bucket": bucket, "key": key}
	failAudit := p.objectFailAudit(ctx, audit.ActionObjectUndelete, bucket, key, payload)
	if err := ValidateObjectKey(key); err != nil {
		return ObjectVersion{}, failAudit(apierror.New(http.StatusBadRequest, "object_invalid_key", err.Error()))
	}
	s3, err := p.clients(ctx)
	if err != nil {
		return ObjectVersion{}, failAudit(err)
	}
	infos, _, err := listObjectVersions(ctx, s3, bucket, key)
	if err != nil {
		return ObjectVersion{}, failAudit(mapClientError(err, "failed to resolve versions"))
	}
	latest, ok := findLatest(infos)
	if !ok || !latest.IsDeleteMarker {
		return ObjectVersion{}, failAudit(apierror.New(http.StatusUnprocessableEntity,
			"not_delete_marked", "latest version is not a delete marker"))
	}
	if err := removeObjectVersion(ctx, s3, bucket, key, latest.VersionID); err != nil {
		return ObjectVersion{}, failAudit(mapClientError(err, "failed to remove delete marker"))
	}
	exposed := ObjectVersion{Key: key}
	if newLatest, ok := findNextNonMarker(infos, latest.VersionID); ok {
		exposed = versionFromObjectInfo(newLatest)
	}
	p.recordAudit(ctx, audit.Event{
		Actor: actor, SourceIP: sourceIP, Action: audit.ActionObjectUndelete,
		TargetType: "object", TargetID: bucket + "/" + key,
		Outcome: audit.OutcomeSuccess, PayloadSummary: payload,
	})
	return exposed, nil
}

// objectFailAudit returns the standard failure-audit closure for an object
// action with the given verb and target.
func (p *Processor) objectFailAudit(ctx context.Context, action, bucket, key string, payload map[string]any) func(error) error {
	return func(err error) error {
		p.recordAudit(ctx, audit.Event{
			Action: action, TargetType: "object", TargetID: bucket + "/" + key,
			Outcome: audit.OutcomeFailure, ErrorMessage: err.Error(), PayloadSummary: payload,
		})
		return err
	}
}

// findVersion returns the ObjectInfo whose VersionID matches.
func findVersion(infos []miniogo.ObjectInfo, versionID string) (miniogo.ObjectInfo, bool) {
	for _, info := range infos {
		if info.VersionID == versionID {
			return info, true
		}
	}
	return miniogo.ObjectInfo{}, false
}

// findLatest returns the IsLatest entry (falling back to the first, since
// the WithVersions listing is newest-first).
func findLatest(infos []miniogo.ObjectInfo) (miniogo.ObjectInfo, bool) {
	for _, info := range infos {
		if info.IsLatest {
			return info, true
		}
	}
	if len(infos) > 0 {
		return infos[0], true
	}
	return miniogo.ObjectInfo{}, false
}

// findNextNonMarker returns the newest non-delete-marker version after the
// removed delete marker (the version that becomes current).
func findNextNonMarker(infos []miniogo.ObjectInfo, removedVersionID string) (miniogo.ObjectInfo, bool) {
	for _, info := range infos {
		if info.VersionID == removedVersionID {
			continue
		}
		if !info.IsDeleteMarker {
			return info, true
		}
	}
	return miniogo.ObjectInfo{}, false
}
```

Add `"encoding/base64"` and `"strconv"` to the `processor.go` imports.

- [ ] **Step 3: Run tests to verify pass**

Run: `cd apps/backend && go test ./internal/objects/ -run 'Versions|Restore|DeleteVersion|Undelete' -v`
Expected: PASS.

- [ ] **Step 4: Add audit payload coverage**

In `audit_events_test.go` (objects), add cases asserting restore/delete-version/undelete emit the right verb and that the payload contains only `bucket/key/version_id` (no URLs/documents). Mirror the existing pattern in that file.

Run: `cd apps/backend && go test ./internal/objects/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/backend/internal/objects/processor.go apps/backend/internal/objects/processor_test.go apps/backend/internal/objects/audit_events_test.go
git commit -m "feat(objects): ListVersions/RestoreVersion/DeleteVersion/Undelete with audit"
```

### Task B5: Version-aware download/preview + REST surface

**Files:**
- Modify: `apps/backend/internal/objects/processor.go` (version-aware Download/Presigned)
- Modify: `apps/backend/internal/objects/resource.go`
- Modify: `apps/backend/internal/objects/rest.go`
- Test: `apps/backend/internal/objects/resource_test.go` (extend)

- [ ] **Step 1: Thread `versionID` through Download / PresignedURL**

In `processor.go`, change `Download` and `PresignedURL` signatures to accept a trailing `versionID string` (empty = current version), using `statObjectVersion`/`getObjectVersion` when set and adding `versionId` to the presign `reqParams`. Update existing call sites in `resource.go` to pass `""` where they don't have a version. Keep the audit action `ActionObjectDownloadProxy`; add `version_id` to the payload when non-empty.

```go
func (p *Processor) Download(ctx context.Context, bucket, key, versionID, actor, sourceIP string) (io.ReadCloser, Entry, error) {
	// ... same as today, but:
	//   info, err := statObjectVersion(ctx, s3, bucket, key, versionID)
	//   rc, err := getObjectVersion(ctx, s3, bucket, key, versionID)
	// and add "version_id": versionID to payload when versionID != "".
}

func (p *Processor) PresignedURL(ctx context.Context, bucket, key, versionID string, ttl time.Duration) (string, time.Time, error) {
	// build url.Values; if versionID != "" { params.Set("versionId", versionID) }
	// u, err := presignedGet(ctx, s3, bucket, key, ttl, params)
}
```

- [ ] **Step 2: Add the new routes + handlers**

In `resource.go` `Routes`, register:

```go
		r.Get("/buckets/{bucket}/objects/versions", h.listVersions)
		r.Post("/buckets/{bucket}/objects/restore-version", h.restoreVersion)
		r.Delete("/buckets/{bucket}/objects/version", h.deleteVersion)
		r.Post("/buckets/{bucket}/objects/undelete", h.undelete)
```

Implement the handlers. `listVersions` reads `key`, `page[size]`, `page[token]` from the query and renders a JSON:API collection of `object_versions` with `meta.page.next_token`. The action handlers decode plain-JSON bodies and render `apierror.StyleAction` errors:

```go
func (h *handler) listVersions(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	q := r.URL.Query()
	key := q.Get("key")
	pageSize, _ := strconv.Atoi(q.Get("page[size]"))
	res, err := h.p.ListVersions(r.Context(), bucket, key, pageSize, q.Get("page[token]"))
	if err != nil {
		apierror.Write(w, apierror.StyleJSONAPI, err)
		return
	}
	resources := make([]jsonapi.Resource, 0, len(res.Versions))
	for _, v := range res.Versions {
		resources = append(resources, versionResource{ObjectVersion: v})
	}
	meta := &jsonapi.Meta{Page: &jsonapi.Page{
		Size: clampPageSize(pageSize), TotalRecords: len(resources), NextToken: res.NextToken,
	}}
	w.Header().Set("Content-Type", "application/vnd.api+json")
	w.WriteHeader(http.StatusOK)
	_ = h.enc.Collection(w, resources, meta, nil)
}

func (h *handler) restoreVersion(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	var body RestoreVersionRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.StyleAction, apierror.New(http.StatusBadRequest, "bad_request", "invalid JSON body"))
		return
	}
	actor, ip := actorFromRequest(r)
	v, err := h.p.RestoreVersion(r.Context(), bucket, body.Key, body.VersionID, actor, ip)
	if err != nil {
		apierror.Write(w, apierror.StyleAction, err)
		return
	}
	writeActionJSON(w, http.StatusOK, map[string]any{
		"key": v.Key, "version_id": v.VersionID, "restored_from": body.VersionID,
	})
}

func (h *handler) deleteVersion(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	q := r.URL.Query()
	var body ConfirmRequest
	_ = json.NewDecoder(r.Body).Decode(&body) // body optional-but-required confirm
	actor, ip := actorFromRequest(r)
	if err := h.p.DeleteVersion(r.Context(), bucket, q.Get("key"), q.Get("version_id"), body.Confirm, actor, ip); err != nil {
		apierror.Write(w, apierror.StyleAction, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) undelete(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	var body UndeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.Write(w, apierror.StyleAction, apierror.New(http.StatusBadRequest, "bad_request", "invalid JSON body"))
		return
	}
	actor, ip := actorFromRequest(r)
	v, err := h.p.Undelete(r.Context(), bucket, body.Key, actor, ip)
	if err != nil {
		apierror.Write(w, apierror.StyleAction, err)
		return
	}
	writeActionJSON(w, http.StatusOK, map[string]any{"key": v.Key, "version_id": v.VersionID})
}

// writeActionJSON writes a plain-JSON action response at status.
func writeActionJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
```

Also thread `version_id` into the existing `download` handler: read `r.URL.Query().Get("version_id")` and pass to `PresignedURL` / `Download`.

- [ ] **Step 3: Add the wire types + version resource**

In `rest.go` add the `object_versions` resource and request bodies:

```go
// versionResource is the JSON:API wrapper for an ObjectVersion. The ID is
// "<key>@<version_id>" per the API contract.
type versionResource struct {
	ObjectVersion
}

func (versionResource) ResourceType() string { return "object_versions" }
func (v versionResource) ResourceID() string  { return v.Key + "@" + v.VersionID }

func (v versionResource) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Key            string    `json:"key"`
		VersionID      string    `json:"version_id"`
		Size           *int64    `json:"size"`
		LastModified   time.Time `json:"last_modified"`
		ETag           string    `json:"etag,omitempty"`
		ContentType    string    `json:"content_type,omitempty"`
		IsLatest       bool      `json:"is_latest"`
		IsDeleteMarker bool      `json:"is_delete_marker"`
	}{v.Key, v.VersionID, v.Size, v.LastModified, v.ETag, v.ContentType, v.IsLatest, v.IsDeleteMarker})
}

// RestoreVersionRequest is the body for POST .../objects/restore-version.
type RestoreVersionRequest struct {
	Key       string `json:"key"`
	VersionID string `json:"version_id"`
}

// ConfirmRequest is the body for DELETE .../objects/version.
type ConfirmRequest struct {
	Confirm bool `json:"confirm"`
}

// UndeleteRequest is the body for POST .../objects/undelete.
type UndeleteRequest struct {
	Key string `json:"key"`
}
```

- [ ] **Step 4: Tests + golden**

Add a golden test marshaling `versionResource` for a regular version and a delete marker (asserting `size: null` for the marker). Add a `resource_test.go` HTTP test for `listVersions` happy path (stub returns a couple versions, assert `data[].type == "object_versions"` and `meta.page.next_token`).

Run: `cd apps/backend && go test ./internal/objects/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/backend/internal/objects/
git commit -m "feat(objects): version REST endpoints + version-aware download/preview"
```

### Task B6: Wire the version-list + copy adapter in cmd/harbormaster

**Files:**
- Modify: `apps/backend/cmd/harbormaster/audit_adapter.go`
- Test: `apps/backend/cmd/harbormaster/serve_test.go` (build smoke only)

- [ ] **Step 1: Implement the two new adapter methods on `objectS3Adapter`**

`objectS3Adapter` embeds `*miniogo.Client` and already routes `ListObjectsV2` through `Core`. Add:

```go
// ListObjectVersions drains the high-level WithVersions channel for a
// single key into a slice. The bool return is "truncated" — true when the
// scan hit maxScan before the channel closed.
func (a objectS3Adapter) ListObjectVersions(ctx context.Context, bucket, key string, maxScan int) ([]miniogo.ObjectInfo, bool, error) {
	ch := a.Client.ListObjects(ctx, bucket, miniogo.ListObjectsOptions{
		Prefix: key, WithVersions: true,
	})
	out := make([]miniogo.ObjectInfo, 0, 16)
	truncated := false
	for info := range ch {
		if info.Err != nil {
			return nil, false, info.Err
		}
		if len(out) >= maxScan {
			truncated = true
			break
		}
		out = append(out, info)
	}
	return out, truncated, nil
}

// CopyObject delegates to the embedded high-level client (server-side copy).
func (a objectS3Adapter) CopyObject(ctx context.Context, dst miniogo.CopyDestOptions, src miniogo.CopySrcOptions) (miniogo.UploadInfo, error) {
	return a.Client.CopyObject(ctx, dst, src)
}
```

The compile-time anchor `var _ objects.S3Client = objectS3Adapter{}` already exists — it will now fail to build until both methods are present, which is the safety net.

Note: when `truncated` breaks the loop early, the channel goroutine may leak. To drain it, replace the `break` with a flag and continue ranging (discarding) — or use a cancelable context. Simplest correct form: derive `cctx, cancel := context.WithCancel(ctx)`, pass `cctx` to `ListObjects`, and `cancel()` after the loop (deferred); on early stop, `cancel()` unblocks the producer. Implement that.

- [ ] **Step 2: Build everything**

Run: `cd apps/backend && CGO_ENABLED=0 go build ./... && go vet ./...`
Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add apps/backend/cmd/harbormaster/audit_adapter.go
git commit -m "feat(objects): wire high-level version listing + copy adapter"
```

### Task B7: Frontend — version history sheet + actions

**Files:**
- Modify: `apps/frontend/src/lib/api/keys.ts` (add `objectsKeys.versions`)
- Modify: `apps/frontend/src/features/objects/api.ts`
- Modify: `apps/frontend/src/features/objects/types.ts`
- Create: `apps/frontend/src/features/objects/ObjectVersionsSheet.tsx`
- Modify: `apps/frontend/src/features/objects/VirtualizedObjectList.tsx` (Versions action)
- Modify: `apps/frontend/src/features/objects/ObjectBrowserPage.tsx` (wire the sheet)
- Test: `apps/frontend/src/features/objects/ObjectVersionsSheet.test.tsx` (create)

- [ ] **Step 1: Query key + API + types**

`keys.ts`: add to `objectsKeys`:

```ts
  versions: (bucket: string, key: string) => ["objects", bucket, "versions", key] as const,
```

`types.ts`: add the version wire types:

```ts
export type ObjectVersion = {
  key: string;
  version_id: string;
  size: number | null;
  last_modified: string;
  etag?: string;
  content_type?: string;
  is_latest: boolean;
  is_delete_marker: boolean;
};

export type ObjectVersionListResponse = {
  data: Array<{ type: "object_versions"; id: string; attributes: ObjectVersion }>;
  meta?: { page?: { size: number; next_token?: string } };
};
```

`api.ts`: add the version client functions:

```ts
export async function listVersions(bucket: string, key: string, pageToken?: string) {
  const sp = new URLSearchParams({ key, "page[size]": "100" });
  if (pageToken) sp.set("page[token]", pageToken);
  return api.get<ObjectVersionListResponse>(
    `/api/v1/buckets/${encodeURIComponent(bucket)}/objects/versions?${sp.toString()}`,
  );
}

export async function restoreVersion(bucket: string, key: string, versionId: string) {
  return api.post<{ key: string; version_id: string; restored_from: string }>(
    `/api/v1/buckets/${encodeURIComponent(bucket)}/objects/restore-version`,
    { key, version_id: versionId },
  );
}

export async function deleteVersion(bucket: string, key: string, versionId: string) {
  const sp = new URLSearchParams({ key, version_id: versionId });
  await api.delete<void>(
    `/api/v1/buckets/${encodeURIComponent(bucket)}/objects/version?${sp.toString()}`,
    { confirm: true },
  );
}

export async function undeleteObject(bucket: string, key: string) {
  return api.post<{ key: string; version_id: string }>(
    `/api/v1/buckets/${encodeURIComponent(bucket)}/objects/undelete`,
    { key },
  );
}

// versionDownloadURL adds version_id to the existing download endpoint.
export function versionDownloadURL(bucket: string, key: string, versionId: string): string {
  const sp = new URLSearchParams({ key, version_id: versionId });
  return `/api/v1/buckets/${encodeURIComponent(bucket)}/objects/download?${sp.toString()}`;
}
```

- [ ] **Step 2: Write the failing sheet test**

Create `ObjectVersionsSheet.test.tsx`: render the sheet with a mocked `listVersions` returning two versions (one delete marker), assert it shows version rows, a "Latest" badge, a "Delete marker" badge, and that clicking Restore on a non-marker calls `restoreVersion` (after confirming). Mirror existing dialog tests' QueryClient + mock setup.

Run: `cd apps/frontend && npm test -- ObjectVersionsSheet`
Expected: FAIL — component doesn't exist.

- [ ] **Step 3: Implement `ObjectVersionsSheet.tsx`**

Build a shadcn `Sheet` (`components/ui/sheet`) driven by `useInfiniteQuery` on `objectsKeys.versions(bucket, key)` over `next_token`. Render a table (version id, size, modified, latest/delete-marker badges) with per-row Download (anchor to `versionDownloadURL`), Preview, Restore, Delete-version buttons. Restore + Delete-version each open a confirmation `Dialog` with distinct, irreversible-action wording (Delete-version styled destructive). Mutations call the api fns and on success invalidate `objectsKeys.versions(bucket,key)` and `objectsKeys.list(bucket, prefix)`. Restore disabled for delete-marker rows. When the latest row is a delete marker, show an Undelete button calling `undeleteObject`.

- [ ] **Step 4: Add the Versions row action + wire the sheet**

In `VirtualizedObjectList.tsx`, add a per-entry-row "Versions" button (`History` icon from lucide-react) calling a new `onVersions(key: string)` prop. In `ObjectBrowserPage.tsx`, hold `versionsKey` state, render `<ObjectVersionsSheet bucket={bucket} objectKey={versionsKey} prefix={prefix} open={...} onOpenChange={...} />`, and pass `onVersions={setVersionsKey}` to the list.

- [ ] **Step 5: Run tests + lint/format**

Run: `cd apps/frontend && npm test -- objects && npm run lint && npm run format`
Expected: PASS / clean.

- [ ] **Step 6: Commit**

```bash
git add apps/frontend/src/features/objects/ apps/frontend/src/lib/api/keys.ts
git commit -m "feat(objects-ui): version history sheet with restore/delete/undelete"
```

---
