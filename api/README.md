# Snapshot der Papierkram-API-Spec

| | |
|---|---|
| Datei | `openapi.json` (unverändert) |
| Quelle | https://demo.papierkram.de/api/v1/api-docs/api/v1/swagger.json |
| Swagger UI | https://demo.papierkram.de/api/v1/api-docs/index.html |
| Abgerufen | 2026-11-03 |
| Format | OpenAPI 3.0.3, `info.version` = `v1` |
| Größe | 499 181 Bytes |
| SHA-256 | `da826bc0bd364c1749ae088d42287b818b99e8c536ca5ced61193f2581e9ccea` |
| Operationen | 89 |

Die Basis-URL in der Spec (`https://demo.papierkram.de/api/v1`) gilt nur für die Demo-Instanz. Echte Accounts verwenden `https://<subdomain>.papierkram.de/api/v1`.

`openapi.json` nie von Hand ändern. Korrekturen gehören in `overlay.yaml` (siehe unten). Zum Aktualisieren des Snapshots die Datei erneut per `curl` herunterladen, diese Tabelle anpassen und den Diff prüfen.

## Bekannte Mängel der Spec

Diese Mängel werden über das Overlay und im Client behandelt, nicht durch Ändern der Datei:

- Keine Operation hat eine `operationId`.
- Alle Request-Bodies sind Inline-Schemas; es gibt kein gemeinsames Fehlerschema und keine Response-Header (die Kontingent-Header `X-Remaining-Quota` / `X-Consumed-Quota` sind nicht dokumentiert).
- Die PDF-Endpunkte verwenden den wörtlichen Content-Type-Schlüssel `["application/pdf"]` und verweisen auf ein `*List`-Schema.
- `deliver` dokumentiert `201` mit einem Objekt als Body; die Live-API antwortet mit 2xx und leerem Body.
- `order_direction` ist als Array aus `asc|desc` typisiert; die API erwartet einen einzelnen Wert.
- `send_via` und mehrere andere Enums enthalten `null`; `difference_reason` enthält `sonstige` doppelt; das Enum der Ausgabekategorie (`category`) hat 99 Einträge, davon 94 eindeutig.
- Die Pfadparameter `id`, `voucher_id` und `company_id` sind teils als `string`, teils als `integer` typisiert.
- 169 Vorkommen von `patternProperties`, das es erst ab OpenAPI 3.1 gibt.
- Die Response-Schemas kombinieren per `allOf` Schemas mit `additionalProperties: false` und lehnen damit jede echte Antwort ab; mehrere Felder sind fälschlich nicht `nullable` oder falsch typisiert. Einige Fehlerbeispiele (422) stehen unter `201`.
- Die Status-Enums (`state`) der Belege sind nicht dokumentiert.

## Overlay (`overlay.yaml`)

`overlay.yaml` ist ein [OpenAPI Overlay 1.0](https://spec.openapis.org/overlay/v1.0.0.html) mit allen Korrekturen am Snapshot. Jede Aktion trägt einen Kommentar, der erklärt, warum sie nötig ist. Die Ziele sind JSONPath-Ausdrücke nach RFC 9535.

Das Overlay

- vergibt `operationId`s für alle 89 Operationen nach dem Schema `<Verb><Ressource>[<Aktion>]` (z. B. `ListInvoices`, `GetCompany`, `DeliverInvoice`, `UploadVoucherDocument`),
- korrigiert PDF-Responses (`application/pdf`, `type: string`, `format: binary`) und die `deliver`-Responses (200/201 ohne Body),
- typisiert `order_direction` als einzelnen String (`asc|desc`),
- entfernt `null` und doppelte Werte aus Enums (die Nullbarkeit bleibt über `nullable` erhalten),
- typisiert die Pfadparameter einheitlich als `integer` (`int64`),
- ergänzt `format: date` an Datumsfeldern der Request-Bodies (`supply_date` bleibt Freitext),
- markiert String-Felder mit boolescher Bedeutung (`x-cli-bool-string`) und Enum-Hinweise (`x-cli-enum-hints`),
- ersetzt `patternProperties` durch OpenAPI-3.0-Konstrukte (`x-cli-key-pattern` bewahrt das Schlüsselmuster),
- passt die Response-Schemas an die aufgezeichneten Beispiele an,
- macht den Dateiteil `file` beim Beleg-Upload (`UploadVoucherDocument`) zur Pflicht,
- verlangt für `started_at_time`/`ended_at_time` der Zeiteinträge das Format `HH:MM` (`pattern`, `x-cli-pattern-format` nennt die Form),
- nennt Sekunden als Einheit von `billable_duration` im Zeiteintrag-Body (aus den Responses abgeleitet) und warnt in der Hilfe von `time create`, dass die API den Wert beim Anlegen durch die Dauer ersetzt (gemessen 2026-11-04),
- macht `document_date` beim Anlegen eines Angebots (`CreateEstimate`) zur Pflicht und verbietet dort `null` (live gemessen: sonst 422 „Datum muss ausgefüllt werden“).

Das Werkzeug `tools/applyoverlay` wendet das Overlay strikt an: Trifft ein Ziel keinen Knoten oder ändert eine Aktion nichts, bricht die Erzeugung ab. Behebt ein neuer Snapshot einen Mangel, fällt die zugehörige Aktion dadurch auf und wird aus dem Overlay entfernt.

### Gepatchte Spec neu erzeugen

```sh
go generate ./internal/spec/
```

Das schreibt `internal/spec/openapi.gen.json` (sortierte Schlüssel, zwei Leerzeichen Einrückung, Zeilenumbruch am Ende). Die Datei wird eingecheckt und in das Binary eingebettet; nie von Hand ändern. Direkter Aufruf:

```sh
go run ./tools/applyoverlay api/openapi.json api/overlay.yaml internal/spec/openapi.gen.json
```

Die Tests in `internal/spec` prüfen, dass der Snapshot unverändert ist, dass die eingecheckte Datei dem Ergebnis des Overlays entspricht und dass die gepatchte Spec mit kin-openapi lädt und validiert. Die CI führt zusätzlich `go generate ./...` aus und verlangt einen leeren Diff.
