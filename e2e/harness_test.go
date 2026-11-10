//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Padrio/papierkram-cli/internal/auth"
	"github.com/Padrio/papierkram-cli/internal/cli"
	"github.com/Padrio/papierkram-cli/internal/config"
	"github.com/Padrio/papierkram-cli/internal/e2eguard"
)

// liveSuite is the state of one live run against the sandbox.
type liveSuite struct {
	sub, token string
	runID      string
	prefix     string // pk-e2e-<runid>-
	red        e2eguard.Redactor
	meter      *e2eguard.Meter
	coverage   *e2eguard.Coverage // operations reached (reported by TestMain)
	tr         *e2eguard.Transport
	loc        *time.Location
	// mainDir holds config and cache of the CLI runs of TestMain
	// (cleanup); tests use t.TempDir().
	mainDir string

	// created lists every object of this run for the cleanup at the end,
	// in creation order.
	created []object
	// fixtures memoizes shared objects by key; failed records fixtures
	// whose creation failed, so later tests fail fast instead of retrying.
	fixtures map[string]map[string]any
	failed   map[string]bool
	// exhausted is set once the budget or the quota stopped a test; all
	// later tests skip, and the coverage gate does not apply.
	exhausted bool
	// quota is set by the first CLI run of the tests that hit the monthly
	// quota (HTTP 429, exit 6: e2eguard.QuotaHit), quotaCleanup by one of
	// the cleanup; cleaning is set when the cleanup starts. After a hit
	// execAt sends nothing more in that phase: the cleanup still tries
	// once after a hit in the tests. The tests run one after another (no
	// t.Parallel), so plain fields suffice.
	quota, quotaCleanup, cleaning bool
	// deviations are the ABWEICHUNG lines of the run (deviatef).
	deviations []string
	// sent holds the request body of every create by "resource/id", so
	// a check can assert what was actually sent.
	sent map[string][]byte
	// permanent are the companies named e2eguard.PermanentCustomer that
	// TestE2E_CleanupLeftovers saw (never touched there); permanentSeen
	// is set once it listed all companies, so permanentCustomer needs no
	// list of its own.
	permanent     []map[string]any
	permanentSeen bool
}

// object is something the run created. name is what Owned checks: name,
// first_name of a person, comments of a time entry.
type object struct {
	resource string // CLI resource: company, person, invoice, …
	id       int64
	company  int64 // person: its company
	name     string
	// followUpOf is the id of the object of this run whose cancel or
	// reverse answered with this object (cancelMeasured); 0 otherwise.
	followUpOf int64
	// followUpType is the type of that answer (e2eguard.JudgeFollowUp);
	// remove touches a follow-up only when it is the type of resource.
	followUpType string
	// archiveOnly makes remove only archive the object (1 credit), never
	// delete or cancel it: the permanent customer, numbered (delivered)
	// invoices and estimates and the cancellation invoice, which the API
	// cannot delete (spec v0.2 §2; each further attempt costs credits
	// without effect).
	archiveOnly bool
}

// newLiveSuite prepares a run. It sends nothing.
func newLiveSuite(getenv func(string) string, red e2eguard.Redactor) (*liveSuite, error) {
	sub := getenv(e2eguard.EnvSubdomain)
	norm, err := config.NormalizeSubdomain(sub, e2eguard.EnvSubdomain)
	if err != nil {
		return nil, err
	}
	runID, err := e2eguard.RunID(getenv, nil)
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "pk-e2e-")
	if err != nil {
		return nil, err
	}
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		loc = time.Local
	}
	meter := e2eguard.NewMeter(creditBudget, cleanupReserve)
	coverage := e2eguard.NewCoverage()
	return &liveSuite{
		sub:      sub,
		token:    getenv(e2eguard.EnvToken),
		runID:    runID,
		prefix:   e2eguard.Prefix(runID),
		red:      red,
		meter:    meter,
		coverage: coverage,
		tr:       &e2eguard.Transport{Base: e2eguard.DirectTransport(), Scheme: "https", Host: norm + ".papierkram.de", Meter: meter, Coverage: coverage},
		loc:      loc,
		mainDir:  dir,
		fixtures: map[string]map[string]any{},
		failed:   map[string]bool{},
		sent:     map[string][]byte{},
	}, nil
}

