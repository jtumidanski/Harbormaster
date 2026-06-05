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

## Track C — Custom / Inline IAM Policy Editor

Adds a full canned-policy CRUD HTTP surface to `internal/policies` (currently a library: `templates.go`, `materializer.go`, `bucket_canned.go` — all preserved). Follows the standard DDD layering used by `internal/users`. The four madmin calls (`ListCannedPolicies`, `InfoCannedPolicy`, `AddCannedPolicy`, `RemoveCannedPolicy`) back a new unexported `adminAPI`; `policy_in_use` detection enumerates users (`ListUsers`) and groups (`ListGroups`/`GetGroupDescription`). See design §4.

### Task C1: New audit verbs for policy mutations

**Files:**
- Modify: `apps/backend/internal/audit/model.go`

- [ ] **Step 1: Add the constants and register them**

In the `const (...)` action block in `model.go` (after `ActionLifecycleRuleDelete`), add:

```go
	ActionPolicyCreate          = "policy.create"
	ActionPolicyUpdate          = "policy.update"
	ActionPolicyDelete          = "policy.delete"
```

Add the same three to the slice returned by `AllActions()` (after `ActionLifecycleRuleDelete`).

- [ ] **Step 2: Run the audit action-coverage test**

Run: `cd apps/backend && go test ./internal/audit/ -v`
Expected: PASS (the test that enumerates `AllActions()` stays consistent).

- [ ] **Step 3: Commit**

```bash
git add apps/backend/internal/audit/model.go
git commit -m "feat(audit): add policy create/update/delete verbs"
```

### Task C2: Policy domain model, origin classifier, and validators

**Files:**
- Create: `apps/backend/internal/policies/model.go`
- Create: `apps/backend/internal/policies/classifier.go`
- Create: `apps/backend/internal/policies/builder.go`
- Test: `apps/backend/internal/policies/classifier_test.go` (create)
- Test: `apps/backend/internal/policies/builder_test.go` (create)

- [ ] **Step 1: Write the model**

Create `apps/backend/internal/policies/model.go`:

```go
package policies

import "encoding/json"

// Origin classifies a canned policy by provenance. Only custom policies are
// editable/deletable through Harbormaster.
const (
	OriginBuiltin  = "minio-builtin"
	OriginTemplate = "harbormaster-template"
	OriginCustom   = "custom"
)

// Policy is the immutable read view of a single canned policy. Editable is
// true exactly when Origin == OriginCustom. StatementSummary is a short,
// human-readable précis of the first statement (never the full document).
type Policy struct {
	Name             string
	Origin           string
	Editable         bool
	StatementSummary string
}

// PolicyDetail adds the full IAM document to a Policy (returned by Get only).
type PolicyDetail struct {
	Policy
	Document json.RawMessage
}
```

- [ ] **Step 2: Write the failing classifier test**

Create `apps/backend/internal/policies/classifier_test.go`:

```go
package policies

import "testing"

func TestOriginFor(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"readonly", OriginBuiltin},
		{"consoleAdmin", OriginBuiltin},
		{"diagnostics", OriginBuiltin},
		{"harbormaster-read-only", OriginTemplate},
		{"harbormaster-backup-target-photos", OriginTemplate},
		{"my-custom-policy", OriginCustom},
	}
	for _, c := range cases {
		if got := OriginFor(c.name); got != c.want {
			t.Errorf("OriginFor(%q)=%q want %q", c.name, got, c.want)
		}
	}
}

func TestEditableForOnlyCustom(t *testing.T) {
	if EditableFor("readonly") {
		t.Error("builtin must not be editable")
	}
	if EditableFor("harbormaster-read-only") {
		t.Error("template must not be editable")
	}
	if !EditableFor("my-custom-policy") {
		t.Error("custom must be editable")
	}
}
```

Run: `cd apps/backend && go test ./internal/policies/ -run 'OriginFor|EditableFor' -v`
Expected: FAIL — undefined `OriginFor`/`EditableFor`.

- [ ] **Step 3: Implement the classifier**

Create `apps/backend/internal/policies/classifier.go`:

```go
package policies

import "strings"

// builtinPolicies is the set of MinIO server built-in canned policy names.
// These are managed by MinIO itself and are never editable through
// Harbormaster.
var builtinPolicies = map[string]struct{}{
	"readonly":     {},
	"readwrite":    {},
	"writeonly":    {},
	"consoleAdmin": {},
	"diagnostics":  {},
}

// templateMaterializedNames returns the set of canonical names every bundled
// template materializes to. Parameterized templates (backup-target) own the
// "harbormaster-<template>-" prefix; non-parameterized ones own the exact
// "harbormaster-<template>" name. We treat the whole "harbormaster-" prefix
// as template-owned so operators cannot shadow a template name with a custom
// policy.
func isTemplateName(name string) bool {
	return strings.HasPrefix(name, "harbormaster-")
}

// OriginFor classifies a canned policy name by provenance from the name
// alone: MinIO built-ins first, then the Harbormaster-template prefix, else
// custom.
func OriginFor(name string) string {
	if _, ok := builtinPolicies[name]; ok {
		return OriginBuiltin
	}
	if isTemplateName(name) {
		return OriginTemplate
	}
	return OriginCustom
}

// EditableFor reports whether a policy may be edited/deleted through
// Harbormaster (custom-origin only).
func EditableFor(name string) bool {
	return OriginFor(name) == OriginCustom
}

// IsBuiltin reports whether name is a MinIO server built-in policy.
func IsBuiltin(name string) bool {
	_, ok := builtinPolicies[name]
	return ok
}
```

Run: `cd apps/backend && go test ./internal/policies/ -run 'OriginFor|EditableFor' -v`
Expected: PASS.

- [ ] **Step 4: Write the failing validator test**

Create `apps/backend/internal/policies/builder_test.go`:

```go
package policies

import "testing"

func TestValidatePolicyName(t *testing.T) {
	good := []string{"my-policy", "team_read.v2", "a/b", "abc123"}
	for _, n := range good {
		if err := ValidatePolicyName(n); err != nil {
			t.Errorf("ValidatePolicyName(%q) unexpected error: %v", n, err)
		}
	}
	bad := []string{"", "has space", "bad$char", string(make([]byte, 129))}
	for _, n := range bad {
		if err := ValidatePolicyName(n); err == nil {
			t.Errorf("ValidatePolicyName(%q) expected error", n)
		}
	}
}

func TestValidatePolicyNameReserved(t *testing.T) {
	if err := ValidatePolicyName("readonly"); err == nil {
		t.Error("expected builtin name to be rejected by structural name validator? (no) — reserved handled separately")
	}
}

func TestValidatePolicyDocument(t *testing.T) {
	valid := []byte(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:GetObject"],"Resource":["arn:aws:s3:::b/*"]}]}`)
	if err := ValidatePolicyDocument(valid); err != nil {
		t.Fatalf("valid doc rejected: %v", err)
	}

	if err := ValidatePolicyDocument([]byte(`{not json`)); err == nil {
		t.Error("expected invalid_policy_json")
	}
	noStmt := []byte(`{"Version":"2012-10-17","Statement":[]}`)
	if err := ValidatePolicyDocument(noStmt); err == nil {
		t.Error("expected invalid_policy_structure for empty Statement")
	}
	badEffect := []byte(`{"Version":"2012-10-17","Statement":[{"Effect":"Maybe","Action":["s3:GetObject"]}]}`)
	if err := ValidatePolicyDocument(badEffect); err == nil {
		t.Error("expected invalid_policy_structure for bad Effect")
	}
	noAction := []byte(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow"}]}`)
	if err := ValidatePolicyDocument(noAction); err == nil {
		t.Error("expected invalid_policy_structure for missing Action/NotAction")
	}
}
```

Note: the reserved-name *test* above is illustrative — `ValidatePolicyName` is structural only (charset/length). The reserved-name check is a separate processor step (Task C4) that returns `409 policy_name_reserved`. Drop the `TestValidatePolicyNameReserved` case if it doesn't match the final split; keep only structural-name and document tests.

Run: `cd apps/backend && go test ./internal/policies/ -run 'ValidatePolicy' -v`
Expected: FAIL — undefined validators.

- [ ] **Step 5: Implement the validators**

Create `apps/backend/internal/policies/builder.go`:

```go
package policies

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

// policyNameRE is MinIO's canned-policy name charset: alphanumerics plus
// - _ . / — and 1..128 characters.
var policyNameRE = regexp.MustCompile(`^[A-Za-z0-9_./-]{1,128}$`)

// ValidatePolicyName enforces the MinIO canned-policy name charset/length.
// Reserved-name and origin checks are layered on top in the processor.
func ValidatePolicyName(name string) error {
	if !policyNameRE.MatchString(name) {
		return fmt.Errorf("policy name must be 1-128 chars of [A-Za-z0-9_./-]")
	}
	return nil
}

// policyDoc is the minimal structural shape we enforce. We deliberately do
// NOT model the full IAM grammar — MinIO remains the final authority
// (AddCannedPolicy rejection surfaces as minio_rejected_policy). Each field
// is json.RawMessage / loose so we accept string-or-array forms.
type policyDoc struct {
	Version   string          `json:"Version"`
	Statement []policyStmt    `json:"Statement"`
	Raw       json.RawMessage `json:"-"`
}

type policyStmt struct {
	Effect    string          `json:"Effect"`
	Action    json.RawMessage `json:"Action"`
	NotAction json.RawMessage `json:"NotAction"`
}

// errInvalidJSON / errInvalidStructure are sentinel errors mapped to the
// apierror codes by the processor (so this file stays apierror-free and
// unit-testable in isolation).
var (
	errInvalidJSON      = errors.New("invalid_policy_json")
	errInvalidStructure = errors.New("invalid_policy_structure")
)

// ValidatePolicyDocument performs best-effort structural validation:
//  1. valid JSON object;
//  2. non-empty Statement array, each with Effect in {Allow,Deny} and at
//     least one of Action/NotAction.
//
// Returns errInvalidJSON or errInvalidStructure (sentinels) on failure.
func ValidatePolicyDocument(doc []byte) error {
	if !json.Valid(doc) {
		return errInvalidJSON
	}
	var d policyDoc
	if err := json.Unmarshal(doc, &d); err != nil {
		return errInvalidJSON
	}
	if d.Version == "" || len(d.Statement) == 0 {
		return errInvalidStructure
	}
	for _, s := range d.Statement {
		if s.Effect != "Allow" && s.Effect != "Deny" {
			return errInvalidStructure
		}
		if len(s.Action) == 0 && len(s.NotAction) == 0 {
			return errInvalidStructure
		}
	}
	return nil
}

// IsInvalidJSON / IsInvalidStructure let the processor map the sentinels to
// apierror codes without importing apierror here.
func IsInvalidJSON(err error) bool      { return errors.Is(err, errInvalidJSON) }
func IsInvalidStructure(err error) bool { return errors.Is(err, errInvalidStructure) }
```

(If `errors` is unused after the rewrite, drop the import; here it is used by `errors.New`/`errors.Is`.)

- [ ] **Step 6: Run tests**

Run: `cd apps/backend && go test ./internal/policies/ -v`
Expected: PASS (existing template/materializer tests + new ones).

- [ ] **Step 7: Commit**

```bash
git add apps/backend/internal/policies/model.go apps/backend/internal/policies/classifier.go apps/backend/internal/policies/builder.go apps/backend/internal/policies/classifier_test.go apps/backend/internal/policies/builder_test.go
git commit -m "feat(policies): policy model, origin classifier, name + document validators"
```

### Task C3: Provider mapping + administrator (madmin wrappers + in-use detection)

**Files:**
- Create: `apps/backend/internal/policies/provider.go`
- Create: `apps/backend/internal/policies/administrator.go`
- Test: `apps/backend/internal/policies/provider_test.go` (create)

- [ ] **Step 1: Write the failing provider test**

Create `apps/backend/internal/policies/provider_test.go`:

```go
package policies

import (
	"encoding/json"
	"testing"
)

