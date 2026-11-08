# papierkram context

Kontexte (Account-Setups) verwalten

```text
Kontexte verwalten, ähnlich wie bei hcloud: anlegen, wechseln, auflisten, umbenennen und löschen.
Befehle, die die Konfiguration ändern, schreiben config.toml neu; Kommentare und eigene Formatierung gehen dabei verloren.

Verwendung:
  papierkram context [Optionen]
  papierkram context [Befehl]

Verfügbare Befehle:
  active      Aktiven Kontext anzeigen
  create      Kontext anlegen
  delete      Kontext löschen
  list        Kontexte auflisten
  rename      Kontext umbenennen
  use         Kontext aktivieren

Optionen:
  -h, --help   Hilfe zu context

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

Mit "papierkram context [Befehl] --help" gibt es weitere Informationen zu einem Befehl.
```

## Siehe auch

- [papierkram](papierkram.md) – Kommandozeile für die Papierkram-API
- [papierkram context active](papierkram_context_active.md) – Aktiven Kontext anzeigen
- [papierkram context create](papierkram_context_create.md) – Kontext anlegen
- [papierkram context delete](papierkram_context_delete.md) – Kontext löschen
- [papierkram context list](papierkram_context_list.md) – Kontexte auflisten
- [papierkram context rename](papierkram_context_rename.md) – Kontext umbenennen
- [papierkram context use](papierkram_context_use.md) – Kontext aktivieren
