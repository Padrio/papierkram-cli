package main

import (
	"errors"
	"os"
	"os/exec"
	"regexp"
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
		// a cold module cache without network (modules of another OS);
		// in CI the error stays fatal
		if os.Getenv("CI") == "" && missingModules(err) {
			t.Skipf("Module fehlen im Cache, erst go mod download ausführen: %v", err)
		}
		t.Fatal(err)
	}
	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if normalize(string(got)) != want {
		t.Errorf("%s ist veraltet: go run ./tools/gennotices ausführen", output)
	}
	// the carries of task 45 name these explicitly (module and license;
	// the version follows go.mod, so a dependency update keeps this test)
	for _, m := range []struct{ path, license string }{
		{"github.com/cpuguy83/go-md2man/v2", "MIT"},
		{"github.com/russross/blackfriday/v2", "BSD-2-Clause"},
		{"github.com/rogpeppe/go-internal", "BSD-3-Clause"},
	} {
		re := regexp.MustCompile("\\| `" + regexp.QuoteMeta(m.path) + "` \\| v[^ |]+ \\| " + regexp.QuoteMeta(m.license) + " \\|")
		if !re.MatchString(want) {
			t.Errorf("Zeile für %s (%s) fehlt", m.path, m.license)
		}
	}
	if !strings.Contains(want, "papierkram-mcp-server") {
		t.Errorf("%q fehlt", "papierkram-mcp-server")
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
	for _, name := range []string{"LICENSE", "LICENSE.md", "LICENSE.txt", "License", "LICENCE", "COPYING", "NOTICE", "APACHE-LICENSE-2.0", "PATENTS"} {
		if !licenseFile.MatchString(name) {
			t.Errorf("%s: nicht erkannt", name)
		}
	}
	for _, name := range []string{"AUTHORS", "README.md", "licenses.go", "go.mod"} {
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

// missingModules reports a go command error that only says modules could
// not be fetched (offline, GOPROXY=off, GOFLAGS=-mod=mod without network).
func missingModules(err error) bool {
	msg := err.Error()
	for _, s := range []string{"module lookup disabled", "dial tcp", "no such host", "cannot find module providing package", "missing go.sum entry"} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

func TestMissingModules(t *testing.T) {
	if !missingModules(errors.New("go list -deps: exit status 1: github.com/godbus/dbus/v5@v5.2.2: module lookup disabled by GOPROXY=off")) {
		t.Error("GOPROXY=off nicht erkannt")
	}
	if missingModules(errors.New("keine Lizenz für example.com/neu eingetragen")) {
		t.Error("fehlender Lizenzeintrag als Cache-Problem erkannt")
	}
}