func TestPolicyFromEntry(t *testing.T) {
	doc := json.RawMessage(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:GetObject"],"Resource":["arn:aws:s3:::photos/*"]}]}`)
	p := policyFromEntry("my-custom", doc)
	if p.Name != "my-custom" || p.Origin != OriginCustom || !p.Editable {
		t.Fatalf("classification wrong: %+v", p)
	}
	if p.StatementSummary == "" {
		t.Error("expected a non-empty statement summary")
	}
}

func TestStatementSummaryAllowGet(t *testing.T) {
	doc := json.RawMessage(`{"Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"arn:aws:s3:::photos/*"}]}`)
	got := statementSummary(doc)
	want := "Allow s3:GetObject on arn:aws:s3:::photos/*"
	if got != want {
		t.Errorf("statementSummary = %q want %q", got, want)
	}
}
```

Run: `cd apps/backend && go test ./internal/policies/ -run 'PolicyFromEntry|StatementSummary' -v`
Expected: FAIL — undefined `policyFromEntry`/`statementSummary`.

- [ ] **Step 2: Implement the provider**

Create `apps/backend/internal/policies/provider.go`:

```go
package policies

import (
	"encoding/json"
	"fmt"
)

// policyFromEntry maps a canned-policy listing entry (name + document) into
// the domain Policy, computing origin/editable and a short statement summary.
func policyFromEntry(name string, doc json.RawMessage) Policy {
	origin := OriginFor(name)
	return Policy{
		Name:             name,
		Origin:           origin,
		Editable:         origin == OriginCustom,
		StatementSummary: statementSummary(doc),
	}
}

// summaryDoc / summaryStmt accept the string-or-array forms IAM uses.
type summaryDoc struct {
	Statement []summaryStmt `json:"Statement"`
}

type summaryStmt struct {
	Effect   string        `json:"Effect"`
	Action   stringOrSlice `json:"Action"`
	Resource stringOrSlice `json:"Resource"`
}

// stringOrSlice unmarshals a JSON value that may be a string or a []string.
type stringOrSlice []string

func (s *stringOrSlice) UnmarshalJSON(b []byte) error {
	var one string
	if err := json.Unmarshal(b, &one); err == nil {
		*s = []string{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return err
	}
	*s = many
	return nil
}

// statementSummary returns "Effect Action on Resource" for the first
// statement, truncated, or "" if the document has no statements. Best-effort
// only — a parse failure yields "".
func statementSummary(doc json.RawMessage) string {
	var d summaryDoc
	if err := json.Unmarshal(doc, &d); err != nil || len(d.Statement) == 0 {
		return ""
	}
	st := d.Statement[0]
	action := "(no action)"
	if len(st.Action) > 0 {
		action = st.Action[0]
	}
	resource := ""
	if len(st.Resource) > 0 {
		resource = " on " + st.Resource[0]
	}
	more := ""
	if len(d.Statement) > 1 {
		more = fmt.Sprintf(" (+%d more)", len(d.Statement)-1)
	}
	return fmt.Sprintf("%s %s%s%s", st.Effect, action, resource, more)
}
```

- [ ] **Step 3: Implement the administrator (madmin wrappers + in-use scan)**

Create `apps/backend/internal/policies/administrator.go`:

```go
package policies

import (
	"context"
	"fmt"
	"strings"
)

// listCanned wraps adminAPI.ListCannedPolicies, mapping the entries to the
// domain Policy slice (names only; documents are not summarized in the list
// to keep the call cheap — only Get returns a document). The summary is
// computed from the listed document, which ListCannedPolicies already returns.
func listCanned(ctx context.Context, adm adminAPI) ([]Policy, error) {
	raw, err := adm.ListCannedPolicies(ctx)
	if err != nil {
		return nil, fmt.Errorf("policies.listCanned: %w", err)
	}
	out := make([]Policy, 0, len(raw))
	for name, doc := range raw {
		out = append(out, policyFromEntry(name, doc))
	}
	return out, nil
}

// attachmentScan returns the users and groups currently referencing policy
// name. madmin has no reverse-lookup, so we enumerate users (UserInfo.
// PolicyName is a comma-joined list) and groups (GroupDesc.Policy). Bounded
// by user/group count, which is small for the target audience.
func attachmentScan(ctx context.Context, adm adminAPI, name string) (users, groups []string, err error) {
	rawUsers, err := adm.ListUsers(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("policies.attachmentScan users: %w", err)
	}
	for ak, info := range rawUsers {
		if containsPolicy(info.PolicyName, name) {
			users = append(users, ak)
		}
	}
	groupNames, err := adm.ListGroups(ctx)
	if err != nil {
		// Group enumeration is best-effort: if unavailable, fall back to
		// users-only and rely on MinIO's own rejection as the backstop.
		return users, nil, nil //nolint:nilerr // best-effort group scan
	}
	for _, g := range groupNames {
		desc, derr := adm.GetGroupDescription(ctx, g)
		if derr != nil || desc == nil {
			continue
		}
		if containsPolicy(desc.Policy, name) {
			groups = append(groups, g)
		}
	}
	return users, groups, nil
}

// containsPolicy reports whether the comma-joined policy list contains name.
func containsPolicy(csv, name string) bool {
	for _, p := range strings.Split(csv, ",") {
		if strings.TrimSpace(p) == name {
			return true
		}
	}
	return false
}
```

Run: `cd apps/backend && go test ./internal/policies/ -run 'PolicyFromEntry|StatementSummary' -v`
Expected: PASS. (The administrator file won't fully build until `adminAPI` exists in Task C4; if the package fails to build only because `adminAPI` is undefined, that's expected — proceed to C4 and build together. To keep this task self-contained, you may instead introduce the `adminAPI` interface stub here in `processor.go` first; sequence C3/C4 so the package builds at each commit.)

- [ ] **Step 4: Commit**

```bash
git add apps/backend/internal/policies/provider.go apps/backend/internal/policies/administrator.go apps/backend/internal/policies/provider_test.go
git commit -m "feat(policies): provider mapping + madmin wrappers + in-use attachment scan"
```

### Task C4: Processor CRUD with audit + guards

**Files:**
- Create: `apps/backend/internal/policies/processor.go`
- Test: `apps/backend/internal/policies/processor_test.go` (create)

- [ ] **Step 1: Write the processor scaffolding (interfaces + constructor)**

Create `apps/backend/internal/policies/processor.go`. Mirror the `users` package wiring verbatim (`users/processor.go:17-117`): an unexported `adminAPI`, an exported `AdminClient` mirror, `ClientGetter`, `NewClientGetter`, `Processor` with `Clients`, `Audit`, `Logger`, `NewProcessor`, `WithAudit`, `WithLogger`, `recordAudit`, `clients`, and `mapClientError`. The interface methods:

```go
package policies

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	madmin "github.com/minio/madmin-go/v3"
	"github.com/rs/zerolog"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
	"github.com/jtumidanski/Harbormaster/internal/audit"
)

// adminAPI is the subset of *madmin.AdminClient the policies processor uses.
// The live client satisfies it by structural typing.
type adminAPI interface {
	ListCannedPolicies(ctx context.Context) (map[string]json.RawMessage, error)
	InfoCannedPolicy(ctx context.Context, name string) ([]byte, error)
	AddCannedPolicy(ctx context.Context, name string, policy []byte) error
	RemoveCannedPolicy(ctx context.Context, name string) error
	ListUsers(ctx context.Context) (map[string]madmin.UserInfo, error)
	ListGroups(ctx context.Context) ([]string, error)
	GetGroupDescription(ctx context.Context, group string) (*madmin.GroupDesc, error)
}

// AdminClient is the exported mirror of adminAPI (see users.AdminClient).
type AdminClient interface {
	ListCannedPolicies(ctx context.Context) (map[string]json.RawMessage, error)
	InfoCannedPolicy(ctx context.Context, name string) ([]byte, error)
	AddCannedPolicy(ctx context.Context, name string, policy []byte) error
	RemoveCannedPolicy(ctx context.Context, name string) error
	ListUsers(ctx context.Context) (map[string]madmin.UserInfo, error)
	ListGroups(ctx context.Context) ([]string, error)
	GetGroupDescription(ctx context.Context, group string) (*madmin.GroupDesc, error)
}

type ClientGetter func(ctx context.Context) (adminAPI, error)

func NewClientGetter(resolve func(ctx context.Context) (AdminClient, error)) ClientGetter {
	return func(ctx context.Context) (adminAPI, error) {
		c, err := resolve(ctx)
		if err != nil {
			return nil, err
		}
		return c, nil
	}
}

// Processor is the policies-domain orchestrator (no local persistence).
type Processor struct {
	Clients ClientGetter
	Logger  zerolog.Logger
	Audit   *audit.Processor
}

func NewProcessor(clients ClientGetter) *Processor {
	return &Processor{Clients: clients, Logger: zerolog.Nop()}
}

func (p *Processor) WithLogger(l zerolog.Logger) *Processor { p.Logger = l; return p }
func (p *Processor) WithAudit(a *audit.Processor) *Processor { p.Audit = a; return p }

func (p *Processor) recordAudit(ctx context.Context, e audit.Event) {
	if p.Audit == nil {
		return
	}
	_ = p.Audit.Record(ctx, e)
}

func (p *Processor) clients(ctx context.Context) (adminAPI, error) {
	if p.Clients == nil {
		return nil, apierror.Internal("policies: client getter not configured")
	}
	adm, err := p.Clients(ctx)
	if err != nil {
		return nil, apierror.New(http.StatusServiceUnavailable,
			"minio_unavailable", "MinIO client is not available: "+err.Error())
	}
	return adm, nil
}

func mapClientError(err error, fallback string) *apierror.Error {
	if err == nil {
		return nil
	}
	var ae *apierror.Error
	if errors.As(err, &ae) {
		return ae
	}
	return apierror.New(http.StatusBadGateway, "minio_error", fallback+": "+err.Error())
}
```

- [ ] **Step 2: Write the failing CRUD tests**

Create `apps/backend/internal/policies/processor_test.go` with an in-memory `stubAdmin` implementing `adminAPI` (a `map[string]json.RawMessage` of policies + a programmable `users map[string]madmin.UserInfo` and `groups`). Cover:

```go
func TestCreateRejectsInvalidJSON(t *testing.T)        // 422 invalid_policy_json
func TestCreateRejectsBadStructure(t *testing.T)       // 422 invalid_policy_structure
func TestCreateRejectsBadName(t *testing.T)            // 422 invalid_policy_name
func TestCreateRejectsReservedName(t *testing.T)       // 409 policy_name_reserved (builtin or harbormaster- prefix)
func TestCreateAddsCannedPolicy(t *testing.T)          // happy path calls AddCannedPolicy, audit success
func TestUpdateRejectsNonCustom(t *testing.T)          // 403 policy_read_only for builtin/template
func TestDeleteRejectsInUse(t *testing.T)              // 409 policy_in_use, details.attached_to.users
func TestDeleteRemovesUnusedCustom(t *testing.T)       // happy path calls RemoveCannedPolicy
func TestGetReturnsDocument(t *testing.T)              // Get → PolicyDetail.Document
```

Use an `assertAPIError(t, err, status, code)` helper (mirror the one in `users`/`objects` tests; if none is exported, write a small local one that `errors.As`-es to `*apierror.Error` and checks `HTTPStatus`/`Code`). For `TestDeleteRejectsInUse`, program the stub's `users` so one `UserInfo.PolicyName` contains the target and assert `err.(*apierror.Error).Details["attached_to"]`.

Run: `cd apps/backend && go test ./internal/policies/ -run 'Create|Update|Delete|Get' -v`
Expected: FAIL — undefined processor methods.

- [ ] **Step 3: Implement the CRUD methods**

Append to `processor.go`:

```go
// List returns every canned policy with origin/editable classification,
// sorted by name.
func (p *Processor) List(ctx context.Context) ([]Policy, error) {
	adm, err := p.clients(ctx)
	if err != nil {
		return nil, err
	}
	out, err := listCanned(ctx, adm)
	if err != nil {
		return nil, mapClientError(err, "failed to list policies")
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Get returns the policy plus its full IAM document.
func (p *Processor) Get(ctx context.Context, name string) (PolicyDetail, error) {
	adm, err := p.clients(ctx)
	if err != nil {
		return PolicyDetail{}, err
	}
	doc, err := adm.InfoCannedPolicy(ctx, name)
	if err != nil {
		return PolicyDetail{}, mapClientError(err, "failed to read policy")
	}
	return PolicyDetail{Policy: policyFromEntry(name, doc), Document: doc}, nil
}

// Create validates and adds a new custom canned policy. Reserved names
// (built-ins + the harbormaster- template prefix) are rejected before any
// MinIO call.
func (p *Processor) Create(ctx context.Context, name string, doc []byte, actor, sourceIP string) (Policy, error) {
	payload := map[string]any{"policy": name}
	failAudit := p.policyFailAudit(ctx, audit.ActionPolicyCreate, name, payload)
	if err := p.validateForWrite(name, doc, true); err != nil {
		return Policy{}, failAudit(err)
	}
	adm, err := p.clients(ctx)
	if err != nil {
		return Policy{}, failAudit(err)
	}
	if err := adm.AddCannedPolicy(ctx, name, doc); err != nil {
		return Policy{}, failAudit(apierror.New(http.StatusUnprocessableEntity,
			"minio_rejected_policy", "MinIO rejected the policy: "+err.Error()))
	}
	p.recordAudit(ctx, audit.Event{
		Actor: actor, SourceIP: sourceIP, Action: audit.ActionPolicyCreate,
		TargetType: "policy", TargetID: name, Outcome: audit.OutcomeSuccess, PayloadSummary: payload,
	})
	return policyFromEntry(name, doc), nil
}

// Update overwrites an existing custom policy's document. Non-custom origins
// are rejected 403 policy_read_only.
func (p *Processor) Update(ctx context.Context, name string, doc []byte, actor, sourceIP string) (Policy, error) {
	payload := map[string]any{"policy": name}
	failAudit := p.policyFailAudit(ctx, audit.ActionPolicyUpdate, name, payload)
	if !EditableFor(name) {
		return Policy{}, failAudit(apierror.New(http.StatusForbidden, "policy_read_only",
			"only custom policies can be edited"))
	}
	if err := p.validateForWrite(name, doc, false); err != nil {
		return Policy{}, failAudit(err)
	}
	adm, err := p.clients(ctx)
	if err != nil {
		return Policy{}, failAudit(err)
	}
	// AddCannedPolicy is an upsert on MinIO.
	if err := adm.AddCannedPolicy(ctx, name, doc); err != nil {
		return Policy{}, failAudit(apierror.New(http.StatusUnprocessableEntity,
			"minio_rejected_policy", "MinIO rejected the policy: "+err.Error()))
	}
	p.recordAudit(ctx, audit.Event{
		Actor: actor, SourceIP: sourceIP, Action: audit.ActionPolicyUpdate,
		TargetType: "policy", TargetID: name, Outcome: audit.OutcomeSuccess, PayloadSummary: payload,
	})
	return policyFromEntry(name, doc), nil
}

// Delete removes a custom policy after verifying it is not attached to any
// user or group.
func (p *Processor) Delete(ctx context.Context, name, actor, sourceIP string) error {
	payload := map[string]any{"policy": name}
	failAudit := p.policyFailAudit(ctx, audit.ActionPolicyDelete, name, payload)
	if !EditableFor(name) {
		return failAudit(apierror.New(http.StatusForbidden, "policy_read_only",
			"only custom policies can be deleted"))
	}
	adm, err := p.clients(ctx)
	if err != nil {
		return failAudit(err)
	}
	users, groups, scanErr := attachmentScan(ctx, adm, name)
	if scanErr != nil {
		return failAudit(mapClientError(scanErr, "failed to check policy attachments"))
	}
	if len(users) > 0 || len(groups) > 0 {
		return failAudit(apierror.New(http.StatusConflict, "policy_in_use",
			"policy is attached to one or more users or groups").
			WithDetails(map[string]any{"attached_to": map[string]any{"users": users, "groups": groups}}))
	}
	if err := adm.RemoveCannedPolicy(ctx, name); err != nil {
		return failAudit(mapClientError(err, "failed to delete policy"))
	}
	p.recordAudit(ctx, audit.Event{
		Actor: actor, SourceIP: sourceIP, Action: audit.ActionPolicyDelete,
		TargetType: "policy", TargetID: name, Outcome: audit.OutcomeSuccess, PayloadSummary: payload,
	})
	return nil
}

// validateForWrite runs name (create-only), reserved-name (create-only), and
// document validation, mapping the builder sentinels to apierror codes.
func (p *Processor) validateForWrite(name string, doc []byte, checkName bool) error {
	if checkName {
		if err := ValidatePolicyName(name); err != nil {
			return apierror.New(http.StatusUnprocessableEntity, "invalid_policy_name", err.Error()).
				WithPointer("/data/attributes/name")
		}
		if IsBuiltin(name) || isTemplateName(name) {
			return apierror.New(http.StatusConflict, "policy_name_reserved",
				"name is reserved for a built-in or Harbormaster template policy").
				WithPointer("/data/attributes/name")
		}
	}
	if err := ValidatePolicyDocument(doc); err != nil {
		switch {
		case IsInvalidJSON(err):
			return apierror.New(http.StatusUnprocessableEntity, "invalid_policy_json",
				"document is not valid JSON").WithPointer("/data/attributes/document")
		case IsInvalidStructure(err):
			return apierror.New(http.StatusUnprocessableEntity, "invalid_policy_structure",
				"document is missing Version or a valid Statement array").WithPointer("/data/attributes/document")
		default:
			return apierror.New(http.StatusUnprocessableEntity, "invalid_policy_structure", err.Error())
		}
	}
	return nil
}

// policyFailAudit returns the standard failure-audit closure for a policy
// action.
func (p *Processor) policyFailAudit(ctx context.Context, action, name string, payload map[string]any) func(error) error {
	return func(err error) error {
		p.recordAudit(ctx, audit.Event{
			Action: action, TargetType: "policy", TargetID: name,
			Outcome: audit.OutcomeFailure, ErrorMessage: err.Error(), PayloadSummary: payload,
		})
		return err
	}
}
```

Add `"sort"` to the `processor.go` imports.

- [ ] **Step 4: Run tests**

Run: `cd apps/backend && go test ./internal/policies/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/backend/internal/policies/processor.go apps/backend/internal/policies/processor_test.go
git commit -m "feat(policies): processor List/Get/Create/Update/Delete with guards + audit"
```

### Task C5: REST resource + routes

**Files:**
- Create: `apps/backend/internal/policies/rest.go`
- Create: `apps/backend/internal/policies/resource.go`
- Test: `apps/backend/internal/policies/golden_test.go` (create)
- Test: `apps/backend/internal/policies/resource_test.go` (create)

- [ ] **Step 1: Write the wire layer**

Create `apps/backend/internal/policies/rest.go` (mirror `users/rest.go` shapes):

```go
package policies

import "encoding/json"

// policyResource is the JSON:API wrapper for a Policy (collection + single
// without document).
type policyResource struct {
	Policy
}

func (policyResource) ResourceType() string { return "policies" }
func (r policyResource) ResourceID() string  { return r.Name }

func (r policyResource) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Name             string `json:"name"`
		Origin           string `json:"origin"`
		Editable         bool   `json:"editable"`
		StatementSummary string `json:"statement_summary"`
	}{r.Name, r.Origin, r.Editable, r.StatementSummary})
}

// policyDetailResource adds the raw document (Get only).
type policyDetailResource struct {
	PolicyDetail
}

func (policyDetailResource) ResourceType() string { return "policies" }
func (r policyDetailResource) ResourceID() string  { return r.Name }

func (r policyDetailResource) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Name             string          `json:"name"`
		Origin           string          `json:"origin"`
		Editable         bool            `json:"editable"`
		StatementSummary string          `json:"statement_summary"`
		Document         json.RawMessage `json:"document"`
	}{r.Name, r.Origin, r.Editable, r.StatementSummary, r.Document})
}

// CreateRequest / UpdateRequest are the JSON:API attribute blocks. Document
// is a raw JSON object preserved verbatim for MinIO.
type CreateRequest struct {
	Name     string          `json:"name"`
	Document json.RawMessage `json:"document"`
}

type UpdateRequest struct {
	Document json.RawMessage `json:"document"`
}
```

- [ ] **Step 2: Write the resource (handlers + routes)**

Create `apps/backend/internal/policies/resource.go` mirroring `users/resource.go` (handler struct with `enc`/`dec`, `actorFromRequest` copied verbatim from `users/resource.go:20-29`, `writeSingle`/collection helpers). Routes:

```go
func Routes(p *Processor) func(chi.Router) {
	h := &handler{p: p, enc: jsonapi.NewEncoder(), dec: jsonapi.NewDecoder()}
	return func(r chi.Router) {
		r.Get("/policies", h.list)
		r.Post("/policies", h.create)
		r.Get("/policies/{name}", h.get)
		r.Put("/policies/{name}", h.update)
		r.Delete("/policies/{name}", h.delete)
	}
}
```

Handlers: `list` renders a `policies` collection with `Meta.Page{TotalRecords: len}`; `get` renders `policyDetailResource` single; `create` decodes `CreateRequest` via `h.dec.Single`, calls `p.Create`, renders 201 single; `update` decodes `UpdateRequest`, calls `p.Update`, renders 200 single; `delete` calls `p.Delete`, 204. All resource routes render errors with `apierror.StyleJSONAPI`. Mirror the exact encoder calls in `users/resource.go` (`h.enc.Single`/`h.enc.Collection`, `Content-Type: application/vnd.api+json`).

- [ ] **Step 3: Golden + HTTP tests**

Create `golden_test.go` marshaling `policyResource` (custom + builtin) and `policyDetailResource` (asserting the `document` passthrough). Create `resource_test.go` with an HTTP-level test (stub processor via a stub `adminAPI`) for: `GET /policies` collection shape (`data[].type=="policies"`), `POST /policies` invalid JSON → 422 `invalid_policy_json` rendered JSON:API. Mirror the HTTP test setup in `users/resource_test.go` if present (chi router + httptest).

Run: `cd apps/backend && go test ./internal/policies/ -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add apps/backend/internal/policies/rest.go apps/backend/internal/policies/resource.go apps/backend/internal/policies/golden_test.go apps/backend/internal/policies/resource_test.go
git commit -m "feat(policies): JSON:API resource + CRUD routes"
```

### Task C6: Wire the policies surface in cmd/harbormaster

**Files:**
- Modify: `apps/backend/cmd/harbormaster/serve.go`
- Modify: `apps/backend/cmd/harbormaster/audit_adapter.go`

- [ ] **Step 1: Add the policies ClientGetter**

In `audit_adapter.go`, add (mirror `newUsersClientGetter`, `audit_adapter.go:188-196` — the live `*madmin.AdminClient` satisfies `policies.AdminClient` structurally):

```go
// newPoliciesClientGetter returns a policies.ClientGetter bound to the live
// MinIO pool. The live *madmin.AdminClient satisfies policies.AdminClient by
// structural typing.
func newPoliciesClientGetter(pool *hmminio.Pool) policies.ClientGetter {
	return policies.NewClientGetter(func(ctx context.Context) (policies.AdminClient, error) {
		madm, _, err := pool.Get(ctx)
		if err != nil {
			return nil, err
		}
		return madm, nil
	})
}
```

Add the `policies` import if not already present.

- [ ] **Step 2: Construct the processor and mount the routes**

In `serve.go`, near the users processor construction, add:

```go
policyProc := policies.NewProcessor(newPoliciesClientGetter(pool)).
	WithAudit(auditProc).
	WithLogger(logger)
```

(Use the same logger variable the other processors use; check the surrounding lines.) In `protectedRoutes` (the `g.Group` block, `serve.go:206-234`), add after `users.Routes(...)`:

```go
		policies.Routes(policyProc)(g)
```

- [ ] **Step 3: Build + vet**

Run: `cd apps/backend && CGO_ENABLED=0 go build ./... && go vet ./...`
Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add apps/backend/cmd/harbormaster/serve.go apps/backend/cmd/harbormaster/audit_adapter.go
git commit -m "feat(policies): wire policies CRUD routes in serve"
```

### Task C7: Frontend — Policies management page + editor dialog

**Files:**
- Create: `apps/frontend/src/components/ui/textarea.tsx` (shadcn primitive — not yet present)
- Modify: `apps/frontend/src/lib/api/keys.ts` (`policiesKeys`)
- Create: `apps/frontend/src/features/policies/policiesApi.ts`
- Modify: `apps/frontend/src/features/policies/types.ts`
- Create: `apps/frontend/src/features/policies/PoliciesPage.tsx`
- Create: `apps/frontend/src/features/policies/PolicyEditorDialog.tsx`
- Modify: `apps/frontend/src/routes.tsx` (point `/policies` at `PoliciesPage`)
- Test: `apps/frontend/src/features/policies/PolicyEditorDialog.test.tsx` (create)

- [ ] **Step 1: Add the shadcn `textarea` primitive**

Create `apps/frontend/src/components/ui/textarea.tsx` (standard shadcn Textarea):

```tsx
import * as React from "react";
import { cn } from "@/lib/utils";

const Textarea = React.forwardRef<HTMLTextAreaElement, React.ComponentProps<"textarea">>(
  ({ className, ...props }, ref) => (
    <textarea
      ref={ref}
      className={cn(
        "flex min-h-[160px] w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50",
        className,
      )}
      {...props}
    />
  ),
);
Textarea.displayName = "Textarea";

export { Textarea };
```

- [ ] **Step 2: Query keys + API + types**

In `keys.ts` add:

```ts
export const policiesKeys = {
  all: () => ["policies"] as const,
  list: () => ["policies", "list"] as const,
  detail: (name: string) => ["policies", "detail", name] as const,
};
```

In `features/policies/types.ts` add (keep existing `PolicyTemplate*` types):

```ts
export type Policy = {
  name: string;
  origin: "minio-builtin" | "harbormaster-template" | "custom";
  editable: boolean;
  statement_summary: string;
};

export type PolicyDetail = Policy & { document: unknown };

export type PolicyCollectionResponse = {
  data: Array<{ type: "policies"; id: string; attributes: Policy }>;
};

export type PolicySingleResponse = {
  data: { type: "policies"; id: string; attributes: PolicyDetail };
};
```

Create `features/policies/policiesApi.ts`:

```ts
import { api } from "@/lib/api/client";
import type { Policy, PolicyCollectionResponse, PolicyDetail, PolicySingleResponse } from "./types";

export async function listPolicies(): Promise<Policy[]> {
  const res = await api.get<PolicyCollectionResponse>("/api/v1/policies");
  return res.data.map((d) => d.attributes);
}

export async function getPolicy(name: string): Promise<PolicyDetail> {
  const res = await api.get<PolicySingleResponse>(`/api/v1/policies/${encodeURIComponent(name)}`);
  return res.data.attributes;
}

export async function createPolicy(name: string, document: unknown): Promise<void> {
  await api.post<PolicySingleResponse>("/api/v1/policies", {
    data: { type: "policies", attributes: { name, document } },
  });
}

export async function updatePolicy(name: string, document: unknown): Promise<void> {
  await api.put<PolicySingleResponse>(`/api/v1/policies/${encodeURIComponent(name)}`, {
    data: { type: "policies", attributes: { document } },
  });
}

export async function deletePolicy(name: string): Promise<void> {
  await api.delete<void>(`/api/v1/policies/${encodeURIComponent(name)}`);
}
```

- [ ] **Step 3: Write the failing editor-dialog test**

Create `PolicyEditorDialog.test.tsx`: render the dialog in create mode, type an invalid JSON document, submit, and assert a client-side `invalid JSON` message appears and `createPolicy` is NOT called; then type valid JSON + a name and assert `createPolicy` is called with the parsed object. Mock `./policiesApi`. Mirror the QueryClient + mock setup in `features/lifecycle/CreateRuleDialog.test.tsx`.

Run: `cd apps/frontend && npm test -- PolicyEditorDialog`
Expected: FAIL — component doesn't exist.

- [ ] **Step 4: Implement `PolicyEditorDialog.tsx`**

Mirror `CreateRuleDialog.tsx`'s mutation/toast/onError structure. Props: `{ open, onOpenChange, mode: "create" | "edit", policyName?: string }`. A `name` `Input` (disabled in edit mode), a `Textarea` (monospace via `className="font-mono"`) for the document. On submit: `JSON.parse` the textarea client-side first — on failure set a local error "Document is not valid JSON." and abort (mirrors the server's `invalid_policy_json`). On success call `createPolicy`/`updatePolicy`; invalidate `policiesKeys.list()` and `policiesKeys.detail(name)`; toast; close. In `onError`, map `AppError.pointer === "/data/attributes/document"` → document field error, `/data/attributes/name` → name field error, else toast `err.message`. In edit mode, prefill the document via `useQuery(policiesKeys.detail(name), () => getPolicy(name))` and `JSON.stringify(doc, null, 2)`.

- [ ] **Step 5: Implement `PoliciesPage.tsx`**

A unified table (shadcn `Table`) of all canned policies from `useQuery(policiesKeys.list(), listPolicies)`. Columns: name, `origin` badge (`Badge variant="outline"`), statement summary, actions. A "New policy" button opens `PolicyEditorDialog` in create mode. Edit/Delete buttons render only for `editable` rows; Edit opens the dialog in edit mode; Delete opens a confirm `Dialog` and calls `deletePolicy` — in `onError`, when `AppError.code === "policy_in_use"`, render `err.details.attached_to.users`/`groups` in the confirm dialog instead of a bare toast. Keep the existing template list reachable as a filter/section (templates appear as `origin: harbormaster-template`, read-only) — the repurposed page subsumes `PolicyTemplatesPage`.

- [ ] **Step 6: Point the route at the new page**

In `routes.tsx`, change the `/policies` element from `<PolicyTemplatesPage />` to `<PoliciesPage />` and update the import (line 13). Leave `PolicyTemplatesPage.tsx` in place only if still referenced; otherwise remove it and its now-dead imports (verify with `npm run build`). The nav entry already exists (`AppShell.tsx:47`, label "Policies").

- [ ] **Step 7: Run tests + lint/format**

Run: `cd apps/frontend && npm test -- policies && npm run lint && npm run format`
Expected: PASS / clean.

- [ ] **Step 8: Commit**

```bash
git add apps/frontend/src/features/policies/ apps/frontend/src/components/ui/textarea.tsx apps/frontend/src/lib/api/keys.ts apps/frontend/src/routes.tsx
git commit -m "feat(policies-ui): full policies management page + JSON editor dialog"
```

---

## Track D — User Custom-Policy Attachment

Depends on Track C's origin classifier. Extends `internal/users` so the existing `PUT /users/{access_key}/policies` endpoint also attaches/detaches **named custom policies** alongside templates, using per-policy `AttachPolicy`/`DetachPolicy` (never set-replace) so built-in/foreign grants (e.g. `consoleAdmin`) always survive. See design §4.4.

### Task D1: New audit verbs for policy attach/detach

**Files:**
- Modify: `apps/backend/internal/audit/model.go`

- [ ] **Step 1: Add the constants and register them**

Add to the action `const (...)` block (after `ActionUserPoliciesUpdate`):

```go
	ActionUserPolicyAttach      = "user.policy.attach"
	ActionUserPolicyDetach      = "user.policy.detach"
```

Add both to `AllActions()`.

- [ ] **Step 2: Run the audit test**

Run: `cd apps/backend && go test ./internal/audit/ -v`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add apps/backend/internal/audit/model.go
git commit -m "feat(audit): add user policy attach/detach verbs"
```

### Task D2: Extend the users adminAPI for canned-policy introspection

**Files:**
- Modify: `apps/backend/internal/users/processor.go`

- [ ] **Step 1: Add the three methods to both interfaces**

In `processor.go`, add to **both** `adminAPI` (line 22-31) and `AdminClient` (line 38-47), keeping them identical:

```go
	ListCannedPolicies(ctx context.Context) (map[string]json.RawMessage, error)
	InfoCannedPolicy(ctx context.Context, name string) ([]byte, error)
```

Add `"encoding/json"` to the imports. `AddCannedPolicy` is already present on both interfaces (used by the materializer wiring). The live `*madmin.AdminClient` already satisfies these by structural typing — no adapter change needed in `audit_adapter.go` (the users getter returns the live client directly).

- [ ] **Step 2: Build**

Run: `cd apps/backend && CGO_ENABLED=0 go build ./internal/users/...`
Expected: FAIL only in the in-package test stub (it doesn't implement the two new methods yet) — fixed in Task D3. If non-test code builds, proceed.

- [ ] **Step 3: Commit**

```bash
git add apps/backend/internal/users/processor.go
git commit -m "feat(users): adminAPI gains canned-policy list/info for custom attachment"
```

### Task D3: Owned-set diff including custom policies (TDD)

**Files:**
- Modify: `apps/backend/internal/users/model.go` (add `AttachedPolicies`)
- Modify: `apps/backend/internal/users/administrator.go` (classify owned custom)
- Modify: `apps/backend/internal/users/processor.go` (extend `UpdatePolicies`)
- Test: `apps/backend/internal/users/processor_test.go` (extend; update the stub)

- [ ] **Step 1: Add the model field + owned-custom classifier**

In `model.go`, add to `User` (after `OtherPolicies`):

```go
	// AttachedPolicies are named custom (origin == custom) canned policies
	// attached to the user — the Harbormaster-managed custom set, distinct
	// from AttachedTemplates and from the read-only OtherPolicies.
	AttachedPolicies []string
```

In `administrator.go`, add a helper that splits an attached policy list into the three buckets using the policies classifier and the deployment's custom-policy set:

```go
// classifyAttachments splits a user's attached policy names into
// (templates, customOwned, other). customOwned = names with origin==custom
// that exist on the deployment (deploymentCustom). Anything else
// (built-ins, foreign) lands in other and is never touched by Harbormaster.
func classifyAttachments(names []string, deploymentCustom map[string]struct{}) (templates []TemplateRef, customOwned, other []string) {
	for _, n := range names {
		if ref, ok := parsePolicyName(n); ok {
			templates = append(templates, ref)
			continue
		}
		if policies.OriginFor(n) == policies.OriginCustom {
			if _, exists := deploymentCustom[n]; exists {
				customOwned = append(customOwned, n)
				continue
			}
		}
		other = append(other, n)
	}
	return templates, customOwned, other
}
```

(`policies` is already imported by the users package.)

- [ ] **Step 2: Extend the stub + write failing tests**

In `processor_test.go`, extend the existing admin stub to implement `ListCannedPolicies`/`InfoCannedPolicy` (program a `canned map[string]json.RawMessage`) and to capture `AttachPolicy`/`DetachPolicy` calls (record the policy names). Then add:

```go
func TestUpdatePolicies_AttachesCustom(t *testing.T)         // policies:["proj-a"] → AttachPolicy("proj-a")
func TestUpdatePolicies_DetachesRemovedCustom(t *testing.T)  // had proj-a (custom, exists), request none → DetachPolicy("proj-a")
func TestUpdatePolicies_NeverDetachesBuiltin(t *testing.T)   // user has consoleAdmin; update with empty sets → consoleAdmin NOT detached
func TestUpdatePolicies_NeverDetachesForeign(t *testing.T)   // user has "some-foreign" not in deployment custom set → never detached
func TestUpdatePolicies_RejectsUnknownPolicy(t *testing.T)   // policies:["nope"] not on deployment → 422 unknown_policy
```

For `NeverDetachesBuiltin`: program the stub `GetUserInfo` to return `PolicyName: "consoleAdmin"`, call `UpdatePolicies` with empty templates+policies, assert the stub recorded zero `DetachPolicy` calls.

Run: `cd apps/backend && go test ./internal/users/ -run 'UpdatePolicies_' -v`
Expected: FAIL — `UpdatePolicies` doesn't handle custom policies yet.

- [ ] **Step 3: Extend `UpdatePolicies`**

Change the signature to accept custom policy names and thread them through. The HTTP layer passes both:

```go
func (p *Processor) UpdatePolicies(ctx context.Context, accessKey string, requested []TemplateRef, customPolicies []string, actor, sourceIP string) error {
```

Inside, after the existing up-front template validation and `p.clients(ctx)`:

```go
	// Resolve the deployment's custom-policy set once (for ownership +
	// unknown_policy validation).
	rawCanned, err := adm.ListCannedPolicies(ctx)
	if err != nil {
		return failAudit(mapClientError(err, "failed to list policies"))
	}
	deploymentCustom := map[string]struct{}{}
	for name := range rawCanned {
		if policies.OriginFor(name) == policies.OriginCustom {
			deploymentCustom[name] = struct{}{}
		}
	}
	// Up-front: every requested custom policy must exist on the deployment.
	for _, name := range customPolicies {
		if _, ok := deploymentCustom[name]; !ok {
			return failAudit(apierror.New(http.StatusUnprocessableEntity, "unknown_policy",
				"unknown custom policy: "+name).WithPointer("/data/attributes/policies"))
		}
	}
```

Replace the current-set classification so it splits templates AND owned-custom:

```go
	info, err := adm.GetUserInfo(ctx, accessKey)
	if err != nil {
		return failAudit(mapClientError(err, "failed to read user info"))
	}
	currentTemplates, currentCustom, _ := classifyAttachments(splitPolicyList(info.PolicyName), deploymentCustom)
```

Keep the existing template diff exactly as-is (build `currentSet`/`requestedSet` from `currentTemplates`/`requested`, detach removed + attach added). Then add a parallel custom-policy diff that records the new attach/detach verbs per policy:

```go
	// Custom-policy diff: per-policy AttachPolicy/DetachPolicy, never
	// set-replace. Built-ins/foreign are absent from both sets and thus
	// never touched.
	currentCustomSet := map[string]struct{}{}
	for _, n := range currentCustom {
		currentCustomSet[n] = struct{}{}
	}
	requestedCustomSet := map[string]struct{}{}
	for _, n := range customPolicies {
		requestedCustomSet[n] = struct{}{}
	}
	for n := range currentCustomSet {
		if _, keep := requestedCustomSet[n]; keep {
			continue
		}
		if _, err := adm.DetachPolicy(ctx, madmin.PolicyAssociationReq{Policies: []string{n}, User: accessKey}); err != nil {
			return failAudit(mapClientError(err, "failed to detach policy"))
		}
		p.recordAudit(ctx, audit.Event{
			Actor: actor, SourceIP: sourceIP, Action: audit.ActionUserPolicyDetach,
			TargetType: "user", TargetID: accessKey, Outcome: audit.OutcomeSuccess,
			PayloadSummary: map[string]any{"access_key": accessKey, "policy": n},
		})
	}
	for n := range requestedCustomSet {
		if _, have := currentCustomSet[n]; have {
			continue
		}
		if _, err := adm.AttachPolicy(ctx, madmin.PolicyAssociationReq{Policies: []string{n}, User: accessKey}); err != nil {
			return failAudit(mapClientError(err, "failed to attach policy"))
		}
		p.recordAudit(ctx, audit.Event{
			Actor: actor, SourceIP: sourceIP, Action: audit.ActionUserPolicyAttach,
			TargetType: "user", TargetID: accessKey, Outcome: audit.OutcomeSuccess,
			PayloadSummary: map[string]any{"access_key": accessKey, "policy": n},
		})
	}
```

Add `"encoding/json"` to imports if not already present (the interface methods reference it). Update the success audit payload to include `"policies": customPolicies`.

Also update `List` and `Get` to populate `AttachedPolicies`: they must resolve `deploymentCustom` (one `ListCannedPolicies` per call) and use `classifyAttachments` instead of `classifyPolicies`. Keep `AttachedTemplates`/`OtherPolicies` semantics unchanged.

- [ ] **Step 4: Update existing callers**

The `Create` path and the HTTP `updatePolicies` handler call `UpdatePolicies`. Update the create path's post-create flow only if it calls `UpdatePolicies` (it does not — Create attaches templates directly; leave it). Update the HTTP handler in Task D4. For now, fix the compile by passing `nil` for `customPolicies` anywhere internal code calls it.

Run: `cd apps/backend && go test ./internal/users/ -v`
Expected: PASS (existing template tests + new custom tests).

- [ ] **Step 5: Commit**

```bash
git add apps/backend/internal/users/model.go apps/backend/internal/users/administrator.go apps/backend/internal/users/processor.go apps/backend/internal/users/processor_test.go
git commit -m "feat(users): attach/detach named custom policies via owned-set diff"
```

### Task D4: REST — accept `policies`, expose `attached_policies`

**Files:**
- Modify: `apps/backend/internal/users/rest.go`
- Modify: `apps/backend/internal/users/resource.go`
- Test: `apps/backend/internal/users/golden_test.go` (extend) / `resource_test.go`

- [ ] **Step 1: Extend the request + resource wire shapes**

In `rest.go`, extend `UpdatePoliciesRequest`:

```go
type UpdatePoliciesRequest struct {
	Templates []TemplateWire `json:"templates"`
	Policies  []string       `json:"policies"`
}
```

Extend `UserResource.MarshalJSON` (or the user attribute struct it marshals) to add `attached_policies`. Find the existing `MarshalJSON` on `UserResource` and add the field mirroring `attached_templates`:

```go
		AttachedPolicies []string `json:"attached_policies"`
```

populated from `r.AttachedPolicies`.

- [ ] **Step 2: Pass `policies` through the handler**

In `resource.go` `updatePolicies` handler, pass `body.Policies`:

```go
	if err := h.p.UpdatePolicies(r.Context(), ak, body.ToTemplateRefs(), body.Policies, actor, ip); err != nil {
```

- [ ] **Step 3: Tests**

Extend the golden test to assert `attached_policies` appears in the user resource JSON. Add an HTTP test: `PUT /users/{ak}/policies` with `{"policies":["proj-a"]}` (stub deployment has `proj-a`) calls `AttachPolicy("proj-a")`; with an unknown policy → 422 `unknown_policy`. Mirror the existing `updatePolicies` HTTP test.

Run: `cd apps/backend && go test ./internal/users/ -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add apps/backend/internal/users/rest.go apps/backend/internal/users/resource.go apps/backend/internal/users/golden_test.go apps/backend/internal/users/resource_test.go
git commit -m "feat(users): accept policies[] and expose attached_policies on the wire"
```

### Task D5: Frontend — custom-policy attachment UI

**Files:**
- Modify: `apps/frontend/src/features/users/types.ts` (add `attached_policies`)
- Modify: `apps/frontend/src/features/users/api.ts` (extend `updateUserPolicies`)
- Create: `apps/frontend/src/features/users/EditCustomPoliciesDialog.tsx`
- Modify: `apps/frontend/src/features/users/UserDetailPage.tsx` (Custom policies section)
- Test: `apps/frontend/src/features/users/EditCustomPoliciesDialog.test.tsx` (create)

- [ ] **Step 1: Types + API**

In `users/types.ts`, add `attached_policies: string[]` to the `User` type. In `users/api.ts`, extend `updateUserPolicies` to send both templates and policies:

```ts
export async function updateUserPolicies(
  accessKey: string,
  templates: TemplateRef[],
  policies: string[],
): Promise<void> {
  await api.put<void>(`/api/v1/users/${encodeURIComponent(accessKey)}/policies`, {
    templates,
    policies,
  });
}
```

Update the existing `EditPoliciesDialog.tsx` caller of `updateUserPolicies` to pass the user's current `attached_policies` as the third arg (so editing templates doesn't drop custom policies, and vice-versa).

- [ ] **Step 2: Write the failing dialog test**

Create `EditCustomPoliciesDialog.test.tsx`: mock `listPolicies` (from `features/policies/policiesApi`) returning a few `origin: "custom"` and one `origin: "minio-builtin"` policy; assert only custom policies render as multi-select options; pre-check the user's current `attached_policies`; on submit assert `updateUserPolicies(accessKey, currentTemplates, selectedCustomNames)` is called. Mirror `EditPoliciesDialog.test.tsx` setup.

Run: `cd apps/frontend && npm test -- EditCustomPoliciesDialog`
Expected: FAIL — component doesn't exist.

- [ ] **Step 3: Implement `EditCustomPoliciesDialog.tsx`**

Mirror `EditPoliciesDialog.tsx`'s structure (checkbox list + mutation + toast). Fetch all policies via `useQuery(policiesKeys.list(), listPolicies)`, filter to `origin === "custom"`, render a checkbox per policy pre-checked from the user's `attached_policies`. On submit call `updateUserPolicies(accessKey, currentTemplates, selectedNames)` (templates unchanged — pass the user's current `attached_templates` mapped to `TemplateRef[]`). Invalidate `usersKeys.detail`/`usersKeys.list`; toast; `onError` maps `AppError` to a toast.

- [ ] **Step 4: Wire the section into `UserDetailPage.tsx`**

Add a "Custom policies" `Card` beside the existing "Attached templates" card, rendering `user.attached_policies` as chips (reuse the chip style) with an "Edit custom policies" button opening `EditCustomPoliciesDialog`. Leave the existing read-only "Other attached policies" row (`user.other_policies`) untouched.

- [ ] **Step 5: Run tests + lint/format**

Run: `cd apps/frontend && npm test -- users && npm run lint && npm run format`
Expected: PASS / clean.

- [ ] **Step 6: Commit**

```bash
git add apps/frontend/src/features/users/
git commit -m "feat(users-ui): custom-policy attachment dialog + detail section"
```

---

## Track E — Prometheus Metrics Dashboard

New `internal/metrics` domain: a background poller scrapes `madmin.MetricsClient.ClusterMetrics`/`ResourceMetrics` into a `metrics_samples` SQLite table; a query-time aggregator downsamples and derives counter rates; `GET /api/v1/metrics` returns a plain-JSON view. See design §5. The pool gains a `NewMetricsClient` helper (the only change to existing wiring beyond `serve.go`/config).

### Task E1: Migration + GORM entity for metrics samples

**Files:**
- Create: `apps/backend/migrations/0007_metrics_samples.up.sql`
- Create: `apps/backend/migrations/0007_metrics_samples.down.sql`
- Create: `apps/backend/internal/metrics/entity.go`

- [ ] **Step 1: Write the migration**

Create `apps/backend/migrations/0007_metrics_samples.up.sql` (mirror the format of `0005_audit_events.up.sql`):

```sql
CREATE TABLE metrics_samples (
  id           TEXT PRIMARY KEY,
  collected_at TEXT NOT NULL,
  metric       TEXT NOT NULL,
  value        REAL NOT NULL
);
CREATE INDEX metrics_samples_metric_time_idx ON metrics_samples(metric, collected_at);
CREATE INDEX metrics_samples_collected_at_idx ON metrics_samples(collected_at);
```

Create `apps/backend/migrations/0007_metrics_samples.down.sql`:

```sql
DROP TABLE metrics_samples;
```

- [ ] **Step 2: Write the entity**

Create `apps/backend/internal/metrics/entity.go` (mirror `audit/entity.go`):

```go
package metrics

// metricsSample is the GORM persistence struct for the metrics_samples
// table. Unexported — only this package constructs or reads it.
type metricsSample struct {
	ID          string  `gorm:"column:id;primaryKey"`
	CollectedAt string  `gorm:"column:collected_at;not null"` // RFC3339Nano UTC
	Metric      string  `gorm:"column:metric;not null"`
	Value       float64 `gorm:"column:value;not null"`
}

// TableName satisfies gorm.Tabler.
func (metricsSample) TableName() string { return "metrics_samples" }
```

- [ ] **Step 3: Verify migration runs**

Run: `cd apps/backend && go test ./internal/db/... -v` (the migration suite picks up the embedded `.sql` automatically). If there is no migration test, defer verification to the store test in Task E3 (which migrates an in-memory DB).
Expected: PASS / no error.

- [ ] **Step 4: Commit**

```bash
git add apps/backend/migrations/0007_metrics_samples.up.sql apps/backend/migrations/0007_metrics_samples.down.sql apps/backend/internal/metrics/entity.go
git commit -m "feat(metrics): metrics_samples migration + GORM entity"
```

### Task E2: Domain model + window validation

**Files:**
- Create: `apps/backend/internal/metrics/model.go`
- Test: `apps/backend/internal/metrics/model_test.go` (create)

- [ ] **Step 1: Write the failing test**

Create `apps/backend/internal/metrics/model_test.go`:

```go
package metrics

import "testing"

func TestParseWindow(t *testing.T) {
	cases := map[string]bool{"1h": true, "6h": true, "24h": true, "7d": true, "": false, "30d": false, "bogus": false}
	for in, ok := range cases {
		w, err := ParseWindow(in)
		if ok && err != nil {
			t.Errorf("ParseWindow(%q) unexpected error: %v", in, err)
		}
		if !ok && err == nil {
			t.Errorf("ParseWindow(%q) expected error", in)
		}
		if ok && w.Duration() <= 0 {
			t.Errorf("ParseWindow(%q) duration must be positive", in)
		}
	}
}

func TestWindowStep(t *testing.T) {
	// step must keep each series at <= ~300 points.
	for _, in := range []string{"1h", "6h", "24h", "7d"} {
		w, _ := ParseWindow(in)
		points := int(w.Duration() / w.Step())
		if points > 300 {
			t.Errorf("window %q yields %d points (>300)", in, points)
		}
	}
}
```

Run: `cd apps/backend && go test ./internal/metrics/ -run 'ParseWindow|WindowStep' -v`
Expected: FAIL — undefined `ParseWindow`.

- [ ] **Step 2: Implement the model**

Create `apps/backend/internal/metrics/model.go`:

```go
package metrics

import (
	"errors"
	"time"
)

// Window is a validated dashboard time window.
type Window string

const (
	Window1h  Window = "1h"
	Window6h  Window = "6h"
	Window24h Window = "24h"
	Window7d  Window = "7d"
)

// ErrInvalidWindow is returned by ParseWindow for an unrecognized value.
var ErrInvalidWindow = errors.New("invalid metrics window")

// ParseWindow validates a raw window string.
func ParseWindow(s string) (Window, error) {
	switch Window(s) {
	case Window1h, Window6h, Window24h, Window7d:
		return Window(s), nil
	default:
		return "", ErrInvalidWindow
	}
}

// Duration is the wall-clock span of the window.
func (w Window) Duration() time.Duration {
	switch w {
	case Window1h:
		return time.Hour
	case Window6h:
		return 6 * time.Hour
	case Window24h:
		return 24 * time.Hour
	case Window7d:
		return 7 * 24 * time.Hour
	default:
		return 0
	}
}

// Step is the downsample bucket width, chosen so each series stays at
// <= ~300 points (design §5.2).
func (w Window) Step() time.Duration {
	switch w {
	case Window1h:
		return 60 * time.Second // 60 points
	case Window6h:
		return 300 * time.Second // 72 points
	case Window24h:
		return 300 * time.Second // 288 points
	case Window7d:
		return 1800 * time.Second // 336 points... clamp below
	default:
		return time.Minute
	}
}

// Point is one downsampled value at time T.
type Point struct {
	T time.Time
	V float64
}

// MetricsView is the aggregated dashboard payload.
type MetricsView struct {
	Window      Window
	StepSeconds int
	Collected   bool
	Series      map[string][]Point
}
```

Note: 7d/1800s = 336 points (> 300). Either raise the 7d step to `2400 * time.Second` (252 points) so the `WindowStep` test passes, or relax the test bound to `<= 350`. **Pick raising the step to 2400s** to keep the ≤300 contract; update the comment.

Run: `cd apps/backend && go test ./internal/metrics/ -run 'ParseWindow|WindowStep' -v`
Expected: PASS.

- [ ] **Step 2b: Commit**

```bash
git add apps/backend/internal/metrics/model.go apps/backend/internal/metrics/model_test.go
git commit -m "feat(metrics): window/step model + validation"
```

### Task E3: Store (insert / window query / retention sweep)

**Files:**
- Create: `apps/backend/internal/metrics/store.go`
- Test: `apps/backend/internal/metrics/store_test.go` (create)

- [ ] **Step 1: Write the failing test**

Create `apps/backend/internal/metrics/store_test.go`. Open an in-memory SQLite via the same helper other domains use in tests (find it: `grep -rn "func.*testDB\|glebarez\|gorm.Open" apps/backend/internal/*/.*_test.go` — reuse it; if none is shared, open `gorm.Open(sqlite.Open(":memory:"))` and run `db.Migrate(gdb)` to create the table). Tests:

```go
func TestStoreInsertAndQueryWindow(t *testing.T) {
	st := newTestStore(t) // migrates + returns *Store
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	_ = st.Insert(context.Background(), base, map[string]float64{"minio_s3_requests_total": 100})
	_ = st.Insert(context.Background(), base.Add(time.Minute), map[string]float64{"minio_s3_requests_total": 110})
	pts, err := st.Query(context.Background(), []string{"minio_s3_requests_total"}, base.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(pts["minio_s3_requests_total"]) != 2 {
		t.Fatalf("expected 2 samples, got %d", len(pts["minio_s3_requests_total"]))
	}
}

func TestStoreRetentionSweep(t *testing.T) {
	st := newTestStore(t)
	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	_ = st.Insert(context.Background(), old, map[string]float64{"m": 1})
	n, err := st.RetentionSweep(old.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 deleted, got %d", n)
	}
}
```

Run: `cd apps/backend && go test ./internal/metrics/ -run 'Store' -v`
Expected: FAIL — undefined `Store`.

- [ ] **Step 2: Implement the store**

Create `apps/backend/internal/metrics/store.go` (mirror `audit` insert/query/delete + ULID generation from `audit/processor.go:79-83`):

```go
package metrics

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
)

// Store persists and queries metric samples.
type Store struct {
	db *gorm.DB
}

// NewStore returns a Store backed by db.
func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

// Insert writes one sample row per (metric, value) at collectedAt.
func (s *Store) Insert(ctx context.Context, collectedAt time.Time, values map[string]float64) error {
	ts := collectedAt.UTC().Format(time.RFC3339Nano)
	rows := make([]metricsSample, 0, len(values))
	for metric, v := range values {
		rows = append(rows, metricsSample{ID: newULID(), CollectedAt: ts, Metric: metric, Value: v})
	}
	if len(rows) == 0 {
		return nil
	}
	if err := s.db.WithContext(ctx).Create(&rows).Error; err != nil {
		return fmt.Errorf("metrics.Insert: %w", err)
	}
	return nil
}

// Query returns all samples for the given metrics at or after cutoff,
// grouped by metric and ordered by collected_at ascending.
func (s *Store) Query(ctx context.Context, metrics []string, cutoff time.Time) (map[string][]Point, error) {
	var rows []metricsSample
	if err := s.db.WithContext(ctx).
		Where("metric IN ? AND collected_at >= ?", metrics, cutoff.UTC().Format(time.RFC3339Nano)).
		Order("collected_at ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("metrics.Query: %w", err)
	}
	out := map[string][]Point{}
	for _, r := range rows {
		t, err := time.Parse(time.RFC3339Nano, r.CollectedAt)
		if err != nil {
			continue
		}
		out[r.Metric] = append(out[r.Metric], Point{T: t, V: r.Value})
	}
	return out, nil
}

// RetentionSweep deletes samples older than cutoff and returns the count.
func (s *Store) RetentionSweep(cutoff time.Time) (int64, error) {
	res := s.db.Where("collected_at < ?", cutoff.UTC().Format(time.RFC3339Nano)).Delete(&metricsSample{})
	if res.Error != nil {
		return 0, fmt.Errorf("metrics.RetentionSweep: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// newULID returns a new monotonic ULID string (mirror audit.newULID).
func newULID() string {
	entropy := ulid.Monotonic(rand.New(rand.NewSource(time.Now().UnixNano())), 0) //nolint:gosec
	return ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()
}
```

(Confirm the ulid import path matches `audit/processor.go`'s — likely `github.com/oklog/ulid/v2`. Match it exactly.)

Run: `cd apps/backend && go test ./internal/metrics/ -run 'Store' -v`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add apps/backend/internal/metrics/store.go apps/backend/internal/metrics/store_test.go
git commit -m "feat(metrics): sample store (insert/query/retention)"
```

### Task E4: Collector (madmin MetricsClient → flattened samples)

**Files:**
- Create: `apps/backend/internal/metrics/collector.go`
- Test: `apps/backend/internal/metrics/collector_test.go` (create)

- [ ] **Step 1: Write the failing test**

Create `apps/backend/internal/metrics/collector_test.go` using canned `prom2json.Family` fixtures:

```go
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
```

Run: `cd apps/backend && go test ./internal/metrics/ -run 'Flatten' -v`
Expected: FAIL — undefined `flattenFamilies`.

- [ ] **Step 2: Implement the collector**

Create `apps/backend/internal/metrics/collector.go`:

```go
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
```

Run: `cd apps/backend && go mod tidy && go test ./internal/metrics/ -run 'Flatten' -v`
(`go mod tidy` drops the `// indirect` marker on `prom2json` now that we import it directly — context.md §build.)
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add apps/backend/internal/metrics/collector.go apps/backend/internal/metrics/collector_test.go apps/backend/go.mod apps/backend/go.sum
git commit -m "feat(metrics): collector flattening tracked prometheus families"
```

### Task E5: Aggregator (downsample + rate derivation)

**Files:**
- Create: `apps/backend/internal/metrics/aggregator.go`
- Test: `apps/backend/internal/metrics/aggregator_test.go` (create)

- [ ] **Step 1: Write the failing test**

Create `apps/backend/internal/metrics/aggregator_test.go`:

```go
package metrics

import (
	"testing"
	"time"
)

func TestAggregateCounterRate(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// counter rising 0 → 60 over 60s ⇒ rate ~1/s
	raw := map[string][]Point{
		"minio_s3_requests_total": {
			{T: base, V: 0},
			{T: base.Add(60 * time.Second), V: 60},
		},
	}
	view := Aggregate(Window1h, raw, base.Add(2*time.Minute))
	pts := view.Series["minio_s3_requests_total"]
	if len(pts) == 0 {
		t.Fatal("expected rate points")
	}
	// last non-zero rate bucket should be ~1.0/s
	var maxRate float64
	for _, p := range pts {
		if p.V > maxRate {
			maxRate = p.V
		}
	}
	if maxRate < 0.8 || maxRate > 1.2 {
		t.Errorf("expected ~1/s rate, got %v", maxRate)
	}
}

func TestAggregateCounterResetClampedToZero(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	raw := map[string][]Point{
		"minio_s3_requests_total": {
			{T: base, V: 100},
			{T: base.Add(60 * time.Second), V: 5}, // counter reset
		},
	}
	view := Aggregate(Window1h, raw, base.Add(2*time.Minute))
	for _, p := range view.Series["minio_s3_requests_total"] {
		if p.V < 0 {
			t.Errorf("reset must clamp to >= 0, got %v", p.V)
		}
	}
}

func TestAggregateGaugePassthrough(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	raw := map[string][]Point{
		"minio_cluster_drive_online_total": {
			{T: base, V: 4},
			{T: base.Add(60 * time.Second), V: 4},
		},
	}
	view := Aggregate(Window1h, raw, base.Add(2*time.Minute))
	pts := view.Series["minio_cluster_drive_online_total"]
	if len(pts) == 0 || pts[len(pts)-1].V != 4 {
		t.Errorf("gauge should pass through as 4, got %+v", pts)
	}
}
```

Run: `cd apps/backend && go test ./internal/metrics/ -run 'Aggregate' -v`
Expected: FAIL — undefined `Aggregate`.

- [ ] **Step 2: Implement the aggregator**

Create `apps/backend/internal/metrics/aggregator.go`:

```go
package metrics

import "time"

// Aggregate downsamples raw samples into <=~300 points per series for the
// window. Counters become per-second rates (negative deltas from resets
// clamped to 0); gauges pass through as the last value in each bucket. now
// is the upper bound of the window (injected for deterministic tests).
func Aggregate(w Window, raw map[string][]Point, now time.Time) MetricsView {
	step := w.Step()
	start := now.Add(-w.Duration())
	view := MetricsView{
		Window:      w,
		StepSeconds: int(step / time.Second),
		Collected:   len(raw) > 0,
		Series:      map[string][]Point{},
	}
	for metric, pts := range raw {
		if _, isCounter := counterMetrics[metric]; isCounter {
			view.Series[metric] = downsampleRate(pts, start, now, step)
		} else {
			view.Series[metric] = downsampleGauge(pts, start, now, step)
		}
	}
	return view
}

// bucketIndex returns the step bucket a timestamp falls into.
func bucketIndex(t, start time.Time, step time.Duration) int {
	return int(t.Sub(start) / step)
}

// downsampleGauge takes the last value seen in each step bucket.
func downsampleGauge(pts []Point, start, now time.Time, step time.Duration) []Point {
	buckets := map[int]Point{}
	for _, p := range pts {
		if p.T.Before(start) || p.T.After(now) {
			continue
		}
		idx := bucketIndex(p.T, start, step)
		buckets[idx] = Point{T: start.Add(time.Duration(idx) * step), V: p.V}
	}
	return orderedBuckets(buckets)
}

// downsampleRate computes a per-second counter rate per step bucket from the
// delta between the first and last raw sample in that bucket; counter resets
// (negative delta) clamp to 0.
func downsampleRate(pts []Point, start, now time.Time, step time.Duration) []Point {
	type span struct{ first, last Point }
	spans := map[int]*span{}
	for _, p := range pts {
		if p.T.Before(start) || p.T.After(now) {
			continue
		}
		idx := bucketIndex(p.T, start, step)
		s := spans[idx]
		if s == nil {
			spans[idx] = &span{first: p, last: p}
			continue
		}
		s.last = p
	}
	out := map[int]Point{}
	for idx, s := range spans {
		dt := s.last.T.Sub(s.first.T).Seconds()
		rate := 0.0
		if dt > 0 {
			delta := s.last.V - s.first.V
			if delta < 0 {
				delta = 0 // counter reset
			}
			rate = delta / dt
		}
		out[idx] = Point{T: start.Add(time.Duration(idx) * step), V: rate}
	}
	return orderedBuckets(out)
}

// orderedBuckets returns bucket points sorted by time.
func orderedBuckets(buckets map[int]Point) []Point {
	if len(buckets) == 0 {
		return nil
	}
	maxIdx := -1
	for idx := range buckets {
		if idx > maxIdx {
			maxIdx = idx
		}
	}
	out := make([]Point, 0, len(buckets))
	for i := 0; i <= maxIdx; i++ {
		if p, ok := buckets[i]; ok {
			out = append(out, p)
		}
	}
	return out
}
```

Run: `cd apps/backend && go test ./internal/metrics/ -run 'Aggregate' -v`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add apps/backend/internal/metrics/aggregator.go apps/backend/internal/metrics/aggregator_test.go
git commit -m "feat(metrics): query-time downsample + counter-rate aggregator"
```

### Task E6: Processor + poller

**Files:**
- Create: `apps/backend/internal/metrics/processor.go`
- Create: `apps/backend/internal/metrics/poller.go`
- Test: `apps/backend/internal/metrics/poller_test.go` (create)

- [ ] **Step 1: Write the processor (view assembly + paused detection)**

Create `apps/backend/internal/metrics/processor.go`:

```go
package metrics

import (
	"context"
	"time"
)

// Processor assembles the dashboard view from stored samples.
type Processor struct {
	store      *Store
	pollPeriod time.Duration
	now        func() time.Time // injectable for tests
}

// NewProcessor returns a Processor reading from store. pollPeriod is used to
// decide the "paused" indicator (no fresh sample within ~2 polls).
func NewProcessor(store *Store, pollPeriod time.Duration) *Processor {
	return &Processor{store: store, pollPeriod: pollPeriod, now: time.Now}
}

// trackedNames is every metric the view may contain.
func trackedNames() []string {
	out := make([]string, 0, len(trackedMetrics))
	for n := range trackedMetrics {
		out = append(out, n)
	}
	return out
}

// View returns the aggregated MetricsView for window w. Collected is false
// when no samples exist in the window OR the newest sample is older than
// ~2 poll periods (collection paused / MinIO down).
func (p *Processor) View(ctx context.Context, w Window) (MetricsView, error) {
	now := p.now().UTC()
	raw, err := p.store.Query(ctx, trackedNames(), now.Add(-w.Duration()))
	if err != nil {
		return MetricsView{}, err
	}
	view := Aggregate(w, raw, now)
	view.Collected = p.isFresh(raw, now)
	return view, nil
}

// isFresh reports whether any series has a sample within ~2 poll periods.
func (p *Processor) isFresh(raw map[string][]Point, now time.Time) bool {
	if len(raw) == 0 {
		return false
	}
	staleAfter := 2 * p.pollPeriod
	for _, pts := range raw {
		if len(pts) == 0 {
			continue
		}
		last := pts[len(pts)-1].T
		if now.Sub(last) <= staleAfter {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Write the failing poller test**

Create `apps/backend/internal/metrics/poller_test.go`:

```go
package metrics

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeCollector struct {
	values map[string]float64
	err    error
}

func (f fakeCollector) Collect(_ context.Context) (map[string]float64, error) {
	return f.values, f.err
}

func TestPollOnce_SuccessWritesSample(t *testing.T) {
	st := newTestStore(t)
	c := fakeCollector{values: map[string]float64{"minio_s3_requests_total": 7}}
	if err := pollOnce(context.Background(), c, st, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	pts, _ := st.Query(context.Background(), []string{"minio_s3_requests_total"}, time.Time{})
	if len(pts["minio_s3_requests_total"]) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(pts["minio_s3_requests_total"]))
	}
}

func TestPollOnce_ErrorWritesNothing(t *testing.T) {
	st := newTestStore(t)
	c := fakeCollector{err: errors.New("minio down")}
	if err := pollOnce(context.Background(), c, st, time.Now().UTC()); err == nil {
		t.Fatal("expected error")
	}
	pts, _ := st.Query(context.Background(), []string{"minio_s3_requests_total"}, time.Time{})
	if len(pts) != 0 {
		t.Fatalf("expected no samples on error, got %v", pts)
	}
}
```

(Define a `collectorIface` the poller depends on so `fakeCollector` substitutes for `*Collector`.)

Run: `cd apps/backend && go test ./internal/metrics/ -run 'PollOnce' -v`
Expected: FAIL — undefined `pollOnce`.

- [ ] **Step 3: Implement the poller**

Create `apps/backend/internal/metrics/poller.go` (mirror `audit.StartRetentionSweeper`'s ticker/select; `audit/retention.go:14-35`):

```go
package metrics

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

// collectorIface is the poller's view of a Collector (lets tests stub it).
type collectorIface interface {
	Collect(ctx context.Context) (map[string]float64, error)
}

// pollOnce collects once and, on success, writes one timestamped sample
// batch. On collect error nothing is written (natural gap) and the error is
// returned for logging.
func pollOnce(ctx context.Context, c collectorIface, store *Store, at time.Time) error {
	values, err := c.Collect(ctx)
	if err != nil {
		return err
	}
	return store.Insert(ctx, at, values)
}

// StartPoller launches the background collection goroutine. On each tick it
// collects + stores; collect errors are logged and skipped (gap), never
// fatal. Exits when ctx is cancelled.
func StartPoller(ctx context.Context, c collectorIface, store *Store, every time.Duration) {
	go func() {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := pollOnce(ctx, c, store, time.Now().UTC()); err != nil {
					log.Ctx(ctx).Warn().Err(err).Msg("metrics poll failed; skipping sample")
				}
			}
		}
	}()
}

// StartRetentionSweeper deletes samples older than retention on a fixed
// cadence (mirror audit). Exits when ctx is cancelled.
func StartRetentionSweeper(ctx context.Context, store *Store, retention, every time.Duration) {
	go func() {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cutoff := time.Now().UTC().Add(-retention)
				if n, err := store.RetentionSweep(cutoff); err != nil {
					log.Ctx(ctx).Error().Err(err).Msg("metrics retention sweep failed")
				} else if n > 0 {
					log.Ctx(ctx).Info().Int64("deleted", n).Msg("metrics retention sweep complete")
				}
			}
		}
	}()
}
```

Run: `cd apps/backend && go test ./internal/metrics/ -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add apps/backend/internal/metrics/processor.go apps/backend/internal/metrics/poller.go apps/backend/internal/metrics/poller_test.go
git commit -m "feat(metrics): view processor + background poller + retention sweeper"
```

### Task E7: Config — poll interval + retention

**Files:**
- Modify: `apps/backend/internal/config/config.go`
- Test: `apps/backend/internal/config/config_test.go` (extend if present)

- [ ] **Step 1: Add the fields, defaults, reads**

In `config.go`, add to `Config` (after `AuditRetention`):

```go
	MetricsPollInterval       time.Duration
	MetricsRetention          time.Duration
