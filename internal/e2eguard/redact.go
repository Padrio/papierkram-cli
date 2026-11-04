package e2eguard

import (
	"cmp"
	"net/url"
	"slices"
	"strings"
)

// Redacted replaces a secret in redacted text.
const Redacted = "[REDACTED]"

// Redactor removes secrets (the API token) from text before it is logged.
type Redactor struct {
	secrets []string
}

// NewRedactor redacts the given secrets and their URL-escaped forms; empty
// secrets are ignored.
func NewRedactor(secrets ...string) Redactor {
	var r Redactor
	for _, s := range secrets {
		if s == "" {
			continue
		}
		r.secrets = append(r.secrets, s)
		for _, e := range []string{url.QueryEscape(s), url.PathEscape(s)} {
			if e != s {
				r.secrets = append(r.secrets, e)
			}
		}
	}
	// longest first: a secret that contains another leaves no tail
	slices.SortFunc(r.secrets, func(a, b string) int { return cmp.Compare(len(b), len(a)) })
	return r
}

// String is s with every secret replaced by Redacted.
func (r Redactor) String(s string) string {
	for _, sec := range r.secrets {
		s = strings.ReplaceAll(s, sec, Redacted)
	}
	return s
}

// Contains reports whether s contains a secret.
func (r Redactor) Contains(s string) bool {
	for _, sec := range r.secrets {
		if strings.Contains(s, sec) {
			return true
		}
	}
	return false
}
