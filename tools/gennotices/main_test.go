package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// THIRD_PARTY_NOTICES.md matches the modules of the release binaries: a new
// or updated dependency needs a regenerated file (and, when it is new, a
// license entry after reading its license).
func TestNoticesUpToDate(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go-Befehl nicht gefunden")
	}
	t.Chdir("../..")
	want, err := generate(goCmd)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if normalize(string(got)) != want {
		t.Errorf("%s ist veraltet: go run ./tools/gennotices ausführen", output)
	}
	// the carries of task 45 name these explicitly
	for _, s := range []string{
		"| `github.com/cpuguy83/go-md2man/v2` | v2.0.6 | MIT |",
		"| `github.com/russross/blackfriday/v2` | v2.1.0 | BSD-2-Clause |",
		"| `github.com/rogpeppe/go-internal` | v1.16.0 | BSD-3-Clause |",
		"papierkram-mcp-server",
	} {
		if !strings.Contains(want, s) {
			t.Errorf("%q fehlt", s)
		}
	}
}

// fakeGo answers the go commands of generate from canned output.
func fakeGo(list string) runner {
	return func(_ []string, args ...string) ([]byte, error) {
		switch args[0] {
		case "list":
			return []byte(list), nil
		case "mod":
			if args[1] == "edit" {
				return []byte(`{"Require":[{"Path":"golang.org/x/sys","Version":"v1"}]}`), nil
			}
		}
		return nil, exec.ErrNotFound
	}
}

func TestGenerateRefusesModulesWithoutLicenseEntry(t *testing.T) {
	_, err := generate(fakeGo("golang.org/x/sys v1\nexample.com/neu v0.1.0\n"))
	if err == nil || !strings.Contains(err.Error(), "example.com/neu") {
		t.Fatalf("err = %v, want refusal naming example.com/neu", err)
	}
}

func TestLicenseFilePattern(t *testing.T) {
	for _, name := range []string{"LICENSE", "LICENSE.md", "LICENSE.txt", "License", "LICENCE", "COPYING", "NOTICE", "APACHE-LICENSE-2.0"} {
		if !licenseFile.MatchString(name) {
			t.Errorf("%s: nicht erkannt", name)
		}
	}
	for _, name := range []string{"PATENTS", "AUTHORS", "README.md", "licenses.go", "go.mod"} {
		if licenseFile.MatchString(name) {
			t.Errorf("%s: als Lizenzdatei erkannt", name)
		}
	}
}

func TestFenceIsLongerThanBacktickRuns(t *testing.T) {
	for text, want := range map[string]string{
		"plain":           "```",
		"a ``` b":         "````",
		"x ````` y ``` z": "``````",
	} {
		if got := fence(text); got != want {
			t.Errorf("fence(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestPlatforms(t *testing.T) {
	if got := platforms(targets); got != "alle" {
		t.Errorf("alle Ziele: %q", got)
	}
	if got := platforms([]string{"windows/amd64", "windows/arm64"}); got != "nur windows" {
		t.Errorf("windows: %q", got)
	}
}
