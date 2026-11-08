# papierkram

Kommandozeile für die Papierkram-API

```text
papierkram steuert Papierkram (Buchhaltung) von der Kommandozeile aus.

Verwendung:
  papierkram [Optionen]
  papierkram [Befehl]

Verfügbare Befehle:
  api              Beliebige Anfrage an die Papierkram-API senden (wie gh api)
  auth             API-Token anmelden, abmelden und prüfen
  bank-connection  Bankverbindungen anzeigen
  commands         Alle Befehle mit Klasse, Credits, Flags und Beispielen auflisten
  company          Firmen (Kunden und Lieferanten) verwalten
  completion       Skript für die Shell-Vervollständigung ausgeben
  context          Kontexte (Account-Setups) verwalten
  custom-attribute Benutzerdefinierte Datenfelder verwalten
  estimate         Angebote verwalten
  help             Hilfe zu einem Befehl anzeigen
  info             Account-Informationen inkl. Rechnungsvorlagen anzeigen
  invoice          Rechnungen verwalten
  local            Projektdatei .papierkram.toml anlegen und anzeigen
  payment-term     Zahlungsbedingungen anzeigen
  person           Kontaktpersonen einer Firma verwalten
  project          Projekte verwalten
  proposition      Artikel und Leistungen verwalten
  schema           Request- und Response-Schema einer Aktion als JSON anzeigen
  task             Aufgaben verwalten
  time             Zeiteinträge erfassen und verwalten
  transaction      Bankumsätze anzeigen
  user             Benutzer des Accounts anzeigen
  version          Version, Commit und Build-Datum anzeigen
  voucher          Belege (Ausgaben) verwalten

Optionen:
      --context Text    Kontext statt des aktiven verwenden
      --debug           HTTP-Debug-Ausgabe auf stderr (ohne Token)
      --dry-run         Mutation nur anzeigen, nicht senden
      --fields Text     Felder der Ausgabe, kommagetrennt
  -h, --help            Hilfe zu papierkram
      --jq Text         jq-Ausdruck auf die Ausgabe anwenden
      --no-color        Farben abschalten
      --no-input        keine interaktiven Rückfragen
  -o, --output Text     Ausgabeformat: table|wide|json|yaml (Standard: "table")
      --timeout Dauer   Zeitlimit je Anfrage, pro Versuch (Uploads ohne Angabe mindestens 120 s); GET wird bis zu zweimal wiederholt (Standard: 30s)
  -y, --yes             Bestätigungen automatisch bejahen

Mit "papierkram [Befehl] --help" gibt es weitere Informationen zu einem Befehl.
```

## Siehe auch

- [papierkram api](papierkram_api.md) – Beliebige Anfrage an die Papierkram-API senden (wie gh api)
- [papierkram auth](papierkram_auth.md) – API-Token anmelden, abmelden und prüfen
- [papierkram bank-connection](papierkram_bank-connection.md) – Bankverbindungen anzeigen
- [papierkram commands](papierkram_commands.md) – Alle Befehle mit Klasse, Credits, Flags und Beispielen auflisten
- [papierkram company](papierkram_company.md) – Firmen (Kunden und Lieferanten) verwalten
- [papierkram completion](papierkram_completion.md) – Skript für die Shell-Vervollständigung ausgeben
- [papierkram context](papierkram_context.md) – Kontexte (Account-Setups) verwalten
- [papierkram custom-attribute](papierkram_custom-attribute.md) – Benutzerdefinierte Datenfelder verwalten
- [papierkram estimate](papierkram_estimate.md) – Angebote verwalten
- [papierkram info](papierkram_info.md) – Account-Informationen inkl. Rechnungsvorlagen anzeigen
- [papierkram invoice](papierkram_invoice.md) – Rechnungen verwalten
- [papierkram local](papierkram_local.md) – Projektdatei .papierkram.toml anlegen und anzeigen
- [papierkram payment-term](papierkram_payment-term.md) – Zahlungsbedingungen anzeigen
- [papierkram person](papierkram_person.md) – Kontaktpersonen einer Firma verwalten
- [papierkram project](papierkram_project.md) – Projekte verwalten
- [papierkram proposition](papierkram_proposition.md) – Artikel und Leistungen verwalten
- [papierkram schema](papierkram_schema.md) – Request- und Response-Schema einer Aktion als JSON anzeigen
- [papierkram task](papierkram_task.md) – Aufgaben verwalten
- [papierkram time](papierkram_time.md) – Zeiteinträge erfassen und verwalten
- [papierkram transaction](papierkram_transaction.md) – Bankumsätze anzeigen
- [papierkram user](papierkram_user.md) – Benutzer des Accounts anzeigen
- [papierkram version](papierkram_version.md) – Version, Commit und Build-Datum anzeigen
- [papierkram voucher](papierkram_voucher.md) – Belege (Ausgaben) verwalten
