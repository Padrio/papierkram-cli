# papierkram project describe

Projekt anzeigen

```text
Projekt anzeigen.

Kosten: 1 Credit je Anfrage.

Ohne die ID gilt PAPIERKRAM_PROJECT oder project aus der Projektdatei .papierkram.toml (anlegen: papierkram local init).

Verwendung:
  papierkram project describe [<id>] [Optionen]

Beispiele:
  papierkram project describe 4710
  papierkram project describe
  papierkram project describe 4710 -o json

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

- [papierkram project](papierkram_project.md) – Projekte verwalten