// result is one CLI run.
type result struct {
	code           int
	stdout, stderr string
	// exchanges are the HTTP requests of this run.
	exchanges []e2eguard.Exchange
	// refused is set when execAt did not run the CLI because the quota
	// was hit before (code 6, no request).
	refused bool
}

// exec runs the real CLI (cli.Run) with config and cache below dir. The
// environment of the CLI holds only the subdomain and token of the
// sandbox and these directories: no user config, no keychain (in-memory
// store), no PAPIERKRAM_BASE_URL or other variable of the caller.
func (s *liveSuite) exec(dir, stdin string, args ...string) result {
	return s.execAt(dir, time.Now, stdin, args...)
}

// execAt is exec with the clock now of the CLI (App.Now): the local timer
// (time start, status, stop, discard) reads it, and its state file lies
// below <dir>/cache/papierkram/. A run that hits the quota (HTTP 429, exit
// 6) marks the suite (markQuota); once marked, execAt runs nothing more in
// that phase and returns code 6 with refused set.
func (s *liveSuite) execAt(dir string, now func() time.Time, stdin string, args ...string) result {
	if s.quotaNow() {
		return result{code: 6, stderr: quotaRefused + "\n", refused: true}
	}
	var out, errb bytes.Buffer
	app := s.newApp(dir, now, stdin, &out, &errb)
	n := s.tr.Len()
	code := cli.Run(app, append([]string{"--no-input"}, args...))
	r := result{code: code, stdout: out.String(), stderr: errb.String(), exchanges: s.tr.Since(n)}
	if e2eguard.QuotaHit(r.code, r.exchanges) {
		s.markQuota()
	}
	return r
}

// newApp is the App of a CLI run in dir (execAt, mcpSession): only the
// subdomain and token of the sandbox and config and cache below dir in
// its environment, the transport of the suite, an in-memory keychain.
func (s *liveSuite) newApp(dir string, now func() time.Time, stdin string, out, errw io.Writer) *cli.App {
	cfg, cache := filepath.Join(dir, "config"), filepath.Join(dir, "cache")
	env := map[string]string{
		"PAPIERKRAM_SUBDOMAIN":  s.sub,
		"PAPIERKRAM_TOKEN":      s.token,
		"PAPIERKRAM_CONFIG_DIR": cfg,
		"PAPIERKRAM_CACHE_DIR":  cache,
	}
	return &cli.App{
		In: strings.NewReader(stdin), Out: out, Err: errw,
		Env:          func(k string) string { return env[k] },
		Now:          now,
		Location:     s.loc,
		ConfigDir:    cfg,
		CacheDir:     cache,
		HTTPClient:   &http.Client{Transport: s.tr},
		Keyring:      auth.NewMemory(),
		Getwd:        func() (string, error) { return dir, nil },
		ReadPassword: func() ([]byte, error) { return nil, errors.New("kein Terminal in der E2E-Suite") },
		Sleep:        time.Sleep,
	}
}

// mcpSession is a client session of the MCP server of the CLI
// (cli.NewMCPServer, default options: read tools only, no files
// directory), in-process over the in-memory transport of the SDK. The
// server runs every tool call with the App of execAt in dir, so its
// requests go through s.tr (meter, coverage, the rules of the transport)
// like those of exec. stderr of the start must not contain the token.
// After a hit of the quota it starts nothing (skip).
func (s *liveSuite) mcpSession(t *testing.T, dir string) *mcp.ClientSession {
	t.Helper()
	if s.quotaNow() {
		s.skipf(t, "Kontingent erschöpft (HTTP 429): kein MCP-Server gestartet, Test übersprungen")
	}
	var out, errb bytes.Buffer
	srv, err := cli.NewMCPServer(s.newApp(dir, time.Now, "", &out, &errb), cli.MCPOptions{})
	if s.red.Contains(out.String()) || s.red.Contains(errb.String()) {
		s.errorf(t, "Ausgabe beim Start des MCP-Servers enthält das Token")
	}
	if err != nil {
		s.fatalf(t, "MCP-Server nicht gestartet: %v", err)
	}
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		s.fatalf(t, "MCP-Server nicht verbunden: %v", err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "pk-e2e", Version: "v0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		_ = ss.Close()
		s.fatalf(t, "MCP-Client nicht verbunden: %v", err)
	}
	t.Cleanup(func() {
		_ = cs.Close()
		_ = ss.Wait()
		// the server is done: what it wrote to stderr since the start
		// (hints, --debug) must not contain the token either
		if s.red.Contains(out.String()) || s.red.Contains(errb.String()) {
			s.errorf(t, "Ausgabe des MCP-Servers enthält das Token")
		}
	})
	return cs
}

