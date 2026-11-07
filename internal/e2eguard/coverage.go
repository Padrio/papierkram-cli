package e2eguard

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/Padrio/papierkram-cli/internal/spec"
)

// Mode is what an operation needs to count as covered by a live run.
type Mode int

const (
	// ModeSuccess needs at least one 2xx answer.
	ModeSuccess Mode = iota
	// ModeAnswered needs any answered request, whatever its status: for
	// operations whose live semantics are unknown (e.g. a pay that must
	// fail, a status change whose precondition is not known).
	ModeAnswered
	// ModeIfData needs a 2xx answer when the account has data; otherwise
	// the suite must report the operation with Coverage.NotApplicable.
	ModeIfData
)

// Phase is when a request of an operation counts.
type Phase int

const (
	// PhaseTest counts only requests before Coverage.BeginCleanup.
	PhaseTest Phase = iota
	// PhaseAny counts requests of the tests and of the final cleanup.
	PhaseAny
)

// Expectation is how the live suite covers one operation.
type Expectation struct {
	// Test is the Go test name that covers the operation first (subtests
	// with "/"); TestMain/cleanup is the final cleanup of TestMain. Empty
	// while the operation is pending.
	Test  string
	Mode  Mode
	Phase Phase
	// Pending is the task of the plan that covers the operation; 0 means
	// it is expected now.
	Pending int
	// Note gives the reason for ModeAnswered and ModeIfData.
	Note string
}

const (
	notePay            = "nie echt bezahlt: nur value null mit NoTransactionID, 4xx erwartet"
	noteCancelMeasured = "Statuswechsel; Voraussetzung live unbekannt, nur gemessen"
	noteCancelVoucher  = "Statuswechsel; Voraussetzung live unbekannt"
	noteIfData         = "nur wenn die Liste Einträge hat"
	cleanupTest        = "TestMain/cleanup"
	testLeftover       = "TestE2E_CleanupLeftovers"
	testDeliver        = "TestE2E_DeliverPdfEmptyBody"
	testArticle        = "TestE2E_VatRate019Is19Percent/Artikel"
	testPartial        = "TestE2E_PartialUpdateSemantics/"
	testInvoice        = "TestE2E_InvoiceFlow"
	testVoucher        = "TestE2E_VoucherFlow"
)

