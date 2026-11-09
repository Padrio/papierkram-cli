package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/Padrio/papierkram-cli/internal/auth"
	"github.com/Padrio/papierkram-cli/internal/config"
	"github.com/Padrio/papierkram-cli/internal/registry"
	"github.com/Padrio/papierkram-cli/internal/spec"
	"github.com/Padrio/papierkram-cli/internal/testutil/mockapi"
)

// completionResult is the answer of papierkram __complete: the candidates
// and the directive line cobra prints last.
type completionResult struct {
	lines     []string
	directive cobra.ShellCompDirective
}

// complete runs papierkram __complete with args and checks that it exits 0
// and that stderr holds nothing but cobra's own directive line: completion
// is silent.
func complete(t *testing.T, app *App, args ...string) completionResult {
	t.Helper()
	before, beforeErr := bufs(app)
	code := Run(app, append([]string{cobra.ShellCompRequestCmd}, args...))
	out, errs := bufs(app)
	out, errs = out[len(before):], errs[len(beforeErr):]
	if code != 0 {
		t.Fatalf("__complete %q: exit %d\nstdout: %s\nstderr: %s", args, code, out, errs)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	last := lines[len(lines)-1]
	var d int
	if _, err := fmt.Sscanf(last, ":%d", &d); err != nil {
		t.Fatalf("__complete %q: keine Direktive in %q", args, out)
	}
	res := completionResult{lines: lines[:len(lines)-1], directive: cobra.ShellCompDirective(d)}
	if want := "Completion ended with directive: " + directiveName(res.directive) + "\n"; errs != want {
		t.Errorf("__complete %q: stderr %q, erwartet nur %q", args, errs, want)
	}
	return res
}

// directiveName is cobra's name of d in its stderr line.
func directiveName(d cobra.ShellCompDirective) string {
	var names []string
	if d&cobra.ShellCompDirectiveNoSpace != 0 {
		names = append(names, "ShellCompDirectiveNoSpace")
	}
	if d&cobra.ShellCompDirectiveNoFileComp != 0 {
		names = append(names, "ShellCompDirectiveNoFileComp")
	}
	if len(names) == 0 {
		return "ShellCompDirectiveDefault"
	}
	return strings.Join(names, ", ")
}

func (r completionResult) want(t *testing.T, lines []string, d cobra.ShellCompDirective) {
	t.Helper()
	if !slices.Equal(r.lines, lines) {
		t.Errorf("Kandidaten %q, erwartet %q", r.lines, lines)
	}
	if r.directive != d {
		t.Errorf("Direktive %d, erwartet %d", r.directive, d)
	}
}

// listBody is a list envelope with entries.
func listBody(t *testing.T, entries ...map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"type": "list", "page": 1, "page_size": 100, "total_pages": 1,
		"total_entries": len(entries), "has_more": false, "entries": entries,
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func invoiceEntries(t *testing.T) []byte {
	return listBody(t,
		map[string]any{"id": 441, "name": "Beratung Oktober"},
		map[string]any{"id": 4410, "name": "Wartung"},
		map[string]any{"id": 52, "name": ""},
	)
}

const noFile = cobra.ShellCompDirectiveNoFileComp

// The positional ID of describe completes "id<TAB>name" from a list
// request (page 1, page_size 100, spec 7.3); the answer is cached per
// context and resource for 60 s.
func TestCompletionIDsFromAPIAndCache(t *testing.T) {
	app, srv := apiApp(t)
	srv.On("GET", "/income/invoices", http.StatusOK, invoiceEntries(t))
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	app.Now = func() time.Time { return now }
	want := []string{"441\tBeratung Oktober", "4410\tWartung", "52"}

	complete(t, app, "invoice", "describe", "").want(t, want, noFile)
	reqs := wantRequests(t, srv, 1)
	if reqs[0].Path != "/income/invoices" || reqs[0].Query.Encode() != "page_size=100" {
		t.Errorf("Request %s?%s", reqs[0].Path, reqs[0].Query.Encode())
	}

	// within 60 s: from the cache, no request; the prefix filters
	now = now.Add(59 * time.Second)
	complete(t, app, "invoice", "describe", "").want(t, want, noFile)
	complete(t, app, "rechnung", "describe", "44").want(t, want[:2], noFile)
	wantRequests(t, srv, 1)

	// after the TTL the list is loaded again
	now = now.Add(time.Second)
	complete(t, app, "invoice", "describe", "").want(t, want, noFile)
	wantRequests(t, srv, 2)

	// a second positional argument gets nothing
	complete(t, app, "invoice", "describe", "441", "").want(t, nil, noFile)
	wantRequests(t, srv, 2)
}

// The cache file is <cacheDir>/papierkram/completion-<ctx>-<resource>.json
// with the context name in the file name form of the timer state; it is
// private, has no temporary leftovers and holds neither the token nor the
// userinfo of the base URL.
func TestCompletionCacheFile(t *testing.T) {
	app, srv := apiApp(t)
	srv.On("GET", "/income/invoices", http.StatusOK, invoiceEntries(t))
	base := strings.Replace(srv.URL(), "http://", "http://nutzer:geheim@", 1)
	app.Env = envMap(map[string]string{"PAPIERKRAM_BASE_URL": base, "PAPIERKRAM_TOKEN": "test-token-cache"})
	writeCfg(t, app, "[contexts.Firma]\nsubdomain = \"firma\"\n")
	app.Flags = Flags{}
	complete(t, app, "invoice", "describe", "--context", "Firma", "")
	dir := filepath.Join(app.CacheDir, "papierkram")
	path := filepath.Join(dir, "completion-"+config.FileName("Firma")+"-invoice.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Cache-Datei fehlt: %v", err)
	}
	if s := string(data); strings.Contains(s, "geheim") || strings.Contains(s, "nutzer") || strings.Contains(s, "test-token") {
		t.Errorf("Cache enthält Zugangsdaten: %s", s)
	}
	if runtime.GOOS != "windows" {
		if st, err := os.Stat(path); err != nil || st.Mode().Perm() != 0o600 {
			t.Errorf("Rechte %v, %v", st.Mode().Perm(), err)
		}
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "*.tmp")); len(left) > 0 {
		t.Errorf("temporäre Dateien: %v", left)
	}
	// direct mode (no context) has its own file
	app.Env = envMap(map[string]string{"PAPIERKRAM_BASE_URL": srv.URL(), "PAPIERKRAM_TOKEN": "test-token-cache"})
	writeCfg(t, app, "")
	complete(t, app, "invoice", "describe", "")
	if _, err := os.Stat(filepath.Join(dir, "completion-"+config.DirectFileName+"-invoice.json")); err != nil {
		t.Errorf("Cache ohne Kontext: %v", err)
	}
}

