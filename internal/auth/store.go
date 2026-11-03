// Package auth stores API tokens per context, preferably in the OS keyring.
//
// The plain-file fallback (--insecure-storage) is deliberately not part of
// this package; it lives with the config handling.
package auth

import (
	"errors"
	"sync"
)

var (
	// ErrNotFound is returned when no token is stored for a context.
	ErrNotFound = errors.New("kein Token für diesen Kontext gespeichert")
	// ErrUnavailable is returned when the platform keyring cannot be used,
	// e.g. on Linux without a running Secret Service / D-Bus session.
	ErrUnavailable = errors.New("kein Schlüsselbund verfügbar")
)

// Store keeps one token per context name. Implementations never put the
// token into returned errors.
type Store interface {
	Get(context string) (string, error)
	Set(context, token string) error
	Delete(context string) error
}

type memory struct {
	mu     sync.Mutex
	tokens map[string]string
}

// NewMemory returns an in-memory Store, intended for tests.
func NewMemory() Store {
	return &memory{tokens: map[string]string{}}
}

func (m *memory) Get(context string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tokens[context]
	if !ok {
		return "", ErrNotFound
	}
	return t, nil
}

func (m *memory) Set(context, token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tokens[context] = token
	return nil
}

func (m *memory) Delete(context string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.tokens[context]; !ok {
		return ErrNotFound
	}
	delete(m.tokens, context)
	return nil
}
