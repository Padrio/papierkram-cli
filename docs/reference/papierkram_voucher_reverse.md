# papierkram voucher reverse

Beleg mit Gegenbuchung stornieren (Generalumkehr)

```text
Beleg mit Gegenbuchung stornieren (Generalumkehr).

Kosten: 1 Credit je Anfrage.

Verwendung:
  papierkram voucher reverse <id> [Optionen]

Beispiele:
  papierkram voucher reverse 295
  papierkram voucher reverse 295 --yes -o json

Optionen:
  -h, --help   Hilfe zu reverse

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
```

## Siehe auch

- [papierkram voucher](papierkram_voucher.md) – Belege (Ausgaben) verwalten
