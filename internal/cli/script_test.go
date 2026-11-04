package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rogpeppe/go-internal/testscript"

	"github.com/Padrio/papierkram-cli/internal/auth"
	"github.com/Padrio/papierkram-cli/internal/testutil/mockapi"
	"github.com/Padrio/papierkram-cli/internal/testutil/modroot"
)

// Scenario tests (spec 11, plan task 42): testscript runs the txtar files
// in testdata/script. Each script gets its own spec-validating mock API;
// the CLI is this test binary, started as a separate process.

// scriptBin is the name under which TestMain installs the test binary as
// the CLI. Scripts never call it directly: the papierkram command wraps it
// with the checks every run needs.
const scriptBin = "papierkram-script-bin"

// scriptToken is the token of every script; it must never show up in an
// output (checked for the prefix test-token- after each run).
const scriptToken = "test-token-script"

// scriptNow is the default PK_TEST_NOW of the scripts (a Wednesday).
const scriptNow = "2026-11-04T10:00:00+01:00"

func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){
		scriptBin: func() { os.Exit(Run(scriptApp(), os.Args[1:])) },
	})
}

// scriptApp builds the App of one CLI run in a script from the process
// environment (the script's env). Only this test binary reads the test
// hooks PK_TEST_TTY (1: stdin and stdout are terminals) and PK_TEST_NOW
// (fixed time, RFC 3339); the production binary never does
// (TestTestHooksOnlyInTests). Tokens live in an in-memory store, config
// and cache only in the directories of the env: nothing touches the real
// keychain or home directory.
func scriptApp() *App {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		fatalScript("Zeitzone: %v", err)
	}
	now := time.Now
	if v := os.Getenv("PK_TEST_NOW"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			fatalScript("PK_TEST_NOW: %v", err)
		}
		now = func() time.Time { return t }
	}
	tty := os.Getenv("PK_TEST_TTY") == "1"
	return &App{
		In: os.Stdin, Out: os.Stdout, Err: os.Stderr,
		Env:       os.Getenv,
		Now:       now,
		Location:  loc,
		StdinTTY:  tty,
		StdoutTTY: tty,
		// the env values take precedence; these are the fallbacks
		ConfigDir:    os.Getenv("PAPIERKRAM_CONFIG_DIR"),
		CacheDir:     os.Getenv("PAPIERKRAM_CACHE_DIR"),
		HTTPClient:   http.DefaultClient,
		Keyring:      auth.NewMemory(),
		Getwd:        os.Getwd,
		ReadPassword: func() ([]byte, error) { return nil, errors.New("kein Terminal im Skripttest") },
		Sleep:        func(time.Duration) {},
	}
}

// fatalScript ends a CLI run of a script on a broken test setup.
func fatalScript(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Skripttest: "+format+"\n", args...)
	os.Exit(70)
}

// scriptState is the per-script state of the custom commands.
type scriptState struct {
	srv  *mockapi.Server
	exit int // exit code of the last papierkram run, -1 before the first
}

type scriptStateKey struct{}

func stateOf(ts *testscript.TestScript) *scriptState {
	st, ok := ts.Value(scriptStateKey{}).(*scriptState)
	if !ok {
		ts.Fatalf("kein Skriptzustand (Setup fehlt)")
	}
	return st
}

func TestScripts(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "script", "*.txtar"))
	if err != nil || len(files) == 0 {
		t.Fatalf("keine Skripte: %v", err)
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(raw, []byte(scriptBin)) {
			t.Errorf("%s ruft %s direkt auf; nur über den Befehl papierkram (Prüfungen je Lauf)", f, scriptBin)
		}
	}
	testscript.Run(t, testscript.Params{
		Dir:                 filepath.Join("testdata", "script"),
		Setup:               scriptSetup,
		RequireExplicitExec: true,
		RequireUniqueNames:  true,
		Cmds: map[string]func(ts *testscript.TestScript, neg bool, args []string){
			"papierkram": cmdPapierkram,
			"pkexit":     cmdPKExit,
			"mock":       cmdMock,
			"mockredir":  cmdMockRedirect,
			"mockreq":    cmdMockReq,
			"mockbody":   cmdMockBody,
			"jsonschema": cmdJSONSchema,
		},
	})
}

