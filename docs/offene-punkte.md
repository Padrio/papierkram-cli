# Offene Punkte der Papierkram-API

Die öffentliche API-Spezifikation lässt einige Fragen offen oder widerspricht dem Live-Verhalten. Diese Liste nennt je Punkt, **was offen ist**, **was das CLI derzeit annimmt** und **welcher Test der Live-E2E-Suite** (`e2e/`, Build-Tag `e2e`, nur gegen einen Sandbox-Account, siehe [e2e/README.md](../e2e/README.md)) ihn misst. Grundlage sind Abschnitt 17 der [Spezifikation](specs/2026-11-03-papierkram-cli-design.md) und die Funde aus den Reviews.

Die Suite unterscheidet zwei Arten von Prüfungen:

- **hart:** Das CLI verlässt sich auf das Verhalten; weicht die API ab, schlägt der Test fehl.
- **Messung:** Das Ergebnis steht im Log mit dem Präfix `MESSUNG`; weicht es von der Annahme des CLI ab, zusätzlich mit `ABWEICHUNG:` und in der Zusammenfassung am Ende des Laufs. Eine Messung schlägt nicht fehl.

Testnamen sind die Namen für `go test -run`, Subtests mit `/` (z. B. `go test -tags e2e -run 'TestE2E_Carries/01_DeadlineISO' ./e2e/...`). Stand: Die Suite ist gebaut, ein Sandbox-Account existiert noch nicht; alle Punkte sind deshalb noch **nicht live beantwortet**.

## Übersicht

| Nr. | Punkt | Annahme des CLI | E2E-Test | Art |
|---|---|---|---|---|
| 1 | Statuswerte (`state`) der Belege | `state` wird roh angezeigt, nicht geprüft | `TestE2E_VoucherStateValues` | Messung |
| 2 | Erlaubte Upload-MIME-Typen | keine lokale Einschränkung | `TestE2E_UploadMimeTypes` | PDF hart, sonst Messung |
| 3 | Maximale Upload-Dateigröße | keine lokale Grenze | – | **bewusst nicht gemessen** |
| 4 | `0.19` ergibt 19 % (Position und Artikel) | `0.19` als Zahl bzw. beim Artikel als String `"0.19"` | `TestE2E_VatRate019Is19Percent/Position`, `TestE2E_VatRate019Is19Percent/Artikel` | hart |
| 5 | Kosten von `GET /info` | 1 Credit | `TestE2E_InfoCreditCost` | Messung |
| 6 | `PUT` ist ein Teil-Update | nur angegebene Felder werden gesendet | `TestE2E_PartialUpdateSemantics` (ein Subtest je Ressource, siehe unten) | hart |
| 7 | `line_items` im `PUT` ersetzen alle Positionen | ja; deshalb `--replace-items` | `TestE2E_ReplaceLineItems` | hart |
| 8 | `{id}`-Pfadparameter in der Spec nicht deklariert | im Overlay ergänzt | – | erledigt (kein Live-Punkt) |
| 9 | `deadline` als ISO-Datum | `YYYY-MM-DD` | `TestE2E_Carries/01_DeadlineISO` | hart |
| 10 | `due_date` per `UpdateInvoice` nicht änderbar | Feld wird aus `--input` entfernt | – | nicht gemessen |
| 11 | `sent_on`/`sent_to` als `null` im Angebots-`PUT` | wird akzeptiert | `TestE2E_Carries/03_EstimateNullSentFields` | hart |
| 12 | `entertainment_persons: []` im Beleg-`PUT` | wird akzeptiert | `TestE2E_Carries/04_VoucherEmptyEntertainmentPersons` | hart (Bedeutung: Messung) |
| 13 | `supply_date` im Antwortformat und `customer` als `{id}` im Rundlauf | wird akzeptiert, Kontaktperson und Projekt bleiben | `TestE2E_Carries/02_InvoiceRoundTrip` | hart |
| 14 | Werte von `due_time_unit` | meist `day`; die Tabelle zeigt `ZIEL` und `EINHEIT` getrennt | `TestE2E_Carries/05_PaymentTermUnits` | Messung |
| 15 | Einheit von `billable_duration` | Sekunden | `TestE2E_Carries/06_BillableDurationSeconds` | hart |
| 16 | Setzt `deliver --via pdf` `sent_on`? | ja (für das Nachlesen nach Abbruch) | `TestE2E_Carries/07_DeliverPdfSentOn` | Messung |
| 17 | `Accept` beim PDF-Download | `application/pdf, application/json;q=0.9` ergibt 200 `application/pdf` | `TestE2E_Carries/08_PDFDownloadAccept` | hart |
| 18 | `price_gross: 0.0` bei `gross: false` (`--from-estimate`) | wird akzeptiert | `TestE2E_Carries/09_FromEstimatePriceGrossZero` | hart |
| 19 | `voucher pay`: `value: null` mit Bankumsatz | wird akzeptiert | `TestE2E_Carries/10_VoucherPayValueNull` | Messung |
| 20 | `voucher pay`: Betrag und Bankumsatz zusammen | beide werden gesendet | – | **bewusst nicht gemessen** |
| 21 | Dauer an Tagen der Zeitumstellung | Wanduhr-Differenz | `TestE2E_Carries/11_DurationWallClock` (nur normaler Tag) | hart; Umstellungstag **bewusst nicht gemessen** |
| 22 | Ende um Mitternacht (`00:00`, `24:00`) | vom CLI abgelehnt (Exit 2) | `TestE2E_Carries/12_MidnightEnd` | Messung |
| 23 | Antwort von `GET /info` ohne Schema | JSON wird unverändert ausgegeben | `TestE2E_Carries/13_InfoStructure` | Messung |
| 24 | `deliver` antwortet mit leerem Body | leerer 2xx-Body ist Erfolg | `TestE2E_DeliverPdfEmptyBody` | hart (2xx, Exit 0); Body: Messung |
| 25 | Quota-Header `X-Remaining-Quota`, `X-Consumed-Quota` | vorhanden und ganzzahlig | `TestE2E_QuotaHeadersPresent` | hart |
| 26 | Person `default` als String | `"true"`/`"false"` | `TestE2E_PersonDefaultIsString` | hart |
| 27 | **Entscheidung offen (Pascal):** `PAPIERKRAM_COMPANY`/`company` aus der Projektdatei als Default für `person … --company`? | keine Voreinstellung; nur Anzeige in `local show` | – | keine API-Frage, nicht gemessen |
| 28 | `document_date` Pflicht beim Anlegen | Angebot: Pflicht (gemessen); Rechnung: nicht Pflicht | `TestE2E_DeliverPdfEmptyBody` (Angebot, indirekt) | Angebot erledigt; Rechnung **offen**, nicht gemessen |

