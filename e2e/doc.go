// Package e2e is the opt-in live suite against a Papierkram sandbox
// account (spec 11.1). The tests carry the build tag e2e and skip unless
// PAPIERKRAM_E2E_SUBDOMAIN, PAPIERKRAM_E2E_TOKEN and
// PAPIERKRAM_E2E_SANDBOX_CONFIRM (equal to the subdomain) are set; see
// README.md. The safety logic lives in internal/e2eguard.
package e2e