// scriptSetup starts the mock API of a script and sets its environment:
// API URL and token, config and cache below $WORK, fixed time. A spec
// violation fails the script (after each run, and at the end).
func scriptSetup(env *testscript.Env) error {
	tb, ok := env.T().(testing.TB)
	if !ok {
		return errors.New("testscript.Env.T ist kein testing.TB")
	}
	srv := mockapi.New(tb, mockapi.WithoutAutoFail())
	env.Values[scriptStateKey{}] = &scriptState{srv: srv, exit: -1}
	env.Setenv("PAPIERKRAM_BASE_URL", srv.URL())
	env.Setenv("PAPIERKRAM_TOKEN", scriptToken)
	env.Setenv("PAPIERKRAM_CONFIG_DIR", filepath.Join(env.WorkDir, ".config"))
	env.Setenv("PAPIERKRAM_CACHE_DIR", filepath.Join(env.WorkDir, ".cache"))
	env.Setenv("PK_TEST_NOW", scriptNow)
	// Coverage runs (go test -cover) pass GOCOVERDIR on to the CLI runs.
	// Parallel scripts writing their coverage files into one directory
	// race, and the CLI prints the failure on stderr; each script gets a
	// directory of its own (the data of the CLI runs is not merged).
	if os.Getenv("GOCOVERDIR") != "" {
		dir := filepath.Join(env.WorkDir, ".gocoverdir")
		if err := os.Mkdir(dir, 0o700); err != nil {
			return err
		}
		env.Setenv("GOCOVERDIR", dir)
	}
	env.Defer(func() {
		if v := srv.Violations(); len(v) > 0 {
			tb.Errorf("Spec-Verstöße:\n%s", strings.Join(v, "\n"))
		}
	})
	return nil
}

// cmdPapierkram runs the CLI: papierkram args... (! papierkram for a
// non-zero exit; pkexit checks the exact code). Every run fails the script
// when stdout or stderr contains a test token or the mock saw a request
// that violates the spec.
func cmdPapierkram(ts *testscript.TestScript, neg bool, args []string) {
	st := stateOf(ts)
	before := len(st.srv.Violations())
	err := ts.Exec(scriptBin, args...)
	st.exit = 0
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			ts.Fatalf("papierkram nicht gestartet: %v", err)
		}
		st.exit = ee.ExitCode()
	}
	for _, name := range []string{"stdout", "stderr"} {
		if strings.Contains(ts.ReadFile(name), "test-token-") {
			ts.Fatalf("Token in %s", name)
		}
	}
	if v := st.srv.Violations(); len(v) > before {
		ts.Fatalf("Spec-Verstöße:\n%s", strings.Join(v[before:], "\n"))
	}
	switch {
	case neg && st.exit == 0:
		ts.Fatalf("papierkram: unerwarteter Erfolg (Exit 0)")
	case !neg && st.exit != 0:
		ts.Fatalf("papierkram: Exit %d", st.exit)
	}
}

// cmdPKExit checks the exit code of the last papierkram run: pkexit N.
func cmdPKExit(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 1 {
		ts.Fatalf("Verwendung: pkexit N")
	}
	want, err := strconv.Atoi(args[0])
	ts.Check(err)
	if got := stateOf(ts).exit; got != want {
		ts.Fatalf("Exit %d, erwartet %d", got, want)
	}
}

// cmdMock overrides the answer of the mock: mock METHOD PATH STATUS
// [bodyfile]. PATH is concrete or a spec template, without /api/v1.
func cmdMock(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) < 3 || len(args) > 4 {
		ts.Fatalf("Verwendung: mock METHOD PATH STATUS [bodyfile]")
	}
	status, err := strconv.Atoi(args[2])
	ts.Check(err)
	var body []byte
	if len(args) == 4 {
		body = []byte(ts.ReadFile(args[3]))
	}
	stateOf(ts).srv.On(args[0], args[1], status, body)
}

