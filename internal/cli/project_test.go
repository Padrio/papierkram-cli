package cli

import (
	"net/url"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/registry"
	"github.com/Padrio/papierkram-cli/internal/testutil/golden"
)

// project, task and custom-attribute commands (spec 7.1, 9.8, 15 no.
// 60–80).

// TestProjectTaskCustomAttributeGolden renders list and describe from the
// spec examples of the mock in every output format. The working directory
// has no project file, so no preset applies.
func TestProjectTaskCustomAttributeGolden(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		path  string
		query url.Values
	}{
		{"project-list", []string{"project", "list"}, "/projects", url.Values{"page_size": {"25"}}},
		{"project-describe", []string{"project", "describe", "4727"}, "/projects/4727", url.Values{}},
		{"task-list", []string{"task", "list"}, "/tracker/tasks", url.Values{"page_size": {"25"}}},
		{"task-describe", []string{"task", "describe", "492"}, "/tracker/tasks/492", url.Values{}},
		{"custom-attribute-list", []string{"custom-attribute", "list"}, "/settings/custom_attributes", url.Values{"page_size": {"25"}}},
		{"custom-attribute-describe", []string{"custom-attribute", "describe", "342"}, "/settings/custom_attributes/342", url.Values{}},
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

func TestProjectTaskCustomAttributeAliases(t *testing.T) {
	for _, args := range [][]string{
		{"projekt", "list", "-o", "json"},
		{"aufgabe", "describe", "492", "-o", "json"},
		{"datenfeld", "list", "-o", "json"},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, args), 0)
		wantRequests(t, srv, 1)
	}
}

func TestProjectTaskListFilters(t *testing.T) {
	for _, tc := range []struct {
		args  []string
		path  string
		query url.Values
	}{
		{[]string{"project", "list", "--company", "3283"}, "/projects", url.Values{"company_id": {"3283"}, "page_size": {"25"}}},
		{[]string{"task", "list", "--project", "4823", "--proposition", "0"}, "/tracker/tasks",
			url.Values{"project_id": {"4823"}, "proposition_id": {"0"}, "page_size": {"25"}}},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, append(tc.args, "-o", "json")), 0)
		if r := wantRequests(t, srv, 1)[0]; r.Path != tc.path || r.Query.Encode() != tc.query.Encode() {
			t.Errorf("%v: request = %s?%s", tc.args, r.Path, r.Query.Encode())
		}
	}
}

// Spec R7 at the CLI level (TestR07_BudgetAsString covers the coercion):
// budget_money and budget_time are strings in the request schema; flags
// send them as strings, and so do numbers from --input. The mock validates
// the body against the spec (auto-fail).
func TestProjectCreateSendsBudgetAsString(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"project", "create", "--name", "Website-Relaunch", "--customer", "3283",
		"--budget-money", "5000", "-o", "json"}), 0)
	raw := string(wantRequests(t, srv, 1)[0].Body)
	if !strings.Contains(raw, `"budget_money":"5000"`) {
		t.Errorf("body ohne \"budget_money\":\"5000\": %s", raw)
	}
	wantBody(t, srv, "POST", "/projects", `{"name":"Website-Relaunch","customer":{"id":3283},"budget_money":"5000"}`)
	if out, errs := bufs(app); !strings.Contains(out, `"type": "project"`) || errs != "" {
		t.Errorf("stdout = %s, stderr = %q", out, errs)
	}

	app, srv = apiApp(t)
	app.In = strings.NewReader(`{"name":"Wartung","customer":{"id":3283},"budget_type":"working_time","budget_time":40,"budget_time_unit":"hour","budget_money":5000.5}`)
	wantExit(t, app, Run(app, []string{"project", "create", "--input", "-", "-o", "json"}), 0)
	wantBody(t, srv, "POST", "/projects", `{"name":"Wartung","customer":{"id":3283},"budget_type":"working_time",`+
		`"budget_time":"40","budget_time_unit":"hour","budget_money":"5000.5"}`)
}

