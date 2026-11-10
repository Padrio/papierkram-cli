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

func TestParseSubdomain(t *testing.T) {
	for _, tt := range []struct {
		in, want   string
		normalized bool
	}{
		{"pkrason", "pkrason", false}, {" PKRASON ", "pkrason", false},
		{"pkrason.papierkram.de", "pkrason", true}, {"PKRASON.Papierkram.DE", "pkrason", true},
		{"https://pkrason.papierkram.de", "pkrason", true}, {"https://pkrason.papierkram.de/", "pkrason", true},
		{"HTTP://pkrason.papierkram.de/login?x=1#a", "pkrason", true}, {"pkrason.papierkram.de.", "pkrason", true},
	} {
		got, normalized, err := ParseSubdomain(tt.in, "--subdomain")
		if err != nil || got != tt.want || normalized != tt.normalized {
			t.Errorf("%q: got %q, %t, %v; want %q, %t", tt.in, got, normalized, err, tt.want, tt.normalized)
		}
	}
}

func TestParseSubdomainRejects(t *testing.T) {
	const portHint = "Ohne Port angeben, z. B. firma (für https://firma.papierkram.de)"
	const domainHint = "Nur die Subdomain angeben, z. B. firma (für https://firma.papierkram.de)"
	// labelHint: what is left after normalizing is empty (NormalizeSubdomain)
	const labelHint = "Erlaubt sind Buchstaben, Ziffern und Bindestriche (nicht am Anfang oder Ende), höchstens 63 Zeichen, z. B. firma"
	for in, hint := range map[string]string{
		"pkrason.papierkram.de:443":           portHint,
		"https://pkrason.papierkram.de:8443/": portHint,
		"firma.example.com":                   domainHint,
		"https://":                            labelHint,
		".papierkram.de":                      labelHint,
		"a.b.papierkram.de":                   domainHint,
		"https://user@pkrason.papierkram.de":  domainHint,
		"ftp://pkrason.papierkram.de":         domainHint,
		"":                                    labelHint,
	} {
		_, _, err := ParseSubdomain(in, "--subdomain")
		wantCode(t, err, clierr.CodeUsage)
		ce := clierr.From(err)
		if want := `ungültige Subdomain "` + in + `" (--subdomain)`; ce.Message != want {
			t.Errorf("%q: message %q, want %q", in, ce.Message, want)
		}
		if ce.Hint != hint {
			t.Errorf("%q: hint %q, want %q", in, ce.Hint, hint)
		}
	}
}
