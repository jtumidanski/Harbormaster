package audit_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jtumidanski/Harbormaster/internal/audit"
)

func TestSanitize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input map[string]any
		want  map[string]any
	}{
		{
			name:  "nil input returns nil",
			input: nil,
			want:  nil,
		},
		{
			name: "sensitive keys are removed; safe keys are preserved",
			input: map[string]any{
				"secret_key":    "A",
				"password":      "B",
				"token":         "C",
				"csrf_token":    "D",
				"signature":     "E",
				"presigned_url": "F",
				"share_url":     "G",
				"safe":          "ok",
			},
			want: map[string]any{
				"safe": "ok",
			},
		},
		{
			name: "nested map is sanitised recursively",
			input: map[string]any{
				"outer": map[string]any{
					"password": "X",
					"ok":       "yes",
				},
			},
			want: map[string]any{
				"outer": map[string]any{
					"ok": "yes",
				},
			},
		},
		{
			name: "plain map of safe keys passes through unchanged",
			input: map[string]any{
				"action":        "bucket.create",
				"bucket":        "my-bucket",
				"versioning":    true,
				"deleted_count": 42,
			},
			want: map[string]any{
				"action":        "bucket.create",
				"bucket":        "my-bucket",
				"versioning":    true,
				"deleted_count": 42,
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := audit.Sanitize(tc.input)
			require.Equal(t, tc.want, got)
		})
	}
}
