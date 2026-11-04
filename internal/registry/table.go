package registry

import "github.com/Padrio/papierkram-cli/internal/output"

// Default columns (spec 10.3). Paths are the JSON paths of the API objects.
var (
	// customer_no/supplier_no (spec 10.3) is one column: the customer
	// number, else the supplier number; TYP says which one it is.
	companyColumns = output.Columns{
		Table: []output.Column{
			{Header: "ID", Path: "id"},
			{Header: "NAME", Path: "name"},
			{Header: "TYP", Path: "contact_type"},
			{Header: "NR.", Path: "customer_no", Fallback: "supplier_no"},
			{Header: "E-MAIL", Path: "email"},
		},
		Wide: []output.Column{
			{Header: "TELEFON", Path: "phone"},
			{Header: "ORT", Path: "physical_city"},
			{Header: "STATUS", Path: "record_state"},
		},
	}
	personColumns = output.Columns{
		Table: []output.Column{
			{Header: "ID", Path: "id"},
			{Header: "VORNAME", Path: "first_name"},
			{Header: "NACHNAME", Path: "last_name"},
			{Header: "E-MAIL", Path: "email"},
			{Header: "POSITION", Path: "position"},
		},
		Wide: []output.Column{
			{Header: "TELEFON", Path: "phone"},
			{Header: "MOBIL", Path: "mobile"},
			{Header: "ABTEILUNG", Path: "department"},
		},
	}
	// invoice and estimate (spec 10.3); billing.company is the customer
	// name as printed on the document.
	invoiceColumns = output.Columns{
		Table: []output.Column{
			{Header: "ID", Path: "id"},
			{Header: "NR.", Path: "invoice_no"},
			{Header: "NAME", Path: "name"},
			{Header: "STATUS", Path: "state"},
			{Header: "DATUM", Path: "document_date", Fmt: output.FmtDate},
			{Header: "FÄLLIG", Path: "due_date", Fmt: output.FmtDate},
			{Header: "BRUTTO", Path: "total_gross", Fmt: output.FmtMoney},
			{Header: "KUNDE", Path: "billing.company"},
		},
		Wide: []output.Column{
			{Header: "NETTO", Path: "total_net", Fmt: output.FmtMoney},
			{Header: "OFFEN", Path: "outstanding_amount", Fmt: output.FmtMoney},
			{Header: "BEZAHLT AM", Path: "paid_at_date", Fmt: output.FmtDate},
			{Header: "VERSENDET", Path: "sent_on", Fmt: output.FmtDate},
		},
	}
	estimateColumns = output.Columns{
		Table: []output.Column{
			{Header: "ID", Path: "id"},
			{Header: "NR.", Path: "estimate_no"},
			{Header: "NAME", Path: "name"},
			{Header: "STATUS", Path: "state"},
			{Header: "DATUM", Path: "document_date", Fmt: output.FmtDate},
			{Header: "BRUTTO", Path: "total_gross", Fmt: output.FmtMoney},
			{Header: "KUNDE", Path: "billing.company"},
		},
		Wide: []output.Column{
			{Header: "NETTO", Path: "total_net", Fmt: output.FmtMoney},
			{Header: "VERSENDET", Path: "sent_on", Fmt: output.FmtDate},
		},
	}
	bankConnectionColumns = output.Columns{
		Table: []output.Column{
			{Header: "ID", Path: "id"},
			{Header: "NAME", Path: "name"},
		},
		Wide: []output.Column{
			{Header: "IBAN", Path: "iban"},
			{Header: "BIC", Path: "bic"},
		},
	}
	transactionColumns = output.Columns{
		Table: []output.Column{
			{Header: "ID", Path: "id"},
			{Header: "DATUM", Path: "bdate", Fmt: output.FmtDate},
			{Header: "BETRAG", Path: "value", Fmt: output.FmtMoney},
			{Header: "STATUS", Path: "state"},
			{Header: "VON", Path: "from.name"},
			{Header: "VERWENDUNGSZWECK", Path: "usage"},
		},
		Wide: []output.Column{
			{Header: "WÄHRUNG", Path: "currency"},
			{Header: "BANKVERBINDUNG", Path: "bank_connection.name"},
		},
	}
	// due_time is the due period, in days for due_time_unit "day" (the
	// only unit in the spec examples).
	paymentTermColumns = output.Columns{
		Table: []output.Column{
			{Header: "ID", Path: "id"},
			{Header: "NAME", Path: "name"},
			{Header: "FÄLLIG", Path: "due_time"},
		},
		Wide: []output.Column{
			{Header: "EINHEIT", Path: "due_time_unit"},
			{Header: "SKONTO", Path: "cash_discount"},
		},
	}
	infoColumns = output.Columns{
		Table: []output.Column{
			{Header: "API-VERSION", Path: "api.version"},
			{Header: "RECHNUNGSVORLAGEN", Path: "settings.custom_templates.invoices", Fmt: output.FmtIDNameList},
		},
	}
	userColumns = output.Columns{
		Table: []output.Column{
			{Header: "ID", Path: "id"},
			{Header: "NAME", Path: "full_name"},
			{Header: "E-MAIL", Path: "email"},
			{Header: "ROLLE", Path: "role"},
		},
		Wide: []output.Column{
			{Header: "VORNAME", Path: "first_name"},
			{Header: "NACHNAME", Path: "last_name"},
		},
	}
)

