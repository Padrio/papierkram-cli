package cli

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/spec"
	"github.com/Padrio/papierkram-cli/internal/testutil/golden"
)

// Reference data commands (spec 7.1, 15 no. 1–4, 50–51, 59, 88–89):
// payment-term, bank-connection, transaction, user and info, all read-only.

// TestReferenceCommandsGolden renders list and describe (and the single
// read of info) from the spec examples of the mock in every output format.
func TestReferenceCommandsGolden(t *testing.T) {
	// GET /info answers with the spec example that lists invoice
	// templates; the first example (the mock default) has none.
	templates := specExample(t, "GetInfo", "shows custom templates")
	cases := []struct {
		name  string
		args  []string
		path  string
		query url.Values
		body  []byte
	}{
		{"payment-term-list", []string{"payment-term", "list"}, "/income/payment_terms", url.Values{"page_size": {"25"}}, nil},
		{"payment-term-describe", []string{"payment-term", "describe", "1236"}, "/income/payment_terms/1236", url.Values{}, nil},
		{"bank-connection-list", []string{"bank-connection", "list"}, "/banking/bank_connections", url.Values{"page_size": {"25"}}, nil},
		{"bank-connection-describe", []string{"bank-connection", "describe", "239"}, "/banking/bank_connections/239", url.Values{}, nil},
		{"transaction-list", []string{"transaction", "list"}, "/banking/transactions", url.Values{"page_size": {"25"}}, nil},
		{"transaction-describe", []string{"transaction", "describe", "457"}, "/banking/transactions/457", url.Values{}, nil},
		{"user-list", []string{"user", "list"}, "/users", url.Values{"page_size": {"25"}}, nil},
		{"user-describe", []string{"user", "describe", "2516"}, "/users/2516", url.Values{}, nil},
		{"info", []string{"info"}, "/info", url.Values{}, templates},
	}
	for _, tc := range cases {
		for _, format := range []string{"table", "wide", "json", "yaml"} {
			t.Run(tc.name+"/"+format, func(t *testing.T) {
				app, srv := apiApp(t)
				if tc.body != nil {
					srv.On("GET", tc.path, 200, tc.body)
				}
				wantExit(t, app, Run(app, append(tc.args, "-o", format)), 0)
				r := wantRequests(t, srv, 1)[0]
				if r.Method != "GET" || r.Path != tc.path || r.Query.Encode() != tc.query.Encode() {
					t.Errorf("request = %s %s?%s", r.Method, r.Path, r.Query.Encode())
				}
				out, errs := bufs(app)
				if errs != "" {
					t.Errorf("stderr = %q", errs)
				}
				golden.Assert(t, tc.name+"."+format, []byte(out))
				assertNoToken(t, app)
			})
		}
	}
}

// specExample returns the JSON of the 200 example of an operation whose
// name contains key.
func specExample(t *testing.T, opID, key string) []byte {
	t.Helper()
	op, ok := spec.OperationByID(opID)
	if !ok {
		t.Fatalf("operation %s not in spec", opID)
	}
	if r := op.Op.Responses.Status(200); r != nil && r.Value != nil {
		if mt := r.Value.Content.Get("application/json"); mt != nil {
			for name, ex := range mt.Examples {
				if strings.Contains(name, key) && ex != nil && ex.Value != nil {
					raw, err := json.Marshal(ex.Value.Value)
					if err != nil {
						t.Fatal(err)
					}
					return raw
				}
			}
		}
	}
	t.Fatalf("%s: no 200 example %q", opID, key)
	return nil
}

func TestReferenceAliases(t *testing.T) {
	for _, args := range [][]string{
		{"zahlungsbedingung", "list", "-o", "json"},
		{"bankverbindung", "describe", "239", "-o", "json"},
		{"umsatz", "list", "-o", "json"},
		{"benutzer", "describe", "2516", "-o", "json"},
	} {
		app, srv := apiApp(t)
		wantExit(t, app, Run(app, args), 0)
		wantRequests(t, srv, 1)
	}
}

// Spec 4.2, 7.1: transaction list --bank-connection maps to
// bank_connection_id; an invalid ID sends nothing.
func TestTransactionListFilter(t *testing.T) {
	app, srv := apiApp(t)
	wantExit(t, app, Run(app, []string{"transaction", "list", "--bank-connection", "239", "-o", "json"}), 0)
	want := url.Values{"bank_connection_id": {"239"}, "page_size": {"25"}}
	if q := wantRequests(t, srv, 1)[0].Query; q.Encode() != want.Encode() {
		t.Errorf("query = %s, erwartet %s", q.Encode(), want.Encode())
	}

	app, srv = apiApp(t)
	wantExit(t, app, Run(app, []string{"transaction", "list", "--bank-connection", "Sparkasse"}), 2)
	wantRequests(t, srv, 0)
}

// The reference data are read-only: no mutating action is built, and the
// read-only lock does not block them.
func TestReferenceCommandsReadOnly(t *testing.T) {
	for _, resource := range []string{"payment-term", "bank-connection", "transaction", "user"} {
		app, _ := apiApp(t)
		wantExit(t, app, Run(app, []string{resource}), 0)
		help := mustOut(app)
		for _, a := range []string{"create", "update", "delete", "archive"} {
			if strings.Contains(help, "  "+a+" ") {
				t.Errorf("%s: Gruppenhilfe nennt %s:\n%s", resource, a, help)
			}
		}
	}
	for _, args := range [][]string{
		{"payment-term", "list"},
		{"bank-connection", "describe", "239"},
		{"transaction", "list"},
		{"user", "list"},
		{"info"},
	} {
		app, srv := mockApp(t, map[string]string{"PAPIERKRAM_TOKEN": "test-token-ro", "PAPIERKRAM_READ_ONLY": "1"})
		wantExit(t, app, Run(app, append(args, "-o", "json")), 0)
		wantRequests(t, srv, 1)
	}
}
