package mcpserver

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
)

func f(v float64) *float64 { return &v }

// invoiceList mirrors the invoice_list tool that mcpToolSpecs builds from
// commands --json (flags only, no positional argument).
var invoiceList = ToolSpec{
	Name:    "invoice_list",
	Class:   "read_only",
	Command: []string{"invoice", "list"},
	Params: []Param{
		{Name: "all", Flag: "all", Pos: -1, Kind: KindBool},
		{Name: "company", Flag: "company", Pos: -1, Kind: KindRef},
		{Name: "limit", Flag: "limit", Pos: -1, Kind: KindInteger, Default: 25, Minimum: f(1), Maximum: f(100)},
		{Name: "max_pages", Flag: "max-pages", Pos: -1, Kind: KindInteger, Minimum: f(1)},
		{Name: "sort", Flag: "sort", Pos: -1, Kind: KindString},
	},
}

// upload is a write tool with two positional arguments and every other
// kind of flag (the shape of voucher upload plus the update flags).
var upload = ToolSpec{
	Name:    "voucher_upload",
	Class:   "create",
	Command: []string{"voucher", "upload"},
	Params: []Param{
		{Name: "file", Pos: 1, Kind: KindPath, Required: true},
		{Name: "voucher_id", Pos: 0, Kind: KindInteger, Minimum: f(0), Required: true},
		{Name: "name", Flag: "name", Pos: -1, Kind: KindString},
		{Name: "amount", Flag: "amount", Pos: -1, Kind: KindNumber},
		{Name: "attr", Flag: "attr", Pos: -1, Kind: KindStrings},
		{Name: "input", Flag: "input", Pos: -1, Kind: KindJSONFile, JSONType: "object"},
		{Name: "items", Flag: "items", Pos: -1, Kind: KindJSONFile, JSONType: "array"},
		{Name: "output_file", Flag: "output-file", Pos: -1, Kind: KindPath},
	},
}

var describe = ToolSpec{
	Name:    "invoice_describe",
	Class:   "read_only",
	Command: []string{"invoice", "describe"},
	Params:  []Param{{Name: "id", Pos: 0, Kind: KindInteger, Minimum: f(0), Required: true}},
}

// filesDirs are the files directories of the running tests (fd).
var filesDirs sync.Map

// fd is the files directory of test t, the same for every call in t.
func fd(t *testing.T) string {
	t.Helper()
	if d, ok := filesDirs.Load(t); ok {
		return d.(string)
	}
	d := t.TempDir()
	filesDirs.Store(t, d)
	t.Cleanup(func() { filesDirs.Delete(t) })
	return d
}

// inFD is the absolute path of name in the files directory of t.
func inFD(t *testing.T, name string) string { return filepath.Join(fd(t), name) }

// mustArgs is Args with the files directory of t unless o names one.
func mustArgs(t *testing.T, spec ToolSpec, raw string, o Options) []string {
	t.Helper()
	if o.FilesDir == "" {
		o.FilesDir = fd(t)
	}
	args, err := Args(spec, json.RawMessage(raw), o, t.TempDir())
	if err != nil {
		t.Fatalf("Args(%s): %v", raw, err)
	}
	return args
}

func TestMCPArgsOrder(t *testing.T) {
	got := mustArgs(t, invoiceList, `{"company":3214,"all":true,"max_pages":2}`, Options{})
	want := []string{"invoice", "list", "--all=true", "--company=3214", "--max-pages=2", "--no-input", "-o", "json", "--"}
	if !slices.Equal(got, want) {
		t.Errorf("Args\n got %q\nwant %q", got, want)
	}
	// positional arguments follow "--" in Pos order, whatever the order of
	// the params and properties
	got = mustArgs(t, upload, `{"file":"beleg.pdf","voucher_id":7,"name":"x"}`, Options{})
	want = []string{"voucher", "upload", "--name=x", "--no-input", "-o", "json", "--", "7", inFD(t, "beleg.pdf")}
	if !slices.Equal(got, want) {
		t.Errorf("Args\n got %q\nwant %q", got, want)
	}
	// Fixed comes after the flags, before the output options
	fixed := describe
	fixed.Fixed = []string{"--via=pdf"}
	got = mustArgs(t, fixed, `{"id":5}`, Options{})
	want = []string{"invoice", "describe", "--via=pdf", "--no-input", "-o", "json", "--", "5"}
	if !slices.Equal(got, want) {
		t.Errorf("Args\n got %q\nwant %q", got, want)
	}
	// empty or null arguments are an empty object
	for _, raw := range []string{"", "null", "{}"} {
		got = mustArgs(t, invoiceList, raw, Options{})
		if want := []string{"invoice", "list", "--no-input", "-o", "json", "--"}; !slices.Equal(got, want) {
			t.Errorf("Args(%q) = %q", raw, got)
		}
	}
}

