package registry

// infoShort is the short help of the info resource and its command.
const infoShort = "Account-Informationen inkl. Rechnungsvorlagen anzeigen"

// resources are the command nouns of spec 7.1: English as in the API,
// German aliases. info is a command of its own (Action "").
var resources = []Resource{
	{Name: "company", Aliases: []string{"firma", "kontakt"}, Short: "Firmen (Kunden und Lieferanten) verwalten", Noun: "Firma"},
	{Name: "person", Aliases: []string{"kontaktperson"}, Short: "Kontaktpersonen einer Firma verwalten", Noun: "Kontaktperson"},
	{Name: "invoice", Aliases: []string{"rechnung"}, Short: "Rechnungen verwalten", Noun: "Rechnung"},
	{Name: "estimate", Aliases: []string{"angebot"}, Short: "Angebote verwalten", Noun: "Angebot"},
	{Name: "voucher", Aliases: []string{"beleg", "expense"}, Short: "Belege (Ausgaben) verwalten", Noun: "Beleg"},
	{Name: "proposition", Aliases: []string{"artikel", "leistung"}, Short: "Artikel und Leistungen verwalten", Noun: "Artikel"},
	{Name: "payment-term", Aliases: []string{"zahlungsbedingung"}, Short: "Zahlungsbedingungen anzeigen", Noun: "Zahlungsbedingung"},
	{Name: "project", Aliases: []string{"projekt"}, Short: "Projekte verwalten", Noun: "Projekt"},
	{Name: "task", Aliases: []string{"aufgabe"}, Short: "Aufgaben verwalten", Noun: "Aufgabe"},
	{Name: "time", Aliases: []string{"zeit", "time-entry"}, Short: "Zeiteinträge erfassen und verwalten", Noun: "Zeiteintrag"},
	{Name: "custom-attribute", Aliases: []string{"datenfeld"}, Short: "Benutzerdefinierte Datenfelder verwalten", Noun: "Datenfeld"},
	{Name: "bank-connection", Aliases: []string{"bankverbindung"}, Short: "Bankverbindungen anzeigen", Noun: "Bankverbindung"},
	{Name: "transaction", Aliases: []string{"umsatz"}, Short: "Bankumsätze anzeigen", Noun: "Umsatz"},
	{Name: "user", Aliases: []string{"benutzer"}, Short: "Benutzer des Accounts anzeigen", Noun: "Benutzer"},
	{Name: "info", Short: infoShort, Noun: "Account-Information"},
}
