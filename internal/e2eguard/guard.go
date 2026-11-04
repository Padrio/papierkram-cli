package e2eguard

import "strings"

// The variables that unlock the live suite (spec 11.1).
const (
	EnvSubdomain = "PAPIERKRAM_E2E_SUBDOMAIN"
	EnvToken     = "PAPIERKRAM_E2E_TOKEN"
	EnvConfirm   = "PAPIERKRAM_E2E_SANDBOX_CONFIRM"
)

// Decide reports whether the live suite may run: only when
// PAPIERKRAM_E2E_SUBDOMAIN, PAPIERKRAM_E2E_TOKEN and
// PAPIERKRAM_E2E_SANDBOX_CONFIRM are all set (non-empty) and the
// confirmation equals the subdomain exactly (spec 11.1: protection against
// mixing up the sandbox with the real bookkeeping). Otherwise reason says
// why the suite is skipped; it names variables, never their values.
// Decide reads only these three variables and does nothing else.
func Decide(getenv func(string) string) (run bool, reason string) {
	sub, tok, confirm := getenv(EnvSubdomain), getenv(EnvToken), getenv(EnvConfirm)
	var missing []string
	for _, v := range []struct{ name, value string }{
		{EnvSubdomain, sub}, {EnvToken, tok}, {EnvConfirm, confirm},
	} {
		if v.value == "" {
			missing = append(missing, v.name)
		}
	}
	switch {
	case len(missing) > 0:
		return false, "Live-E2E übersprungen: " + strings.Join(missing, ", ") + " nicht gesetzt (Spezifikation 11.1; nur gegen einen Sandbox-Account)"
	case confirm != sub:
		return false, "Live-E2E übersprungen: " + EnvConfirm + " stimmt nicht genau mit " + EnvSubdomain + " überein (Schutz vor Verwechslung mit der echten Buchhaltung)"
	}
	return true, ""
}
