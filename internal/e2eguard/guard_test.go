package e2eguard

import (
	"strings"
	"testing"
)

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// Spec 11.1: the live suite runs only with all three variables set and
// PAPIERKRAM_E2E_SANDBOX_CONFIRM equal to PAPIERKRAM_E2E_SUBDOMAIN.
func TestDecide(t *testing.T) {
	full := map[string]string{
		EnvSubdomain: "example-sandbox",
		EnvToken:     "test-token-dummy",
		EnvConfirm:   "example-sandbox",
	}
	without := func(k string) map[string]string {
		m := map[string]string{}
		for kk, v := range full {
			if kk != k {
				m[kk] = v
			}
		}
		return m
	}
	with := func(k, v string) map[string]string {
		m := without(k)
		m[k] = v
		return m
	}
	tests := []struct {
		name    string
		env     map[string]string
		run     bool
		mention []string
	}{
		{"all set and equal", full, true, nil},
		{"nothing set", map[string]string{}, false, []string{EnvSubdomain, EnvToken, EnvConfirm}},
		{"subdomain missing", without(EnvSubdomain), false, []string{EnvSubdomain}},
		{"token missing", without(EnvToken), false, []string{EnvToken}},
		{"confirm missing", without(EnvConfirm), false, []string{EnvConfirm}},
		{"empty token counts as missing", with(EnvToken, ""), false, []string{EnvToken}},
		{"confirm differs", with(EnvConfirm, "other-sandbox"), false, []string{EnvConfirm, EnvSubdomain}},
		{"confirm differs in case", with(EnvConfirm, "Example-Sandbox"), false, []string{EnvConfirm}},
		{"confirm with blank", with(EnvConfirm, "example-sandbox "), false, []string{EnvConfirm}},
		{"confirm equal to token, not subdomain", with(EnvConfirm, "test-token-dummy"), false, []string{EnvConfirm}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			run, reason := Decide(envOf(tc.env))
			if run != tc.run {
				t.Fatalf("Decide = %v (%q), want %v", run, reason, tc.run)
			}
			if run {
				if reason != "" {
					t.Errorf("reason %q for a run", reason)
				}
				return
			}
			for _, m := range tc.mention {
				if !strings.Contains(reason, m) {
					t.Errorf("reason %q does not name %s", reason, m)
				}
			}
			// the reason is printed by every skipped test: never a value
			for _, v := range tc.env {
				if v != "" && strings.Contains(reason, v) {
					t.Errorf("reason %q contains the value %q", reason, v)
				}
			}
		})
	}
}

// Decide reads nothing but the three variables (no network, no files).
func TestDecideReadsOnlyItsVariables(t *testing.T) {
	var read []string
	Decide(func(k string) string { read = append(read, k); return "" })
	for _, k := range read {
		if k != EnvSubdomain && k != EnvToken && k != EnvConfirm {
			t.Errorf("Decide read %s", k)
		}
	}
}

// Only a run without -run, -skip and -list is a full run (the coverage
// gate of TestMain applies only then).
func TestFullRun(t *testing.T) {
	tests := []struct {
		name  string
		flags map[string]string
		want  bool
	}{
		{"no flag", map[string]string{}, true},
		{"flags empty", map[string]string{"test.run": "", "test.skip": "", "test.list": ""}, true},
		{"run", map[string]string{"test.run": "TestE2E_TimeFlow"}, false},
		{"skip", map[string]string{"test.skip": "TestE2E_Carries"}, false},
		{"list", map[string]string{"test.list": "."}, false},
		{"other flag", map[string]string{"test.v": "true"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := FullRun(envOf(tc.flags)); got != tc.want {
				t.Errorf("FullRun(%v) = %v, want %v", tc.flags, got, tc.want)
			}
		})
	}
}

// A CLI run hit the quota when it ended with exit 6 or any of its
// requests was answered with 429 (a CLI that maps it differently, or a
// 429 on an earlier step of a multi-step command).
func TestQuotaHit(t *testing.T) {
	tests := []struct {
		name   string
		code   int
		status []int
		want   bool
	}{
		{"success", 0, []int{200}, false},
		{"no request", 2, nil, false},
		{"exit 6", 6, []int{429}, true},
		{"exit 6 without exchange", 6, nil, true},
		{"429 on an earlier step", 11, []int{201, 429}, true},
		{"other error", 5, []int{422}, false},
		{"unknown outcome", 12, nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			xs := make([]Exchange, 0, len(tc.status))
			for _, s := range tc.status {
				xs = append(xs, Exchange{Method: "GET", Path: "/api/v1/info", Status: s})
			}
			if got := QuotaHit(tc.code, xs); got != tc.want {
				t.Errorf("QuotaHit(%d, %v) = %v, want %v", tc.code, tc.status, got, tc.want)
			}
		})
	}
}
