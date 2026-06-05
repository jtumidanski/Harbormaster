package lifecycle

import (
	"strings"
	"testing"

	mlifecycle "github.com/minio/minio-go/v7/pkg/lifecycle"
)

// TestClassifyManaged covers the happy path: a rule whose ID matches
// the managed format AND whose server-side config matches the exact
// subset Harbormaster's create form produces (expiration-only, no
// transitions, no abort-incomplete-multipart, no tag filters) is
// classified Managed=true and the structured (days, prefix) values
// are surfaced as-is.
func TestClassifyManaged(t *testing.T) {
	t.Parallel()
	r := mlifecycle.Rule{
		ID:     "harbormaster-expire-30d-uploads",
		Status: "Enabled",
		Expiration: mlifecycle.Expiration{
			Days: mlifecycle.ExpirationDays(30),
		},
		RuleFilter: mlifecycle.Filter{Prefix: "uploads/"},
	}
	got := classify(r)
	if !got.Managed {
		t.Fatalf("classify(managed-shape) → Managed=false; want true. Rule: %#v", got)
	}
	if got.Kind != "expiration" {
		t.Errorf("Kind = %q; want %q", got.Kind, "expiration")
	}
	if got.Days != 30 {
		t.Errorf("Days = %d; want 30", got.Days)
	}
	if got.Prefix != "uploads/" {
		t.Errorf("Prefix = %q; want %q", got.Prefix, "uploads/")
	}
	if got.Summary != "" {
		t.Errorf("Summary = %q; want empty for managed rule", got.Summary)
	}
}

// TestClassifyUnmanaged_TransitionPresent locks in the rule that a
// matching ID is not sufficient on its own — any extra action (here,
// a transition) flips the rule to unmanaged even though the ID looks
// like ours.
func TestClassifyUnmanaged_TransitionPresent(t *testing.T) {
	t.Parallel()
	r := mlifecycle.Rule{
		ID:     "harbormaster-expire-30d-uploads",
		Status: "Enabled",
		Expiration: mlifecycle.Expiration{
			Days: mlifecycle.ExpirationDays(30),
		},
		Transition: mlifecycle.Transition{
			Days:         mlifecycle.ExpirationDays(7),
			StorageClass: "GLACIER",
		},
		RuleFilter: mlifecycle.Filter{Prefix: "uploads/"},
	}
	got := classify(r)
	if got.Managed {
		t.Fatalf("classify(rule-with-transition) → Managed=true; want false")
	}
	if !strings.Contains(got.Summary, "Transition") {
		t.Errorf("Summary = %q; expected mention of Transition", got.Summary)
	}
}

// TestClassifyUnmanaged_RandomID locks in the cheaper unmanaged path:
// any rule ID that doesn't match the managedIDRE pattern is unmanaged,
// even if the body happens to look expiration-only.
func TestClassifyUnmanaged_RandomID(t *testing.T) {
	t.Parallel()
	r := mlifecycle.Rule{
		ID:     "my-custom-rule",
		Status: "Enabled",
		Expiration: mlifecycle.Expiration{
			Days: mlifecycle.ExpirationDays(30),
		},
	}
	got := classify(r)
	if got.Managed {
		t.Fatalf("classify(random-id) → Managed=true; want false")
	}
	if got.ID != "my-custom-rule" {
		t.Errorf("ID = %q; want %q (preserved verbatim)", got.ID, "my-custom-rule")
	}
}