func TestMCPArgsKinds(t *testing.T) {
	got := mustArgs(t, upload, `{"voucher_id":0,"file":"a b.pdf","amount":12.50,"attr":["a=1","b=2"],"name":"","output_file":"x.pdf"}`, Options{})
	want := []string{"voucher", "upload", "--amount=12.50", "--attr=a=1", "--attr=b=2", "--name=", "--output-file=" + inFD(t, "x.pdf"),
		"--no-input", "-o", "json", "--", "0", inFD(t, "a b.pdf")}
	if !slices.Equal(got, want) {
		t.Errorf("Args\n got %q\nwant %q", got, want)
	}
	got = mustArgs(t, invoiceList, `{"company":"Mustermann GmbH","all":false,"limit":100}`, Options{})
	want = []string{"invoice", "list", "--all=false", "--company=Mustermann GmbH", "--limit=100", "--no-input", "-o", "json", "--"}
	if !slices.Equal(got, want) {
		t.Errorf("Args\n got %q\nwant %q", got, want)
	}
}

// Review focus 2: values that look like flags stay values; no property
// sets --yes, --context or any global flag.
func TestMCPArgsNoFlagInjection(t *testing.T) {
	for _, v := range []string{"--yes", "-o", "--context=x", "--dry-run", "-y", "--", "--debug"} {
		raw, _ := json.Marshal(map[string]any{"sort": v, "company": v})
		got := mustArgs(t, invoiceList, string(raw), Options{})
		want := []string{"invoice", "list", "--company=" + v, "--sort=" + v, "--no-input", "-o", "json", "--"}
		if !slices.Equal(got, want) {
			t.Errorf("Wert %q\n got %q\nwant %q", v, got, want)
		}
		raw, _ = json.Marshal(map[string]any{"attr": []string{v}, "name": v, "voucher_id": 1, "file": v + ".pdf"})
		got = mustArgs(t, upload, string(raw), Options{})
		want = []string{"voucher", "upload", "--attr=" + v, "--name=" + v, "--no-input", "-o", "json", "--", "1", inFD(t, v+".pdf")}
		if !slices.Equal(got, want) {
			t.Errorf("Wert %q\n got %q\nwant %q", v, got, want)
		}
	}
	// a file name that starts with "-" becomes a path in the files
	// directory, behind "--"
	got := mustArgs(t, upload, `{"voucher_id":1,"file":"-x.pdf"}`, Options{})
	if i := slices.Index(got, "--"); i < 0 || !slices.Equal(got[i+1:], []string{"1", inFD(t, "-x.pdf")}) {
		t.Errorf("Positionsargumente nicht hinter --: %q", got)
	}
	// no property name can be a global flag: they are rejected as unknown
	for _, name := range []string{"yes", "context", "dry_run", "dry-run", "output", "o", "debug", "jq", "fields", "timeout", "--yes"} {
		raw, _ := json.Marshal(map[string]any{name: true})
		if _, err := Args(invoiceList, raw, Options{}, t.TempDir()); err == nil {
			t.Errorf("Eigenschaft %q angenommen", name)
		}
	}
	// --yes and --context only come from the options of the server
	for _, spec := range []ToolSpec{invoiceList, upload, describe} {
		raw := map[string]string{"invoice_list": `{"sort":"x"}`, "voucher_upload": `{"voucher_id":1,"file":"a.pdf"}`, "invoice_describe": `{"id":1}`}[spec.Name]
		for _, a := range mustArgs(t, spec, raw, Options{}) {
			if a == "--yes" || a == "-y" || strings.HasPrefix(a, "--context") || a == "--dry-run" {
				t.Errorf("%s: %q ohne Option", spec.Name, a)
			}
		}
	}
}

