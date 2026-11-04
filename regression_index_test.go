// Package papierkram_test holds module-wide checks that belong to no single
// package. It has no non-test files and is never built into the binary.
package papierkram_test

import (
	"fmt"
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

// regressionTest matches a test named like a regression test,
// TestRNN_<Kurzname>. It also matches other digit counts (TestR7_,
// TestR100_), which the test rejects: the number is always two digits.
var regressionTest = regexp.MustCompile(`(?m)^func TestR(\d+)_`)

// skipDirs are directories without tests of the module.
var skipDirs = map[string]bool{".git": true, ".superpowers": true, "vendor": true, "testdata": true}

// TestRegressionIndex enforces spec 16: every regression R01–R39 is a test
// named TestRNN_<Kurzname>, each exactly once across all test files of the
// module, and there are no others.
func TestRegressionIndex(t *testing.T) {
	root := modroot.Find(t)
	found := map[int][]string{}
	var badName []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		for _, m := range regressionTest.FindAllSubmatch(src, -1) {
			if len(m[1]) != 2 {
				badName = append(badName, fmt.Sprintf("TestR%s_ in %s", m[1], filepath.ToSlash(rel)))
				continue
			}
			n, _ := strconv.Atoi(string(m[1])) // two digits always parse
			found[n] = append(found[n], filepath.ToSlash(rel))
		}
		return nil
	})
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
