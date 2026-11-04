# papierkram time create

Zeiteintrag anlegen

```text
Zeiteintrag anlegen.

Kosten: 10 Credits je Anfrage.

Verwendung:
  papierkram time create [Optionen]

Beispiele:
  papierkram time create --entry-date 2026-11-04 --started-at-time 09:00 --ended-at-time 10:30 --task 545 --user 1
  papierkram time create --entry-date heute --started-at-time 13:00 --ended-at-time 14:15 --task 545 --user 1 --comments "Abstimmung mit Kunde" --dry-run
  papierkram time create --input zeit.json

Optionen:
      --billable-duration Zahl   Abrechenbare Dauer in Sekunden; wirkungslos: Die API setzt beim Anlegen und Ändern die Dauer aus Beginn und Ende ein (gemessen 2026-11-04)
      --comments Text            Wert für comments
      --ended-at-time Text       Wert für ended_at_time (HH:MM) (Pflicht)
      --entry-date Datum         Wert für entry_date (YYYY-MM-DD, heute, gestern) (Pflicht)
  -h, --help                     Hilfe zu create
      --input Text               Body als JSON-Datei oder - für die Standardeingabe; Flags überschreiben einzelne Felder
      --started-at-time Text     Wert für started_at_time (HH:MM) (Pflicht)
      --task ID                  ID für task (Pflicht)
      --unbillable               Wert für unbillable (true/false)
      --user ID                  ID für user (Pflicht)

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