func TestMCPArgsRejects(t *testing.T) {
	for _, tc := range []struct {
		spec ToolSpec
		raw  string
		msg  string
	}{
		{invoiceList, `{"via":"email"}`, "unbekannter Parameter „via“"},
		{describe, `{"id":"5"}`, "„id“"},
		{describe, `{"id":1.5}`, "ganze Zahl"},
		{describe, `{"id":1e3}`, "ganze Zahl"},
		{describe, `{"id":null}`, "„id“"},
		{describe, `{}`, "fehlt"},
		{invoiceList, `{"limit":"25"}`, "„limit“"},
		{invoiceList, `{"all":"true"}`, "„all“"},
		{invoiceList, `{"company":1.5}`, "ganze Zahl"},
		{invoiceList, `{"company":true}`, "„company“"},
		{invoiceList, `[1]`, "Objekt"},
		{invoiceList, `{"limit":1`, "JSON"},
		{upload, `{"voucher_id":1,"file":"x","output_file":"-"}`, "„-“"},
		{upload, `{"voucher_id":1,"file":"-"}`, "„-“"},
		{upload, `{"voucher_id":1,"file":"x","input":"{\"name\":\"x\"}"}`, "„input“"},
		{upload, `{"voucher_id":1,"file":"x","input":[1]}`, "„input“"},
		{upload, `{"voucher_id":1,"file":"x","items":{"a":1}}`, "„items“"},
		{upload, `{"voucher_id":1,"file":"x","attr":"a=1"}`, "„attr“"},
		{upload, `{"voucher_id":1,"file":"x","attr":[1]}`, "„attr“"},
		{upload, `{"voucher_id":1,"file":"x","amount":"1"}`, "„amount“"},
		{upload, `{"voucher_id":1,"file":"x","name":5}`, "„name“: Text"},
		{upload, `{"voucher_id":1,"file":7}`, "„file“: Dateipfad"},
	} {
		_, err := Args(tc.spec, json.RawMessage(tc.raw), Options{FilesDir: fd(t)}, t.TempDir())
		var ae *ArgError
		if !errors.As(err, &ae) {
			t.Errorf("%s %s: Fehler %v, erwartet ArgError", tc.spec.Name, tc.raw, err)
			continue
		}
		if !strings.Contains(ae.Msg, tc.msg) || ae.Hint == "" {
			t.Errorf("%s %s: %q / %q, erwartet %q", tc.spec.Name, tc.raw, ae.Msg, ae.Hint, tc.msg)
		}
	}
}

func TestMCPArgsJSONFile(t *testing.T) {
	dir := t.TempDir()
	args, err := Args(upload, json.RawMessage(`{"voucher_id":1,"file":"x.pdf","input":{"name":"x"},"items":[{"name":"a"}]}`), Options{FilesDir: fd(t)}, dir)
	if err != nil {
		t.Fatal(err)
	}
	in, items := filepath.Join(dir, "input.json"), filepath.Join(dir, "items.json")
	want := []string{"voucher", "upload", "--input=" + in, "--items=" + items, "--no-input", "-o", "json", "--", "1", inFD(t, "x.pdf")}
	if !slices.Equal(args, want) {
		t.Fatalf("Args\n got %q\nwant %q", args, want)
	}
	for path, content := range map[string]string{in: `{"name":"x"}`, items: `[{"name":"a"}]`} {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != content {
			t.Errorf("%s = %q, erwartet %q", path, b, content)
		}
		st, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && st.Mode().Perm() != 0o600 {
			t.Errorf("%s: Rechte %v", path, st.Mode().Perm())
		}
	}
}

