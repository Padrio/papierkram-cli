# papierkram person update

Kontaktperson ändern

```text
Kontaktperson ändern.

Kosten: 1 Credit je Anfrage.

Verwendung:
  papierkram person update <id> [Optionen]

Beispiele:
  papierkram person update 1063 --company 2862 --position Einkauf
  papierkram person update 1063 --company 2862 --flagged=false

Optionen:
      --comment Text      Wert für comment
      --company Zahl      ID des übergeordneten Objekts (company_id, Pflicht)
      --default           Wert für default (true/false)
      --department Text   Wert für department
      --email Text        Wert für email
      --fax Text          Wert für fax
      --first-name Text   Wert für first_name
      --flagged           Wert für flagged (true/false)
  -h, --help              Hilfe zu update
      --input Text        Body als JSON-Datei oder - für die Standardeingabe; Flags überschreiben einzelne Felder
      --last-name Text    Wert für last_name
      --mobile Text       Wert für mobile
      --phone Text        Wert für phone
      --position Text     Wert für position
      --salutation Text   Wert für salutation
      --skype Text        Wert für skype
      --title Text        Wert für title

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

- [papierkram person](papierkram_person.md) – Kontaktpersonen einer Firma verwalten
