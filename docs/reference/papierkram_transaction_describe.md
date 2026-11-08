# papierkram transaction describe

Bankumsatz anzeigen

```text
Bankumsatz anzeigen.

Kosten: 1 Credit je Anfrage.

Verwendung:
  papierkram transaction describe <id> [Optionen]

Beispiele:
  papierkram transaction describe 457
  papierkram transaction describe 457 -o json

Optionen:
  -h, --help   Hilfe zu describe

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

- [papierkram transaction](papierkram_transaction.md) – Bankumsätze anzeigen
