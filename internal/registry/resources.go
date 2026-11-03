package registry

// resources are the command nouns of spec 7.1: English as in the API,
// German aliases. info is a command of its own (Action "").
var resources = []Resource{
	{Name: "company", Aliases: []string{"firma", "kontakt"}, Short: "Firmen (Kunden und Lieferanten) verwalten"},
	{Name: "person", Aliases: []string{"kontaktperson"}, Short: "Kontaktpersonen einer Firma verwalten"},
	{Name: "invoice", Aliases: []string{"rechnung"}, Short: "Rechnungen verwalten"},
	{Name: "estimate", Aliases: []string{"angebot"}, Short: "Angebote verwalten"},
	{Name: "voucher", Aliases: []string{"beleg", "expense"}, Short: "Belege (Ausgaben) verwalten"},
	{Name: "proposition", Aliases: []string{"artikel", "leistung"}, Short: "Artikel und Leistungen verwalten"},
	{Name: "payment-term", Aliases: []string{"zahlungsbedingung"}, Short: "Zahlungsbedingungen anzeigen"},
	{Name: "project", Aliases: []string{"projekt"}, Short: "Projekte verwalten"},
	{Name: "task", Aliases: []string{"aufgabe"}, Short: "Aufgaben verwalten"},
	{Name: "time", Aliases: []string{"zeit", "time-entry"}, Short: "Zeiteinträge erfassen und verwalten"},
	{Name: "custom-attribute", Aliases: []string{"datenfeld"}, Short: "Benutzerdefinierte Datenfelder verwalten"},
	{Name: "bank-connection", Aliases: []string{"bankverbindung"}, Short: "Bankverbindungen anzeigen"},
	{Name: "transaction", Aliases: []string{"umsatz"}, Short: "Bankumsätze anzeigen"},
	{Name: "user", Aliases: []string{"benutzer"}, Short: "Benutzer des Accounts anzeigen"},
	{Name: "info", Short: "Account-Informationen inkl. Rechnungsvorlagen anzeigen"},
}
