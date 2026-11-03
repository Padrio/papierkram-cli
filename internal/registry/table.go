package registry

import "github.com/Padrio/papierkram-cli/internal/output"

// Default columns (spec 10.3). Paths are the JSON paths of the API objects.
var (
	// customer_no/supplier_no (spec 10.3) are two columns: a column shows
	// one path, and contact_type says which of the two numbers is set.
	companyColumns = output.Columns{
		Table: []output.Column{
			{Header: "ID", Path: "id"},
			{Header: "NAME", Path: "name"},
			{Header: "TYP", Path: "contact_type"},
			{Header: "KUNDENNR.", Path: "customer_no"},
			{Header: "LIEFERANTENNR.", Path: "supplier_no"},
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
