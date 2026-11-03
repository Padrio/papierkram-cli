package auth

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/zalando/go-keyring"
)

// service is the keyring service name; the account is the context name.
const service = "papierkram-cli"

type osKeyring struct{}

// Keyring returns a Store backed by the OS keyring (macOS Keychain, Windows
// Credential Manager, Secret Service on Linux/BSD).
func Keyring() Store { return osKeyring{} }

func (osKeyring) Get(context string) (string, error) {
	v, err := keyring.Get(service, context)
	if err != nil {
		return "", classify(err)
	}
	return v, nil
}

func (osKeyring) Set(context, token string) error {
	return classify(keyring.Set(service, context, token))
}

func (osKeyring) Delete(context string) error {
	return classify(keyring.Delete(service, context))
}

// unavailableHints are lower-case substrings of go-keyring/godbus error
// messages that mean "no usable keyring" rather than a failed operation.
// go-keyring exposes no typed error for this, so we match heuristically:
//   - "unsupported platform": fallback provider (e.g. non-cgo BSD builds)
//   - "session bus" / "dbus-launch" / "dbus_session_bus_address": no D-Bus
//     session (headless Linux, CI, containers); bare "dbus" is not matched
//     because other godbus errors (protocol/message errors) are real failures
//   - "org.freedesktop.secrets" / "secret service": no Secret Service provider
//     (e.g. "name org.freedesktop.secrets was not provided by any .service files")
//   - "serviceunknown": bus reachable, service missing
var unavailableHints = []string{
	"unsupported platform",
	"session bus",
	"dbus_session_bus_address",
	"dbus-launch",
	"org.freedesktop.secrets",
	"secret service",
	"serviceunknown",
}

// classify maps go-keyring errors onto this package's sentinels. Other errors
// are wrapped unchanged; go-keyring errors never contain the secret.
func classify(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, keyring.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, keyring.ErrUnsupportedPlatform), isUnavailable(err):
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	default:
		return fmt.Errorf("schlüsselbund-fehler: %w", err)
	}
}

func isUnavailable(err error) bool {
	// A failed dial to the (unix) session bus socket.
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Op == "dial" {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, h := range unavailableHints {
		if strings.Contains(msg, h) {
			return true
		}
	}
	return false
}