## Punkte im Einzelnen

### 1. Statuswerte der Belege

- **Offen:** Die Spec dokumentiert die Werte von `state` für Rechnungen, Angebote und Belege nicht (Beispiele zeigen u. a. `draft`, `canceled`, `unpaid`, `partially_paid`, `deleted`, `imported`).
- **Annahme:** Tabellen zeigen `state` roh; das CLI prüft oder übersetzt keine Statuswerte. Filter nach Status bietet die API nur für Zeiteinträge (`--state`, dort ist das Enum dokumentiert).
- **E2E:** `TestE2E_VoucherStateValues` (Messung) sammelt die Werte der ersten Listenseite von Belegen, Rechnungen und Angeboten und den Status eines neuen Belegs. Danach sollen die Werte in der Doku ergänzt werden.

### 2. Erlaubte Upload-MIME-Typen

- **Offen:** Welche Dateitypen nimmt `POST /expense/vouchers/{id}/documents` an?
- **Annahme:** Keine lokale Einschränkung. Der MIME-Typ folgt aus der Dateiendung, unbekannte Endungen werden als `application/octet-stream` gesendet; eine Ablehnung der API kommt als Exit 5.
- **E2E:** `TestE2E_UploadMimeTypes` lädt PDF, PNG, JPG und TXT hoch. Hart nur für PDF (`voucher create --file beleg.pdf` muss funktionieren), die übrigen sind Messung.

### 3. Maximale Upload-Dateigröße — bewusst nicht gemessen

- **Offen:** Wie groß darf eine hochgeladene Datei sein?
- **Annahme:** Keine lokale Grenze; die ganze Datei muss innerhalb von `--timeout` (Standard 30 s) übertragen sein. Lehnt die API eine Datei ab, kommt Exit 5 (bzw. bei `voucher create --file` Exit 11 mit dem Befehl zum Nachholen).
- **E2E:** Bewusst nicht gemessen: Das hieße, große Dateien in einen echten Account hochzuladen. Bleibt offen, bis Papierkram die Grenze dokumentiert.

### 4. `0.19` ergibt 19 %

