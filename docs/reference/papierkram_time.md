# papierkram time

Zeiteinträge erfassen und verwalten

```text
Zeiteinträge erfassen und verwalten

Verwendung:
  papierkram time [Optionen]
  papierkram time [Befehl]

Aliase:
  time, zeit, time-entry

Verfügbare Befehle:
  archive     Zeiteintrag archivieren
  create      Zeiteintrag anlegen
  delete      Zeiteintrag löschen
  describe    Zeiteintrag anzeigen
  discard     Timer verwerfen, ohne zu buchen
  list        Zeiteinträge auflisten
  log         Zeit erfassen (Zeiteintrag mit Zeitraum, Aufgabe und Benutzer)
  start       Timer starten (lokal, Buchung mit time stop)
  status      Laufenden Timer anzeigen
  stop        Timer stoppen und als Zeiteintrag buchen
  unarchive   Zeiteintrag aus dem Archiv holen
  update      Zeiteintrag ändern

Optionen:
  -h, --help   Hilfe zu time

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

Mit "papierkram time [Befehl] --help" gibt es weitere Informationen zu einem Befehl.
```

## Siehe auch

- [papierkram](papierkram.md) – Kommandozeile für die Papierkram-API
- [papierkram time archive](papierkram_time_archive.md) – Zeiteintrag archivieren
- [papierkram time create](papierkram_time_create.md) – Zeiteintrag anlegen
- [papierkram time delete](papierkram_time_delete.md) – Zeiteintrag löschen
- [papierkram time describe](papierkram_time_describe.md) – Zeiteintrag anzeigen
- [papierkram time discard](papierkram_time_discard.md) – Timer verwerfen, ohne zu buchen
- [papierkram time list](papierkram_time_list.md) – Zeiteinträge auflisten
- [papierkram time log](papierkram_time_log.md) – Zeit erfassen (Zeiteintrag mit Zeitraum, Aufgabe und Benutzer)
- [papierkram time start](papierkram_time_start.md) – Timer starten (lokal, Buchung mit time stop)
- [papierkram time status](papierkram_time_status.md) – Laufenden Timer anzeigen
- [papierkram time stop](papierkram_time_stop.md) – Timer stoppen und als Zeiteintrag buchen
- [papierkram time unarchive](papierkram_time_unarchive.md) – Zeiteintrag aus dem Archiv holen
- [papierkram time update](papierkram_time_update.md) – Zeiteintrag ändern
