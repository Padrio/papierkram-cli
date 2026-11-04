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
// subtest is one of its subtests (see e2eSubtests).
func TestOpenPointsNameExistingE2ETests(t *testing.T) {
	root := modroot.Find(t)
	doc, err := os.ReadFile(filepath.Join(root, "docs", "offene-punkte.md"))
	if err != nil {
		t.Fatal(err)
	}
	subtests, err := e2eSubtests(filepath.Join(root, "e2e"))
	if err != nil {
		t.Fatal(err)
	}
	refs := e2eTestRef.FindAllStringSubmatch(string(doc), -1)
	if len(refs) == 0 {
		t.Fatal("keine E2E-Tests genannt")
	}
	for _, m := range refs {
		name, sub, _ := strings.Cut(m[1], "/")
		subs, ok := subtests[name]
		switch {
		case !ok:
			t.Errorf("%s: kein Test in e2e/", m[1])
		case sub != "" && !subs[sub]:
			t.Errorf("%s: kein Subtest %q in %s", m[1], sub, name)
		}
	}
}

// e2eSubtests maps every test function TestE2E_… of the _test.go files in
// dir to the names of its subtests: the string literals that t.Run gets as
// its first argument, and for a table test (for _, c := range cases {
// t.Run(c.name, …) }) the names of the cases: the first element of each
// case literal in the table assigned to cases, when it is a string
// literal, or its name field.
func e2eSubtests(dir string) (map[string]map[string]bool, error) {
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
			out[fn.Name.Name] = subtestNames(fn.Body)
		}
	}
	return out, nil
}

// subtestNames collects the subtest names of one test body (see
// e2eSubtests).
func subtestNames(body *ast.BlockStmt) map[string]bool {
	tables := map[string]*ast.CompositeLit{} // variable -> table literal
	ranged := map[string]string{}            // range value variable -> table variable
	var runs []ast.Expr                      // first arguments of t.Run
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			for i, l := range x.Lhs {
				id, ok := l.(*ast.Ident)
				if !ok || i >= len(x.Rhs) {
					continue
				}
				if cl, ok := x.Rhs[i].(*ast.CompositeLit); ok {
					tables[id.Name] = cl
				}
			}
		case *ast.RangeStmt:
			v, vok := x.Value.(*ast.Ident)
			tbl, tok := x.X.(*ast.Ident)
			if vok && tok {
				ranged[v.Name] = tbl.Name
			}
		case *ast.CallExpr:
			if sel, ok := x.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Run" && len(x.Args) > 0 {
				runs = append(runs, x.Args[0])
			}
		}
		return true
	})
	names := map[string]bool{}
	for _, arg := range runs {
		if s, ok := stringLit(arg); ok {
			names[s] = true
			continue
		}
		sel, ok := arg.(*ast.SelectorExpr)
		if !ok {
			continue
		}
		v, ok := sel.X.(*ast.Ident)
		if !ok {
			continue
		}
		tbl := tables[ranged[v.Name]]
		if tbl == nil {
			continue
		}
		for _, e := range tbl.Elts {
			if c, ok := e.(*ast.CompositeLit); ok && len(c.Elts) > 0 {
				first := c.Elts[0]
				if kv, ok := first.(*ast.KeyValueExpr); ok {
					if k, ok := kv.Key.(*ast.Ident); !ok || k.Name != sel.Sel.Name {
						continue
					}
					first = kv.Value
				}
				if s, ok := stringLit(first); ok {
					names[s] = true
				}
			}
		}
	}
	return names
}

// stringLit is the value of e when it is a string literal.
func stringLit(e ast.Expr) (string, bool) {
	bl, ok := e.(*ast.BasicLit)
	if !ok || bl.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(bl.Value)
	return s, err == nil
}

func TestE2ESubtests(t *testing.T) {
	subs, err := e2eSubtests(filepath.Join(modroot.Find(t), "e2e"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"TestE2E_Carries/01_DeadlineISO", "TestE2E_VatRate019Is19Percent/Artikel",
		"TestE2E_PartialUpdateSemantics/company", "TestE2E_PartialUpdateSemantics/custom-attribute"} {
		name, sub, _ := strings.Cut(want, "/")
		if !subs[name][sub] {
			t.Errorf("%s: nicht als Subtest erkannt", want)
		}
	}
	// other literals of the body are no subtests
	for _, not := range []string{"TestE2E_Carries/day", "TestE2E_Carries/2026-12-31", "TestE2E_PartialUpdateSemantics/notes", "TestE2E_PartialUpdateSemantics/name"} {
		name, sub, _ := strings.Cut(not, "/")
		if subs[name][sub] {
			t.Errorf("%s: fälschlich als Subtest erkannt", not)
		}
	}
}
