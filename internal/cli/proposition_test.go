package cli

import (
	"net/url"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/testutil/golden"
)

// proposition commands (spec 7.1, 15 no. 52–58).

// TestPropositionCommandsGolden renders list and describe of proposition
// from the spec examples of the mock in every output format.
func TestPropositionCommandsGolden(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		path  string
		query url.Values
	}{
		{"proposition-list", []string{"proposition", "list"}, "/income/propositions", url.Values{"page_size": {"25"}}},
		{"proposition-describe", []string{"proposition", "describe", "386"}, "/income/propositions/386", url.Values{}},
	}
	for _, tc := range cases {
		for _, format := range []string{"table", "wide", "json", "yaml"} {
			t.Run(tc.name+"/"+format, func(t *testing.T) {
				app, srv := apiApp(t)
				wantExit(t, app, Run(app, append(tc.args, "-o", format)), 0)
				r := wantRequests(t, srv, 1)[0]
				if r.Method != "GET" || r.Path != tc.path || r.Query.Encode() != tc.query.Encode() {
					t.Errorf("request = %s %s?%s", r.Method, r.Path, r.Query.Encode())
				}
				out, errs := bufs(app)
				if errs != "" {
					t.Errorf("stderr = %q", errs)
				}
				golden.Assert(t, tc.name+"."+format, []byte(out))
				assertNoToken(t, app)
			})
		}
	}
}

func TestPropositionAliases(t *testing.T) {
	for _, args := range [][]string{
		{"artikel", "list", "-o", "json"},
		{"leistung", "describe", "386", "-o", "json"},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, args), 0)
		wantRequests(t, srv, 1)
	}
}

// Spec R10 at the CLI level (TestR10_PropositionPriceVatAsString covers
// the coercion): price and vat_rate of a proposition are strings in the
// request schema; the flags send them as strings, the VAT normalized, and
// the mock validates the body against the spec (auto-fail). Numbers from
// --input become strings too.
func TestPropositionCreateSendsPriceAndVatAsStrings(t *testing.T) {
	const required = `"name":"Beratung","article_no":"AN-00010","time_unit":"hour","proposition_type":"service"`
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"proposition", "create", "--name", "Beratung", "--article-no", "AN-00010",
		"--time-unit", "hour", "--proposition-type", "service", "--vat-rate", "19", "--price", "12.5", "-o", "json"}), 0)
	raw := string(wantRequests(t, srv, 1)[0].Body)
	for _, want := range []string{`"vat_rate":"0.19"`, `"price":"12.5"`} {
		if !strings.Contains(raw, want) {
			t.Errorf("body ohne %s: %s", want, raw)
		}
	}
	wantBody(t, srv, "POST", "/income/propositions", `{`+required+`,"vat_rate":"0.19","price":"12.5"}`)
	if out, errs := bufs(app); !strings.Contains(out, `"type": "proposition"`) || errs != "" {
		t.Errorf("stdout = %s, stderr = %q", out, errs)
	}

	app, srv = apiApp(t)
	app.In = strings.NewReader(`{` + required + `,"vat_rate":19,"price":12.5}`)
	wantExit(t, app, Run(app, []string{"proposition", "create", "--input", "-", "-o", "json"}), 0)
	wantBody(t, srv, "POST", "/income/propositions", `{`+required+`,"vat_rate":"0.19","price":"12.5"}`)
}

func TestPropositionCreate(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"proposition", "create", "--name", "Schulung vor Ort", "--article-no", "AN-00011",
		"--proposition-type", "service", "--time-unit", "day", "--price", "950", "--vat-rate", "7%",
		"--description", "Ganztägig, inkl. Unterlagen", "--unit-name-1", "Tag", "--unit-name-n", "Tage", "-o", "json"}), 0)
	wantBody(t, srv, "POST", "/income/propositions", `{"name":"Schulung vor Ort","article_no":"AN-00011","proposition_type":"service",`+
		`"time_unit":"day","price":"950","vat_rate":"0.07","description":"Ganztägig, inkl. Unterlagen","unit_name_1":"Tag","unit_name_n":"Tage"}`)

	// required fields are checked before any request (spec 6.4)
	app, srv = apiApp(t)
	wantExit(t, app, Run(app, []string{"proposition", "create", "--name", "Beratung", "--vat-rate", "19"}), 5)
	wantRequests(t, srv, 0)
	if _, errs := bufs(app); !strings.Contains(errs, "article_no") {
		t.Errorf("stderr = %q", errs)
	}
}

func TestPropositionUpdateSendsOnlyChangedFields(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"proposition", "update", "386", "--price", "135.50", "--vat-rate", "7", "-o", "json"}), 0)
	wantBody(t, srv, "PUT", "/income/propositions/386", `{"price":"135.50","vat_rate":"0.07"}`)

	// nothing to change is exit 2; vat 1 is ambiguous (1 % or 100 %?), exit 5
	for _, tc := range []struct {
		args []string
		exit int
	}{
		{[]string{"proposition", "update", "386"}, 2},
		{[]string{"proposition", "update", "386", "--vat-rate", "1"}, 5},
		{[]string{"proposition", "update", "386", "--vat-rate", "1", "--dry-run"}, 5},
		{[]string{"proposition", "update", "386", "--proposition-type", "dienstleistung"}, 5},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, tc.args), tc.exit)
		wantRequests(t, srv, 0)
	}
}

