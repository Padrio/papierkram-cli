package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/testutil/mockapi"
)

// Controller ruling N1 (project-wide): userinfo (user:password@) of
// PAPIERKRAM_BASE_URL appears in no output, message, debug line or file,
// also when the URL cannot be parsed. Requests may still use it.
func TestNoCredentialsFromBaseURL(t *testing.T) {
	creds := func(srv *mockapi.Server) string {
		return strings.Replace(srv.URL(), "http://", "http://alice:s3cret@", 1)
	}
	const badPort = "http://alice:s3cret@127.0.0.1:badport/api/v1"
	clean := func(t *testing.T, what, s string) {
		t.Helper()
		if strings.Contains(s, "alice") || strings.Contains(s, "s3cret") {
			t.Errorf("%s shows credentials:\n%s", what, s)
		}
	}
	// cleanFiles checks every file below dir (timer state, name cache)
	cleanFiles := func(t *testing.T, dir string) {
		t.Helper()
		_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				data, _ := os.ReadFile(p)
				clean(t, p, string(data))
			}
			return nil
		})
	}
	pdf := writeFile(t, "beleg.pdf", pdfContent)
	type step struct {
		args []string
		tty  string // stdin in a terminal; "" without one
	}
	for _, base := range []string{"creds", "bad port"} {
		for _, tc := range []struct {
			name  string
			steps []step
		}{
			{"dry run generic", []step{{args: []string{"invoice", "delete", "459", "--dry-run"}}, {args: []string{"invoice", "delete", "459", "--dry-run", "-o", "json"}}}},
			{"dry run deliver", []step{{args: []string{"invoice", "deliver", "459", "--via", "pdf", "--dry-run", "-o", "json"}}}},
			{"dry run upload", []step{{args: []string{"voucher", "upload", "295", pdf, "--dry-run", "-o", "json"}},
				{args: append(append([]string{}, voucherCreateArgs...), "--file", pdf, "--dry-run")}}},
			{"dry run from estimate", []step{{args: append(append([]string{}, fromEstimateArgs...), "--dry-run", "-o", "json")}}},
			{"debug", []step{{args: []string{"user", "list", "--debug"}}, {args: []string{"invoice", "delete", "459", "--yes", "--debug"}}}},
			{"auth status", []step{{args: []string{"auth", "status"}}, {args: []string{"auth", "status", "-o", "json"}}}},
			{"time log summary", []step{{args: []string{"time", "log", "--task", "492", "--user", "1", "--from", "09:00", "--to", "10:00"}, tty: "\n"}}},
			{"timer", []step{
				{args: []string{"time", "start", "--task", "545", "--user", "1"}},
				{args: []string{"time", "status", "-o", "json"}},
				{args: []string{"time", "status", "-o", "wide"}},
				{args: []string{"time", "stop", "--dry-run", "-o", "json"}},
				{args: []string{"time", "stop", "--yes", "--debug"}},
			}},
		} {
			t.Run(base+"/"+tc.name, func(t *testing.T) {
				app, srv := logApp(t, morning(t))
				url := badPort
				if base == "creds" {
					url = creds(srv)
				}
				app.Env = envMap(map[string]string{"PAPIERKRAM_BASE_URL": url, "PAPIERKRAM_TOKEN": "test-token-redact"})
				for _, s := range tc.steps {
					app.StdinTTY = s.tty != ""
					app.In = strings.NewReader(s.tty)
					at(app, morning(t), s.args...)
					out, errs := bufs(app)
					clean(t, strings.Join(s.args, " "), out+errs)
				}
				cleanFiles(t, app.CacheDir)
			})
		}
	}

	// the timer in another account: the warning of status and the
	// refusal of stop name hosts only
	app, srv := logApp(t, morning(t))
	app.Env = envMap(map[string]string{"PAPIERKRAM_BASE_URL": creds(srv), "PAPIERKRAM_TOKEN": "test-token-redact"})
	wantExit(t, app, at(app, nine(t), "time", "start", "--task", "545", "--user", "1"), 0)
	if !strings.Contains(string(timerFile(t, app, "")), `"base_url":"`+srv.URL()+`"`) {
		t.Errorf("state = %s", timerFile(t, app, ""))
	}
	for _, other := range []string{creds(mockapi.New(t)), badPort} {
		app.Env = envMap(map[string]string{"PAPIERKRAM_BASE_URL": other, "PAPIERKRAM_TOKEN": "test-token-redact"})
		wantExit(t, app, at(app, morning(t), "time", "status", "-o", "json"), 0)
		out, errs := bufs(app)
		clean(t, "status in another account", out+errs)
		wantExit(t, app, at(app, morning(t), "time", "stop", "--yes"), 2)
		out, errs = bufs(app)
		clean(t, "stop in another account", out+errs)
	}
	// the same account with other credentials is the same account
	app.Env = envMap(map[string]string{"PAPIERKRAM_BASE_URL": strings.Replace(srv.URL(), "http://", "http://bob:other@", 1), "PAPIERKRAM_TOKEN": "test-token-redact"})
	wantExit(t, app, at(app, morning(t), "time", "stop", "--yes", "-o", "json"), 0)

	// auth login names the base URL it checks the token against
	app, srv = mockApp(t, nil)
	writeCfg(t, app, twoCtx)
	app.Env = envMap(map[string]string{"PAPIERKRAM_BASE_URL": creds(srv)})
	app.In = strings.NewReader("test-token-9\n")
	wantExit(t, app, Run(app, []string{"auth", "login", "--token-stdin"}), 0)
	out, errs := bufs(app)
	clean(t, "auth login", out+errs)
	if !strings.Contains(errs, "PAPIERKRAM_BASE_URL") {
		t.Errorf("stderr = %s", errs)
	}
}
