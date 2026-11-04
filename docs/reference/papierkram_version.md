# papierkram version

Version, Commit und Build-Datum anzeigen

```text
Version, Commit und Build-Datum anzeigen

Verwendung:
  papierkram version [Optionen]

Beispiele:
  papierkram version
  papierkram version -o json

Optionen:
  -h, --help   Hilfe zu version

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

- [papierkram](papierkram.md) – Kommandozeile für die Papierkram-API
