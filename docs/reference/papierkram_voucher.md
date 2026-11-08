# papierkram voucher

Belege (Ausgaben) verwalten

```text
Belege (Ausgaben) verwalten

Verwendung:
  papierkram voucher [Optionen]
  papierkram voucher [Befehl]

Aliase:
  voucher, beleg, expense

Verfügbare Befehle:
  archive     Beleg archivieren
  cancel      Beleg stornieren
  create      Beleg anlegen
  delete      Beleg löschen
  describe    Beleg anzeigen
  document    Dateien (Belegbilder) eines Belegs verwalten
  list        Belege auflisten
  pay         Zahlung zu einem Beleg erfassen
  pdf         Beleg als PDF herunterladen
  reverse     Beleg mit Gegenbuchung stornieren (Generalumkehr)
  unarchive   Beleg aus dem Archiv holen
  update      Beleg ändern
  upload      Datei (Belegbild) an einen Beleg anhängen

Optionen:
  -h, --help   Hilfe zu voucher

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

Mit "papierkram voucher [Befehl] --help" gibt es weitere Informationen zu einem Befehl.
```

## Siehe auch

- [papierkram](papierkram.md) – Kommandozeile für die Papierkram-API
- [papierkram voucher archive](papierkram_voucher_archive.md) – Beleg archivieren
- [papierkram voucher cancel](papierkram_voucher_cancel.md) – Beleg stornieren
- [papierkram voucher create](papierkram_voucher_create.md) – Beleg anlegen
- [papierkram voucher delete](papierkram_voucher_delete.md) – Beleg löschen
- [papierkram voucher describe](papierkram_voucher_describe.md) – Beleg anzeigen
- [papierkram voucher document](papierkram_voucher_document.md) – Dateien (Belegbilder) eines Belegs verwalten
- [papierkram voucher list](papierkram_voucher_list.md) – Belege auflisten
- [papierkram voucher pay](papierkram_voucher_pay.md) – Zahlung zu einem Beleg erfassen
- [papierkram voucher pdf](papierkram_voucher_pdf.md) – Beleg als PDF herunterladen
- [papierkram voucher reverse](papierkram_voucher_reverse.md) – Beleg mit Gegenbuchung stornieren (Generalumkehr)
- [papierkram voucher unarchive](papierkram_voucher_unarchive.md) – Beleg aus dem Archiv holen
- [papierkram voucher update](papierkram_voucher_update.md) – Beleg ändern
- [papierkram voucher upload](papierkram_voucher_upload.md) – Datei (Belegbild) an einen Beleg anhängen
