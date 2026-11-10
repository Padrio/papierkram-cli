// Command gennotices writes THIRD_PARTY_NOTICES.md: every module linked into
// a release binary (all release targets) with version, SPDX license and the
// full license and notice texts from the module cache, the Go standard
// library, and the other requirements of go.mod (tests, code generation,
// tools; not shipped) with their license.
//
// Licenses are not detected: licenses holds the SPDX expression of every
// module, entered after reading its license files. A module without an
// entry stops the generator, so a new dependency is never listed with a
// guessed license.
//
// Usage, from the module root:
//
//	go run ./tools/gennotices          # write THIRD_PARTY_NOTICES.md
//	go run ./tools/gennotices -check   # exit 1 when the file is outdated
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// output is the generated file, relative to the module root.
const output = "THIRD_PARTY_NOTICES.md"

// mainPackage is the package of the released binary.
const mainPackage = "./cmd/papierkram"

// targets are the release targets (spec 12.2), as GOOS/GOARCH.
var targets = []string{
	"darwin/amd64", "darwin/arm64",
	"linux/amd64", "linux/arm64",
	"windows/amd64", "windows/arm64",
}

// licenses is the SPDX expression of every module, verified by reading its
// license files (task 45). Keep it sorted.
var licenses = map[string]string{
	"github.com/apapsch/go-jsonmerge/v2":       "MIT",
	"github.com/cpuguy83/go-md2man/v2":         "MIT",
	"github.com/danieljoos/wincred":            "MIT",
	"github.com/dprotaso/go-yit":               "MIT",
	"github.com/getkin/kin-openapi":            "MIT",
	"github.com/go-openapi/jsonpointer":        "Apache-2.0",
	"github.com/go-openapi/swag/jsonname":      "Apache-2.0",
	"github.com/goccy/go-yaml":                 "MIT",
	"github.com/godbus/dbus/v5":                "BSD-2-Clause",
	"github.com/google/jsonschema-go":          "MIT",
	"github.com/google/uuid":                   "BSD-3-Clause",
	"github.com/inconshreveable/mousetrap":     "Apache-2.0",
	"github.com/itchyny/gojq":                  "MIT",
	"github.com/itchyny/timefmt-go":            "MIT",
	"github.com/modelcontextprotocol/go-sdk":   "Apache-2.0 AND MIT",
	"github.com/oapi-codegen/oapi-codegen/v2":  "Apache-2.0",
	"github.com/oapi-codegen/runtime":          "Apache-2.0",
	"github.com/oasdiff/yaml":                  "MIT AND BSD-3-Clause",
	"github.com/oasdiff/yaml3":                 "MIT AND Apache-2.0",
	"github.com/pelletier/go-toml/v2":          "MIT",
	"github.com/rogpeppe/go-internal":          "BSD-3-Clause",
	"github.com/russross/blackfriday/v2":       "BSD-2-Clause",
	"github.com/santhosh-tekuri/jsonschema/v6": "Apache-2.0",
	"github.com/segmentio/asm":                 "MIT",
	"github.com/segmentio/encoding":            "MIT AND BSD-3-Clause",
	"github.com/sergi/go-diff":                 "MIT AND Apache-2.0",
	"github.com/speakeasy-api/jsonpath":        "Apache-2.0",
	"github.com/speakeasy-api/openapi":         "MIT",
	"github.com/spf13/cobra":                   "Apache-2.0",
	"github.com/spf13/pflag":                   "BSD-3-Clause",
	"github.com/vmware-labs/yaml-jsonpath":     "Apache-2.0",
	"github.com/yosida95/uritemplate/v3":       "BSD-3-Clause",
	"github.com/zalando/go-keyring":            "MIT",
	"go.yaml.in/yaml/v3":                       "MIT AND Apache-2.0",
	"golang.org/x/mod":                         "BSD-3-Clause",
	"golang.org/x/oauth2":                      "BSD-3-Clause",
	"golang.org/x/sync":                        "BSD-3-Clause",
	"golang.org/x/sys":                         "BSD-3-Clause",
	"golang.org/x/term":                        "BSD-3-Clause",
	"golang.org/x/text":                        "BSD-3-Clause",
	"golang.org/x/time":                        "BSD-3-Clause",
	"golang.org/x/tools":                       "BSD-3-Clause",
	"gopkg.in/yaml.v3":                         "MIT AND Apache-2.0",
	stdlib:                                     "BSD-3-Clause",
}

// stdlib names the Go standard library and runtime in the notices.
const stdlib = "Go (Standardbibliothek und Laufzeit)"

// licenseFile matches the files of a module root that carry its license,
// notices or patent grant (LICENSE, LICENSE.md, COPYING, NOTICE,
// APACHE-LICENSE-2.0, PATENTS).
var licenseFile = regexp.MustCompile(`(?i)^(licen[cs]e|copying|notice|apache-license|patents)([-._].*)?$`)

