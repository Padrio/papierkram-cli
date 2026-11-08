package cli

import (
	"strings"
	"testing"
)

// Review C-I2 (ruling FR-D): a flag that cannot be parsed stops the parse
// before later flags are set, so the format of the error comes from the
// raw arguments: -o X, -oX, -o=X, --output X, --output=X and --jq. The
// AGENTS contract (with -o json or --jq the last line of stderr is the
// JSON error) then also holds for --bogus -o json.
func TestFlagErrorFormatFromRawArgs(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string // "json", "yaml" or "text"
	}{
		{"bogus -o json", []string{"user", "list", "--bogus", "-o", "json"}, "json"},
		{"bogus -ojson", []string{"user", "list", "--bogus", "-ojson"}, "json"},
		{"bogus -o=json", []string{"user", "list", "--bogus", "-o=json"}, "json"},
		{"bogus --output json", []string{"user", "list", "--bogus", "--output", "json"}, "json"},
		{"bogus --output=yaml", []string{"user", "list", "--bogus", "--output=yaml"}, "yaml"},
		{"limit abc", []string{"user", "list", "--limit", "abc", "-o", "json"}, "json"},
		{"all=vielleicht", []string{"user", "list", "--all=vielleicht", "--output=json"}, "json"},
		{"bogus --jq", []string{"user", "list", "--bogus", "--jq", "."}, "json"},
		{"bogus --jq=", []string{"user", "list", "--bogus", "--jq=.entries"}, "json"},
		{"last wins", []string{"user", "list", "--bogus", "-o", "json", "-o", "table"}, "text"},
		{"invalid format", []string{"user", "list", "--bogus", "-o", "xml"}, "text"},
		{"after --", []string{"api", "GET", "/users", "--bogus", "--", "-o", "json"}, "text"},
		{"without format", []string{"user", "list", "--bogus"}, "text"},
		// the value of a value flag is no format, as in a full parse
		// (pflag takes the next argument even if it starts with "-")
		{"value of --name", []string{"company", "create", "--name", "-ojson", "--bogus"}, "text"},
		{"value of --context", []string{"user", "list", "--context", "-ojson", "--bogus"}, "text"},
		{"value of -O", []string{"invoice", "pdf", "459", "-O", "-ojson", "--bogus"}, "text"},
		{"value of --name, then -o", []string{"company", "create", "--name", "-ojson", "-o", "yaml", "--bogus"}, "yaml"},
		{"--name=value", []string{"company", "create", "--name=x", "-o", "json", "--bogus"}, "json"},
		{"--limit value", []string{"user", "list", "--limit", "5", "-o", "json", "--bogus"}, "json"},
		{"bool flag", []string{"user", "list", "--all", "-o", "json", "--bogus"}, "json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, out, errb := newTestApp(t)
			if code := Run(app, tc.args); code != 2 {
				t.Fatalf("exit %d: %s", code, errb.String())
			}
			errs := errb.String()
			e, isJSON := lastJSONError(t, errs)
			isYAML := strings.Contains(errs, "\nerror:\n  code: usage\n")
			switch tc.want {
			case "json":
				if !isJSON || e.Code != "usage" || e.Hint == "" {
					t.Errorf("keine JSON-Fehlerzeile: %q", errs)
				}
			case "yaml":
				if !isYAML {
					t.Errorf("kein YAML-Fehler: %q", errs)
				}
			default:
				if isJSON || isYAML {
					t.Errorf("strukturierter Fehler ohne Format: %q", errs)
				}
			}
			if out.Len() != 0 {
				t.Errorf("stdout = %q", out.String())
			}
		})
	}
}
