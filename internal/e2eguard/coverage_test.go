package e2eguard

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/registry"
	"github.com/Padrio/papierkram-cli/internal/testutil/modroot"
)

// The plan names exactly the operations of the registry (spec 15: 89).
func TestCoveragePlanMatchesRegistry(t *testing.T) {
	var ids []string
	for _, e := range registry.Entries() {
		ids = append(ids, e.OperationID)
	}
	slices.Sort(ids)
	var keys []string
	for k := range CoveragePlan {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	if len(keys) != 89 || len(ids) != 89 {
		t.Errorf("plan %d entries, registry %d operations, want 89 each", len(keys), len(ids))
	}
	if !slices.Equal(keys, ids) {
		for _, id := range ids {
			if _, ok := CoveragePlan[id]; !ok {
				t.Errorf("operation %s missing from CoveragePlan", id)
			}
		}
		for _, k := range keys {
			if !slices.Contains(ids, k) {
				t.Errorf("CoveragePlan names %s, which is no registry operation", k)
			}
		}
	}
}

// Every expected operation names a test function of e2e/ (the part
// before "/") and, after "/", subtests that function runs by a string
// literal (review T1-M4); a pending one may name none yet.
func TestCoveragePlanTestsExist(t *testing.T) {
	funcs := e2eTestFuncs(t, filepath.Join(modroot.Find(t), "e2e"))
	for _, bad := range checkPlanTests(CoveragePlan, funcs) {
		t.Error(bad)
	}
}

// checkPlanTests lists the entries of plan whose Test names no function
// of funcs (top-level test name to the subtest names it runs) or a
// subtest that function does not run.
func checkPlanTests(plan map[string]Expectation, funcs map[string]map[string]bool) []string {
	var bad []string
	for _, id := range slices.Sorted(maps.Keys(plan)) {
		e := plan[id]
		switch e.Test {
		case "":
			if e.Pending == 0 {
				bad = append(bad, id+": expected (Pending 0) without Test")
			}
		case cleanupTest:
		default:
			top, rest, _ := strings.Cut(e.Test, "/")
			subs, ok := funcs[top]
			if !ok {
				bad = append(bad, fmt.Sprintf("%s: Test %q: no func %s in e2e/*_test.go", id, e.Test, top))
				break
			}
			if rest == "" {
				break
			}
			for sub := range strings.SplitSeq(rest, "/") {
				if !subs[sub] {
					bad = append(bad, fmt.Sprintf("%s: Test %q: %s runs no subtest %q (t.Run with a literal or a table field)", id, e.Test, top, sub))
				}
			}
		}
		if e.Pending < 0 {
			bad = append(bad, fmt.Sprintf("%s: Pending %d", id, e.Pending))
		}
	}
	return bad
}

// The check of the plan finds a missing function, a missing subtest at
// any level and a pending entry without test; nested subtests and the
// cleanup pass.
func TestCheckPlanTestsFindsViolations(t *testing.T) {
	funcs := map[string]map[string]bool{
		"TestE2E_A": {"01_X": true, "inner": true},
		"TestE2E_B": {},
	}
	plan := map[string]Expectation{
		"OpOK":       {Test: "TestE2E_A"},
		"OpSub":      {Test: "TestE2E_A/01_X"},
		"OpNested":   {Test: "TestE2E_A/01_X/inner"},
		"OpCleanup":  {Test: cleanupTest},
		"OpPending":  {Pending: 1},
		"OpNoFunc":   {Test: "TestE2E_C"},
		"OpNoSub":    {Test: "TestE2E_A/02_Y"},
		"OpNoNested": {Test: "TestE2E_A/01_X/other"},
		"OpEmpty":    {Test: "TestE2E_B/x"},
		"OpNoTest":   {},
		"OpNegative": {Test: "TestE2E_B", Pending: -1},
	}
	want := []string{
		`OpEmpty: Test "TestE2E_B/x": TestE2E_B runs no subtest "x" (t.Run with a literal or a table field)`,
		`OpNegative: Pending -1`,
		`OpNoFunc: Test "TestE2E_C": no func TestE2E_C in e2e/*_test.go`,
		`OpNoNested: Test "TestE2E_A/01_X/other": TestE2E_A runs no subtest "other" (t.Run with a literal or a table field)`,
		`OpNoSub: Test "TestE2E_A/02_Y": TestE2E_A runs no subtest "02_Y" (t.Run with a literal or a table field)`,
		`OpNoTest: expected (Pending 0) without Test`,
	}
	if got := checkPlanTests(plan, funcs); !slices.Equal(got, want) {
		t.Errorf("checkPlanTests =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// e2eTestFuncs finds subtests by literal, by a table of an anonymous or
// a package-level struct type (positional or keyed), nested, and not
// those of a computed name or of another range variable.
func TestE2ETestFuncsSubtests(t *testing.T) {
	dir := t.TempDir()
	src := `package e2e

import "testing"

type row struct {
	n    int
	name string
}

func TestA(t *testing.T) {
	t.Run("01 literal", func(t *testing.T) {
		t.Run("inner", func(t *testing.T) {})
	})
	cases := []struct {
		resource string
		id       int
	}{{"company", 1}, {resource: "task"}}
	for _, c := range cases {
		t.Run(c.resource, func(t *testing.T) {})
	}
	rows := []row{{1, "positional"}, {name: "keyed", n: 2}}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {})
		for _, c := range cases {
			t.Run(r.name+c.resource, func(t *testing.T) {})
		}
	}
	other := []row{{3, "unused"}}
	for _, o := range other {
		_ = o
		t.Run(fmt(), func(t *testing.T) {})
	}
}

func TestB(t *testing.T) {}

func helper() {}

func fmt() string { return "" }
`
	if err := os.WriteFile(filepath.Join(dir, "a_test.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	got := e2eTestFuncs(t, dir)
	want := map[string]map[string]bool{
		"TestA": {"01_literal": true, "inner": true, "company": true, "task": true, "positional": true, "keyed": true},
		"TestB": {},
	}
	if !maps.EqualFunc(got, want, maps.Equal) {
		t.Errorf("e2eTestFuncs = %v, want %v", got, want)
	}
}

// e2eTestFuncs are the top-level Test functions of the _test.go files in
// dir, each with the names of the subtests it runs (subtestNames).
func e2eTestFuncs(t *testing.T, dir string) map[string]map[string]bool {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "*_test.go"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no test files in %s: %v", dir, err)
	}
	fset := token.NewFileSet()
	var files []*ast.File
	structs := map[string]*ast.StructType{} // package-level struct types
	for _, p := range paths {
		f, err := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
		ast.Inspect(f, func(n ast.Node) bool {
			if ts, ok := n.(*ast.TypeSpec); ok {
				if st, ok := ts.Type.(*ast.StructType); ok {
					structs[ts.Name.Name] = st
				}
			}
			return true
		})
	}
	out := map[string]map[string]bool{}
	for _, f := range files {
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if ok && fn.Recv == nil && strings.HasPrefix(fn.Name.Name, "Test") {
				out[fn.Name.Name] = subtestNames(fn.Body, structs)
			}
		}
	}
	return out
}

// subtestNames are the names of the subtests body runs, spaces replaced
// by "_" as go test does: the string literal of t.Run("name", …), and for
// a table t.Run(c.field, …) inside "for _, c := range cases" the literal
// of that field in each element of the composite literal assigned to
// cases in body (cases := []T{…}, T a struct type of the function or of
// the package). Other computed names are not known.
func subtestNames(body *ast.BlockStmt, structs map[string]*ast.StructType) map[string]bool {
	tables := map[string]*ast.CompositeLit{}
	ast.Inspect(body, func(n ast.Node) bool {
		if as, ok := n.(*ast.AssignStmt); ok && len(as.Lhs) == 1 && len(as.Rhs) == 1 {
			if id, ok := as.Lhs[0].(*ast.Ident); ok {
				if cl, ok := as.Rhs[0].(*ast.CompositeLit); ok {
					tables[id.Name] = cl
				}
			}
		}
		return true
	})
	out := map[string]bool{}
	add := func(e ast.Expr) {
		if lit, ok := e.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			if name, err := strconv.Unquote(lit.Value); err == nil {
				out[strings.ReplaceAll(name, " ", "_")] = true
			}
		}
	}
	ast.Inspect(body, func(n ast.Node) bool {
		if arg := runArg(n); arg != nil {
			add(arg)
		}
		rs, ok := n.(*ast.RangeStmt)
		if !ok {
			return true
		}
		v, vok := rs.Value.(*ast.Ident)
		x, xok := rs.X.(*ast.Ident)
		if !vok || !xok || tables[x.Name] == nil {
			return true
		}
		ast.Inspect(rs.Body, func(n ast.Node) bool {
			sel, ok := runArg(n).(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); !ok || id.Name != v.Name {
				return true
			}
			for _, e := range tableField(tables[x.Name], sel.Sel.Name, structs) {
				add(e)
			}
			return true
		})
		return true
	})
	return out
}

// runArg is the first argument of a call x.Run(name, f), nil for any
// other node.
func runArg(n ast.Node) ast.Expr {
	call, ok := n.(*ast.CallExpr)
	if !ok || len(call.Args) != 2 {
		return nil
	}
	if sel, ok := call.Fun.(*ast.SelectorExpr); !ok || sel.Sel.Name != "Run" {
		return nil
	}
	return call.Args[0]
}

// tableField is the value of field in each element of the slice literal
// cl ([]T{{…}, …}), by key or by position in the struct type T.
func tableField(cl *ast.CompositeLit, field string, structs map[string]*ast.StructType) []ast.Expr {
	at, ok := cl.Type.(*ast.ArrayType)
	if !ok {
		return nil
	}
	st, ok := at.Elt.(*ast.StructType)
	if id, isIdent := at.Elt.(*ast.Ident); isIdent {
		st, ok = structs[id.Name]
	}
	if !ok {
		return nil
	}
	pos, i := -1, 0
	for _, f := range st.Fields.List {
		for _, n := range f.Names {
			if n.Name == field {
				pos = i
			}
			i++
		}
	}
	var out []ast.Expr
	for _, el := range cl.Elts {
		row, ok := el.(*ast.CompositeLit)
		if !ok {
			continue
		}
		for j, e := range row.Elts {
			if kv, ok := e.(*ast.KeyValueExpr); ok {
				if k, ok := kv.Key.(*ast.Ident); ok && k.Name == field {
					out = append(out, kv.Value)
				}
			} else if j == pos {
				out = append(out, e)
			}
		}
	}
	return out
}

// ModeIfData only for the bank details read by ID; ModeAnswered and
// ModeIfData give their reason; only deletes count in the cleanup.
func TestCoveragePlanModes(t *testing.T) {
	ifData := []string{"GetBankConnection", "GetTransaction"}
	for id, e := range CoveragePlan {
		if (e.Mode == ModeIfData) != slices.Contains(ifData, id) {
			t.Errorf("%s: Mode %d, ModeIfData only and always for %v", id, e.Mode, ifData)
		}
		if (e.Mode == ModeAnswered || e.Mode == ModeIfData) && strings.TrimSpace(e.Note) == "" {
			t.Errorf("%s: Mode %d without Note", id, e.Mode)
		}
		if e.Phase == PhaseAny && (!strings.HasPrefix(id, "Delete") || id == "DeleteVoucherDocument") {
			t.Errorf("%s: PhaseAny only for the deletes of the cleanup", id)
		}
	}
}

// Task 6 (round 2): archive/unarchive of the master data and the read
// commands are expected now, each by its subtest of
// TestE2E_MasterDataArchive or TestE2E_ReadCommands; nothing is left
// pending for task 6.
func TestCoveragePlanMasterDataAndReads(t *testing.T) {
	const archive, read = "TestE2E_MasterDataArchive/", "TestE2E_ReadCommands/"
	want := map[string]string{
		"ArchiveCompany":           archive + "company",
		"UnarchiveCompany":         archive + "company",
		"ArchiveProject":           archive + "project",
		"UnarchiveProject":         archive + "project",
		"ArchiveTask":              archive + "task",
		"UnarchiveTask":            archive + "task",
		"ArchiveProposition":       archive + "proposition",
		"UnarchiveProposition":     archive + "proposition",
		"ArchiveCustomAttribute":   archive + "custom-attribute",
		"UnarchiveCustomAttribute": archive + "custom-attribute",
		"ArchiveEstimate":          archive + "estimate",
		"UnarchiveEstimate":        archive + "estimate",
		"ListPersons":              read + "person_list",
		"GetUser":                  read + "user_describe",
		"GetPaymentTerm":           read + "payment_term_describe",
		"ListBankConnections":      read + "bank_connection",
		"GetBankConnection":        read + "bank_connection",
		"ListTransactions":         read + "transaction",
		"GetTransaction":           read + "transaction",
	}
	for _, id := range slices.Sorted(maps.Keys(want)) {
		if e := CoveragePlan[id]; e.Test != want[id] || e.Pending != 0 {
			t.Errorf("%s: Test %q, Pending %d; want %q, 0", id, e.Test, e.Pending, want[id])
		}
	}
	for id, e := range CoveragePlan {
		if e.Pending == 6 {
			t.Errorf("%s: still pending for task 6", id)
		}
	}
}

// Task 7 (round 2) closes the plan: every operation is expected by a
// full run, none is pending for a later task any more.
func TestCoveragePlanComplete(t *testing.T) {
	for _, id := range slices.Sorted(maps.Keys(CoveragePlan)) {
		if e := CoveragePlan[id]; e.Pending != 0 {
			t.Errorf("%s: Pending %d, want 0 (expected by a full run)", id, e.Pending)
		}
	}
}

func TestCoverageRecord(t *testing.T) {
	plan := map[string]Expectation{
		"DeliverInvoice": {Test: "TestA"},
		"GetInvoice":     {Test: "TestA"},
		"ListInvoices":   {Test: "TestA"},
		"GetCompany":     {Test: "TestA"},
		"DeleteCompany":  {Test: cleanupTest, Phase: PhaseAny},
		"DeleteTask":     {Test: cleanupTest, Phase: PhaseAny},
		"PayVoucher":     {Test: "TestB", Mode: ModeAnswered, Note: "n"},
		"CancelVoucher":  {Test: "TestB"},
	}
	c := NewCoverage()
	c.Record("POST", "/api/v1/income/invoices/12/deliver", 204)
	c.Record("GET", "/api/v1/income/invoices/12/?x=1", 200) // query and slash ignored
	c.Record("get", "/api/v1/income/invoices", 200)         // method in any case
	c.Record("GET", "/api/v1/nirgends/1", 200)
	c.Record("GET", "/api/v1/nirgends/1", 404) // listed once
	c.Record("POST", "/api/v1/expense/vouchers/7/pay", 422)
	c.Record("POST", "/api/v1/expense/vouchers/7/cancel", 422) // ModeSuccess: an error does not count
	c.Record("DELETE", "/api/v1/tracker/tasks/3", 200)         // test phase
	c.BeginCleanup()
	c.Record("GET", "/api/v1/contact/companies/5", 200) // PhaseTest: the cleanup does not count
	c.Record("DELETE", "/api/v1/contact/companies/5", 200)
	c.Record("DELETE", "/api/v1/tracker/tasks/3", 200)

	r := c.Report(plan)
	if want := []string{"DeleteCompany", "DeleteTask", "DeliverInvoice", "GetInvoice", "ListInvoices", "PayVoucher"}; !slices.Equal(r.Covered, want) {
		t.Errorf("Covered %v, want %v", r.Covered, want)
	}
	if want := []string{"PayVoucher"}; !slices.Equal(r.OnlyAnswered, want) {
		t.Errorf("OnlyAnswered %v, want %v", r.OnlyAnswered, want)
	}
	if want := []string{"DeleteCompany"}; !slices.Equal(r.OnlyCleanup, want) {
		t.Errorf("OnlyCleanup %v, want %v", r.OnlyCleanup, want)
	}
	if want := []string{"CancelVoucher", "GetCompany"}; !slices.Equal(r.Missing, want) {
		t.Errorf("Missing %v, want %v", r.Missing, want)
	}
	if want := []string{"GET /api/v1/nirgends/1"}; !slices.Equal(r.Unknown, want) {
		t.Errorf("Unknown %v, want %v", r.Unknown, want)
	}
	if len(r.Pending) != 0 || len(r.NotApplicable) != 0 {
		t.Errorf("Pending %v, NotApplicable %v, want none", r.Pending, r.NotApplicable)
	}
}

// ModeIfData: covered by a 2xx; without one, NotApplicable is required.
// NotApplicable does not excuse an operation of another mode.
func TestCoverageIfData(t *testing.T) {
	plan := map[string]Expectation{
		"GetTransaction":    {Test: "TestA", Mode: ModeIfData, Note: "n"},
		"GetBankConnection": {Test: "TestA", Mode: ModeIfData, Note: "n"},
		"ListTransactions":  {Test: "TestA"},
		"GetUser":           {Test: "TestA", Mode: ModeIfData, Note: "n"},
	}
	c := NewCoverage()
	c.NotApplicable("GetTransaction", "keine Umsätze")
	c.NotApplicable("ListTransactions", "keine Umsätze")
	c.Record("GET", "/api/v1/users/4", 200)
	c.NotApplicable("GetUser", "egal") // covered wins
	r := c.Report(plan)
	if want := []string{"GetTransaction"}; !slices.Equal(r.NotApplicable, want) {
		t.Errorf("NotApplicable %v, want %v", r.NotApplicable, want)
	}
	if want := []string{"GetBankConnection", "ListTransactions"}; !slices.Equal(r.Missing, want) {
		t.Errorf("Missing %v, want %v", r.Missing, want)
	}
	if want := []string{"GetUser"}; !slices.Equal(r.Covered, want) {
		t.Errorf("Covered %v, want %v", r.Covered, want)
	}
}

func TestCoverageReportLines(t *testing.T) {
	plan := map[string]Expectation{
		"GetInfo":        {Test: "TestE2E_InfoCreditCost"},
		"DeleteTask":     {Test: cleanupTest, Phase: PhaseAny},
		"PayVoucher":     {Test: "TestE2E_Carries/10_VoucherPayValueNull", Mode: ModeAnswered, Note: "n"},
		"ListUsers":      {Test: "TestE2E_QuotaHeadersPresent"},
		"GetCompany":     {Test: "TestE2E_PartialUpdateSemantics/company"},
		"GetTransaction": {Test: "TestE2E_ReadCommands/transaction", Mode: ModeIfData, Note: "n"},
		"DeliverInvoice": {Pending: 3},
		"ArchiveTask":    {Pending: 6},
	}
	c := NewCoverage()
	c.Record("GET", "/api/v1/info", 200)
	c.Record("POST", "/api/v1/expense/vouchers/7/pay", 422)
	c.Record("DELETE", "/api/v1/foo", 404)
	c.Record("GET", "/api/v1/bar", 200)
	c.NotApplicable("GetTransaction", "keine Umsätze in der Sandbox")
	c.BeginCleanup()
	c.Record("DELETE", "/api/v1/tracker/tasks/3", 204)

	got := c.Report(plan).Lines
	want := []string{
		"ABDECKUNG: abgedeckt 3/8 (davon 1 nur mit Fehlerantwort, 1 nur im Aufräumen), nicht anwendbar 1, ausstehend 2, fehlend 2",
		"ABDECKUNG fehlt: GetCompany (erwartet von TestE2E_PartialUpdateSemantics/company)",
		"ABDECKUNG fehlt: ListUsers (erwartet von TestE2E_QuotaHeadersPresent)",
		"ABDECKUNG nicht anwendbar: GetTransaction: keine Umsätze in der Sandbox",
		"ABDECKUNG ausstehend: ArchiveTask (Task 6)",
		"ABDECKUNG ausstehend: DeliverInvoice (Task 3)",
		"ABDECKUNG unbekannter Pfad: DELETE /api/v1/foo",
		"ABDECKUNG unbekannter Pfad: GET /api/v1/bar",
	}
	if !slices.Equal(got, want) {
		t.Errorf("Lines\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// With nothing recorded, the real plan reports every expected operation
// as missing and every pending one as pending.
func TestCoverageReportEmptyRun(t *testing.T) {
	r := NewCoverage().Report(CoveragePlan)
	pending := 0
	for _, e := range CoveragePlan {
		if e.Pending != 0 {
			pending++
		}
	}
	if len(r.Covered) != 0 || len(r.Pending) != pending || len(r.Missing) != len(CoveragePlan)-pending {
		t.Errorf("Covered %d, Pending %d, Missing %d; want 0, %d, %d", len(r.Covered), len(r.Pending), len(r.Missing), pending, len(CoveragePlan)-pending)
	}
	if !strings.HasPrefix(r.Lines[0], "ABDECKUNG: abgedeckt 0/89 (") {
		t.Errorf("summary %q", r.Lines[0])
	}
}

// A nil Coverage (Transport without tracker) ignores every call.
func TestCoverageNil(t *testing.T) {
	var c *Coverage
	c.Record("GET", "/api/v1/info", 200)
	c.BeginCleanup()
	c.NotApplicable("GetInfo", "x")
	plan := map[string]Expectation{
		"GetInfo":   {Test: "TestE2E_InfoCreditCost", Mode: ModeSuccess},
		"ListUsers": {Test: "TestE2E_QuotaHeadersPresent", Mode: ModeSuccess, Pending: 3},
	}
	r := c.Report(plan)
	if len(r.Covered) != 0 || len(r.NotApplicable) != 0 || !slices.Equal(r.Missing, []string{"GetInfo"}) || !slices.Equal(r.Pending, []string{"ListUsers"}) {
		t.Errorf("Report of nil = %+v; want only GetInfo missing, ListUsers pending", r)
	}
	want := "ABDECKUNG: abgedeckt 0/2 (davon 0 nur mit Fehlerantwort, 0 nur im Aufräumen), nicht anwendbar 0, ausstehend 1, fehlend 1"
	if len(r.Lines) == 0 || r.Lines[0] != want {
		t.Errorf("Lines %q; want first %q", r.Lines, want)
	}
}
