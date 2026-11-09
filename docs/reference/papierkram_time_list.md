# papierkram time list

Zeiteinträge auflisten

```text
Zeiteinträge auflisten.

Kosten: 1 Credit je Anfrage (mit --all je geladener Seite; ab 10 Seiten fragt --all im Terminal nach).

Ohne --project gilt PAPIERKRAM_PROJECT oder project aus der Projektdatei .papierkram.toml (anlegen: papierkram local init).

Verwendung:
  papierkram time list [Optionen]

Beispiele:
  papierkram time list --since 2026-10-01 --until 2026-10-31
  papierkram time list --project 4963 --state unbilled -o wide
  papierkram time list --user 1 --since heute --all -o json

Optionen:
      --all              alle Seiten laden und zusammenführen (1 Anfrage je Seite)
      --desc             absteigend sortieren (nur mit --sort)
  -h, --help             Hilfe zu list
      --invoice ID       Nur Zeiteinträge dieser Rechnung (ID)
      --limit Zahl       Einträge pro Seite, 1–100 (--all ohne --limit: 100) (Standard: 25)
      --max-pages Zahl   mit --all höchstens so viele Seiten laden
      --page Zahl        nur diese Seite laden (ab 1)
      --project ID       Nur Zeiteinträge dieses Projekts (ID oder Name); ohne Angabe PAPIERKRAM_PROJECT oder project aus der Projektdatei .papierkram.toml
      --since Text       Nur Zeiteinträge mit Beginn ab diesem Zeitpunkt (RFC 3339; YYYY-MM-DD, heute, gestern: ab 00:00:00 Uhr lokaler Zeit)
      --sort Text        nach diesem Feld sortieren (setzt order_by und order_direction)
      --state Text       Nur Zeiteinträge mit diesem Abrechnungsstatus (erlaubt: billed, unbilled, billable, unbillable, archived)
      --task ID          Nur Zeiteinträge dieser Aufgabe (ID oder Name)
      --until Text       Nur Zeiteinträge mit Beginn bis zu diesem Zeitpunkt (RFC 3339; YYYY-MM-DD, heute, gestern: bis 23:59:59 Uhr lokaler Zeit)
      --user ID          Nur Zeiteinträge dieses Benutzers (ID oder Name)

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

- [papierkram time](papierkram_time.md) – Zeiteinträge erfassen und verwalten