```

In `defaults()` add:

```go
	v.SetDefault("METRICS_POLL_INTERVAL", 30*time.Second)
	v.SetDefault("METRICS_RETENTION", 8*24*time.Hour)
```

In `Load()`'s `cfg := Config{...}` add:

```go
		MetricsPollInterval:      v.GetDuration("METRICS_POLL_INTERVAL"),
		MetricsRetention:         v.GetDuration("METRICS_RETENTION"),
```

In `validate()` add:

```go
	if c.MetricsPollInterval <= 0 {
		return errors.New("HARBORMASTER_METRICS_POLL_INTERVAL must be positive")
	}
	if c.MetricsRetention <= 0 {
		return errors.New("HARBORMASTER_METRICS_RETENTION must be positive")
	}
```

(Leave the existing unrelated `MetricsEnabled`/`MetricsListenAddr` app-self-metrics fields untouched — design §5.3.)

- [ ] **Step 2: Build + test**

Run: `cd apps/backend && CGO_ENABLED=0 go build ./internal/config/... && go test ./internal/config/ -v`
Expected: clean / PASS.

- [ ] **Step 3: Commit**

```bash
git add apps/backend/internal/config/config.go apps/backend/internal/config/config_test.go
git commit -m "feat(config): metrics poll interval + retention settings"
```

### Task E8: Pool — NewMetricsClient

**Files:**
- Modify: `apps/backend/internal/minio/pool.go`
- Test: `apps/backend/internal/minio/pool_test.go` (extend if present)

- [ ] **Step 1: Add the method**

In `pool.go`, add a method that builds a `madmin.MetricsClient` from the cached credentials + the same custom transport. It reuses `parseEndpoint`/`transport` already in the file:

```go
// NewMetricsClient builds a madmin MetricsClient from the active connection,
// reusing the pool's TLS transport (custom CA / skip-verify). Returns
// ErrNotInitialized when no connection is configured. The Prometheus bearer
// token is minted internally by the SDK.
func (p *Pool) NewMetricsClient(ctx context.Context) (*madmin.MetricsClient, error) {
	_ = ctx
	p.mu.RLock()
	cred := p.cred
	ready := p.mc != nil && p.madm != nil
	p.mu.RUnlock()
	if !ready {
		return nil, ErrNotInitialized
	}
	_, useTLS, host, err := parseEndpoint(cred.EndpointURL)
	if err != nil {
		return nil, err
	}
	tr, err := transport(cred, useTLS)
	if err != nil {
		return nil, err
	}
	mcl, err := madmin.NewMetricsClient(host, cred.AccessKey, cred.SecretKey, useTLS)
	if err != nil {
		return nil, err
	}
	mcl.SetCustomTransport(tr)
	return mcl, nil
}
```

- [ ] **Step 2: Build + vet**

Run: `cd apps/backend && CGO_ENABLED=0 go build ./internal/minio/... && go vet ./internal/minio/...`
Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add apps/backend/internal/minio/pool.go
git commit -m "feat(minio): Pool.NewMetricsClient reusing the pool transport"
```

