// Package redact keeps credentials of URLs out of everything the CLI
// shows or stores (controller ruling, project-wide): PAPIERKRAM_BASE_URL
// may carry userinfo (user:password@), which a request may use but no
// output, message, debug line or file may contain. A URL that cannot be
// parsed is never shown as it is, since its credentials cannot be cut
// out reliably; it becomes the placeholder Invalid.
package redact

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
)

// Invalid stands for a URL that cannot be parsed.
const Invalid = "(ungültige URL)"

// URL is raw without userinfo; "" stays "", an unparseable raw is Invalid.
func URL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return Invalid
	}
	return URLOf(u)
}

// URLOf is u without userinfo (u is not changed); nil is Invalid. A URL
// without host ("http:user:pw@host/x", "http:/user:pw@host/x") keeps
// "user:pw@host" in Opaque or in the path, not in User: there the first
// part after the scheme is masked up to its last "@" (maskOpaque).
func URLOf(u *url.URL) string {
	if u == nil {
		return Invalid
	}
	c := *u
	c.User = nil
	s := c.String()
	if c.Opaque == "" && c.Host != "" {
		return s
	}
	return maskOpaque(s, c.Scheme)
}

// maskOpaque replaces everything up to and including the last "@" in the
// first part of s after "scheme:" and leading slashes (up to the next
// "/", "?" or "#", where a host would end) with "***@".
func maskOpaque(s, scheme string) string {
	prefix, rest := "", s
	if scheme != "" && len(s) > len(scheme) && s[len(scheme)] == ':' {
		prefix, rest = s[:len(scheme)+1], s[len(scheme)+1:]
	}
	lead := len(rest) - len(strings.TrimLeft(rest, "/"))
	prefix, rest = prefix+rest[:lead], rest[lead:]
	end := strings.IndexAny(rest, "/?#")
	if end < 0 {
		end = len(rest)
	}
	if at := strings.LastIndex(rest[:end], "@"); at >= 0 {
		rest = "***@" + rest[at+1:]
	}
	return prefix + rest
}

// Host is the host (with port) of raw, never its userinfo; Invalid when
// raw cannot be parsed or has no host.
func Host(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return Invalid
	}
	return u.Host
}

// Error is the text of err with the URL of a *url.Error in its chain
// (from url.Parse or an HTTP request) replaced by its redacted form:
// net/http masks only the password, url.Parse shows the URL as given.
func Error(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	var ue *url.Error
	if errors.As(err, &ue) {
		safe := ue.Op + " " + strconv.Quote(URL(ue.URL)) + ": " + ue.Err.Error()
		msg = strings.ReplaceAll(msg, ue.Error(), safe)
	}
	return msg
}

// Wrap is err with the text of Error; errors.Is and errors.As still see
// err.
func Wrap(err error) error {
	if err == nil {
		return nil
	}
	return wrapped{err}
}

type wrapped struct{ err error }

func (w wrapped) Error() string { return Error(w.err) }
func (w wrapped) Unwrap() error { return w.err }
