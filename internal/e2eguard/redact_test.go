package e2eguard

import (
	"strings"
	"testing"
)

func TestRedactorHidesSecrets(t *testing.T) {
	r := NewRedactor("test-token-a/b+c", "", "test-token-2")
	in := "Bearer test-token-a/b+c, q=test-token-a%2Fb%2Bc and test-token-2"
	got := r.String(in)
	for _, s := range []string{"test-token-a/b+c", "test-token-a%2Fb%2Bc", "test-token-2"} {
		if strings.Contains(got, s) {
			t.Errorf("String(%q) = %q still contains %q", in, got, s)
		}
	}
	if !strings.Contains(got, Redacted) {
		t.Errorf("String = %q, want the placeholder %q", got, Redacted)
	}
	if r.Contains(got) {
		t.Errorf("Contains(%q) = true after redaction", got)
	}
	if !r.Contains(in) {
		t.Errorf("Contains(%q) = false", in)
	}
}

// An empty secret is ignored: it would match everywhere.
func TestRedactorIgnoresEmpty(t *testing.T) {
	r := NewRedactor("")
	if got := r.String("abc"); got != "abc" {
		t.Errorf("String = %q", got)
	}
	if r.Contains("abc") {
		t.Error("Contains with only an empty secret")
	}
}

func TestRedactorLongestFirst(t *testing.T) {
	// a secret that contains another must not leave a tail behind
	r := NewRedactor("test-token-x", "test-token-xyz")
	if got := r.String("test-token-xyz"); strings.Contains(got, "yz") {
		t.Errorf("String = %q leaves a tail", got)
	}
}
