# papierkram company

Firmen (Kunden und Lieferanten) verwalten

```text
Firmen (Kunden und Lieferanten) verwalten

Verwendung:
  papierkram company [Optionen]
  papierkram company [Befehl]

Aliase:
  company, firma, kontakt

Verfügbare Befehle:
  archive     Firma archivieren
  create      Firma anlegen
  delete      Firma löschen
  describe    Firma anzeigen
  list        Firmen auflisten
  unarchive   Firma aus dem Archiv holen
  update      Firma ändern

Optionen:
  -h, --help   Hilfe zu company

Globale Optionen:
      --context Text    Kontext statt des aktiven verwenden
      --debug           HTTP-Debug-Ausgabe auf stderr (ohne Token)
      --dry-run         Mutation nur anzeigen, nicht senden
      --fields Text     Felder der Ausgabe, kommagetrennt
      --jq Text         jq-Ausdruck auf die Ausgabe anwenden
      --no-color        Farben abschalten
      --no-input        keine interaktiven Rückfragen
  -o, --output Text     Ausgabeformat: table|wide|json|yaml (Standard: "table")
      --timeout Dauer   Zeitlimit je Anfrage, pro Versuch; GET wird bis zu zweimal wiederholt (Standard: 30s)
  -y, --yes             Bestätigungen automatisch bejahen

Mit "papierkram company [Befehl] --help" gibt es weitere Informationen zu einem Befehl.
```

## Siehe auch

- [papierkram](papierkram.md) – Kommandozeile für die Papierkram-API
- [papierkram company archive](papierkram_company_archive.md) – Firma archivieren
- [papierkram company create](papierkram_company_create.md) – Firma anlegen
- [papierkram company delete](papierkram_company_delete.md) – Firma löschen
- [papierkram company describe](papierkram_company_describe.md) – Firma anzeigen
- [papierkram company list](papierkram_company_list.md) – Firmen auflisten
- [papierkram company unarchive](papierkram_company_unarchive.md) – Firma aus dem Archiv holen
- [papierkram company update](papierkram_company_update.md) – Firma ändern
