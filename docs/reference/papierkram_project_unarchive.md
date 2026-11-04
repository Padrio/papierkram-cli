# papierkram project unarchive

Projekt aus dem Archiv holen

```text
Projekt aus dem Archiv holen.

Kosten: 1 Credit je Anfrage.

Verwendung:
  papierkram project unarchive <id> [Optionen]

Beispiele:
  papierkram project unarchive 4710
  papierkram project unarchive 4710 -o json

Optionen:
  -h, --help   Hilfe zu unarchive

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

- [papierkram project](papierkram_project.md) – Projekte verwalten
