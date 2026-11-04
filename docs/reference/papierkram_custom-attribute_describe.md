# papierkram custom-attribute describe

Benutzerdefiniertes Datenfeld anzeigen

```text
Benutzerdefiniertes Datenfeld anzeigen.

Kosten: 1 Credit je Anfrage.

Verwendung:
  papierkram custom-attribute describe <id> [Optionen]

Beispiele:
  papierkram custom-attribute describe 338
  papierkram custom-attribute describe 338 -o json

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
      --timeout Dauer   Zeitlimit je Anfrage, pro Versuch; GET wird bis zu zweimal wiederholt (Standard: 30s)
  -y, --yes             Bestätigungen automatisch bejahen
```

## Siehe auch

- [papierkram custom-attribute](papierkram_custom-attribute.md) – Benutzerdefinierte Datenfelder verwalten