// CoveragePlan names, for every operation of the registry (spec 15), how
// a full live run covers it. TestMain fails a full run in which an
// expected operation (Pending 0) is missing (see Coverage.Report).
var CoveragePlan = map[string]Expectation{
	// banking
	"ListBankConnections": {Pending: 6},
	"GetBankConnection":   {Mode: ModeIfData, Pending: 6, Note: noteIfData},
	"ListTransactions":    {Pending: 6},
	"GetTransaction":      {Mode: ModeIfData, Pending: 6, Note: noteIfData},

	// companies and persons
	"ListCompanies":    {Test: testLeftover},
	"CreateCompany":    {Test: testDeliver},
	"GetCompany":       {Test: testPartial + "company"},
	"UpdateCompany":    {Test: testPartial + "company"},
	"DeleteCompany":    {Test: cleanupTest, Phase: PhaseAny},
	"ArchiveCompany":   {Pending: 6},
	"UnarchiveCompany": {Pending: 6},
	"ListPersons":      {Pending: 6},
	"CreatePerson":     {Test: testDeliver},
	"GetPerson":        {Test: "TestE2E_PersonDefaultIsString"},
	"UpdatePerson":     {Test: testPartial + "person"},
	"DeletePerson":     {Test: cleanupTest, Phase: PhaseAny},

	// vouchers
	"UploadVoucherDocument":         {Test: "TestE2E_UploadMimeTypes"},
	"DeleteVoucherDocument":         {Test: testVoucher},
	"ListVouchers":                  {Test: testLeftover},
	"CreateVoucher":                 {Test: "TestE2E_UploadMimeTypes"},
	"GetVoucher":                    {Test: testPartial + "voucher"},
	"UpdateVoucher":                 {Test: testPartial + "voucher"},
	"DeleteVoucher":                 {Test: cleanupTest, Phase: PhaseAny},
	"ArchiveVoucher":                {Test: testVoucher},
	"UnarchiveVoucher":              {Test: testVoucher},
	"CancelVoucher":                 {Test: testVoucher, Mode: ModeAnswered, Note: noteCancelVoucher},
	"CancelVoucherWithReverseEntry": {Test: testVoucher, Mode: ModeAnswered, Note: noteCancelVoucher},
	"PayVoucher":                    {Test: "TestE2E_Carries/10_VoucherPayValueNull", Mode: ModeAnswered, Note: notePay},
	"GetVoucherPdf":                 {Test: testVoucher},

	// estimates
	"ListEstimates":     {Test: testLeftover},
	"CreateEstimate":    {Test: testDeliver},
	"GetEstimate":       {Test: testDeliver},
	"UpdateEstimate":    {Test: testPartial + "estimate"},
	"DeleteEstimate":    {Test: cleanupTest, Phase: PhaseAny},
	"ArchiveEstimate":   {Pending: 6},
	"UnarchiveEstimate": {Pending: 6},
	"CancelEstimate":    {Test: testInvoice, Mode: ModeAnswered, Note: noteCancelMeasured},
	"DeliverEstimate":   {Test: testDeliver},
	"GetEstimatePdf":    {Test: "TestE2E_Carries/08_PDFDownloadAccept"},

	// invoices
	"ListInvoices":     {Test: testLeftover},
	"CreateInvoice":    {Test: testPartial + "invoice"},
	"GetInvoice":       {Test: testPartial + "invoice"},
	"UpdateInvoice":    {Test: testPartial + "invoice"},
	"DeleteInvoice":    {Test: cleanupTest, Phase: PhaseAny},
	"ArchiveInvoice":   {Test: testInvoice},
	"UnarchiveInvoice": {Test: testInvoice},
	"CancelInvoice":    {Test: testInvoice, Mode: ModeAnswered, Note: noteCancelMeasured},
	"DeliverInvoice":   {Test: testInvoice},
	"GetInvoicePdf":    {Test: testInvoice},

	// payment terms, propositions, info
	"ListPaymentTerms":     {Test: testPartial + "invoice"},
	"GetPaymentTerm":       {Pending: 6},
	"ListPropositions":     {Test: testLeftover},
	"CreateProposition":    {Test: testArticle},
	"GetProposition":       {Test: testArticle},
	"UpdateProposition":    {Test: testPartial + "proposition"},
	"DeleteProposition":    {Test: cleanupTest, Phase: PhaseAny},
	"ArchiveProposition":   {Pending: 6},
	"UnarchiveProposition": {Pending: 6},
	"GetInfo":              {Test: "TestE2E_InfoCreditCost"},

	// projects and custom attributes
	"ListProjects":             {Test: testLeftover},
	"CreateProject":            {Test: testDeliver},
	"GetProject":               {Test: testPartial + "project"},
	"UpdateProject":            {Test: testPartial + "project"},
	"DeleteProject":            {Test: cleanupTest, Phase: PhaseAny},
	"ArchiveProject":           {Pending: 6},
	"UnarchiveProject":         {Pending: 6},
	"ListCustomAttributes":     {Test: testLeftover},
	"CreateCustomAttribute":    {Test: testPartial + "custom-attribute"},
	"GetCustomAttribute":       {Test: testPartial + "custom-attribute"},
	"UpdateCustomAttribute":    {Test: testPartial + "custom-attribute"},
	"DeleteCustomAttribute":    {Test: cleanupTest, Phase: PhaseAny},
	"ArchiveCustomAttribute":   {Pending: 6},
	"UnarchiveCustomAttribute": {Pending: 6},

	// tasks and time entries
	"ListTasks":          {Test: testLeftover},
	"CreateTask":         {Test: testPartial + "task"},
	"GetTask":            {Test: testPartial + "task"},
	"UpdateTask":         {Test: testPartial + "task"},
	"DeleteTask":         {Test: cleanupTest, Phase: PhaseAny},
	"ArchiveTask":        {Pending: 6},
	"UnarchiveTask":      {Pending: 6},
	"ListTimeEntries":    {Test: testLeftover},
	"CreateTimeEntry":    {Test: testPartial + "time"},
	"GetTimeEntry":       {Test: testPartial + "time"},
	"UpdateTimeEntry":    {Test: testPartial + "time"},
	"DeleteTimeEntry":    {Test: cleanupTest, Phase: PhaseAny},
	"ArchiveTimeEntry":   {Pending: 5},
	"UnarchiveTimeEntry": {Pending: 5},

	// users
	"ListUsers": {Test: "TestE2E_QuotaHeadersPresent"},
	"GetUser":   {Pending: 6},
}

// hits counts the answers of one operation by phase (0 test, 1 cleanup).
type hits struct {
	ok, other [2]int // 2xx, any other status
}

// Coverage records which operations a live run reached. It is safe for
// concurrent use; a nil *Coverage ignores every call.
type Coverage struct {
	mu            sync.Mutex
	cleanup       bool
	ops           map[string]*hits
	unknown       map[string]bool // "METHOD path"
	notApplicable map[string]string
}

// NewCoverage returns an empty tracker in the test phase.
func NewCoverage() *Coverage {
	return &Coverage{ops: map[string]*hits{}, unknown: map[string]bool{}, notApplicable: map[string]string{}}
}

// BeginCleanup starts the cleanup phase: later requests count only for
// expectations with PhaseAny.
func (c *Coverage) BeginCleanup() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cleanup = true
}

