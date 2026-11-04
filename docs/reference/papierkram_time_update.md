# papierkram time update

Zeiteintrag ändern

```text
Zeiteintrag ändern.

Kosten: 1 Credit je Anfrage.

Verwendung:
  papierkram time update <id> [Optionen]

Beispiele:
  papierkram time update 272 --ended-at-time 12:30
  papierkram time update 272 --unbillable --comments "Interne Abstimmung"
  papierkram time update 272 --input zeit.json

Optionen:
      --billable-duration Zahl   Abrechenbare Dauer in Sekunden
      --comments Text            Wert für comments
      --ended-at-time Text       Wert für ended_at_time (HH:MM)
      --entry-date Datum         Wert für entry_date (YYYY-MM-DD, heute, gestern)
  -h, --help                     Hilfe zu update
      --input Text               Body als JSON-Datei oder - für die Standardeingabe; Flags überschreiben einzelne Felder
      --started-at-time Text     Wert für started_at_time (HH:MM)
      --task ID                  ID für task
      --unbillable               Wert für unbillable (true/false)
      --user ID                  ID für user

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

- [papierkram time](papierkram_time.md) – Zeiteinträge erfassen und verwalten