// project flagged is a real boolean (unlike task and person, R8/R9);
// --attr values are strings, --team-member repeats.
func TestProjectCreate(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"project", "create", "--name", "Wartung", "--customer", "3283",
		"--start-date", "2026-11-01", "--end-date", "2026-12-31", "--color", "green", "--flagged",
		"--team-member", "2516", "--team-member", "2517", "--default-proposition", "386",
		"--attr", "338=42", "--attr", "339=ja", "--description", "Wartung für Müller & Söhne", "-o", "json"}), 0)
	wantBody(t, srv, "POST", "/projects", `{"name":"Wartung","customer":{"id":3283},"start_date":"2026-11-01",`+
		`"end_date":"2026-12-31","color":"green","flagged":true,"team_members":[{"id":2516},{"id":2517}],`+
		`"default_proposition":{"id":386},"custom_attributes":{"338":"42","339":"ja"},"description":"Wartung für Müller & Söhne"}`)

	// required fields and enums are checked before any request (spec 6.4)
	for _, args := range [][]string{
		{"project", "create", "--name", "Ohne Kunde"},
		{"project", "create", "--name", "P", "--customer", "1", "--color", "rot"},
		{"project", "create", "--name", "P", "--customer", "1", "--budget-type", "geld", "--dry-run"},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, args), 5)
		wantRequests(t, srv, 0)
	}
}

func TestProjectUpdateSendsOnlyChangedFields(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"project", "update", "4710", "--end-date", "2027-03-31", "--budget-time", "120",
		"--attr", "338=7", "-o", "json"}), 0)
	wantBody(t, srv, "PUT", "/projects/4710", `{"end_date":"2027-03-31","budget_time":"120","custom_attributes":{"338":"7"}}`)

	app, srv = apiApp(t)
	wantExit(t, app, Run(app, []string{"project", "update", "4710"}), 2)
	wantRequests(t, srv, 0)
}

// Spec R8 at the CLI level (TestR08_TaskFieldsAsString covers the coercion):
// complete and flagged of a task are strings in the request schema; the
// switches send "true", relative_costs is a string too. Booleans and
// numbers from --input are normalized to strings.
func TestTaskCreateSendsCompleteFlaggedAsStrings(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"task", "create", "--name", "Konzept", "--project", "4823",
		"--complete", "--flagged", "-o", "json"}), 0)
	raw := string(wantRequests(t, srv, 1)[0].Body)
	for _, want := range []string{`"complete":"true"`, `"flagged":"true"`} {
		if !strings.Contains(raw, want) {
			t.Errorf("body ohne %s: %s", want, raw)
		}
	}
	wantBody(t, srv, "POST", "/tracker/tasks", `{"name":"Konzept","project":{"id":4823},"complete":"true","flagged":"true"}`)
	if out, errs := bufs(app); !strings.Contains(out, `"type": "task"`) || errs != "" {
		t.Errorf("stdout = %s, stderr = %q", out, errs)
	}

	app, srv = apiApp(t)
	app.In = strings.NewReader(`{"name":"Konzept","project":{"id":4823},"complete":false,"flagged":true,"relative_costs":12.5}`)
	wantExit(t, app, Run(app, []string{"task", "create", "--input", "-", "-o", "json"}), 0)
	wantBody(t, srv, "POST", "/tracker/tasks", `{"name":"Konzept","project":{"id":4823},"complete":"false","flagged":"true","relative_costs":"12.5"}`)
}

func TestTaskCreate(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"task", "create", "--name", "Support", "--project", "4823", "--proposition", "386",
		"--user", "2516", "--deadline", "2026-12-15", "--relative-costs", "50", "--flagged=false", "-o", "json"}), 0)
	wantBody(t, srv, "POST", "/tracker/tasks", `{"name":"Support","project":{"id":4823},"proposition":{"id":386},`+
		`"user":{"id":2516},"deadline":"2026-12-15","relative_costs":"50","flagged":"false"}`)

	for _, args := range [][]string{
		{"task", "create", "--name", "Ohne Projekt"},                                    // project required, no preset
		{"task", "create", "--name", "T", "--project", "1", "--deadline", "15.12.2026"}, // deadline is format:date
		{"task", "create", "--name", "T", "--project", "1", "--complete=vielleicht"},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, args), 5)
		wantRequests(t, srv, 0)
	}
}