// module is one module of the notices.
type module struct {
	Path, Version string
	Targets       []string // release targets that link it (binary modules)
	Dir           string   // module directory (binary modules)
}

// runner runs the go command with extra environment and returns stdout.
type runner func(env []string, args ...string) ([]byte, error)

func goCmd(env []string, args ...string) ([]byte, error) {
	cmd := exec.Command("go", args...)
	cmd.Env = append(os.Environ(), env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func main() {
	check := flag.Bool("check", false, "nur prüfen, ob "+output+" aktuell ist")
	flag.Parse()
	text, err := generate(goCmd)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gennotices:", err)
		os.Exit(1)
	}
	if *check {
		cur, err := os.ReadFile(output)
		if err != nil || normalize(string(cur)) != text {
			fmt.Fprintf(os.Stderr, "gennotices: %s ist veraltet; go run ./tools/gennotices ausführen\n", output)
			os.Exit(1)
		}
		return
	}
	if err := os.WriteFile(output, []byte(text), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "gennotices:", err)
		os.Exit(1)
	}
}

// normalize makes a checkout with CRLF line endings comparable.
func normalize(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }

// generate builds the content of THIRD_PARTY_NOTICES.md.
func generate(run runner) (string, error) {
	bin, err := binaryModules(run)
	if err != nil {
		return "", err
	}
	others, err := otherModules(run, bin)
	if err != nil {
		return "", err
	}
	for _, m := range append(slices.Clone(bin), others...) {
		if _, ok := licenses[m.Path]; !ok {
			return "", fmt.Errorf("keine Lizenz für %s eingetragen: Lizenzdateien lesen und in licenses ergänzen", m.Path)
		}
	}
	for i := range bin {
		out, err := run(nil, "mod", "download", "-json", bin[i].Path+"@"+bin[i].Version)
		if err != nil {
			return "", err
		}
		var d struct{ Dir string }
		if err := json.Unmarshal(out, &d); err != nil || d.Dir == "" {
			return "", fmt.Errorf("%s: Modulverzeichnis unbekannt (%v)", bin[i].Path, err)
		}
		bin[i].Dir = d.Dir
	}
	goroot, err := run(nil, "env", "GOROOT")
	if err != nil {
		return "", err
	}
	return render(bin, others, strings.TrimSpace(string(goroot)))
}

// binaryModules lists the modules that the release targets link into the
// binary, sorted by path, with the targets that link each.
func binaryModules(run runner) ([]module, error) {
	byPath := map[string]*module{}
	for _, t := range targets {
		goos, goarch, _ := strings.Cut(t, "/")
		out, err := run([]string{"GOOS=" + goos, "GOARCH=" + goarch, "CGO_ENABLED=0"},
			"list", "-deps", "-f", "{{with .Module}}{{if not .Main}}{{.Path}} {{.Version}}{{end}}{{end}}", mainPackage)
		if err != nil {
			return nil, err
		}
		seen := map[string]bool{}
		for _, line := range strings.Split(string(out), "\n") {
			path, version, ok := strings.Cut(strings.TrimSpace(line), " ")
			if !ok || seen[path] {
				continue
			}
			seen[path] = true
			m := byPath[path]
			if m == nil {
				m = &module{Path: path, Version: version}
				byPath[path] = m
			}
			if m.Version != version {
				return nil, fmt.Errorf("%s: Versionen %s und %s", path, m.Version, version)
			}
			m.Targets = append(m.Targets, t)
		}
	}
	var mods []module
	for _, m := range byPath {
		mods = append(mods, *m)
	}
	slices.SortFunc(mods, func(a, b module) int { return strings.Compare(a.Path, b.Path) })
	return mods, nil
}

// otherModules lists the requirements of go.mod that no release binary
// links, sorted by path.
func otherModules(run runner, bin []module) ([]module, error) {
	out, err := run(nil, "mod", "edit", "-json")
	if err != nil {
		return nil, err
	}
	var gm struct {
		Require []struct{ Path, Version string }
	}
	if err := json.Unmarshal(out, &gm); err != nil {
		return nil, fmt.Errorf("go mod edit -json: %w", err)
	}
	var mods []module
	for _, r := range gm.Require {
		if !slices.ContainsFunc(bin, func(m module) bool { return m.Path == r.Path }) {
			mods = append(mods, module{Path: r.Path, Version: r.Version})
		}
	}
	slices.SortFunc(mods, func(a, b module) int { return strings.Compare(a.Path, b.Path) })
	return mods, nil
}

// platforms describes the targets of a binary module for the table.
func platforms(ts []string) string {
	if len(ts) == len(targets) {
		return "alle"
	}
	var oses []string
	for _, t := range ts {
		goos, _, _ := strings.Cut(t, "/")
		if !slices.Contains(oses, goos) {
			oses = append(oses, goos)
		}
	}
	return "nur " + strings.Join(oses, ", ")
}

