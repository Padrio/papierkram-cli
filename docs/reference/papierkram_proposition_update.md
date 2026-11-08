# papierkram proposition update

Artikel oder Leistung ändern

```text
Artikel oder Leistung ändern.

Kosten: 1 Credit je Anfrage.

Verwendung:
  papierkram proposition update <id> [Optionen]

Beispiele:
  papierkram proposition update 386 --price 135.50
  papierkram proposition update 386 --vat-rate 7 --dry-run
  papierkram proposition update 386 --input artikel.json

Optionen:
      --article-no Text         Wert für article_no
      --description Text        Wert für description
  -h, --help                    Hilfe zu update
      --input Text              Body als JSON-Datei oder - für die Standardeingabe; Flags überschreiben einzelne Felder
      --name Text               Wert für name
      --price Text              Wert für price
      --proposition-type Text   Wert für proposition_type (erlaubt: service, product, ecommerce)
      --time-unit Text          Wert für time_unit (erlaubt: hour, day)
      --unit-name-1 Text        Wert für unit_name_1
      --unit-name-n Text        Wert für unit_name_n
      --vat-rate Text           Wert für vat_rate (19, 19%, 0,19 oder 0.19)

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

- [papierkram proposition](papierkram_proposition.md) – Artikel und Leistungen verwalten