// quotaRefused is the stderr of a CLI run execAt did not start.
const quotaRefused = "e2e: Kontingent erschöpft (HTTP 429): nicht ausgeführt"

// quotaNow reports whether the quota was hit in the current phase (tests
// or cleanup).
func (s *liveSuite) quotaNow() bool {
	if s.cleaning {
		return s.quotaCleanup
	}
	return s.quota
}

// markQuota records a hit of the quota in the current phase and logs it
// once (quotaNotice): a hit in the tests stops all later tests like an
// exhausted budget (AGENTS.md rule 7: never retry a 429).
func (s *liveSuite) markQuota() {
	if s.quotaNow() {
		return
	}
	if s.cleaning {
		s.quotaCleanup = true
	} else {
		s.quota = true
	}
	s.exhausted = true
	s.quotaNotice()
}

// stopReason is why no further test runs: the quota or the budget.
func (s *liveSuite) stopReason() string {
	if s.quota {
		return "Kontingent erschöpft (HTTP 429)"
	}
	return "Credit-Budget für Tests erschöpft"
}

// run is exec in a test: own directories, and the CLI output must never
// contain the token (hard: the CLI promises that).
func (s *liveSuite) run(t *testing.T, stdin string, args ...string) result {
	t.Helper()
	return s.runIn(t, t.TempDir(), time.Now, stdin, args...)
}

// runIn is run with the directory dir and the clock now (execAt): runs
// that share dir share the timer of the CLI.
func (s *liveSuite) runIn(t *testing.T, dir string, now func() time.Time, stdin string, args ...string) result {
	t.Helper()
	r := s.execAt(dir, now, stdin, args...)
	if r.refused {
		// neutral: a local command (time start, local init) sends
		// nothing either way (review F2-M4)
		s.skipf(t, "Kontingent erschöpft (HTTP 429): %q nicht ausgeführt, Test übersprungen", commandOf(args))
	}
	if s.red.Contains(r.stdout) || s.red.Contains(r.stderr) {
		// only the command (resource and action): no ids or values
		s.errorf(t, "CLI-Ausgabe von %q enthält das Token", commandOf(args))
	}
	return r
}

// commandOf is the command of a CLI run: its first two arguments
// (resource and action, e.g. "invoice describe"), never its ids or values.
func commandOf(args []string) string {
	return strings.Join(args[:min(2, len(args))], " ")
}

// ok runs args with -o json, requires exit 0 and decodes stdout.
func (s *liveSuite) ok(t *testing.T, stdin string, args ...string) map[string]any {
	t.Helper()
	r := s.run(t, stdin, append(args, "-o", "json")...)
	if r.code != 0 {
		s.quotaSkip(t, r, args)
		s.fatalf(t, "papierkram %s: Exit %d\n%s", strings.Join(args, " "), r.code, r.stderr)
	}
	return s.decode(t, r.stdout)
}

// quotaSkip skips the test when r hit the monthly quota
// (e2eguard.QuotaHit): like need after the hit, no failure of the
// command (review F2-M2). The caller registers what was created first.
func (s *liveSuite) quotaSkip(t *testing.T, r result, args []string) {
	t.Helper()
	if e2eguard.QuotaHit(r.code, r.exchanges) {
		s.skipf(t, "Kontingent erschöpft (HTTP 429) bei %q, Exit %d: Test übersprungen", commandOf(args), r.code)
	}
}

// tryDecode is raw as a JSON object with exact numbers; nil when it is
// none (no failure: for output that may be missing).
func tryDecode(raw string) map[string]any {
	var m map[string]any
	d := json.NewDecoder(strings.NewReader(raw))
	d.UseNumber()
	if d.Decode(&m) != nil {
		return nil
	}
	return m
}

func (s *liveSuite) decode(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	d := json.NewDecoder(strings.NewReader(raw))
	d.UseNumber()
	if err := d.Decode(&m); err != nil {
		s.fatalf(t, "keine JSON-Ausgabe: %v\n%s", err, raw)
	}
	return m
}

