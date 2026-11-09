# papierkram voucher create

Beleg anlegen

```text
Beleg anlegen.

Kosten: 50 Credits je Anfrage.

--file datei lädt die Datei nach dem Anlegen an den neuen Beleg hoch (10 weitere Credits; --dry-run zeigt beide Schritte und die Summe). Die Datei wird vor jeder Anfrage geprüft: fehlt sie, ist sie leer oder nicht lesbar, wird nichts angelegt. Scheitert nur das Hochladen, bleibt der Beleg bestehen: Exit 11 mit Beleg-ID und dem Befehl zum Nachholen (papierkram voucher upload mit Beleg-ID und absolutem Dateipfad) – dann NICHT den ganzen Befehl wiederholen, sonst entsteht ein zweiter Beleg.

Verwendung:
  papierkram voucher create [Optionen]

Beispiele:
  papierkram voucher create --name "Büromaterial Oktober" --provenance domestic --creditor 2904 --item "name=Papier,amount=11.90,vat=19,category=Bürobedarf"
  papierkram voucher create --name "Geschäftsessen" --provenance domestic --entertainment-reason "Projektbesprechung" --entertainment-person "Anna Muster" --item "name=Restaurant,amount=150.80,vat=19,category=Bewirtungskosten" --dry-run
  papierkram voucher create --input beleg.json

Optionen:
      --creditor ID                  ID oder Name für creditor
      --description Text             Wert für description
      --document-date Datum          Wert für document_date (YYYY-MM-DD, heute, gestern)
      --due-date Datum               Wert für due_date (YYYY-MM-DD, heute, gestern)
      --entertainment-person Texte   Wert für entertainment_persons (wiederholbar)
      --entertainment-reason Text    Wert für entertainment_reason
      --file Text                    Datei (Belegbild), die nach dem Anlegen an den Beleg hochgeladen wird (10 weitere Credits)
      --flagged                      Wert für flagged (true/false)
  -h, --help                         Hilfe zu create
      --input Text                   Body als JSON-Datei oder - für die Standardeingabe; Flags überschreiben einzelne Felder
      --item Texte                   Position als key=wert,key=wert (wiederholbar) (Pflicht)
      --items Text                   Positionen als JSON-Array aus Datei oder - für die Standardeingabe
      --name Text                    Wert für name (Pflicht)
      --provenance Text              Wert für provenance (erlaubt: domestic, eu, foreign) (Pflicht)

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

- [papierkram voucher](papierkram_voucher.md) – Belege (Ausgaben) verwalten
