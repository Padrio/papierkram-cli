package cli

import (
	"strings"
	"testing"
)

// Review B-M5: a context name the shell would read otherwise (here a;b,
// a valid name) is quoted in the commands the hints name, like the hints
// of the timer (shellWord); a plain name stays as is.
func TestHintCommandsQuoteContextName(t *testing.T) {
	const quoted = `--context 'a;b'`
	cfgAB := "[contexts.'a;b']\nsubdomain = \"firma\"\n"

	t.Run("auth login without subdomain", func(t *testing.T) {
		app, _, errb := newTestApp(t)
		app.In = strings.NewReader("test-token-ab\n")
		if code := Run(app, []string{"auth", "login", "--context", "a;b", "--token-stdin", "--no-input", "-o", "json"}); code != 2 {
			t.Fatalf("exit %d: %s", code, errb.String())
		}
		e, ok := lastJSONError(t, errb.String())
		if !ok || e.Hint != "Mit Subdomain anlegen: papierkram auth login "+quoted+" --subdomain <subdomain>" {
			t.Errorf("Fehler = %+v (%v)", e, ok)
		}
	})

	t.Run("no token", func(t *testing.T) {
		app, _ := mockApp(t, nil)
		writeCfg(t, app, cfgAB)
		if code := run(t, app, "user", "list", "--context", "a;b", "-o", "json"); code != 3 {
			t.Fatalf("exit %d", code)
		}
		_, errs := bufs(app)
		e, ok := lastJSONError(t, errs)
		if !ok || e.Hint != "Anmelden: papierkram auth login "+quoted+" (oder PAPIERKRAM_TOKEN setzen)" {
			t.Errorf("Fehler = %+v (%v)", e, ok)
		}
	})

	t.Run("auth logout half done", func(t *testing.T) {
		app, _, errb := newTestApp(t)
		writeCfg(t, app, cfgAB+"token = \"test-token-cfg\"\n")
		if err := app.Keyring.Set("a;b", "test-token-ab"); err != nil {
			t.Fatal(err)
		}
		saveFails(app)
		if code := Run(app, []string{"auth", "logout", "--context", "a;b", "-o", "json"}); code != 1 {
			t.Fatalf("exit %d: %s", code, errb.String())
		}
		e, ok := lastJSONError(t, errb.String())
		if !ok || !strings.HasSuffix(e.Hint, "danach erneut papierkram auth logout "+quoted) {
			t.Errorf("Fehler = %+v (%v)", e, ok)
		}
	})

	t.Run("auth login without keychain", func(t *testing.T) {
		app, _ := mockApp(t, nil)
		writeCfg(t, app, cfgAB)
		app.Keyring = unavailableStore{}
		app.In = strings.NewReader("test-token-k\n")
		if code := run(t, app, "auth", "login", "--context", "a;b", "--token-stdin"); code != 3 {
			t.Fatalf("exit %d", code)
		}
		wantStderr(t, app, "papierkram auth login "+quoted+" --token-stdin --insecure-storage < token.txt")
	})

	t.Run("context create with an old keychain entry", func(t *testing.T) {
		app, _, errb := newTestApp(t)
		if err := app.Keyring.Set("a;b", "test-token-alt"); err != nil {
			t.Fatal(err)
		}
		if code := Run(app, []string{"context", "create", "a;b", "--subdomain", "firma"}); code != 0 {
			t.Fatalf("exit %d: %s", code, errb.String())
		}
		if !strings.Contains(errb.String(), "Token neu hinterlegen: papierkram auth login "+quoted+"\n") {
			t.Errorf("stderr = %q", errb.String())
		}
	})
}
