package e2eguard

import (
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/testutil/modroot"
)

// logFile is the only file of e2e/ that may write test output (it
// redacts the token first).
const logFile = "log_test.go"

// outputCalls are methods of testing.T/TB that write output or skip.
var outputCalls = []string{"Log", "Logf", "Error", "Errorf", "Fatal", "Fatalf", "Skip", "Skipf"}

// checkSuiteSource checks the rules the live suite in dir relies on and
// returns the violations:
//   - every _test.go file carries //go:build e2e; other files declare
//     nothing (the package doc only);
//   - TestMain starts with e2eguard.Decide (nothing before the decision);
//     no init and no package variable initialised by a call other than a
//     pure one (allowedVarCalls, conversions), since those run earlier;
//   - every other Test function starts with live(t), so a skipped suite
//     sends no request; no Example, Fuzz or Benchmark functions;
//   - only log_test.go writes output (t.Log …, fmt.Print …, os.Stdout,
//     os.Stderr, package log): all output goes through the redaction;
//   - no string literal "email" (deliver only --via pdf).
func checkSuiteSource(dir string) ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}
	var bad []string
	add := func(pos token.Position, msg string) {
		bad = append(bad, filepath.Base(pos.Filename)+":"+strconv.Itoa(pos.Line)+": "+msg)
	}
	fset := token.NewFileSet()
	sawMain := false
	for _, p := range paths {
		f, err := parser.ParseFile(fset, p, nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		name := filepath.Base(p)
		start := fset.Position(f.Package)
		start.Line = 1
		if !strings.HasSuffix(name, "_test.go") {
			if len(f.Decls) > 0 {
				add(start, "Datei ohne Build-Tag darf nichts deklarieren")
			}
			continue
		}
		if !hasE2ETag(f) {
			add(start, "fehlt //go:build e2e")
		}
		for _, imp := range f.Imports {
			if imp.Path.Value == `"log"` {
				add(fset.Position(imp.Pos()), "Paket log umgeht die Schwärzung")
			}
		}
		for _, d := range f.Decls {
			if gd, ok := d.(*ast.GenDecl); ok && gd.Tok == token.VAR {
				for _, call := range varCalls(gd) {
					add(fset.Position(call.Pos()), "Paketvariable mit Aufruf "+callName(call)+" läuft vor TestMain")
				}
				continue
			}
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv != nil {
				continue
			}
			pos := fset.Position(fn.Pos())
			name := fn.Name.Name
			switch {
			case name == "init":
				add(pos, "init läuft vor TestMain")
				continue
			case strings.HasPrefix(name, "Example"), strings.HasPrefix(name, "Fuzz"), strings.HasPrefix(name, "Benchmark"):
				add(pos, name+": Example-, Fuzz- und Benchmark-Funktionen laufen ohne live(t)")
				continue
			case !strings.HasPrefix(name, "Test"):
				continue
			}
			switch {
			case fn.Name.Name == "TestMain":
				sawMain = true
				if !startsWithCall(fn, "e2eguard", "Decide") {
					add(pos, "TestMain beginnt nicht mit e2eguard.Decide")
				}
			case !startsWithCall(fn, "", "live"):
				add(pos, fn.Name.Name+" beginnt nicht mit live(t)")
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.BasicLit:
				if v, err := strconv.Unquote(x.Value); err == nil && x.Kind == token.STRING && strings.EqualFold(v, "email") {
					add(fset.Position(x.Pos()), `Literal "email" (deliver nur --via pdf)`)
				}
			case *ast.CallExpr:
				if name != logFile && writesOutput(x) {
					add(fset.Position(x.Pos()), "Ausgabe außerhalb von "+logFile+" (ohne Schwärzung)")
				}
			case *ast.SelectorExpr:
				if id, ok := x.X.(*ast.Ident); ok && id.Name == "os" && (x.Sel.Name == "Stdout" || x.Sel.Name == "Stderr") && name != logFile {
					add(fset.Position(x.Pos()), "os."+x.Sel.Name+" außerhalb von "+logFile)
				}
			}
			return true
		})
	}
	if !sawMain {
		bad = append(bad, "kein TestMain")
	}
	return bad, nil
}

// allowedVarCalls are the calls a package variable of the suite may be
// initialised with: pure, no I/O (they run before TestMain).
var allowedVarCalls = []string{"e2eguard.CleanupReserve"}

// conversions are the type names whose conversion is allowed.
var conversions = []string{"string", "rune", "byte", "int", "int64", "uint", "float64"}

