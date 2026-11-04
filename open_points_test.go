package papierkram_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/testutil/modroot"
)

// e2eTestRef matches a test of the live suite named in the open points:
// TestE2E_Name or TestE2E_Name/Subtest.
var e2eTestRef = regexp.MustCompile("`(TestE2E_[A-Za-z0-9_]+(?:/[^`\\s]+)?)`")

// TestOpenPointsNameExistingE2ETests checks every E2E test that
// docs/offene-punkte.md names: the test function exists in e2e/, and a
// subtest name is a string literal in its body (t.Run("01_…") or the
// name of a table case).
func TestOpenPointsNameExistingE2ETests(t *testing.T) {
	root := modroot.Find(t)
	doc, err := os.ReadFile(filepath.Join(root, "docs", "offene-punkte.md"))
	if err != nil {
		t.Fatal(err)
	}
	literals, err := e2eTestLiterals(filepath.Join(root, "e2e"))
	if err != nil {
		t.Fatal(err)
	}
	refs := e2eTestRef.FindAllStringSubmatch(string(doc), -1)
	if len(refs) == 0 {
		t.Fatal("keine E2E-Tests genannt")
	}
	for _, m := range refs {
		name, sub, _ := strings.Cut(m[1], "/")
		lits, ok := literals[name]
		switch {
		case !ok:
			t.Errorf("%s: kein Test in e2e/", m[1])
		case sub != "" && !lits[sub]:
			t.Errorf("%s: kein Subtest %q in %s", m[1], sub, name)
		}
	}
}

// e2eTestLiterals maps every test function TestE2E_… of the _test.go files
// in dir to the string literals of its body.
func e2eTestLiterals(dir string) (map[string]map[string]bool, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*_test.go"))
	if err != nil {
		return nil, err
	}
	out := map[string]map[string]bool{}
	fset := token.NewFileSet()
	for _, f := range files {
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			return nil, err
		}
		for _, d := range file.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "TestE2E_") {
				continue
			}
			lits := map[string]bool{}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if bl, ok := n.(*ast.BasicLit); ok && bl.Kind == token.STRING {
					if s, err := strconv.Unquote(bl.Value); err == nil {
						lits[s] = true
					}
				}
				return true
			})
			out[fn.Name.Name] = lits
		}
	}
	return out, nil
}
