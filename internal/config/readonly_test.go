package config

import (
	"strings"
	"testing"
)

func TestEnvReadOnlyFailClosed(t *testing.T) {
	tests := []struct {
		value    string
		on, warn bool
	}{
		{"", false, false},
		{"0", false, false},
		{" 0 ", false, false},
		{"false", false, false},
		{"FALSE", false, false},
		{"no", false, false},
		{"off", false, false},
		{"Off", false, false},
		{"1", true, false},
		{"1 ", true, false},
		{" 1", true, false},
		{"true", true, false},
		{"TRUE", true, false},
		{"yes", true, false},
		{"on", true, false},
		{"ja", true, true},
		{"2", true, true},
		{"enabled", true, true},
	}
	for _, tc := range tests {
		t.Run(tc.value, func(t *testing.T) {
			getenv := env(map[string]string{ReadOnlyEnvVar: tc.value})
			if got := EnvReadOnly(getenv); got != tc.on {
				t.Errorf("EnvReadOnly(%q) = %v, want %v", tc.value, got, tc.on)
			}
			w := EnvReadOnlyWarning(getenv)
			if (w != "") != tc.warn {
				t.Errorf("EnvReadOnlyWarning(%q) = %q, want warning %v", tc.value, w, tc.warn)
			}
			if tc.warn && (!strings.Contains(w, ReadOnlyEnvVar) || !strings.Contains(w, tc.value)) {
				t.Errorf("warning does not name variable and value: %q", w)
			}
		})
	}
	if EnvReadOnly(nil) || EnvReadOnlyWarning(nil) != "" {
		t.Error("nil getenv must mean off")
	}
}
