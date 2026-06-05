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
