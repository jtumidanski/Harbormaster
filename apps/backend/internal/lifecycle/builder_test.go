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
