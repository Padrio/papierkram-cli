// Package modroot finds the module root in tests, for checks that read files
// of the repository (spec markdown, all test files) independent of the
// package directory.
package modroot

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// Find returns the directory with go.mod nearest above the working
// directory of the test (the package directory).
func Find(t testing.TB) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root, err := findFrom(dir)
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// findFrom walks up from dir to the first directory with go.mod.
func findFrom(dir string) (string, error) {
	for {
		_, err := os.Stat(filepath.Join(dir, "go.mod"))
		if err == nil {
			return dir, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("go.mod not found")
		}
		dir = parent
	}
}
