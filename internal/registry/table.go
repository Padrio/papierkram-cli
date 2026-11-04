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
	// voucher (spec 10.3); list entries have no creditor, so only the
	// object responses show the supplier in wide.
	voucherColumns = output.Columns{
		Table: []output.Column{
			{Header: "ID", Path: "id"},
			{Header: "NR.", Path: "voucher_no"},
			{Header: "NAME", Path: "name"},
			{Header: "STATUS", Path: "state"},
			{Header: "DATUM", Path: "document_date", Fmt: output.FmtDate},
			{Header: "FÄLLIG", Path: "due_date", Fmt: output.FmtDate},
			{Header: "BETRAG", Path: "amount", Fmt: output.FmtMoney},
		},
		Wide: []output.Column{
			{Header: "HERKUNFT", Path: "provenance"},
			{Header: "BESCHREIBUNG", Path: "description"},
		},
	}
	voucherDetailColumns = output.Columns{
		Table: voucherColumns.Table,
		Wide: append([]output.Column{
			{Header: "LIEFERANT", Path: "creditor.name"},
		}, voucherColumns.Wide...),
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
	// proposition (spec 10.3). List entries (PropositionFlat) have no
	// vat_rate, so only the object responses show it; vat_rate is the
	// embedded VatRateFlat, its name is the rate as text ("19%").
	propositionColumns = output.Columns{
		Table: []output.Column{
			{Header: "ID", Path: "id"},
			{Header: "ARTIKELNR.", Path: "article_no"},
			{Header: "NAME", Path: "name"},
			{Header: "PREIS", Path: "price", Fmt: output.FmtMoney},
			{Header: "TYP", Path: "proposition_type"},
		},
		Wide: []output.Column{
			{Header: "ZEITEINHEIT", Path: "time_unit"},
			{Header: "EINHEIT", Path: "unit_name_1"},
			{Header: "STATUS", Path: "record_state"},
		},
	}
	propositionDetailColumns = output.Columns{
		Table: []output.Column{
			{Header: "ID", Path: "id"},
			{Header: "ARTIKELNR.", Path: "article_no"},
			{Header: "NAME", Path: "name"},
			{Header: "PREIS", Path: "price", Fmt: output.FmtMoney},
			{Header: "UST.", Path: "vat_rate.name"},
			{Header: "TYP", Path: "proposition_type"},
		},
		Wide: propositionColumns.Wide,
	}
	// payment-term [Builder] (spec 10.3: due period in days): due_time of
	// PaymentTerm{Flat,WithDetailedEmbeds} is that period, counted in
	// due_time_unit. The unit is a free string without enum ("day" in
	// every spec example), so the table shows it next to the period instead
	// of assuming days; wide does the same for the cash discount period.
	paymentTermColumns = output.Columns{
		Table: []output.Column{
			{Header: "ID", Path: "id"},
			{Header: "NAME", Path: "name"},
			{Header: "ZIEL", Path: "due_time"},
			{Header: "EINHEIT", Path: "due_time_unit"},
		},
		Wide: []output.Column{
			{Header: "SKONTO", Path: "cash_discount"},
			{Header: "SKONTOFRIST", Path: "cash_discount_time"},
			{Header: "SKONTO-EINHEIT", Path: "cash_discount_unit"},
		},
	}
	infoColumns = output.Columns{
		Table: []output.Column{
			{Header: "API-VERSION", Path: "api.version"},
			{Header: "RECHNUNGSVORLAGEN", Path: "settings.custom_templates.invoices", Fmt: output.FmtIDNameList},
		},
	}
	// project (spec 10.3); company_id is the customer. Only the object
	// responses embed the customer, so only they show its name in wide.
	projectColumns = output.Columns{
		Table: []output.Column{
			{Header: "ID", Path: "id"},
			{Header: "NAME", Path: "name"},
			{Header: "KUNDEN-ID", Path: "company_id"},
			{Header: "STATUS", Path: "record_state"},
			{Header: "START", Path: "start_date", Fmt: output.FmtDate},
			{Header: "ENDE", Path: "end_date", Fmt: output.FmtDate},
		},
		Wide: []output.Column{
			{Header: "BUDGETART", Path: "budget_type"},
			{Header: "BUDGET", Path: "budget_money", Fmt: output.FmtMoney},
			{Header: "ZEITBUDGET", Path: "budget_time"},
			{Header: "ZEITEINHEIT", Path: "budget_time_unit"},
		},
	}
	projectDetailColumns = output.Columns{
		Table: projectColumns.Table,
		Wide: append([]output.Column{
			{Header: "KUNDE", Path: "customer.name"},
		}, projectColumns.Wide...),
	}
	// task (spec 10.3); the object responses embed project, user and
	// proposition, list entries only have project_id.
	taskColumns = output.Columns{
		Table: []output.Column{
			{Header: "ID", Path: "id"},
			{Header: "NAME", Path: "name"},
			{Header: "PROJEKT-ID", Path: "project_id"},
			{Header: "ERLEDIGT", Path: "complete"},
			{Header: "FRIST", Path: "deadline", Fmt: output.FmtDate},
		},
		Wide: []output.Column{
			{Header: "MARKIERT", Path: "flagged"},
			{Header: "STATUS", Path: "record_state"},
		},
	}
	taskDetailColumns = output.Columns{
		Table: taskColumns.Table,
		Wide: append([]output.Column{
			{Header: "PROJEKT", Path: "project.name"},
			{Header: "BENUTZER", Path: "user.full_name"},
			{Header: "ARTIKEL", Path: "proposition.name"},
		}, taskColumns.Wide...),
	}
	customAttributeColumns = output.Columns{
		Table: []output.Column{
			{Header: "ID", Path: "id"},
			{Header: "NAME", Path: "name"},
			{Header: "GILT FÜR", Path: "attributable_type"},
			{Header: "DATENTYP", Path: "data_type"},
		},
		Wide: []output.Column{
			{Header: "PLATZHALTER", Path: "placeholder"},
			{Header: "STATUS", Path: "record_state"},
		},
	}
	// time (spec 10.3); duration and billable_duration are seconds in the
	// API, shown as h:mm. The object responses embed project, task and
	// user, list entries only have their IDs.
	timeColumns = output.Columns{
		Table: []output.Column{
			{Header: "ID", Path: "id"},
			{Header: "START", Path: "started_at", Fmt: output.FmtDate},
			{Header: "ENDE", Path: "ended_at", Fmt: output.FmtDate},
			{Header: "DAUER", Path: "duration", Fmt: output.FmtDurationHM},
			{Header: "AUFGABEN-ID", Path: "task_id"},
			{Header: "PROJEKT-ID", Path: "project_id"},
			{Header: "KOMMENTAR", Path: "comments"},
		},
		Wide: []output.Column{
			{Header: "ABRECHENBAR", Path: "billable_duration", Fmt: output.FmtDurationHM},
			{Header: "NICHT ABRECHENBAR", Path: "unbillable"},
			{Header: "BENUTZER-ID", Path: "user_id"},
			{Header: "RECHNUNGS-ID", Path: "invoice_id"},
			{Header: "STATUS", Path: "record_state"},
		},
	}
	timeDetailColumns = output.Columns{
		Table: timeColumns.Table,
		Wide: append([]output.Column{
			{Header: "PROJEKT", Path: "project.name"},
			{Header: "AUFGABE", Path: "task.name"},
			{Header: "BENUTZER", Path: "user.full_name"},
		}, timeColumns.Wide...),
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
	// 17–29
	{
		// own handler (Task 33, spec 9.3)
		Resource: "voucher", Action: "upload", OperationID: "UploadVoucherDocument",
		Method: "POST", Path: "/expense/vouchers/{voucher_id}/documents", Class: ClassCreate, Credits: 10,
		Short: "Datei (Belegbild) an einen Beleg anhängen",
		Examples: []string{
			"papierkram voucher upload 295 beleg.pdf",
			"papierkram voucher upload 295 quittung.jpg --dry-run",
		},
		Custom: true,
	},
	{
		// own handler (Task 33)
		Resource: "voucher", Action: "document delete", OperationID: "DeleteVoucherDocument",
		Method: "DELETE", Path: "/expense/vouchers/{voucher_id}/documents/{id}", Class: ClassDestructive, Credits: 1,
		Short: "Datei eines Belegs löschen",
		Examples: []string{
			"papierkram voucher document delete 295 32",
			"papierkram voucher document delete 295 32 --yes",
		},
		Custom: true,
	},
	{
		Resource: "voucher", Action: "list", OperationID: "ListVouchers",
		Method: "GET", Path: "/expense/vouchers", Class: ClassReadOnly, Credits: 1,
		Filters: []Filter{
			{Flag: "creditor", Param: "creditor_id", Kind: "id", Help: "Nur Belege dieses Lieferanten (Firmen-ID)"},
			{Flag: "project", Param: "project_id", Kind: "id", Help: "Nur Belege dieses Projekts (ID)"},
			{Flag: "from", Param: "document_date_range_start", Kind: "date", Help: "Nur Belege mit Belegdatum ab diesem Tag (YYYY-MM-DD, heute, gestern)"},
			{Flag: "to", Param: "document_date_range_end", Kind: "date", Help: "Nur Belege mit Belegdatum bis zu diesem Tag (YYYY-MM-DD, heute, gestern)"},
		},
		Columns: voucherColumns,
		Short:   "Belege auflisten",
		Examples: []string{
			"papierkram voucher list",
			"papierkram voucher list --creditor 2904 --from 2026-01-01 --to 2026-03-31",
			"papierkram voucher list --project 4316 --all -o json",
		},
	},
	{
		Resource: "voucher", Action: "create", OperationID: "CreateVoucher",
		Method: "POST", Path: "/expense/vouchers", Class: ClassCreate, Credits: 50,
		Columns: voucherDetailColumns,
		Short:   "Beleg anlegen",
		Examples: []string{
			`papierkram voucher create --name "Büromaterial Oktober" --provenance domestic --creditor 2904 --item "name=Papier,amount=11.90,vat=19,category=Bürobedarf"`,
			`papierkram voucher create --name "Geschäftsessen" --provenance domestic --entertainment-reason "Projektbesprechung" --entertainment-person "Anna Muster" --item "name=Restaurant,amount=150.80,vat=19,category=Bewirtungskosten" --dry-run`,
			"papierkram voucher create --input beleg.json",
		},
	},
	{
		Resource: "voucher", Action: "describe", OperationID: "GetVoucher",
		Method: "GET", Path: "/expense/vouchers/{id}", Class: ClassReadOnly, Credits: 1,
		Columns: voucherDetailColumns,
		Short:   "Beleg anzeigen",
		Examples: []string{
			"papierkram voucher describe 295",
			"papierkram voucher describe 295 -o json",
		},
	},
	{
		Resource: "voucher", Action: "update", OperationID: "UpdateVoucher",
		Method: "PUT", Path: "/expense/vouchers/{id}", Class: ClassUpdate, Credits: 1,
		Columns: voucherDetailColumns,
		Short:   "Beleg ändern",
		Examples: []string{
			"papierkram voucher update 295 --due-date 2026-12-15",
			`papierkram voucher update 295 --item "name=Papier,amount=23.80,vat=19,category=Bürobedarf" --replace-items`,
			"papierkram voucher update 295 --input beleg.json",
		},
	},
	{
		Resource: "voucher", Action: "delete", OperationID: "DeleteVoucher",
		Method: "DELETE", Path: "/expense/vouchers/{id}", Class: ClassDestructive, Credits: 1,
		Short: "Beleg löschen",
		Examples: []string{
			"papierkram voucher delete 295",
			"papierkram voucher delete 295 --yes",
		},
	},
	{
		Resource: "voucher", Action: "archive", OperationID: "ArchiveVoucher",
		Method: "POST", Path: "/expense/vouchers/{id}/archive", Class: ClassArchive, Credits: 1,
		Columns: voucherDetailColumns,
		Short:   "Beleg archivieren",
		Examples: []string{
			"papierkram voucher archive 295",
			"papierkram voucher archive 295 -o json",
		},
	},
	{
		Resource: "voucher", Action: "unarchive", OperationID: "UnarchiveVoucher",
		Method: "POST", Path: "/expense/vouchers/{id}/unarchive", Class: ClassArchive, Credits: 1,
		Columns: voucherDetailColumns,
		Short:   "Beleg aus dem Archiv holen",
		Examples: []string{
			"papierkram voucher unarchive 295",
			"papierkram voucher unarchive 295 -o json",
		},
	},
	{
		Resource: "voucher", Action: "cancel", OperationID: "CancelVoucher",
		Method: "POST", Path: "/expense/vouchers/{id}/cancel", Class: ClassDestructive, Credits: 1,
		Columns: voucherDetailColumns,
		Short:   "Beleg stornieren",
		Examples: []string{
			"papierkram voucher cancel 295",
			"papierkram voucher cancel 295 --yes",
		},
	},
	{
		Resource: "voucher", Action: "reverse", OperationID: "CancelVoucherWithReverseEntry",
		Method: "POST", Path: "/expense/vouchers/{id}/cancel_with_reverse_entry", Class: ClassDestructive, Credits: 1,
		Columns: voucherDetailColumns,
		Short:   "Beleg mit Gegenbuchung stornieren (Generalumkehr)",
		Examples: []string{
			"papierkram voucher reverse 295",
			"papierkram voucher reverse 295 --yes -o json",
		},
	},
	{
		// own handler (Task 35, spec 9.5)
		Resource: "voucher", Action: "pay", OperationID: "PayVoucher",
		Method: "POST", Path: "/expense/vouchers/{id}/pay", Class: ClassDestructive, Credits: 1,
		Short: "Zahlung zu einem Beleg erfassen",
		Examples: []string{
			"papierkram voucher pay 295 --transaction 457",
			"papierkram voucher pay 295 --amount 165.80 --date 2026-11-04 --yes",
		},
		Custom: true,
	},
	{
		// own handler (Task 32, spec 9.2)
		Resource: "voucher", Action: "pdf", OperationID: "GetVoucherPdf",
		Method: "GET", Path: "/expense/vouchers/{id}/pdf", Class: ClassReadOnly, Credits: 1,
		Short: "Beleg als PDF herunterladen",
		Examples: []string{
			"papierkram voucher pdf 295 -O beleg.pdf",
			"papierkram voucher pdf 295 -O - > beleg.pdf",
		},
		Custom: true,
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
	// 52–58
	{
		Resource: "proposition", Action: "list", OperationID: "ListPropositions",
		Method: "GET", Path: "/income/propositions", Class: ClassReadOnly, Credits: 1,
		Columns: propositionColumns,
		Short:   "Artikel und Leistungen auflisten",
		Examples: []string{
			"papierkram proposition list",
			"papierkram proposition list --sort name -o wide",
			"papierkram proposition list --all -o json",
		},
	},
	{
		Resource: "proposition", Action: "create", OperationID: "CreateProposition",
		Method: "POST", Path: "/income/propositions", Class: ClassCreate, Credits: 10,
		Columns: propositionDetailColumns,
		Short:   "Artikel oder Leistung anlegen",
		Examples: []string{
			"papierkram proposition create --name Beratung --article-no AN-00010 --proposition-type service --time-unit hour --price 120 --vat-rate 19",
			`papierkram proposition create --name "Schulung vor Ort" --article-no AN-00011 --proposition-type service --time-unit day --price 950 --vat-rate 19% --dry-run`,
			"papierkram proposition create --input artikel.json",
		},
	},
	{
		Resource: "proposition", Action: "describe", OperationID: "GetProposition",
		Method: "GET", Path: "/income/propositions/{id}", Class: ClassReadOnly, Credits: 1,
		Columns: propositionDetailColumns,
		Short:   "Artikel oder Leistung anzeigen",
		Examples: []string{
			"papierkram proposition describe 386",
			"papierkram proposition describe 386 -o json",
		},
	},
	{
		Resource: "proposition", Action: "update", OperationID: "UpdateProposition",
		Method: "PUT", Path: "/income/propositions/{id}", Class: ClassUpdate, Credits: 1,
		Columns: propositionDetailColumns,
		Short:   "Artikel oder Leistung ändern",
		Examples: []string{
			"papierkram proposition update 386 --price 135.50",
			"papierkram proposition update 386 --vat-rate 7 --dry-run",
			"papierkram proposition update 386 --input artikel.json",
		},
	},
	{
		Resource: "proposition", Action: "delete", OperationID: "DeleteProposition",
		Method: "DELETE", Path: "/income/propositions/{id}", Class: ClassDestructive, Credits: 1,
		Short: "Artikel oder Leistung löschen",
		Examples: []string{
			"papierkram proposition delete 386",
			"papierkram proposition delete 386 --yes",
		},
	},
	{
		Resource: "proposition", Action: "archive", OperationID: "ArchiveProposition",
		Method: "POST", Path: "/income/propositions/{id}/archive", Class: ClassArchive, Credits: 1,
		Columns: propositionDetailColumns,
		Short:   "Artikel oder Leistung archivieren",
		Examples: []string{
			"papierkram proposition archive 386",
			"papierkram proposition archive 386 -o json",
		},
	},
	{
		Resource: "proposition", Action: "unarchive", OperationID: "UnarchiveProposition",
		Method: "POST", Path: "/income/propositions/{id}/unarchive", Class: ClassArchive, Credits: 1,
		Columns: propositionDetailColumns,
		Short:   "Artikel oder Leistung aus dem Archiv holen",
		Examples: []string{
			"papierkram proposition unarchive 386",
			"papierkram proposition unarchive 386 -o json",
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
	// 60–66
	{
		Resource: "project", Action: "list", OperationID: "ListProjects",
		Method: "GET", Path: "/projects", Class: ClassReadOnly, Credits: 1,
		Filters: []Filter{
			{Flag: "company", Param: "company_id", Kind: "id", Help: "Nur Projekte dieses Kunden (Firmen-ID)"},
		},
		Columns: projectColumns,
		Short:   "Projekte auflisten",
		Examples: []string{
			"papierkram project list",
			"papierkram project list --company 3283",
			"papierkram project list --all -o json",
		},
	},
	{
		Resource: "project", Action: "create", OperationID: "CreateProject",
		Method: "POST", Path: "/projects", Class: ClassCreate, Credits: 10,
		Columns: projectDetailColumns,
		Short:   "Projekt anlegen",
		Examples: []string{
			`papierkram project create --name "Website-Relaunch" --customer 3283 --budget-type money --budget-money 5000`,
			"papierkram project create --name Wartung --customer 3283 --start-date 2026-11-01 --end-date 2026-12-31 --color green --dry-run",
			"papierkram project create --input projekt.json",
		},
	},
	{
		// spec 9.8: without ID the project of the project file
		Resource: "project", Action: "describe", OperationID: "GetProject",
		Method: "GET", Path: "/projects/{id}", Class: ClassReadOnly, Credits: 1,
		Columns: projectDetailColumns,
		Short:   "Projekt anzeigen",
		Examples: []string{
			"papierkram project describe 4710",
			"papierkram project describe",
			"papierkram project describe 4710 -o json",
		},
		Preset: "project",
	},
	{
		Resource: "project", Action: "update", OperationID: "UpdateProject",
		Method: "PUT", Path: "/projects/{id}", Class: ClassUpdate, Credits: 1,
		Columns: projectDetailColumns,
		Short:   "Projekt ändern",
		Examples: []string{
			"papierkram project update 4710 --end-date 2027-03-31",
			"papierkram project update 4710 --budget-type working_time --budget-time 120 --budget-time-unit hour",
			"papierkram project update 4710 --attr 338=42",
		},
	},
	{
		Resource: "project", Action: "delete", OperationID: "DeleteProject",
		Method: "DELETE", Path: "/projects/{id}", Class: ClassDestructive, Credits: 1,
		Short: "Projekt löschen",
		Examples: []string{
			"papierkram project delete 4710",
			"papierkram project delete 4710 --yes",
		},
	},
	{
		Resource: "project", Action: "archive", OperationID: "ArchiveProject",
		Method: "POST", Path: "/projects/{id}/archive", Class: ClassArchive, Credits: 1,
		Columns: projectDetailColumns,
		Short:   "Projekt archivieren",
		Examples: []string{
			"papierkram project archive 4710",
			"papierkram project archive 4710 -o json",
		},
	},
	{
		Resource: "project", Action: "unarchive", OperationID: "UnarchiveProject",
		Method: "POST", Path: "/projects/{id}/unarchive", Class: ClassArchive, Credits: 1,
		Columns: projectDetailColumns,
		Short:   "Projekt aus dem Archiv holen",
		Examples: []string{
			"papierkram project unarchive 4710",
			"papierkram project unarchive 4710 -o json",
		},
	},
	// 67–73
	{
		Resource: "custom-attribute", Action: "list", OperationID: "ListCustomAttributes",
		Method: "GET", Path: "/settings/custom_attributes", Class: ClassReadOnly, Credits: 1,
		Columns: customAttributeColumns,
		Short:   "Benutzerdefinierte Datenfelder auflisten",
		Examples: []string{
			"papierkram custom-attribute list",
			"papierkram custom-attribute list -o wide",
		},
	},
	{
		Resource: "custom-attribute", Action: "create", OperationID: "CreateCustomAttribute",
		Method: "POST", Path: "/settings/custom_attributes", Class: ClassCreate, Credits: 10,
		Columns: customAttributeColumns,
		Short:   "Benutzerdefiniertes Datenfeld anlegen",
		Examples: []string{
			"papierkram custom-attribute create --name Kostenstelle --attributable-type Project --data-type string",
			`papierkram custom-attribute create --name "Rabatt in %" --attributable-type Contact::Company --data-type decimal --dry-run`,
		},
	},
	{
		Resource: "custom-attribute", Action: "describe", OperationID: "GetCustomAttribute",
		Method: "GET", Path: "/settings/custom_attributes/{id}", Class: ClassReadOnly, Credits: 1,
		Columns: customAttributeColumns,
		Short:   "Benutzerdefiniertes Datenfeld anzeigen",
		Examples: []string{
			"papierkram custom-attribute describe 338",
			"papierkram custom-attribute describe 338 -o json",
		},
	},
	{
		Resource: "custom-attribute", Action: "update", OperationID: "UpdateCustomAttribute",
		Method: "PUT", Path: "/settings/custom_attributes/{id}", Class: ClassUpdate, Credits: 1,
		Columns: customAttributeColumns,
		Short:   "Benutzerdefiniertes Datenfeld ändern",
		Examples: []string{
			"papierkram custom-attribute update 338 --name Kostenträger",
			"papierkram custom-attribute update 338 --input datenfeld.json",
		},
	},
	{
		Resource: "custom-attribute", Action: "delete", OperationID: "DeleteCustomAttribute",
		Method: "DELETE", Path: "/settings/custom_attributes/{id}", Class: ClassDestructive, Credits: 1,
		Short: "Benutzerdefiniertes Datenfeld löschen",
		Examples: []string{
			"papierkram custom-attribute delete 338",
			"papierkram custom-attribute delete 338 --yes",
		},
	},
	{
		Resource: "custom-attribute", Action: "archive", OperationID: "ArchiveCustomAttribute",
		Method: "POST", Path: "/settings/custom_attributes/{id}/archive", Class: ClassArchive, Credits: 1,
		Columns: customAttributeColumns,
		Short:   "Benutzerdefiniertes Datenfeld archivieren",
		Examples: []string{
			"papierkram custom-attribute archive 338",
			"papierkram custom-attribute archive 338 -o json",
		},
	},
	{
		Resource: "custom-attribute", Action: "unarchive", OperationID: "UnarchiveCustomAttribute",
		Method: "POST", Path: "/settings/custom_attributes/{id}/unarchive", Class: ClassArchive, Credits: 1,
		Columns: customAttributeColumns,
		Short:   "Benutzerdefiniertes Datenfeld aus dem Archiv holen",
		Examples: []string{
			"papierkram custom-attribute unarchive 338",
			"papierkram custom-attribute unarchive 338 -o json",
		},
	},
	// 74–80
	{
		// spec 9.8: without --project the project of the project file
		Resource: "task", Action: "list", OperationID: "ListTasks",
		Method: "GET", Path: "/tracker/tasks", Class: ClassReadOnly, Credits: 1,
		Filters: []Filter{
			{Flag: "project", Param: "project_id", Kind: "id", Help: "Nur Aufgaben dieses Projekts (ID)"},
			{Flag: "proposition", Param: "proposition_id", Kind: "id", Help: "Nur Aufgaben mit diesem Artikel (ID)"},
		},
		Columns: taskColumns,
		Short:   "Aufgaben auflisten",
		Examples: []string{
			"papierkram task list",
			"papierkram task list --project 4823",
			"papierkram task list --proposition 386 --all -o json",
		},
		Preset: "project",
	},
	{
		// spec 9.8: without --project the project of the project file
		Resource: "task", Action: "create", OperationID: "CreateTask",
		Method: "POST", Path: "/tracker/tasks", Class: ClassCreate, Credits: 10,
		Columns: taskDetailColumns,
		Short:   "Aufgabe anlegen",
		Examples: []string{
			`papierkram task create --name "Konzept erstellen" --project 4823 --deadline 2026-12-15`,
			"papierkram task create --name Support --project 4823 --proposition 386 --flagged --dry-run",
			"papierkram task create --input aufgabe.json",
		},
		Preset: "project",
	},
	{
		Resource: "task", Action: "describe", OperationID: "GetTask",
		Method: "GET", Path: "/tracker/tasks/{id}", Class: ClassReadOnly, Credits: 1,
		Columns: taskDetailColumns,
		Short:   "Aufgabe anzeigen",
		Examples: []string{
			"papierkram task describe 488",
			"papierkram task describe 488 -o wide",
		},
	},
	{
		Resource: "task", Action: "update", OperationID: "UpdateTask",
		Method: "PUT", Path: "/tracker/tasks/{id}", Class: ClassUpdate, Credits: 1,
		Columns: taskDetailColumns,
		Short:   "Aufgabe ändern",
		Examples: []string{
			"papierkram task update 488 --complete",
			"papierkram task update 488 --deadline 2027-01-31 --flagged=false",
			"papierkram task update 488 --input aufgabe.json",
		},
	},
	{
		Resource: "task", Action: "delete", OperationID: "DeleteTask",
		Method: "DELETE", Path: "/tracker/tasks/{id}", Class: ClassDestructive, Credits: 1,
		Short: "Aufgabe löschen",
		Examples: []string{
			"papierkram task delete 488",
			"papierkram task delete 488 --yes",
		},
	},
	{
		Resource: "task", Action: "archive", OperationID: "ArchiveTask",
		Method: "POST", Path: "/tracker/tasks/{id}/archive", Class: ClassArchive, Credits: 1,
		Columns: taskDetailColumns,
		Short:   "Aufgabe archivieren",
		Examples: []string{
			"papierkram task archive 488",
			"papierkram task archive 488 -o json",
		},
	},
	{
		Resource: "task", Action: "unarchive", OperationID: "UnarchiveTask",
		Method: "POST", Path: "/tracker/tasks/{id}/unarchive", Class: ClassArchive, Credits: 1,
		Columns: taskDetailColumns,
		Short:   "Aufgabe aus dem Archiv holen",
		Examples: []string{
			"papierkram task unarchive 488",
			"papierkram task unarchive 488 -o json",
		},
	},
	// 81–87; time log, start, stop, status and discard are custom commands
	// (spec 9.6) of a later task
	{
		// spec 9.8: without --project the project of the project file
		Resource: "time", Action: "list", OperationID: "ListTimeEntries",
		Method: "GET", Path: "/tracker/time_entries", Class: ClassReadOnly, Credits: 1,
		Filters: []Filter{
			{Flag: "project", Param: "project_id", Kind: "id", Help: "Nur Zeiteinträge dieses Projekts (ID)"},
			{Flag: "task", Param: "task_id", Kind: "id", Help: "Nur Zeiteinträge dieser Aufgabe (ID)"},
			{Flag: "invoice", Param: "invoice_id", Kind: "id", Help: "Nur Zeiteinträge dieser Rechnung (ID)"},
			{Flag: "user", Param: "user_id", Kind: "id", Help: "Nur Zeiteinträge dieses Benutzers (ID)"},
			{Flag: "state", Param: "billing_state", Kind: "enum", Help: "Nur Zeiteinträge mit diesem Abrechnungsstatus"},
			{Flag: "since", Param: "start_time_range_start", Kind: "datetime_from",
				Help: "Nur Zeiteinträge mit Beginn ab diesem Zeitpunkt (RFC 3339; YYYY-MM-DD, heute, gestern: ab 00:00:00 Uhr lokaler Zeit)"},
			{Flag: "until", Param: "start_time_range_end", Kind: "datetime_to",
				Help: "Nur Zeiteinträge mit Beginn bis zu diesem Zeitpunkt (RFC 3339; YYYY-MM-DD, heute, gestern: bis 23:59:59 Uhr lokaler Zeit)"},
		},
		Columns: timeColumns,
		Short:   "Zeiteinträge auflisten",
		Examples: []string{
			"papierkram time list --since 2026-10-01 --until 2026-10-31",
			"papierkram time list --project 4963 --state unbilled -o wide",
			"papierkram time list --user 1 --since heute --all -o json",
		},
		Preset: "project",
	},
	{
		Resource: "time", Action: "create", OperationID: "CreateTimeEntry",
		Method: "POST", Path: "/tracker/time_entries", Class: ClassCreate, Credits: 10,
		Columns: timeDetailColumns,
		Short:   "Zeiteintrag anlegen",
		Examples: []string{
			"papierkram time create --entry-date 2026-11-04 --started-at-time 09:00 --ended-at-time 10:30 --task 545 --user 1",
			`papierkram time create --entry-date heute --started-at-time 13:00 --ended-at-time 14:15 --task 545 --user 1 --comments "Abstimmung mit Kunde" --dry-run`,
			"papierkram time create --input zeit.json",
		},
	},
	{
		Resource: "time", Action: "describe", OperationID: "GetTimeEntry",
		Method: "GET", Path: "/tracker/time_entries/{id}", Class: ClassReadOnly, Credits: 1,
		Columns: timeDetailColumns,
		Short:   "Zeiteintrag anzeigen",
		Examples: []string{
			"papierkram time describe 272",
			"papierkram time describe 272 -o wide",
		},
	},
	{
		Resource: "time", Action: "update", OperationID: "UpdateTimeEntry",
		Method: "PUT", Path: "/tracker/time_entries/{id}", Class: ClassUpdate, Credits: 1,
		Columns: timeDetailColumns,
		Short:   "Zeiteintrag ändern",
		Examples: []string{
			"papierkram time update 272 --ended-at-time 12:30",
			`papierkram time update 272 --unbillable --comments "Interne Abstimmung"`,
			"papierkram time update 272 --input zeit.json",
		},
	},
	{
		Resource: "time", Action: "delete", OperationID: "DeleteTimeEntry",
		Method: "DELETE", Path: "/tracker/time_entries/{id}", Class: ClassDestructive, Credits: 1,
		Short: "Zeiteintrag löschen",
		Examples: []string{
			"papierkram time delete 272",
			"papierkram time delete 272 --yes",
		},
	},
	{
		Resource: "time", Action: "archive", OperationID: "ArchiveTimeEntry",
		Method: "POST", Path: "/tracker/time_entries/{id}/archive", Class: ClassArchive, Credits: 1,
		Columns: timeDetailColumns,
		Short:   "Zeiteintrag archivieren",
		Examples: []string{
			"papierkram time archive 272",
			"papierkram time archive 272 -o json",
		},
	},
	{
		Resource: "time", Action: "unarchive", OperationID: "UnarchiveTimeEntry",
		Method: "POST", Path: "/tracker/time_entries/{id}/unarchive", Class: ClassArchive, Credits: 1,
		Columns: timeDetailColumns,
		Short:   "Zeiteintrag aus dem Archiv holen",
		Examples: []string{
			"papierkram time unarchive 272",
			"papierkram time unarchive 272 -o json",
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
