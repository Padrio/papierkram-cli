# papierkram estimate

Angebote verwalten

```text
Angebote verwalten

Verwendung:
  papierkram estimate [Optionen]
  papierkram estimate [Befehl]

Aliase:
  estimate, angebot

Verfügbare Befehle:
  archive     Angebot archivieren
  cancel      Angebot stornieren
  create      Angebot anlegen
  delete      Angebot löschen
  deliver     Angebot versenden (per E-Mail oder als PDF finalisieren)
  describe    Angebot anzeigen
  list        Angebote auflisten
  pdf         Angebot als PDF herunterladen
  unarchive   Angebot aus dem Archiv holen
  update      Angebot ändern

Optionen:
  -h, --help   Hilfe zu estimate

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

Mit "papierkram estimate [Befehl] --help" gibt es weitere Informationen zu einem Befehl.
```

## Siehe auch

- [papierkram](papierkram.md) – Kommandozeile für die Papierkram-API
- [papierkram estimate archive](papierkram_estimate_archive.md) – Angebot archivieren
- [papierkram estimate cancel](papierkram_estimate_cancel.md) – Angebot stornieren
- [papierkram estimate create](papierkram_estimate_create.md) – Angebot anlegen
- [papierkram estimate delete](papierkram_estimate_delete.md) – Angebot löschen
- [papierkram estimate deliver](papierkram_estimate_deliver.md) – Angebot versenden (per E-Mail oder als PDF finalisieren)
- [papierkram estimate describe](papierkram_estimate_describe.md) – Angebot anzeigen
- [papierkram estimate list](papierkram_estimate_list.md) – Angebote auflisten
- [papierkram estimate pdf](papierkram_estimate_pdf.md) – Angebot als PDF herunterladen
- [papierkram estimate unarchive](papierkram_estimate_unarchive.md) – Angebot aus dem Archiv holen
- [papierkram estimate update](papierkram_estimate_update.md) – Angebot ändern
