# papierkram invoice update

Rechnung ändern

```text
Rechnung ändern.

Kosten: 1 Credit je Anfrage.

Ohne --payment-term und ohne payment_term in --input liest das CLI die Rechnung vorher (1 weiterer Credit) und sendet ihre aktuelle Zahlungsbedingung (payment_term.id) mit: Ein PUT ohne payment_term stellt die Rechnung sonst auf die Standard-Zahlungsbedingung des Accounts um und ändert das Fälligkeitsdatum (gemessen 2026-11-04). --dry-run liest ebenfalls und zeigt das Lesen. Scheitert das Lesen, wird nichts geändert. Mit --payment-term entfällt das Lesen. Mit --customer ohne --contact-person/--customer-project liest das CLI die Rechnung ebenfalls: beim gleichen Kunden werden Kontaktperson und Projekt mitgesendet, bei einem anderen entfallen sie (Hinweis auf stderr).

Verwendung:
  papierkram invoice update <id> [Optionen]

Beispiele:
  papierkram invoice update 459 --payment-term 1131
  papierkram invoice update 459 --item "name=Beratung,qty=3,price=100,vat=19" --replace-items
  papierkram invoice update 459 --input rechnung.json

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
      --gross                         Wert für gross (true/false)
  -h, --help                          Hilfe zu update
      --input Text                    Body als JSON-Datei oder - für die Standardeingabe; Flags überschreiben einzelne Felder
      --item Texte                    Position als key=wert,key=wert (wiederholbar)
      --items Text                    Positionen als JSON-Array aus Datei oder - für die Standardeingabe
      --name Text                     Wert für name
      --payment-term ID               ID für payment_term; spart das Lesen der Rechnung (1 Credit), außer mit --customer ohne --contact-person/--customer-project
      --replace-items                 alle Positionen ersetzen (nötig für --item/--items; ohne Positionen werden alle gelöscht)
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