### Task E9: REST — GET /api/v1/metrics

**Files:**
- Create: `apps/backend/internal/metrics/rest.go`
- Create: `apps/backend/internal/metrics/resource.go`
- Test: `apps/backend/internal/metrics/resource_test.go` (create)

- [ ] **Step 1: Write the wire shape + handler**

Create `apps/backend/internal/metrics/rest.go` (plain-JSON aggregate, design §5.2 — NOT JSON:API):

```go
package metrics

// viewResponse is the plain-JSON body of GET /api/v1/metrics.
type viewResponse struct {
	Window      string                   `json:"window"`
	StepSeconds int                      `json:"step_seconds"`
	Collected   bool                     `json:"collected"`
	Series      map[string][]pointWire   `json:"series"`
}

type pointWire struct {
	T string  `json:"t"` // RFC3339
	V float64 `json:"v"`
}

// toResponse converts a MetricsView to the wire body.
func toResponse(v MetricsView) viewResponse {
	out := viewResponse{
		Window:      string(v.Window),
		StepSeconds: v.StepSeconds,
		Collected:   v.Collected,
		Series:      map[string][]pointWire{},
	}
	for name, pts := range v.Series {
		wire := make([]pointWire, 0, len(pts))
		for _, p := range pts {
			wire = append(wire, pointWire{T: p.T.UTC().Format("2006-01-02T15:04:05Z07:00"), V: p.V})
		}
		out.Series[name] = wire
	}
	return out
}
```