// varCalls returns the calls in the initialisers of a var declaration
// that are not allowed; function literals do not run at initialisation
// and are skipped.
func varCalls(gd *ast.GenDecl) []*ast.CallExpr {
	var out []*ast.CallExpr
	for _, spec := range gd.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for _, v := range vs.Values {
			ast.Inspect(v, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.FuncLit:
					return false
				case *ast.CallExpr:
					if !allowedCall(x) {
						out = append(out, x)
					}
				}
				return true
			})
		}
	}
	return out
}

func allowedCall(c *ast.CallExpr) bool {
	switch f := c.Fun.(type) {
	case *ast.ArrayType:
		return true // conversion such as []byte("…")
	case *ast.Ident:
		return slices.Contains(conversions, f.Name)
	}
	return slices.Contains(allowedVarCalls, callName(c))
}

// callName is pkg.Func or Func of a call, or "?" for other forms.
func callName(c *ast.CallExpr) string {
	switch f := c.Fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		if id, ok := f.X.(*ast.Ident); ok {
			return id.Name + "." + f.Sel.Name
		}
	}
	return "?"
}

func hasE2ETag(f *ast.File) bool {
	for _, cg := range f.Comments {
		if cg.Pos() > f.Package {
			break
		}
		for _, c := range cg.List {
			if expr, err := constraint.Parse(c.Text); err == nil {
				return expr.String() == "e2e"
			}
		}
	}
	return false
}

// startsWithCall reports whether the first statement of fn calls pkg.fun
// (pkg "" for a plain function), as a statement or on the right side of
// an assignment.
func startsWithCall(fn *ast.FuncDecl, pkg, fun string) bool {
	if fn.Body == nil || len(fn.Body.List) == 0 {
		return false
	}
	var call ast.Expr
	switch st := fn.Body.List[0].(type) {
	case *ast.ExprStmt:
		call = st.X
	case *ast.AssignStmt:
		if len(st.Rhs) == 1 {
			call = st.Rhs[0]
		}
	}
	c, ok := call.(*ast.CallExpr)
	if !ok {
		return false
	}
	switch f := c.Fun.(type) {
	case *ast.Ident:
		return pkg == "" && f.Name == fun
	case *ast.SelectorExpr:
		id, ok := f.X.(*ast.Ident)
		return ok && id.Name == pkg && f.Sel.Name == fun
	}
	return false
}

// writesOutput reports whether call writes test or process output:
// x.Log … x.Skipf (any receiver; Error only with arguments, err.Error()
// is the error text), fmt.Print*/Fprint*, anything of package log, print
// and println.
func writesOutput(call *ast.CallExpr) bool {
	switch f := call.Fun.(type) {
	case *ast.Ident:
		return f.Name == "print" || f.Name == "println"
	case *ast.SelectorExpr:
		if id, ok := f.X.(*ast.Ident); ok {
			switch id.Name {
			case "fmt":
				return strings.HasPrefix(f.Sel.Name, "Print") || strings.HasPrefix(f.Sel.Name, "Fprint")
			case "log":
				return true
			}
		}
		if f.Sel.Name == "Error" && len(call.Args) == 0 {
			return false
		}
		return slices.Contains(outputCalls, f.Sel.Name)
	}
	return false
}

// The real suite keeps the rules (it is not compiled without the e2e
// tag, so this check runs in every normal `go test ./...`).
func TestE2ESuiteSource(t *testing.T) {
	bad, err := checkSuiteSource(filepath.Join(modroot.Find(t), "e2e"))
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range bad {
		t.Error(b)
	}
}