// need skips the test when n more credits would cut into the cleanup
// reserve (e2eguard.Meter.Fits); all later tests skip too, so the cleanup
// at the end always gets to run. Once a run hit the monthly quota (HTTP
// 429, s.quota) it skips every later test as well, before the budget is
// looked at.
func (s *liveSuite) need(t *testing.T, n int) {
	t.Helper()
	if s.quota {
		s.skipf(t, "Kontingent erschöpft (HTTP 429): Test übersprungen")
	}
	if s.exhausted {
		s.skipf(t, "Credit-Budget für Tests erschöpft (%d von %d verbraucht, %d fürs Aufräumen reserviert); Test übersprungen",
			s.meter.Spent(), s.meter.Budget(), s.meter.Reserved())
	}
	if !s.meter.Fits(n) {
		s.exhausted = true
		s.skipf(t, "Credit-Budget: %d nötig, verbraucht %d von %d, %d fürs Aufräumen reserviert; dieser und alle folgenden Tests übersprungen",
			n, s.meter.Spent(), s.meter.Budget(), s.meter.Reserved())
	}
}

// sentBody is the decoded request body of the create of resource id;
// nil when it was not recorded.
func (s *liveSuite) sentBody(t *testing.T, resource string, id int64) map[string]any {
	t.Helper()
	raw, ok := s.sent[resource+"/"+ids(id)]
	if !ok {
		return nil
	}
	return s.decode(t, string(raw))
}

// track registers an object of this run for the cleanup.
func (s *liveSuite) track(o object) { s.created = append(s.created, o) }

// archiveOnly marks the registered object resource id as archiveOnly (a
// delivered document: it has a number now and cannot be deleted).
func (s *liveSuite) archiveOnly(resource string, id int64) {
	for i := range s.created {
		if s.created[i].resource == resource && s.created[i].id == id {
			s.created[i].archiveOnly = true
		}
	}
}

// tracked reports whether this run registered resource id.
func (s *liveSuite) tracked(resource string, id int64) bool {
	for _, c := range s.created {
		if c.resource == resource && c.id == id {
			return true
		}
	}
	return false
}

// fixture returns the shared object key, creating it with mk on first use.
// Once the budget is exhausted it skips; a creation that failed makes
// later uses fail fast.
func (s *liveSuite) fixture(t *testing.T, key string, mk func() map[string]any) map[string]any {
	t.Helper()
	if v, ok := s.fixtures[key]; ok {
		return v
	}
	if s.exhausted {
		s.skipf(t, "%s: Vorbedingung %s nicht angelegt", s.stopReason(), key)
	}
	if s.failed[key] {
		s.fatalf(t, "Vorbedingung %s ist bereits gescheitert", key)
	}
	done := false
	defer func() {
		// only a failure marks the fixture; a skip (budget) does not, so
		// later tests skip as well instead of failing
		if !done && t.Failed() {
			s.failed[key] = true
		}
	}()
	v := mk()
	s.fixtures[key] = v
	done = true
	return v
}

// checkContract checks stdout, the -o json output of a CLI run, against
// the strict contract docs/schemas/<schema>.json. It returns the
// violation ("verletzt <schema>.json bei /<path>: <rule>", or that stdout
// is no JSON), "" when stdout fulfils it; the caller decides whether that
// is a failure or a measurement. The violation names only the JSON path
// and the rule, never values. A contract that cannot be read is a failure
// of the suite (errorf, "" returned).
func (s *liveSuite) checkContract(t *testing.T, schema, stdout string) string {
	t.Helper()
	path := "../docs/schemas/" + schema + ".json"
	raw, err := os.ReadFile(path)
	if err != nil {
		s.errorf(t, "Vertrag %s nicht lesbar: %v", path, err)
		return ""
	}
	var sc openapi3.Schema
	if err := json.Unmarshal(raw, &sc); err != nil {
		s.errorf(t, "Vertrag %s ist kein Schema-Objekt: %v", path, err)
		return ""
	}
	if err := sc.Validate(context.Background()); err != nil {
		s.errorf(t, "Vertrag %s ist kein gültiges OpenAPI-3-Schema: %v", path, err)
		return ""
	}
	var doc any
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		return "ist keine JSON-Ausgabe, Vertrag " + schema + ".json nicht prüfbar"
	}
	err = sc.VisitJSON(doc)
	var se *openapi3.SchemaError
	switch {
	case err == nil:
		return ""
	case errors.As(err, &se):
		return fmt.Sprintf("verletzt %s.json bei /%s: %s", schema, strings.Join(se.JSONPointer(), "/"), se.Reason)
	default:
		return fmt.Sprintf("verletzt %s.json (%T)", schema, err)
	}
}

