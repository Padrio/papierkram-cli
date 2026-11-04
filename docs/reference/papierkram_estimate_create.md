# papierkram estimate create

Angebot anlegen

```text
Angebot anlegen.

Kosten: 50 Credits je Anfrage.

Verwendung:
  papierkram estimate create [Optionen]

Beispiele:
  papierkram estimate create --name "Angebot Website" --customer 3052 --item "name=Konzeption,qty=3,unit=Tag,price=800,vat=19"
  papierkram estimate create --name Wartung --customer 3052 --items positionen.json --dry-run
  papierkram estimate create --input angebot.json

Optionen:
      --billing-city Text             Wert für billing.city
      --billing-company Text          Wert für billing.company
      --billing-contact-person Text   Wert für billing.contact_person
      --billing-country Text          Wert für billing.country
      --billing-department Text       Wert für billing.department
      --billing-email Text            Wert für billing.email
      --billing-street Text           Wert für billing.street
      --billing-ust-idnr Text         Wert für billing.ust_idnr
      --billing-zip Text              Wert für billing.zip
      --contact-person ID             ID für customer.contact_person
      --custom-template ID            ID für custom_template
      --customer ID                   ID für customer
      --customer-project ID           ID für customer.project
      --description Text              Wert für description
      --document-date Datum           Wert für document_date (YYYY-MM-DD, heute, gestern)
      --flagged                       Wert für flagged (true/false)
      --greetings-text Text           Wert für greetings_text
      --gross                         Wert für gross (true/false)
  -h, --help                          Hilfe zu create
      --inbound-address Text          Wert für inbound_address
      --input Text                    Body als JSON-Datei oder - für die Standardeingabe; Flags überschreiben einzelne Felder
      --item Texte                    Position als key=wert,key=wert (wiederholbar) (Pflicht)
      --items Text                    Positionen als JSON-Array aus Datei oder - für die Standardeingabe
      --name Text                     Wert für name (Pflicht)
      --reminder-date Datum           Wert für reminder_date (YYYY-MM-DD, heute, gestern)
      --sent-on Text                  Wert für sent_on
      --sent-to Text                  Wert für sent_to
      --thanks-text Text              Wert für thanks_text

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

- [papierkram estimate](papierkram_estimate.md) – Angebote verwalten