func TestTaskUpdateSendsOnlyChangedFields(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"task", "update", "488", "--complete", "--deadline", "2027-01-31", "-o", "json"}), 0)
	wantBody(t, srv, "PUT", "/tracker/tasks/488", `{"complete":"true","deadline":"2027-01-31"}`)

	// update never takes the project from the project file (spec 9.8)
	app, srv = apiApp(t)
	writeProjectFile(t, app, "project = 34\n")
	wantExit(t, app, Run(app, []string{"task", "update", "488", "--flagged=false", "-o", "json"}), 0)
	wantBody(t, srv, "PUT", "/tracker/tasks/488", `{"flagged":"false"}`)

	app, srv = apiApp(t)
	wantExit(t, app, Run(app, []string{"task", "update", "488"}), 2)
	wantRequests(t, srv, 0)
}

func TestCustomAttributeCreateUpdate(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"custom-attribute", "create", "--name", "Kostenstelle", "--attributable-type", "Project",
		"--data-type", "string", "-o", "json"}), 0)
	wantBody(t, srv, "POST", "/settings/custom_attributes", `{"name":"Kostenstelle","attributable_type":"Project","data_type":"string"}`)
	if out := mustOut(app); !strings.Contains(out, `"type": "custom_attribute"`) {
		t.Errorf("stdout = %s", out)
	}

	app, srv = apiApp(t)
	wantExit(t, app, Run(app, []string{"custom-attribute", "update", "338", "--name", "Kostenträger", "-o", "json"}), 0)
	wantBody(t, srv, "PUT", "/settings/custom_attributes/338", `{"name":"Kostenträger"}`)

	for _, tc := range []struct {
		args []string
		exit int
	}{
		{[]string{"custom-attribute", "create", "--name", "X", "--attributable-type", "Company", "--data-type", "string"}, 5},
		{[]string{"custom-attribute", "create", "--name", "X", "--attributable-type", "Project", "--data-type", "zahl"}, 5},
		{[]string{"custom-attribute", "create", "--name", "X"}, 5},
		{[]string{"custom-attribute", "update", "338"}, 2},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, tc.args), tc.exit)
		wantRequests(t, srv, 0)
	}
}

// resourcePaths are the object paths of the three resources.
var resourcePaths = map[string]string{
	"project":          "/projects/4710",
	"task":             "/tracker/tasks/488",
	"custom-attribute": "/settings/custom_attributes/338",
}

func TestProjectTaskCustomAttributeDeleteNeedsConfirmation(t *testing.T) {
	for resource, path := range resourcePaths {
		id := path[strings.LastIndexByte(path, '/')+1:]
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, []string{resource, "delete", id}), 8)
		wantRequests(t, srv, 0)
		if _, errs := bufs(app); !strings.Contains(errs, "--yes") {
			t.Errorf("%s: stderr = %q", resource, errs)
		}

		app, srv = apiApp(t)
		wantExit(t, app, Run(app, []string{resource, "delete", id, "--yes", "-o", "json"}), 0)
		if r := wantRequests(t, srv, 1)[0]; r.Method != "DELETE" || r.Path != path {
			t.Errorf("%s: request = %s %s", resource, r.Method, r.Path)
		}
		if out := mustOut(app); out != "{\n  \"id\": "+id+",\n  \"deleted\": true\n}\n" {
			t.Errorf("%s: stdout = %q", resource, out)
		}
	}
}

// archive is not destructive (spec 8.1): no confirmation, also without TTY.
func TestProjectTaskCustomAttributeArchiveWithoutConfirmation(t *testing.T) {
	for resource, path := range resourcePaths {
		id := path[strings.LastIndexByte(path, '/')+1:]
		for _, action := range []string{"archive", "unarchive"} {
			app, srv := apiApp(t) // no TTY, no --yes
			wantExit(t, app, Run(app, []string{resource, action, id, "-o", "json"}), 0)
			if r := wantRequests(t, srv, 1)[0]; r.Method != "POST" || r.Path != path+"/"+action {
				t.Errorf("%s %s: request = %s %s", resource, action, r.Method, r.Path)
			}
			if out := mustOut(app); !strings.Contains(out, `"id"`) {
				t.Errorf("%s %s: stdout = %s", resource, action, out)
			}
		}
	}
}