// --- JSON helpers -----------------------------------------------------------

// get walks m along keys (objects only); nil when a step is missing.
func get(v any, keys ...string) any {
	for _, k := range keys {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[k]
	}
	return v
}

func str(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	default:
		return fmt.Sprint(x)
	}
}

func idOf(v any) int64 {
	n, _ := strconv.ParseInt(str(get(v, "id")), 10, 64)
	return n
}

func num(v any) (float64, bool) {
	f, err := strconv.ParseFloat(str(v), 64)
	return f, err == nil && v != nil
}

func ids(id int64) string { return strconv.FormatInt(id, 10) }

// asSlice is v as a JSON array (nil otherwise).
func asSlice(v any) []any {
	a, _ := v.([]any)
	return a
}

// entries are the list entries of a list output.
func entries(m map[string]any) []map[string]any {
	raw, _ := m["entries"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, e := range raw {
		if em, ok := e.(map[string]any); ok {
			out = append(out, em)
		}
	}
	return out
}

// jsonType is the JSON type name of a decoded value.
func jsonType(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case json.Number:
		return "number"
	case bool:
		return "boolean"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return fmt.Sprintf("%T", v)
}

// firstExchange is the first exchange of r with method; nil when there is
// none.
func firstExchange(r result, method string) *e2eguard.Exchange {
	for i := range r.exchanges {
		if r.exchanges[i].Method == method {
			return &r.exchanges[i]
		}
	}
	return nil
}

// lastExchange is the last exchange of r with method and a path ending in
// suffix; nil when there is none.
func lastExchange(r result, method, suffix string) *e2eguard.Exchange {
	for i := len(r.exchanges) - 1; i >= 0; i-- {
		e := r.exchanges[i]
		if e.Method == method && strings.HasSuffix(strings.TrimRight(e.Path, "/"), suffix) {
			return &e
		}
	}
	return nil
}

// apiMessage is the message of an API error body, or the body itself.
func apiMessage(body []byte) string {
	var e struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &e) == nil && e.Message != "" {
		return e.Message
	}
	if len(body) > 300 {
		body = body[:300]
	}
	return string(body)
}

// --- cleanup ----------------------------------------------------------------

// cleanupDocs are the resources whose objects may be final (numbered):
// they cannot always be deleted, then they are cancelled and archived.
var cleanupDocs = map[string]bool{"invoice": true, "estimate": true, "voucher": true}

// noArchive are the resources without archive.
var noArchive = map[string]bool{"person": true}

// archivedForGood ends what remove returns for an archiveOnly object that
// is archived now.
const archivedForGood = "archiviert (dauerhaft)"

// archivedResult reports whether what (of remove) ends with archiving: the
// object is not gone but archived.
func archivedResult(what string) bool {
	return strings.HasSuffix(what, archivedForGood) || strings.HasSuffix(what, ", archiviert")
}

