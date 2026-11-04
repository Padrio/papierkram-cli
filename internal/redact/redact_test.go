package redact

import (
	"errors"
	"net/url"
	"strings"
	"testing"
)

const (
	withCreds = "http://alice:s3cret@127.0.0.1:1/api/v1"
	badPort   = "http://alice:s3cret@127.0.0.1:badport/api/v1"
	badEscape = "http://al%ice:s3c%zzret@127.0.0.1:1/api/v1"
)

func clean(t *testing.T, what, got string) {
	t.Helper()
	if strings.Contains(got, "alice") || strings.Contains(got, "al%ice") || strings.Contains(got, "s3cret") || strings.Contains(got, "s3c%zzret") {
		t.Errorf("%s = %q shows credentials", what, got)
	}
}

func TestURL(t *testing.T) {
	for raw, want := range map[string]string{
		withCreds:                            "http://127.0.0.1:1/api/v1",
		"https://firma.papierkram.de/api/v1": "https://firma.papierkram.de/api/v1",
		"http://alice@h/x?q=1":               "http://h/x?q=1",
		"":                                   "",
		badPort:                              Invalid,
		badEscape:                            Invalid,
	} {
		got := URL(raw)
		clean(t, "URL("+raw+")", got)
		if got != want {
			t.Errorf("URL(%q) = %q, want %q", raw, got, want)
		}
	}
	u, _ := url.Parse(withCreds)
	if got := URLOf(u); got != "http://127.0.0.1:1/api/v1" || u.User == nil {
		t.Errorf("URLOf = %q (input changed: %v)", got, u.User == nil)
	}
	if URLOf(nil) != Invalid {
		t.Error("URLOf(nil)")
	}
}

// A URL without "//" has no host: url.Parse keeps "user:password@host"
// in Opaque (or the path), not in User. Everything up to the last "@"
// before the host part is masked (N4).
func TestURLWithoutAuthority(t *testing.T) {
	for raw, want := range map[string]string{
		"http:alice:s3cret@firma.example/api/v1":  "http:***@firma.example/api/v1",
		"http:alice@firma.example/api/v1?x=1":     "http:***@firma.example/api/v1?x=1",
		"http:/alice:s3cret@firma.example/api/v1": "http:/***@firma.example/api/v1",
		"http:alice:s3c@x@firma.example/a@b":      "http:***@firma.example/a@b",
		"http:firma.example/api/v1":               "http:firma.example/api/v1",
		"https://firma.example/a@b":               "https://firma.example/a@b",
	} {
		got := URL(raw)
		clean(t, "URL("+raw+")", got)
		if got != want {
			t.Errorf("URL(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestHost(t *testing.T) {
	for raw, want := range map[string]string{
		withCreds:                            "127.0.0.1:1",
		"https://firma.papierkram.de/api/v1": "firma.papierkram.de",
		badPort:                              Invalid,
		badEscape:                            Invalid,
		"kein-url":                           Invalid,
	} {
		got := Host(raw)
		clean(t, "Host("+raw+")", got)
		if got != want {
			t.Errorf("Host(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestError(t *testing.T) {
	perr := parseErr(t)
	for _, err := range []error{
		perr,
		&url.Error{Op: "Post", URL: withCreds + "/income/invoices", Err: errString("connection refused")},
	} {
		got := Error(err)
		clean(t, "Error", got)
		if got == "" {
			t.Error("empty")
		}
	}
	if got := Error(&url.Error{Op: "Post", URL: withCreds, Err: errString("refused")}); got != `Post "http://127.0.0.1:1/api/v1": refused` {
		t.Errorf("Error = %q", got)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestWrapKeepsChain(t *testing.T) {
	perr := parseErr(t)
	w := Wrap(perr)
	clean(t, "Wrap", w.Error())
	var ue *url.Error
	if !errors.As(w, &ue) || Wrap(nil) != nil {
		t.Errorf("chain lost: %v", w)
	}
}

// parseErr is the error of url.Parse for badPort. The URL is joined at
// run time: staticcheck (SA1007) folds constants and would flag the
// literal, which is invalid on purpose.
func parseErr(t *testing.T) error {
	t.Helper()
	raw := strings.Join([]string{"http://alice:s3cret@127.0.0.1", "badport/api/v1"}, ":")
	if raw != badPort {
		t.Fatalf("raw = %q", raw)
	}
	_, err := url.Parse(raw)
	if err == nil {
		t.Fatal("badPort parses")
	}
	return err
}