// mutations are the mutating commands of the three resources with valid
// arguments, and what their dry run shows.
var mutations = []struct {
	args          []string
	method, path  string
	credits, body string
}{
	{[]string{"project", "create", "--name", "P", "--customer", "3283", "--budget-money", "5000"},
		"POST", "/projects", `"credits": 10`, `"budget_money": "5000"`},
	{[]string{"project", "update", "4710", "--budget-time", "40"},
		"PUT", "/projects/4710", `"credits": 1`, `"budget_time": "40"`},
	{[]string{"project", "delete", "4710"}, "DELETE", "/projects/4710", `"credits": 1`, `"body": null`},
	{[]string{"project", "archive", "4710"}, "POST", "/projects/4710/archive", `"credits": 1`, `"body": null`},
	{[]string{"project", "unarchive", "4710"}, "POST", "/projects/4710/unarchive", `"credits": 1`, `"body": null`},
	{[]string{"task", "create", "--name", "T", "--project", "4823", "--complete"},
		"POST", "/tracker/tasks", `"credits": 10`, `"complete": "true"`},
	{[]string{"task", "update", "488", "--flagged"}, "PUT", "/tracker/tasks/488", `"credits": 1`, `"flagged": "true"`},
	{[]string{"task", "delete", "488"}, "DELETE", "/tracker/tasks/488", `"credits": 1`, `"body": null`},
	{[]string{"task", "archive", "488"}, "POST", "/tracker/tasks/488/archive", `"credits": 1`, `"body": null`},
	{[]string{"task", "unarchive", "488"}, "POST", "/tracker/tasks/488/unarchive", `"credits": 1`, `"body": null`},
	{[]string{"custom-attribute", "create", "--name", "K", "--attributable-type", "Project", "--data-type", "boolean"},
		"POST", "/settings/custom_attributes", `"credits": 10`, `"data_type": "boolean"`},
	{[]string{"custom-attribute", "update", "338", "--name", "K"},
		"PUT", "/settings/custom_attributes/338", `"credits": 1`, `"name": "K"`},
	{[]string{"custom-attribute", "delete", "338"}, "DELETE", "/settings/custom_attributes/338", `"credits": 1`, `"body": null`},
	{[]string{"custom-attribute", "archive", "338"}, "POST", "/settings/custom_attributes/338/archive", `"credits": 1`, `"body": null`},
	{[]string{"custom-attribute", "unarchive", "338"}, "POST", "/settings/custom_attributes/338/unarchive", `"credits": 1`, `"body": null`},
}