// licenseTexts returns the license and notice files of dir, sorted by name.
func licenseTexts(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.Type().IsRegular() && licenseFile.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("%s: keine Lizenzdatei", dir)
	}
	slices.Sort(names)
	return names, nil
}

// fence returns a code fence longer than every backtick run in text.
func fence(text string) string {
	longest := 0
	for _, run := range regexp.MustCompile("`+").FindAllString(text, -1) {
		longest = max(longest, len(run))
	}
	return strings.Repeat("`", max(3, longest+1))
}

// render writes the notices in German.
func render(bin, others []module, goroot string) (string, error) {
	var b strings.Builder
	b.WriteString(`# Hinweise zu Drittsoftware

Diese Datei erzeugt ` + "`go run ./tools/gennotices`" + ` aus den Modulen im Go-Modulcache; ein Test prüft, dass sie aktuell ist. Die Lizenzen sind nicht automatisch erkannt, sondern nach dem Lesen der Lizenzdateien eingetragen.

papierkram-cli selbst steht unter der MIT-Lizenz, siehe [LICENSE](LICENSE).

## Dank

Viele Fallen der Papierkram-API hat [papierkram-mcp-server](https://github.com/Wagner-Emden-IT-Services/papierkram-mcp-server) (MIT) live gefunden und dokumentiert. Dieses Projekt übernimmt von dort nur Wissen, keinen Code; papierkram-mcp-server ist daher keine Abhängigkeit und nicht im Binary enthalten.

## Im Binary enthalten

Diese Module bindet ` + "`go build`" + ` in das Binary ` + "`papierkram`" + ` der Release-Ziele (` + strings.Join(targets, ", ") + `) ein. Ihre Lizenz- und Hinweistexte stehen vollständig unten, ebenso die Patentzusagen (PATENTS) von Go und golang.org/x.

| Modul | Version | Lizenz (SPDX) | Plattformen |
|---|---|---|---|
| ` + stdlib + ` | Version der Go-Toolchain des Builds (` + "`papierkram version`" + `) | ` + licenses[stdlib] + ` | alle |
`)
	for _, m := range bin {
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n", m.Path, m.Version, licenses[m.Path], platforms(m.Targets))
	}
	b.WriteString(`
## Nicht im Binary enthalten

Diese Anforderungen aus ` + "`go.mod`" + ` (direkte und indirekte) nutzt das Projekt nur für Tests (z. B. ` + "`github.com/rogpeppe/go-internal`" + ` mit testscript), Code-Generierung (oapi-codegen) und Werkzeuge; sie werden nicht ausgeliefert. Der übrige Modulgraph (` + "`go list -m all`" + `) nennt weitere Module, die nur als Versionsangabe in Abhängigkeiten anderer Module vorkommen; sie werden weder gebaut noch ausgeliefert und sind deshalb nicht aufgeführt.

| Modul | Version | Lizenz (SPDX) |
|---|---|---|
`)
	for _, m := range others {
		fmt.Fprintf(&b, "| `%s` | %s | %s |\n", m.Path, m.Version, licenses[m.Path])
	}
	b.WriteString("\n## Lizenztexte\n")
	for _, name := range []string{"LICENSE", "PATENTS"} {
		data, err := readGoFile(goroot, name)
		if err != nil {
			return "", err
		}
		writeText(&b, stdlib, name, string(data))
	}
	for _, m := range bin {
		names, err := licenseTexts(m.Dir)
		if err != nil {
			return "", err
		}
		for _, n := range names {
			data, err := os.ReadFile(filepath.Join(m.Dir, n))
			if err != nil {
				return "", err
			}
			writeText(&b, m.Path+" "+m.Version, n, string(data))
		}
	}
	return b.String(), nil
}

// readGoFile reads a file of the Go distribution (LICENSE, PATENTS):
// GOROOT/name, or one level up where a package manager keeps GOROOT in a
// subdirectory (Homebrew keeps LICENSE outside libexec).
func readGoFile(goroot, name string) ([]byte, error) {
	var errs []error
	for _, p := range []string{filepath.Join(goroot, name), filepath.Join(filepath.Dir(goroot), name)} {
		data, err := os.ReadFile(p)
		if err == nil {
			return data, nil
		}
		errs = append(errs, err)
	}
	return nil, fmt.Errorf("keine Datei %s der Go-Distribution gefunden: %w", name, errors.Join(errs...))
}

// writeText adds one license or notice file under a heading.
func writeText(b *strings.Builder, title, name, text string) {
	text = strings.TrimRight(normalize(text), "\n") + "\n"
	f := fence(text)
	fmt.Fprintf(b, "\n### %s – %s\n\n%stext\n%s%s\n", title, name, f, text, f)
}
