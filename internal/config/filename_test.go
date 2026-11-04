package config

import (
	"strings"
	"testing"
)

func TestFileName(t *testing.T) {
	for ctx, want := range map[string]string{
		"firma-gmbh": "firma-gmbh",
		"a_1":        "a_1",
		// path separators, case and other characters are escaped: a
		// context never names a file outside the directory, and names that
		// differ only in case never share a file (case-insensitive file
		// systems)
		"Firma":    "%46irma",
		"a/b":      "a%2Fb",
		`a\..\b`:   "a%5C%2E%2E%5Cb",
		"müller":   "m%C3%BCller",
		"a%2Fb":    "a%252%46b",
		"":         DirectFileName,
		"C:x":      "%43%3Ax",
		"x*?<>|\"": "x%2A%3F%3C%3E%7C%22",
	} {
		if got := FileName(ctx); got != want {
			t.Errorf("FileName(%q) = %q, want %q", ctx, got, want)
		}
	}
	// the direct mode name cannot belong to a context
	if ValidateName(DirectFileName) == nil {
		t.Errorf("DirectFileName %q is a valid context name", DirectFileName)
	}
	// very long names stay short and distinct
	long1, long2 := strings.Repeat("a", 300)+"1", strings.Repeat("a", 300)+"2"
	n1, n2 := FileName(long1), FileName(long2)
	if len(n1) > 64 || n1 == n2 || !strings.HasPrefix(n1, "~") {
		t.Errorf("long names: %q, %q", n1, n2)
	}
}
