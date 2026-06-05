package users

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestUserResourceGolden_WithAttachedPolicy verifies that UserResource
// serialises attached_policies as a non-null JSON array (not "null") when
// the user has one custom policy attached, and that the field appears next
// to attached_templates in the output.
func TestUserResourceGolden_WithAttachedPolicy(t *testing.T) {
	res := UserResource{User{
		AccessKey:         "alice",
		Status:            "enabled",
		AttachedTemplates: []TemplateRef{{Name: "read-only"}},
		AttachedPolicies:  []string{"proj-a"},
		OtherPolicies:     nil,
	}}

	raw, err := json.Marshal(res)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))

	// attached_policies must be a JSON array, not null.
	ap, ok := m["attached_policies"]
	require.True(t, ok, "attached_policies missing from UserResource JSON")
	apSlice, ok := ap.([]any)
	require.True(t, ok, "attached_policies must be a JSON array")
	require.Len(t, apSlice, 1)
	require.Equal(t, "proj-a", apSlice[0])

	// attached_templates must also be present.
	at, ok := m["attached_templates"]
	require.True(t, ok, "attached_templates missing from UserResource JSON")
	atSlice, ok := at.([]any)
	require.True(t, ok, "attached_templates must be a JSON array")
	require.Len(t, atSlice, 1)

	// other_policies must be [] not null.
	op, ok := m["other_policies"]
	require.True(t, ok, "other_policies missing from UserResource JSON")
	opSlice, ok := op.([]any)
	require.True(t, ok, "other_policies must be a JSON array")
	require.Empty(t, opSlice)
}

// TestUserResourceGolden_EmptyAttachedPolicies verifies that a user with no
// custom policies emits "attached_policies":[] (not null) matching how
// other_policies behaves when nil.
func TestUserResourceGolden_EmptyAttachedPolicies(t *testing.T) {
	res := UserResource{User{
		AccessKey:        "bob",
		Status:           "enabled",
		AttachedPolicies: nil, // nil slice → must still be []
	}}

	raw, err := json.Marshal(res)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))

	ap, ok := m["attached_policies"]
	require.True(t, ok, "attached_policies missing from UserResource JSON")
	apSlice, ok := ap.([]any)
	require.True(t, ok, "attached_policies must be a JSON array (not null)")
	require.Empty(t, apSlice, "nil AttachedPolicies must serialise as []")
}

// TestCreatedUserResourceGolden_WithAttachedPolicy verifies that the
// one-time-secret variant (CreatedUserResource) also emits attached_policies.
func TestCreatedUserResourceGolden_WithAttachedPolicy(t *testing.T) {
	res := CreatedUserResource{
		User: User{
			AccessKey:        "carol",
			Status:           "enabled",
			AttachedPolicies: []string{"proj-a"},
		},
		SecretKey: "s3cr3t",
	}

	raw, err := json.Marshal(res)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))

	ap, ok := m["attached_policies"]
	require.True(t, ok, "attached_policies missing from CreatedUserResource JSON")
	apSlice, ok := ap.([]any)
	require.True(t, ok, "attached_policies must be a JSON array")
	require.Len(t, apSlice, 1)
	require.Equal(t, "proj-a", apSlice[0])
}

// TestCreatedUserResourceGolden_EmptyAttachedPolicies verifies that
// CreatedUserResource with nil AttachedPolicies emits [] not null.
func TestCreatedUserResourceGolden_EmptyAttachedPolicies(t *testing.T) {
	res := CreatedUserResource{
		User:      User{AccessKey: "dave", Status: "enabled"},
		SecretKey: "s3cr3t",
	}

	raw, err := json.Marshal(res)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))

	ap, ok := m["attached_policies"]
	require.True(t, ok, "attached_policies missing from CreatedUserResource JSON")
	apSlice, ok := ap.([]any)
	require.True(t, ok, "attached_policies must be a JSON array (not null)")
	require.Empty(t, apSlice, "nil AttachedPolicies must serialise as []")
}
