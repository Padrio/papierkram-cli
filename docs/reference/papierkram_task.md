# papierkram task

Aufgaben verwalten

```text
Aufgaben verwalten

Verwendung:
  papierkram task [Optionen]
  papierkram task [Befehl]

Aliase:
  task, aufgabe

Verfügbare Befehle:
  archive     Aufgabe archivieren
  create      Aufgabe anlegen
  delete      Aufgabe löschen
  describe    Aufgabe anzeigen
  list        Aufgaben auflisten
  unarchive   Aufgabe aus dem Archiv holen
  update      Aufgabe ändern

Optionen:
  -h, --help   Hilfe zu task

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

Mit "papierkram task [Befehl] --help" gibt es weitere Informationen zu einem Befehl.
```

## Siehe auch

- [papierkram](papierkram.md) – Kommandozeile für die Papierkram-API
- [papierkram task archive](papierkram_task_archive.md) – Aufgabe archivieren
- [papierkram task create](papierkram_task_create.md) – Aufgabe anlegen
- [papierkram task delete](papierkram_task_delete.md) – Aufgabe löschen
- [papierkram task describe](papierkram_task_describe.md) – Aufgabe anzeigen
- [papierkram task list](papierkram_task_list.md) – Aufgaben auflisten
- [papierkram task unarchive](papierkram_task_unarchive.md) – Aufgabe aus dem Archiv holen
- [papierkram task update](papierkram_task_update.md) – Aufgabe ändern
