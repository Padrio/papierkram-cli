# papierkram voucher list

Belege auflisten

```text
Belege auflisten.

Kosten: 1 Credit je Anfrage (mit --all je geladener Seite; ab 10 Seiten fragt --all im Terminal nach).

Verwendung:
  papierkram voucher list [Optionen]

Beispiele:
  papierkram voucher list
  papierkram voucher list --creditor 2904 --from 2026-01-01 --to 2026-03-31
  papierkram voucher list --project 4316 --all -o json

Optionen:
      --all              alle Seiten laden und zusammenführen (1 Anfrage je Seite)
      --creditor Zahl    Nur Belege dieses Lieferanten (Firmen-ID)
      --desc             absteigend sortieren (nur mit --sort)
      --from Text        Nur Belege mit Belegdatum ab diesem Tag (YYYY-MM-DD, heute, gestern)
  -h, --help             Hilfe zu list
      --limit Zahl       Einträge pro Seite, 1–100 (--all ohne --limit: 100) (Standard: 25)
      --max-pages Zahl   mit --all höchstens so viele Seiten laden
      --page Zahl        nur diese Seite laden (ab 1)
      --project Zahl     Nur Belege dieses Projekts (ID)
      --sort Text        nach diesem Feld sortieren (setzt order_by und order_direction)
      --to Text          Nur Belege mit Belegdatum bis zu diesem Tag (YYYY-MM-DD, heute, gestern)

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

- [papierkram voucher](papierkram_voucher.md) – Belege (Ausgaben) verwalten
