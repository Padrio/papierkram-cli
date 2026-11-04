# papierkram invoice

Rechnungen verwalten

```text
Rechnungen verwalten

Verwendung:
  papierkram invoice [Optionen]
  papierkram invoice [Befehl]

Aliase:
  invoice, rechnung

Verfügbare Befehle:
  archive     Rechnung archivieren
  cancel      Rechnung stornieren
  create      Rechnung anlegen
  delete      Rechnung löschen
  deliver     Rechnung versenden (per E-Mail oder als PDF finalisieren)
  describe    Rechnung anzeigen
  list        Rechnungen auflisten
  pdf         Rechnung als PDF herunterladen
  unarchive   Rechnung aus dem Archiv holen
  update      Rechnung ändern

Optionen:
  -h, --help   Hilfe zu invoice

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

Mit "papierkram invoice [Befehl] --help" gibt es weitere Informationen zu einem Befehl.
```

## Siehe auch

- [papierkram](papierkram.md) – Kommandozeile für die Papierkram-API
- [papierkram invoice archive](papierkram_invoice_archive.md) – Rechnung archivieren
- [papierkram invoice cancel](papierkram_invoice_cancel.md) – Rechnung stornieren
- [papierkram invoice create](papierkram_invoice_create.md) – Rechnung anlegen
- [papierkram invoice delete](papierkram_invoice_delete.md) – Rechnung löschen
- [papierkram invoice deliver](papierkram_invoice_deliver.md) – Rechnung versenden (per E-Mail oder als PDF finalisieren)
- [papierkram invoice describe](papierkram_invoice_describe.md) – Rechnung anzeigen
- [papierkram invoice list](papierkram_invoice_list.md) – Rechnungen auflisten
- [papierkram invoice pdf](papierkram_invoice_pdf.md) – Rechnung als PDF herunterladen
- [papierkram invoice unarchive](papierkram_invoice_unarchive.md) – Rechnung aus dem Archiv holen
- [papierkram invoice update](papierkram_invoice_update.md) – Rechnung ändern
