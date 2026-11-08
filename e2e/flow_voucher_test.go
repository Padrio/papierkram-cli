//go:build e2e

package e2e

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// Spec 9.3, 9.2, 8.1, 17 (open points 1 and 2): the life cycle of a
// voucher of its own created with its file (create --file, describe, pdf,
// document delete, archive, unarchive, reverse, cancel). Hard: create
// --file exits 0 (no exit 11) with a 2xx upload, the voucher has exactly
// one document, pdf, document delete, archive and unarchive exit 0.
// Measured: the key shape of the document, the PDF download, and reverse
// and cancel with the states after them.
//
// reverse runs before cancel on the same voucher (no second voucher, saves
// 50 credits): both are status changes whose precondition is not known
// live, so the measurement shows which state allows them. The voucher
// books on a creditor of its own (stornoSupplier, 10 credits): after
// reverse and cancel that company may only be archivable, and s.supplier
// stays deletable for the cleanup (DeleteCompany).
func TestE2E_VoucherFlow(t *testing.T) {
	s := live(t)

	// 1. own creditor (10), then create --file: create 50 and upload 10,
	// hard exit 0
	sup := s.stornoSupplier(t)
	file := filepath.Join(t.TempDir(), "beleg.pdf")
	if err := os.WriteFile(file, minimalPDF, 0o600); err != nil {
		s.fatalf(t, "%v", err)
	}
	name := s.prefix + "beleg-datei"
	s.need(t, 60)
	v, r := s.createRun(t, object{resource: "voucher", name: name},
		"voucher", "create", "--name", name, "--creditor", ids(sup), "--provenance", "domestic",
		"--document-date", "today", "--item", "name="+pos19+",amount=119,vat=19,category=Bürobedarf",
		"--file", file)
	id := ids(idOf(v))
	switch x := lastExchange(r, "POST", "/documents"); {
	case x == nil:
		s.errorf(t, "voucher create --file: kein Upload aufgezeichnet")
	case x.Status < 200 || x.Status > 299:
		s.errorf(t, "voucher create --file: Upload HTTP %d", x.Status)
	}

	// 2. describe (1), hard: exactly one document
	s.need(t, 1)
	m := s.ok(t, "", "voucher", "describe", id)
	docs := asSlice(m["documents"])
	var doc int64
	if len(docs) != 1 {
		s.errorf(t, "voucher describe: %d Dokumente, erwartet genau 1", len(docs))
	} else {
		doc = idOf(docs[0])
		s.measuref(t, "Beleg-Dokument", "Schlüssel %s", keyShape(docs[0]))
	}

	// 3. pdf (1), hard exit 0; measured: what the download is
	s.need(t, 1)
	out := filepath.Join(t.TempDir(), "beleg-download.pdf")
	r = s.run(t, "", "voucher", "pdf", id, "-O", out, "-o", "json")
	if x := lastExchange(r, "GET", "/pdf"); x != nil {
		s.measuref(t, "voucher pdf", "HTTP %d, Content-Type %q, %d Bytes", x.Status, x.RespHeader.Get("Content-Type"), len(x.RespBody))
	}
	if r.code != 0 {
		s.quotaSkip(t, r, []string{"voucher", "pdf"})
		s.errorf(t, "voucher pdf: Exit %d\n%s", r.code, r.stderr)
	} else if data, err := os.ReadFile(out); err != nil {
		s.errorf(t, "voucher pdf: Datei nicht lesbar: %v", err)
	} else {
		s.measuref(t, "voucher pdf", "Datei %d Bytes, beginnt mit %%PDF: %t", len(data), bytes.HasPrefix(data, []byte("%PDF")))
	}

	// 4. document delete (1), hard exit 0; needs the document id of 2
	if doc != 0 {
		s.need(t, 1)
		if d := s.run(t, "", "voucher", "document", "delete", id, ids(doc), "--yes", "-o", "json"); d.code != 0 {
			s.quotaSkip(t, d, []string{"voucher", "document"})
			s.errorf(t, "voucher document delete: Exit %d\n%s", d.code, d.stderr)
		}
	}

	// 5. archive, unarchive (2), hard exit 0
	s.need(t, 2)
	if a := s.run(t, "", "voucher", "archive", id, "-o", "json"); a.code != 0 {
		s.quotaSkip(t, a, []string{"voucher", "archive"})
		s.errorf(t, "voucher archive: Exit %d\n%s", a.code, a.stderr)
	} else if u := s.run(t, "", "voucher", "unarchive", id, "-o", "json"); u.code != 0 {
		s.quotaSkip(t, u, []string{"voucher", "unarchive"})
		s.errorf(t, "voucher unarchive: Exit %d\n%s", u.code, u.stderr)
	}

	// 6. reverse and describe (2), measured
	s.need(t, 2)
	s.cancelMeasured(t, "voucher", "reverse", id)
	m = s.ok(t, "", "voucher", "describe", id)
	s.measuref(t, "Statuswerte Beleg", "nach reverse: state %q", str(m["state"]))

	// 7. cancel and describe (2), measured
	s.need(t, 2)
	s.cancelMeasured(t, "voucher", "cancel", id)
	m = s.ok(t, "", "voucher", "describe", id)
	s.measuref(t, "Statuswerte Beleg", "nach reverse und cancel: state %q", str(m["state"]))
}
