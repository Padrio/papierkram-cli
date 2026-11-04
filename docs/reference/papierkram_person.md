# papierkram person

Kontaktpersonen einer Firma verwalten

```text
Kontaktpersonen einer Firma verwalten

Verwendung:
  papierkram person [Optionen]
  papierkram person [Befehl]

Aliase:
  person, kontaktperson

Verfügbare Befehle:
  create      Kontaktperson anlegen
  delete      Kontaktperson löschen
  describe    Kontaktperson anzeigen
  list        Kontaktpersonen einer Firma auflisten
  update      Kontaktperson ändern

Optionen:
  -h, --help   Hilfe zu person

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

Mit "papierkram person [Befehl] --help" gibt es weitere Informationen zu einem Befehl.
```

## Siehe auch

- [papierkram](papierkram.md) – Kommandozeile für die Papierkram-API
- [papierkram person create](papierkram_person_create.md) – Kontaktperson anlegen
- [papierkram person delete](papierkram_person_delete.md) – Kontaktperson löschen
- [papierkram person describe](papierkram_person_describe.md) – Kontaktperson anzeigen
- [papierkram person list](papierkram_person_list.md) – Kontaktpersonen einer Firma auflisten
- [papierkram person update](papierkram_person_update.md) – Kontaktperson ändern