- **Offen:** Verbucht die API den Dezimalbruch `0.19` in Positionen (Zahl) und beim Artikel (String) als 19 %?
- **Annahme:** Ja. Das CLI normalisiert jede Schreibweise (`19`, `19%`, `0,19`, Zahl 19) auf 0.19 und sendet sie beim Artikel als String `"0.19"`.
- **E2E:** `TestE2E_VatRate019Is19Percent/Position` und `TestE2E_VatRate019Is19Percent/Artikel` (hart).

### 5. Kosten von `GET /info`

- **Offen:** Kostet `GET /info` Credits?
- **Annahme:** 1 Credit (Regel aus Spec 4.3: alles außer Anlegen kostet 1). Betrifft `info`, `auth login` und `auth status`.
- **E2E:** `TestE2E_InfoCreditCost` (Messung) ruft `info` dreimal auf und vergleicht `X-Consumed-Quota`; eine andere Zahl als in der Registry steht als `ABWEICHUNG` im Log. Dann die Registry anpassen.

### 6. `PUT` als Teil-Update

- **Offen:** Ändert `PUT` nur die gesendeten Felder, oder setzt es fehlende Felder zurück?
- **Annahme:** Teil-Update (so arbeitet auch papierkram-mcp-server live). `update` sendet nur die angegebenen Felder.
- **E2E:** `TestE2E_PartialUpdateSemantics` (hart) mit je einem Subtest pro Ressource: `TestE2E_PartialUpdateSemantics/company`, `TestE2E_PartialUpdateSemantics/person`, `TestE2E_PartialUpdateSemantics/project`, `TestE2E_PartialUpdateSemantics/task`, `TestE2E_PartialUpdateSemantics/proposition`, `TestE2E_PartialUpdateSemantics/estimate`, `TestE2E_PartialUpdateSemantics/invoice`, `TestE2E_PartialUpdateSemantics/voucher`, `TestE2E_PartialUpdateSemantics/time`, `TestE2E_PartialUpdateSemantics/custom-attribute`. Jeder ändert ein Feld und prüft, dass die anderen (und die Positionen) bleiben.

### 7. Positionen im `PUT` ersetzen

- **Offen:** Ersetzen `line_items` im `PUT` alle Positionen? Sie haben im Request keine `id`.
- **Annahme:** Ja. `update` mit `--item`/`--items` verlangt deshalb `--replace-items` (sonst Exit 2); `line_items` aus `--input` werden ohne `--replace-items` nicht gesendet (mit Hinweis).
- **E2E:** `TestE2E_ReplaceLineItems` (hart).

### 8. Nicht deklarierte `{id}`-Pfadparameter — erledigt

- **Offen war:** Die Spec deklariert die Pfadparameter `{id}` teils nicht.
- **Stand:** Im Overlay ergänzt ([api/overlay.yaml](../api/overlay.yaml), Abschnitt 6); kein Live-Punkt, daher kein E2E-Test.

### 9. `deadline` als ISO-Datum

- **Offen:** Das Beispiel der Spec zeigt `deadline` einer Aufgabe als `"15.05.2020"`.
- **Annahme:** Die API nimmt ISO `YYYY-MM-DD` (das Overlay setzt `format: date`); das CLI sendet ISO.
- **E2E:** `TestE2E_Carries/01_DeadlineISO` (hart).

### 10. `due_date` per `UpdateInvoice` nicht änderbar — nicht gemessen

- **Offen:** Das Request-Schema von `UpdateInvoice` hat kein `due_date`. Lässt sich das Fälligkeitsdatum nur über `payment_term` ändern, oder nimmt die API das Feld trotzdem an?
- **Annahme:** Nur über `payment_term`. `invoice update --input` entfernt `due_date` als nur lesbares Feld (Hinweis auf stderr); `papierkram api PUT` sendet es unverändert, die API lehnt es dann vermutlich mit 422 ab.
- **E2E:** Nicht gemessen; die Suite hat dafür keinen Test. Bei Bedarf ließe es sich mit `papierkram api PUT /income/invoices/<id>` und einem Body mit `due_date` an einer Entwurfsrechnung messen.

### 11. `sent_on`/`sent_to` als `null` im Angebots-`PUT`

- **Offen:** Ein Rundlauf `estimate describe -o json | estimate update --input -` sendet für einen Entwurf `sent_on: null` und `sent_to: null`. Nimmt die API das an?
- **Annahme:** Ja.
- **E2E:** `TestE2E_Carries/03_EstimateNullSentFields` (hart).

### 12. `entertainment_persons: []` im Beleg-`PUT`

- **Offen:** Nimmt die API ein leeres Array an, und leert es die Liste oder bleibt sie unverändert?
- **Annahme:** Wird angenommen; das CLI sendet es, wenn es angegeben ist.
- **E2E:** `TestE2E_Carries/04_VoucherEmptyEntertainmentPersons` (Annahme hart, Bedeutung gemessen).