func TestMCPArgsContextAndDebug(t *testing.T) {
	got := mustArgs(t, describe, `{"id":7}`, Options{Context: "firma", Timeout: "45s", Debug: true})
	want := []string{"invoice", "describe", "--no-input", "-o", "json", "--context=firma", "--timeout=45s", "--debug", "--", "7"}
	if !slices.Equal(got, want) {
		t.Errorf("Args\n got %q\nwant %q", got, want)
	}
	// a context that looks like a flag stays the value of --context
	got = mustArgs(t, describe, `{"id":7}`, Options{Context: "--yes"})
	if !slices.Contains(got, "--context=--yes") || slices.Contains(got, "--yes") {
		t.Errorf("Args = %q", got)
	}
}

func TestMCPArgsEdgeCases(t *testing.T) {
	// a tool without parameters names that in the hint
	_, err := Args(ToolSpec{Name: "info"}, json.RawMessage(`{"x":1}`), Options{}, t.TempDir())
	var ae *ArgError
	if !errors.As(err, &ae) || ae.Hint != "Erlaubte Parameter: keine" {
		t.Errorf("Fehler %v", err)
	}
	// a name with control characters is quoted escaped
	_, err = Args(ToolSpec{Name: "info"}, json.RawMessage(`{"a\nb\"":1}`), Options{}, t.TempDir())
	if !errors.As(err, &ae) || !strings.Contains(ae.Msg, `„a\nb"“`) {
		t.Errorf("Fehler %v", err)
	}
	// a kind this package does not know is a programming error, no ArgError
	bad := ToolSpec{Name: "x", Params: []Param{{Name: "v", Flag: "v", Pos: -1, Kind: "weird"}}}
	if _, err := Args(bad, json.RawMessage(`{"v":1}`), Options{}, t.TempDir()); err == nil || errors.As(err, &ae) {
		t.Errorf("Fehler %v", err)
	}
	// the JSON file cannot be written: an error, no ArgError
	missing := filepath.Join(t.TempDir(), "fehlt")
	if _, err := Args(upload, json.RawMessage(`{"voucher_id":1,"file":"x","input":{}}`), Options{FilesDir: fd(t)}, missing); err == nil || errors.As(err, &ae) {
		t.Errorf("Fehler %v", err)
	}
}

