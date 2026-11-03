//go:build !keychain

// These tests replace go-keyring's process-wide provider with a mock and
// cannot restore the real one, so they must never share a binary with the
// keychain integration test.

package auth

import (
	"errors"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestKeyringStore_Mock(t *testing.T) {
	keyring.MockInit()
	s := Keyring()

	if _, err := s.Get("ctx"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if err := s.Delete("ctx"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete missing: want ErrNotFound, got %v", err)
	}
	if err := s.Set("ctx", "test-token-abc"); err != nil {
		t.Fatal(err)
	}
	// Service and account mapping per spec: service papierkram-cli, account = context.
	if v, err := keyring.Get("papierkram-cli", "ctx"); err != nil || v != "test-token-abc" {
		t.Fatalf("raw keyring Get = %q, %v", v, err)
	}
	if v, err := s.Get("ctx"); err != nil || v != "test-token-abc" {
		t.Fatalf("Get = %q, %v", v, err)
	}
	if err := s.Delete("ctx"); err != nil {
		t.Fatal(err)
	}
}

func TestKeyringStore_UnavailableAndNoTokenInErrors(t *testing.T) {
	keyring.MockInitWithError(errors.New("The name org.freedesktop.secrets was not provided by any .service files"))
	t.Cleanup(keyring.MockInit)
	s := Keyring()

	const token = "test-token-secret"
	errs := []error{
		s.Set("ctx", token),
		func() error { _, err := s.Get("ctx"); return err }(),
		s.Delete("ctx"),
	}
	for i, err := range errs {
		if !errors.Is(err, ErrUnavailable) {
			t.Errorf("op %d: want ErrUnavailable, got %v", i, err)
		}
		if err != nil && strings.Contains(err.Error(), token) {
			t.Errorf("op %d: error leaks token", i)
		}
	}
}
