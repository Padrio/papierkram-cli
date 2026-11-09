# papierkram project update

Projekt ändern

```text
Projekt ändern.

Kosten: 1 Credit je Anfrage.

Verwendung:
  papierkram project update <id> [Optionen]

Beispiele:
  papierkram project update 4710 --end-date 2027-03-31
  papierkram project update 4710 --budget-type working_time --budget-time 120 --budget-time-unit hour
  papierkram project update 4710 --attr 338=42

Optionen:
      --attr Texte               eigenes Feld als <id>=<wert> (wiederholbar)
      --budget-money Text        Wert für budget_money
      --budget-time Text         Wert für budget_time
      --budget-time-unit Text    Wert für budget_time_unit (erlaubt: hour, day)
      --budget-type Text         Wert für budget_type (erlaubt: money, working_time)
      --color Text               Wert für color (erlaubt: blue, green, lime, pink, purple, orange)
      --customer ID              ID oder Name für customer
      --default-proposition ID   ID oder Name für default_proposition
      --description Text         Wert für description
      --end-date Datum           Wert für end_date (YYYY-MM-DD, heute, gestern)
      --flagged                  Wert für flagged (true/false)
  -h, --help                     Hilfe zu update
      --input Text               Body als JSON-Datei oder - für die Standardeingabe; Flags überschreiben einzelne Felder
      --name Text                Wert für name
      --start-date Datum         Wert für start_date (YYYY-MM-DD, heute, gestern)
      --team-member Texte        ID oder Name für team_members (wiederholbar)

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