### 13. Rundlauf einer Rechnung: `supply_date` und `customer`

- **Offen:** Die API liefert `supply_date` als `"2026-08-28 13:41:23 +0200"`. Nimmt `PUT` dieses Format zurück? Und bleiben Kontaktperson und Projekt erhalten, wenn `customer` auf `{id}` reduziert gesendet wird?
- **Annahme:** Ja zu beidem; `describe -o json | update --input -` ist ein dokumentierter Ablauf.
- **E2E:** `TestE2E_Carries/02_InvoiceRoundTrip` (hart).

### 14. Werte von `due_time_unit`

- **Offen:** Welche Einheiten haben Zahlungsbedingungen? `due_time_unit` ist in der Spec ein freier String ohne Enum; alle Beispiele zeigen `day`.
- **Annahme:** Meist `day`, aber das CLI nimmt keine Einheit an: Die Tabelle von `payment-term list` zeigt die Spalten `ZIEL` (`due_time`) und `EINHEIT` (`due_time_unit`) getrennt, `-o wide` ebenso für das Skonto. Eine andere Einheit braucht keine Codeänderung.
- **E2E:** `TestE2E_Carries/05_PaymentTermUnits` (Messung); eine andere Einheit als `day` steht als `ABWEICHUNG` von den Spec-Beispielen im Log.

### 15. Einheit von `billable_duration`

- **Offen:** In welcher Einheit erwartet die API `billable_duration` im Request?
- **Annahme:** Sekunden (Overlay).
- **E2E:** `TestE2E_Carries/06_BillableDurationSeconds` (hart).

### 16. Setzt `deliver --via pdf` `sent_on`?

- **Offen:** Setzt die API beim PDF-Versand `sent_on`?
- **Annahme:** Bricht die Verbindung nach dem Senden ab, liest `deliver` den Beleg nach: `sent_on` von heute ergibt Exit 0 mit Warnung, sonst Exit 12. Setzt die API `sent_on` bei `--via pdf` nicht, endet dieser Fall immer mit Exit 12 (sicher, aber unnötig vorsichtig).
- **E2E:** `TestE2E_Carries/07_DeliverPdfSentOn` (Messung).

### 17. `Accept` beim PDF-Download

- **Offen:** Die Spec nennt den Content-Type des PDFs wörtlich `["application/pdf"]`. Welcher `Accept`-Header liefert das PDF?
- **Annahme:** `Accept: application/pdf, application/json;q=0.9` ergibt 200 mit `application/pdf`; Fehler kommen weiter als JSON.
- **E2E:** `TestE2E_Carries/08_PDFDownloadAccept` (hart).

### 18. `price_gross: 0.0` bei `gross: false`

- **Offen:** `invoice create --from-estimate` übernimmt `price_gross` aus dem Angebot; bei Netto-Angeboten ist das 0.0. Nimmt die API das an?
- **Annahme:** Ja.
- **E2E:** `TestE2E_Carries/09_FromEstimatePriceGrossZero` (hart).

### 19. `voucher pay` mit `value: null` und Bankumsatz

- **Offen:** Nimmt `pay` `value: null` neben `banking_transaction` an?
- **Annahme:** Ja; mit nur `--transaction` sendet das CLI `value` gar nicht.
- **E2E:** `TestE2E_Carries/10_VoucherPayValueNull` (Messung) mit der Bankumsatz-ID 999999999, die es nicht gibt, und ohne Betrag: Die Suite bezahlt nie etwas, ihr Transport lässt für `pay` nur genau diesen Body durch.

### 20. `voucher pay` mit Betrag und Bankumsatz zusammen — bewusst nicht gemessen

- **Offen:** Was passiert, wenn `value` und `banking_transaction` zusammen kommen (Vorrang, Abweichung, Fehler)?
- **Annahme:** Das CLI sendet beide, wenn beide angegeben sind, und überlässt die Entscheidung der API.
- **E2E:** Bewusst nicht gemessen: Die Messung würde eine echte Zahlung buchen, die sich per API nicht zurücknehmen lässt.

### 21. Dauer an Tagen der Zeitumstellung