Create `apps/backend/internal/metrics/resource.go`:

```go
package metrics

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
)

type handler struct {
	p *Processor
}

// Routes mounts GET /metrics (action-style plain JSON).
func Routes(p *Processor) func(chi.Router) {
	h := &handler{p: p}
	return func(r chi.Router) {
		r.Get("/metrics", h.view)
	}
}

func (h *handler) view(w http.ResponseWriter, r *http.Request) {
	win, err := ParseWindow(r.URL.Query().Get("window"))
	if err != nil {
		apierror.Write(w, apierror.StyleAction, apierror.New(http.StatusUnprocessableEntity,
			"invalid_metrics_window", "window must be one of 1h, 6h, 24h, 7d"))
		return
	}
	view, err := h.p.View(r.Context(), win)
	if err != nil {
		apierror.Write(w, apierror.StyleAction, apierror.New(http.StatusInternalServerError,
			"internal_error", "failed to assemble metrics view"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(toResponse(view))
}
```

- [ ] **Step 2: HTTP tests**

Create `resource_test.go`: build a `Processor` over an in-memory store, insert a couple of samples, and assert: `GET /metrics?window=1h` → 200 with `series` populated and `collected:true`; `GET /metrics?window=30d` → 422 `invalid_metrics_window`; empty store → 200 `collected:false`. Mirror the chi+httptest setup from another resource test.

