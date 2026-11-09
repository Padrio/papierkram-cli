# papierkram voucher update

Beleg ändern

```text
Beleg ändern.

Kosten: 1 Credit je Anfrage.

Verwendung:
  papierkram voucher update <id> [Optionen]

Beispiele:
  papierkram voucher update 295 --due-date 2026-12-15
  papierkram voucher update 295 --item "name=Papier,amount=23.80,vat=19,category=Bürobedarf" --replace-items
  papierkram voucher update 295 --input beleg.json

Optionen:
      --creditor ID                  ID oder Name für creditor
      --description Text             Wert für description
      --document-date Datum          Wert für document_date (YYYY-MM-DD, heute, gestern)
      --due-date Datum               Wert für due_date (YYYY-MM-DD, heute, gestern)
      --entertainment-person Texte   Wert für entertainment_persons (wiederholbar)
      --entertainment-reason Text    Wert für entertainment_reason
      --flagged                      Wert für flagged (true/false)
  -h, --help                         Hilfe zu update
      --input Text                   Body als JSON-Datei oder - für die Standardeingabe; Flags überschreiben einzelne Felder
      --item Texte                   Position als key=wert,key=wert (wiederholbar)
      --items Text                   Positionen als JSON-Array aus Datei oder - für die Standardeingabe
      --name Text                    Wert für name
      --provenance Text              Wert für provenance (erlaubt: domestic, eu, foreign)
      --replace-items                alle Positionen ersetzen (nötig für --item/--items; ohne Positionen werden alle gelöscht)

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