// TestSummaryNeverContainsTagValues is the security-sensitive
// regression: the summary string must list a tag-filter *count*, never
// the tag keys or values, because tag contents may be sensitive
// (PII-bearing labels, customer IDs, etc.). We construct a rule with
// two tag filters carrying obvious sentinel keys/values and assert
// none of them leak into the summary.
func TestSummaryNeverContainsTagValues(t *testing.T) {
	t.Parallel()
	const (
		key1 = "SENSITIVE_KEY_ONE"
		val1 = "SENSITIVE_VAL_ONE"
		key2 = "SENSITIVE_KEY_TWO"
		val2 = "SENSITIVE_VAL_TWO"
	)
	r := mlifecycle.Rule{
		ID:     "mc-imported-rule",
		Status: "Enabled",
		Expiration: mlifecycle.Expiration{
			Days: mlifecycle.ExpirationDays(7),
		},
		RuleFilter: mlifecycle.Filter{
			And: mlifecycle.And{
				Tags: []mlifecycle.Tag{
					{Key: key1, Value: val1},
					{Key: key2, Value: val2},
				},
			},
		},
	}
	got := classify(r)
	if got.Managed {
		t.Fatalf("classify(tagged-rule) → Managed=true; tagged rules must be unmanaged")
	}
	if !strings.Contains(got.Summary, "2 tag filter(s)") {
		t.Errorf("Summary = %q; expected to mention '2 tag filter(s)'", got.Summary)
	}
	for _, leak := range []string{key1, val1, key2, val2} {
		if strings.Contains(got.Summary, leak) {
			t.Errorf("Summary leaked tag key/value %q: %s", leak, got.Summary)
		}
	}
}

// TestCountTagFilters checks both shapes the upstream Filter supports:
// the single Tag field (legacy single-tag scoping) and the And.Tags
// slice (multi-tag conjunction). The two contribute to the same count
// so a rule with both surfaces a non-zero total without
// double-counting.
func TestCountTagFilters(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		rule mlifecycle.Rule
		want int
	}{
		{
			name: "no tags",
			rule: mlifecycle.Rule{ID: "x"},
			want: 0,
		},
		{
			name: "single Tag only",
			rule: mlifecycle.Rule{
				ID: "x",
				RuleFilter: mlifecycle.Filter{
					Tag: mlifecycle.Tag{Key: "k", Value: "v"},
				},
			},
			want: 1,
		},
		{
			name: "And.Tags only",
			rule: mlifecycle.Rule{
				ID: "x",
				RuleFilter: mlifecycle.Filter{
					And: mlifecycle.And{
						Tags: []mlifecycle.Tag{
							{Key: "k1", Value: "v1"},
							{Key: "k2", Value: "v2"},
							{Key: "k3", Value: "v3"},
						},
					},
				},
			},
			want: 3,
		},
		{
			name: "both single and And",
			rule: mlifecycle.Rule{
				ID: "x",
				RuleFilter: mlifecycle.Filter{
					Tag: mlifecycle.Tag{Key: "k", Value: "v"},
					And: mlifecycle.And{
						Tags: []mlifecycle.Tag{
							{Key: "k1", Value: "v1"},
						},
					},
				},
			},
			want: 2,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := countTagFilters(tc.rule); got != tc.want {
				t.Errorf("countTagFilters() = %d; want %d", got, tc.want)
			}
		})
	}
}

// TestGenerateRuleIDFormat locks in the deterministic ID format the
// classifier relies on to recognise our own rules. A drift here that
// is not mirrored in expireIDRE would silently re-classify every
// previously-managed rule as unmanaged on the next list call.
func TestGenerateRuleIDFormat(t *testing.T) {
	t.Parallel()
	cases := []struct {
		days   int
		prefix string
		want   string
	}{
		{30, "", "harbormaster-expire-30d"},
		{30, "uploads/", "harbormaster-expire-30d-uploads"},
		{1, "Photos/2025/Vacation!", "harbormaster-expire-1d-photos-2025-vacation"},
	}
	for _, tc := range cases {
		got := generateRuleID(tc.days, tc.prefix)
		if got != tc.want {
			t.Errorf("generateRuleID(%d, %q) = %q; want %q", tc.days, tc.prefix, got, tc.want)
		}
		if !expireIDRE.MatchString(got) {
			t.Errorf("generateRuleID(%d, %q) = %q does NOT match expireIDRE", tc.days, tc.prefix, got)
		}
	}
}

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
