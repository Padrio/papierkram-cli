# papierkram person create

Kontaktperson anlegen

```text
Kontaktperson anlegen.

Kosten: 10 Credits je Anfrage.

Ohne --company gilt PAPIERKRAM_COMPANY oder company aus der Projektdatei .papierkram.toml (anlegen: papierkram local init).

Verwendung:
  papierkram person create [Optionen]

Beispiele:
  papierkram person create --company 2843 --first-name Anna --last-name Groß --email anna@example.org
  papierkram person create --company 2843 --first-name Jörg --last-name Weiß --default

Optionen:
      --comment Text      Wert für comment
      --company ID        ID oder Name des übergeordneten Objekts (company_id); ohne Angabe PAPIERKRAM_COMPANY oder company aus der Projektdatei .papierkram.toml (Pflicht)
      --default           Wert für default (true/false)
      --department Text   Wert für department
      --email Text        Wert für email
      --fax Text          Wert für fax
      --first-name Text   Wert für first_name (Pflicht)
      --flagged           Wert für flagged (true/false)
  -h, --help              Hilfe zu create
      --input Text        Body als JSON-Datei oder - für die Standardeingabe; Flags überschreiben einzelne Felder
      --last-name Text    Wert für last_name (Pflicht)
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
      --timeout Dauer   Zeitlimit je Anfrage, pro Versuch (Uploads ohne Angabe mindestens 120 s); GET wird bis zu zweimal wiederholt (Standard: 30s)
  -y, --yes             Bestätigungen automatisch bejahen
```

## Siehe auch

- [papierkram person](papierkram_person.md) – Kontaktpersonen einer Firma verwalten
