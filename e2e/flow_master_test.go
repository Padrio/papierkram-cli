//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// Spec 7.1, 8.1: archive and unarchive (class archive: no confirmation,
// reversible) of the master data and of an estimate. Per resource archive
// and unarchive (2), hard exit 0; measured: what each answer is (the
// record_state of the object, or the empty answer {"id":N,"ok":true}).
//
// unarchive runs after an archive that may have archived the object
// (exit 0, 1 or 12; mayBeArchived), so every object is active again
// afterwards: later tests use them (TimeFlow and Carries the task,
// VoucherFlow the supplier, Carries the estimate). After any other exit
// (an API error such as 4xx, 429, or nothing sent) it is skipped and the
// MESSUNG says so: no second error, no request after a 429. A failed
// unarchive logs one line "Fixture <res> <id> womöglich noch archiviert –
// Folgefehler möglich", so later failures can be traced back to it. In a
// full run every object exists already (fixtures of earlier tests).
func TestE2E_MasterDataArchive(t *testing.T) {
	s := live(t)
	idf := func(f func(*testing.T) map[string]any) func(*testing.T) int64 {
		return func(t *testing.T) int64 { return idOf(f(t)) }
	}
	cases := []struct {
		resource string
		id       func(*testing.T) int64
	}{
		{"company", s.supplier},
		{"project", s.project},
		{"task", s.task},
		{"proposition", idf(s.proposition)},
		{"custom-attribute", s.customAttribute},
		{"estimate", idf(s.estimate)},
	}
	for _, c := range cases {
		t.Run(c.resource, func(t *testing.T) {
			id := ids(c.id(t)) // creates the object if needed
			s.need(t, 2)
			a := s.run(t, "", c.resource, "archive", id, "-o", "json")
			if a.code != 0 {
				s.errorf(t, "%s archive %s: Exit %d\n%s", c.resource, id, a.code, a.stderr)
			}
			if !mayBeArchived(a.code) {
				s.measuref(t, c.resource+" archive", "archive: %s; unarchive übersprungen (Objekt nicht archiviert)", archiveAnswer(a))
				return
			}
			u := s.run(t, "", c.resource, "unarchive", id, "-o", "json")
			s.measuref(t, c.resource+" archive", "archive: %s; unarchive: %s", archiveAnswer(a), archiveAnswer(u))
			if u.code != 0 {
				s.errorf(t, "%s unarchive %s: Exit %d\n%s", c.resource, id, u.code, u.stderr)
				s.logf(t, "Fixture %s %s womöglich noch archiviert – Folgefehler möglich", c.resource, id)
			}
		})
	}
}

// mayBeArchived reports whether an archive that ended with code may have
// archived the object: exit 0 (done), 1 (unexpected error or 5xx, or
// only the output failed) and 12 (outcome unknown). Every other exit
// means the API refused it (4xx, 429) or nothing was sent.
func mayBeArchived(code int) bool { return code == 0 || code == 1 || code == 12 }

// archiveAnswer is what an archive or unarchive run shows: the HTTP
// status, then the record_state of the object in its output, the empty
// answer of the API (the CLI prints {"id":N,"ok":true}) or the exit code.
func archiveAnswer(r result) string {
	status := "keine Antwort"
	if n := len(r.exchanges); n > 0 {
		status = fmt.Sprintf("HTTP %d", r.exchanges[n-1].Status)
	}
	m := tryDecode(r.stdout)
	switch {
	case r.code != 0:
		return fmt.Sprintf("%s, Exit %d", status, r.code)
	case m == nil:
		return status + ", keine JSON-Ausgabe"
	case len(m) == 2 && m["ok"] == true:
		return status + ", leere Antwort"
	}
	if v, ok := m["record_state"]; ok {
		return fmt.Sprintf("%s, record_state %q", status, str(v))
	}
	return status + ", Objekt ohne record_state"
}

