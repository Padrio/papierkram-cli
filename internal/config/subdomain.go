package config

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Padrio/papierkram-cli/internal/clierr"
)

var subdomainRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// NormalizeSubdomain trims and lowercases a Papierkram subdomain and checks
// that it is a single DNS label, so that https://<subdomain>.papierkram.de
// cannot point to another host. source names where the value came from
// (e.g. "--subdomain", "PAPIERKRAM_SUBDOMAIN") for the error message.
func NormalizeSubdomain(value, source string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(value))
	if subdomainRe.MatchString(s) {
		return s, nil
	}
	hint := "Erlaubt sind Buchstaben, Ziffern und Bindestriche (nicht am Anfang oder Ende), höchstens 63 Zeichen, z. B. firma"
	switch {
	case strings.ContainsAny(s, "./:"):
		hint = "Nur die Subdomain angeben, z. B. firma (für https://firma.papierkram.de)"
	case len(s) > 63 && subdomainRe.MatchString(s[:63]):
		hint = "Die Subdomain ist länger als 63 Zeichen"
	}
	return "", clierr.New(clierr.CodeUsage, fmt.Sprintf("ungültige Subdomain %q (%s)", value, source), hint)
}

// subdomainSuffix is the host suffix of every Papierkram account.
const subdomainSuffix = ".papierkram.de"

// ParseSubdomain is the lenient form of NormalizeSubdomain for flags: it
// also takes the host (firma.papierkram.de) or the URL of the account
// (https://firma.papierkram.de/login) and reduces it to the subdomain.
// normalized is true when a scheme, path, query, fragment, trailing dot or
// the suffix .papierkram.de was removed (not for trimming or lowercasing
// alone). Ports, user info, other schemes and other domains are rejected;
// the error message always names the original value.
func ParseSubdomain(value, source string) (sub string, normalized bool, err error) {
	invalid := func(hint string) error {
		return clierr.New(clierr.CodeUsage, fmt.Sprintf("ungültige Subdomain %q (%s)", value, source), hint)
	}
	// strict validates rest, but names the original value in the error.
	strict := func(rest string) (string, error) {
		s, err := NormalizeSubdomain(rest, source)
		if err != nil {
			return "", invalid(clierr.From(err).Hint)
		}
		return s, nil
	}
	rest := strings.TrimSpace(value)
	if i := strings.Index(rest, "://"); i >= 0 {
		if scheme := strings.ToLower(rest[:i]); scheme != "http" && scheme != "https" {
			_, err := strict(rest)
			return "", false, err
		}
		rest, normalized = rest[i+len("://"):], true
	}
	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		rest, normalized = rest[:i], true
	}
	if strings.Contains(rest, "@") {
		_, err := strict(rest)
		return "", false, err
	}
	if strings.Contains(rest, ":") {
		return "", false, invalid("Ohne Port angeben, z. B. firma (für https://firma.papierkram.de)")
	}
	if r, ok := strings.CutSuffix(rest, "."); ok {
		rest, normalized = r, true
	}
	if n := len(rest) - len(subdomainSuffix); n >= 0 && strings.EqualFold(rest[n:], subdomainSuffix) {
		rest, normalized = rest[:n], true
	}
	sub, err = strict(rest)
	if err != nil {
		return "", false, err
	}
	return sub, normalized, nil
}
