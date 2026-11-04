# papierkram voucher document

Dateien (Belegbilder) eines Belegs verwalten

```text
Dateien (Belegbilder) eines Belegs verwalten

Verwendung:
  papierkram voucher document [Optionen]
  papierkram voucher document [Befehl]

Verfügbare Befehle:
  delete      Datei eines Belegs löschen

Optionen:
  -h, --help   Hilfe zu document

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

Mit "papierkram voucher document [Befehl] --help" gibt es weitere Informationen zu einem Befehl.
```

## Siehe auch

- [papierkram voucher](papierkram_voucher.md) – Belege (Ausgaben) verwalten
- [papierkram voucher document delete](papierkram_voucher_document_delete.md) – Datei eines Belegs löschen