// PAPIERKRAM_CACHE_DIR moves the cache like for the timer and the names;
// a cache of another account (base URL) is not used.
func TestCompletionCacheScope(t *testing.T) {
	app, srv := apiApp(t)
	srv.On("GET", "/income/invoices", http.StatusOK, invoiceEntries(t))
	other := t.TempDir()
	env := map[string]string{"PAPIERKRAM_BASE_URL": srv.URL(), "PAPIERKRAM_TOKEN": "test-token-scope", "PAPIERKRAM_CACHE_DIR": other}
	app.Env = envMap(env)
	complete(t, app, "invoice", "describe", "")
	if _, err := os.Stat(filepath.Join(other, "papierkram", "completion-.direct-invoice.json")); err != nil {
		t.Errorf("PAPIERKRAM_CACHE_DIR: %v", err)
	}
	srv2 := mockapi.New(t)
	srv2.On("GET", "/income/invoices", http.StatusOK, listBody(t, map[string]any{"id": 7, "name": "Andere"}))
	env["PAPIERKRAM_BASE_URL"] = srv2.URL()
	complete(t, app, "invoice", "describe", "").want(t, []string{"7\tAndere"}, noFile)
	wantRequests(t, srv2, 1)
}

// completion_lookup = false: neither a request nor a cache read.
func TestCompletionLookupOff(t *testing.T) {
	app, srv := apiApp(t)
	app.Env = envMap(map[string]string{"PAPIERKRAM_BASE_URL": srv.URL()})
	writeCfg(t, app, "active_context = \"a\"\n\n[contexts.a]\nsubdomain = \"firma\"\ncompletion_lookup = false\n")
	if err := app.Keyring.Set("a", "test-token-off"); err != nil {
		t.Fatal(err)
	}
	// a valid, fresh cache of this context is ignored as well
	writeCompletionCacheFile(t, app, "a", srv.URL(), `[{"id":1,"name":"Alt"}]`)
	complete(t, app, "invoice", "describe", "").want(t, nil, noFile)
	complete(t, app, "invoice", "list", "--company", "").want(t, nil, noFile)
	wantRequests(t, srv, 0)

	// switched on (default): the same context completes
	writeCfg(t, app, "active_context = \"a\"\n\n[contexts.a]\nsubdomain = \"firma\"\n")
	srv.On("GET", "/income/invoices", http.StatusOK, invoiceEntries(t))
	if r := complete(t, app, "invoice", "describe", ""); len(r.lines) != 1 || r.lines[0] != "1\tAlt" {
		t.Errorf("mit Cache: %q", r.lines)
	}
	wantRequests(t, srv, 0)
}

// writeCompletionCacheFile writes a fresh cache file of ctx for invoices.
func writeCompletionCacheFile(t *testing.T, app *App, ctx, baseURL, entries string) string {
	t.Helper()
	return writeCompletionCacheAt(t, app, ctx, baseURL, entries, app.Now())
}

