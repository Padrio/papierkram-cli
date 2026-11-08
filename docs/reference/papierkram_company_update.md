# papierkram company update

Firma ändern

```text
Firma ändern.

Kosten: 1 Credit je Anfrage.

Verwendung:
  papierkram company update <id> [Optionen]

Beispiele:
  papierkram company update 2843 --email buchhaltung@example.org
  papierkram company update 2843 --input firma.json

Optionen:
      --attr Texte                      eigenes Feld als <id>=<wert> (wiederholbar)
      --bank-account-holder-name Text   Wert für bank_account_holder_name
      --bank-account-no Text            Wert für bank_account_no
      --bank-bic Text                   Wert für bank_bic
      --bank-blz Text                   Wert für bank_blz
      --bank-iban Text                  Wert für bank_iban
      --bank-institute Text             Wert für bank_institute
      --color Text                      Wert für color
      --contact-type Text               Wert für contact_type (erlaubt: customer, supplier)
      --delivery-method Text            Wert für delivery_method (erlaubt: pdf, email)
      --email Text                      Wert für email
      --fax Text                        Wert für fax
  -h, --help                            Hilfe zu update
      --input Text                      Body als JSON-Datei oder - für die Standardeingabe; Flags überschreiben einzelne Felder
      --name Text                       Wert für name
      --notes Text                      Wert für notes
      --phone Text                      Wert für phone
      --physical-city Text              Wert für physical_city
      --physical-country Text           Wert für physical_country
      --physical-street Text            Wert für physical_street
      --physical-zip Text               Wert für physical_zip
      --postal-city Text                Wert für postal_city
      --postal-country Text             Wert für postal_country
      --postal-street Text              Wert für postal_street
      --postal-zip Text                 Wert für postal_zip
      --twitter Text                    Wert für twitter
      --ust-idnr Text                   Wert für ust_idnr
      --website Text                    Wert für website

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
