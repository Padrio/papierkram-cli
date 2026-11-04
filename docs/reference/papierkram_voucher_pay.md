# papierkram voucher pay

Zahlung zu einem Beleg erfassen

```text
Zahlung zu einem Beleg erfassen.

Kosten: 1 Credit je Anfrage.

Mindestens --amount oder --transaction ist nötig: --amount bucht eine Zahlung ohne Bankumsatz, --transaction verknüpft einen vorhandenen Bankumsatz. Weicht die Zahlung vom offenen Betrag ab, verlangt Papierkram --reason. --input liefert den Body (value, banking_transaction, payment_date, difference_reason); Flags überschreiben einzelne Felder daraus.

Eine Zahlung lässt sich per API nicht zurücknehmen: Im Terminal fragt der Befehl nach, ohne Terminal ist --yes nötig.

Verwendung:
  papierkram voucher pay <id> [Optionen]

Beispiele:
  papierkram voucher pay 295 --transaction 457
  papierkram voucher pay 295 --amount 165.80 --date 2026-11-04 --yes
  papierkram voucher pay 295 --amount 150 --reason skonto --date heute --dry-run

Optionen:
      --amount Dezimalzahl   gezahlter Betrag mit Dezimalpunkt, z. B. 165.80 (Pflicht; eines von --amount und --transaction oder --input)
      --date Datum           Zahlungsdatum (YYYY-MM-DD, heute, gestern)
  -h, --help                 Hilfe zu pay
      --input Text           Body als JSON-Datei oder - für die Standardeingabe; Flags überschreiben einzelne Felder
      --reason Text          Grund, wenn der Betrag vom offenen Betrag abweicht (erlaubt: sonstige, mahnung, teilzahlung, skonto, schmaelerung)
      --transaction ID       ID des Bankumsatzes, mit dem der Beleg bezahlt wurde (papierkram transaction list) (Pflicht; eines von --amount und --transaction oder --input)

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

- [papierkram voucher](papierkram_voucher.md) – Belege (Ausgaben) verwalten
