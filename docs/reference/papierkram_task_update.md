# papierkram task update

Aufgabe ändern

```text
Aufgabe ändern.

Kosten: 1 Credit je Anfrage.

Verwendung:
  papierkram task update <id> [Optionen]

Beispiele:
  papierkram task update 488 --complete
  papierkram task update 488 --deadline 2027-01-31 --flagged=false
  papierkram task update 488 --input aufgabe.json

Optionen:
      --complete              Wert für complete (true/false)
      --deadline Datum        Wert für deadline (YYYY-MM-DD, heute, gestern)
      --flagged               Wert für flagged (true/false)
  -h, --help                  Hilfe zu update
      --input Text            Body als JSON-Datei oder - für die Standardeingabe; Flags überschreiben einzelne Felder
      --name Text             Wert für name
      --project ID            ID für project
      --proposition ID        ID für proposition
      --relative-costs Text   Wert für relative_costs
      --user ID               ID für user

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

- [papierkram task](papierkram_task.md) – Aufgaben verwalten