// Ruling T5-A: every file path lies in the files directory; absolute
// paths, "..", "-", empty names and links out of it are argument errors.
func TestMCPArgsPathPolicy(t *testing.T) {
	dir := fd(t)
	outside := t.TempDir()
	secret := filepath.Join(outside, "geheim.txt")
	if err := os.WriteFile(secret, []byte("geheim"), 0o600); err != nil {
		t.Fatal(err)
	}
	// fail closed: a missing directory, a NUL byte
	bad := []string{"", "-", "/dev/fd/1", secret, "../x.pdf", "a/../../x.pdf", filepath.Join(dir, "x.pdf"), "fehlt/x.pdf", "a\x00b.pdf"}
	if runtime.GOOS == "windows" {
		bad = append(bad, `C:\x.pdf`, "NUL")
	}
	if err := os.Symlink(secret, filepath.Join(dir, "link.pdf")); err == nil {
		if err := os.Symlink(outside, filepath.Join(dir, "raus")); err != nil {
			t.Fatal(err)
		}
		bad = append(bad, "link.pdf", "raus/neu.pdf", "raus/geheim.txt")
		// a link that leads nowhere, a loop
		if err := os.Symlink(filepath.Join(dir, "nichts.pdf"), filepath.Join(dir, "leer.pdf")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(dir, "schleife2"), filepath.Join(dir, "schleife1")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(dir, "schleife1"), filepath.Join(dir, "schleife2")); err != nil {
			t.Fatal(err)
		}
		bad = append(bad, "leer.pdf", "schleife1", "schleife1/x.pdf")
	}
	for _, v := range bad {
		for _, param := range []string{"file", "output_file"} {
			raw, _ := json.Marshal(map[string]any{"voucher_id": 1, "file": "ok.pdf", param: v})
			_, err := Args(upload, raw, Options{FilesDir: dir}, t.TempDir())
			var ae *ArgError
			if !errors.As(err, &ae) || !strings.Contains(ae.Msg, quoted(param)) || ae.Hint == "" {
				t.Errorf("%s %q: Fehler %v, erwartet ArgError", param, v, err)
			}
		}
	}
	// inside: nested names, a link that stays inside, a missing directory
	if err := os.Mkdir(filepath.Join(dir, "ablage"), 0o700); err != nil {
		t.Fatal(err)
	}
	good := []string{"x.pdf", "ablage/x.pdf", "./x.pdf"}
	if err := os.Symlink(filepath.Join(dir, "ablage"), filepath.Join(dir, "innen")); err == nil {
		good = append(good, "innen/x.pdf")
	}
	for _, v := range good {
		raw, _ := json.Marshal(map[string]any{"voucher_id": 1, "file": v})
		args, err := Args(upload, raw, Options{FilesDir: dir}, t.TempDir())
		if err != nil || args[len(args)-1] != filepath.Join(dir, v) {
			t.Errorf("%q: %v, %q", v, err, args)
		}
	}
	// a component that is no file, directory or link (a Windows junction:
	// Lstat reports ModeIrregular; simulated) is refused, also as leaf
	prev := lstat
	lstat = func(p string) (os.FileInfo, error) {
		fi, err := os.Lstat(p)
		if err == nil && filepath.Base(p) == "ablage" {
			return irregular{fi}, nil
		}
		return fi, err
	}
	for _, v := range []string{"ablage/x.pdf", "ablage"} {
		raw, _ := json.Marshal(map[string]any{"voucher_id": 1, "file": v})
		_, err := Args(upload, raw, Options{FilesDir: dir}, t.TempDir())
		var ae *ArgError
		if !errors.As(err, &ae) || !strings.Contains(ae.Msg, "Junction") {
			t.Errorf("%q: %v", v, err)
		}
	}
	lstat = prev
	// a component that cannot be read is refused
	lstat = func(p string) (os.FileInfo, error) { return nil, os.ErrPermission }
	if _, err := Args(upload, json.RawMessage(`{"voucher_id":1,"file":"x.pdf"}`), Options{FilesDir: dir}, t.TempDir()); err == nil {
		t.Error("nicht lesbarer Pfadteil angenommen")
	}
	lstat = prev
	// without a files directory a required path is an argument error
	// (ruling T6-A; New does not serve such a tool)
	_, err := Args(upload, json.RawMessage(`{"voucher_id":1,"file":"x.pdf"}`), Options{}, t.TempDir())
	var ae *ArgError
	if !errors.As(err, &ae) || !strings.Contains(ae.Msg, "braucht einen Dateiordner") {
		t.Errorf("ohne Dateiordner: %v", err)
	}
	// localPath itself never resolves without a directory
	if _, err := localPath("file", "", "x.pdf"); err == nil || errors.As(err, &ae) {
		t.Errorf("localPath ohne Dateiordner: %v", err)
	}
	// a files directory that vanished: error, no ArgError
	_, err = Args(upload, json.RawMessage(`{"voucher_id":1,"file":"x.pdf"}`), Options{FilesDir: filepath.Join(dir, "weg")}, t.TempDir())
	if err == nil || errors.As(err, &ae) {
		t.Errorf("Dateiordner fehlt: %v", err)
	}
}

// irregular is a FileInfo with ModeIrregular (a Windows junction).
type irregular struct{ os.FileInfo }

func (i irregular) Mode() fs.FileMode { return i.FileInfo.Mode()&^fs.ModeType | fs.ModeIrregular }