// Spec 7.1, 10.1, 10.3: read commands not covered elsewhere. Hard: the
// person of the run in person list, user and payment-term describe answer
// with the asked id, bank-connection and transaction list exit 0 (and
// describe of the first entry, if there is one), auth status shows an
// integer quota.remaining, read_only false and token_source env. Measured:
// the number and key shape of the bank details, never their values.
func TestE2E_ReadCommands(t *testing.T) {
	s := live(t)

	// person list --company (1), hard: the person of the run is listed
	t.Run("person_list", func(t *testing.T) {
		c, p := s.customer(t), idOf(s.person(t))
		s.need(t, 1)
		found := false
		for _, e := range entries(s.ok(t, "", "person", "list", "--company", ids(c))) {
			found = found || idOf(e) == p
		}
		if !found {
			s.errorf(t, "person list --company %d: Person %d des Laufs fehlt", c, p)
		}
	})

	// user describe (1), hard: same id
	t.Run("user_describe", func(t *testing.T) {
		u := s.user(t)
		s.need(t, 1)
		if got := idOf(s.ok(t, "", "user", "describe", ids(u))); got != u {
			s.errorf(t, "user describe %d: id %d", u, got)
		}
	})

	// payment-term describe of the first one (1), hard: same id
	t.Run("payment_term_describe", func(t *testing.T) {
		pt := idOf(s.paymentTerms(t)[0])
		s.need(t, 1)
		if got := idOf(s.ok(t, "", "payment-term", "describe", ids(pt))); got != pt {
			s.errorf(t, "payment-term describe %d: id %d", pt, got)
		}
	})

	// bank details: list (1), describe of the first entry (1, only with
	// data)
	t.Run("bank_connection", func(t *testing.T) {
		s.bankRead(t, "bank-connection", "GetBankConnection", "keine Bankverbindung im Sandbox-Account (gemessen)")
	})
	t.Run("transaction", func(t *testing.T) {
		s.bankRead(t, "transaction", "GetTransaction", "kein Bankumsatz im Sandbox-Account (gemessen)")
	})

	// auth status: GET /info (1), hard: quota.remaining an integer,
	// read_only false, token_source env (the suite passes PAPIERKRAM_TOKEN)
	t.Run("auth_status", func(t *testing.T) {
		s.need(t, 1)
		m := s.ok(t, "", "auth", "status")
		rem, isNum := get(m, "quota", "remaining").(json.Number)
		if _, err := rem.Int64(); !isNum || err != nil {
			s.errorf(t, "auth status: quota.remaining %v, erwartet eine ganze Zahl", get(m, "quota", "remaining"))
		}
		if m["read_only"] != false {
			s.errorf(t, "auth status: read_only %v, erwartet false", m["read_only"])
		}
		if src := str(m["token_source"]); src != "env" {
			s.errorf(t, "auth status: token_source %q, erwartet \"env\"", src)
		}
		s.measuref(t, "auth status", "quota.remaining %s, quota.request_cost %s", rem, str(get(m, "quota", "request_cost")))
	})
}

// bankRead lists the first entry of resource (bank-connection or
// transaction; 1) and describes it (1), hard exit 0 each. Without an
// entry the read by id getOp is reported as not applicable (reason
// none). Bank details are never logged with values: only total_entries
// and the key shape of the first entry (not even its id); a failure shows
// the exit code and the error code of the CLI, never stdout or the
// message.
func (s *liveSuite) bankRead(t *testing.T, resource, getOp, none string) {
	t.Helper()
	s.need(t, 1)
	r := s.run(t, "", resource, "list", "--limit", "1", "-o", "json")
	if r.code != 0 {
		s.fatalf(t, "%s list: Exit %d (%s)", resource, r.code, errorCode(r.stderr))
	}
	m := tryDecode(r.stdout)
	if m == nil {
		s.fatalf(t, "%s list: keine JSON-Ausgabe (%d Bytes)", resource, len(r.stdout))
	}
	es := entries(m)
	if len(es) == 0 {
		s.measuref(t, resource, "total_entries %s, keine Einträge", str(m["total_entries"]))
		// excuses only a ModeIfData operation (GetBankConnection,
		// GetTransaction); for an operation of another mode it has no
		// effect
		s.coverage.NotApplicable(getOp, none)
		return
	}
	s.measuref(t, resource, "total_entries %s, Schlüssel des ersten Eintrags %s", str(m["total_entries"]), keyShape(es[0]))
	id := idOf(es[0])
	if id == 0 {
		s.fatalf(t, "%s list: erster Eintrag ohne id", resource)
	}
	s.need(t, 1)
	if d := s.run(t, "", resource, "describe", ids(id), "-o", "json"); d.code != 0 {
		s.errorf(t, "%s describe (erster Eintrag): Exit %d (%s)", resource, d.code, errorCode(d.stderr))
	}
}

// errorCode is code and http_status of the structured error, the last
// line of stderr with -o json; "kein strukturierter Fehler" without one.
func errorCode(stderr string) string {
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	var e struct {
		Error struct {
			Code       string `json:"code"`
			HTTPStatus int    `json:"http_status"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(lines[len(lines)-1]), &e) != nil || e.Error.Code == "" {
		return "kein strukturierter Fehler"
	}
	if e.Error.HTTPStatus == 0 {
		return "code " + e.Error.Code
	}
	return fmt.Sprintf("code %s, HTTP %d", e.Error.Code, e.Error.HTTPStatus)
}
