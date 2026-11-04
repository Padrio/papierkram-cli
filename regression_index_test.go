// Package papierkram_test holds module-wide checks that belong to no single
// package. It has no non-test files and is never built into the binary.
package papierkram_test

import (
	"errors"
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/testutil/modroot"
)

// regressionCount is the number of rows of the regression list (spec 16).
const regressionCount = 39

// regressionTest matches the name of a regression test,
// TestRNN_<Kurzname>. It also matches other digit counts (TestR7_,
// TestR100_), which the test rejects: the number is always two digits.
var regressionTest = regexp.MustCompile(`^TestR(\d+)_`)

// TestRegressionIndex enforces spec 16: every regression R01–R39 is a test
// named TestRNN_<Kurzname>, each exactly once across all test files of the
// module, and there are no others.
func TestRegressionIndex(t *testing.T) {
	found, badName, err := scanRegressionTests(modroot.Find(t))
	if err != nil {
		t.Fatalf("scan test files: %v", err)
	}
	for n := 1; n <= regressionCount; n++ {
		switch files := found[n]; len(files) {
		case 0:
			t.Errorf("R%02d: no test func TestR%02d_…", n, n)
		case 1:
		default:
			slices.Sort(files)
			t.Errorf("R%02d: %d tests func TestR%02d_…, want 1: %s", n, len(files), n, strings.Join(files, ", "))
		}
	}
	for _, b := range badName {
		t.Errorf("%s: regression number must have two digits", b)
	}
	var extra []int
	for n := range found {
		if n < 1 || n > regressionCount {
			extra = append(extra, n)
		}
	}
	slices.Sort(extra)
	for _, n := range extra {
		t.Errorf("R%02d: not in the regression list of spec 16 (R01–R%02d): %s",
			n, regressionCount, strings.Join(found[n], ", "))
	}
}

// scanRegressionTests returns the files per regression number of all
// top-level test funcs named TestRNN_ under root, and the names with another
// digit count. It reads the test files `go test ./...` builds (see
// skipEntry, ignoredByBuild) and parses them, so commented-out code, strings
// and methods do not count. A file that does not parse is an error.
func scanRegressionTests(root string) (map[int][]string, []string, error) {
	found := map[int][]string{}
	var badName []string
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		if skip, err := skipEntry(path, d); skip || err != nil {
			if skip && d.IsDir() {
				return filepath.SkipDir
			}
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		if ignoredByBuild(f) {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Recv != nil {
				continue
			}
			m := regressionTest.FindStringSubmatch(fd.Name.Name)
			if m == nil {
				continue
			}
			if len(m[1]) != 2 {
				badName = append(badName, fmt.Sprintf("TestR%s_ in %s", m[1], rel))
				continue
			}
			n, _ := strconv.Atoi(m[1]) // two digits always parse
			found[n] = append(found[n], rel)
		}
		return nil
	})
	return found, badName, err
}

// skipEntry reports whether the go tool ignores a directory or file below
// the module root: names starting with _ or ., testdata and vendor
// directories, and nested modules (a directory with its own go.mod, e.g. a
// worktree).
func skipEntry(path string, d fs.DirEntry) (bool, error) {
	name := d.Name()
	if strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".") {
		return true, nil
	}
	if !d.IsDir() {
		return false, nil
	}
	if name == "testdata" || name == "vendor" {
		return true, nil
	}
	_, err := os.Stat(filepath.Join(path, "go.mod"))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// ignoredByBuild reports whether the //go:build constraint of f uses the
// tag ignore, the convention for files no build includes. Other tags (OS,
// keychain) do not matter: those tests run in some configuration.
func ignoredByBuild(f *ast.File) bool {
	for _, cg := range f.Comments {
		if cg.Pos() >= f.Package {
			break
		}
		for _, c := range cg.List {
			if !constraint.IsGoBuild(c.Text) {
				continue
			}
			expr, err := constraint.Parse(c.Text)
			if err != nil {
				continue // the parser of the go tool reports it
			}
			ignored := false
			expr.Eval(func(tag string) bool {
				ignored = ignored || tag == "ignore"
				return true
			})
			if ignored {
				return true
			}
		}
	}
	return false
}