func TestPropositionDeleteNeedsConfirmation(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"proposition", "delete", "386"}), 8)
	wantRequests(t, srv, 0)
	if _, errs := bufs(app); !strings.Contains(errs, "--yes") {
		t.Errorf("stderr = %q", errs)
	}

	app, srv = apiApp(t)
	wantExit(t, app, Run(app, []string{"proposition", "delete", "386", "--yes", "-o", "json"}), 0)
	if r := wantRequests(t, srv, 1)[0]; r.Method != "DELETE" || r.Path != "/income/propositions/386" {
		t.Errorf("request = %s %s", r.Method, r.Path)
	}
	if out := mustOut(app); out != "{\n  \"id\": 386,\n  \"deleted\": true\n}\n" {
		t.Errorf("stdout = %q", out)
	}
}

// archive is not destructive (spec 8.1): no confirmation, also without TTY.
func TestPropositionArchiveWithoutConfirmation(t *testing.T) {
	for _, action := range []string{"archive", "unarchive"} {
		app, srv := apiApp(t) // no TTY, no --yes
		wantExit(t, app, Run(app, []string{"proposition", action, "386", "-o", "json"}), 0)
		if r := wantRequests(t, srv, 1)[0]; r.Method != "POST" || r.Path != "/income/propositions/386/"+action {
			t.Errorf("request = %s %s", r.Method, r.Path)
		}
		if out := mustOut(app); !strings.Contains(out, `"type": "proposition"`) {
			t.Errorf("%s: stdout = %s", action, out)
		}
	}
}

// The registered mutations run the protection layer (Task 24).
func TestPropositionMutationsDryRun(t *testing.T) {
	for _, tc := range []struct {
		args          []string
		method, path  string
		credits, body string
	}{
		{[]string{"proposition", "create", "--name", "P", "--article-no", "AN-1", "--time-unit", "hour",
			"--proposition-type", "product", "--vat-rate", "19"},
			"POST", "/income/propositions", `"credits": 10`, `"vat_rate": "0.19"`},
		{[]string{"proposition", "update", "386", "--price", "99"},
			"PUT", "/income/propositions/386", `"credits": 1`, `"price": "99"`},
		{[]string{"proposition", "delete", "386"},
			"DELETE", "/income/propositions/386", `"credits": 1`, `"body": null`},
		{[]string{"proposition", "archive", "386"},
			"POST", "/income/propositions/386/archive", `"credits": 1`, `"body": null`},
		{[]string{"proposition", "unarchive", "386"},
			"POST", "/income/propositions/386/unarchive", `"credits": 1`, `"body": null`},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, append(tc.args, "--dry-run", "-o", "json")), 0)
		wantRequests(t, srv, 0)
		out := mustOut(app)
		for _, want := range []string{`"dry_run": true`, `"method": "` + tc.method + `"`, tc.path + `"`, tc.credits, tc.body} {
			if !strings.Contains(out, want) {
				t.Errorf("%v: stdout ohne %q:\n%s", tc.args, want, out)
			}
		}
	}
}

func TestPropositionMutationsReadOnly(t *testing.T) {
	for _, args := range [][]string{
		{"proposition", "create", "--name", "P", "--article-no", "AN-1", "--time-unit", "hour", "--proposition-type", "product", "--vat-rate", "19"},
		{"proposition", "update", "7", "--price", "99"},
		{"proposition", "update", "7", "--price", "99", "--dry-run"},
		{"proposition", "delete", "7", "--yes"},
		{"proposition", "archive", "7"},
		{"proposition", "unarchive", "7"},
	} {
		app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-ro", "PAPIERKRAM_READ_ONLY": "1"})
		wantExit(t, app, Run(app, args), 9)
		wantRequests(t, srv, 0)
	}
	app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-ro", "PAPIERKRAM_READ_ONLY": "1"})
	wantExit(t, app, Run(app, []string{"proposition", "list", "-o", "json"}), 0)
	wantRequests(t, srv, 1)
}

// Spec 6.3a at the CLI level: describe -o json | update --input - sends a
// spec-conformant PUT; the embedded vat_rate object becomes its rate as a
// string, read-only fields are removed and named.
func TestPropositionDescribeUpdateRoundTrip(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"proposition", "describe", "386", "-o", "json"}), 0)
	wantRequests(t, srv, 1)
	described := mustOut(app)

	app, srv = apiApp(t)
	app.In = strings.NewReader(described)
	wantExit(t, app, Run(app, []string{"proposition", "update", "386", "--input", "-", "-o", "json"}), 0)
	wantBody(t, srv, "PUT", "/income/propositions/386", `{"article_no":"AN-00001","description":"Notes for the proposition",`+
		`"name":"My proposition","price":"150.0","proposition_type":"product","time_unit":"hour",`+
		`"unit_name_1":"Produkt","unit_name_n":"Produkte","vat_rate":"0.19"}`)
	if _, errs := bufs(app); !strings.Contains(errs, "nur lesbare Felder entfernt") || !strings.Contains(errs, "record_state") {
		t.Errorf("stderr = %q", errs)
	}
}
