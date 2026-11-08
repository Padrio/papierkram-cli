# papierkram task create

Aufgabe anlegen

```text
Aufgabe anlegen.

Kosten: 10 Credits je Anfrage.

Ohne --project gilt PAPIERKRAM_PROJECT oder project aus der Projektdatei .papierkram.toml (anlegen: papierkram local init).

Verwendung:
  papierkram task create [Optionen]

Beispiele:
  papierkram task create --name "Konzept erstellen" --project 4823 --deadline 2026-12-15
  papierkram task create --name Support --project 4823 --proposition 386 --flagged --dry-run
  papierkram task create --input aufgabe.json

Optionen:
      --complete              Wert für complete (true/false)
      --deadline Datum        Wert für deadline (YYYY-MM-DD, heute, gestern)
      --flagged               Wert für flagged (true/false)
  -h, --help                  Hilfe zu create
      --input Text            Body als JSON-Datei oder - für die Standardeingabe; Flags überschreiben einzelne Felder
      --name Text             Wert für name (Pflicht)
      --project ID            ID für project; ohne Angabe PAPIERKRAM_PROJECT oder project aus der Projektdatei .papierkram.toml (Pflicht)
      --proposition ID        ID für proposition
      --relative-costs Text   Wert für relative_costs
      --user ID               ID für user

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

- [papierkram task](papierkram_task.md) – Aufgaben verwalten