// remove deletes o; where that is not possible it cancels (documents) and
// archives it (spec 11.1); an archiveOnly object it only archives (never
// delete or cancel). It touches o only when its name is owned by the
// suite, or when o is a follow-up object (followUpOf): the API may name
// that one itself, but its id was read verbatim from the answer to a
// cancel or reverse of an object of this run, or it is the only new
// invoice of the permanent customer after the cancel of an invoice of
// this run (findCancellation; the target checked by ownTarget), its type is
// that of the resource (followUpType, checked here again besides the two
// places that register a follow-up, followUp and findCancellation), and
// only that id is touched. A
// follow-up is never cancelled (it is a cancellation or reverse entry
// itself; its cancel could create yet another object), only deleted or
// archived. After a hit of the quota (HTTP 429) it sends nothing more. It
// returns what happened and false when o is still active.
func (s *liveSuite) remove(dir string, o object) (string, bool) {
	switch {
	case s.quotaNow():
		return "nicht angefasst: Kontingent erschöpft (HTTP 429)", false
	case o.followUpOf != 0 && !e2eguard.IsFollowUpType(o.resource, o.followUpType):
		return fmt.Sprintf("nicht angefasst: Folgeobjekt mit Typ %q, nicht dem Typ von %s", o.followUpType, o.resource), false
	case o.followUpOf != 0 && !s.ownTarget(o):
		return fmt.Sprintf("nicht angefasst: Folgeobjekt von %s %d, das dieser Lauf nicht angelegt hat", o.resource, o.followUpOf), false
	case o.followUpOf == 0 && !e2eguard.Owned(o.name):
		return fmt.Sprintf("nicht angefasst: Name %q ohne Präfix %s", o.name, e2eguard.NamePrefix), false
	}
	id := ids(o.id)
	if o.archiveOnly {
		return s.archiveForGood(dir, o)
	}
	extra := []string{"--yes", "-o", "json"}
	if o.resource == "person" {
		extra = append(extra, "--company", ids(o.company))
	}
	do := func(action string) result {
		return s.exec(dir, "", append([]string{o.resource, action, id}, extra...)...)
	}
	r := do("delete")
	if r.code == 0 {
		return "gelöscht", true
	}
	steps := []string{fmt.Sprintf("delete Exit %d", r.code)}
	quotaStopped := func() (string, bool) {
		return strings.Join(steps, ", ") + ": Kontingent erschöpft (HTTP 429), abgebrochen", false
	}
	if s.quotaNow() {
		return quotaStopped()
	}
	if cleanupDocs[o.resource] && o.followUpOf == 0 {
		if c := do("cancel"); c.code == 0 {
			steps = append(steps, "storniert")
			if d := do("delete"); d.code == 0 {
				return strings.Join(append(steps, "gelöscht"), ", "), true
			}
		} else {
			steps = append(steps, fmt.Sprintf("cancel Exit %d", c.code))
		}
		if s.quotaNow() {
			return quotaStopped()
		}
	}
	if noArchive[o.resource] {
		return strings.Join(append(steps, "kein archive verfügbar"), ", ") + ": " + strings.TrimSpace(r.stderr), false
	}
	a := s.exec(dir, "", o.resource, "archive", id, "-o", "json")
	if a.code == 0 {
		return strings.Join(append(steps, "archiviert"), ", "), true
	}
	return strings.Join(append(steps, fmt.Sprintf("archive Exit %d", a.code)), ", ") + ": " + strings.TrimSpace(a.stderr), false
}

// archiveForGood archives the archiveOnly object o (1 credit). When
// archive ends with an exit other than 0 (e.g. the object is archived
// already, or exit 12), one describe (1 credit more, within the 2 the
// reserve counts per such object) decides: record_state "archived" is a
// success. After a hit of the quota nothing more is sent.
func (s *liveSuite) archiveForGood(dir string, o object) (string, bool) {
	id := ids(o.id)
	a := s.exec(dir, "", o.resource, "archive", id, "-o", "json")
	if a.code == 0 {
		return archivedForGood, true
	}
	step := fmt.Sprintf("archive Exit %d", a.code)
	if s.quotaNow() {
		return step + ": Kontingent erschöpft (HTTP 429), abgebrochen", false
	}
	d := s.exec(dir, "", o.resource, "describe", id, "-o", "json")
	if d.code == 0 && str(tryDecode(d.stdout)["record_state"]) == "archived" {
		return step + ", laut describe " + archivedForGood, true
	}
	return fmt.Sprintf("%s, describe Exit %d, record_state %q: %s", step, d.code,
		str(tryDecode(d.stdout)["record_state"]), strings.TrimSpace(a.stderr)), false
}

// ownTarget reports whether the target of follow-up o (followUpOf) is an
// object this run registered itself: same resource, no follow-up, name
// owned by the suite.
func (s *liveSuite) ownTarget(o object) bool {
	for _, c := range s.created {
		if c.resource == o.resource && c.id == o.followUpOf && c.followUpOf == 0 && e2eguard.Owned(c.name) {
			return true
		}
	}
	return false
}

// cleanup removes all objects of this run, newest first (children before
// their parents). It reports whether every object is gone or archived.
func (s *liveSuite) cleanup() bool {
	s.cleaning = true
	all := true
	for i := len(s.created) - 1; i >= 0; i-- {
		o := s.created[i]
		what, ok := s.remove(s.mainDir, o)
		s.stderrf("Aufräumen %s %d (%s): %s", o.resource, o.id, o.name, what)
		all = all && ok
	}
	return all
}
