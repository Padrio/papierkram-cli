package papierkram_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// writeTree creates the files (slash paths relative to root) with their
// contents.
func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, src := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// testFile is a test file of package p with one top-level func.
func testFile(fn string) string {
	return "package p\n\nimport \"testing\"\n\nfunc " + fn + "(t *testing.T) {}\n"
}

// The scan sees the files `go test ./...` builds: it skips directories
// starting with _ or ., testdata, vendor, nested modules (own go.mod, e.g.
// a worktree) and files starting with _ or ..
func TestScanRegressionTestsFollowsGoToolDirRules(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"go.mod":                        "module x\n",
		"a/a_test.go":                   testFile("TestR01_Counted"),
		"_attic/old_test.go":            testFile("TestR02_Attic"),
		"a/_old/old_test.go":            testFile("TestR02_AtticNested"),
		".hidden/x_test.go":             testFile("TestR03_Hidden"),
		".claude/worktrees/w/go.mod":    "module x\n",
		".claude/worktrees/w/x_test.go": testFile("TestR01_Worktree"),
		"testdata/x_test.go":            testFile("TestR04_Testdata"),
		"vendor/v/x_test.go":            testFile("TestR05_Vendor"),
		"nested/go.mod":                 "module y\n",
		"nested/x_test.go":              testFile("TestR06_Nested"),
		"nested/deep/y_test.go":         testFile("TestR06_NestedDeep"),
		"a/_skip_test.go":               testFile("TestR07_UnderscoreFile"),
		"a/.dot_test.go":                testFile("TestR08_DotFile"),
	})
	found, bad, err := scanRegressionTests(root)
	if err != nil {
		t.Fatal(err)
	}
	want := map[int][]string{1: {"a/a_test.go"}}
	if !reflect.DeepEqual(found, want) || len(bad) != 0 {
		t.Errorf("found %v, bad %v, want %v and no bad names", found, bad, want)
	}
}

// Only top-level funcs count: not commented-out code, not raw strings, not
// methods. Files the default build never includes (//go:build ignore) are
// skipped; files with other constraints (OS, keychain) count.
func TestScanRegressionTestsCountsOnlyTopLevelFuncs(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"go.mod": "module x\n",
		"b_test.go": "package p\n\nimport \"testing\"\n\n" +
			"/*\nfunc TestR02_BlockComment(t *testing.T) {}\n*/\n\n" +
			"// func TestR03_LineComment(t *testing.T) {}\n\n" +
			"const src = `\nfunc TestR04_RawString(t *testing.T) {}\n`\n\n" +
			"type s struct{}\n\nfunc (s) TestR05_Method(t *testing.T) {}\n\n" +
			"func TestR01_Counted(t *testing.T) {}\n\n" +
			"func TestR9_OneDigit(t *testing.T) {}\n",
		"ignored_test.go":  "//go:build ignore\n\n" + testFile("TestR06_Ignored"),
		"ignored2_test.go": "//go:build linux && ignore\n\n" + testFile("TestR06_IgnoredToo"),
		"keychain_test.go": "//go:build keychain\n\n" + testFile("TestR07_Keychain"),
		"windows_test.go":  "//go:build windows\n\n" + testFile("TestR08_Windows"),
	})
	found, bad, err := scanRegressionTests(root)
	if err != nil {
		t.Fatal(err)
	}
	want := map[int][]string{1: {"b_test.go"}, 7: {"keychain_test.go"}, 8: {"windows_test.go"}}
	wantBad := []string{"TestR9_ in b_test.go"}
	if !reflect.DeepEqual(found, want) || !reflect.DeepEqual(bad, wantBad) {
		t.Errorf("found %v, bad %v, want %v, %v", found, bad, want, wantBad)
	}
}

// A test file that does not parse fails the scan instead of hiding its
// tests.
func TestScanRegressionTestsFailsOnSyntaxError(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"go.mod":      "module x\n",
		"bad_test.go": "package p\n\nfunc TestR01_Broken(\n",
	})
	if _, _, err := scanRegressionTests(root); err == nil {
		t.Error("no error for a file that does not parse")
	}
}
