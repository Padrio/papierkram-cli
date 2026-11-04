# papierkram custom-attribute create

Benutzerdefiniertes Datenfeld anlegen

```text
Benutzerdefiniertes Datenfeld anlegen.

Kosten: 10 Credits je Anfrage.

Verwendung:
  papierkram custom-attribute create [Optionen]

Beispiele:
  papierkram custom-attribute create --name Kostenstelle --attributable-type Project --data-type string
  papierkram custom-attribute create --name "Rabatt in %" --attributable-type Contact::Company --data-type decimal --dry-run

Optionen:
      --attributable-type Text   Wert für attributable_type (erlaubt: Contact::Company, Project) (Pflicht)
      --data-type Text           Wert für data_type (erlaubt: string, text, date, integer, decimal, boolean) (Pflicht)
  -h, --help                     Hilfe zu create
      --input Text               Body als JSON-Datei oder - für die Standardeingabe; Flags überschreiben einzelne Felder
      --name Text                Wert für name (Pflicht)

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

- [papierkram custom-attribute](papierkram_custom-attribute.md) – Benutzerdefinierte Datenfelder verwalten