// Record counts one answered request. The operation is resolved with
// spec.MatchPath (prefix /api/v1, query and trailing slash ignored); a
// path without operation is kept as unknown.
func (c *Coverage) Record(method, path string, status int) {
	if c == nil {
		return
	}
	op, _, ok := spec.MatchPath(method, path)
	c.mu.Lock()
	defer c.mu.Unlock()
	if !ok {
		c.unknown[strings.ToUpper(method)+" "+path] = true
		return
	}
	h := c.ops[op.ID]
	if h == nil {
		h = &hits{}
		c.ops[op.ID] = h
	}
	phase := 0
	if c.cleanup {
		phase = 1
	}
	if status >= 200 && status <= 299 {
		h.ok[phase]++
	} else {
		h.other[phase]++
	}
}

// NotApplicable reports that the run could not cover opID for reason
// (e.g. no data in the account). It excuses only a ModeIfData operation.
func (c *Coverage) NotApplicable(opID, reason string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.notApplicable[opID] = reason
}

// CoverageReport is the result of a run against a plan. All lists hold
// operation IDs sorted, Unknown holds "METHOD path"; Lines is the text for
// the log.
type CoverageReport struct {
	// Covered are the operations covered as their expectation requires;
	// OnlyAnswered and OnlyCleanup are the part covered only by a non-2xx
	// answer or only in the cleanup.
	Covered, OnlyAnswered, OnlyCleanup []string
	// NotApplicable are ModeIfData operations reported without data.
	NotApplicable []string
	// Pending are uncovered operations of a later task; they never fail
	// a run.
	Pending []string
	// Missing are expected operations the run did not cover.
	Missing []string
	Unknown []string
	Lines   []string
}

// Report evaluates the recorded requests against plan. Per operation, in
// this order: covered (by Mode within Phase), else pending (Pending set),
// else not applicable (ModeIfData and NotApplicable reported), else
// missing. Operations recorded but not in plan are ignored.
func (c *Coverage) Report(plan map[string]Expectation) CoverageReport {
	if c == nil {
		c = NewCoverage()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var r CoverageReport
	var missingLines, naLines, pendingLines []string
	ids := make([]string, 0, len(plan))
	for id := range plan {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		e := plan[id]
		covered, onlyAnswered, onlyCleanup := c.ops[id].judge(e)
		reason, na := c.notApplicable[id]
		switch {
		case covered:
			r.Covered = append(r.Covered, id)
			if onlyAnswered {
				r.OnlyAnswered = append(r.OnlyAnswered, id)
			}
			if onlyCleanup {
				r.OnlyCleanup = append(r.OnlyCleanup, id)
			}
		case e.Pending != 0:
			r.Pending = append(r.Pending, id)
			pendingLines = append(pendingLines, fmt.Sprintf("ABDECKUNG ausstehend: %s (Task %d)", id, e.Pending))
		case e.Mode == ModeIfData && na:
			r.NotApplicable = append(r.NotApplicable, id)
			naLines = append(naLines, fmt.Sprintf("ABDECKUNG nicht anwendbar: %s: %s", id, reason))
		default:
			r.Missing = append(r.Missing, id)
			missingLines = append(missingLines, fmt.Sprintf("ABDECKUNG fehlt: %s (erwartet von %s)", id, e.Test))
		}
	}
	for k := range c.unknown {
		r.Unknown = append(r.Unknown, k)
	}
	slices.Sort(r.Unknown)

	r.Lines = append(r.Lines, fmt.Sprintf(
		"ABDECKUNG: abgedeckt %d/%d (davon %d nur mit Fehlerantwort, %d nur im Aufräumen), nicht anwendbar %d, ausstehend %d, fehlend %d",
		len(r.Covered), len(plan), len(r.OnlyAnswered), len(r.OnlyCleanup), len(r.NotApplicable), len(r.Pending), len(r.Missing)))
	r.Lines = append(r.Lines, missingLines...)
	r.Lines = append(r.Lines, naLines...)
	r.Lines = append(r.Lines, pendingLines...)
	for _, u := range r.Unknown {
		r.Lines = append(r.Lines, "ABDECKUNG unbekannter Pfad: "+u)
	}
	return r
}

// judge reports whether h covers e, and whether only by non-2xx answers
// or only in the cleanup. A nil h covers nothing.
func (h *hits) judge(e Expectation) (covered, onlyAnswered, onlyCleanup bool) {
	if h == nil {
		return false, false, false
	}
	phases := 1
	if e.Phase == PhaseAny {
		phases = 2
	}
	ok, other := 0, 0
	for p := range phases {
		ok += h.ok[p]
		other += h.other[p]
	}
	inTest := h.ok[0]
	if e.Mode == ModeAnswered {
		covered = ok+other > 0
		inTest += h.other[0]
	} else {
		covered = ok > 0
	}
	return covered, covered && ok == 0, covered && inTest == 0
}