- **Offen:** Rechnet die API die Dauer eines Zeiteintrags als Wanduhr-Differenz, auch am Tag der Zeitumstellung (z. B. 01:30–02:30 am 25.10.)?
- **Annahme:** Wanduhr-Differenz; `time log --from 01:30 --duration 2h` ergibt 01:30–03:30, auch an Umstellungstagen.
- **E2E:** `TestE2E_Carries/11_DurationWallClock` (hart) misst nur einen normalen Tag. Der Umstellungstag ist **bewusst nicht gemessen**, sondern nur dokumentiert (Entscheidung zu Task 44).

### 22. Ende um Mitternacht

- **Offen:** Nimmt die API `ended_at_time` `"00:00"` oder `"24:00"` nach einem Beginn um 23:00 an, und als eine Stunde?
- **Annahme:** Das CLI lehnt Einträge über Mitternacht ab (Exit 2), auch ein Ende um 00:00.
- **E2E:** `TestE2E_Carries/12_MidnightEnd` (Messung, über `papierkram api` ohne lokale Prüfung).

### 23. Antwort von `GET /info`

- **Offen:** Die Spec hat für `GET /info` kein Response-Schema; `papierkram schema info` zeigt deshalb `"response": null`.
- **Annahme:** Das CLI gibt die Antwort unverändert aus und wertet sie nicht aus; das Restkontingent (`auth status`) kommt aus den Headern. Die Rechnungsvorlagen stehen laut Spec-Beispiel unter `settings.custom_templates.invoices`.
- **E2E:** `TestE2E_Carries/13_InfoStructure` (Messung) loggt die Struktur der Antwort.

### 24. Leerer Body bei `deliver`

- **Offen:** Die Spec behauptet 201 mit Objekt, live antwortet `deliver` laut papierkram-mcp-server mit 2xx und leerem Body.
- **Annahme:** Ein leerer 2xx-Body ist Erfolg; danach liest das CLI den Beleg nach.
- **E2E:** `TestE2E_DeliverPdfEmptyBody` (2xx und Exit 0 hart, der Body ist Messung).

### 25. Quota-Header

- **Offen:** `X-Remaining-Quota` und `X-Consumed-Quota` fehlen in der Spec.
- **Annahme:** Beide kommen mit jeder Antwort als ganze Zahl; `X-RateLimit-Remaining` wird bewusst ignoriert.
- **E2E:** `TestE2E_QuotaHeadersPresent` (hart).

### 26. Person `default` als String

- **Offen:** Die Spec typisiert `default` (und `flagged`) der Kontaktperson als String.
- **Annahme:** Das CLI sendet `"true"`/`"false"` als String.
- **E2E:** `TestE2E_PersonDefaultIsString` (hart).

### 27. Entscheidung offen (Pascal): `company` als Voreinstellung?

- **Offen:** Soll `PAPIERKRAM_COMPANY` bzw. `company` aus der Projektdatei künftig `--company` der `person`-Befehle (`person list|describe|create|update|delete`, dort Pflicht) vorbelegen? Spec 9.8 nennt den Schlüssel im Beispiel, aber keinen Befehl, der ihn nutzt.
- **Stand des CLI:** Keine Voreinstellung. `local init --company` schreibt den Wert, `local show` zeigt ihn mit Herkunft an; kein Befehl verwendet ihn als Standardwert. Das README sagt das ausdrücklich.
- **E2E:** Keine API-Frage, daher kein Test. Entscheidet Pascal für die Voreinstellung, braucht es eine eigene Task (Spec 9.8, `default_from` in `commands --json`, Tests).

### 28. `document_date` Pflicht beim Anlegen

- **Gemessen 2026-11-04 (erster Live-Lauf):** `POST /income/estimates` ohne `document_date` lehnt die API mit 422 „Datum muss ausgefüllt werden“ ab und berechnet trotzdem die 50 Credits des Anlegens. Die Spec nennt nur `name` und `line_items` als Pflicht.
- **Stand des CLI:** Das Overlay ([api/overlay.yaml](../api/overlay.yaml), Abschnitt 16) macht `document_date` im Anlege-Body des Angebots zur Pflicht (auch `null` gilt als fehlend). `estimate create` ohne `--document-date` (bzw. ohne `document_date` in `--input`) endet mit Exit 5, bevor eine Anfrage gesendet wird. `invoice create --from-estimate` ist nicht betroffen: Das Rechnungsdatum kommt nie aus dem Angebot, sondern aus `--document-date`.
- **Offen:** Ob auch `POST /income/invoices` ohne `document_date` scheitert, ist nicht gemessen. Das CLI behandelt das Feld bei Rechnungen deshalb weiter als optional; die E2E-Suite sendet es vorsichtshalber immer mit. Bei einer Messung (422 wie beim Angebot) das Overlay entsprechend ergänzen.