Run: `cd apps/backend && go test ./internal/metrics/ -v`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add apps/backend/internal/metrics/rest.go apps/backend/internal/metrics/resource.go apps/backend/internal/metrics/resource_test.go
git commit -m "feat(metrics): GET /api/v1/metrics plain-JSON endpoint"
```

### Task E10: Wire metrics in cmd/harbormaster

**Files:**
- Modify: `apps/backend/cmd/harbormaster/serve.go`
- Modify: `apps/backend/cmd/harbormaster/audit_adapter.go`

- [ ] **Step 1: Add the metrics source getter**

In `audit_adapter.go`, add an adapter so `*madmin.MetricsClient` satisfies `metrics.MetricsSource` (it already has `ClusterMetrics`/`ResourceMetrics` — structural), wired through the pool:

```go
// newMetricsSourceGetter returns a metrics.SourceGetter bound to the live
// pool. Each call builds a fresh madmin MetricsClient (cheap; re-reads creds
// + transport) so credential rotations are picked up automatically.
func newMetricsSourceGetter(pool *hmminio.Pool) metrics.SourceGetter {
	return func(ctx context.Context) (metrics.MetricsSource, error) {
		return pool.NewMetricsClient(ctx)
	}
}
```

Add the `metrics` import.

- [ ] **Step 2: Construct store/collector/processor; start goroutines; mount route**

In `serve.go`, after the DB/`auditProc` setup (the migration already ran via `db.Migrate`), add:

```go
metricsStore := metrics.NewStore(gdb)
metricsCollector := metrics.NewCollector(newMetricsSourceGetter(pool))
metricsProc := metrics.NewProcessor(metricsStore, cfg.MetricsPollInterval)
metrics.StartPoller(ctx, metricsCollector, metricsStore, cfg.MetricsPollInterval)
metrics.StartRetentionSweeper(ctx, metricsStore, cfg.MetricsRetention, 24*time.Hour)
```

(Place near the audit sweeper start, `serve.go:87-88`.) In `protectedRoutes`, add after `audit.Routes(...)`:

```go
		metrics.Routes(metricsProc)(g)