// The registered mutations run the protection layer (Task 24).
func TestProjectTaskCustomAttributeMutationsDryRun(t *testing.T) {
	for _, tc := range mutations {
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

func TestProjectTaskCustomAttributeMutationsReadOnly(t *testing.T) {
	ro := map[string]string{"PAPIERKRAM_TOKEN": "test-token-ro", "PAPIERKRAM_READ_ONLY": "1"}
	for _, tc := range mutations {
		for _, extra := range [][]string{{"--yes"}, {"--dry-run"}} {
			app, srv := mockApp(t, ro)
			wantExit(t, app, Run(app, append(tc.args, extra...)), 9)
			wantRequests(t, srv, 0)
		}
	}
	for _, resource := range []string{"project", "task", "custom-attribute"} {
		app, srv := mockApp(t, ro)
		wantExit(t, app, Run(app, []string{resource, "list", "-o", "json"}), 0)
		wantRequests(t, srv, 1)
	}
}

// Spec 9.8: project describe without ID takes the project from the
// environment or the project file; an ID always wins.
func TestProjectDescribeWithoutIDUsesProjectFile(t *testing.T) {
	app, srv := apiApp(t)
	path := writeProjectFile(t, app, "project = 34\n")
	wantExit(t, app, Run(app, []string{"project", "describe", "-o", "json"}), 0)
	if r := wantRequests(t, srv, 1)[0]; r.Method != "GET" || r.Path != "/projects/34" {
		t.Errorf("request = %s %s", r.Method, r.Path)
	}
	out, errs := bufs(app)
	if !strings.Contains(out, `"type": "project"`) {
		t.Errorf("stdout = %s", out)
	}
	if want := "Hinweis: Projekt 34 aus Projektdatei " + path + "\n"; errs != want {
		t.Errorf("stderr = %q, erwartet %q", errs, want)
	}

	// the ID wins over the project file, without a hint
	app, srv = apiApp(t)
	writeProjectFile(t, app, "project = 34\n")
	wantExit(t, app, Run(app, []string{"project", "describe", "4710", "-o", "json"}), 0)
	if r := wantRequests(t, srv, 1)[0]; r.Path != "/projects/4710" {
		t.Errorf("request = %s", r.Path)
	}
	if _, errs := bufs(app); errs != "" {
		t.Errorf("stderr = %q", errs)
	}

	// PAPIERKRAM_PROJECT wins over the project file
	app, srv = mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-p", "PAPIERKRAM_PROJECT": "7"})
	writeProjectFile(t, app, "project = 34\n")
	wantExit(t, app, Run(app, []string{"project", "describe", "-o", "json"}), 0)
	if r := wantRequests(t, srv, 1)[0]; r.Path != "/projects/7" {
		t.Errorf("request = %s", r.Path)
	}
	if _, errs := bufs(app); errs != "Hinweis: Projekt 7 aus PAPIERKRAM_PROJECT\n" {
		t.Errorf("stderr = %q", errs)
	}
	assertNoToken(t, app)
}

// Without ID and without a preset project describe is a usage error
// (exit 2) before any request, with a hint how to preset the project.
func TestProjectDescribeWithoutIDNeedsProjectFile(t *testing.T) {
	for _, tc := range []struct {
		name, file string
		env        map[string]string
		args       []string
		want       []string
	}{
		{"no project file", "", nil, nil, []string{"keine ID angegeben", "papierkram local init --project <id>", "PAPIERKRAM_PROJECT"}},
		{"project file without project", "company = 12\n", nil, nil, []string{"keine ID angegeben", ".papierkram.toml"}},
		{"invalid env", "", map[string]string{"PAPIERKRAM_PROJECT": "abc"}, nil, []string{"PAPIERKRAM_PROJECT", `"abc"`}},
		{"broken project file", "project = \"x\"\n", nil, nil, []string{"Projektdatei", "project"}},
		{"two IDs", "project = 34\n", nil, []string{"1", "2"}, []string{"unerwartetes Argument"}},
		{"invalid ID", "project = 34\n", nil, []string{"abc"}, []string{"ungültige ID"}},
	} {
		env := map[string]string{"PAPIERKRAM_TOKEN": "test-token-p"}
		for k, v := range tc.env {
			env[k] = v
		}
		app, srv := mockApp(t, env)
		if tc.file != "" {
			writeProjectFile(t, app, tc.file)
		}
		wantExit(t, app, Run(app, append([]string{"project", "describe"}, tc.args...)), 2)
		wantRequests(t, srv, 0)
		_, errs := bufs(app)
		for _, want := range tc.want {
			if !strings.Contains(errs, want) {
				t.Errorf("%s: stderr ohne %q: %s", tc.name, want, errs)
			}
		}
	}
}

// The ID of project describe is optional; the German usage line says so.
func TestProjectDescribeUsageLine(t *testing.T) {
	app, _ := apiApp(t)
	cmd, _, err := NewRootCmd(app).Find([]string{"project", "describe"})
	if err != nil {
		t.Fatal(err)
	}
	if got := useLine(cmd); got != "papierkram project describe [<id>] [Optionen]" {
		t.Errorf("useLine = %q", got)
	}
	// the other describe commands keep the required ID
	cmd, _, _ = NewRootCmd(app).Find([]string{"task", "describe"})
	if got := useLine(cmd); got != "papierkram task describe <id> [Optionen]" {
		t.Errorf("task describe: useLine = %q", got)
	}
}

// Spec 9.8: task list filters by the preset project unless --project is
// given.
func TestTaskListUsesProjectFile(t *testing.T) {
	app, srv := apiApp(t)
	path := writeProjectFile(t, app, "project = 34\n")
	wantExit(t, app, Run(app, []string{"task", "list", "-o", "json"}), 0)
	want := url.Values{"project_id": {"34"}, "page_size": {"25"}}
	if r := wantRequests(t, srv, 1)[0]; r.Query.Encode() != want.Encode() {
		t.Errorf("query = %s", r.Query.Encode())
	}
	if _, errs := bufs(app); errs != "Hinweis: Projekt 34 aus Projektdatei "+path+"\n" {
		t.Errorf("stderr = %q", errs)
	}

	app, srv = apiApp(t)
	writeProjectFile(t, app, "project = 34\n")
	wantExit(t, app, Run(app, []string{"task", "list", "--project", "5", "-o", "json"}), 0)
	want = url.Values{"project_id": {"5"}, "page_size": {"25"}}
	if r := wantRequests(t, srv, 1)[0]; r.Query.Encode() != want.Encode() {
		t.Errorf("--project: query = %s", r.Query.Encode())
	}
	if _, errs := bufs(app); errs != "" {
		t.Errorf("--project: stderr = %q", errs)
	}

	// only task list: project list and the other lists ignore the preset
	app, srv = apiApp(t)
	writeProjectFile(t, app, "project = 34\ncompany = 12\n")
	wantExit(t, app, Run(app, []string{"project", "list", "-o", "json"}), 0)
	if r := wantRequests(t, srv, 1)[0]; r.Query.Encode() != "page_size=25" {
		t.Errorf("project list: query = %s", r.Query.Encode())
	}

	// an invalid preset is a usage error before any request
	app, srv = mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-p", "PAPIERKRAM_PROJECT": "-1"})
	wantExit(t, app, Run(app, []string{"task", "list"}), 2)
	wantRequests(t, srv, 0)
}

// Spec 9.8: task create takes the project from the preset unless --project
// or --input sets it.
func TestTaskCreateUsesProjectFile(t *testing.T) {
	app, srv := apiApp(t)
	path := writeProjectFile(t, app, "project = 34\n")
	wantExit(t, app, Run(app, []string{"task", "create", "--name", "Support", "-o", "json"}), 0)
	wantBody(t, srv, "POST", "/tracker/tasks", `{"name":"Support","project":{"id":34}}`)
	if _, errs := bufs(app); errs != "Hinweis: Projekt 34 aus Projektdatei "+path+"\n" {
		t.Errorf("stderr = %q", errs)
	}

	app, srv = apiApp(t)
	writeProjectFile(t, app, "project = 34\n")
	wantExit(t, app, Run(app, []string{"task", "create", "--name", "Support", "--project", "5", "-o", "json"}), 0)
	wantBody(t, srv, "POST", "/tracker/tasks", `{"name":"Support","project":{"id":5}}`)

	app, srv = apiApp(t)
	writeProjectFile(t, app, "project = 34\n")
	app.In = strings.NewReader(`{"name":"Support","project":{"id":6}}`)
	wantExit(t, app, Run(app, []string{"task", "create", "--input", "-", "-o", "json"}), 0)
	wantBody(t, srv, "POST", "/tracker/tasks", `{"name":"Support","project":{"id":6}}`)

	// dry run shows the preset project
	app, srv = mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-p", "PAPIERKRAM_PROJECT": "7"})
	wantExit(t, app, Run(app, []string{"task", "create", "--name", "Support", "--dry-run", "-o", "json"}), 0)
	wantRequests(t, srv, 0)
	if out := mustOut(app); !strings.Contains(out, `"project": {`) || !strings.Contains(out, `"id": 7`) {
		t.Errorf("dry run: stdout = %s", out)
	}

	// read-only before the project file is read (plan order)
	app, srv = mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-p", "PAPIERKRAM_READ_ONLY": "1"})
	writeProjectFile(t, app, "project = 34\n")
	wantExit(t, app, Run(app, []string{"task", "create", "--name", "Support"}), 9)
	wantRequests(t, srv, 0)
}

// Every preset of the registry is a flag of its built command (create
// field flag, list filter, path flag) or the optional ID of describe of
// its own resource (project describe).
func TestPresetFlagsExist(t *testing.T) {
	app, _ := apiApp(t)
	root := NewRootCmd(app)
	n := 0
	for _, e := range registry.Entries() {
		if e.Preset == "" {
			continue
		}
		n++
		cmd, _, err := root.Find([]string{e.Resource, e.Action})
		if err != nil || cmd.Name() != e.Action {
			t.Errorf("%s %s: Befehl fehlt", e.Resource, e.Action)
			continue
		}
		if !strings.Contains(cmd.Long, presetNote(e.Preset)) {
			t.Errorf("%s %s: Hilfe nennt die Voreinstellung nicht: %q", e.Resource, e.Action, cmd.Long)
		}
		if presetsID(e) {
			if !strings.Contains(cmd.Use, "[<id>]") {
				t.Errorf("%s describe: Use = %q", e.Resource, cmd.Use)
			}
			continue
		}
		if cmd.Flags().Lookup(e.Preset) == nil {
			t.Errorf("%s %s: kein Flag --%s für die Voreinstellung", e.Resource, e.Action, e.Preset)
		}
	}
	if n == 0 {
		t.Error("keine Voreinstellungen geprüft")
	}
}