// writeCompletionCacheAt writes a cache file of ctx for invoices stored at
// the given time.
func writeCompletionCacheAt(t *testing.T, app *App, ctx, baseURL, entries string, at time.Time) string {
	t.Helper()
	path := completionCachePath(app.CacheDir, ctx, "invoice")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	stored, _ := json.Marshal(at)
	content := fmt.Sprintf(`{"scope":%q,"resource":"invoice","stored":%s,"entries":%s}`, ctx+" "+baseURL, stored, entries)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// Every failure means no IDs, exit 0, nothing on stderr (besides cobra's
// directive line) and no file completion.
func TestCompletionFailuresAreSilent(t *testing.T) {
	t.Run("ohne Token", func(t *testing.T) {
		app, srv := mockApp(t, nil)
		complete(t, app, "invoice", "describe", "").want(t, nil, noFile)
		wantRequests(t, srv, 0)
	})
	t.Run("ohne Subdomain", func(t *testing.T) {
		app, _, _ := newTestApp(t)
		complete(t, app, "invoice", "describe", "").want(t, nil, noFile)
	})
	t.Run("Serverfehler", func(t *testing.T) {
		app, srv := apiApp(t)
		srv.On("GET", "/income/invoices", http.StatusInternalServerError, []byte(`{"error":"kaputt"}`))
		complete(t, app, "invoice", "describe", "").want(t, nil, noFile)
		if len(srv.Requests()) == 0 {
			t.Error("kein Request")
		}
	})
	t.Run("Netzwerkfehler", func(t *testing.T) {
		app, _, _ := newTestApp(t)
		app.Env = envMap(map[string]string{"PAPIERKRAM_BASE_URL": "http://127.0.0.1:1/api/v1", "PAPIERKRAM_TOKEN": "test-token-net"})
		complete(t, app, "invoice", "describe", "").want(t, nil, noFile)
	})
	t.Run("unerwartete Antwort", func(t *testing.T) {
		app, srv := apiApp(t)
		srv.On("GET", "/income/invoices", http.StatusOK, []byte(`[1,2]`))
		complete(t, app, "invoice", "describe", "").want(t, nil, noFile)
	})
	t.Run("kaputte Konfiguration", func(t *testing.T) {
		app, srv := apiApp(t)
		writeCfg(t, app, "active_context = [")
		complete(t, app, "invoice", "describe", "").want(t, nil, noFile)
		complete(t, app, "invoice", "list", "--context", "").want(t, nil, noFile)
		wantRequests(t, srv, 0)
	})
	t.Run("kaputter Cache ohne Token", func(t *testing.T) {
		app, srv := mockApp(t, nil)
		for _, content := range []string{"{kaputt", "", `{"scope":" ` + srv.URL() + `","resource":"invoice","stored":"2999-01-01T00:00:00Z","entries":[{"id":1}]}`} {
			path := completionCachePath(app.CacheDir, "", "invoice")
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			complete(t, app, "invoice", "describe", "").want(t, nil, noFile)
		}
		wantRequests(t, srv, 0)
	})
	t.Run("Cache-Verzeichnis blockiert", func(t *testing.T) {
		app, srv := apiApp(t)
		srv.On("GET", "/income/invoices", http.StatusOK, invoiceEntries(t))
		if err := os.WriteFile(filepath.Join(app.CacheDir, "papierkram"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		// the IDs of the request are shown, the cache is skipped
		if r := complete(t, app, "invoice", "describe", ""); len(r.lines) != 3 {
			t.Errorf("Kandidaten %q", r.lines)
		}
	})
	t.Run("Warnungen bleiben stumm", func(t *testing.T) {
		// an unclear PAPIERKRAM_READ_ONLY, environment credentials beside
		// --context and a low quota warn in normal commands; listing is a
		// read and allowed under read-only
		app, srv := apiApp(t)
		writeCfg(t, app, twoCtx)
		srv.On("GET", "/income/invoices", http.StatusOK, invoiceEntries(t), "X-Remaining-Quota", "3")
		app.Env = envMap(map[string]string{"PAPIERKRAM_BASE_URL": srv.URL(), "PAPIERKRAM_TOKEN": "test-token-warn", "PAPIERKRAM_READ_ONLY": "vielleicht"})
		if r := complete(t, app, "invoice", "describe", "--context", "b", "--debug", ""); len(r.lines) != 3 {
			t.Errorf("Kandidaten %q", r.lines)
		}
	})
}

// A corrupt or foreign cache file is a miss: the list is loaded again
// and the cache repaired.
func TestCompletionCorruptCacheIsMiss(t *testing.T) {
	app, srv := apiApp(t)
	srv.On("GET", "/income/invoices", http.StatusOK, invoiceEntries(t))
	path := completionCachePath(app.CacheDir, "", "invoice")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{kaputt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if r := complete(t, app, "invoice", "describe", ""); len(r.lines) != 3 {
		t.Errorf("Kandidaten %q", r.lines)
	}
	complete(t, app, "invoice", "describe", "")
	wantRequests(t, srv, 1)

	// a symbolic link is never read
	if runtime.GOOS != "windows" {
		target := filepath.Join(t.TempDir(), "x.json")
		data, _ := os.ReadFile(path)
		if err := os.WriteFile(target, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
		complete(t, app, "invoice", "describe", "")
		wantRequests(t, srv, 2)
	}
}

// Names from the API and from the cache are one line of printable text:
// no tab, line break, ESC or bidi override, at most 60 runes.
func TestCompletionSanitizesNames(t *testing.T) {
	app, srv := apiApp(t)
	long := strings.Repeat("ä", 80)
	srv.On("GET", "/income/invoices", http.StatusOK, listBody(t,
		map[string]any{"id": 1, "name": "Tab\tNeu\nZeile"},
		map[string]any{"id": 2, "name": "\x1b[31mRot\u202eTxt"},
		map[string]any{"id": 3, "name": long},
		map[string]any{"id": 4, "first_name": "Anna", "last_name": "Groß"},
		map[string]any{"id": 0, "name": "ohne ID"},
		map[string]any{"name": "fehlende ID"},
	))
	want := []string{"1\tTab Neu Zeile", "2\t[31mRot Txt", "3\t" + strings.Repeat("ä", 59) + "…", "4\tAnna Groß"}
	complete(t, app, "invoice", "describe", "").want(t, want, noFile)

	// a cache edited by hand is cleaned the same way
	writeCompletionCacheFile(t, app, "", srv.URL(), `[{"id":9,"name":"a\tb\u001b\u202ec\nd"}]`)
	complete(t, app, "invoice", "describe", "").want(t, []string{"9\ta b c d"}, noFile)
}

// ID flags complete the IDs of the resource they reference.
func TestCompletionIDFlags(t *testing.T) {
	app, srv := apiApp(t)
	srv.On("GET", "/contact/companies", http.StatusOK, listBody(t, map[string]any{"id": 2843, "name": "Muster GmbH"}))
	srv.On("GET", "/tracker/tasks", http.StatusOK, listBody(t, map[string]any{"id": 545, "name": "Support"}))
	companies := []string{"2843\tMuster GmbH"}
	complete(t, app, "invoice", "list", "--company", "").want(t, companies, noFile)
	complete(t, app, "invoice", "create", "--customer", "").want(t, companies, noFile)
	complete(t, app, "voucher", "create", "--creditor=").want(t, companies, noFile)
	complete(t, app, "time", "log", "--task", "").want(t, []string{"545\tSupport"}, noFile)
	// one request per resource, the rest from the cache
	if n := len(srv.Requests()); n != 2 {
		t.Errorf("%d Requests, erwartet 2", n)
	}
	// a reference without a list (contact person: per company) gets nothing
	complete(t, app, "invoice", "create", "--contact-person", "").want(t, nil, noFile)
	complete(t, app, "person", "describe", "").want(t, nil, noFile)
}

// The ID completion has its own short timeout, independent of --timeout.
func TestCompletionTimeout(t *testing.T) {
	defer func(d time.Duration) { completionTimeout = d }(completionTimeout)
	completionTimeout = 50 * time.Millisecond
	app, srv := apiApp(t)
	srv.OnFunc("GET", "/income/invoices", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
		w.WriteHeader(http.StatusInternalServerError)
	})
	start := time.Now()
	complete(t, app, "invoice", "describe", "--timeout", "1m", "").want(t, nil, noFile)
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("Completion dauerte %v", d)
	}
}

// Flags with allowed values complete them (the pk:enum annotation that
// help and commands --json use).
func TestCompletionEnumValues(t *testing.T) {
	app, srv := apiApp(t)
	complete(t, app, "invoice", "list", "-o", "").want(t, outputFormats, noFile)
	complete(t, app, "invoice", "list", "--output", "j").want(t, []string{"json"}, noFile)

	state := enumOf(t, app, "time list", "state")
	complete(t, app, "time", "list", "--state", "").want(t, state, noFile)
	typ := enumOf(t, app, "company list", "type")
	complete(t, app, "company", "list", "--type", "").want(t, typ, noFile)
	complete(t, app, "invoice", "deliver", "1", "--via", "").want(t, enumOf(t, app, "invoice deliver", "via"), noFile)
	wantRequests(t, srv, 0)
}

// enumOf is the pk:enum annotation of a flag; it must not be empty.
func enumOf(t *testing.T, app *App, command, flag string) []string {
	t.Helper()
	c, _, err := NewRootCmd(app).Find(strings.Fields(command))
	if err != nil {
		t.Fatal(err)
	}
	f := c.Flags().Lookup(flag)
	if f == nil || len(f.Annotations[flagEnum]) == 0 {
		t.Fatalf("%s --%s: keine erlaubten Werte", command, flag)
	}
	return f.Annotations[flagEnum]
}

// --item completes the values of enum keys of a line item: category=
// offers the 94 expense categories of the spec, quoted where the --item
// grammar needs it.
func TestCompletionItemCategory(t *testing.T) {
	app, _ := apiApp(t)
	cats := spec.VoucherCategories()
	if len(cats) != 94 {
		t.Fatalf("%d Kategorien", len(cats))
	}
	r := complete(t, app, "voucher", "create", "--item", "category=")
	if len(r.lines) != len(cats) || r.directive != noFile {
		t.Fatalf("%d Kandidaten, Direktive %d", len(r.lines), r.directive)
	}
	for i, c := range cats {
		want := "category=" + c
		if strings.ContainsAny(c, `,="\`) {
			want = `category="` + strings.ReplaceAll(strings.ReplaceAll(c, `\`, `\\`), `"`, `\"`) + `"`
		}
		if r.lines[i] != want {
			t.Errorf("Kandidat %d = %q, erwartet %q", i, r.lines[i], want)
		}
		// every candidate parses back to its category
		item, err := ParseItem(r.lines[i], nil)
		if err != nil || item["category"] != c {
			t.Errorf("%q: %v, %v", r.lines[i], item, err)
		}
	}
	// earlier pairs are kept, the prefix filters
	complete(t, app, "voucher", "update", "1", "--item", "name=Taxi,qty=1,category=Reisekosten Unternehmer F").
		want(t, []string{"name=Taxi,qty=1,category=Reisekosten Unternehmer Fahrtkosten"}, noFile)
	// a started key completes to category= without a space
	complete(t, app, "voucher", "create", "--item", "name=Taxi,cat").
		want(t, []string{"name=Taxi,category="}, noFile|cobra.ShellCompDirectiveNoSpace)
	// other keys and line items without enum keys offer nothing
	complete(t, app, "voucher", "create", "--item", "name=").want(t, nil, noFile)
}

// --context completes the context names of config.toml.
func TestCompletionContextNames(t *testing.T) {
	app, _ := apiApp(t)
	writeCfg(t, app, twoCtx)
	complete(t, app, "invoice", "list", "--context", "").want(t, []string{"a", "b"}, noFile)
	complete(t, app, "local", "init", "--context", "").want(t, []string{"a", "b"}, noFile)
	complete(t, app, "context", "use", "").want(t, []string{"a", "b"}, noFile)
	complete(t, app, "context", "use", "a", "").want(t, nil, noFile)
}

// Every flag with allowed values and every command with a positional ID
// of a listable resource has a completion: nothing falls back to file
// names by accident.
func TestCompletionCoversEnumFlagsAndIDs(t *testing.T) {
	app, _ := apiApp(t)
	root := NewRootCmd(app)
	app.registerCompletions(root)
	var ids int
	walk(root, func(c *cobra.Command) {
		c.LocalFlags().VisitAll(func(f *pflag.Flag) {
			// switches take no value; text flags may name files, unless
			// their contract type says otherwise (date filters, review R2-M2)
			typ := f.Value.Type()
			if v := f.Annotations[flagValueType]; len(v) == 1 {
				typ = v[0]
			}
			if f.NoOptDefVal != "" || len(f.Annotations[flagEnum]) == 0 && (typ == "string" || typ == "stringArray") {
				return
			}
			if _, ok := c.GetFlagCompletionFunc(f.Name); !ok {
				t.Errorf("%s --%s: keine Completion", c.CommandPath(), f.Name)
			}
		})
		m := metaOf(c)
		if m.op && strings.Contains(c.Use, "<") && c.ValidArgsFunction == nil {
			t.Errorf("%s: keine Completion der Argumente", c.CommandPath())
		}
		if c.ValidArgsFunction != nil {
			ids++
		}
	})
	if ids < 40 {
		t.Errorf("nur %d Befehle mit Argument-Completion", ids)
	}
}

// completion prints the script of the shell; exactly one known shell.
func TestCompletionCommand(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		app, out, _ := newTestApp(t)
		wantExit(t, app, Run(app, []string{"completion", shell}), 0)
		if s := out.String(); len(s) < 100 || !strings.Contains(s, "papierkram") {
			t.Errorf("%s: %q", shell, s[:min(len(s), 80)])
		}
	}
	for _, args := range [][]string{{"completion"}, {"completion", "tcsh"}, {"completion", "bash", "zsh"}, {"completion", "bash", "--dry-run"}} {
		app, out, errs := newTestApp(t)
		wantExit(t, app, Run(app, args), 2)
		if out.Len() != 0 || !strings.Contains(errs.String(), "completion") {
			t.Errorf("%q: stdout %q, stderr %q", args, out, errs)
		}
	}
	// the help is German and names the credits and the switch
	app, out, _ := newTestApp(t)
	wantExit(t, app, Run(app, []string{"completion", "--help"}), 0)
	for _, want := range []string{"Credit", "completion_lookup = false", "60 Sekunden", "~/.zshrc", "PowerShell",
		`category="`, "Entsperrdialog", "auth logout"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("Hilfe ohne %q", want)
		}
	}
	// the shell names complete
	complete(t, app, "completion", "").want(t, []string{"bash", "zsh", "fish", "powershell"}, noFile)
}

// keychainProbe counts the keychain reads; block makes Get wait until the
// channel is closed (a locked keychain waiting for its unlock dialog).
type keychainProbe struct {
	auth.Store
	gets  atomic.Int32
	block chan struct{}
}

func (s *keychainProbe) Get(ctx string) (string, error) {
	s.gets.Add(1)
	if s.block != nil {
		<-s.block
	}
	return s.Store.Get(ctx)
}

// keychainApp is a context "a" whose token is in the (counting) keychain;
// the base URL points to the mock.
func keychainApp(t *testing.T, cfg string) (*App, *mockapi.Server, *keychainProbe) {
	t.Helper()
	app, srv := apiApp(t)
	app.Env = envMap(map[string]string{"PAPIERKRAM_BASE_URL": srv.URL()})
	store := &keychainProbe{Store: auth.NewMemory()}
	if err := store.Set("a", "test-token-keychain"); err != nil {
		t.Fatal(err)
	}
	app.Keyring = store
	writeCfg(t, app, "active_context = \"a\"\n\n[contexts.a]\nsubdomain = \"firma\"\n"+cfg)
	return app, srv, store
}

// The keychain is read only for a request: never with completion_lookup
// = false, never on a cache hit, once on a miss (review I1).
func TestCompletionKeychainOnlyOnMiss(t *testing.T) {
	app, srv, store := keychainApp(t, "completion_lookup = false\n")
	complete(t, app, "invoice", "describe", "").want(t, nil, noFile)
	complete(t, app, "invoice", "list", "--company", "").want(t, nil, noFile)
	if n := store.gets.Load(); n != 0 {
		t.Errorf("abgeschaltet: %d Schlüsselbund-Zugriffe", n)
	}
	wantRequests(t, srv, 0)

	app, srv, store = keychainApp(t, "")
	srv.On("GET", "/income/invoices", http.StatusOK, invoiceEntries(t))
	complete(t, app, "invoice", "describe", "")
	if n := store.gets.Load(); n != 1 {
		t.Errorf("Miss: %d Schlüsselbund-Zugriffe, erwartet 1", n)
	}
	wantRequests(t, srv, 1)
	if r := complete(t, app, "invoice", "describe", ""); len(r.lines) != 3 {
		t.Errorf("Treffer: %q", r.lines)
	}
	if n := store.gets.Load(); n != 1 {
		t.Errorf("Treffer: %d Schlüsselbund-Zugriffe, erwartet weiterhin 1", n)
	}
	if req := wantRequests(t, srv, 1); req[0].Header.Get("Authorization") != "Bearer test-token-keychain" {
		t.Errorf("Token aus dem Schlüsselbund nicht benutzt")
	}
}

// A keychain that does not answer (locked, waiting for a dialog) is given
// up after the deadline of the lookup: no IDs, no request.
func TestCompletionBlockingKeychain(t *testing.T) {
	defer func(d time.Duration) { completionTimeout = d }(completionTimeout)
	completionTimeout = 100 * time.Millisecond
	app, srv, store := keychainApp(t, "")
	store.block = make(chan struct{})
	defer close(store.block)
	start := time.Now()
	complete(t, app, "invoice", "describe", "").want(t, nil, noFile)
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("wartete %v auf den Schlüsselbund", d)
	}
	wantRequests(t, srv, 0)
}

// A cache entry from the future is a miss, also with a token (review M2).
func TestCompletionFutureCacheIsMiss(t *testing.T) {
	app, srv := apiApp(t)
	srv.On("GET", "/income/invoices", http.StatusOK, invoiceEntries(t))
	writeCompletionCacheAt(t, app, "", srv.URL(), `[{"id":1,"name":"Zukunft"}]`, app.Now().Add(time.Hour))
	if r := complete(t, app, "invoice", "describe", ""); len(r.lines) != 3 {
		t.Errorf("Kandidaten %q", r.lines)
	}
	wantRequests(t, srv, 1)
}

// local init --context X completes the IDs of X; context create --user
// completes none: the account of the new context is unknown (review M3).
func TestCompletionTargetContext(t *testing.T) {
	app, srv := apiApp(t)
	app.Env = envMap(map[string]string{"PAPIERKRAM_BASE_URL": srv.URL()})
	writeCfg(t, app, twoCtx)
	if err := app.Keyring.Set("b", "test-token-b"); err != nil {
		t.Fatal(err)
	}
	srv.On("GET", "/contact/companies", http.StatusOK, listBody(t, map[string]any{"id": 2843, "name": "Muster GmbH"}))
	// the active context a has no token: without --context nothing
	complete(t, app, "local", "init", "--company", "").want(t, nil, noFile)
	complete(t, app, "local", "init", "--context", "b", "--company", "").want(t, []string{"2843\tMuster GmbH"}, noFile)
	if _, err := os.Stat(completionCachePath(app.CacheDir, "b", "company")); err != nil {
		t.Errorf("Cache von b: %v", err)
	}
	// even with a usable --context: the new context's account is unknown
	complete(t, app, "context", "create", "neu", "--context", "b", "--user", "").want(t, nil, noFile)
	wantRequests(t, srv, 1)
}

// A category with a comma matches while its first word is typed; the
// candidate is quoted (review M4).
func TestCompletionItemCategoryWithComma(t *testing.T) {
	app, _ := apiApp(t)
	want := []string{`category="Gas, Strom, Wasser"`}
	complete(t, app, "voucher", "create", "--item", "category=Gas").want(t, want, noFile)
	complete(t, app, "voucher", "create", "--item", `category="Gas`).want(t, want, noFile)
	complete(t, app, "voucher", "create", "--item", "qty=1,category=Zuwendungen").
		want(t, []string{`qty=1,category="Zuwendungen,Spenden steuerl. n. abziehb."`}, noFile)
}

// Number, ID, date and duration flags never offer file names; schema
// completes resources and their actions, api the methods (review M6).
func TestCompletionMetaArgsAndNumbers(t *testing.T) {
	app, srv := apiApp(t)
	complete(t, app, "invoice", "list", "--page", "").want(t, nil, noFile)
	complete(t, app, "invoice", "list", "--limit", "").want(t, nil, noFile)
	complete(t, app, "invoice", "list", "--timeout", "").want(t, nil, noFile)
	complete(t, app, "schema", "inv").want(t, []string{"invoice"}, noFile)
	if r := complete(t, app, "schema", ""); len(r.lines) != len(registry.Resources()) {
		t.Errorf("Ressourcen %q", r.lines)
	}
	complete(t, app, "schema", "invoice", "de").want(t, []string{"delete", "deliver", "describe"}, noFile)
	complete(t, app, "schema", "beleg", "doc").want(t, []string{"document"}, noFile)
	complete(t, app, "schema", "voucher", "document", "").want(t, []string{"delete"}, noFile)
	complete(t, app, "schema", "info", "").want(t, nil, noFile)
	complete(t, app, "api", "").want(t, apiMethods, noFile)
	// in the case the user started typing (review R2-M6)
	complete(t, app, "api", "g").want(t, []string{"get"}, noFile)
	complete(t, app, "api", "P").want(t, []string{"POST", "PUT", "PATCH"}, noFile)
	complete(t, app, "api", "pa").want(t, []string{"patch"}, noFile)
	complete(t, app, "invoice", "list", "--from", "").want(t, nil, noFile)
	complete(t, app, "api", "GET", "").want(t, nil, noFile)
	wantRequests(t, srv, 0)
}

// auth logout, context delete and context rename (old name) remove the
// completion caches of that context, by exact file name: a cache of
// context a-b survives the logout of a.
func TestCompletionCachesRemovedWithContext(t *testing.T) {
	cfg := "active_context = \"a\"\n\n[contexts.a]\nsubdomain = \"firma-a\"\n\n[contexts.a-b]\nsubdomain = \"firma-ab\"\n"
	seed := func(t *testing.T, app *App) {
		t.Helper()
		writeCfg(t, app, cfg)
		for _, ctx := range []string{"a", "a-b"} {
			for _, res := range []string{"invoice", "company", "payment-term"} {
				path := completionCachePath(app.CacheDir, ctx, res)
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
		}
		// a foreign file with the prefix of a stays
		if err := os.WriteFile(filepath.Join(app.CacheDir, "papierkram", "completion-a-notiz.txt"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	left := func(t *testing.T, app *App) []string {
		t.Helper()
		return dirNames(t, filepath.Join(app.CacheDir, "papierkram"))
	}
	ab := []string{"completion-a-b-company.json", "completion-a-b-invoice.json", "completion-a-b-payment-term.json", "completion-a-notiz.txt"}
	for _, args := range [][]string{
		{"auth", "logout", "--context", "a"},
		{"context", "delete", "a"},
		{"context", "rename", "a", "neu"},
	} {
		t.Run(strings.Join(args[:2], " "), func(t *testing.T) {
			app, _, _ := newTestApp(t)
			seed(t, app)
			wantExit(t, app, Run(app, args), 0)
			if got := left(t, app); !slices.Equal(got, ab) {
				t.Errorf("übrig %v, erwartet %v", got, ab)
			}
		})
	}
	// without a cache directory nothing fails
	app, _, _ := newTestApp(t)
	writeCfg(t, app, cfg)
	wantExit(t, app, Run(app, []string{"auth", "logout", "--context", "a-b"}), 0)
}

// Without a cache directory there is no cache: nothing is read or written
// relative to the working directory, logout removes nothing there.
func TestCompletionWithoutCacheDir(t *testing.T) {
	app, srv := apiApp(t)
	app.CacheDir = ""
	srv.On("GET", "/income/invoices", http.StatusOK, invoiceEntries(t))
	complete(t, app, "invoice", "describe", "")
	complete(t, app, "invoice", "describe", "")
	wantRequests(t, srv, 2)
	if _, err := os.Stat("papierkram"); err == nil {
		t.Error("Cache im Arbeitsverzeichnis angelegt")
	}
}

// The script of every shell also completes the short form pk (deb/rpm
// link, Homebrew target): the same registration as papierkram,
// with zsh's #compdef header naming both. The lines are pinned to the
// function names cobra generates and to the line that runs the program
// named on the command line (bash, zsh and PowerShell run "pk __complete";
// fish runs the wrap target "papierkram __complete"); a cobra that renames
// them fails here.
func TestCompletionShortForm(t *testing.T) {
	cases := []struct {
		shell string
		// lines holds exact lines of the script; first the first lines
		lines, first []string
	}{
		{"bash", []string{
			"__start_papierkram()",
			`    requestComp="${words[0]} __complete ${args[*]}"`,
			"    complete -o default -F __start_papierkram papierkram",
			"    complete -o default -o nospace -F __start_papierkram papierkram",
			"    complete -o default -F __start_papierkram pk",
			"    complete -o default -o nospace -F __start_papierkram pk",
		}, nil},
		{"zsh", []string{
			"_papierkram()",
			`    requestComp="${words[1]} __complete ${words[2,-1]}"`,
		}, []string{"#compdef papierkram pk", "compdef _papierkram papierkram pk"}},
		{"fish", []string{
			"function __papierkram_perform_completion",
			`    set -l requestComp "PAPIERKRAM_ACTIVE_HELP=0 $args[1] __complete $args[2..-1] $lastArg"`,
			"complete -c papierkram -e",
			"complete -c pk -w papierkram",
		}, nil},
		{"powershell", []string{
			"[scriptblock]${__papierkramCompleterBlock} = {",
			`    $RequestComp="$Program __complete $Arguments"`,
			"Register-ArgumentCompleter -CommandName 'papierkram' -ScriptBlock ${__papierkramCompleterBlock}",
			"Register-ArgumentCompleter -CommandName 'pk' -ScriptBlock ${__papierkramCompleterBlock}",
		}, nil},
	}
	for _, c := range cases {
		app, out, _ := newTestApp(t)
		wantExit(t, app, Run(app, []string{"completion", c.shell}), 0)
		lines := strings.Split(out.String(), "\n")
		for _, want := range c.lines {
			if !slices.Contains(lines, want) {
				t.Errorf("%s: Zeile fehlt: %q", c.shell, want)
			}
		}
		if len(c.first) > 0 && !slices.Equal(lines[:min(len(lines), len(c.first))], c.first) {
			t.Errorf("%s: Kopf = %q, want %q", c.shell, lines[:min(len(lines), len(c.first))], c.first)
		}
	}
}

// withShortForm refuses a script without the line it builds on (a cobra
// with other names) instead of printing a script without pk: a foreign
// script, and cobra's real script with exactly one of those lines changed.
func TestCompletionShortFormNeedsCobraLines(t *testing.T) {
	for _, shell := range completionShells {
		if _, err := withShortForm(shell, "#compdef andere\nfunction x\n"); err == nil {
			t.Errorf("%s: kein Fehler", shell)
		}
	}
	if _, err := withShortForm("tcsh", "x\n"); err == nil {
		t.Error("tcsh: kein Fehler")
	}
	cases := []struct{ shell, line, changed string }{
		{"bash", "    complete -o default -F __start_papierkram papierkram",
			"    complete -o default -F __start_papierkram_v2 papierkram"},
		{"bash", "    complete -o default -o nospace -F __start_papierkram papierkram",
			"    complete -o default -o nospace -F __start_papierkram_v2 papierkram"},
		{"zsh", "#compdef papierkram", "#compdef papierkram papierkram.exe"},
		{"zsh", "compdef _papierkram papierkram", "compdef _papierkram_v2 papierkram"},
		{"fish", "complete -c papierkram -e", "complete --command papierkram --erase"},
		{"powershell", "Register-ArgumentCompleter -CommandName 'papierkram' -ScriptBlock ${__papierkramCompleterBlock}",
			"Register-ArgumentCompleter -CommandName 'papierkram' -ScriptBlock ${__papierkramCompleterBlockV2}"},
	}
	for _, c := range cases {
		var buf strings.Builder
		app, _, _ := newTestApp(t)
		root := NewRootCmd(app)
		var err error
		switch c.shell {
		case "bash":
			err = root.GenBashCompletionV2(&buf, true)
		case "zsh":
			err = root.GenZshCompletion(&buf)
		case "fish":
			err = root.GenFishCompletion(&buf, true)
		default:
			err = root.GenPowerShellCompletionWithDesc(&buf)
		}
		if err != nil {
			t.Fatal(err)
		}
		script := buf.String()
		if _, err := withShortForm(c.shell, script); err != nil {
			t.Fatalf("%s: unverändertes Skript abgelehnt: %v", c.shell, err)
		}
		lines := strings.Split(script, "\n")
		i := slices.Index(lines, c.line)
		if i < 0 {
			t.Fatalf("%s: Zeile %q fehlt im Skript von cobra", c.shell, c.line)
		}
		lines[i] = c.changed
		if _, err := withShortForm(c.shell, strings.Join(lines, "\n")); err == nil {
			t.Errorf("%s: geänderte Zeile %q nicht abgelehnt", c.shell, c.line)
		}
	}
}
