# papierkram person delete

Kontaktperson löschen

```text
Kontaktperson löschen.

Kosten: 1 Credit je Anfrage.

Ohne --company gilt PAPIERKRAM_COMPANY oder company aus der Projektdatei .papierkram.toml (anlegen: papierkram local init).

Verwendung:
  papierkram person delete <id> [Optionen]

Beispiele:
  papierkram person delete 1063 --company 2862
  papierkram person delete 1063 --company 2862 --yes

Optionen:
      --company Zahl   ID des übergeordneten Objekts (company_id); ohne Angabe PAPIERKRAM_COMPANY oder company aus der Projektdatei .papierkram.toml (Pflicht)
  -h, --help           Hilfe zu delete

Globale Optionen:
      --context Text    Kontext statt des aktiven verwenden
      --debug           HTTP-Debug-Ausgabe auf stderr (ohne Token)
      --dry-run         Mutation nur anzeigen, nicht senden
      --fields Text     Felder der Ausgabe, kommagetrennt
      --jq Text         jq-Ausdruck auf die Ausgabe anwenden
      --no-color        Farben abschalten
      --no-input        keine interaktiven Rückfragen
  -o, --output Text     Ausgabeformat: table|wide|json|yaml (Standard: "table")
      --timeout Dauer   Zeitlimit je Anfrage, pro Versuch (Uploads ohne Angabe mindestens 120 s); GET wird bis zu zweimal wiederholt (Standard: 30s)
  -y, --yes             Bestätigungen automatisch bejahen
```

## Siehe auch

- [papierkram person](papierkram_person.md) – Kontaktpersonen einer Firma verwalten
