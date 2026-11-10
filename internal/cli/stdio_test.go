package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/testutil/modroot"
)

// stdioUse is one direct use of the process stdio in a source file.
type stdioUse struct {
	file, fn, what string
	line           int
}

// stdioAllowed are the places that may touch the process stdio: NewApp
// wires the App to it, main exits with the code of the CLI. Everything
// else writes through App.In/Out/Err, so mcp serve keeps stdout for
// JSON-RPC (review focus 1).
func stdioAllowed(u stdioUse) bool {
	return (u.file == "internal/cli/app.go" && u.fn == "NewApp") || u.file == "cmd/papierkram/main.go"
}

// stdioUses parses the Go file at path (rel: relative to the module root
// with "/") and lists its uses of os.Stdin/Stdout/Stderr, fmt.Print*,
// the builtins print/println and imports of log and log/slog.
func stdioUses(t *testing.T, path, rel string) []stdioUse {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	local := map[string]string{} // import name in the file → path
	var uses []stdioUse
	for _, imp := range f.Imports {
		p, _ := strconv.Unquote(imp.Path.Value)
		name := p[strings.LastIndex(p, "/")+1:]
		if imp.Name != nil {
			name = imp.Name.Name
		}
		local[name] = p
		if p == "log" || p == "log/slog" {
			uses = append(uses, stdioUse{file: rel, what: "import " + p, line: fset.Position(imp.Pos()).Line})
		}
	}
	var fn string
	visit := func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncDecl:
			fn = x.Name.Name
		case *ast.SelectorExpr:
			if id, ok := x.X.(*ast.Ident); ok {
				switch {
				case local[id.Name] == "os" && (x.Sel.Name == "Stdin" || x.Sel.Name == "Stdout" || x.Sel.Name == "Stderr"),
					local[id.Name] == "fmt" && strings.HasPrefix(x.Sel.Name, "Print"):
					uses = append(uses, stdioUse{file: rel, fn: fn, what: id.Name + "." + x.Sel.Name, line: fset.Position(x.Pos()).Line})
				}
			}
		case *ast.CallExpr:
			if id, ok := x.Fun.(*ast.Ident); ok && (id.Name == "print" || id.Name == "println") {
				uses = append(uses, stdioUse{file: rel, fn: fn, what: id.Name, line: fset.Position(x.Pos()).Line})
			}
		}
		return true
	}
	ast.Inspect(f, visit)
	return uses
}

// Review focus 1: no code but NewApp and main touches the process stdio.
func TestNoDirectStdio(t *testing.T) {
	root := modroot.Find(t)
	var allowed int
	for _, dir := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && d.Name() == "testdata" {
				return filepath.SkipDir
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			for _, u := range stdioUses(t, path, filepath.ToSlash(rel)) {
				if stdioAllowed(u) {
					allowed++
					continue
				}
				t.Errorf("%s:%d: %s in %s – über App.In/Out/Err schreiben", u.file, u.line, u.what, u.fn)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if allowed == 0 {
		t.Error("NewApp ohne os.Stdout gefunden: Scan prüft nichts")
	}
}

// The scanner finds every form it forbids.
func TestStdioUsesFinds(t *testing.T) {
	src := `package x

import (
	"fmt"
	"log"
	stdos "os"
)

func A() {
	fmt.Println("x")
	stdos.Stdout.Write(nil)
	println("y")
	log.Print("z")
	_ = stdos.Stdin
}
`
	path := writeFile(t, "x.go", src)
	var got []string
	for _, u := range stdioUses(t, path, "x.go") {
		got = append(got, u.what)
	}
	want := "import log,fmt.Println,stdos.Stdout,println,stdos.Stdin"
	if strings.Join(got, ",") != want {
		t.Errorf("gefunden %v, erwartet %s", got, want)
	}
}
