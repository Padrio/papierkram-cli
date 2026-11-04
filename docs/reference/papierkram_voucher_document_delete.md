# papierkram voucher document delete

Datei eines Belegs löschen

```text
Datei eines Belegs löschen.

Kosten: 1 Credit je Anfrage.

Löscht die Datei <datei-id> des Belegs <beleg-id>; die IDs stehen in voucher describe <beleg-id> unter documents. Im Terminal wird zur Bestätigung die Datei-ID abgefragt, ohne Terminal ist --yes nötig.

Verwendung:
  papierkram voucher document delete <beleg-id> <datei-id> [Optionen]

Beispiele:
  papierkram voucher document delete 295 32
  papierkram voucher document delete 295 32 --yes

Optionen:
  -h, --help   Hilfe zu delete

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

- [papierkram voucher document](papierkram_voucher_document.md) – Dateien (Belegbilder) eines Belegs verwalten
