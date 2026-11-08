# Offene Punkte der Papierkram-API

Die öffentliche API-Spezifikation lässt einige Fragen offen oder widerspricht dem Live-Verhalten. Diese Liste nennt je Punkt, **was offen ist**, **was das CLI derzeit annimmt** und **welcher Test der Live-E2E-Suite** (`e2e/`, Build-Tag `e2e`, nur gegen einen Sandbox-Account, siehe [e2e/README.md](../e2e/README.md)) ihn misst. Grundlage sind Abschnitt 17 der [Spezifikation](specs/2026-11-03-papierkram-cli-design.md) und die Funde aus den Reviews.

Die Suite unterscheidet zwei Arten von Prüfungen:

- **hart:** Das CLI verlässt sich auf das Verhalten; weicht die API ab, schlägt der Test fehl.
- **Messung:** Das Ergebnis steht im Log mit dem Präfix `MESSUNG`; weicht es von der Annahme des CLI ab, zusätzlich mit `ABWEICHUNG:` und in der Zusammenfassung am Ende des Laufs. Eine Messung schlägt nicht fehl.

Testnamen sind die Namen für `go test -run`, Subtests mit `/` (z. B. `go test -tags e2e -run 'TestE2E_Carries/01_DeadlineISO' ./e2e/...`). Stand: Die Live-Läufe 1 bis 4 gegen einen Sandbox-Account fanden am 2026-11-04 statt; ihre Ergebnisse stehen bei den Punkten unter **Gemessen 2026-11-04**. Live-Lauf 5 mit den neuen Prüfungen (`TestE2E_InvoiceFlow`, `TestE2E_VoucherFlow`, `TestE2E_TimeFlow`, `TestE2E_MasterDataArchive`, `TestE2E_ReadCommands`, `TestE2E_OpenMeasurements`, `TestE2E_Carries/12_MidnightEnd` in der Form über `time log`, `TestE2E_Carries/14_InvoiceUpdateSameCustomer`) fand am 2026-11-08 statt (17 von 17 Tests bestanden, Abdeckung 88 von 89 Operationen, `GetTransaction` nicht anwendbar, 688 von 760 Credits); seine Ergebnisse stehen bei den Punkten unter **Gemessen Live-Lauf 5, 2026-11-08** und unter [Weitere Ergebnisse von Live-Lauf 5](#weitere-ergebnisse-von-live-lauf-5). In Live-Lauf 1 scheiterten viele Prüfungen an einer Vorbedingung (Angebot ohne `document_date`, Account als Kleinunternehmer ohne Umsatzsteuer); Punkte, die auch die Läufe 2 bis 4 nicht gemessen haben, sind weiterhin **nicht live beantwortet**.

## Übersicht

| Nr. | Punkt | Annahme des CLI | E2E-Test | Art |
|---|---|---|---|---|
| 1 | Statuswerte (`state`) der Belege | `state` wird roh angezeigt, nicht geprüft | `TestE2E_VoucherStateValues`, `TestE2E_InvoiceFlow`, `TestE2E_VoucherFlow` | Messung (Lebenszyklus gemessen Live-Lauf 5, 2026-11-08) |
| 2 | Erlaubte Upload-MIME-Typen | keine lokale Einschränkung; gemessen: PDF, PNG, JPG ja, TXT HTTP 500 | `TestE2E_UploadMimeTypes`, `TestE2E_VoucherFlow` | PDF hart, sonst Messung (`TestE2E_VoucherFlow` gemessen Live-Lauf 5, 2026-11-08) |
| 3 | Maximale Upload-Dateigröße | keine lokale Grenze | – | **bewusst nicht gemessen** |
| 4 | `0.19` ergibt 19 % (Position und Artikel) | `0.19` als Zahl bzw. beim Artikel als String `"0.19"` | `TestE2E_VatRate019Is19Percent/Position`, `TestE2E_VatRate019Is19Percent/Artikel` | hart |
| 5 | Kosten von `GET /info` | 1 Credit | `TestE2E_InfoCreditCost` | Messung |
| 6 | `PUT` ist ein Teil-Update | nur angegebene Felder werden gesendet; Rechnung: ohne `payment_term` stellt die API auf den Standard um, `invoice update` sendet die aktuelle mit | `TestE2E_PartialUpdateSemantics` (ein Subtest je Ressource, siehe unten) | hart |
| 7 | `line_items` im `PUT` ersetzen alle Positionen | ja; deshalb `--replace-items` | `TestE2E_ReplaceLineItems` | hart |
| 8 | `{id}`-Pfadparameter in der Spec nicht deklariert | im Overlay ergänzt | – | erledigt (kein Live-Punkt) |
| 9 | `deadline` als ISO-Datum | `YYYY-MM-DD` | `TestE2E_Carries/01_DeadlineISO` | hart |
| 10 | `due_date` per `UpdateInvoice` nicht änderbar | Feld wird aus `--input` entfernt; die API lehnt es mit 422 ab | `TestE2E_OpenMeasurements/10_InvoiceDueDatePut` | Messung; geklärt (gemessen Live-Lauf 5, 2026-11-08) |
| 11 | `sent_on`/`sent_to` als `null` im Angebots-`PUT` | wird akzeptiert | `TestE2E_Carries/03_EstimateNullSentFields` | hart |
| 12 | `entertainment_persons: []` im Beleg-`PUT` | wird akzeptiert und leert die Liste | `TestE2E_OpenMeasurements/12_VoucherEntertainmentPersons` (Bedeutung); Annahme hart in `TestE2E_Carries/04_VoucherEmptyEntertainmentPersons` | Messung (Bedeutung gemessen Live-Lauf 5, 2026-11-08) |
| 13 | `supply_date` im Antwortformat und Kontaktperson/Projekt im Rundlauf | Kontaktperson und Projekt werden unter `customer` mitgesendet und bleiben (gemessen Lauf 3 und 4); `supply_date` kommt als `YYYY-MM-DD` und im Rundlauf unverändert zurück (gemessen Lauf 5) | `TestE2E_OpenMeasurements/13_InvoiceSupplyDate` (`supply_date`); Kontaktperson und Projekt hart in `TestE2E_Carries/02_InvoiceRoundTrip` | Messung (`supply_date` gemessen Live-Lauf 5, 2026-11-08) |
| 14 | Werte von `due_time_unit` | meist `day`; die Tabelle zeigt `ZIEL` und `EINHEIT` getrennt | `TestE2E_Carries/05_PaymentTermUnits` | Messung |
| 15 | Einheit von `billable_duration` | Sekunden; die API ignoriert den Wert beim Anlegen und Ändern (gemessen) | `TestE2E_Carries/06_BillableDurationSeconds` | Messung |
| 16 | Setzt `deliver --via pdf` `sent_on`? | nein (gemessen an Angebot und Rechnung); abgebrochener PDF-Versand endet mit Exit 12 | `TestE2E_Carries/07_DeliverPdfSentOn`, `TestE2E_InvoiceFlow` | Messung (Rechnung gemessen Live-Lauf 5, 2026-11-08) |
| 17 | `Accept` beim PDF-Download | `application/pdf, application/json;q=0.9` ergibt 200 `application/pdf` | `TestE2E_Carries/08_PDFDownloadAccept` | hart |
| 18 | `price_gross: 0.0` bei `gross: false` (`--from-estimate`) | wird akzeptiert | `TestE2E_Carries/09_FromEstimatePriceGrossZero` | hart |
| 19 | `voucher pay`: `value: null` mit Bankumsatz | wird akzeptiert | `TestE2E_Carries/10_VoucherPayValueNull` | Messung |
| 20 | `voucher pay`: Betrag und Bankumsatz zusammen | beide werden gesendet | – | **bewusst nicht gemessen** |
| 21 | Dauer an Tagen der Zeitumstellung | Wanduhr-Differenz | `TestE2E_Carries/11_DurationWallClock` (nur normaler Tag) | hart; Umstellungstag **bewusst nicht gemessen** |
| 22 | Ende um Mitternacht (`00:00`, `24:00`) | erlaubt: `time log --to 00:00`/`24:00` und `time stop` in der ersten Minute nach Mitternacht senden `24:00`; andere Einträge über Mitternacht Exit 2 | `TestE2E_Carries/12_MidnightEnd` | entschieden und umgesetzt (Task 10); hart (über `time log` bestätigt, Live-Lauf 5, 2026-11-08) |
| 23 | Antwort von `GET /info` ohne Schema | JSON wird unverändert ausgegeben | `TestE2E_Carries/13_InfoStructure` | Messung |
| 24 | `deliver` antwortet mit leerem Body | leerer 2xx-Body ist Erfolg | `TestE2E_DeliverPdfEmptyBody` | hart (2xx, Exit 0); Body: Messung |
| 25 | Quota-Header `X-Remaining-Quota`, `X-Consumed-Quota` | vorhanden und ganzzahlig | `TestE2E_QuotaHeadersPresent` | hart |
| 26 | Person `default` als String | Request `"true"`/`"false"`; Antwort Boolean (gemessen) | `TestE2E_PersonDefaultIsString` | hart |
| 27 | `PAPIERKRAM_COMPANY`/`company` aus der Projektdatei als Default für `person … --company` | Voreinstellung wie die übrigen: Flag > `PAPIERKRAM_COMPANY` > Projektdatei; ohne Wert Exit 2 | – | entschieden und umgesetzt (Task 9); keine API-Frage, nicht gemessen |
| 28 | `document_date` Pflicht beim Anlegen | Angebot und Rechnung: Pflicht (gemessen), Exit 5 ohne Anfrage | `TestE2E_OpenMeasurements/28_InvoiceWithoutDocumentDate` (lokale Ablehnung der Rechnung); Angebot indirekt in `TestE2E_DeliverPdfEmptyBody` | entschieden und umgesetzt (Rechnung gemessen Live-Lauf 5, 2026-11-08); hart (0 Credits) |
| 29 | `invoice update --customer X` ohne Kontaktperson/Projekt | gleicher Kunde: Kontaktperson und Projekt aus der gelesenen Rechnung mitgesendet; anderer Kunde: entfallen (Hinweis) | `TestE2E_Carries/14_InvoiceUpdateSameCustomer` | entschieden und umgesetzt (Task 8); hart (bestätigt Live-Lauf 5, 2026-11-08) |

## Punkte im Einzelnen

### 1. Statuswerte der Belege

- **Offen:** Die Spec dokumentiert die Werte von `state` für Rechnungen, Angebote und Belege nicht (Beispiele zeigen u. a. `draft`, `canceled`, `unpaid`, `partially_paid`, `deleted`, `imported`).
- **Annahme:** Tabellen zeigen `state` roh; das CLI prüft oder übersetzt keine Statuswerte. Filter nach Status bietet die API nur für Zeiteinträge (`--state`, dort ist das Enum dokumentiert).
- **E2E:** `TestE2E_VoucherStateValues` (Messung) sammelt die Werte der ersten Listenseite von Belegen, Rechnungen und Angeboten und den Status eines neuen Belegs. `TestE2E_InvoiceFlow` (Messung) misst die Statuswerte einer eigenen Rechnung im Lebenszyklus: `state` nach dem Anlegen, nach `deliver --via pdf` (mit `invoice_no`, `sent_on`, `sent_via`) und nach `cancel` (mit Exit, HTTP-Status und Meldung der API), `record_state` nach `archive`, außerdem `state` des versendeten Angebots nach `estimate cancel`. `TestE2E_VoucherFlow` (Messung) misst an einem eigenen Beleg `voucher reverse` und danach `voucher cancel` (je Exit, HTTP-Status und Meldung der API) und `state` nach jedem Schritt; nennt die Antwort ein anderes Objekt als den Beleg (Folgeobjekt, z. B. eine Gegenbuchung), steht das als `ABWEICHUNG` im Log, und das Objekt wird mit aufgeräumt.
- **Gemessen 2026-11-04 (Lauf 2):** Ein neuer Beleg hat `state` `unpaid`; die erste Belegliste zeigte nur `unpaid`, die Angebotsliste `draft` und `sent` (nach `deliver --via pdf`), die Rechnungsliste war leer. Weitere Werte (z. B. von Rechnungen) bleiben offen.
- **Gemessen Live-Lauf 5, 2026-11-08:** **Rechnung:** nach dem Anlegen `draft`, nach `deliver --via pdf` `unpaid` (Nummer `R-00001` vergeben, `sent_on` und `sent_via` `null`), nach `cancel` (Exit 0, HTTP 200) `canceled`; der Name bekommt dabei den Zusatz „ (storniert)“. Nach `archive` ist `record_state` `archived`. **Angebot:** `draft`, nach `deliver --via pdf` `sent`, nach `estimate cancel` (Exit 0, HTTP 200) `canceled`. **Beleg:** neu `unpaid`; `voucher reverse` an einem unbezahlten Beleg lehnt die API mit HTTP 422 „Voucher is not cancelable with reverse entry.“ ab, `voucher cancel` mit HTTP 422 „Voucher is not cancelable.“ (je Exit 5); der Beleg bleibt `unpaid`, ein Folgeobjekt entsteht nicht. Die Listen zeigten bei Belegen nur `unpaid`, bei Angeboten `canceled`, `draft` und `sent`; die Rechnungsliste war leer. Ob `reverse` und `cancel` an einem bezahlten Beleg gelingen, bleibt offen (die Suite bezahlt nie).

### 2. Erlaubte Upload-MIME-Typen

- **Offen:** Welche Dateitypen nimmt `POST /expense/vouchers/{id}/documents` an?
- **Annahme:** Keine lokale Einschränkung. Der MIME-Typ folgt aus der Dateiendung, unbekannte Endungen werden als `application/octet-stream` gesendet. Eine Ablehnung mit 4xx käme als Exit 5; gemessen lehnt die API TXT aber mit HTTP 500 ab (Exit 1, siehe unten).
- **E2E:** `TestE2E_UploadMimeTypes` lädt PDF, PNG, JPG und TXT per `voucher upload` hoch. Hart nur für PDF, die übrigen sind Messung. `TestE2E_VoucherFlow` prüft hart, dass `voucher create --file beleg.pdf` mit Exit 0 endet (kein Exit 11) und der Beleg danach genau ein Dokument hat; die Schlüssel dieses Dokuments (ohne Werte) und der PDF-Download des Belegs (`voucher pdf`: Content-Type, Größe, beginnt mit `%PDF`) sind Messung.
- **Gemessen 2026-11-04 (Lauf 2):** PDF, PNG und JPG nimmt die API an (HTTP 201). TXT lehnt sie mit **HTTP 500** („Internal server error.“) ab statt mit 4xx (andere Typen nicht gemessen); das CLI meldet das als Exit 1 (`server_error`), bei `voucher create --file` als Exit 11 mit „Ergebnis des Hochladens unbekannt“. Ein Upload eines nicht unterstützten Typs ist also nicht vorübergehend: nicht blind wiederholen, sondern den Dateityp ändern.
- **Gemessen Live-Lauf 5, 2026-11-08:** erneut PDF, PNG und JPG HTTP 201, TXT HTTP 500 (Exit 1). `voucher create --file beleg.pdf` endet mit Exit 0 (kein Exit 11), der Beleg hat danach genau ein Dokument mit den Schlüsseln `{id,type,uri}`; `voucher pdf` liefert HTTP 200 mit `application/pdf` (eine Datei, die mit `%PDF` beginnt).

### 3. Maximale Upload-Dateigröße — bewusst nicht gemessen

- **Offen:** Wie groß darf eine hochgeladene Datei sein?
- **Annahme:** Keine lokale Grenze; die ganze Datei muss innerhalb des Zeitlimits übertragen sein (ohne `--timeout` je Versuch mindestens 120 s, ein größeres `timeout` des Kontexts gilt; mit `--timeout` genau dieser Wert). Lehnt die API eine Datei ab, kommt Exit 5 (bzw. bei `voucher create --file` Exit 11 mit dem Befehl zum Nachholen).
- **E2E:** Bewusst nicht gemessen: Das hieße, große Dateien in einen echten Account hochzuladen. Bleibt offen, bis Papierkram die Grenze dokumentiert.

### 4. `0.19` ergibt 19 %

- **Offen:** Verbucht die API den Dezimalbruch `0.19` in Positionen (Zahl) und beim Artikel (String) als 19 %?
- **Annahme:** Ja. Das CLI normalisiert jede Schreibweise (`19`, `19%`, `0,19`, Zahl 19) auf 0.19 und sendet sie beim Artikel als String `"0.19"`.
- **E2E:** `TestE2E_VatRate019Is19Percent/Position` und `TestE2E_VatRate019Is19Percent/Artikel` (hart).
- **Gemessen 2026-11-04 (Lauf 2):** bestätigt (nach Umstellung des Sandbox-Accounts von Kleinunternehmer auf Regelbesteuerung): Position mit `0.19` (Zahl) ergibt `vat_rate` `"19%"`, total_vat 38 bei total_net 200; Artikel mit `"0.19"` ergibt den Steuersatz „19%“ (rate 0.19). In Lauf 1 buchte der Kleinunternehmer-Account 0 %.

### 5. Kosten von `GET /info`

- **Offen:** Kostet `GET /info` Credits?
- **Annahme:** 1 Credit (Regel aus Spec 4.3: alles außer Anlegen kostet 1). Betrifft `info`, `auth login` und `auth status`.
- **E2E:** `TestE2E_InfoCreditCost` (Messung) ruft `info` dreimal auf; die Kosten jedes Aufrufs sind sein eigenes `X-Consumed-Quota` (siehe Punkt 25). Weichen sie von der Registry ab oder voneinander, steht das als `ABWEICHUNG` im Log; dann die Registry anpassen. Die Differenz von `X-Remaining-Quota` wird nur zur Information geloggt.
- **Gemessen 2026-11-04 (erster Live-Lauf):** `X-Consumed-Quota` war bei allen drei Aufrufen 1, also 1 Credit wie in der Registry. Die damalige `ABWEICHUNG` („kostet 0 Credits“) war ein Messfehler der Suite: Sie las den Header als laufenden Zähler und nahm die Differenz.
- **Gemessen 2026-11-04 (Lauf 2):** erneut `X-Consumed-Quota` 1 je Aufruf, keine Abweichung.

### 6. `PUT` als Teil-Update

- **Offen:** Ändert `PUT` nur die gesendeten Felder, oder setzt es fehlende Felder zurück?
- **Annahme:** Teil-Update (so arbeitet auch papierkram-mcp-server live). `update` sendet nur die angegebenen Felder.
- **E2E:** `TestE2E_PartialUpdateSemantics` (hart) mit je einem Subtest pro Ressource: `TestE2E_PartialUpdateSemantics/company`, `TestE2E_PartialUpdateSemantics/person`, `TestE2E_PartialUpdateSemantics/project`, `TestE2E_PartialUpdateSemantics/task`, `TestE2E_PartialUpdateSemantics/proposition`, `TestE2E_PartialUpdateSemantics/estimate`, `TestE2E_PartialUpdateSemantics/invoice`, `TestE2E_PartialUpdateSemantics/voucher`, `TestE2E_PartialUpdateSemantics/time`, `TestE2E_PartialUpdateSemantics/custom-attribute`. Jeder ändert ein Feld und prüft, dass die anderen (und die Positionen) bleiben.
- **Gemessen 2026-11-04 (Lauf 2):** Teil-Update bestätigt für company, person, project, task, proposition, estimate, voucher, time und custom-attribute. **Rechnung:** nach `invoice update --description …` (gesendet nur `description`, per Test belegt) hatte `payment_term.id` einen anderen Wert (22 → 25). `payment_term` einer Rechnung ist eine Kopie der gewählten Vorlage (in allen Spec-Beispielen `id` = `template_id` + 1); ob nur die Kopie neu angelegt wurde (gleiche `template_id`) oder die Zahlungsbedingung auf einen Standard zurückfiel, ist offen. Lauf 3 misst `id`, `template_id` und `name` vorher und nachher und loggt die Schlüssel aller anderen geänderten Felder; der Subtest prüft jetzt hart nur noch `payment_term.template_id`. Bis dahin: wer sichergehen will, gibt bei `invoice update` `--payment-term` mit.
- **Gemessen 2026-11-04 (Lauf 3):** Bei der Rechnung stellte der `PUT` mit nur `description` die Zahlungsbedingung von Vorlage 1 („Barzahlung“) auf Vorlage 16 („Auf Rechnung, 14 Tage“, Standard des Accounts) um und änderte `due_date`; Kontaktperson und Projekt blieben. Beim Angebot zeigte sich nichts dergleichen. (Der Rundlauf in Lauf 3 sendete die aktuelle `payment_term.id` und behielt sie, die Rechnung stand da aber schon auf der Standardvorlage; als Beleg taugt das nicht.)
- **Gemessen 2026-11-04 (Lauf 4, mit dem Lesen):** Teil-Update und Rundlauf behalten die Zahlungsbedingung: Vorlage 1 „Barzahlung“, `payment_term.id` 34 vorher und nachher, in beiden Fällen. Damit ist belegt, dass ein `PUT` mit der aktuellen `payment_term.id` (der Kopie) sie behält und keine neue Kopie anlegt.
- **Stand des CLI (Ruling E2E-A):** `invoice update` ohne `--payment-term` und ohne `payment_term` in `--input` liest die Rechnung vorher (1 Credit, bei `--dry-run` als `read` ausgewiesen) und sendet `payment_term: {id: <aktuelle payment_term.id>}` mit; scheitert das Lesen, wird nichts geändert. Der Subtest `TestE2E_PartialUpdateSemantics/invoice` prüft seitdem hart, dass `payment_term.template_id` und `due_date` bleiben. `papierkram api PUT` sendet weiter nur, was angegeben ist.

### 7. Positionen im `PUT` ersetzen

- **Offen:** Ersetzen `line_items` im `PUT` alle Positionen? Sie haben im Request keine `id`.
- **Annahme:** Ja. `update` mit `--item`/`--items` verlangt deshalb `--replace-items` (sonst Exit 2); `line_items` aus `--input` werden ohne `--replace-items` nicht gesendet (mit Hinweis).
- **E2E:** `TestE2E_ReplaceLineItems` (hart).
- **Gemessen 2026-11-04 (Lauf 2):** bestätigt: vorher 2 Positionen, nachher nur die gesendete.

### 8. Nicht deklarierte `{id}`-Pfadparameter — erledigt

- **Offen war:** Die Spec deklariert die Pfadparameter `{id}` teils nicht.
- **Stand:** Im Overlay ergänzt ([api/overlay.yaml](../api/overlay.yaml), Abschnitt 6); kein Live-Punkt, daher kein E2E-Test.

### 9. `deadline` als ISO-Datum

- **Offen:** Das Beispiel der Spec zeigt `deadline` einer Aufgabe als `"15.05.2020"`.
- **Annahme:** Die API nimmt ISO `YYYY-MM-DD` (das Overlay setzt `format: date`); das CLI sendet ISO.
- **E2E:** `TestE2E_Carries/01_DeadlineISO` (hart).
- **Gemessen 2026-11-04 (Lauf 2):** bestätigt: gesendet und gespeichert `2026-12-31`.

### 10. `due_date` per `UpdateInvoice` nicht änderbar

- **Offen:** Das Request-Schema von `UpdateInvoice` hat kein `due_date`. Lässt sich das Fälligkeitsdatum nur über `payment_term` ändern, oder nimmt die API das Feld trotzdem an?
- **Annahme:** Nur über `payment_term`. `invoice update --input` entfernt `due_date` als nur lesbares Feld (Hinweis auf stderr); `papierkram api PUT` sendet es unverändert, die API lehnt es dann mit 422 ab (gemessen, siehe unten).
- **E2E:** `TestE2E_OpenMeasurements/10_InvoiceDueDatePut` (Messung): `papierkram api PUT /income/invoices/<id>` ohne `--validate` (der Body geht unverändert hinaus) an der Entwurfsrechnung der Suite mit `{"due_date":"<heute + 45 Tage>","payment_term":{"id":<aktuelle>}}`, davor und danach `invoice describe` (3 Credits). Im Log stehen HTTP-Status und `due_date` vorher und nachher; ändert sich `due_date`, steht das als `ABWEICHUNG` im Log. Lehnte `api` den Body lokal ab (Exit 5, nichts gesendet), stünde „lokal abgelehnt“ im Log.
- **Gemessen Live-Lauf 5, 2026-11-08:** `PUT` mit `due_date` (heute + 45 Tage) und der aktuellen `payment_term` 37: HTTP 422 „Die folgenden Felder können nicht geschrieben werden und müssen aus dem Payload entfernt werden: 'due_date'.“ (Exit 5); `due_date` blieb `2026-11-08`. Damit ist bestätigt, dass das CLI das Feld zu Recht entfernt; der Punkt gilt als geklärt.

### 11. `sent_on`/`sent_to` als `null` im Angebots-`PUT`

- **Offen:** Ein Rundlauf `estimate describe -o json | estimate update --input -` sendet für einen Entwurf `sent_on: null` und `sent_to: null`. Nimmt die API das an?
- **Annahme:** Ja.
- **E2E:** `TestE2E_Carries/03_EstimateNullSentFields` (hart).
- **Gemessen 2026-11-04 (Lauf 2):** bestätigt: `PUT` mit `sent_on: null` und `sent_to: null` HTTP 200, beide danach `null`.

### 12. `entertainment_persons: []` im Beleg-`PUT`

- **Offen:** Nimmt die API ein leeres Array an, und leert es die Liste oder bleibt sie unverändert?
- **Annahme:** Wird angenommen; das CLI sendet es, wenn es angegeben ist.
- **E2E:** `TestE2E_Carries/04_VoucherEmptyEntertainmentPersons` (Annahme hart); die Bedeutung misst `TestE2E_OpenMeasurements/12_VoucherEntertainmentPersons` (Messung, 4 Credits): Er setzt am Beleg der Suite `entertainment_persons` auf eine Person (`pk-e2e-<lauf-id>-gast`), liest den Beleg, sendet `[]` und liest ihn erneut; im Log steht „geleert“ oder „unverändert“. Lehnt die API schon die Person ab, endet die Messung dort; ein abgelehntes `[]` steht als `ABWEICHUNG` im Log.
- **Gemessen 2026-11-04 (Lauf 2):** angenommen; danach `[]` (die Liste war vorher schon leer, die Bedeutung „leeren oder unverändert“ bleibt damit offen).
- **Gemessen Live-Lauf 5, 2026-11-08:** Die API nimmt eine Bewirtungsperson (`pk-e2e-<lauf-id>-gast`) an und danach `[]`; die Liste ist danach leer. `[]` leert also die Liste.

### 13. Rundlauf einer Rechnung: `supply_date` und `customer`

- **Offen:** Die API liefert `supply_date` als `"2026-08-28 13:41:23 +0200"`. Nimmt `PUT` dieses Format zurück? Und bleiben Kontaktperson und Projekt erhalten, wenn `customer` auf `{id}` reduziert gesendet wird?
- **Annahme:** Ja zu beidem; `describe -o json | update --input -` ist ein dokumentierter Ablauf.
- **E2E:** `TestE2E_Carries/02_InvoiceRoundTrip` (hart); loggt seit Lauf 3 die Schlüssel des gesendeten Bodys (ohne Werte) und die IDs unter `customer`. `supply_date` misst `TestE2E_OpenMeasurements/13_InvoiceSupplyDate` (Messung, 5 Credits): `invoice update --supply-date 2026-11-01` (liest die Rechnung vorher), `invoice describe` (Format der Antwort im Log, Ziffern als `9`), dann genau diese Ausgabe per `invoice update --input -` zurück (mit `payment_term` im Body, ohne Lesen) und erneut `describe`. Scheitert ein Schritt, weicht die Antwort von `2026-11-01` ab oder ändert der Rundlauf `supply_date`, steht das als `ABWEICHUNG` im Log.
- **Gemessen 2026-11-04 (Lauf 2):** Gesendet wurde `customer` als `{id}` ohne Kontaktperson und Projekt (die Antwort hat beide auf oberster Ebene, die das CLI damals als nur lesbar entfernte). Danach waren `contact_person` und `project` der Rechnung leer. `supply_date` war im Lauf leer und ist damit weiter ungeprüft.
- **Stand des CLI:** Die Bereinigung von `--input` übernimmt `contact_person` und `project` der Antwort jetzt als `customer.contact_person`/`customer.project` (`{id}`), wenn `customer` im Body steht (Hinweis „Referenzen übernommen“); nennt `--customer` einen anderen Kunden, entfallen sie wieder (Hinweis „Kunde X statt Y aus --input“).
- **Gemessen 2026-11-04 (Lauf 3 und 4):** Mit `customer{contact_person{id},id,project{id}}` im `PUT` bleiben Kontaktperson und Projekt erhalten (Lauf 3: 7 → 7, 16 → 16; Lauf 4: 10 → 10, 22 → 22). `supply_date` war weiter leer, das Antwortformat ist damit ungeprüft.
- **Gemessen Live-Lauf 5, 2026-11-08:** Nach `invoice update --supply-date 2026-11-01` liefert `describe` `supply_date` als String `YYYY-MM-DD` (`2026-11-01`), nicht im Format `"2026-08-28 13:41:23 +0200"` der Spec. Der Rundlauf `describe -o json | update --input -` sendet es unverändert zurück (HTTP 200, keine Leseanfrage davor), danach steht weiter `2026-11-01` da. Kontaktperson und Projekt blieben auch in Lauf 5 erhalten (13 → 13, 28 → 28).

### 14. Werte von `due_time_unit`

- **Offen:** Welche Einheiten haben Zahlungsbedingungen? `due_time_unit` ist in der Spec ein freier String ohne Enum; alle Beispiele zeigen `day`.
- **Annahme:** Meist `day`, aber das CLI nimmt keine Einheit an: Die Tabelle von `payment-term list` zeigt die Spalten `ZIEL` (`due_time`) und `EINHEIT` (`due_time_unit`) getrennt, `-o wide` ebenso für das Skonto. Eine andere Einheit braucht keine Codeänderung.
- **E2E:** `TestE2E_Carries/05_PaymentTermUnits` (Messung); eine andere Einheit als `day` steht als `ABWEICHUNG` von den Spec-Beispielen im Log.
- **Gemessen 2026-11-04 (Lauf 2):** alle 7 Zahlungsbedingungen der Sandbox haben `day`.

### 15. Einheit von `billable_duration`

- **Offen:** In welcher Einheit erwartet die API `billable_duration` im Request?
- **Annahme:** Sekunden (Overlay).
- **Gemessen 2026-11-04: API überschreibt billable_duration mit duration bei create.** Ein Zeiteintrag 09:00–10:30 mit `billable_duration` 1800 wurde mit `billable_duration` 5400 gespeichert, also mit `duration`. Die Einheit lässt sich so nicht bestätigen. Das CLI kann dagegen nichts tun; die Hilfe von `time create --billable-duration` weist darauf hin.
- **Gemessen 2026-11-04 (Lauf 2):** Auch `time update --billable-duration 1800` ändert nichts: gespeichert bleibt 5400 (= `duration`). Die API ignoriert den Wert beim Anlegen und beim Ändern und rechnet aus Beginn und Ende. Die Hilfe von `time create` und `time update` nennt `--billable-duration` deshalb wirkungslos; das Flag bleibt, weil es zur Spec gehört.
- **E2E:** `TestE2E_Carries/06_BillableDurationSeconds` (Messung, seit 2026-11-04 nicht mehr hart): misst den Wert nach dem Anlegen und nach `time update --billable-duration`; weicht der gespeicherte Wert vom gesendeten ab, steht das als `ABWEICHUNG` im Log.

### 16. Setzt `deliver --via pdf` `sent_on`?

- **Offen:** Setzt die API beim PDF-Versand `sent_on`?
- **Annahme:** Bricht die Verbindung nach dem Senden ab, liest `deliver` den Beleg nach: `sent_on` von heute ergibt Exit 0 mit Warnung, sonst Exit 12. Setzt die API `sent_on` bei `--via pdf` nicht, endet dieser Fall immer mit Exit 12 (sicher, aber unnötig vorsichtig).
- **E2E:** `TestE2E_Carries/07_DeliverPdfSentOn` (Messung, Angebot); `TestE2E_InvoiceFlow` misst dasselbe für eine Rechnung (`sent_on`, `sent_via`, `state`, `invoice_no` nach `invoice deliver --via pdf`).
- **Gemessen 2026-11-04 (Lauf 2):** **nein.** `deliver --via pdf` antwortet 204 ohne Body; danach sind `sent_on`, `sent_via` und `sent_to` `null`, `state` ist `sent`, die Nummer (`A-00001`) ist vergeben. Folge: Bricht die Verbindung bei einem PDF-Versand ab, endet `deliver` immer mit Exit 12; die Meldung sagt jetzt, dass die API bei `--via pdf` kein `sent_on` setzt, und nennt `state`. `state` allein ist kein Beweis (der Beleg kann schon früher versendet worden sein), deshalb bleibt es bei Exit 12.
- **Gemessen Live-Lauf 5, 2026-11-08:** auch bei der Rechnung **nein**: Nach `invoice deliver --via pdf` sind `sent_on` und `sent_via` `null`, `state` ist `unpaid`, die Nummer `R-00001` ist vergeben. Das Angebot zeigte erneut `sent_on`/`sent_via` `null` und `state` `sent`. Die Regel bleibt: Ein abgebrochener PDF-Versand endet mit Exit 12.

### 17. `Accept` beim PDF-Download

- **Offen:** Die Spec nennt den Content-Type des PDFs wörtlich `["application/pdf"]`. Welcher `Accept`-Header liefert das PDF?
- **Annahme:** `Accept: application/pdf, application/json;q=0.9` ergibt 200 mit `application/pdf`; Fehler kommen weiter als JSON.
- **E2E:** `TestE2E_Carries/08_PDFDownloadAccept` (hart).
- **Gemessen 2026-11-04 (Lauf 2):** bestätigt: HTTP 200, `application/pdf`, 29 844 Bytes.

### 18. `price_gross: 0.0` bei `gross: false`

- **Offen:** `invoice create --from-estimate` übernimmt `price_gross` aus dem Angebot; bei Netto-Angeboten ist das 0.0. Nimmt die API das an?
- **Annahme:** Ja.
- **E2E:** `TestE2E_Carries/09_FromEstimatePriceGrossZero` (hart).
- **Gemessen 2026-11-04 (Lauf 2):** bestätigt: `gross` false mit `price_gross` 0 je Position, HTTP 201.

### 19. `voucher pay` mit `value: null` und Bankumsatz

- **Offen:** Nimmt `pay` `value: null` neben `banking_transaction` an?
- **Annahme:** Ja; mit nur `--transaction` sendet das CLI `value` gar nicht.
- **E2E:** `TestE2E_Carries/10_VoucherPayValueNull` (Messung) mit der Bankumsatz-ID 999999999, die es nicht gibt, und ohne Betrag: Die Suite bezahlt nie etwas, ihr Transport lässt für `pay` nur genau diesen Body durch.
- **Gemessen 2026-11-04 (Lauf 2):** Mit `value: null` und der Bankumsatz-ID 999999999 antwortet die API 404 „Couldn't find Banking::Transaction with 'id'=999999999 [WHERE `banking_transactions`.`value` < 0.0]“. Der Body wird also bis zur Suche des Umsatzes verarbeitet (kein 422 wegen `value: null`); gesucht werden nur Ausgaben (`value` < 0). Ob die Zahlung mit einem echten Umsatz gelingt, bleibt offen.

### 20. `voucher pay` mit Betrag und Bankumsatz zusammen — bewusst nicht gemessen

- **Offen:** Was passiert, wenn `value` und `banking_transaction` zusammen kommen (Vorrang, Abweichung, Fehler)?
- **Annahme:** Das CLI sendet beide, wenn beide angegeben sind, und überlässt die Entscheidung der API.
- **E2E:** Bewusst nicht gemessen: Die Messung würde eine echte Zahlung buchen, die sich per API nicht zurücknehmen lässt.

### 21. Dauer an Tagen der Zeitumstellung

- **Offen:** Rechnet die API die Dauer eines Zeiteintrags als Wanduhr-Differenz, auch am Tag der Zeitumstellung (z. B. 01:30–02:30 am 25.10.)?
- **Annahme:** Wanduhr-Differenz; `time log --from 01:30 --duration 2h` ergibt 01:30–03:30, auch an Umstellungstagen.
- **E2E:** `TestE2E_Carries/11_DurationWallClock` (hart) misst nur einen normalen Tag. Der Umstellungstag ist **bewusst nicht gemessen**, sondern nur dokumentiert (Entscheidung zu Task 44).
- **Gemessen 2026-11-04 (Lauf 2):** bestätigt für einen normalen Tag: 09:00–10:30 ergibt `duration` 5400.

### 22. Ende um Mitternacht

- **Frage:** Nimmt die API `ended_at_time` `"00:00"` oder `"24:00"` nach einem Beginn um 23:00 an, und als eine Stunde?
- **Gemessen 2026-11-04 (Lauf 2, über `papierkram api`):** Die API nimmt beide an: 23:00–00:00 und 23:00–24:00 ergeben je `duration` 3600 und `ended_at` 00:00 des Folgetags. Damals lehnte das CLI ein Ende um Mitternacht mit Exit 2 ab (Ruling E2E-Mitternacht, inzwischen ersetzt).
- **Entschieden und umgesetzt (Task 10):** Ein Ende genau um Mitternacht ist erlaubt. `time log --to 24:00` und, bei Beginn nach 00:00, `--to 00:00` enden um Mitternacht am Ende des Datums, ebenso `--from 23:00 --duration 1h`; gesendet wird `ended_at_time` `"24:00"` (eindeutig als Tagesende). `--from 00:00 --to 24:00` ist der ganze Tag (00:00–24:00 live nicht gemessen; gemessen sind nur 23:00–00:00 und 23:00–24:00), `--from 00:00 --to 00:00` bleibt Dauer 0 und `--from 24:00` ungültig (je Exit 2). `time stop` bucht einen Timer, der zwischen 00:00:00 und 00:00:59 des Folgetags stoppt, bis `24:00` des Starttags. Andere Einträge über Mitternacht bleiben Exit 2; der Hinweis nennt die zwei Einträge (bis `24:00` und ab `00:00` am Folgetag). Das Overlay (Abschnitt 14) erlaubt `24:00` nur für `ended_at_time` (POST und PUT), sodass auch `time create|update --ended-at-time 24:00` gesendet wird; `--started-at-time 24:00` ist Exit 5.
- **E2E:** `TestE2E_Carries/12_MidnightEnd` (hart, 22 Credits): `time log --date 2026-06-10 --from 23:00 --to 00:00` und `--to 24:00` müssen mit Exit 0 enden und in ihrer Antwort `duration` 3600 haben; `ended_at` und das gesendete `ended_at_time` stehen als Messung im Log.
- **Gemessen Live-Lauf 5, 2026-11-08 (über `time log`):** bestätigt: `time log --date 2026-06-10 --from 23:00 --to 00:00` und `--to 24:00` senden beide `ended_at_time` `24:00` und ergeben je `duration` 3600, `started_at` 2026-06-10 23:00 und `ended_at` 00:00 des Folgetags.

### 23. Antwort von `GET /info`

- **Offen:** Die Spec hat für `GET /info` kein Response-Schema; `papierkram schema info` zeigt deshalb `"response": null`.
- **Annahme:** Das CLI gibt die Antwort unverändert aus und wertet sie nicht aus; das Restkontingent (`auth status`) kommt aus den Headern. Die Rechnungsvorlagen stehen laut Spec-Beispiel unter `settings.custom_templates.invoices`.
- **E2E:** `TestE2E_Carries/13_InfoStructure` (Messung) loggt die Struktur der Antwort.
- **Gemessen 2026-11-04 (Lauf 2):** `api.version` (String) und `settings.custom_templates.invoices` (Array).

### 24. Leerer Body bei `deliver`

- **Offen:** Die Spec behauptet 201 mit Objekt, live antwortet `deliver` laut papierkram-mcp-server mit 2xx und leerem Body.
- **Annahme:** Ein leerer 2xx-Body ist Erfolg; danach liest das CLI den Beleg nach.
- **E2E:** `TestE2E_DeliverPdfEmptyBody` (2xx und Exit 0 hart, der Body ist Messung).
- **Gemessen 2026-11-04 (Lauf 2):** bestätigt: HTTP 204, kein Content-Type, leerer Body; das CLI endet mit Exit 0 und liest nach.

### 25. Quota-Header

- **Offen:** `X-Remaining-Quota` und `X-Consumed-Quota` fehlen in der Spec.
- **Annahme:** Beide kommen mit jeder Antwort als ganze Zahl; `X-RateLimit-Remaining` wird bewusst ignoriert.
- **E2E:** `TestE2E_QuotaHeadersPresent` (hart).
- **Gemessen 2026-11-04 (erster Live-Lauf):** Beide Header kommen. `X-Consumed-Quota` nennt die Kosten **der einen beantworteten Anfrage** (z. B. 1 für eine Liste oder `GET /info`), keinen laufenden Monatszähler; `X-Remaining-Quota` ist das Restkontingent. Auch eine abgelehnte Anfrage kostet: drei mit 422 abgelehnte Anlagen (2 Angebote, 1 Beleg) kosteten je 50 Credits. `auth status` nennt ihn deshalb „Kosten dieser Abfrage“ (JSON `quota.request_cost`, früher `consumed`/„verbraucht“), `--debug` ergänzt „(Kosten dieser Anfrage)“; einen Monatsverbrauch meldet die API nicht.
- **Gemessen 2026-11-04 (Lauf 2):** erneut beide Header; Restkontingent sinkt je `GET /info` um 1.

### 26. Person `default` als String

- **Offen:** Die Spec typisiert `default` (und `flagged`) der Kontaktperson als String.
- **Annahme:** Das CLI sendet `"true"`/`"false"` als String.
- **E2E:** `TestE2E_PersonDefaultIsString` (hart).
- **Gemessen 2026-11-04 (erster Live-Lauf):** `person create --default` (gesendet als String `"true"`) wurde angenommen und gespeichert; die Antwort liefert `default` als **Boolean** `true` (JSON-Typ `boolean`), wie es das Response-Schema schon sagt. Der Request bleibt laut Spec ein String; ein Rundlauf `person describe -o json | person update --input -` wandelt das Boolean wieder in `"true"`/`"false"` um.

### 27. `company` als Voreinstellung

- **Frage:** Soll `PAPIERKRAM_COMPANY` bzw. `company` aus der Projektdatei `--company` der `person`-Befehle (`person list|describe|create|update|delete`, dort Pflicht) vorbelegen? Spec 9.8 nannte den Schlüssel im Beispiel, aber keinen Befehl, der ihn nutzt.
- **Entschieden und umgesetzt (Task 9):** Ja, mit derselben Mechanik und Priorität wie die übrigen Voreinstellungen: Flag > `PAPIERKRAM_COMPANY` > `company` der Projektdatei (nur im Kontext der Datei, sonst Warnung) > keine; einen Kontext-Default gibt es nicht. Ein genutzter Wert steht als Hinweis auf stderr („Hinweis: Firma 2843 aus PAPIERKRAM_COMPANY“). Ohne Wert bleibt es Exit 2 („Pflichtangabe --company fehlt“, Hinweis auf die Quellen); ein ungültiger Wert in `PAPIERKRAM_COMPANY` ist Exit 2 ohne Anfrage. Die Person-ID bleibt Pflicht. `commands --json` nennt für `--company` `default_from` (`flag`, `env`, `projectfile`) und `env` `PAPIERKRAM_COMPANY`; `required` bleibt `true`.
- **E2E:** Keine API-Frage, daher kein Live-Test; die Tests in `internal/cli` decken alle fünf Befehle ab.

### 28. `document_date` Pflicht beim Anlegen

- **Gemessen 2026-11-04 (erster Live-Lauf):** `POST /income/estimates` ohne `document_date` lehnt die API mit 422 „Datum muss ausgefüllt werden“ ab und berechnet trotzdem die 50 Credits des Anlegens. Die Spec nennt nur `name` und `line_items` als Pflicht.
- **Gemessen 2026-11-04 (Lauf 2):** `estimate create` mit `--document-date` gelingt (zwei Angebote angelegt).
- **Gemessen Live-Lauf 5, 2026-11-08:** Auch `POST /income/invoices` ohne `document_date` lehnt die API mit HTTP 422 „Datum muss ausgefüllt werden“ ab (Exit 5). Das Feld ist also bei Rechnungen ebenso Pflicht wie beim Angebot.
- **Stand des CLI:** Das Overlay ([api/overlay.yaml](../api/overlay.yaml), Abschnitt 16) macht `document_date` im Anlege-Body von Angebot und Rechnung zur Pflicht (auch `null` gilt als fehlend). `estimate create` und `invoice create` ohne `--document-date` (bzw. ohne `document_date` in `--input`) enden mit Exit 5, bevor eine Anfrage gesendet wird. Das Rechnungsdatum kommt bei `invoice create --from-estimate` nie aus dem Angebot, sondern aus `--document-date`; fehlt es, endet der Befehl mit Exit 5, bevor das Angebot gelesen wird.
- **E2E:** `TestE2E_OpenMeasurements/28_InvoiceWithoutDocumentDate` (hart, 0 Credits): `invoice create --name pk-e2e-<lauf-id>-rechnung-ohne-datum --customer <C> --payment-term <PT> --item name=pk-e2e-pos-19,qty=1,price=100,vat=19` ohne `--document-date` muss mit Exit 5 enden und darf keine Anfrage senden. Bis Live-Lauf 5 war der Subtest eine Messung, die die Rechnung an die API schickte (50 Credits). Das Angebot (immer mit `--document-date`) prüft indirekt `TestE2E_DeliverPdfEmptyBody`.

### 29. `invoice update --customer X` leert Kontaktperson und Projekt

- **Gemessen 2026-11-04 (Lauf 2):** Ein Rechnungs-`PUT` mit `customer` ohne `contact_person`/`project` leert beide, auch beim selben Kunden (damals im Rundlauf, der `customer` als `{id}` sendete).
- **Entschieden und umgesetzt (Task 8):** `invoice update --customer X` ohne `--contact-person`/`--customer-project` nutzt das Lesen der Zahlungsbedingung (Spec 9.4a), einmal für beide Zwecke. Ist X der Kunde der Rechnung, sendet es deren Kontaktperson und Projekt als `{id}` unter `customer` mit (Hinweis „Kontaktperson <id> und Projekt <id> der Rechnung mitgesendet (gleicher Kunde; ohne sie leert die API beide)“); bei einem anderen Kunden entfallen sie wie beim Rundlauf (Hinweis „Kunde X statt Y aus der Rechnung: … nicht übernommen“). Ein gesetztes Flag gewinnt. Das Lesen kostet 1 Credit, auch mit `--payment-term`.
- **E2E:** `TestE2E_Carries/14_InvoiceUpdateSameCustomer` (hart, 4 Credits): `invoice describe`, `invoice update <id> --customer <eigener Kunde>`, `invoice describe`; Kontaktperson und Projekt müssen unverändert sein.
- **Gemessen Live-Lauf 5, 2026-11-08:** bestätigt: gesendet wurden `customer{contact_person{id},id,project{id}}` und `payment_term{id}`; Kontaktperson 13 und Projekt 28 blieben.

## Weitere Ergebnisse von Live-Lauf 5

- **Bank:** `bank-connection list` lieferte 1 Eintrag (Schlüssel `{id,name,type}`), `transaction list` 0 Einträge. Ohne Bankumsatz lässt sich `GetTransaction` nicht aufrufen; der Abdeckungsbericht führt die Operation deshalb begründet als „nicht anwendbar“ (kein Bankumsatz im Sandbox-Account, gemessen).
- **Aufräumen:** Stornierte Rechnungen und Angebote lassen sich weder löschen noch erneut stornieren (je Exit 5); die Suite archiviert sie. Ebenso lässt sich der Kunde nicht löschen, solange Belege an ihm hängen (Exit 5, archiviert). Diese Reste wachsen mit jedem Lauf: Vor Lauf 5 fand die Restesuche 6 archivierte bzw. übrige Objekte früherer Läufe.