```

- [ ] **Step 3: Build + vet + full backend test**

Run: `cd apps/backend && CGO_ENABLED=0 go build ./... && go vet ./... && go test -race -count=1 ./...`
Expected: clean / PASS.

- [ ] **Step 4: Commit**

```bash
git add apps/backend/cmd/harbormaster/serve.go apps/backend/cmd/harbormaster/audit_adapter.go
git commit -m "feat(metrics): wire store/collector/poller + /metrics route in serve"
```

### Task E11: Frontend — Metrics page

**Files:**
- Modify: `apps/frontend/src/lib/api/keys.ts` (`metricsKeys`)
- Create: `apps/frontend/src/features/metrics/api.ts`
- Create: `apps/frontend/src/features/metrics/types.ts`
- Create: `apps/frontend/src/features/metrics/MetricsPage.tsx`
- Test: `apps/frontend/src/features/metrics/MetricsPage.test.tsx` (create)

- [ ] **Step 1: Keys + types + API**

In `keys.ts` add:

```ts
export const metricsKeys = {
  view: (window: string) => ["metrics", "view", window] as const,
};
```

Create `features/metrics/types.ts`:

```ts
export type MetricsWindow = "1h" | "6h" | "24h" | "7d";

export type MetricPoint = { t: string; v: number };

