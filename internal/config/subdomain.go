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
