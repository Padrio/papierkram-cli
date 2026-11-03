package registry

import "github.com/Padrio/papierkram-cli/internal/output"

// Default columns (spec 10.3). Paths are the JSON paths of the API objects.
var (
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
