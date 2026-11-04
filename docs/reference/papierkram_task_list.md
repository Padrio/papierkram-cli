# papierkram task list

Aufgaben auflisten

```text
Aufgaben auflisten.

Kosten: 1 Credit je Anfrage (mit --all je geladener Seite; ab 10 Seiten fragt --all im Terminal nach).

Ohne --project gilt PAPIERKRAM_PROJECT oder project aus der Projektdatei .papierkram.toml (anlegen: papierkram local init).

Verwendung:
  papierkram task list [Optionen]

Beispiele:
  papierkram task list
  papierkram task list --project 4823
  papierkram task list --proposition 386 --all -o json

Optionen:
      --all                alle Seiten laden und zusammenführen (1 Anfrage je Seite)
      --desc               absteigend sortieren (nur mit --sort)
  -h, --help               Hilfe zu list
      --limit Zahl         Einträge pro Seite, 1–100 (--all ohne --limit: 100) (Standard: 25)
      --max-pages Zahl     mit --all höchstens so viele Seiten laden
      --page Zahl          nur diese Seite laden (ab 1)
      --project Zahl       Nur Aufgaben dieses Projekts (ID); ohne Angabe PAPIERKRAM_PROJECT oder project aus der Projektdatei .papierkram.toml
      --proposition Zahl   Nur Aufgaben mit diesem Artikel (ID)
      --sort Text          nach diesem Feld sortieren (setzt order_by und order_direction)

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

- [papierkram task](papierkram_task.md) – Aufgaben verwalten
