# papierkram-cli – Design-Spec

| | |
|---|---|
| Stand | 2026-11-03 |
| Autor | Pascal Krason (Entscheidungen), Claude (Recherche, Ausarbeitung) |
| Status | Freigegeben (2026-11-03) |
| API-Snapshot | [`api/openapi.json`](../../api/openapi.json), SHA-256 `da826bc0…ccea`, siehe [`api/README.md`](../../api/README.md) |
| Nächster Schritt | Implementierungsplan `docs/plans/2026-11-03-papierkram-cli.md` |

Markierungen: **[Pascal]** = von Pascal entschieden. **[Call]** = Entscheidung des Autors, darf in der Umsetzung nur mit Begründung geändert werden. **[Builder]** = bewusst der Umsetzung überlassen.

---

## 1. Zweck

`papierkram` ist ein plattformübergreifendes Kommandozeilenwerkzeug (macOS, Windows, Linux) für die offizielle REST-API von [Papierkram](https://www.papierkram.de) (Buchhaltung, Rechnungen, Angebote, Belege, Projekte, Zeiterfassung). Es soll sich bedienen wie `hcloud`, `gh`, `sentry-cli` und `jira-cli`.

Zwei Zielgruppen, gleich wichtig:

1. **Menschen** im Terminal: lesbare Tabellen, Shell-Completion, Bestätigungen vor folgenreichen Aktionen, Projektdatei mit Standardwerten für die Zeiterfassung.
2. **KI-Agents** (Claude Code, Codex): stabile JSON-Ausgabe, dokumentierte Exit-Codes, strukturierte Fehler, keine Prompts ohne TTY, maschinenlesbare Selbstbeschreibung, Schutzmodus `read_only`. Ein Agent soll damit eigenständig Buchhaltungsaufgaben erledigen können, ohne versehentlich Rechnungen doppelt zu versenden oder das Monatskontingent zu verbrennen.

## 2. Ziele, Nicht-Ziele, Anti-Ziele

**Ziele**
- Jeder der 89 Endpunkte der offiziellen Spec ist über ein eigenes Command nutzbar (Abschnitt 15, per Test erzwungen).
- Bekannte API-Fallen sind strukturell gelöst, nicht nur dokumentiert (Abschnitt 16, jede als Regressionstest).
- Mehrere Papierkram-Accounts parallel über Kontexte, Token im OS-Schlüsselbund.
- Ein statisches Binary pro Plattform, verteilt über Homebrew, Scoop, deb/rpm und GitHub-Releases.

**Nicht-Ziele (v1)**
- MCP-Server-Modus (`papierkram mcp serve`) **[Pascal]** – kommt in v1.x. Die Command-Registry (Abschnitt 5.3) wird so gebaut, dass MCP-Tools daraus generiert werden können.
- Namensauflösung in Flags (`--customer "Mustermann GmbH"`) **[Call]**: kostet Credits, ist mehrdeutig. Flags nehmen IDs; Namen erscheinen nur in Bestätigungszusammenfassungen.
- `$EDITOR`-basiertes Bearbeiten, Offline-Cache, automatische Wiederholung von Mutationen **[Call]**.
- Buchhaltungslogik, die Papierkram selbst nicht anbietet (z. B. Mahnwesen, Auswertungen über die API hinaus). Ausnahme: dünne Komfort-Commands, die nur vorhandene Endpunkte verketten (`invoice create --from-estimate`, `voucher create --file`, `time log`).

**Anti-Ziele** (wäre ein Fehlschlag, selbst wenn alles „funktioniert“)
- Eine Rechnung oder ein Angebot wird doppelt finalisiert oder verschickt, weil das CLI einen Erfolg als Fehler meldet.
- Eine Mail geht an einen Kunden, ohne dass Empfänger, Betreff und Text ausdrücklich angegeben wurden.
- Eingaben gehen still verloren (z. B. `note` statt `notes`).
- Ein Agent bleibt in einem interaktiven Prompt hängen.
- Ein Token erscheint in Logs, `--debug`-Ausgabe, Fehlermeldungen oder Testfixtures.
- Ein Test läuft gegen eine Produktivbuchhaltung.

## 3. Rahmenbedingungen

| Thema | Entscheidung |
|---|---|
| Sprache | Go **[Pascal, Handoff]** – gleiche Muster wie hcloud/gh/jira-cli (cobra), triviales Cross-Compiling, GoReleaser, reifes OpenAPI-Codegen. Kein Performance-Engpass, Rust brächte nichts. |
| Binary | `papierkram` **[Pascal]**. Homebrew, Scoop, deb und rpm installieren zusätzlich `pk`. Der Alias von `mickeyl/pkram` heißt ebenfalls `pk`; README weist darauf hin. |
| Go-Modul | `github.com/Padrio/papierkram-cli` **[Call]** |
| Repo | `Padrio/papierkram-cli`, **öffentlich**, **MIT** **[Pascal]** |
| Paketquellen | eigene Repos `Padrio/homebrew-tap` und `Padrio/scoop-bucket` **[Pascal]** |
| Projektdatei | `.papierkram.toml` in v1 **[Pascal]** |
| E2E | Testaccount legt Pascal an; Suite wird gebaut und läuft, sobald Env-Vars gesetzt sind **[Pascal]** |
| Commits | Conventional Commits, klein, **keine** `Co-Authored-By: Claude`-Zeilen |
| Sprache der Texte | **Deutsch** **[Pascal]**: Hilfetexte, Flag-Beschreibungen, Fehlermeldungen, Hinweise, README, AGENTS.md, Skill, Man-Pages, Command-Referenz. Command- und Flag-Namen bleiben englisch (deutsche Aliase siehe 7.1), JSON-Schlüssel und Fehlercodes (`not_found`, …) bleiben englisch und stabil. Kein i18n-Framework. Code-Kommentare und Commit-Messages englisch. |
| Go-Version | aktuelle stabile Version (lokal 1.27.3; CI liest `1.27.x` aus `.go-version`); `go.mod` auf die vorletzte Minor-Version setzen, damit Distributionen mitkommen **[Call]** |

## 4. API-Fakten (Phase 0, verifiziert 2026-11-03)

Quellen: Live-Spec (siehe `api/README.md`) und Auswertung von [papierkram-mcp-server](https://github.com/Wagner-Emden-IT-Services/papierkram-mcp-server) (MIT, v2.0.4, Commit `66c63c4`), der viele Fallen live gefunden hat. Von dort wird Wissen übernommen, kein Code.

### 4.1 Grundlagen
- Basis-URL `https://<subdomain>.papierkram.de/api/v1`. Auth `Authorization: Bearer <token>`, Token unter `/einstellungen/api` anlegen. Immer `Accept: application/json`.
- 89 Operationen auf 16 Ressourcen, 51 Schemas, 142 von 145 Responses mit Beispielen (nutzbar als Fixtures).
- Listen-Envelope: `{type:"list", page, page_size, total_pages, total_entries, has_more, entries[]}`. Parameter `page`, `page_size` (1–100), `order_by` nur zusammen mit `order_direction` (`asc|desc`).
- Statuscodes: create 201, update und Aktionen 200, delete 204, 404 bei unbekannter ID, 422 bei Validierungsfehlern, 400 nur bei unbekanntem Filterwert (z. B. `billing_state`).
- Fehlerkörper: `{type:"error", error_type:"bad_request"|"not_found"|"unprocessable_entity", message:"<deutsch>", correlation_id:"<uuid>"}`.

### 4.2 Filter je Liste (aus der Spec)
| Liste | Filter |
|---|---|
| `GET /banking/transactions` | `bank_connection_id` |
| `GET /expense/vouchers` | `creditor_id`, `project_id`, `document_date_range_start/_end` (date) |
| `GET /income/estimates`, `/income/invoices` | `company_id`, `project_id`, `document_date_range_start/_end` (date) |
| `GET /projects` | `company_id` |
| `GET /tracker/tasks` | `project_id`, `proposition_id` |
| `GET /tracker/time_entries` | `project_id`, `task_id`, `invoice_id`, `user_id`, `billing_state` (`billed|unbilled|billable|unbillable|archived`), `start_time_range_start/_end` (RFC 3339) |
| alle anderen | nur Pagination und Sortierung |

### 4.3 Kontingent
Kein klassisches Rate-Limit, sondern Monats-Credits (Paket M 10 000, L 20 000); danach 429. Header `X-Remaining-Quota` und `X-Consumed-Quota` (nicht in der Spec dokumentiert, live per E2E bestätigen). Kosten:

| Credits | Requests |
|---|---|
| 50 | `POST /income/invoices`, `POST /income/estimates`, `POST /expense/vouchers` |
| 10 | `POST` auf companies, persons, voucher-documents, propositions, projects, custom_attributes, tasks, time_entries |
| 1 | alles andere |

### 4.4 Request-Bodies
- Alle Bodies inline mit `additionalProperties:false`: **jedes unbekannte Feld → 422**. Felder, die nur gelesen werden (254 Stellen mit `readOnly:true` in den Response-Schemas, z. B. `id`, `customer_no`, `record_state`, `created_at`), müssen vor PUT entfernt werden.
- Bodies verweisen mit verschachtelten Objekten (`customer:{id, contact_person:{id}, project:{id}}`, `task:{id}`, `payment_term:{id}`), Responses liefern flache `*_id` bzw. eingebettete Objekte.
- Typen weichen vom Naheliegenden ab. **String** erwartet bei: `budget_money`, `budget_time` (Projekt), `complete`, `relative_costs`, `flagged` (Aufgabe), `default`, `flagged` (Person), `price`, `vat_rate` (Artikel). `vat_rate` in Positionen: Zahl oder String, Dezimalbruch `0.19`. Responses liefern `vat_rate` als `"19%"` und `supply_date` als `"2026-08-28 13:41:23 +0200"`.
- Pflichtfelder:

| Ressource | Pflicht |
|---|---|
| Company | `name`, `contact_type` (`customer|supplier`) |
| Person | `first_name`, `last_name` |
| Voucher | `name`, `provenance` (`domestic|eu|foreign`), `line_items` (je Position `vat_rate`) |
| Invoice | `name`, `payment_term`, `line_items` |
| Estimate | `name`, `line_items` |
| Proposition | `name`, `article_no`, `time_unit` (`hour|day`), `proposition_type` (`service|product|ecommerce`), `vat_rate` |
| Project | `name`, `customer` |
| CustomAttribute | `attributable_type` (`Contact::Company|Project`), `name`, `data_type` (`string|text|date|integer|decimal|boolean`) |
| Task | `name`, `project` |
| TimeEntry | `entry_date` (`YYYY-MM-DD`), `started_at_time`, `ended_at_time` (`HH:MM`; das Ende auch `24:00`), `task`, `user` |
| deliver | `send_via` (`email|pdf`); bei `email` zusätzlich `email:{recipient, subject, body}` |
| pay | `banking_transaction:{id}` oder `value`; optional `payment_date`, `difference_reason` (`sonstige|mahnung|teilzahlung|skonto|schmaelerung`) |

- Weitere Enums: Projekt `budget_type` (`money|working_time`), `budget_time_unit` (`hour|day`), `color` (`blue|green|lime|pink|purple|orange`); Company `delivery_method` (`pdf|email`); Ausgabekategorie: 94 eindeutige deutsche Kontenbezeichnungen (Spec-Enum hat 99 Einträge inkl. Duplikate und `null`).
- `custom_attributes` (Company, Project): Objekt, Schlüssel = Attribut-ID als String (`^[0-9]+$`), Wert string/int/number/bool/null. Definitionen unter `/settings/custom_attributes`.
- `custom_template:{id}` (Rechnung, Angebot): kein eigener Listen-Endpoint; verfügbare Vorlagen liefert `GET /info` unter `settings.custom_templates.invoices`.

### 4.5 Aktionen und Dateien
- `archive`/`unarchive` an fast jeder Ressource, `cancel` (Rechnung, Angebot, Beleg), `cancel_with_reverse_entry` (Beleg, Generalumkehr), `pay` (Beleg), `deliver` (Rechnung, Angebot). Alle Aktionen außer `deliver` und `pay` sind POST ohne Body.
- `deliver --via pdf` finalisiert und vergibt die Belegnummer ohne Mail. `deliver` **ohne** `email`-Objekt verschickt still die Standardmail an die Standardadresse. Betreff und Text erlauben Platzhalter wie `{{rechnung.rechnungsnummer}}`, `{{angebot.angebotsnummer}}`; fehlerhafte Platzhalter → 422.
- `deliver` antwortet live **2xx mit leerem Body** (Spec behauptet 201 mit Objekt).
- Upload: `POST /expense/vouchers/{voucher_id}/documents`, `multipart/form-data`, Feld `file`, 201. Löschen: `DELETE …/documents/{id}`.
- PDF: `GET …/pdf` an Rechnung, Angebot, Beleg; liefert `application/pdf`.

### 4.6 Serverseitige Regeln (aus Fehlermeldungen in Spec-Beispielen)
- „Kunde muss ein Kunde sein“ (Projekt), „Firma / Kontaktname muss ein Kunde sein“ (Rechnung): Kunde braucht `contact_type=customer`.
- „Lieferant muss ein Lieferant sein“ (Beleg): Kreditor braucht `contact_type=supplier`.
- „Dieser User kann keine Zeiten erfassen!“ (Zeiteintrag).
- „Status kann nicht geändert werden via "cancel"“: ungültiger Statuswechsel kommt als 422.
- `GET /contact/companies` ignoriert `contact_type` als Filter.

### 4.7 Mängel der Spec (Ursache für Overlay, siehe `api/README.md`)
Keine `operationId`s; PDF-Content-Type-Key wörtlich `["application/pdf"]` mit falschem Schema; `deliver`-Response falsch; `order_direction` als Array typisiert; `null` in Enums; doppelte Enum-Werte; Quota-Header fehlen; **Status-Enums (`state`) der Belege sind nicht dokumentiert** (Beispiele zeigen u. a. `draft`, `canceled`, `unpaid`, `partially_paid`, `deleted`, `imported`).

## 5. Architektur

### 5.1 Grundprinzip: spec-getrieben **[Call]**
Die eingebettete Spec (gepatcht per Overlay) ist die einzige Wahrheit für:
- die Flags jedes `create`/`update`-Commands (jedes Skalar-Feld des Request-Schemas wird ein kebab-case-Flag),
- die Typumwandlung der Flag-Werte in den Spec-Typ (`--budget-money 5000` → `"5000"`, `--flagged` → `"true"` bei Aufgaben, `true` bei Rechnungen),
- die lokale Validierung jedes Bodys vor dem Senden (spart Credits, Exit 5 statt 422),
- das Bereinigen von `--input`-Dateien (Abschnitt 6.3),
- `papierkram schema` und den Abdeckungstest.

Damit sind die Typ-Fallen aus Abschnitt 4.4 strukturell gelöst. Neue Felder in einer neuen Spec-Version erscheinen nach `go generate` automatisch als Flags.

### 5.2 Verzeichnisstruktur
```
cmd/papierkram/main.go        Einstieg, ruft internal/cli
internal/cli/                 cobra-Commands (Root, Meta-Commands, Ressourcen-Builder)
internal/registry/            Command-Metadaten (Abschnitt 5.3)
internal/spec/                go:embed der gepatchten Spec, Schema-Lookups, Coercion, Validierung, Prune
internal/api/                 Client-Wrapper: Middleware, Fehler, Pagination, Quota
internal/api/gen/             oapi-codegen-Ausgabe (eingecheckt, nie von Hand ändern)
internal/config/              config.toml, Kontexte, Auflösungsreihenfolge
internal/auth/                Keychain-Zugriff, Datei-Fallback
internal/output/              table/wide/json/yaml, --fields, --jq, TTY-Erkennung, Fehlerausgabe
internal/projectfile/         .papierkram.toml suchen und lesen
internal/timer/               lokaler Timer-State für time start/stop
api/openapi.json              Original-Spec (unverändert)
api/overlay.yaml              OpenAPI-Overlay mit Korrekturen
docs/reference/               aus cobra generierte Command-Referenz
skills/papierkram/SKILL.md    Claude-Code-Skill
```
Handoff-Vorgabe `internal/api`, `internal/config`, `internal/output`, `internal/auth` ist enthalten; `internal/cli` statt Top-Level-`cmd/`-Paket, damit `cmd/` nur `main.go` enthält **[Call]**.

### 5.3 Command-Registry **[Call]**
Eine Go-Tabelle beschreibt jedes API-Command: Ressource, Aktion, Method, Pfad-Template, Aktionsklasse (Abschnitt 8.1), Credit-Kosten, Listen-Filter (Flag → Query-Parameter), Tabellenspalten (table/wide), Beispiele, Kurzhilfe. Aus ihr entstehen:
- die cobra-Commands (über einen generischen Ressourcen-Builder; Sonderfälle registrieren eigene Handler),
- `papierkram commands --json`,
- der Abdeckungstest (jede Spec-Operation ↔ genau ein Registry-Eintrag, beide Richtungen),
- später die MCP-Tools.

### 5.4 Codegen
- `api/overlay.yaml` (OpenAPI Overlay 1.0) ergänzt `operationId`s nach Schema `<Verb><Resource>[<Action>]` (z. B. `ListInvoices`, `DeliverInvoice`, `UploadVoucherDocument`), korrigiert PDF-Content-Type und Schema (`application/pdf`, `format: binary`) und die `deliver`-Response (2xx ohne Body). **[Builder]**: weitere Korrekturen, falls oapi-codegen an `null`-Enums oder `order_direction` scheitert; jede Korrektur im Overlay kommentieren.
- `oapi-codegen` v2 erzeugt Typen und Client nach `internal/api/gen`; Aufruf per `//go:generate`, Tool-Version gepinnt (`go tool`-Direktive in `go.mod`). CI prüft, dass `go generate ./...` keinen Diff erzeugt.
- Der Wrapper nutzt die generierten **Request-Builder** (Pfad, Query) und schickt sie über die eigene Middleware. Rückgabe sind **rohe JSON-Bytes**, damit `-o json` immer die volle Antwort zeigt, auch Felder, die die Spec noch nicht kennt. Typisierte Modelle nur dort, wo Fachlogik Felder liest (deliver-Nachlesen, `--from-estimate`, Timer, Zusammenfassungen). **[Call]**

### 5.5 HTTP-Client
- Header: `Authorization: Bearer …`, `Accept: application/json`, `User-Agent: papierkram-cli/<version> (<os>/<arch>)`; `Content-Type` nur bei Body.
- Timeout 30 s, überschreibbar per `--timeout` und Kontext-Option. Uploads (`voucher upload`, Upload-Schritt von `voucher create --file`) nutzen ohne `--timeout` je Versuch `max(wirksames Zeitlimit aus Kontext bzw. Standard 30 s, 120 s)` (`uploadTimeout`), mit `--timeout` genau diesen Wert; umgesetzt über eine Client-Kopie (`(*api.Client).WithTimeout`) nur für den Upload-Aufruf. Alle anderen Anfragen, auch das Anlegen bei `voucher create --file`, behalten das wirksame Zeitlimit.
- **Leerer 2xx-Body = kein Inhalt** (nicht nur bei 204), nie ein Parse-Fehler.
- **Antwortgröße höchstens 64 MiB** (`api.MaxResponseSize = 64 << 20`); gelesen wird `io.LimitReader(Body, MaxResponseSize+1)`. Ist die Antwort länger: Bei einem Nicht-2xx-Status entscheidet der Status wie sonst (Body gekürzt, Code nach Status, eine GET-5xx wird wie sonst wiederholt). Bei 2xx auf GET/HEAD Exit 1 (`error`) „Antwort größer als 64 MiB, abgebrochen“, Hinweis „Antwort zu groß für das CLI; Ergebnis nicht verwertbar“, keine Wiederholung; nur bei `list` (auch `--all`) „mit --limit kleinere Seiten anfordern“, bei `api` mit GET auf eine Liste (auch `--paginate`) „mit kleinerem page_size kleinere Seiten anfordern (z. B. -f page_size=25)“ (Ruling T15-A, gesetzt im CLI-Handler); bei 2xx auf POST/PUT/DELETE Exit 1 „Antwort größer als 64 MiB; die Änderung wurde ausgeführt“, Hinweis „NICHT wiederholen; Ergebnis per describe/list prüfen“. `http_status` ist der 2xx-Status, die Ursache `api.ErrResponseTooLarge`. Eine gekürzte Antwort wird nie weitergegeben (`pdf -O` speichert nichts). Mehrstufige Befehle behandeln sie wie eine angekommene Änderung: `deliver` behält den Hinweis, `voucher create --file` endet nach zu großer Antwort auf das Anlegen mit Exit 11 ohne Beleg-ID, auf das Hochladen mit Exit 11 „Ergebnis des Hochladens unbekannt“, `time stop` lässt den Timer laufen; der Eintrag ist angelegt, deshalb endet der Hinweis mit „Der Timer läuft weiter, der Eintrag wurde angelegt: mit papierkram time list prüfen, dann den Timer mit papierkram time discard verwerfen“ (ohne „sonst erneut papierkram time stop“ wie bei Exit 12). Ist das Lesen von `invoice update` (9.4a) zu groß, wird nichts geändert, und der Hinweis nennt zusätzlich „Zahlungsbedingung per --payment-term <id> angeben (papierkram payment-term list)“ bzw., wenn nur die Referenzen gelesen wurden, deren Flags (wurde für beides gelesen, beides). `--debug` zeigt die Kürzung („gekürzt nach 64 MiB“).
- Quota-Header werden bei jeder Antwort gelesen und im Ergebnis mitgeführt (für `--debug`, `auth status`, Warnungen).
- Wiederholung **nur** bei GET nach Netzwerkfehler oder 5xx: zweimal mit exponentiellem Backoff. Nie bei POST/PUT/DELETE (Credits, Doppelbuchung), nie bei 429 (Monatskontingent). **[Call]**
- `--debug`: Request-Zeile, Header (Token ersetzt durch `Bearer ***`), Body (gekürzt auf 4 KB), Status, Dauer, Quota auf stderr. Upload-Bodies und PDFs nur als Größe.
- Fehler-Mapping in typisierte Fehler mit Exit-Code, Code, Nachricht (API-`message`), Hinweis und `correlation_id` (Abschnitt 10.3).

## 6. Eingabe bei create und update

### 6.1 Flags
- Skalar-Felder des Request-Schemas → Flags in kebab-case (`--document-date`, `--budget-money`). Enums werden in Hilfe und Completion angeboten.
- Referenzen als ID-Flags: `--customer 12` → `customer:{id:12}`, `--contact-person 7` → `customer.contact_person:{id:7}`, `--customer-project 3` → `customer.project:{id:3}` (Rechnung/Angebot), `--payment-term 4`, `--creditor 9`, `--project 5`, `--task 8`, `--user 1`, `--proposition 2`, `--custom-template 249`, `--default-proposition 2`, `--team-member 1` (wiederholbar). **[Builder]** genaue Benennung je Ressource, Regel: Flagname = Feldname in kebab-case.
- Billing-Block (Rechnung/Angebot): `--billing-company`, `--billing-street`, … (ein Flag pro Feld von `billing`).
- Custom Attributes: `--attr <id>=<wert>` (wiederholbar); Wert wird gemäß `data_type` der Attributdefinition umgewandelt, wenn bekannt, sonst als String gesendet. **[Call]**
- Datums-Flags akzeptieren `YYYY-MM-DD`, zusätzlich `today`/`heute`, `yesterday`/`gestern`. **[Call]**

### 6.2 Positionen
- `--item key=value,key=value` (wiederholbar). Schlüssel: Feldnamen der Positions-Schemas plus Kurzformen `qty`→`quantity`, `vat`→`vat_rate`. Werte mit Komma in Anführungszeichen (`name="Beratung, vor Ort"`). **[Builder]**: Parser mit Escape-Regeln und Tests.
- `--items file.json|-`: JSON-Array von Positionen.
- **VAT-Normalisierung** für jede `vat_rate` (Position und Artikel): `19`, `19%`, `"19"`, `0,19`, `0.19` → `0.19`; `7` → `0.07`; `0` → `0`. Zahlen > 1 werden durch 100 geteilt, **auch wenn sie als Zahl kommen** (Fehler im MCP-Server). Nicht interpretierbare Werte → Exit 5. Beim Artikel wird das Ergebnis wegen des String-Typs als `"0.19"` gesendet. Ob die API `0.19` in jeder Position und beim Artikel korrekt als 19 % verbucht, prüft die E2E-Suite.
- Ausgabekategorie (`category`): Eingabe muss exakt einem Enum-Wert entsprechen; bei Abweichung Fehler mit den drei ähnlichsten Werten. Completion bietet alle 94 Werte an. Das Enum kommt aus der Spec, nicht aus dem MCP-Server.
- Ein leeres `line_items`-Array wird gesendet (bei `update` nur mit `--replace-items`, 6.3a), nie still verworfen.

### 6.3 `--input` und Bereinigung
- `--input file.json|-` liefert den Body; Flags überschreiben einzelne Felder daraus.
- **Prune-to-schema**: Der Body wird rekursiv gegen das Request-Schema abgeglichen. **Das Request-Schema hat Vorrang** **[Call]**: Die Spec markiert in den Response-Schemas auch schreibbare Felder als `readOnly` (z. B. alle Positionsfelder in `InvoiceWithDetailedEmbeds`, `sent_on`, Person-`flagged`, `billable_duration`); `readOnly` allein ist daher kein Grund zum Entfernen.
  - Felder, die im Request-Schema vorkommen, bleiben erhalten (und werden typumgewandelt, z. B. Position-`vat_rate` `"19%"` → `0.19`; ein Wahrheitswert nimmt wie das Flag `true`/`false`/`ja`/`nein`/`1`/`0` als Text und die JSON-Zahlen `0` und `1` an, andere Zahlen → Exit 5).
  - Felder, die nicht im Request-Schema, aber im Response-Schema der Ressource vorkommen (z. B. `id`, `customer_no`, `created_at`, `customer.name`, `line_items[].total_net`), werden entfernt; ein Hinweis auf stderr nennt sie.
  - Jedes Feld, das in keinem der beiden Schemas vorkommt, führt zu Exit 5 mit Vorschlag (`unbekanntes Feld "note" – meintest du "notes"?`). Kein stilles Verwerfen.
  - Wechselt der Kunde, gehören übernommene Referenzen nicht mehr dazu: Nennt `--customer` eine andere ID als `customer.id` aus `--input`, werden die von Prune übernommenen `contact_person`/`project` wieder entfernt (Hinweis „Kunde X statt Y aus --input: … nicht übernommen“), wie bei `--from-estimate`; eine Referenz mit eigenem Flag (`--contact-person`, `--customer-project`) bleibt. Hat `customer` in `--input` keine lesbare `id` (fehlt, `null`, keine ganze Zahl), ist das kein Kundenwechsel: Die Referenzen bleiben, kein Hinweis. Ändert jemand `customer.id` per `jq` im Input selbst, erkennt das CLI den Wechsel nicht: Dann `.contact_person` und `.project` im Input mit löschen (`jq '.customer.id=99 | del(.contact_person, .project)'`) oder per Flag setzen. Der Hinweis „Referenzen übernommen“ nennt nur, was danach gesendet wird.
  - Flache bzw. top-level Response-Felder (`company_id`, `contact_person`, `project` auf oberster Ebene) werden grundsätzlich **nicht** in die verschachtelten Request-Referenzen übersetzt **[Call]**; sie werden als Response-Felder entfernt. Referenzen ändert man per Flag (`--customer`, `--contact-person`). Ausnahmen: `--from-estimate` (9.4) und, seit Live-Lauf 2 (2026-11-04), eine Response-Referenz (Objekt mit `id`), die genau ein Request-Objekt derselben Ebene unter gleichem Namen als Referenz führt: Sie wird als `{id}` dorthin übernommen, wenn dieses Objekt im Body steht und sie noch nicht nennt (Hinweis „Referenzen übernommen“). Das betrifft nur `contact_person` und `project` von Rechnung und Angebot (→ `customer.contact_person`, `customer.project`); gemessen: Fehlen sie im `PUT` einer Rechnung, leert die API beide.

### 6.3a Update-Semantik **[Call]**
- `PUT` gilt als Teil-Update: Es werden nur die angegebenen Felder gesendet. Der MCP-Server arbeitet live so; die E2E-Suite bestätigt es für jede Ressource außer der Rechnung: Ein Rechnungs-`PUT` ohne `payment_term` stellt die Zahlungsbedingung auf den Standard des Accounts um und ändert `due_date` (gemessen Live-Lauf 3, 2026-11-04). `invoice update` sendet deshalb ohne `--payment-term` und ohne `payment_term` in `--input` die aktuelle Zahlungsbedingung mit und liest sie dafür vorher (Ruling E2E-A, siehe 9.4a). `papierkram api PUT` sendet weiter nur, was angegeben ist.
- `line_items` haben im Request keine `id`; das Senden von `line_items` ersetzt deshalb **alle** Positionen. `update` mit `--item`/`--items` verlangt darum zusätzlich `--replace-items`, sonst Exit 2 mit Erklärung. Positionen ergänzt man per `describe -o json | jq … | update --input - --replace-items`.
- Ein leeres `line_items` (R18) wird nur mit `--replace-items` gesendet.
- So funktioniert `papierkram invoice describe 12 -o json | jq '.name="Neu"' | papierkram invoice update 12 --input -`.

### 6.4 Validierung vor dem Senden
Der fertige Body wird mit kin-openapi gegen das Request-Schema validiert (Pflichtfelder, Typen, Enums, `additionalProperties`). Fehler → Exit 5 ohne Request. `--dry-run` validiert ebenfalls.

## 7. Command-Referenz

### 7.1 Ressourcen
Englische Nomen wie in der API, deutsche Aliase **[Call]**. Vollständige Endpunktzuordnung in Abschnitt 15.

| Command (Aliase) | Aktionen und Besonderheiten |
|---|---|
| `company` (`firma`, `kontakt`) | `list` (`--type customer\|supplier` clientseitig, Abschnitt 9.7), `describe`, `create`, `update`, `delete`, `archive`, `unarchive` |
| `person` (`kontaktperson`) | `list`, `describe`, `create`, `update`, `delete`; jeweils `--company <id>` Pflicht |
| `invoice` (`rechnung`) | `list` (`--company --project --from --to`), `describe`, `create` (+ `--from-estimate`), `update`, `delete`, `archive`, `unarchive`, `cancel`, `deliver` (Alias `send`), `pdf` |
| `estimate` (`angebot`) | wie `invoice`, ohne `--from-estimate` |
| `voucher` (`beleg`, `expense`) | `list` (`--creditor --project --from --to`), `describe`, `create` (+ `--file`), `update`, `delete`, `archive`, `unarchive`, `cancel`, `reverse`, `pay`, `pdf`, `upload`, `document delete` |
| `proposition` (`artikel`, `leistung`) | `list`, `describe`, `create`, `update`, `delete`, `archive`, `unarchive` |
| `payment-term` (`zahlungsbedingung`) | `list`, `describe` |
| `project` (`projekt`) | `list` (`--company`), `describe`, `create`, `update`, `delete`, `archive`, `unarchive` |
| `task` (`aufgabe`) | `list` (`--project --proposition`), `describe`, `create`, `update`, `delete`, `archive`, `unarchive` |
| `time` (`zeit`, `time-entry`) | `list` (`--project --task --invoice --user --state --since --until`), `describe`, `create`, `update`, `delete`, `archive`, `unarchive`, `log`, `start`, `stop`, `status`, `discard` |
| `custom-attribute` (`datenfeld`) | `list`, `describe`, `create`, `update`, `delete`, `archive`, `unarchive` |
| `bank-connection` (`bankverbindung`) | `list`, `describe` |
| `transaction` (`umsatz`) | `list` (`--bank-connection`), `describe` |
| `user` (`benutzer`) | `list`, `describe` |
| `info` | Account-Info inkl. Rechnungsvorlagen |

Gemeinsame Flags für `list`: `--page`, `--limit` (= `page_size`, 1–100, Standard 25), `--all`, `--max-pages`, `--sort <feld>` und `--desc` (setzen immer beide Parameter `order_by` + `order_direction`) **[Call]**. Filter-Flags mappen 1:1 auf Query-Parameter aus Abschnitt 4.2 (`--from`/`--to` → `document_date_range_*`, `--since`/`--until` → `start_time_range_*` in RFC 3339 mit lokaler Zeitzone, `--state` → `billing_state`).

### 7.2 Meta-Commands
| Command | Zweck |
|---|---|
| `auth login [--context] [--subdomain] [--token-stdin] [--insecure-storage]` | Token speichern, per `GET /info` prüfen |
| `auth logout`, `auth status` | Token entfernen; Status inkl. Kontext, Subdomain, Token-Quelle, Restkontingent |
| `context create\|use\|list\|active\|delete\|rename` | wie `hcloud context` |
| `local init\|show` | Projektdatei anlegen (interaktiv nur mit TTY) bzw. wirksame Werte mit Herkunft anzeigen |
| `api <METHOD> <path> [-f key=value] [-F key=@file] [--input file.json] [--paginate]` | Escape-Hatch wie `gh api`; Body wird **nicht** bereinigt, nur bei `--validate` geprüft. Schutzregeln gelten trotzdem **[Call]**: Method + Pfad werden in der Registry nachgeschlagen; deren Klasse bestimmt Bestätigung/`--yes`, `read_only`-Sperre und `--dry-run`. Für `…/deliver` gilt die `email`-Pflicht aus 9.1 (Body mit `send_via:"email"` ohne vollständiges `email`-Objekt → Exit 5). Unbekannte Pfade gelten bei GET als `read_only`, sonst als `destructive`. |
| `commands --json` | alle Commands mit Flags, Typen, Pflicht, Enums, Klasse, Credits, Beispielen; je Flag optional `default` (pflag-Standardwert wie „(Standard: …)“ in der Hilfe, ausgelassen beim Nullwert des Typs), `minimum`/`maximum` (eingeschlossene Grenzen aus denselben Konstanten wie die Prüfung: `--limit` 1–100, `--page` 1, `--max-pages` 1 bei `list` und `api`; zusätzlich `minimum`/`maximum` des Spec-Schemas eines Feld- oder Filter-Flags, die eingebettete Spec nennt heute keine). `--timeout` (positive Dauer, untere Grenze ausgeschlossen) und ID-Flags (Typ `id`, ≥ 0) tragen keine Grenzen. Additiv, `version` bleibt 1 |
| `schema <resource> [action]` | Request-/Response-Schema als JSON |
| `completion bash\|zsh\|fish\|powershell` | Shell-Completion inkl. dynamischer IDs |
| `version` | Version, Commit, Build-Datum, Go-Version; `-o json` |

Globale Flags: `--context`, `-o/--output`, `--fields`, `--jq`, `--debug`, `--timeout`, `--no-input`, `--yes/-y`, `--dry-run` (nur Mutationen), `--no-color`.

### 7.3 Dynamische Completion
IDs werden per List-Request vervollständigt (Seite 1, `page_size` 100, Anzeige `id<TAB>name`). Jede Completion kostet 1 Credit; Ergebnis wird pro Kontext und Ressource 60 s im Cache-Verzeichnis gehalten. Abschaltbar per Kontext-Option `completion_lookup = false`. **[Call]**

## 8. Schutzmechanismen

### 8.1 Aktionsklassen
| Klasse | Aktionen | Bestätigung |
|---|---|---|
| `read_only` | list, describe, pdf, info, alle Meta-Lesecommands | nein |
| `create` | create, upload, time log/stop | nein (Zusammenfassung in TTY, siehe 9.6) |
| `update` | update | nein |
| `archive` | archive, unarchive | nein, umkehrbar |
| `destructive` | delete, cancel, reverse, pay, document delete | ja |
| `send` | deliver | ja, mit Zusammenfassung |

`pay` gilt als `destructive`, weil eine Zahlung nicht per API zurückgenommen werden kann. **[Call]** `archive` an Rechnungen ist **nicht** destructive (Fehler im MCP-Server).

### 8.2 Bestätigung
- Mit TTY: Zusammenfassung (Ressource, ID, Name, Betrag, bei `send` Empfänger und Betreff), Eingabe `ja`/`yes` bzw. bei `delete` die ID. **[Builder]** Formulierung.
- Ohne TTY oder mit `--no-input` und ohne `--yes`: Abbruch mit Exit 8 und Hinweis auf `--yes`.

### 8.3 Schreibschutz
Zwei Stufen **[Pascal]**:
- **Harter Schreibschutz per Umgebung** (gegen einen Agent, der sich herausarbeiten will): Ist `PAPIERKRAM_READ_ONLY=1` gesetzt (vom Menschen beim Start des Agents), blockiert das CLI alle Klassen außer `read_only`, egal welcher Kontext über `--context`, `PAPIERKRAM_CONTEXT`, Projektdatei oder `active_context` gewählt ist. Auch `papierkram api` mit Nicht-GET. Weder Flag, Projektdatei noch Config können das aufheben. Zusätzlich gesperrt: `context create|use|delete|rename`, `auth login|logout`, `local init` (Schreiben der Config/Projektdatei). Exit 9.
- **Kontext-Option `read_only = true`** (gegen Versehen): blockiert Schreibzugriffe im gewählten Kontext; ein Wechsel per `--context` bleibt möglich.
- `auth status` und `--debug` zeigen, welche Stufe aktiv ist. AGENTS.md empfiehlt für Analyse-Agents die Env-Variable.

### 8.4 `--dry-run`
Bei jeder Mutation (inkl. `api` mit Nicht-GET): validiert lokal, zeigt Method, URL, Body (bei Upload: Dateiname, Größe, MIME-Typ) und Credit-Kosten; sendet nichts; Exit 0. Mit `-o json`: `{"dry_run":true,"method":…,"url":…,"body":…,"credits":…}`. Keine Bestätigung nötig.

### 8.5 Credits
- Kosten pro Command stehen in der Registry und in `commands --json`.
- `auth status` und `--debug` zeigen `X-Remaining-Quota`/`X-Consumed-Quota`.
- `--all`: liest Seite 1, berechnet `total_pages`; ab 10 Seiten Warnung auf stderr, mit TTY zusätzlich Bestätigung. `--max-pages` begrenzt hart.
- Sinkt `X-Remaining-Quota` unter 100, Warnung auf stderr (einmal pro Aufruf). **[Call]**
- 429 → Exit 6, Hinweis: Monatskontingent erschöpft, Paket prüfen.

## 9. Fachliche Sonderfälle

### 9.1 `invoice|estimate deliver <id> --via email|pdf`
- `--via` ist Pflicht, kein Default.
- `--via email` verlangt `--to`, `--subject`, `--body` (oder `--body-file`); alle drei werden als `email:{recipient,subject,body}` gesendet. Fehlt eines → Exit 2, bevor irgendetwas gesendet wird.
- Klasse `send`: Bestätigung oder `--yes`.
- Leerer 2xx-Body = Erfolg. Danach wird der Beleg per `describe` nachgelesen; Ausgabe: `id`, `invoice_no`/`estimate_no`, `state`, `sent_on`, `sent_via`, `sent_to`.
- Scheitert nur das Nachlesen: Exit 0, Ergebnis `{"id":…,"delivered":true,"warnings":["Versand erfolgreich, Nachlesen fehlgeschlagen: … – NICHT erneut ausführen"]}`.
- Antwortet die API auf den Versand mit 5xx oder einem unerwarteten Status, lautet der Hinweis „Papierkram-Fehler – der Versand kann trotzdem erfolgt sein: vor Wiederholung per describe prüfen: papierkram <ressource> describe <id>“ (auch bei `api` auf den deliver-Pfad). Ein lokaler Fehler, bevor die Anfrage gesendet wurde (z. B. eine nicht lesbare Basis-URL), behält Meldung und Hinweis ohne diesen Zusatz.

### 9.2 `… pdf <id> [-O datei.pdf]`
Ohne `-O` und mit TTY auf stdout: Fehler mit Hinweis (kein Binärmüll ins Terminal). `-O -` schreibt nach stdout. Ohne `-O` und ohne TTY: nach stdout. Standard-Dateiname bei `-O` ohne Wert ist nicht vorgesehen. **[Call]** Ausgabe auf stderr bzw. bei `-o json`: `{"path":…,"bytes":…}`.

### 9.3 `voucher upload <voucher-id> <datei>` und `voucher create --file`
- Upload als `multipart/form-data`, Feld `file`, MIME-Typ aus Dateiendung. Erlaubte Typen **[Builder]**: per E2E ermitteln, bis dahin keine lokale Einschränkung.
- Zeitlimit des Uploads (5.5): ohne `--timeout` je Versuch mindestens 120 s (ein größeres `timeout` des Kontexts gilt), mit `--timeout` genau dieser Wert. Bei Zeitüberschreitung endet der Hinweis mit „größere Dateien brauchen ein höheres --timeout (Uploads ohne --timeout: 120 s)“.
- `voucher create … --file beleg.pdf`: erst Beleg anlegen (50 Credits), dann hochladen (10 Credits). Scheitert der Upload, bleibt der Beleg bestehen; **Exit 11 (`partial`)**, Ausgabe enthält die Beleg-ID, `"completed":["create"]`, `"failed":["upload"]` und den Befehl zum Nachholen (`papierkram voucher upload <id> <datei>`). Kein automatisches Löschen. Gleiches Muster für `invoice create --from-estimate` (Lesen gescheitert → nichts angelegt, normaler Fehler) und `time stop` (Anlegen gescheitert → Timer bleibt erhalten, normaler Fehler). **[Call]**

### 9.4 `invoice create --from-estimate <id>`
Liest das Angebot und baut daraus den Rechnungs-Body: `name`, `description`, `customer` (`id`, ggf. `contact_person`, `project`), `billing`, `gross`, alle Positionen (`name`, `description`, `quantity`, `unit`, `price`, `price_gross`, `discount_*`, `proposition:{id}` falls gesetzt, `type_of`, `vat_rate` aus `"19%"` normalisiert). `--payment-term` bleibt Pflicht (Angebote haben keine Zahlungsbedingung). Alle anderen Flags überschreiben. Leere Werte (null) des Angebots, die die Rechnung nicht annimmt, werden weggelassen und im Hinweis „null ignoriert (Feld bleibt unverändert): …“ genannt, außer ein Flag setzt das Feld (bzw. `--item`/`--items` die Positionen; ein Referenz-Flag wie `--customer` setzt nur seine Referenz und deren `id`, nicht `customer.contact_person.id`). Mit `--dry-run` sieht man den Body vor dem Anlegen (50 Credits). **[Builder]**: genaue Feldliste gegen `EstimateWithDetailedEmbeds` und das Rechnungs-Request-Schema abgleichen; Felder, die nicht übertragbar sind, in der Hilfe nennen.

### 9.4a `invoice update <id>`: Zahlungsbedingung erhalten (Ruling E2E-A)
Ohne `--payment-term` und ohne `payment_term` im fertigen Body (nach `--input` und Flags; ein `null` gilt als fehlend) liest `invoice update` die Rechnung vorher (`GET /income/invoices/{id}`, 1 Credit) und sendet `payment_term: {id: <aktuelle payment_term.id>}` mit; ein Hinweis auf stderr nennt die ID. Das Lesen läuft nach Schreibschutz-Gate, Flag- und Body-Prüfung (Fehler dort kosten kein Lesen) und wie bei `--from-estimate` (9.4): derselbe `describe`-Eintrag, ein gescheitertes Lesen behält seinen Code und ändert nichts, eine Antwort ohne `payment_term.id` endet mit Exit 1 (nichts gesendet, Hinweis auf `--payment-term`). `--dry-run` liest ebenfalls und weist das Lesen als `read` aus; `credits` ist dann 2, mit `--payment-term` 1. `commands --json` nennt für `invoice update` 2 Credits. Nur Rechnungen: beim Angebot zeigte Lauf 3 keine solche Änderung.

**Kontaktperson und Projekt (offener Punkt 29):** Ein Rechnungs-`PUT`, dessen `customer` `contact_person` oder `project` nicht nennt, leert beide, auch beim selben Kunden (gemessen Lauf 2). Ist `--customer` gesetzt und fehlt im fertigen Body `customer.contact_person` oder `customer.project`, ohne dass ihr Flag (`--contact-person`, `--customer-project`) gesetzt ist, nutzt `invoice update` dasselbe Lesen: Gelesen wird einmal, wenn die Zahlungsbedingung oder eine Referenz fehlt. Das gelesene Dokument wird wie `--input` bereinigt (`spec.Prune` gegen Request- und Response-Schema, die Referenzen der Antwort wandern unter `customer`; ohne Hinweise auf entfernte Felder, es ist keine Eingabe) und wie bei `--from-estimate` und dem Rundlauf um die Referenzen eines anderen Kunden gekürzt. Gleicher Kunde: jede fehlende Referenz der Rechnung wird als `{id}` mitgesendet, Flags und `--input` gewinnen; Hinweis „Kontaktperson <id> und Projekt <id> der Rechnung mitgesendet (gleicher Kunde; ohne sie leert die API beide)“ (nur die vorhandenen). Anderer Kunde: nichts übernommen, Hinweis „Kunde X statt Y aus der Rechnung: … nicht übernommen (setzen per …)“; er nennt nur die Referenzen, die wirklich entfallen, nicht die aus `--input`, die gesendet werden. Der Body wird danach erneut validiert. Mit `--payment-term` und `--customer` ohne die Referenz-Flags wird ebenfalls gelesen (`--dry-run`: `read`, `credits` 2); ohne `--customer`, mit beiden Referenz-Flags oder wenn `--customer` einen anderen Kunden nennt als `customer.id` aus `--input` nicht (die Referenzen aus `--input` entfallen dann nach 6.3, und meist gehören die Referenzen der Rechnung ebenfalls zum alten Kunden; der Rundlauf mit Kundenwechsel kostet so viel wie ohne das Lesen, der Hinweis „Kunde X statt Y aus --input“ erscheint einmal; Ruling T8-A). Randfall: Stammt `--input` von einer anderen Rechnung und nennt `--customer` den eigenen Kunden der Rechnung, werden die Referenzen der Rechnung ebenfalls nicht gelesen bzw. übernommen (fehlt `payment_term` in `--input`, wird für die Zahlungsbedingung trotzdem gelesen), und der `PUT` leert Kontaktperson und Projekt der Rechnung; sie lassen sich per `--contact-person`/`--customer-project` setzen. Ist die Antwort nicht verwendbar (kein JSON-Objekt, ein Feld in keinem Schema), endet der Befehl mit Exit 1 „… nichts geändert“. Live-Test: `TestE2E_Carries/14_InvoiceUpdateSameCustomer`.

### 9.5 `voucher pay <id>`
Flags `--amount` (→ `value`), `--transaction` (→ `banking_transaction:{id}`), `--date` (→ `payment_date`), `--reason` (→ `difference_reason`). Mindestens `--amount` oder `--transaction` Pflicht (Exit 2).

### 9.6 Zeiterfassung: `time log`, Timer, Projektdatei
- `time log [--task <id|alias>] [--date] (--from HH:MM --to HH:MM | --duration 1h30m [--from]) [-m Kommentar] [--unbillable]` legt einen Zeiteintrag an. `task`, `user` kommen aus Flag > Env > Projektdatei > Kontext; fehlen sie → Exit 2 mit Hinweis auf `papierkram local init`. Ohne `--from` bei `--duration` endet der Eintrag jetzt. Body: `entry_date`, `started_at_time`, `ended_at_time` (`HH:MM`, Ende um Mitternacht `24:00`), `task:{id}`, `user:{id}`, `comments`, `unbillable`.
- Mit TTY zeigt `time log` vor dem Senden eine Zusammenfassung mit aufgelösten Namen (Firma, Projekt, Aufgabe, Nutzer; je 1 Credit, gecacht wie Completion) und fragt nach (Enter = ja). Ohne TTY wird ohne Nachfrage gesendet (Klasse `create`). `--yes` überspringt die Nachfrage. **[Call]**
- Ein Ende genau um Mitternacht ist erlaubt (offener Punkt 22, entschieden; ersetzt Ruling E2E-Mitternacht): `--to 24:00` und, bei Beginn nach 00:00, `--to 00:00` enden um Mitternacht am Ende des Datums, ebenso `--from` plus `--duration` bis genau Mitternacht (`--from 23:00 --duration 1h`); gesendet wird `ended_at_time` `"24:00"` (eindeutig als Tagesende; die API nimmt `00:00` und `24:00` gleich an, `duration` 3600 für 23:00–24:00, gemessen Lauf 2). `24:00` ist immer das Tagesende (`--from 00:00 --to 24:00` ist der ganze Tag), `--from 00:00 --to 00:00` bleibt Dauer 0 (Exit 2), `--from 24:00` bleibt ungültig (Exit 2). Nach einem gebuchten `--to 00:00` steht auf stderr „Hinweis: Ende 00:00 als 24:00 (Mitternacht) gebucht“ (nicht bei `--to 24:00`, `--duration` und `--dry-run`; `time stop` nennt `24:00` schon in „Timer gestoppt: …“). `time stop` bucht einen Timer, der in der ersten Minute des Folgetags (00:00:00–00:00:59) stoppt, bis `24:00` des Starttags. Alle anderen Einträge über Mitternacht werden abgelehnt (Exit 2) **[Call]**, weil der Body nur ein Datum kennt; der Hinweis nennt die zwei Einträge (bis `24:00` und ab `00:00` am Folgetag). Das Overlay (Abschnitt 14) erlaubt `24:00` nur für `ended_at_time`, sodass `time create|update --ended-at-time 24:00` gesendet wird; eine eigene Zeitraumprüfung haben `time create|update` nicht.
- Timer: `time start [--task] [-m] [--unbillable]` speichert Startzeit und Parameter in `os.UserCacheDir()/papierkram/timer-<kontext>.json`; `time status` zeigt laufende Dauer; `time stop [-m]` ruft intern `time log` auf und löscht den State erst nach Erfolg; `time discard` verwirft. Läuft schon ein Timer → Exit 7. Kein serverseitiger Timer.
- `time start --unbillable` (Task 12, Parität zu `time log --unbillable`): dasselbe Flag (Wahrheitswert, ohne Wert `true`, auch `ja`/`nein`); der Wert wird beim Start mit dem Feld `unbillable` des Request-Schemas geprüft (`spec.CoerceFlag`, ungültig → Exit 5, Timer nicht gestartet) und als `unbillable` (bool) im State gespeichert; ohne Flag fehlt das Feld. `time stop` sendet `unbillable` genau dann, wenn es im State steht (als Text `true`/`false` durch denselben Body-Bau wie `time log`); alte Timer-Dateien ohne das Feld bleiben gültig und senden keins. `time stop` hat kein `--unbillable`: Die Abrechenbarkeit steht beim Start fest, nur der Kommentar wird am Ende oft ergänzt. `time start`/`time status` zeigen `unbillable` in JSON (bool, `false` ohne Angabe) und mit `-o wide` als Spalte `NICHT ABRECHENBAR` (`ja`/leer); die Befehle zum Buchen von Hand in den Hinweisen von `time stop` enthalten `--unbillable` bzw. `--unbillable=false`, wenn der Timer es hat.
- `time stop` beansprucht den Timer atomar; ein zweiter Stopp endet mit Exit 7 (Ruling C2, Task 11; Ruling T11-A). Nach Bestätigung und `--dry-run` (nie im Probelauf, nie nach abgelehnter Frage), direkt vor dem POST liest `time stop` die Timer-Datei erneut, legt ihren Inhalt exklusiv als `<datei>.stopping` an (`timer.Claim` über `safefile.CreateAtomic`: harter Link, sonst `O_EXCL`; von gleichzeitigen Stopps gelingt das genau einem) und entfernt die Timer-Datei nur, solange sie noch genau die kopierten Bytes enthält. Ein Umbenennen wäre nicht exklusiv: `os.Rename` ersetzt ein vorhandenes Ziel auf allen Plattformen (unter Windows per `MOVEFILE_REPLACE_EXISTING`), sodass ein neu gestarteter Timer eine fremde `.stopping`-Datei überschreiben könnte. Findet ein Stopp den Anspruch eines anderen oder keine Timer-Datei mehr: Exit 7 „Timer wird bereits von einem anderen papierkram time stop gebucht“, Hinweis „Nichts gesendet. Mit papierkram time status prüfen, ob er noch läuft“, ohne Anfrage. Ist die Timer-Datei nicht mehr der gelesene Timer (inzwischen gebucht und neu gestartet, z. B. während der Rückfrage), wird nichts beansprucht (ein schon angelegter Anspruch wird wieder entfernt), ebenfalls Exit 7 ohne Anfrage; ein neu gestarteter Timer wird nie überschrieben und, außer bei einem gleichzeitigen `time discard` (mit anschließendem `time start` genau zwischen Prüfung und Entfernen), nie entfernt. Unter Windows öffnet `safefile` Dateien zum Lesen mit `FILE_SHARE_DELETE` (`os.Open` teilt nur Lesen und Schreiben), damit ein gleichzeitiger Leser (`time status`, ein zweiter Stopp) das Entfernen nicht mit einer Sharing Violation verhindert. Jeder Fehler nach dem Anspruch (auch Exit 12 und 5xx) stellt die Timer-Datei aus `<datei>.stopping` wieder her (ebenfalls per `safefile.CreateAtomic`, nie über eine vorhandene Datei) und entfernt dann den Anspruch; der Hinweis bleibt wie in 9.3 („Timer bleibt erhalten“). Lässt sich danach nur der Anspruch nicht löschen, bleibt der Fehler des Stopps (mit seinem Code) und sein Hinweis nennt zuerst, `<datei>.stopping` von Hand zu löschen (solange sie liegt, enden `time start` und `time stop` mit Exit 7), danach („dann: …“) den Hinweis des Stopps. Scheitert das Wiederherstellen, Exit 1 „Timer-Datei nicht zurückgestellt: …“ mit dem Hinweis, `<datei>.stopping` in `<datei>` umzubenennen oder `papierkram time discard`, und dem vorherigen Fehler; liegt dort schon eine neue Timer-Datei (`fs.ErrExist`), lautet der Hinweis stattdessen „Beide Dateien prüfen, nicht umbenennen (das ersetzte die vorhandene Timer-Datei): <datei>.stopping ist der Timer dieses time stop, <datei> wurde seither angelegt“, ebenfalls mit dem vorherigen Fehler. Beim Lesen (`time start|status|stop`) kommt zuerst `<datei>.stopping`, dann `<datei>`; fehlt auch diese, wird `<datei>.stopping` noch einmal gelesen (ein anderer Stopp kann zwischen beiden Lesevorgängen beansprucht haben: er legt den Anspruch an, bevor er die Timer-Datei entfernt). Nach Erfolg wird `<datei>.stopping` gelöscht; scheitert das, Exit 11 wie bisher mit dieser Datei. Liegt `<datei>.stopping` vor (ein anderer Stopp sendet gerade, oder einer brach ab; beides ist nicht unterscheidbar): `time start` und `time stop` Exit 7 „Ein time stop läuft gerade oder wurde nicht abgeschlossen (Timer seit <Start>, <Aufgabe/Benutzer>)“, Hinweis „Erst mit papierkram time list --since <Startdatum> prüfen, ob der Eintrag angelegt wurde. Läuft kein anderer time stop mehr: wenn ja, papierkram time discard; sonst papierkram time discard und von Hand buchen mit <time log …>“; `time status` Exit 0 mit dem Timer und dieser Warnung; `time discard` (Bestätigung wie bisher) nennt und löscht beide Dateien; `context rename|delete` zählen die Datei wie einen laufenden Timer. Sie wird nach den Regeln der Timer-Datei gelesen (keine symbolische Verknüpfung, höchstens 64 KiB, gültiger State desselben Kontexts, sonst Exit 1 mit Hinweis auf `time discard`). Wird die Timer-Datei während des Lesens durch eine andere ersetzt (gleichzeitiger Stopp und `time start`; `safefile.ErrSwapped`), ist das ein Wettlauf und keine ungültige Datei: `time start|status|stop|discard` lesen sie bis zu dreimal neu, der Anspruch endet dann mit Exit 7 („bereits … gebucht“ oder „seit dem Lesen ersetzt“). Alte Timer-Dateien bleiben unverändert lesbar.

### 9.7 `company list --type customer|supplier`
Die API filtert nicht. Das CLI lädt alle Seiten (wie `--all`, inkl. Warnung ab 10 Seiten) und filtert lokal. Die Hilfe sagt das ausdrücklich. Pagination-Flags gelten dann für das gefilterte Ergebnis. **[Call]**

### 9.8 Projektdatei `.papierkram.toml`
```toml
context = "firma-gmbh"   # optional
company = 12
project = 34
task    = 56
user    = 1

[tasks]
tickets     = 57
maintenance = 58
```
- Suche: aktuelles Verzeichnis; liegt es in einem Git-Repo, aufwärts bis einschließlich Git-Wurzel; außerhalb von Git nur das aktuelle Verzeichnis. Nie darüber hinaus.
- Priorität: Flag > Env (`PAPIERKRAM_PROJECT`, `PAPIERKRAM_TASK`, …) > Projektdatei > Kontext-Defaults > eingebauter Default.
- Genutzt von `time log|start|stop|list`, `task list|create`, `project describe` ohne ID **[Call]**, `person list|create|describe|update|delete` (`--company`; offener Punkt 27, entschieden: Pfad-Flag, bleibt Pflicht, Person-ID bleibt Pflicht, Quellen Flag > `PAPIERKRAM_COMPANY` > `company` der Projektdatei, kein Kontext-Default; `default_from` in `commands --json`). `--task` akzeptiert ID oder Alias aus `[tasks]`.
- Enthält keine Secrets; unbekannte Schlüssel → Warnung, kein Fehler.
- Aliase in `[tasks]`: nicht leer, keine Zahl (würde als Aufgaben-ID gelesen), ohne Steuer- oder unsichtbare Zeichen, Wert eine positive ganze Zahl; sonst Exit 2 beim Laden (leerer Alias: Ruling T19-B).
- Ist die gefundene Datei nicht lesbar (Rechte, E/A-Fehler), Exit 1 (`error`) mit dem Hinweis „Projektdatei <pfad> prüfen oder entfernen“ (Pfad ohne Steuerzeichen).
- `papierkram local show` zeigt die wirksamen Werte mit Herkunft.

## 10. Konfiguration, Auth, Ausgabe

### 10.1 Kontexte und Config
- Datei `os.UserConfigDir()/papierkram/config.toml`:
```toml
active_context = "firma-gmbh"

[contexts.firma-gmbh]
subdomain = "firma-gmbh"
read_only = false
output = "table"
timeout = "30s"
user = 1          # Standard-User für Zeiteinträge

[contexts.agent-analyse]
subdomain = "firma-gmbh"
read_only = true
```
- Kontextauswahl: `--context` > `PAPIERKRAM_CONTEXT` > `context` aus Projektdatei > `active_context`. Unabhängig davon gilt `PAPIERKRAM_READ_ONLY=1` immer (8.3).
- Direktbetrieb ohne Config: `PAPIERKRAM_TOKEN` + `PAPIERKRAM_SUBDOMAIN` (haben Vorrang vor dem Kontext-Token, `auth status` zeigt die Quelle). `PAPIERKRAM_BASE_URL` überschreibt die komplette Basis-URL (Tests, Mock-Server). `PAPIERKRAM_READ_ONLY=1` erzwingt den Schutzmodus.
- `context delete` löscht auch den Keychain-Eintrag.
- Ist der Schlüsselbund nicht verfügbar, warnen `context create` („Schlüsselbund nicht verfügbar, ein dort gespeichertes Token von "<name>" wurde nicht entfernt.“, wie `auth logout`) und `context rename` („… von "<neu>" wurde nicht geprüft.“ und „… Token von "<alt>" wurde nicht verschoben.“) auf stderr; der Exit-Code bleibt 0.
- Scheitert bei `auth logout` das Schreiben von `config.toml`, nachdem der Schlüsselbund-Eintrag schon entfernt ist, endet der Befehl mit Exit 1; der Hinweis nennt diesen Teilzustand (Token im Schlüsselbund entfernt, in `config.toml` noch nicht) und `papierkram auth logout --context <name>` zum Abschließen.

### 10.2 Secrets
- Token im OS-Schlüsselbund über `zalando/go-keyring`: Service `papierkram-cli`, Account = Kontextname.
- Datei-Fallback (Token in `config.toml`, Rechte 0600) nur mit `--insecure-storage` und deutlicher Warnung; wird verwendet, wenn kein Schlüsselbund verfügbar ist (z. B. Linux ohne Secret Service). Fehlermeldung nennt den Fallback.
- `auth login` liest das Token per verdeckter Eingabe (TTY) oder `--token-stdin`; nie als Flag-Wert.
- Tokens erscheinen nie in Logs, `--debug`, Fehlermeldungen, Golden-Files.

### 10.3 Ausgabe
- `-o table` (Standard), `wide`, `json`, `yaml`; `--fields a,b.c` wählt Spalten bzw. JSON-Felder; `--jq <expr>` (gojq) impliziert JSON.
- Standardspalten **[Call]** (an MCP-Kompaktansicht angelehnt, `wide` ergänzt):

| Ressource | table |
|---|---|
| company | id, name, contact_type, customer_no/supplier_no, email |
| person | id, first_name, last_name, email, position |
| invoice | id, invoice_no, name, state, document_date, due_date, total_gross, billing.company |
| estimate | id, estimate_no, name, state, document_date, total_gross, billing.company |
| voucher | id, voucher_no, name, state, document_date, due_date, amount |
| proposition | id, article_no, name, price, vat_rate, proposition_type |
| payment-term | id, name, (Fälligkeit in Tagen) **[Builder]** |
| project | id, name, company_id, record_state, start_date, end_date |
| task | id, name, project_id, complete, deadline |
| time | id, started_at, ended_at, duration (h:mm), task_id, project_id, comments |
| custom-attribute | id, name, attributable_type, data_type |
| bank-connection | id, name |
| transaction | id, bdate, value, state, from.name, usage |
| user | id, full_name, email, role |

- **JSON-Vertrag** (stabil, versioniert, per Schema-Tests abgesichert):
  - `list`: API-Envelope unverändert; bei `--all` alle `entries` zusammengeführt, `page=1`, `has_more=false`, `total_pages` wie geliefert.
  - `describe`, `create`, `update`, Aktionen: das Objekt der API. Liefert die API leer, wird das Objekt nachgelesen (deliver) bzw. `{"id":N,"ok":true}` ausgegeben.
  - `delete`: `{"id":N,"deleted":true}`.
  - Optionale `warnings[]` nur bei Aktionen, die Warnungen erzeugen.
  - Schemas in `docs/schemas/` (OpenAPI-3-Schema-Objekte, strikt, `additionalProperties: false`, per Vertragstest gegen die echte Ausgabe geprüft): `list.json` (Envelope, auch `--all`, lokaler Filter und `api --paginate`), `ok.json` (leere Antwort: `{"id":N,"ok":true}` bzw. `{"ok":true}`), `delete.json`, `dry-run.json`, `deliver.json`, `partial.json`, `auth-status.json` (`auth status -o json`), `version.json` (`version -o json`), `error.json` (10.4) und `commands.json` (`commands --json`).
- Tabellen bleiben einzeilig je Eintrag: Zeilenumbrüche, Tabs, Steuer- und unsichtbare Formatzeichen sowie U+2028 und U+2029 werden zu Leerzeichen (ebenso in Meldungen, dort bleibt nur `\n`); eine Zelle nur aus Leerraum ist leer, eine Zeile ohne Inhalt zeigt „-“ in der ersten Spalte. Die `--jq`-Ausgabe ist auf 64 MiB begrenzt; geprüft wird je Ergebnis, bevor es gepuffert wird (Exit 1).
- `auth status`: Scheitert der Aufruf (kein Token, 401/403: Exit 3; 404: Exit 4) und zusätzlich die Ausgabe (`--jq`-Laufzeitfehler, Schreibfehler), bleibt der Code des Aufrufs; der Ausgabefehler erscheint als Warnung „Ausgabe fehlgeschlagen: …“ auf stderr.
- Ohne TTY auf stdout: keine Farben, keine Spinner. Ohne TTY auf stdin oder mit `--no-input`: keine Prompts. `NO_COLOR` wird beachtet.
- Hinweise und Warnungen immer auf stderr, Nutzdaten auf stdout.

### 10.4 Fehler und Exit-Codes
- Text: `Fehler: <message>` + `Hinweis: <hint>` + ggf. `Correlation-ID: …`.
- Bei `-o json|yaml` zusätzlich strukturiert auf stderr: `{"error":{"code":"not_found","message":"…","hint":"…","http_status":404,"correlation_id":"…","exit_code":4}}`.

| Exit | Code | Auslöser | Hinweis (Beispiel) |
|---|---|---|---|
| 0 | – | Erfolg, auch `--dry-run` | |
| 1 | `error`, `server_error` | unerwartet, 5xx, 2xx-Antwort > 64 MiB (5.5) | „Papierkram-Serverfehler, später erneut versuchen“ |
| 2 | `usage` | falsche Flags/Argumente, fehlende Pflichtangabe, unbekannter Befehl | „Verwendung: <Usage-Zeile>“; unbekannter Befehl „Befehle anzeigen: <befehl> --help“ |
| 3 | `auth` | 401, 403, kein Token | „Token prüfen: Einstellungen → API; `papierkram auth login`“ |
| 4 | `not_found` | 404 | „ID prüfen; gehört sie zu diesem Account/Kontext?“ |
| 5 | `validation` | 400, 422, lokale Validierung | API-Meldung + Formattipps (Datum `YYYY-MM-DD`, `vat_rate` 0.19) |
| 6 | `quota` | 429 | „Monatskontingent erschöpft“ |
| 7 | `conflict` | 409, laufender Timer, Timer von einem anderen `time stop` beansprucht oder `….stopping` liegt noch (9.6) | |
| 8 | `confirmation_required` | Bestätigung fehlt/abgelehnt oder Rückfrage mit Strg+C abgebrochen (alle Rückfragen liegen vor jeder Änderung) | „`--yes` verwenden“; bei Strg+C „Nichts gesendet“, bei Rückfragen, die nur lokal ändern (Token-Eingabe, `local init`, `time discard`), „Nichts gesendet oder geändert“ |
| 9 | `read_only` | Schreibversuch im `read_only`-Kontext | „anderen Kontext wählen“ |
| 10 | `network` | Timeout, DNS, TLS **bevor** eine Mutation gesendet wurde, oder bei GET (auch Strg+C während eines GET) | „Subdomain und Verbindung prüfen“ |
| 11 | `partial` | mehrstufiges Command teilweise erfolgreich (9.3) | Ausgabe nennt erledigte/fehlgeschlagene Schritte und den Befehl zum Nachholen; **nicht** das ganze Command wiederholen |
| 12 | `outcome_unknown` | Timeout/Verbindungsabbruch, **nachdem** POST/PUT/DELETE gesendet wurde, oder Weiterleitung (3xx) auf POST/PUT/DELETE (wird nicht verfolgt) | „Ergebnis unbekannt – vor Wiederholung per `describe`/`list` prüfen“ |

**Ergebnis unbekannt bei `deliver`** **[Call]**: Bricht die Verbindung nach dem Senden ab, liest das CLI den Beleg einmal nach. Ist `sent_on` bzw. die Belegnummer gesetzt → Exit 0 mit Warnung; sonst Exit 12. Bei `create` wird nicht automatisch gesucht (kein eindeutiges Merkmal); Exit 12.

## 11. Tests

| Ebene | Inhalt |
|---|---|
| Unit | Config, Kontextauflösung, Env-Priorität, Keychain-Fallback (Mock-Keyring), Coercion, VAT-Normalisierung (inkl. Zahl 19), Prune, Kategorie-Vorschläge, Item-Parser, Fehler-Mapping, Exit-Codes, Projektdatei-Suche (Git/Nicht-Git), Timer-State, Datums-Parsing |
| HTTP-Mock | `httptest`-Server validiert **jeden** Request mit kin-openapi gegen die gepatchte Spec (Pfad, Query, Body); Antworten aus Spec-Beispielen. Ein Test schlägt fehl, wenn ein Request nicht spec-konform ist. |
| Szenarien | `rogpeppe/go-internal/testscript` (txtar): komplette Abläufe mit stdout, stderr, Exit-Code; TTY vs. Non-TTY; `read_only`; `--dry-run`; `--yes` |
| Golden | table/wide/json/yaml je Ressource; `-update`-Flag |
| Abdeckung | jede Spec-Operation hat genau einen Registry-Eintrag und umgekehrt |
| Vertrag | `commands --json` und JSON-Ausgaben gegen eingecheckte JSON-Schemas |
| Regression | jede Falle aus Abschnitt 16 als benannter Test |
| E2E | Build-Tag `e2e`, siehe 11.1 |

Coverage-Ziel ≥ 80 % ohne `internal/api/gen`.

### 11.1 E2E
- Läuft nur mit `-tags e2e` und gesetzten `PAPIERKRAM_E2E_SUBDOMAIN`, `PAPIERKRAM_E2E_TOKEN` **und** `PAPIERKRAM_E2E_SANDBOX_CONFIRM` = gleiche Subdomain (Schutz gegen Verwechslung mit der Produktivbuchhaltung). Sonst `t.Skip`.
- Legt Datensätze mit Präfix `pk-e2e-<runid>-` an und räumt sie am Ende auf (löschen, wo möglich, sonst archivieren/stornieren). Ein `cleanup`-Test entfernt Reste früherer Läufe mit dem Präfix. Einzige Ausnahme vom Präfix: Nennt die Antwort auf `cancel` oder `reverse` eines eigenen Objekts ein anderes Objekt (Folgeobjekt mit eigener ID), wird es als `ABWEICHUNG` gemeldet und, nur wenn sein `type` dem der Ressource entspricht, genau diese ID mit aufgeräumt (löschen, sonst archivieren, nie stornieren).
- Prüft live: leerer Body bei `deliver --via pdf`, Quota-Header vorhanden, `vat_rate` 0.19 ergibt 19 %, Person `default` als String, Upload-MIME-Typen, Statuswerte der Belege.
- Abdeckung: Tracker, CoveragePlan, Gate nur bei vollständigem Lauf. Der Transport der Suite ordnet jede beantwortete Anfrage per Pfadauflösung einer der 89 Operationen zu (`e2eguard.Coverage`; abgelehnte und unbeantwortete Anfragen zählen nicht). `e2eguard.CoveragePlan` legt je Operation fest, welcher Test sie abdeckt, ob eine 2xx-Antwort nötig ist oder jede Antwort genügt (unbekannte Live-Semantik, z. B. `pay` ohne echte Zahlung) bzw. nur bei vorhandenen Daten, ob die Aufräumphase zählt (nur `delete`) und welche spätere Task sie erst abdeckt (ausstehend). Am Ende steht je Befund eine Zeile `ABDECKUNG …` im Log. Fehlt eine erwartete Operation, scheitert der Lauf nur, wenn er vollständig war (kein `-run`/`-skip`/`-list`, Budget und Kontingent nicht erschöpft, sonst grün); ausstehende Operationen lassen ihn nie scheitern.
- Credit-Budget pro Lauf wird in der Testdatei dokumentiert (Zielwert < 800; Budget 760, davon 64 Aufräumreserve). Antwortet die API mit 429 (Exit 6), sendet die Suite in den Tests nichts mehr, überspringt alle folgenden Tests (wie bei erschöpftem Budget) und endet mit Exit 1; das Aufräumen versucht es einmal, ein 429 dort beendet es ohne weitere Anfrage. Kein E2E in der normalen CI; manueller Workflow (`workflow_dispatch`) mit Secrets. Der Workflow sucht danach das Token im Log, ersetzt die Sandbox-Subdomain durch `<sandbox>` (das Repository und seine Artefakte sind öffentlich) und sucht danach Token und Subdomain erneut (gefunden, Secret leer oder Prüfung gescheitert: Log löschen, Schritt scheitert, kein Upload; Leerraum und Zeilenumbrüche am Anfang und Ende des Token-Secrets zählen nicht zum Token); nur ein sauberes Log landet mit den Zeilen `ABDECKUNG`, `ABWEICHUNG` und `Credits:` in der Step-Summary und als Artefakt `e2e-log` (14 Tage). **[Call]**
- Testaccount existiert noch nicht **[Pascal]**: Suite muss ohne ihn kompilieren und überspringen.

## 12. CI, Release, Doku

### 12.1 CI (GitHub Actions)
- Matrix `ubuntu-latest`, `macos-latest`, `windows-latest`: `go build`, `go test -race ./...`, Keychain-Integrationstest (Linux mit `dbus-run-session` + `gnome-keyring-daemon --unlock`).
- Einmalig: `go mod verify`, `golangci-lint`, `govulncheck`, `go generate` ohne Diff, Coverage-Report mit Schwelle, `goreleaser release --snapshot --clean`.
- Go-Version aus einer Quelle: `.go-version` (eine Zeile, `1.27.x`); jeder `actions/setup-go`-Schritt in CI, E2E und Release liest sie per `go-version-file: .go-version`, keiner setzt `go-version` selbst. `go.mod` nennt davon getrennt die Mindestversion (`go 1.26.0`, README „Go 1.26 oder neuer“); `scripts/check-go-version.sh` prüft beides, ein Update darf die Mindestversion nicht still anheben.
- Dependabot hält die per Commit-SHA gepinnten Actions (`github-actions`) und die direkten Go-Module (`gomod`) wöchentlich aktuell. Ausgenommen ist `github.com/speakeasy-api/openapi` (in `go.mod` gepinnt, nur zusammen mit oapi-codegen anheben); indirekte Module, darunter das Tool oapi-codegen, bekommen keine Pull Requests.

### 12.2 Release (GoReleaser v2)
- Ziele: darwin/linux/windows × amd64/arm64, `CGO_ENABLED=0`, Checksums, SBOM optional.
- Homebrew → `Padrio/homebrew-tap` mit `pk`-Symlink; Scoop → `Padrio/scoop-bucket` mit `pk`-Shim; nfpm deb/rpm mit `/usr/bin/pk`-Symlink; Archive enthalten Completions und Man-Pages.
- Tag-getrieben (`v*`), SemVer, `CHANGELOG.md` nach Keep a Changelog.
- **Grenze des Plans** **[Call]**: Der Implementierungsplan endet bei grüner CI und erfolgreichem `goreleaser release --snapshot`. Anlegen von `Padrio/homebrew-tap` und `Padrio/scoop-bucket`, Tags und echte Releases geschehen nur auf ausdrücklichen Auftrag Pascals in der jeweiligen Session (nach außen wirksam, wie Deployments). Die GoReleaser-Konfiguration für Tap/Bucket wird vorbereitet. Ein Fine-grained-PAT mit Schreibrecht auf beide Repos als Secret `PACKAGES_GITHUB_TOKEN`, angelegt von Pascal. **[Builder]** Ob GoReleaser-Version den `pk`-Alias für Scoop direkt unterstützt, sonst Manifest-Nachbearbeitung.

### 12.3 Doku
- `README.md` (Mensch): Installation, Schnellstart, Kontexte, Projektdatei, Sicherheit, `pk`-Hinweis.
- `AGENTS.md` (Agent): JSON-Vertrag, Exit-Codes, `read_only`-Kontext, `--dry-run`, Credits, verbotene Muster (z. B. `deliver` nie wiederholen).
- `skills/papierkram/SKILL.md`: Claude-Code-Skill mit Workflows: Beleg hochladen und zuordnen, Rechnung aus Angebot erstellen, offene Posten prüfen, Zeiten erfassen und abrechnen.
- `docs/reference/`: aus cobra generiert, Man-Pages ebenso (`papierkram gen-docs`, versteckt).
- `THIRD_PARTY_NOTICES.md`: Lizenzen der Abhängigkeiten; Dank an papierkram-mcp-server (MIT) für die dokumentierten API-Fallen.

## 13. Abhängigkeiten **[Call]**, Versionen **[Builder]**
`spf13/cobra`, `pelletier/go-toml/v2`, `zalando/go-keyring`, `oapi-codegen/oapi-codegen/v2` (Tool) + `oapi-codegen/runtime`, `getkin/kin-openapi`, `itchyny/gojq`, `mattn/go-isatty` bzw. `golang.org/x/term`, `goccy/go-yaml`, `rogpeppe/go-internal` (testscript). Tabellen mit `text/tabwriter` oder einer kleinen eigenen Funktion, keine schwere Tabellenbibliothek.

## 14. Später (v1.x)
`papierkram mcp serve` aus der Registry; Namensauflösung in Flags; `$EDITOR`-Bearbeitung; `invoice paid-status`-Report über offene Posten, falls die API Zahlungsdaten liefert; `winget`-Manifest.

## 15. Abdeckungsmatrix (alle 89 Operationen)

Erzeugt per `jq` aus `api/openapi.json`. Der Abdeckungstest erzwingt diese Zuordnung. Credits = Kosten der Operation laut Abschnitt 4.3; zusammengesetzte Befehle kosten die Summe ihrer Anfragen (`deliver` mit Nachlesen 2, `invoice update` ohne `--payment-term` mit Lesen 2, siehe 9.1 und 9.4a).

| # | Method | Pfad | Command | Klasse | Credits |
|---|---|---|---|---|---|
| 1 | `GET` | `/banking/bank_connections` | `papierkram bank-connection list` | read_only | 1 |
| 2 | `GET` | `/banking/bank_connections/{id}` | `papierkram bank-connection describe <id>` | read_only | 1 |
| 3 | `GET` | `/banking/transactions` | `papierkram transaction list` | read_only | 1 |
| 4 | `GET` | `/banking/transactions/{id}` | `papierkram transaction describe <id>` | read_only | 1 |
| 5 | `GET` | `/contact/companies` | `papierkram company list` | read_only | 1 |
| 6 | `POST` | `/contact/companies` | `papierkram company create` | create | 10 |
| 7 | `GET` | `/contact/companies/{id}` | `papierkram company describe <id>` | read_only | 1 |
| 8 | `PUT` | `/contact/companies/{id}` | `papierkram company update <id>` | update | 1 |
| 9 | `DELETE` | `/contact/companies/{id}` | `papierkram company delete <id>` | destructive | 1 |
| 10 | `POST` | `/contact/companies/{id}/archive` | `papierkram company archive <id>` | archive | 1 |
| 11 | `POST` | `/contact/companies/{id}/unarchive` | `papierkram company unarchive <id>` | archive | 1 |
| 12 | `GET` | `/contact/companies/{company_id}/persons` | `papierkram person list --company <id>` | read_only | 1 |
| 13 | `POST` | `/contact/companies/{company_id}/persons` | `papierkram person create --company <id>` | create | 10 |
| 14 | `GET` | `/contact/companies/{company_id}/persons/{id}` | `papierkram person describe <id> --company <id>` | read_only | 1 |
| 15 | `PUT` | `/contact/companies/{company_id}/persons/{id}` | `papierkram person update <id> --company <id>` | update | 1 |
| 16 | `DELETE` | `/contact/companies/{company_id}/persons/{id}` | `papierkram person delete <id> --company <id>` | destructive | 1 |
| 17 | `POST` | `/expense/vouchers/{voucher_id}/documents` | `papierkram voucher upload <voucher-id> <file>` | create | 10 |
| 18 | `DELETE` | `/expense/vouchers/{voucher_id}/documents/{id}` | `papierkram voucher document delete <voucher-id> <doc-id>` | destructive | 1 |
| 19 | `GET` | `/expense/vouchers` | `papierkram voucher list` | read_only | 1 |
| 20 | `POST` | `/expense/vouchers` | `papierkram voucher create` | create | 50 |
| 21 | `GET` | `/expense/vouchers/{id}` | `papierkram voucher describe <id>` | read_only | 1 |
| 22 | `PUT` | `/expense/vouchers/{id}` | `papierkram voucher update <id>` | update | 1 |
| 23 | `DELETE` | `/expense/vouchers/{id}` | `papierkram voucher delete <id>` | destructive | 1 |
| 24 | `POST` | `/expense/vouchers/{id}/archive` | `papierkram voucher archive <id>` | archive | 1 |
| 25 | `POST` | `/expense/vouchers/{id}/unarchive` | `papierkram voucher unarchive <id>` | archive | 1 |
| 26 | `POST` | `/expense/vouchers/{id}/cancel` | `papierkram voucher cancel <id>` | destructive | 1 |
| 27 | `POST` | `/expense/vouchers/{id}/cancel_with_reverse_entry` | `papierkram voucher reverse <id>` | destructive | 1 |
| 28 | `POST` | `/expense/vouchers/{id}/pay` | `papierkram voucher pay <id>` | destructive | 1 |
| 29 | `GET` | `/expense/vouchers/{id}/pdf` | `papierkram voucher pdf <id> -O <file>` | read_only | 1 |
| 30 | `GET` | `/income/estimates` | `papierkram estimate list` | read_only | 1 |
| 31 | `POST` | `/income/estimates` | `papierkram estimate create` | create | 50 |
| 32 | `GET` | `/income/estimates/{id}` | `papierkram estimate describe <id>` | read_only | 1 |
| 33 | `PUT` | `/income/estimates/{id}` | `papierkram estimate update <id>` | update | 1 |
| 34 | `DELETE` | `/income/estimates/{id}` | `papierkram estimate delete <id>` | destructive | 1 |
| 35 | `POST` | `/income/estimates/{id}/archive` | `papierkram estimate archive <id>` | archive | 1 |
| 36 | `POST` | `/income/estimates/{id}/unarchive` | `papierkram estimate unarchive <id>` | archive | 1 |
| 37 | `POST` | `/income/estimates/{id}/cancel` | `papierkram estimate cancel <id>` | destructive | 1 |
| 38 | `POST` | `/income/estimates/{id}/deliver` | `papierkram estimate deliver <id> --via email\|pdf` | send | 1 |
| 39 | `GET` | `/income/estimates/{id}/pdf` | `papierkram estimate pdf <id> -O <file>` | read_only | 1 |
| 40 | `GET` | `/income/invoices` | `papierkram invoice list` | read_only | 1 |
| 41 | `POST` | `/income/invoices` | `papierkram invoice create` | create | 50 |
| 42 | `GET` | `/income/invoices/{id}` | `papierkram invoice describe <id>` | read_only | 1 |
| 43 | `PUT` | `/income/invoices/{id}` | `papierkram invoice update <id>` (ohne `--payment-term` zuvor Lesen, +1 Credit, 9.4a) | update | 1 |
| 44 | `DELETE` | `/income/invoices/{id}` | `papierkram invoice delete <id>` | destructive | 1 |
| 45 | `POST` | `/income/invoices/{id}/archive` | `papierkram invoice archive <id>` | archive | 1 |
| 46 | `POST` | `/income/invoices/{id}/unarchive` | `papierkram invoice unarchive <id>` | archive | 1 |
| 47 | `POST` | `/income/invoices/{id}/cancel` | `papierkram invoice cancel <id>` | destructive | 1 |
| 48 | `POST` | `/income/invoices/{id}/deliver` | `papierkram invoice deliver <id> --via email\|pdf` | send | 1 |
| 49 | `GET` | `/income/invoices/{id}/pdf` | `papierkram invoice pdf <id> -O <file>` | read_only | 1 |
| 50 | `GET` | `/income/payment_terms` | `papierkram payment-term list` | read_only | 1 |
| 51 | `GET` | `/income/payment_terms/{id}` | `papierkram payment-term describe <id>` | read_only | 1 |
| 52 | `GET` | `/income/propositions` | `papierkram proposition list` | read_only | 1 |
| 53 | `POST` | `/income/propositions` | `papierkram proposition create` | create | 10 |
| 54 | `GET` | `/income/propositions/{id}` | `papierkram proposition describe <id>` | read_only | 1 |
| 55 | `PUT` | `/income/propositions/{id}` | `papierkram proposition update <id>` | update | 1 |
| 56 | `DELETE` | `/income/propositions/{id}` | `papierkram proposition delete <id>` | destructive | 1 |
| 57 | `POST` | `/income/propositions/{id}/archive` | `papierkram proposition archive <id>` | archive | 1 |
| 58 | `POST` | `/income/propositions/{id}/unarchive` | `papierkram proposition unarchive <id>` | archive | 1 |
| 59 | `GET` | `/info` | `papierkram info` | read_only | 1 |
| 60 | `GET` | `/projects` | `papierkram project list` | read_only | 1 |
| 61 | `POST` | `/projects` | `papierkram project create` | create | 10 |
| 62 | `GET` | `/projects/{id}` | `papierkram project describe <id>` | read_only | 1 |
| 63 | `PUT` | `/projects/{id}` | `papierkram project update <id>` | update | 1 |
| 64 | `DELETE` | `/projects/{id}` | `papierkram project delete <id>` | destructive | 1 |
| 65 | `POST` | `/projects/{id}/archive` | `papierkram project archive <id>` | archive | 1 |
| 66 | `POST` | `/projects/{id}/unarchive` | `papierkram project unarchive <id>` | archive | 1 |
| 67 | `GET` | `/settings/custom_attributes` | `papierkram custom-attribute list` | read_only | 1 |
| 68 | `POST` | `/settings/custom_attributes` | `papierkram custom-attribute create` | create | 10 |
| 69 | `GET` | `/settings/custom_attributes/{id}` | `papierkram custom-attribute describe <id>` | read_only | 1 |
| 70 | `PUT` | `/settings/custom_attributes/{id}` | `papierkram custom-attribute update <id>` | update | 1 |
| 71 | `DELETE` | `/settings/custom_attributes/{id}` | `papierkram custom-attribute delete <id>` | destructive | 1 |
| 72 | `POST` | `/settings/custom_attributes/{id}/archive` | `papierkram custom-attribute archive <id>` | archive | 1 |
| 73 | `POST` | `/settings/custom_attributes/{id}/unarchive` | `papierkram custom-attribute unarchive <id>` | archive | 1 |
| 74 | `GET` | `/tracker/tasks` | `papierkram task list` | read_only | 1 |
| 75 | `POST` | `/tracker/tasks` | `papierkram task create` | create | 10 |
| 76 | `GET` | `/tracker/tasks/{id}` | `papierkram task describe <id>` | read_only | 1 |
| 77 | `PUT` | `/tracker/tasks/{id}` | `papierkram task update <id>` | update | 1 |
| 78 | `DELETE` | `/tracker/tasks/{id}` | `papierkram task delete <id>` | destructive | 1 |
| 79 | `POST` | `/tracker/tasks/{id}/archive` | `papierkram task archive <id>` | archive | 1 |
| 80 | `POST` | `/tracker/tasks/{id}/unarchive` | `papierkram task unarchive <id>` | archive | 1 |
| 81 | `GET` | `/tracker/time_entries` | `papierkram time list` | read_only | 1 |
| 82 | `POST` | `/tracker/time_entries` | `papierkram time create` | create | 10 |
| 83 | `GET` | `/tracker/time_entries/{id}` | `papierkram time describe <id>` | read_only | 1 |
| 84 | `PUT` | `/tracker/time_entries/{id}` | `papierkram time update <id>` | update | 1 |
| 85 | `DELETE` | `/tracker/time_entries/{id}` | `papierkram time delete <id>` | destructive | 1 |
| 86 | `POST` | `/tracker/time_entries/{id}/archive` | `papierkram time archive <id>` | archive | 1 |
| 87 | `POST` | `/tracker/time_entries/{id}/unarchive` | `papierkram time unarchive <id>` | archive | 1 |
| 88 | `GET` | `/users` | `papierkram user list` | read_only | 1 |
| 89 | `GET` | `/users/{id}` | `papierkram user describe <id>` | read_only | 1 |

Zusätzliche Commands ohne eigene Operation: `invoice create --from-estimate` (Nr. 32 + 41), `voucher create --file` (Nr. 20 + 17), `time log|start|stop` (Nr. 82), `company list --type` (Nr. 5, alle Seiten), `api` (beliebig).

## 16. Regressionsliste (jede Zeile = benannter Test)

| # | Falle | Erwartetes Verhalten |
|---|---|---|
| R1 | `deliver` antwortet 2xx mit leerem Body | Erfolg, Nachlesen, Ausgabe `invoice_no`/`sent_*`; nie Exit ≠ 0 |
| R2 | Nachlesen nach `deliver` scheitert | Exit 0 + Warnung „nicht erneut ausführen“ |
| R3 | leerer 2xx-Body bei anderen Aktionen | kein Parse-Fehler |
| R4 | `deliver` ohne `--via` | Exit 2, kein Request |
| R5 | `--via email` ohne `--to/--subject/--body` | Exit 2, kein Request |
| R6 | Feld `address` statt `recipient` in `--input` | Exit 5 lokal, kein Request |
| R7 | `budget_money`/`budget_time` als Zahl | als String gesendet |
| R8 | Task `complete`, `relative_costs`, `flagged` | als String gesendet |
| R9 | Person `default`, `flagged` | als String gesendet |
| R10 | Artikel `price`, `vat_rate` | als String gesendet, VAT normalisiert |
| R11 | `vat_rate` `19`, `19%`, `0,19`, `0.19`, Zahl 19 | jeweils `0.19` |
| R12 | `provenance non_eu` | Exit 5 mit Vorschlag `foreign` |
| R13 | `note` statt `notes` | Exit 5 mit Vorschlag `notes`, nie still verworfen |
| R14 | GET-Antwort per `update --input` zurückschicken | readOnly-Felder entfernt (Hinweis), Request spec-konform |
| R15 | Zeiteintrag ohne Task/User | Exit 2 mit Hinweis auf Projektdatei |
| R16 | Zeiteintrag-Body | `entry_date`, `started_at_time`, `ended_at_time` als `HH:MM`, keine ISO-Zeitstempel |
| R17 | Rechnung/Angebot/Beleg ohne Position | Exit 5 lokal |
| R18 | leeres `line_items` bei update mit `--replace-items` | wird gesendet, nicht verworfen |
| R19 | `company list --type` | alle Seiten, lokal gefiltert |
| R20 | `order_by` ohne Richtung | CLI setzt immer beide |
| R21 | Quota-Header | aus `X-Remaining-Quota`, nicht `X-RateLimit-Remaining` |
| R22 | 429 | Exit 6, keine Wiederholung |
| R23 | POST nach Netzwerkfehler | keine Wiederholung |
| R24 | ID `0` | wird korrekt übertragen (kein Truthiness-Fehler) |
| R25 | `archive` an Rechnung | keine Bestätigung nötig |
| R26 | Token in `--debug` | als `***` |
| R27 | `read_only`-Kontext + `api POST` | Exit 9, kein Request |
| R28 | Kategorie mit Tippfehler | Exit 5 mit drei Vorschlägen |
| R29 | PDF ohne `-O` auf TTY | Fehler statt Binärausgabe |
| R30 | 400 bei unbekanntem `billing_state` | lokal verhindert (Enum-Prüfung) |
| R31 | `describe -o json` → `update --input -` mit Positionen | Positionsfelder bleiben erhalten (Request-Schema hat Vorrang), `vat_rate "19%"` → `0.19` |
| R32 | `update --item` ohne `--replace-items` | Exit 2, kein Request |
| R33 | `api POST …/deliver` mit `send_via:"email"` ohne `email` | Exit 5, kein Request |
| R34 | `api DELETE …` ohne `--yes`, ohne TTY | Exit 8 |
| R35 | `PAPIERKRAM_READ_ONLY=1` + `--context schreibbar` + `invoice create` | Exit 9 |
| R36 | `PAPIERKRAM_READ_ONLY=1` + `context use` | Exit 9 |
| R37 | Timeout nach gesendetem `deliver`, Nachlesen zeigt `sent_on` | Exit 0 + Warnung |
| R38 | Timeout nach gesendetem `create` | Exit 12 |
| R39 | `voucher create --file`, Upload scheitert | Exit 11, Beleg-ID + Nachhol-Befehl |

## 17. Offene Punkte für die Umsetzung
Der aktuelle Stand je Punkt (Annahme des CLI, Entscheidung, E2E-Test, Messergebnisse der Live-Läufe) steht in [docs/offene-punkte.md](../offene-punkte.md); die Liste hier ist der Stand beim Entwurf.

- Status-Enums der Belege sind undokumentiert: Tabellen zeigen `state` roh; E2E sammelt die Werte, Doku ergänzt sie.
- Erlaubte Upload-MIME-Typen und maximale Dateigröße: per E2E klären.
- Verhalten von `0.19` bei Artikeln (String) live bestätigen.
- Ob `GET /info` Credits kostet: per Quota-Header in E2E messen, Registry anpassen.
- Teil-Update-Semantik von `PUT` (6.3a) und Ersetzen der Positionen live bestätigen.
- Die Path-Parameter `{id}` sind in der Spec teils nicht deklariert; im Overlay ergänzen **[Builder]**.
