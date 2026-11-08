# papierkram estimate list

Angebote auflisten

```text
Angebote auflisten.

Kosten: 1 Credit je Anfrage (mit --all je geladener Seite; ab 10 Seiten fragt --all im Terminal nach).

Verwendung:
  papierkram estimate list [Optionen]

Beispiele:
  papierkram estimate list
  papierkram estimate list --company 3052 --from 2026-01-01 --to 2026-12-31
  papierkram estimate list --project 4316 --all -o json

Optionen:
      --all              alle Seiten laden und zusammenführen (1 Anfrage je Seite)
      --company Zahl     Nur Angebote dieser Firma (ID)
      --desc             absteigend sortieren (nur mit --sort)
      --from Text        Nur Angebote mit Belegdatum ab diesem Tag (YYYY-MM-DD, heute, gestern)
  -h, --help             Hilfe zu list
      --limit Zahl       Einträge pro Seite, 1–100 (--all ohne --limit: 100) (Standard: 25)
      --max-pages Zahl   mit --all höchstens so viele Seiten laden
      --page Zahl        nur diese Seite laden (ab 1)
      --project Zahl     Nur Angebote dieses Projekts (ID)
      --sort Text        nach diesem Feld sortieren (setzt order_by und order_direction)
      --to Text          Nur Angebote mit Belegdatum bis zu diesem Tag (YYYY-MM-DD, heute, gestern)

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

- [papierkram estimate](papierkram_estimate.md) – Angebote verwalten
