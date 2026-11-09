# papierkram invoice list

Rechnungen auflisten

```text
Rechnungen auflisten.

Kosten: 1 Credit je Anfrage (mit --all je geladener Seite; ab 10 Seiten fragt --all im Terminal nach).

Verwendung:
  papierkram invoice list [Optionen]

Beispiele:
  papierkram invoice list
  papierkram invoice list --company 3214 --from 2026-01-01 --to 2026-03-31
  papierkram invoice list --project 4576 --all -o json

Optionen:
      --all              alle Seiten laden und zusammenführen (1 Anfrage je Seite)
      --company ID       Nur Rechnungen dieser Firma (ID oder Name)
      --desc             absteigend sortieren (nur mit --sort)
      --from Text        Nur Rechnungen mit Belegdatum ab diesem Tag (YYYY-MM-DD, heute, gestern)
  -h, --help             Hilfe zu list
      --limit Zahl       Einträge pro Seite, 1–100 (--all ohne --limit: 100) (Standard: 25)
      --max-pages Zahl   mit --all höchstens so viele Seiten laden
      --page Zahl        nur diese Seite laden (ab 1)
      --project ID       Nur Rechnungen dieses Projekts (ID oder Name)
      --sort Text        nach diesem Feld sortieren (setzt order_by und order_direction)
      --to Text          Nur Rechnungen mit Belegdatum bis zu diesem Tag (YYYY-MM-DD, heute, gestern)

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

- [papierkram invoice](papierkram_invoice.md) – Rechnungen verwalten
