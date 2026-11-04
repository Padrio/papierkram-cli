package registry

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/spec"
	"github.com/Padrio/papierkram-cli/internal/testutil/modroot"
)

// designSpec is the design spec, relative to the module root.
const designSpec = "docs/specs/2026-11-03-papierkram-cli-design.md"

// operationCount is the number of operations of the API (spec 15).
const operationCount = 89

// matrixRow is one row of the coverage matrix of spec 15.
type matrixRow struct {
	no                    int
	method, path, command string
	class                 Class
	credits               int
}

// matrixColumns maps the German column labels of the spec 15 table to the
// fields of matrixRow: Pfad = path, Klasse = class.
var matrixColumns = []string{"#", "Method", "Pfad", "Command", "Klasse", "Credits"}

// matrixClasses maps the class names of the spec 15 table to the classes of
// spec 8.1. The table uses the same English names as the registry; the map
// is explicit so that a renamed class in either place fails the test.
var matrixClasses = map[string]Class{
	"read_only":   ClassReadOnly,
	"create":      ClassCreate,
	"update":      ClassUpdate,
	"archive":     ClassArchive,
	"destructive": ClassDestructive,
	"send":        ClassSend,
}

// TestCoverageAllOperations is the coverage gate of spec 15: every spec
// operation has exactly one registry entry and vice versa (89 each), and
// every row of the coverage matrix in the design spec matches its entry in
// method, path, command, class and credits. The matrix is parsed from the
// spec markdown, so it cannot drift from a copy in the test.
func TestCoverageAllOperations(t *testing.T) {
	ops := spec.Operations()
	entries := Entries()
	rows := parseMatrix(t, filepath.Join(modroot.Find(t), designSpec))
	if len(ops) != operationCount || len(entries) != operationCount || len(rows) != operationCount {
		t.Errorf("spec operations %d, registry entries %d, matrix rows %d, want %d each",
			len(ops), len(entries), len(rows), operationCount)
	}

	// spec → registry: every operation has exactly one entry.
	perOp := map[string]int{}
	for _, e := range entries {
		perOp[e.OperationID]++
	}
	specOps := map[string]spec.Operation{}
	for _, op := range ops {
		specOps[op.ID] = op
		if n := perOp[op.ID]; n != 1 {
			t.Errorf("spec operation %s (%s %s): %d registry entries, want 1", op.ID, op.Method, op.Path, n)
		}
	}

	// registry → spec: every entry belongs to a spec operation with the
	// same method and path.
	byRoute := map[string]Entry{}
	for _, e := range entries {
		op, ok := specOps[e.OperationID]
		if !ok {
			t.Errorf("registry entry %s %s: operation %q not in spec", e.Resource, e.Action, e.OperationID)
			continue
		}
		if e.Method != op.Method || e.Path != op.Path {
			t.Errorf("registry entry %s: %s %s, spec has %s %s", e.OperationID, e.Method, e.Path, op.Method, op.Path)
		}
		byRoute[e.Method+" "+e.Path] = e
	}

	// matrix → registry: the table lists method and path but no operation
	// ID, so rows are matched by route. Custom entries are included.
	seenRoute := map[string]int{}
	for _, r := range rows {
		route := r.method + " " + r.path
		if prev, dup := seenRoute[route]; dup {
			t.Errorf("matrix no. %d: %s already in no. %d", r.no, route, prev)
			continue
		}
		seenRoute[route] = r.no
		e, ok := byRoute[route]
		if !ok {
			t.Errorf("matrix no. %d: %s has no registry entry", r.no, route)
			continue
		}
		name := strings.TrimSpace("papierkram " + e.Resource + " " + e.Action)
		if r.command != name && !strings.HasPrefix(r.command, name+" ") {
			t.Errorf("matrix no. %d (%s): command %q, registry has %q", r.no, route, r.command, name)
		}
		if e.Class != r.class {
			t.Errorf("matrix no. %d (%s): class %q, registry has %q", r.no, route, r.class, e.Class)
		}
		if e.Credits != r.credits {
			t.Errorf("matrix no. %d (%s): credits %d, registry has %d", r.no, route, r.credits, e.Credits)
		}
	}
	// registry → matrix: no entry is missing from the table.
	for route, e := range byRoute {
		if _, ok := seenRoute[route]; !ok {
			t.Errorf("registry entry %s (%s) not in the matrix of spec 15", e.OperationID, route)
		}
	}
}

// parseMatrix reads the table of section 15 from the design spec. It fails
// on a changed header, a malformed row or a row number out of sequence.
func parseMatrix(t *testing.T, file string) []matrixRow {
	t.Helper()
	f, err := os.Open(file)
	if err != nil {
		t.Fatalf("open design spec: %v", err)
	}
	defer func() { _ = f.Close() }()

	var rows []matrixRow
	inSection, header := false, false
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "## ") {
			if inSection {
				break
			}
			inSection = strings.HasPrefix(line, "## 15. ")
			continue
		}
		if !inSection || !strings.HasPrefix(line, "|") {
			continue
		}
		cells := tableCells(line)
		switch {
		case !header:
			if strings.Join(cells, ",") != strings.Join(matrixColumns, ",") {
				t.Fatalf("spec 15: header %q, want %q", cells, matrixColumns)
			}
			header = true
		case strings.Trim(strings.Join(cells, ""), "-") == "":
			// separator row
		default:
			rows = append(rows, matrixRowOf(t, cells, len(rows)+1))
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("read design spec: %v", err)
	}
	if !header {
		t.Fatalf("spec 15: no coverage table in %s", file)
	}
	return rows
}

// tableCells splits a markdown table row into its cells without backticks;
// an escaped pipe (\|) stays part of the cell.
func tableCells(line string) []string {
	line = strings.ReplaceAll(strings.Trim(line, "|"), `\|`, "\x00")
	cells := strings.Split(line, "|")
	for i, c := range cells {
		cells[i] = strings.ReplaceAll(strings.Trim(strings.TrimSpace(c), "`"), "\x00", "|")
	}
	return cells
}

func matrixRowOf(t *testing.T, cells []string, wantNo int) matrixRow {
	t.Helper()
	if len(cells) != len(matrixColumns) {
		t.Fatalf("spec 15: row %q has %d cells, want %d", cells, len(cells), len(matrixColumns))
	}
	no, err := strconv.Atoi(cells[0])
	if err != nil || no != wantNo {
		t.Fatalf("spec 15: row number %q, want %d", cells[0], wantNo)
	}
	class, ok := matrixClasses[cells[4]]
	if !ok {
		t.Fatalf("spec 15 no. %d: unknown class %q", no, cells[4])
	}
	credits, err := strconv.Atoi(cells[5])
	if err != nil {
		t.Fatalf("spec 15 no. %d: credits %q: %v", no, cells[5], err)
	}
	return matrixRow{no: no, method: cells[1], path: cells[2], command: cells[3], class: class, credits: credits}
}
