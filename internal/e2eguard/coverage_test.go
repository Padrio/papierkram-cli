package e2eguard

import (
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"path/filepath"
	"slices"
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
// before "/"); a pending one may name none yet.
func TestCoveragePlanTestsExist(t *testing.T) {
	funcs := e2eTestFuncs(t, filepath.Join(modroot.Find(t), "e2e"))
	for id, e := range CoveragePlan {
		switch e.Test {
		case "":
			if e.Pending == 0 {
				t.Errorf("%s: expected (Pending 0) without Test", id)
			}
		case cleanupTest:
		default:
			top, _, _ := strings.Cut(e.Test, "/")
			if !funcs[top] {
				t.Errorf("%s: Test %q: no func %s in e2e/*_test.go", id, e.Test, top)
			}
		}
		if e.Pending < 0 {
			t.Errorf("%s: Pending %d", id, e.Pending)
		}
	}
}

// e2eTestFuncs are the names of the top-level Test functions of the
// _test.go files in dir.
func e2eTestFuncs(t *testing.T, dir string) map[string]bool {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "*_test.go"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no test files in %s: %v", dir, err)
	}
	out := map[string]bool{}
	fset := token.NewFileSet()
	for _, p := range paths {
		f, err := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil && strings.HasPrefix(fn.Name.Name, "Test") {
				out[fn.Name.Name] = true
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
}
