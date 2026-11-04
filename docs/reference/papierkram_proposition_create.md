# papierkram proposition create

Artikel oder Leistung anlegen

```text
Artikel oder Leistung anlegen.

Kosten: 10 Credits je Anfrage.

Verwendung:
  papierkram proposition create [Optionen]

Beispiele:
  papierkram proposition create --name Beratung --article-no AN-00010 --proposition-type service --time-unit hour --price 120 --vat-rate 19
  papierkram proposition create --name "Schulung vor Ort" --article-no AN-00011 --proposition-type service --time-unit day --price 950 --vat-rate 19% --dry-run
  papierkram proposition create --input artikel.json

Optionen:
      --article-no Text         Wert für article_no (Pflicht)
      --description Text        Wert für description
  -h, --help                    Hilfe zu create
      --input Text              Body als JSON-Datei oder - für die Standardeingabe; Flags überschreiben einzelne Felder
      --name Text               Wert für name (Pflicht)
      --price Text              Wert für price
      --proposition-type Text   Wert für proposition_type (erlaubt: service, product, ecommerce) (Pflicht)
      --time-unit Text          Wert für time_unit (erlaubt: hour, day) (Pflicht)
      --unit-name-1 Text        Wert für unit_name_1
      --unit-name-n Text        Wert für unit_name_n
      --vat-rate Text           Wert für vat_rate (19, 19%, 0,19 oder 0.19) (Pflicht)

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

- [papierkram proposition](papierkram_proposition.md) – Artikel und Leistungen verwalten
