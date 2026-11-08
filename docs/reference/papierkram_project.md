# papierkram project

Projekte verwalten

```text
Projekte verwalten

Verwendung:
  papierkram project [Optionen]
  papierkram project [Befehl]

Aliase:
  project, projekt

Verfügbare Befehle:
  archive     Projekt archivieren
  create      Projekt anlegen
  delete      Projekt löschen
  describe    Projekt anzeigen
  list        Projekte auflisten
  unarchive   Projekt aus dem Archiv holen
  update      Projekt ändern

Optionen:
  -h, --help   Hilfe zu project

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

Mit "papierkram project [Befehl] --help" gibt es weitere Informationen zu einem Befehl.
```

## Siehe auch

- [papierkram](papierkram.md) – Kommandozeile für die Papierkram-API
- [papierkram project archive](papierkram_project_archive.md) – Projekt archivieren
- [papierkram project create](papierkram_project_create.md) – Projekt anlegen
- [papierkram project delete](papierkram_project_delete.md) – Projekt löschen
- [papierkram project describe](papierkram_project_describe.md) – Projekt anzeigen
- [papierkram project list](papierkram_project_list.md) – Projekte auflisten
- [papierkram project unarchive](papierkram_project_unarchive.md) – Projekt aus dem Archiv holen
- [papierkram project update](papierkram_project_update.md) – Projekt ändern