// Task 6: every tool of a class but read_only has the parameter dry_run
// (--dry-run); destructive and send get --yes, but only when the mode
// allows them (--allow-destructive). Without it the CLI asks and ends
// with exit 8 (fail closed).
func TestMCPArgsDryRunAndYes(t *testing.T) {
	id := Param{Name: "id", Pos: 0, Kind: KindInteger, Minimum: f(0), Required: true}
	create := ToolSpec{Name: "company_create", Class: "create", Command: []string{"company", "create"},
		Params: []Param{{Name: "name", Flag: "name", Pos: -1, Kind: KindString}}}
	del := ToolSpec{Name: "invoice_delete", Class: "destructive", Command: []string{"invoice", "delete"}, Params: []Param{id}}
	deliver := ToolSpec{Name: "invoice_deliver", Class: "send", Command: []string{"invoice", "deliver"}, Params: []Param{id}, Fixed: []string{"--via=pdf"}}
	archive := ToolSpec{Name: "company_archive", Class: "archive", Command: []string{"company", "archive"}, Params: []Param{id}}
	for _, tc := range []struct {
		spec ToolSpec
		dry  bool
	}{{describe, false}, {invoiceList, false}, {create, true}, {upload, true}, {archive, true}, {del, true}, {deliver, true}} {
		props := InputSchema(tc.spec)["properties"].(map[string]any)
		p, ok := props["dry_run"]
		if ok != tc.dry {
			t.Errorf("%s: dry_run %v, erwartet %v", tc.spec.Name, ok, tc.dry)
			continue
		}
		if ok && (p.(map[string]any)["type"] != "boolean" || p.(map[string]any)["description"] == "") {
			t.Errorf("%s: dry_run %v", tc.spec.Name, p)
		}
		if req, _ := InputSchema(tc.spec)["required"].([]string); slices.Contains(req, "dry_run") {
			t.Errorf("%s: dry_run ist Pflicht", tc.spec.Name)
		}
	}
	for _, tc := range []struct {
		spec ToolSpec
		raw  string
		mode Mode
		want []string
	}{
		{create, `{"name":"X","dry_run":true}`, ModeWrite, []string{"company", "create", "--dry-run=true", "--name=X", "--no-input", "-o", "json", "--"}},
		{create, `{"name":"X"}`, ModeDestructive, []string{"company", "create", "--name=X", "--no-input", "-o", "json", "--"}},
		{archive, `{"id":2843,"dry_run":false}`, ModeWrite, []string{"company", "archive", "--dry-run=false", "--no-input", "-o", "json", "--", "2843"}},
		{del, `{"id":459}`, ModeDestructive, []string{"invoice", "delete", "--yes", "--no-input", "-o", "json", "--", "459"}},
		{del, `{"id":459,"dry_run":true}`, ModeDestructive, []string{"invoice", "delete", "--dry-run=true", "--yes", "--no-input", "-o", "json", "--", "459"}},
		{deliver, `{"id":459}`, ModeDestructive, []string{"invoice", "deliver", "--via=pdf", "--yes", "--no-input", "-o", "json", "--", "459"}},
		// not served in these modes; Args alone never confirms for them
		{del, `{"id":459}`, ModeWrite, []string{"invoice", "delete", "--no-input", "-o", "json", "--", "459"}},
		{deliver, `{"id":459}`, ModeRead, []string{"invoice", "deliver", "--via=pdf", "--no-input", "-o", "json", "--", "459"}},
	} {
		if got := mustArgs(t, tc.spec, tc.raw, Options{Mode: tc.mode}); !slices.Equal(got, tc.want) {
			t.Errorf("%s %s Mode %d\n got %q\nwant %q", tc.spec.Name, tc.raw, tc.mode, got, tc.want)
		}
	}
	// a read tool has no dry_run; a write tool takes it only as a boolean
	for _, tc := range []struct {
		spec ToolSpec
		raw  string
		msg  string
	}{
		{describe, `{"id":1,"dry_run":true}`, "unbekannter Parameter „dry_run“"},
		{create, `{"dry_run":"true"}`, "„dry_run“: Wahrheitswert"},
		{deliver, `{"id":1,"via":"email"}`, "unbekannter Parameter „via“"},
		{deliver, `{"id":1,"yes":false}`, "unbekannter Parameter „yes“"},
	} {
		_, err := Args(tc.spec, json.RawMessage(tc.raw), Options{Mode: ModeDestructive}, t.TempDir())
		var ae *ArgError
		if !errors.As(err, &ae) || !strings.Contains(ae.Msg, tc.msg) {
			t.Errorf("%s %s: Fehler %v, erwartet %q", tc.spec.Name, tc.raw, err, tc.msg)
		}
	}
	// the hint lists dry_run with the other parameters
	_, err := Args(create, json.RawMessage(`{"x":1}`), Options{}, t.TempDir())
	var ae *ArgError
	if !errors.As(err, &ae) || ae.Hint != "Erlaubte Parameter: dry_run, name" {
		t.Errorf("Fehler %v", err)
	}
}