// table holds one entry per API command, in the order of spec 15.
var table = []Entry{
	// 1–4
	{
		Resource: "bank-connection", Action: "list", OperationID: "ListBankConnections",
		Method: "GET", Path: "/banking/bank_connections", Class: ClassReadOnly, Credits: 1,
		Columns: bankConnectionColumns,
		Short:   "Bankverbindungen auflisten",
		Examples: []string{
			"papierkram bank-connection list",
			"papierkram bank-connection list -o json",
		},
	},
	{
		Resource: "bank-connection", Action: "describe", OperationID: "GetBankConnection",
		Method: "GET", Path: "/banking/bank_connections/{id}", Class: ClassReadOnly, Credits: 1,
		Columns: bankConnectionColumns,
		Short:   "Bankverbindung anzeigen",
		Examples: []string{
			"papierkram bank-connection describe 237",
			"papierkram bank-connection describe 237 -o yaml",
		},
	},
	{
		Resource: "transaction", Action: "list", OperationID: "ListTransactions",
		Method: "GET", Path: "/banking/transactions", Class: ClassReadOnly, Credits: 1,
		Filters: []Filter{
			{Flag: "bank-connection", Param: "bank_connection_id", Kind: "id", Help: "Nur Umsätze dieser Bankverbindung (ID)"},
		},
		Columns: transactionColumns,
		Short:   "Bankumsätze auflisten",
		Examples: []string{
			"papierkram transaction list --bank-connection 237",
			"papierkram transaction list --all -o json",
		},
	},
	{
		Resource: "transaction", Action: "describe", OperationID: "GetTransaction",
		Method: "GET", Path: "/banking/transactions/{id}", Class: ClassReadOnly, Credits: 1,
		Columns: transactionColumns,
		Short:   "Bankumsatz anzeigen",
		Examples: []string{
			"papierkram transaction describe 457",
			"papierkram transaction describe 457 -o json",
		},
	},
	// 5–11
	{
		Resource: "company", Action: "list", OperationID: "ListCompanies",
		Method: "GET", Path: "/contact/companies", Class: ClassReadOnly, Credits: 1,
		Filters: []Filter{
			// GET /contact/companies ignores contact_type (spec 4.6, 9.7)
			{Flag: "type", Param: "contact_type", Kind: "local_enum",
				Help: "nur Kunden (customer) oder Lieferanten (supplier); die API filtert nicht: lädt alle Seiten (1 Credit je Seite) und filtert lokal"},
		},
		Columns: companyColumns,
		Short:   "Firmen auflisten",
		Examples: []string{
			"papierkram company list",
			"papierkram company list --type customer",
			"papierkram company list --type supplier --limit 10 --page 2 -o json",
		},
	},
	{
		Resource: "company", Action: "create", OperationID: "CreateCompany",
		Method: "POST", Path: "/contact/companies", Class: ClassCreate, Credits: 10,
		Columns: companyColumns,
		Short:   "Firma anlegen",
		Examples: []string{
			`papierkram company create --name "Müller & Söhne GmbH" --contact-type customer --email info@example.org`,
			"papierkram company create --name Bürobedarf --contact-type supplier --dry-run",
			"papierkram company create --input firma.json",
		},
	},
	{
		Resource: "company", Action: "describe", OperationID: "GetCompany",
		Method: "GET", Path: "/contact/companies/{id}", Class: ClassReadOnly, Credits: 1,
		Columns: companyColumns,
		Short:   "Firma anzeigen",
		Examples: []string{
			"papierkram company describe 2843",
			"papierkram company describe 2843 -o json",
		},
	},
	{
		Resource: "company", Action: "update", OperationID: "UpdateCompany",
		Method: "PUT", Path: "/contact/companies/{id}", Class: ClassUpdate, Credits: 1,
		Columns: companyColumns,
		Short:   "Firma ändern",
		Examples: []string{
			"papierkram company update 2843 --email buchhaltung@example.org",
			"papierkram company update 2843 --input firma.json",
		},
	},
	{
		Resource: "company", Action: "delete", OperationID: "DeleteCompany",
		Method: "DELETE", Path: "/contact/companies/{id}", Class: ClassDestructive, Credits: 1,
		Short: "Firma löschen",
		Examples: []string{
			"papierkram company delete 2843",
			"papierkram company delete 2843 --yes",
		},
	},
	{
		Resource: "company", Action: "archive", OperationID: "ArchiveCompany",
		Method: "POST", Path: "/contact/companies/{id}/archive", Class: ClassArchive, Credits: 1,
		Columns: companyColumns,
		Short:   "Firma archivieren",
		Examples: []string{
			"papierkram company archive 2843",
			"papierkram company archive 2843 -o json",
		},
	},
	{
		Resource: "company", Action: "unarchive", OperationID: "UnarchiveCompany",
		Method: "POST", Path: "/contact/companies/{id}/unarchive", Class: ClassArchive, Credits: 1,
		Columns: companyColumns,
		Short:   "Firma aus dem Archiv holen",
		Examples: []string{
			"papierkram company unarchive 2843",
			"papierkram company unarchive 2843 -o json",
		},
	},
	// 12–16
	{
		Resource: "person", Action: "list", OperationID: "ListPersons",
		Method: "GET", Path: "/contact/companies/{company_id}/persons", Class: ClassReadOnly, Credits: 1,
		Columns: personColumns,
		Short:   "Kontaktpersonen einer Firma auflisten",
		Examples: []string{
			"papierkram person list --company 2843",
			"papierkram person list --company 2843 -o json",
		},
	},
	{
		Resource: "person", Action: "create", OperationID: "CreatePerson",
		Method: "POST", Path: "/contact/companies/{company_id}/persons", Class: ClassCreate, Credits: 10,
		Columns: personColumns,
		Short:   "Kontaktperson anlegen",
		Examples: []string{
			"papierkram person create --company 2843 --first-name Anna --last-name Groß --email anna@example.org",
			"papierkram person create --company 2843 --first-name Jörg --last-name Weiß --default",
		},
	},
	{
		Resource: "person", Action: "describe", OperationID: "GetPerson",
		Method: "GET", Path: "/contact/companies/{company_id}/persons/{id}", Class: ClassReadOnly, Credits: 1,
		Columns: personColumns,
		Short:   "Kontaktperson anzeigen",
		Examples: []string{
			"papierkram person describe 1063 --company 2862",
			"papierkram person describe 1063 --company 2862 -o yaml",
		},
	},
	{
		Resource: "person", Action: "update", OperationID: "UpdatePerson",
		Method: "PUT", Path: "/contact/companies/{company_id}/persons/{id}", Class: ClassUpdate, Credits: 1,
		Columns: personColumns,
		Short:   "Kontaktperson ändern",
		Examples: []string{
			"papierkram person update 1063 --company 2862 --position Einkauf",
			"papierkram person update 1063 --company 2862 --flagged=false",
		},
	},
	{
		Resource: "person", Action: "delete", OperationID: "DeletePerson",
		Method: "DELETE", Path: "/contact/companies/{company_id}/persons/{id}", Class: ClassDestructive, Credits: 1,
		Short: "Kontaktperson löschen",
		Examples: []string{
			"papierkram person delete 1063 --company 2862",
			"papierkram person delete 1063 --company 2862 --yes",
		},
	},
	// 30–39
	{
		Resource: "estimate", Action: "list", OperationID: "ListEstimates",
		Method: "GET", Path: "/income/estimates", Class: ClassReadOnly, Credits: 1,
		Filters: []Filter{
			{Flag: "company", Param: "company_id", Kind: "id", Help: "Nur Angebote dieser Firma (ID)"},
			{Flag: "project", Param: "project_id", Kind: "id", Help: "Nur Angebote dieses Projekts (ID)"},
			{Flag: "from", Param: "document_date_range_start", Kind: "date", Help: "Nur Angebote mit Belegdatum ab diesem Tag (YYYY-MM-DD, heute, gestern)"},
			{Flag: "to", Param: "document_date_range_end", Kind: "date", Help: "Nur Angebote mit Belegdatum bis zu diesem Tag (YYYY-MM-DD, heute, gestern)"},
		},
		Columns: estimateColumns,
		Short:   "Angebote auflisten",
		Examples: []string{
			"papierkram estimate list",
			"papierkram estimate list --company 3052 --from 2026-01-01 --to 2026-12-31",
			"papierkram estimate list --project 4316 --all -o json",
		},
	},
	{
		Resource: "estimate", Action: "create", OperationID: "CreateEstimate",
		Method: "POST", Path: "/income/estimates", Class: ClassCreate, Credits: 50,
		Columns: estimateColumns,
		Short:   "Angebot anlegen",
		Examples: []string{
			`papierkram estimate create --name "Angebot Website" --customer 3052 --item "name=Konzeption,qty=3,unit=Tag,price=800,vat=19"`,
			"papierkram estimate create --name Wartung --customer 3052 --items positionen.json --dry-run",
			"papierkram estimate create --input angebot.json",
		},
	},
	{
		Resource: "estimate", Action: "describe", OperationID: "GetEstimate",
		Method: "GET", Path: "/income/estimates/{id}", Class: ClassReadOnly, Credits: 1,
		Columns: estimateColumns,
		Short:   "Angebot anzeigen",
		Examples: []string{
			"papierkram estimate describe 349",
			"papierkram estimate describe 349 -o json",
		},
	},
	{
		Resource: "estimate", Action: "update", OperationID: "UpdateEstimate",
		Method: "PUT", Path: "/income/estimates/{id}", Class: ClassUpdate, Credits: 1,
		Columns: estimateColumns,
		Short:   "Angebot ändern",
		Examples: []string{
			`papierkram estimate update 349 --name "Angebot Website, 2. Fassung"`,
			`papierkram estimate update 349 --item "name=Konzeption,qty=4,price=800,vat=19" --replace-items`,
			"papierkram estimate update 349 --input angebot.json",
		},
	},
	{
		Resource: "estimate", Action: "delete", OperationID: "DeleteEstimate",
		Method: "DELETE", Path: "/income/estimates/{id}", Class: ClassDestructive, Credits: 1,
		Short: "Angebot löschen",
		Examples: []string{
			"papierkram estimate delete 349",
			"papierkram estimate delete 349 --yes",
		},
	},
	{
		Resource: "estimate", Action: "archive", OperationID: "ArchiveEstimate",
		Method: "POST", Path: "/income/estimates/{id}/archive", Class: ClassArchive, Credits: 1,
		Columns: estimateColumns,
		Short:   "Angebot archivieren",
		Examples: []string{
			"papierkram estimate archive 349",
			"papierkram estimate archive 349 -o json",
		},
	},
	{
		Resource: "estimate", Action: "unarchive", OperationID: "UnarchiveEstimate",
		Method: "POST", Path: "/income/estimates/{id}/unarchive", Class: ClassArchive, Credits: 1,
		Columns: estimateColumns,
		Short:   "Angebot aus dem Archiv holen",
		Examples: []string{
			"papierkram estimate unarchive 349",
			"papierkram estimate unarchive 349 -o json",
		},
	},
	{
		Resource: "estimate", Action: "cancel", OperationID: "CancelEstimate",
		Method: "POST", Path: "/income/estimates/{id}/cancel", Class: ClassDestructive, Credits: 1,
		Columns: estimateColumns,
		Short:   "Angebot stornieren",
		Examples: []string{
			"papierkram estimate cancel 349",
			"papierkram estimate cancel 349 --yes",
		},
	},
	{
		// own handler (Task 31, spec 9.1)
		Resource: "estimate", Action: "deliver", OperationID: "DeliverEstimate",
		Method: "POST", Path: "/income/estimates/{id}/deliver", Class: ClassSend, Credits: 1,
		Short: "Angebot versenden (per E-Mail oder als PDF finalisieren)",
		Examples: []string{
			"papierkram estimate deliver 349 --via pdf",
			`papierkram estimate deliver 349 --via email --to kunde@example.org --subject "Angebot {{angebot.angebotsnummer}}" --body-file mail.txt`,
		},
		Custom: true,
	},
	{
		// own handler (Task 32, spec 9.2)
		Resource: "estimate", Action: "pdf", OperationID: "GetEstimatePdf",
		Method: "GET", Path: "/income/estimates/{id}/pdf", Class: ClassReadOnly, Credits: 1,
		Short: "Angebot als PDF herunterladen",
		Examples: []string{
			"papierkram estimate pdf 349 -O angebot.pdf",
			"papierkram estimate pdf 349 -O - > angebot.pdf",
		},
		Custom: true,
	},
	// 40–49
	{
		Resource: "invoice", Action: "list", OperationID: "ListInvoices",
		Method: "GET", Path: "/income/invoices", Class: ClassReadOnly, Credits: 1,
		Filters: []Filter{
			{Flag: "company", Param: "company_id", Kind: "id", Help: "Nur Rechnungen dieser Firma (ID)"},
			{Flag: "project", Param: "project_id", Kind: "id", Help: "Nur Rechnungen dieses Projekts (ID)"},
			{Flag: "from", Param: "document_date_range_start", Kind: "date", Help: "Nur Rechnungen mit Belegdatum ab diesem Tag (YYYY-MM-DD, heute, gestern)"},
			{Flag: "to", Param: "document_date_range_end", Kind: "date", Help: "Nur Rechnungen mit Belegdatum bis zu diesem Tag (YYYY-MM-DD, heute, gestern)"},
		},
		Columns: invoiceColumns,
		Short:   "Rechnungen auflisten",
		Examples: []string{
			"papierkram invoice list",
			"papierkram invoice list --company 3214 --from 2026-01-01 --to 2026-03-31",
			"papierkram invoice list --project 4576 --all -o json",
		},
	},
	{
		Resource: "invoice", Action: "create", OperationID: "CreateInvoice",
		Method: "POST", Path: "/income/invoices", Class: ClassCreate, Credits: 50,
		Columns: invoiceColumns,
		Short:   "Rechnung anlegen",
		Examples: []string{
			`papierkram invoice create --name "Beratung Oktober" --customer 3214 --payment-term 1130 --item "name=Beratung,qty=2,unit=Stunde,price=100,vat=19"`,
			"papierkram invoice create --name Wartung --customer 3214 --payment-term 1130 --items positionen.json --dry-run",
			"papierkram invoice create --input rechnung.json",
		},
	},
	{
		Resource: "invoice", Action: "describe", OperationID: "GetInvoice",
		Method: "GET", Path: "/income/invoices/{id}", Class: ClassReadOnly, Credits: 1,
		Columns: invoiceColumns,
		Short:   "Rechnung anzeigen",
		Examples: []string{
			"papierkram invoice describe 459",
			"papierkram invoice describe 459 -o json",
		},
	},
	{
		Resource: "invoice", Action: "update", OperationID: "UpdateInvoice",
		Method: "PUT", Path: "/income/invoices/{id}", Class: ClassUpdate, Credits: 1,
		Columns: invoiceColumns,
		Short:   "Rechnung ändern",
		Examples: []string{
			"papierkram invoice update 459 --payment-term 1131",
			`papierkram invoice update 459 --item "name=Beratung,qty=3,price=100,vat=19" --replace-items`,
			"papierkram invoice update 459 --input rechnung.json",
		},
	},
	{
		Resource: "invoice", Action: "delete", OperationID: "DeleteInvoice",
		Method: "DELETE", Path: "/income/invoices/{id}", Class: ClassDestructive, Credits: 1,
		Short: "Rechnung löschen",
		Examples: []string{
			"papierkram invoice delete 459",
			"papierkram invoice delete 459 --yes",
		},
	},
	{
		Resource: "invoice", Action: "archive", OperationID: "ArchiveInvoice",
		Method: "POST", Path: "/income/invoices/{id}/archive", Class: ClassArchive, Credits: 1,
		Columns: invoiceColumns,
		Short:   "Rechnung archivieren",
		Examples: []string{
			"papierkram invoice archive 459",
			"papierkram invoice archive 459 -o json",
		},
	},
	{
		Resource: "invoice", Action: "unarchive", OperationID: "UnarchiveInvoice",
		Method: "POST", Path: "/income/invoices/{id}/unarchive", Class: ClassArchive, Credits: 1,
		Columns: invoiceColumns,
		Short:   "Rechnung aus dem Archiv holen",
		Examples: []string{
			"papierkram invoice unarchive 459",
			"papierkram invoice unarchive 459 -o json",
		},
	},
	{
		Resource: "invoice", Action: "cancel", OperationID: "CancelInvoice",
		Method: "POST", Path: "/income/invoices/{id}/cancel", Class: ClassDestructive, Credits: 1,
		Columns: invoiceColumns,
		Short:   "Rechnung stornieren",
		Examples: []string{
			"papierkram invoice cancel 459",
			"papierkram invoice cancel 459 --yes",
		},
	},
	{
		// own handler (Task 31, spec 9.1)
		Resource: "invoice", Action: "deliver", OperationID: "DeliverInvoice",
		Method: "POST", Path: "/income/invoices/{id}/deliver", Class: ClassSend, Credits: 1,
		Short: "Rechnung versenden (per E-Mail oder als PDF finalisieren)",
		Examples: []string{
			"papierkram invoice deliver 459 --via pdf",
			`papierkram invoice deliver 459 --via email --to kunde@example.org --subject "Rechnung {{rechnung.rechnungsnummer}}" --body-file mail.txt`,
		},
		Custom: true,
	},
	{
		// own handler (Task 32, spec 9.2)
		Resource: "invoice", Action: "pdf", OperationID: "GetInvoicePdf",
		Method: "GET", Path: "/income/invoices/{id}/pdf", Class: ClassReadOnly, Credits: 1,
		Short: "Rechnung als PDF herunterladen",
		Examples: []string{
			"papierkram invoice pdf 459 -O rechnung.pdf",
			"papierkram invoice pdf 459 -O - > rechnung.pdf",
		},
		Custom: true,
	},
	// 50–51
	{
		Resource: "payment-term", Action: "list", OperationID: "ListPaymentTerms",
		Method: "GET", Path: "/income/payment_terms", Class: ClassReadOnly, Credits: 1,
		Columns: paymentTermColumns,
		Short:   "Zahlungsbedingungen auflisten",
		Examples: []string{
			"papierkram payment-term list",
			"papierkram payment-term list -o wide",
		},
	},
	{
		Resource: "payment-term", Action: "describe", OperationID: "GetPaymentTerm",
		Method: "GET", Path: "/income/payment_terms/{id}", Class: ClassReadOnly, Credits: 1,
		Columns: paymentTermColumns,
		Short:   "Zahlungsbedingung anzeigen",
		Examples: []string{
			"papierkram payment-term describe 1234",
			"papierkram payment-term describe 1234 -o json",
		},
	},
	// 59
	{
		Resource: "info", Action: "", OperationID: "GetInfo",
		Method: "GET", Path: "/info", Class: ClassReadOnly, Credits: 1,
		Columns: infoColumns,
		Short:   infoShort,
		Examples: []string{
			"papierkram info",
			"papierkram info -o json",
			"papierkram info --jq '.settings.custom_templates.invoices[].name'",
		},
	},
	// 88–89
	{
		Resource: "user", Action: "list", OperationID: "ListUsers",
		Method: "GET", Path: "/users", Class: ClassReadOnly, Credits: 1,
		Columns: userColumns,
		Short:   "Benutzer auflisten",
		Examples: []string{
			"papierkram user list",
			"papierkram user list -o json",
		},
	},
	{
		Resource: "user", Action: "describe", OperationID: "GetUser",
		Method: "GET", Path: "/users/{id}", Class: ClassReadOnly, Credits: 1,
		Columns: userColumns,
		Short:   "Benutzer anzeigen",
		Examples: []string{
			"papierkram user describe 2516",
			"papierkram user describe 2516 -o yaml",
		},
	},
}
