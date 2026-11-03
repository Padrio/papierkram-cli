package auth

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestMemoryStore_CRUD(t *testing.T) {
	s := NewMemory()

	if _, err := s.Get("prod"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get on empty store: want ErrNotFound, got %v", err)
	}
	if err := s.Set("prod", "test-token-1"); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("other", "test-token-2"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get("prod")
	if err != nil || got != "test-token-1" {
		t.Fatalf("Get = %q, %v", got, err)
	}
	if err := s.Set("prod", "test-token-3"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Get("prod"); got != "test-token-3" {
		t.Fatalf("overwrite failed: %q", got)
	}
	if err := s.Delete("prod"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get("prod"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after Delete: want ErrNotFound, got %v", err)
	}
	if got, _ := s.Get("other"); got != "test-token-2" {
		t.Fatalf("Delete touched other context: %q", got)
	}
	if err := s.Delete("prod"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second Delete: want ErrNotFound, got %v", err)
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		name string
		in   error
		want error // sentinel expected via errors.Is; nil = passthrough
	}{
		{"nil", nil, nil},
		{"not found", keyring.ErrNotFound, ErrNotFound},
		{"wrapped not found", fmt.Errorf("x: %w", keyring.ErrNotFound), ErrNotFound},
		{"unsupported platform", keyring.ErrUnsupportedPlatform, ErrUnavailable},
		{"no session bus", errors.New("dbus: couldn't determine address of session bus"), ErrUnavailable},
		{"dbus address unset", errors.New("$DBUS_SESSION_BUS_ADDRESS not set"), ErrUnavailable},
		{"no secrets service", errors.New("The name org.freedesktop.secrets was not provided by any .service files"), ErrUnavailable},
		{"dial error", &net.OpError{Op: "dial", Net: "unix", Err: errors.New("connect: no such file or directory")}, ErrUnavailable},
		{"other", errors.New("boom"), nil},
		{"too big", keyring.ErrSetDataTooBig, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classify(tt.in)
			switch {
			case tt.in == nil:
				if got != nil {
					t.Fatalf("want nil, got %v", got)
				}
			case tt.want != nil:
				if !errors.Is(got, tt.want) {
					t.Fatalf("want %v, got %v", tt.want, got)
				}
			default:
				if errors.Is(got, ErrNotFound) || errors.Is(got, ErrUnavailable) {
					t.Fatalf("unexpected sentinel: %v", got)
				}
				if !errors.Is(got, tt.in) && got.Error() != tt.in.Error() {
					t.Fatalf("passthrough lost cause: %v", got)
				}
			}
		})
	}
}

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
