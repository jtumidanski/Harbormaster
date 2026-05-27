package users

import "testing"

// TestGenerateSecretLength40 — the contract is 40 chars, base62.
func TestGenerateSecretLength40(t *testing.T) {
	s, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}
	if len(s) != 40 {
		t.Errorf("len(secret) = %d, want 40", len(s))
	}
	for i, c := range s {
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		default:
			t.Errorf("byte %d: %q is not in [a-zA-Z0-9]", i, c)
		}
	}
}

// TestGenerateSecretUniqueOver10Calls — base62^40 collisions are
// astronomically unlikely, so 10 calls should yield 10 distinct values.
// (If this ever fails, something is very wrong with crypto/rand.)
func TestGenerateSecretUniqueOver10Calls(t *testing.T) {
	seen := map[string]struct{}{}
	for i := 0; i < 10; i++ {
		s, err := GenerateSecret()
		if err != nil {
			t.Fatalf("GenerateSecret[%d]: %v", i, err)
		}
		if _, dup := seen[s]; dup {
			t.Errorf("duplicate secret on iteration %d: %q", i, s)
		}
		seen[s] = struct{}{}
	}
}
