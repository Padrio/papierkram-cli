package modroot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindIsTheModuleRoot(t *testing.T) {
	want, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if got := Find(t); got != want {
		t.Errorf("Find = %q, want %q", got, want)
	}
}

func TestFindFromWalksUpToTheNearestGoMod(t *testing.T) {
	dir := t.TempDir()
	mod := filepath.Join(dir, "a")
	deep := filepath.Join(mod, "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mod, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, start := range []string{mod, deep} {
		got, err := findFrom(start)
		if err != nil || got != mod {
			t.Errorf("findFrom(%q) = %q, %v, want %q", start, got, err, mod)
		}
	}
}
