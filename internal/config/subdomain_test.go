package config

import (
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/clierr"
)

func TestNormalizeSubdomain(t *testing.T) {
	for in, want := range map[string]string{
		"firma":                 "firma",
		"Firma-GmbH":            "firma-gmbh",
		" firma ":               "firma",
		"a":                     "a",
		"1und1":                 "1und1",
		strings.Repeat("a", 63): strings.Repeat("a", 63),
	} {
		got, err := NormalizeSubdomain(in, "--subdomain")
		if err != nil || got != want {
			t.Errorf("%q: got %q, %v; want %q", in, got, err, want)
		}
	}
	domainHint := "Nur die Subdomain angeben, z. B. firma"
	for in, hint := range map[string]string{
		"firma.papierkram.de":         domainHint,
		"https://firma.papierkram.de": domainHint,
		"evil.example#":               domainHint,
		"firma/api":                   domainHint,
		"-firma":                      "Bindestriche",
		"firma-":                      "Bindestriche",
		"fir ma":                      "Bindestriche",
		"firma#":                      "Bindestriche",
		"müller":                      "Bindestriche",
		strings.Repeat("a", 64):       "63 Zeichen",
	} {
		_, err := NormalizeSubdomain(in, "PAPIERKRAM_SUBDOMAIN")
		wantCode(t, err, clierr.CodeUsage)
		ce := clierr.From(err)
		if !strings.Contains(ce.Message, "PAPIERKRAM_SUBDOMAIN") || !strings.Contains(ce.Message, "ungültige Subdomain") {
			t.Errorf("%q: message %q", in, ce.Message)
		}
		if !strings.Contains(ce.Hint, hint) {
			t.Errorf("%q: hint %q lacks %q", in, ce.Hint, hint)
		}
	}
}

func TestResolveValidatesSubdomain(t *testing.T) {
	_, err := Resolve(&Config{}, ResolveInput{Env: env(map[string]string{"PAPIERKRAM_SUBDOMAIN": "evil.example#"})})
	wantCode(t, err, clierr.CodeUsage)
	if !strings.Contains(clierr.From(err).Message, "PAPIERKRAM_SUBDOMAIN") {
		t.Fatalf("source not named: %v", err)
	}
	r, err := Resolve(&Config{}, ResolveInput{Env: env(map[string]string{"PAPIERKRAM_SUBDOMAIN": "ACME "})})
	if err != nil || r.Subdomain != "acme" || r.BaseURL != "https://acme.papierkram.de/api/v1" {
		t.Fatalf("%+v %v", r, err)
	}
	cfg := &Config{ActiveContext: "a", Contexts: map[string]*Context{"a": {Subdomain: "firma.papierkram.de"}}}
	_, err = Resolve(cfg, ResolveInput{Env: env(nil)})
	wantCode(t, err, clierr.CodeUsage)
	if !strings.Contains(clierr.From(err).Message, `Kontext "a"`) {
		t.Fatalf("source not named: %v", err)
	}
}
