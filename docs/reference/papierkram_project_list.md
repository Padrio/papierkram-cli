# papierkram project list

Projekte auflisten

```text
Projekte auflisten.

Kosten: 1 Credit je Anfrage (mit --all je geladener Seite; ab 10 Seiten fragt --all im Terminal nach).

Verwendung:
  papierkram project list [Optionen]

Beispiele:
  papierkram project list
  papierkram project list --company 3283
  papierkram project list --all -o json

Optionen:
      --all              alle Seiten laden und zusammenführen (1 Anfrage je Seite)
      --company ID       Nur Projekte dieses Kunden (Firmen-ID oder Name)
      --desc             absteigend sortieren (nur mit --sort)
  -h, --help             Hilfe zu list
      --limit Zahl       Einträge pro Seite, 1–100 (--all ohne --limit: 100) (Standard: 25)
      --max-pages Zahl   mit --all höchstens so viele Seiten laden
      --page Zahl        nur diese Seite laden (ab 1)
      --sort Text        nach diesem Feld sortieren (setzt order_by und order_direction)

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

- [papierkram project](papierkram_project.md) – Projekte verwalten