export type MetricsView = {
  window: string;
  step_seconds: number;
  collected: boolean;
  series: Record<string, MetricPoint[]>;
};
```

Create `features/metrics/api.ts`:

```ts
import { api } from "@/lib/api/client";
import type { MetricsView, MetricsWindow } from "./types";

export async function fetchMetrics(window: MetricsWindow): Promise<MetricsView> {
  return api.get<MetricsView>(`/api/v1/metrics?window=${encodeURIComponent(window)}`);
}
```

- [ ] **Step 2: Write the failing page test**

Create `MetricsPage.test.tsx`: mock `fetchMetrics` returning `collected:false` and assert the "metrics collection paused" banner renders; then a `collected:true` payload with a request series and assert a chart container renders (query by role/text). Mirror `DashboardPage`/`BucketSizeChart` test patterns.

Run: `cd apps/frontend && npm test -- MetricsPage`
Expected: FAIL — component doesn't exist.

- [ ] **Step 3: Implement `MetricsPage.tsx`**

Mirror `DashboardPage.tsx` window-persistence (`localStorage`, key `harbormaster:metrics:window`, default `"24h"`) and `BucketSizeChart.tsx` Recharts/`ChartContainer` usage. A window `Select` (1h/6h/24h/7d). `useQuery(metricsKeys.view(window), () => fetchMetrics(window), { refetchInterval: 30_000 })`. When `data.collected === false`, render an `Alert` ("Metrics collection paused — no recent samples. This is normal on a fresh install or when MinIO is unreachable.") instead of charts. Otherwise render line/area charts: request rate (`minio_s3_requests_total`), error rates (`minio_s3_requests_4xx_errors_total`/`_5xx_`), throughput area (`minio_s3_traffic_received_bytes`/`_sent_bytes`), capacity + drives (`minio_cluster_*`). Map each series' `MetricPoint[]` into the Recharts data shape (`{ t, v }`), formatting `t` for the X axis.

- [ ] **Step 4: Run tests + lint/format**

Run: `cd apps/frontend && npm test -- metrics && npm run lint && npm run format`
Expected: PASS / clean.

- [ ] **Step 5: Commit**

```bash
git add apps/frontend/src/features/metrics/ apps/frontend/src/lib/api/keys.ts
git commit -m "feat(metrics-ui): metrics dashboard page with windowed charts"
```

---

## Track F — Nav / Route Integration + Final Verification

Registers the Metrics nav entry and route, confirms the repurposed Policies route, and runs the full done-bar across both apps. Most route/nav wiring landed in C7/E11; this track consolidates and verifies the coordinated release.

### Task F1: Metrics nav entry + route

**Files:**
- Modify: `apps/frontend/src/components/AppShell.tsx`
- Modify: `apps/frontend/src/routes.tsx`

- [ ] **Step 1: Add the nav entry**

In `AppShell.tsx`, add `LineChart` to the lucide-react import block (line 2-15) and add a NAV entry (line 44-51), placed adjacent to Dashboard:

```ts
  { to: "/metrics", label: "Metrics", icon: LineChart },
```

- [ ] **Step 2: Add the route**

In `routes.tsx`, import `MetricsPage` (`import { MetricsPage } from "@/features/metrics/MetricsPage";`) and add under the `<AppShell>` block (mirror line 41):

```tsx
      <Route path="/metrics" element={<MetricsPage />} />
```

Confirm `/policies` now points at `PoliciesPage` (done in C7); if `PolicyTemplatesPage` is now unreferenced, ensure it was removed in C7 or remove it here.

- [ ] **Step 3: Build the frontend**

Run: `cd apps/frontend && npm run build`
Expected: clean (no unused-import or missing-module errors).

- [ ] **Step 4: Commit**

```bash
git add apps/frontend/src/components/AppShell.tsx apps/frontend/src/routes.tsx
git commit -m "feat(nav): add Metrics nav entry + route; confirm Policies route"
```

### Task F2: Full done-bar verification

**Files:** none (verification only).

- [ ] **Step 1: Backend done-bar**

Run, from `apps/backend`:

```bash
go test -race -count=1 ./...
go vet ./...
golangci-lint run
CGO_ENABLED=0 go build ./...
```

Expected: all clean. Fix any cross-track integration failures (e.g. an interface method missed on a stub, an import cycle between `users` and `policies` — there must be none: `users` imports `policies`, never the reverse). If `golangci-lint` flags the new files, address per the existing lint config (no new `//nolint` beyond those already shown).

- [ ] **Step 2: Frontend done-bar**

Run, from `apps/frontend`:

```bash
npm ci
npm run lint
npm run format
npm test
npm run build
```

Expected: all clean.

- [ ] **Step 3: Manual smoke checklist (document results in the PR)**

Verify the four P0 surfaces render and round-trip against a dev MinIO (or note as deferred to on-demand integration/E2E per CLAUDE.md): version sheet restore/undelete; lifecycle noncurrent/abort-mpu create; policy create/edit/delete + `policy_in_use` guard; user custom-policy attach/detach; metrics page with live + paused states.

- [ ] **Step 4: Final commit (if any verification fixes were made)**

```bash
git add -A
git commit -m "chore(task-002): final cross-track verification fixes"
```

> Note: `git add -A` is acceptable here ONLY for a verification-fix sweep in the isolated task worktree; per controller discipline, individual implementer subagents must stage explicit paths, never `git add -A`/`git add .`.

---

## Plan Self-Review (Tracks C–F)

- **Spec coverage:** Track C → design §4.1-4.3 + §1.1/§1.2 (policy verbs + error codes); Track D → design §4.4 (Open Q #4 safety property); Track E → design §5 (all subsections) + §1.3 nav; Track F → design §1.3 + §6 done-bar. PRD P0 gaps #3 (policy editor) and #4 (metrics) are now covered alongside #1/#2 (Tracks A/B).
- **Type consistency:** `OriginFor`/`EditableFor`/`IsBuiltin`/`isTemplateName` are defined once (C2) and reused in C3/C4/D3. `Window.Step()`/`Duration()` are used consistently in E2/E5/E6. `MetricsSource`/`SourceGetter`/`collectorIface` names are stable across E4/E6/E10.
- **Cross-track ordering:** D depends on C2's classifier (must land first). E is independent. F depends on C7+E11.
- **Known follow-through (not placeholders — explicit instructions to the implementer):** exact Prometheus metric names (E4 `trackedMetrics`) are per design §5.1 and must be confirmed against a live MinIO in the on-demand integration test; the `assertAPIError`/`newTestStore`/stub-construction helpers must mirror the package's existing test helpers (inspect first, do not invent).
