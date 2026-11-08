# papierkram invoice create

Rechnung anlegen

```text
Rechnung anlegen.

Kosten: 50 Credits je Anfrage.

--from-estimate <id> liest das Angebot (1 weiterer Credit) und übernimmt daraus name, description, gross, billing, customer (mit contact_person und project des Angebots) und alle Positionen (name, description, quantity, unit, price, price_gross, discount_percentage, discount_calculated, discount_calculated_gross, proposition, type_of, vat_rate; "19%" wird zu 0.19). --payment-term ist Pflicht, weil Angebote keine Zahlungsbedingung haben.

Alle anderen Flags überschreiben die Werte des Angebots; --item/--items ersetzen alle Positionen. Nennt --customer einen anderen Kunden als das Angebot, werden contact_person und project des Angebots nicht übernommen (Hinweis auf stderr); sie lassen sich per --contact-person bzw. --customer-project setzen. --input ist zusammen mit --from-estimate nicht möglich.

Nicht übernommen werden: document_date (Rechnungsdatum per --document-date), custom_template (Angebotsvorlagen gelten nicht für Rechnungen; --custom-template), document_charges und je Position document_charge, document_charge_gross und position_in_group. supply_date, flagged und start_new_item_group hat das Angebot nicht. Leere Werte (null) des Angebots werden übernommen, wo die Rechnung sie erlaubt, sonst weggelassen und auf stderr genannt, außer ein Flag setzt das Feld.

--dry-run liest das Angebot ebenfalls (1 Credit) und zeigt den Body, ohne die Rechnung anzulegen. Scheitert das Lesen, wird nichts angelegt.

Verwendung:
  papierkram invoice create [Optionen]

Beispiele:
  papierkram invoice create --name "Beratung Oktober" --customer 3214 --payment-term 1130 --item "name=Beratung,qty=2,unit=Stunde,price=100,vat=19"
  papierkram invoice create --name Wartung --customer 3214 --payment-term 1130 --items positionen.json --dry-run
  papierkram invoice create --input rechnung.json
  papierkram invoice create --from-estimate 349 --payment-term 1130 --dry-run
  papierkram invoice create --from-estimate 349 --payment-term 1130 --name "Website Relaunch" --document-date heute

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
      --from-estimate ID              Angebot, aus dem die Rechnung entsteht (liest es: 1 weiterer Credit)
      --gross                         Wert für gross (true/false)
  -h, --help                          Hilfe zu create
      --input Text                    Body als JSON-Datei oder - für die Standardeingabe; Flags überschreiben einzelne Felder
      --item Texte                    Position als key=wert,key=wert (wiederholbar) (Pflicht)
      --items Text                    Positionen als JSON-Array aus Datei oder - für die Standardeingabe
      --name Text                     Wert für name (Pflicht)
      --payment-term ID               ID für payment_term (Pflicht)
      --supply-date Text              Wert für supply_date

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

- [papierkram invoice](papierkram_invoice.md) – Rechnungen verwalten