func TestCheckSuiteSourceFindsViolations(t *testing.T) {
	dir := t.TempDir()
	write := func(name, src string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("doc.go", "// Package e2e is the suite.\npackage e2e\n")
	write("log_test.go", `//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"testing"
)

func live(t *testing.T) int { t.Skip("x"); return 0 }

func logf(t *testing.T, s string) { t.Log(s); fmt.Fprintln(os.Stderr, s) }
`)
	write("main_test.go", `//go:build e2e

package e2e

import (
	"os"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/e2eguard"
)

func TestMain(m *testing.M) {
	run, _ := e2eguard.Decide(os.Getenv)
	_ = run
	os.Exit(m.Run())
}

func TestGood(t *testing.T) {
	s := live(t)
	_ = s
}

func TestGoodPlain(t *testing.T) {
	live(t)
}
`)
	bad, err := checkSuiteSource(dir)
	if err != nil || len(bad) != 0 {
		t.Fatalf("clean suite: %v, %v", bad, err)
	}

	write("doc2.go", "package e2e\n\nfunc helper() {}\n")
	write("bad_test.go", `package e2e

import (
	"fmt"
	"log"
	"os"
	"testing"
)

func TestNoGuard(t *testing.T) {
	t.Logf("x")
	live(t)
}

func TestEmpty(t *testing.T) {}

func TestOutput(t *testing.T) {
	s := live(t)
	_ = s.Error()
	t.Errorf("token")
	fmt.Println("x")
	println("x")
	_ = os.Stdout
	log.Print("x")
	_ = []string{"deliver", "--via", "email"}
}
`)
	bad, err = checkSuiteSource(dir)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(bad, "\n")
	for _, want := range []string{
		"doc2.go:1: Datei ohne Build-Tag darf nichts deklarieren",
		"bad_test.go:1: fehlt //go:build e2e",
		"Paket log umgeht",
		"TestNoGuard beginnt nicht mit live(t)",
		"TestEmpty beginnt nicht mit live(t)",
		"bad_test.go:11: Ausgabe außerhalb",
		"bad_test.go:20: Ausgabe außerhalb",
		"bad_test.go:21: Ausgabe außerhalb",
		"bad_test.go:22: Ausgabe außerhalb",
		"bad_test.go:23: os.Stdout außerhalb",
		"bad_test.go:24: Ausgabe außerhalb",
		`bad_test.go:25: Literal "email"`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing violation %q in:\n%s", want, joined)
		}
	}
	// s.Error() without arguments is an error text, not output
	if strings.Contains(joined, "bad_test.go:19:") {
		t.Errorf("err.Error() counted as output:\n%s", joined)
	}
}

// TestMain must decide first; without TestMain the suite would run.
func TestCheckSuiteSourceTestMain(t *testing.T) {
	dir := t.TempDir()
	src := `//go:build e2e

package e2e

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	setup()
	os.Exit(m.Run())
}
`
	if err := os.WriteFile(filepath.Join(dir, "main_test.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	bad, _ := checkSuiteSource(dir)
	if !slices.ContainsFunc(bad, func(s string) bool { return strings.Contains(s, "TestMain beginnt nicht mit e2eguard.Decide") }) {
		t.Errorf("violations %v", bad)
	}
	if err := os.Remove(filepath.Join(dir, "main_test.go")); err != nil {
		t.Fatal(err)
	}
	if bad, _ := checkSuiteSource(dir); !slices.Contains(bad, "kein TestMain") {
		t.Errorf("violations without TestMain: %v", bad)
	}
}

// Nothing may run before TestMain decides, and nothing next to the tests
// without live(t): no init, no package variable initialised by a call
// (except the pure ones of allowedVarCalls), no Example, Fuzz or
// Benchmark functions.
func TestCheckSuiteSourceBeforeTestMain(t *testing.T) {
	dir := t.TempDir()
	src := `//go:build e2e

package e2e

import (
	"net/http"
	"os"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/e2eguard"
)

func TestMain(m *testing.M) {
	run, _ := e2eguard.Decide(os.Getenv)
	_ = run
	os.Exit(m.Run())
}

var (
	okReserve = e2eguard.CleanupReserve(1, 2, 3)
	okBytes   = []byte("pdf")
	okConv    = string(rune(65))
	okFunc    = func() string { return os.Getenv("X") }
	okMap     = map[string]bool{"a": true}
)

var badEnv = os.Getenv("PAPIERKRAM_E2E_TOKEN")

var badNested = map[string]string{"a": os.Getenv("X")}

var badResp, _ = http.Get("https://example.org")

func init() {}

func ExampleLive() {}

func FuzzLive(f *testing.F) {}

func BenchmarkLive(b *testing.B) {}
`
	if err := os.WriteFile(filepath.Join(dir, "main_test.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	bad, err := checkSuiteSource(dir)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(bad, "\n")
	for _, want := range []string{
		"main_test.go:27: Paketvariable mit Aufruf os.Getenv",
		"main_test.go:29: Paketvariable mit Aufruf os.Getenv",
		"main_test.go:31: Paketvariable mit Aufruf http.Get",
		"main_test.go:33: init",
		"main_test.go:35: ExampleLive",
		"main_test.go:37: FuzzLive",
		"main_test.go:39: BenchmarkLive",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing violation %q in:\n%s", want, joined)
		}
	}
	for _, line := range []string{":20:", ":21:", ":22:", ":23:", ":24:"} {
		if strings.Contains(joined, "main_test.go"+line) {
			t.Errorf("allowed variable reported (%s):\n%s", line, joined)
		}
	}
}
