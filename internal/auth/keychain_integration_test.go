//go:build keychain

package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"testing"
)

func randHex(t *testing.T) string {
	t.Helper()
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

// TestKeychain_SetGetDelete talks to the real OS keyring (macOS Keychain,
// Windows Credential Manager, Secret Service on Linux) with a throwaway entry.
func TestKeychain_SetGetDelete(t *testing.T) {
	ctxName := "pk-ci-" + randHex(t)
	token := "test-token-" + randHex(t)
	s := Keyring()
	t.Cleanup(func() { _ = s.Delete(ctxName) })

	if _, err := s.Get(ctxName); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get before Set: want ErrNotFound, got %v", err)
	}
	if err := s.Set(ctxName, token); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := s.Get(ctxName)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != token {
		t.Fatal("Get returned a different token than Set stored")
	}
	if err := s.Delete(ctxName); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(ctxName); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after Delete: want ErrNotFound, got %v", err)
	}
}