// cmdMockRedirect lets the mock answer with a redirect to another path of
// the mock API: mockredir METHOD PATH STATUS TARGET (both without /api/v1).
func cmdMockRedirect(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 4 {
		ts.Fatalf("Verwendung: mockredir METHOD PATH STATUS TARGET")
	}
	status, err := strconv.Atoi(args[2])
	ts.Check(err)
	srv := stateOf(ts).srv
	srv.On(args[0], args[1], status, nil, "Location", srv.URL()+args[3])
}

// cmdMockReq checks the number of requests the mock received: mockreq
// COUNT [METHOD [PATH]] (PATH concrete, without /api/v1).
func cmdMockReq(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) < 1 || len(args) > 3 {
		ts.Fatalf("Verwendung: mockreq COUNT [METHOD [PATH]]")
	}
	want, err := strconv.Atoi(args[0])
	ts.Check(err)
	var got []string
	for _, r := range stateOf(ts).srv.Requests() {
		if len(args) > 1 && r.Method != args[1] || len(args) > 2 && r.Path != args[2] {
			continue
		}
		got = append(got, r.Method+" "+r.Path)
	}
	if len(got) != want {
		ts.Fatalf("%d Requests (%s), erwartet %d", len(got), strings.Join(got, ", "), want)
	}
}

// cmdMockBody compares the body of the Nth request (from 1) with a file,
// both as JSON (key order and formatting do not matter, numbers exact):
// mockbody N file.
func cmdMockBody(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 2 {
		ts.Fatalf("Verwendung: mockbody N file")
	}
	n, err := strconv.Atoi(args[0])
	ts.Check(err)
	reqs := stateOf(ts).srv.Requests()
	if n < 1 || n > len(reqs) {
		ts.Fatalf("Request %d fehlt (%d Requests)", n, len(reqs))
	}
	got, err := canonicalJSON(reqs[n-1].Body)
	if err != nil {
		ts.Fatalf("Body von Request %d: %v", n, err)
	}
	want, err := canonicalJSON([]byte(ts.ReadFile(args[1])))
	if err != nil {
		ts.Fatalf("%s: %v", args[1], err)
	}
	if got != want {
		ts.Fatalf("Body von Request %d:\n%s\nerwartet:\n%s", n, got, want)
	}
}

// canonicalJSON re-encodes raw with sorted keys and exact numbers.
func canonicalJSON(raw []byte) (string, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return "", fmt.Errorf("kein JSON: %w (%q)", err, raw)
	}
	out, err := json.Marshal(v)
	return string(out), err
}

// cmdJSONSchema validates a JSON document against docs/schemas/<name>.json:
// jsonschema [-last] file name. file may be stdout or stderr of the last
// run; -last takes its last non-empty line (the structured error on
// stderr, spec 10.4).
func cmdJSONSchema(ts *testscript.TestScript, neg bool, args []string) {
	last := len(args) > 0 && args[0] == "-last"
	if last {
		args = args[1:]
	}
	if neg || len(args) != 2 {
		ts.Fatalf("Verwendung: jsonschema [-last] file name")
	}
	raw := ts.ReadFile(args[0])
	if last {
		raw = lastLine(raw)
	}
	ts.Check(checkContract(args[1], []byte(raw)))
}

// literalJoin is the boundary of two concatenated string literals
// ("PK_" + "TEST_", also with backquotes and across lines).
var literalJoin = regexp.MustCompile("[\"`]\\s*\\+\\s*[\"`]")

// hookNames are the names that betray a test hook: the prefix (after
// joining concatenated literals) and the hook names without it, which
// also catch "PK_" + suffix with the suffix from elsewhere.
var hookNames = []string{"PK_TEST_", "TEST_TTY", "TEST_NOW"}

// The test hooks PK_TEST_* exist only in this test binary: no Go file
// outside tests may read (or name) them, not even as concatenated
// literals.
func TestTestHooksOnlyInTests(t *testing.T) {
	root := modroot.Find(t)
	var found []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		joined := literalJoin.ReplaceAll(raw, nil)
		for _, name := range hookNames {
			if bytes.Contains(joined, []byte(name)) {
				found = append(found, path+" ("+name+")")
				break
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) > 0 {
		t.Errorf("Test-Hooks PK_TEST_ außerhalb von Tests: %s", strings.Join(found, ", "))
	}
}
