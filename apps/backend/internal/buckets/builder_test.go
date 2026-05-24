package buckets

import (
	"strings"
	"testing"
)

// TestBuilderRejectsInvalidNames is the table-driven sweep for
// ValidateBucketName / NewBucketBuilder.Build. Each row captures one
// MinIO bucket-naming rule; valid rows assert that Build succeeds.
func TestBuilderRejectsInvalidNames(t *testing.T) {
	cases := []struct {
		name        string
		input       string
		wantErr     bool
		errContains string
	}{
		{name: "ipv4_address", input: "192.168.0.1", wantErr: true, errContains: "IP address"},
		{name: "uppercase", input: "Photos", wantErr: true, errContains: "invalid character"},
		{name: "too_short", input: "a", wantErr: true, errContains: "3-63"},
		{name: "leading_dots", input: "..a", wantErr: true, errContains: "must not start"},
		{name: "adjacent_dots", input: "a..b", wantErr: true, errContains: "adjacent dots"},
		{name: "trailing_hyphen", input: "abc-", wantErr: true, errContains: "must not end"},
		{name: "underscore", input: "my_bucket", wantErr: true, errContains: "invalid character"},
		{
			// 64 chars: just over the upper bound.
			name:        "too_long_64",
			input:       strings.Repeat("a", 64),
			wantErr:     true,
			errContains: "3-63",
		},
		{name: "empty", input: "", wantErr: true, errContains: "required"},
		// Valid cases.
		{name: "valid_simple", input: "photos", wantErr: false},
		{name: "valid_with_hyphen", input: "my-bucket", wantErr: false},
		{name: "valid_with_dot", input: "logs.2026", wantErr: false},
		{name: "valid_min_length", input: "abc", wantErr: false},
		{name: "valid_max_length", input: strings.Repeat("a", 63), wantErr: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := NewBucketBuilder().Name(tc.input).Build()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got nil (bucket=%+v)", tc.input, b)
				}
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Fatalf("error for %q missing substring %q: %v", tc.input, tc.errContains, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.input, err)
			}
			if b.Name != tc.input {
				t.Fatalf("Build dropped the name: got %q want %q", b.Name, tc.input)
			}
		})
	}
}
