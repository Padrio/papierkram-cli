# papierkram transaction

Bankumsätze anzeigen

```text
Bankumsätze anzeigen

Verwendung:
  papierkram transaction [Optionen]
  papierkram transaction [Befehl]

Aliase:
  transaction, umsatz

Verfügbare Befehle:
  describe    Bankumsatz anzeigen
  list        Bankumsätze auflisten

Optionen:
  -h, --help   Hilfe zu transaction

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

Mit "papierkram transaction [Befehl] --help" gibt es weitere Informationen zu einem Befehl.
```

## Siehe auch

- [papierkram](papierkram.md) – Kommandozeile für die Papierkram-API
- [papierkram transaction describe](papierkram_transaction_describe.md) – Bankumsatz anzeigen
- [papierkram transaction list](papierkram_transaction_list.md) – Bankumsätze auflisten
