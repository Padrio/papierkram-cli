# papierkram custom-attribute

Benutzerdefinierte Datenfelder verwalten

```text
Benutzerdefinierte Datenfelder verwalten

Verwendung:
  papierkram custom-attribute [Optionen]
  papierkram custom-attribute [Befehl]

Aliase:
  custom-attribute, datenfeld

Verfügbare Befehle:
  archive     Benutzerdefiniertes Datenfeld archivieren
  create      Benutzerdefiniertes Datenfeld anlegen
  delete      Benutzerdefiniertes Datenfeld löschen
  describe    Benutzerdefiniertes Datenfeld anzeigen
  list        Benutzerdefinierte Datenfelder auflisten
  unarchive   Benutzerdefiniertes Datenfeld aus dem Archiv holen
  update      Benutzerdefiniertes Datenfeld ändern

Optionen:
  -h, --help   Hilfe zu custom-attribute

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

Mit "papierkram custom-attribute [Befehl] --help" gibt es weitere Informationen zu einem Befehl.
```

## Siehe auch

- [papierkram](papierkram.md) – Kommandozeile für die Papierkram-API
- [papierkram custom-attribute archive](papierkram_custom-attribute_archive.md) – Benutzerdefiniertes Datenfeld archivieren
- [papierkram custom-attribute create](papierkram_custom-attribute_create.md) – Benutzerdefiniertes Datenfeld anlegen
- [papierkram custom-attribute delete](papierkram_custom-attribute_delete.md) – Benutzerdefiniertes Datenfeld löschen
- [papierkram custom-attribute describe](papierkram_custom-attribute_describe.md) – Benutzerdefiniertes Datenfeld anzeigen
- [papierkram custom-attribute list](papierkram_custom-attribute_list.md) – Benutzerdefinierte Datenfelder auflisten
- [papierkram custom-attribute unarchive](papierkram_custom-attribute_unarchive.md) – Benutzerdefiniertes Datenfeld aus dem Archiv holen
- [papierkram custom-attribute update](papierkram_custom-attribute_update.md) – Benutzerdefiniertes Datenfeld ändern
