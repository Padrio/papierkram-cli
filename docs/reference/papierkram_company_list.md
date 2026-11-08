# papierkram company list

Firmen auflisten

```text
Firmen auflisten.

Kosten: 1 Credit je Anfrage (mit --all je geladener Seite; ab 10 Seiten fragt --all im Terminal nach).

Die API ignoriert diesen Filter: Mit --type lädt das CLI alle Seiten wie --all (je Seite 1 Credit, Warnung ab 10 Seiten, im Terminal mit Rückfrage; --max-pages begrenzt) und filtert lokal. --page und --limit gelten für das gefilterte Ergebnis: page, page_size, total_entries, total_pages und has_more beziehen sich auf die gefilterten Einträge; mit --all kommen alle gefilterten Einträge. Mit --max-pages zählen total_entries und total_pages nur die Treffer der geladenen Seiten; has_more ist dann true, weil weitere Seiten Treffer enthalten können.

Verwendung:
  papierkram company list [Optionen]

Beispiele:
  papierkram company list
  papierkram company list --type customer
  papierkram company list --type supplier --limit 10 --page 2 -o json

Optionen:
      --all              alle Seiten laden und zusammenführen (1 Anfrage je Seite)
      --desc             absteigend sortieren (nur mit --sort)
  -h, --help             Hilfe zu list
      --limit Zahl       Einträge pro Seite, 1–100 (--all ohne --limit: 100); mit --type: Seitengröße des gefilterten Ergebnisses (Standard: 25)
      --max-pages Zahl   mit --all oder --type höchstens so viele Seiten laden
      --page Zahl        nur diese Seite laden (ab 1); mit --type: diese Seite des gefilterten Ergebnisses ausgeben
      --sort Text        nach diesem Feld sortieren (setzt order_by und order_direction)
      --type Text        nur Kunden (customer) oder Lieferanten (supplier); die API filtert nicht: lädt alle Seiten (1 Credit je Seite) und filtert lokal

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

- [papierkram company](papierkram_company.md) – Firmen (Kunden und Lieferanten) verwalten
