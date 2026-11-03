package config

import (
	"fmt"
	"strings"
)

// ReadOnlyEnvVar is the hard read-only switch (spec 8.3).
const ReadOnlyEnvVar = "PAPIERKRAM_READ_ONLY"

// readOnlyValue classifies a PAPIERKRAM_READ_ONLY value, trimmed and
// case-insensitive: "", "0", "false", "no" and "off" disable the mode, every
// other value enables it (fail-closed). known is false for enabling values
// other than 1/true/yes/on.
func readOnlyValue(v string) (on, known bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "0", "false", "no", "off":
		return false, true
	case "1", "true", "yes", "on":
		return true, true
	}
	return true, false
}

// EnvReadOnly reports whether the hard read-only mode is enabled. It is the
// single predicate for PAPIERKRAM_READ_ONLY; never compare the value directly.
func EnvReadOnly(getenv func(string) string) bool {
	if getenv == nil {
		return false
	}
	on, _ := readOnlyValue(getenv(ReadOnlyEnvVar))
	return on
}

// EnvReadOnlyWarning returns a German warning (without "Warnung: " prefix)
// when PAPIERKRAM_READ_ONLY holds a value that is not clearly on or off, and
// "" otherwise. Such values enable the mode.
func EnvReadOnlyWarning(getenv func(string) string) string {
	if getenv == nil {
		return ""
	}
	v := getenv(ReadOnlyEnvVar)
	if _, known := readOnlyValue(v); known {
		return ""
	}
	return fmt.Sprintf("%s hat den unbekannten Wert %q; der Schreibschutz ist vorsichtshalber aktiv (ausschalten nur mit leerem Wert, 0, false, no oder off)", ReadOnlyEnvVar, v)
}
