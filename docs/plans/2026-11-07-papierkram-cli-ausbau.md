# papierkram-cli – Implementierungsplan Runde 2 (Ausbau)

> **Für agentische Ausführung:** PFLICHT-SUB-SKILL: `superpowers:subagent-driven-development`. Je Task: Brief → Implementer (model opus, TDD) → Reviewer (opus) → Fix-Runde → Ledger. Sequenziell, weil viele Tasks `e2e/` bzw. `internal/cli` teilen.

**Ziel:** Live-Abdeckung aller 89 API-Operationen messbar machen (bis auf begründete, gemessene Ausschlüsse), die offenen Entscheidungen 29, 27, 22 und C2 umsetzen, die Spec-Versprechen (JSON-Schemas, `commands --json`) nachziehen, Uploads und Antworten robust machen, die geparkten Minors abarbeiten und die Doku synchronisieren.

**Architektur:** Unverändert spec-getrieben (Spec §5). Neu ist nur ein Abdeckungs-Tracker in `internal/e2eguard`, der jede beantwortete Anfrage der Live-Suite per `spec.MatchPath` einer OperationID zuordnet und gegen eine statische Tabelle `e2eguard.CoveragePlan` prüft. Alle anderen Änderungen erweitern bestehende Pfade (Presets, `keepPaymentTerm`, Timer, Retry).

**Spec:** `docs/specs/2026-11-03-papierkram-cli-design.md` (einzige Quelle der Wahrheit; je Task mitziehen). **Grobplan mit Entscheidungen:** `/Users/pascal/.claude/plans/plane-einmal-dass-wir-lazy-zebra.md` (freigegeben; Entscheidungen 29, 27, 22, C2, „deliver pdf Exit 12 bleibt“, „v1.x nicht in diesem Plan“, „Release nur auf Auftrag“ sind bindend). **Analyse mit Fundstellen:** `.superpowers/sdd/2026-11-07-papierkram-cli-ausbau/analysis-findings.md`.

Zuordnung Grobplan → Tasks: A1 → 1, A2 → 2, A3 → 3, A4 → 4, A5 → 5, A6 → 6, A7 → 7, B1 → 8, B2 → 9, B3 → 10, B4 → 11, B5 → 12, C1 → 13, C2 → 14, C3 → 15, C4 → 16, D1 → 17 + 18, D2 → 19, D3 → 20, D4 → 21, D5 → 22.

## Global Constraints

Gilt für jede Task zusätzlich zu `global-constraints.md` des Workspaces und zu den Abschnitten „Global Constraints“, „Exit-Codes“, „Reihenfolge der Prüfungen“, „Review Focus“, „Dateistruktur“ des alten Plans `docs/plans/2026-11-03-papierkram-cli.md`.

**Bindend aus Runde 1 (verdichtet):**
- Conventional Commits, Englisch, klein; **nie** `Co-Authored-By`; Identität aus globaler Git-Config; nicht pushen (Controller), kein Amend/Rebase/Force.
- TDD: roter Test (Ausgabe im Report), Code, grün.
- Nutzersichtbare Texte Deutsch mit echten Umlauten; Englisch nur Command-/Flag-Namen, JSON-Schlüssel, Fehlercodes, Code-Kommentare, Commits.
- Tokens nie in Logs, `--debug`, Fehlern, Fixtures, Golden-Files; Test-Tokens `test-token-…`.
- Kein Request gegen echte Accounts; Binary bzw. `go run ./cmd/papierkram` nur mit `PAPIERKRAM_BASE_URL` auf 127.0.0.1/mockapi (Ausnahme: rein lokale Befehle).
- Keine Wiederholung bei POST/PUT/DELETE oder 429; leerer 2xx-Body ist nie ein Fehler.
- Spec-getrieben: keine handgepflegten Feldlisten; `api/openapi.json` nie ändern, Korrekturen nur in `api/overlay.yaml` (danach `go generate ./...`).
- Keine `cobra.ExactArgs/…`, kein `MarkFlagRequired`; deutsche Validatoren, Usage-Hinweis über `useLine(c)`.
- Mutationen nur über `mutationClient` → Body → `beforeMutation`/`beforeSend`, Annotation `pk:class`; Tests für dry-run (0 Requests), read-only (Exit 9, 0 Requests), Bestätigung (Exit 8).
- Fehler nach gesendetem Request nie Exit 2, sondern Exit 1 mit Präfix „Befehl ausgeführt, Ausgabe fehlgeschlagen“ (`afterRequest`).
- Keine neuen `TestRNN_`-Namen in dieser Runde (TestRegressionIndex bleibt 39/39).
- Akzeptierte Rulings nicht anfassen: L292 (`pdf_owner_unix.go:14`), L335 (`local.go:429-431`).

**Runde 2:**
- **Live-E2E (`-tags e2e` mit Sandbox-Variablen) läuft nur der Controller**, nie Implementer oder Reviewer. Implementer prüfen E2E-Code ausschließlich offline (Gate G6).
- **Sandbox-Frist:** `<sandbox>` endet ca. **2026-11-19**. Live-Lauf 5 spätestens 2026-11-14, Puffer für Lauf 6 bis 2026-11-18. Restkontingent beim Planen: 8386 Credits.
- **E2E-Suite:** nie `pay` mit echter Transaktion (nur `e2eguard.NoTransactionID`), nie `deliver --via email`, kein String-Literal `"email"` in `e2e/`; Ausgabe nur über `e2e/log_test.go`; jede neue Live-Anfrage steht im Kosten-Header von `e2e/e2e_test.go`, hat ein `s.need(t, n)` davor und passt ins Budget; jedes angelegte Objekt heißt `pk-e2e-<runid>-…` und wird über `s.create`/`s.track` fürs Aufräumen registriert. Bankdaten (Bankverbindungen, Umsätze) nie mit Werten loggen, nur Anzahl und Schlüsselform (`keyShape`).
- **Credit-Budget je Lauf:** `creditBudget = 760`, Compile-Guard `const _ = uint(799 - creditBudget)`, `cleanupReserve = e2eguard.CleanupReserve(6, 13, 10)` = 60; geplant Tests 615 + Aufräumen 30 = **645**, schlimmster Fall 653 (Herleitung im Kopf von `e2e/e2e_test.go`; Stand nach Fix L5, 2026-11-08: Punkt 28 prüft lokal ohne Anfrage. Davor galt `CleanupReserve(7, 13, 10)` = 64, Tests 665 + Aufräumen 31 = 696; beim Planen in Task 2 `CleanupReserve(7, 12, 10)` = 62, 655 + 28 = 683).
- **Entscheidungen verbindlich:** 29 (Task 8), 27 (Task 9), 22 (Task 10), C2 (Task 11), Prompt-Abbruch Exit 8 (Task 17). `deliver --via pdf` mit Abbruch bleibt Exit 12.
- **Cross-Pfad-Disziplin:** Wer einen bestehenden Pfad spiegelt, liefert im Report die Tabelle aus seiner Task (Original-Parameter vs. neu) und begründet jede Abweichung.
- **Doku mitziehen:** Nach jeder Verhaltensänderung Spec, README, AGENTS, `docs/offene-punkte.md`, `skills/papierkram/SKILL.md` soweit betroffen; Eintrag unter `## [Unreleased]` in `CHANGELOG.md`; danach `make docs` ohne Diff.

**Gates (in jeder Task unter „Akzeptanz“ referenziert):**
- **G1** `go test -race ./...` grün.
- **G2** `golangci-lint run ./...` **und** `golangci-lint run --build-tags=e2e ./...` ohne Befunde.
- **G3** `go vet ./...` und `go vet -tags e2e ./...` sauber; `gofmt -l .` leer.
- **G4** `scripts/coverage.sh 80` grün (Ist ≈ 94 %, nicht senken).
- **G5** `go generate ./... && make docs && git status --porcelain` leer (TestReferenceUpToDate, TestDocCommandsAreValid grün).
- **G6** (E2E-Tasks) `go test ./internal/e2eguard/...` grün (u. a. TestE2ESuiteSource) und `go test -count=1 -tags e2e ./e2e/...` ohne `PAPIERKRAM_E2E_*` → jeder Test `SKIP`, Lauf `PASS`.
- **G7** (Workflow-Tasks) `sh scripts/check-action-pins.sh` grün.

## Review Focus

- **Phase A (E2E):** Keine Live-Anfrage durch Implementer/Reviewer. Kostenzeilen = tatsächliche Anfragen (jede `run`/`ok`/`create` zählt, Lesen von `invoice update` mit 1); `need()` vor jeder Gruppe; Header-Summe stimmt. Jedes neue Objekt registriert und mit Präfix. Kein Logging von Bankdaten-Werten, Token-Freiheit der Logs. CoveragePlan-Schlüssel exakt = Registry; die Abdeckungsprüfung bricht nur einen vollständigen Lauf (`-run`/`-skip` leer, Budget nicht erschöpft). Der Workflow lädt das Log nur nach bestandenem Token-Check hoch.
- **Phase B (Entscheidungen):** Cross-Pfad-Tabellen vollständig; Credits in `--dry-run` und `commands --json` stimmen; Reihenfolge der Prüfungen (Exit 2 → 9 → 5 → dry-run → 8 → senden) bleibt; Timer-Claim atomar und auf Windows korrekt (Rename-Semantik); alte Timer-Dateien bleiben lesbar.
- **Phase C (Robustheit):** Schemas strikt und additiv (`commands.json` bleibt `version` 1); längeres Zeitlimit nur für Uploads; Antwortlimit mit richtigem Exit-Code je Methode und Status.
- **Phase D (Minors/Refactor):** Refactor ohne Verhaltensänderung (keine Golden-/txtar-Änderung); Exit-Code-Änderung (Prompt-Abbruch) überall dokumentiert; Rulings L292/L335 unberührt.

## Reihenfolge

1. **Phase A – Live-Abdeckung (Fristsache):** Task 1 → 7.
2. **Phase B – Entscheidungen:** Task 8 → 12 (vor Live-Lauf 5, damit live geprüft).
3. **Meilenstein Live-Lauf 5** (Controller, kein Task).
4. **Phase C – Spec-Versprechen und Robustheit:** Task 13 → 16.
5. **Phase D – Minors, Refactor, CI, Doku:** Task 17 → 22.
6. **Abschluss** (Controller, kein Task).

---

## Phase A – Live-Abdeckung

### Task 1: Abdeckungs-Tracker und CoveragePlan (A1)

**Dateien:** anlegen `internal/e2eguard/coverage.go`, `internal/e2eguard/coverage_test.go`; ändern `internal/e2eguard/transport.go`, `internal/e2eguard/transport_test.go`, `e2e/harness_test.go`, `e2e/e2e_test.go` (TestMain), `e2e/log_test.go`, `e2e/README.md`, Spec §11.1.

**Spec-Abschnitte:** 11.1, 15 (Abdeckungsmatrix), 17.

**Anforderungen:**
- Typen in `coverage.go`:
  - `type Mode int` mit `ModeSuccess` (mindestens eine 2xx-Antwort), `ModeAnswered` (irgendeine beantwortete Anfrage, Status egal; für Operationen mit unbekannter Live-Semantik), `ModeIfData` (nur wenn Daten vorhanden; sonst muss die Suite `NotApplicable` melden).
  - `type Phase int` mit `PhaseTest` (zählt nur vor `BeginCleanup`) und `PhaseAny` (Test- oder Aufräumphase).
  - `type Expectation struct { Test string; Mode Mode; Phase Phase; Pending int; Note string }` – `Test` ist der Go-Testname (Subtests mit `/`), `Pending` die Task-Nummer, die die Operation erst abdeckt (0 = erwartet), `Note` die Begründung bei `ModeAnswered`/`ModeIfData`.
  - `var CoveragePlan = map[string]Expectation{…}` mit **genau** den 89 OperationIDs der Registry.
  - `type Coverage struct` (nebenläufig sicher) mit `NewCoverage() *Coverage`, `BeginCleanup()`, `Record(method, path string, status int)`, `NotApplicable(opID, reason string)`, `Report(plan map[string]Expectation) CoverageReport`.
  - `Record` löst per `spec.MatchPath(method, path)` (strippt `/api/v1`) die Operation auf; Pfade ohne Treffer landen in `Unknown`. Gezählt wird je Operation getrennt nach Phase und nach 2xx/sonstig.
  - `CoverageReport` enthält `Covered`, `OnlyAnswered`, `OnlyCleanup`, `NotApplicable`, `Pending`, `Missing []string` (OperationIDs sortiert) und `Lines []string`.
- Report-Zeilen (exakt, eine je Zeile):
  - `ABDECKUNG: abgedeckt X/89 (davon A nur mit Fehlerantwort, C nur im Aufräumen), nicht anwendbar N, ausstehend P, fehlend M`
  - je fehlend: `ABDECKUNG fehlt: <OpID> (erwartet von <Test>)`
  - je nicht anwendbar: `ABDECKUNG nicht anwendbar: <OpID>: <Grund>`
  - je ausstehend: `ABDECKUNG ausstehend: <OpID> (Task <N>)`
  - je unbekannter Pfad: `ABDECKUNG unbekannter Pfad: <METHOD> <path>`
- `Transport` bekommt das Feld `Coverage *Coverage` (nil erlaubt); `RoundTrip` ruft nach `Meter.Record` `t.Coverage.Record(req.Method, req.URL.Path, resp.StatusCode)`. Abgelehnte (ErrRefused/ErrBudget) und unbeantwortete Anfragen zählen **nicht**.
- Harness: `liveSuite` bekommt `coverage *e2eguard.Coverage`, `newLiveSuite` setzt sie in `Transport`. TestMain ruft direkt nach `s.meter.BeginCleanup()` `s.coverage.BeginCleanup()`; nach `s.cleanup()` und vor `deviationSummary` gibt `s.coverageSummary()` (in `log_test.go`, über `stderrf`) die Report-Zeilen aus.
- **Gate in TestMain:** `Missing` nicht leer → Exit 1 mit `e2e: Abdeckung unvollständig: <n> Operation(en) fehlen`, aber nur wenn (a) `flag.Lookup("test.run")` und `flag.Lookup("test.skip")` leer sind, (b) `s.exhausted` falsch ist und (c) `code == 0`. Sonst nur Zeilen. `Pending` bricht nie.
- Initialer Inhalt des CoveragePlan (Test = erster Test, der die Operation heute trifft):
  - `ModeSuccess`/`PhaseTest`, erwartet (45): ListCompanies, CreateCompany, GetCompany, UpdateCompany, CreatePerson, GetPerson, UpdatePerson, UploadVoucherDocument, ListVouchers, CreateVoucher, GetVoucher, UpdateVoucher, ListEstimates, CreateEstimate, GetEstimate, UpdateEstimate, DeliverEstimate, GetEstimatePdf, ListInvoices, CreateInvoice, GetInvoice, UpdateInvoice, ListPaymentTerms, ListPropositions, CreateProposition, GetProposition, UpdateProposition, GetInfo, ListProjects, CreateProject, GetProject, UpdateProject, ListCustomAttributes, CreateCustomAttribute, GetCustomAttribute, UpdateCustomAttribute, ListTasks, CreateTask, GetTask, UpdateTask, ListTimeEntries, CreateTimeEntry, GetTimeEntry, UpdateTimeEntry, ListUsers – sowie PayVoucher als `ModeAnswered` (Test `TestE2E_Carries/10_VoucherPayValueNull`, Note „nie echt bezahlt: nur value null mit NoTransactionID, 4xx erwartet“).
  - `ModeSuccess`/`PhaseAny`, erwartet (10, Test `TestMain/cleanup`): DeleteCompany, DeletePerson, DeleteVoucher, DeleteEstimate, DeleteInvoice, DeleteProposition, DeleteProject, DeleteCustomAttribute, DeleteTask, DeleteTimeEntry.
  - `Pending: 3`: DeliverInvoice, GetInvoicePdf, ArchiveInvoice, UnarchiveInvoice (ModeSuccess), CancelInvoice, CancelEstimate (ModeAnswered).
  - `Pending: 4`: GetVoucherPdf, DeleteVoucherDocument, ArchiveVoucher, UnarchiveVoucher (ModeSuccess), CancelVoucher, CancelVoucherWithReverseEntry (ModeAnswered).
  - `Pending: 5`: ArchiveTimeEntry, UnarchiveTimeEntry.
  - `Pending: 6`: Archive/Unarchive von Company, Project, Task, Proposition, CustomAttribute, Estimate (12), ListPersons, GetUser, GetPaymentTerm, ListBankConnections, ListTransactions (ModeSuccess), GetBankConnection, GetTransaction (ModeIfData, Note „nur wenn die Liste Einträge hat“).
  - Summe 45 + 1 + 10 + 6 + 6 + 2 + 19 = 89.

**Tests (offline, `internal/e2eguard/coverage_test.go`):**
- `TestCoveragePlanMatchesRegistry` – Schlüssel == OperationIDs aus `registry.Entries()`, 89.
- `TestCoveragePlanTestsExist` – jeder `Test` (Teil vor `/`) ist eine `func Test…` in `e2e/*_test.go` (per `go/parser`, Modulwurzel über `modroot`), außer `TestMain/cleanup`.
- `TestCoveragePlanModes` – `ModeIfData` nur bei GetBankConnection/GetTransaction; `ModeAnswered` hat eine `Note`; `PhaseAny` nur bei `Delete*` (ohne DeleteVoucherDocument).
- `TestCoverageRecord` – `/api/v1/income/invoices/12/deliver` → DeliverInvoice; Query und Slash ignoriert; unbekannter Pfad → `Unknown`; Phase-Trennung; 422 zählt nur für `ModeAnswered`.
- `TestCoverageReportLines` – exakte Zeilen oben für ein gemischtes Szenario (fehlend, ausstehend, nicht anwendbar, nur Aufräumen).
- `TestTransportRecordsCoverage` (transport_test.go) – beantwortete Anfrage gezählt, abgelehnte (Host, Budget, pay-Body) nicht.

**Doku:** Spec §11.1 um einen Punkt „Abdeckung: Tracker, CoveragePlan, Gate nur bei vollständigem Lauf“ ergänzen; `e2e/README.md` Abschnitt „Abdeckung“ (Zeilenformat, Gate-Regeln); CHANGELOG.

**Akzeptanz:** alle Tests oben grün; Gates G1–G6; `go test -tags e2e ./e2e/...` ohne Variablen SKIP. Kein Live-Lauf.

**E2E-Kosten:** 0 Credits (keine neue Anfrage). Implementer läuft die Live-Suite nicht.

### Task 2: Budget, Kostentabelle und E2E-Workflow (A2)

**Dateien:** ändern `e2e/e2e_test.go` (Header, `creditBudget`, Guard, `cleanupReserve`), `internal/e2eguard/meter_test.go`, `e2e/README.md`, `.github/workflows/e2e.yml`, Spec §11.1.

**Spec-Abschnitte:** 11.1, 4.3, 12.1.

**Anforderungen:**
- `const creditBudget = 760`; Guard `const _ = uint(799 - creditBudget)` (Build bricht ab 800); `var cleanupReserve = e2eguard.CleanupReserve(7, 12, 10)` = 62 (7 Belege: 2 Angebote, bis zu 3 Rechnungen, 2 Belege; 12 andere Objekte: Kunde, Lieferant, Person, Projekt, Aufgabe, Artikel, Datenfeld, Zeiteintrag-Fixture, 2 Mitternachtseinträge, Eintrag aus `time log`, Eintrag aus `time stop`).
- Kostentabelle im Header (Reihenfolge = Ausführungsreihenfolge; Dateien alphabetisch, darin Quelltextreihenfolge). Zeilen der Tasks 3–8 tragen „(geplant, Task N)“ und werden von der jeweiligen Task durch die Aufschlüsselung ersetzt, Betrag unverändert:

  | Test | Credits |
  |---|---|
  | CleanupLeftovers (mindestens) | 9 |
  | QuotaHeadersPresent 1, InfoCreditCost 3 | 4 |
  | DeliverPdfEmptyBody | 82 |
  | VatRate019Is19Percent | 62 |
  | PersonDefaultIsString | 1 |
  | UploadMimeTypes | 100 |
  | VoucherStateValues | 3 |
  | PartialUpdateSemantics | 114 |
  | ReplaceLineItems | 4 |
  | InvoiceFlow (geplant, Task 3) | 60 |
  | MasterDataArchive (geplant, Task 6) | 12 |
  | ReadCommands (geplant, Task 6) | 8 |
  | TimeFlow (geplant, Task 5) | 24 |
  | VoucherFlow (geplant, Task 4) | 69 |
  | Carries (13 Subtests 37, Nr. 14 geplant, Task 8: 4) | 41 |
  | OpenMeasurements (geplant, Task 7) | 62 |
  | Aufräumen: 19 Objekte je 1 delete, 3 abgeschlossene Belege je bis zu 3 mehr | 28 |
  | **Summe** | **683** |

  Text darunter: Tests dürfen 760 − 62 = 698 nutzen (geplant 655, Rand 43).
- `meter_test.go`: Beispielwerte `NewMeter(490, 46)` → `NewMeter(760, 62)`, Erwartungen angepasst.
- `e2e.yml`, Schritt `live suite`: `set -o pipefail` und `go test -count=1 -timeout 25m -tags e2e -v ./e2e/... 2>&1 | tee e2e.log`; `id: suite`.
- Neuer Schritt `token check` (`if: always()`, `id: tokencheck`, env `PAPIERKRAM_E2E_TOKEN` aus Secret): fehlt `e2e.log` → `clean=false`; sonst `grep -cFf <(printf '%s\n' "$PAPIERKRAM_E2E_TOKEN") e2e.log` (Token nie in argv); Treffer ≠ 0 → `rm -f e2e.log`, Meldung „Token im Log gefunden: Log gelöscht, kein Upload“, Exit 1; sonst `echo "clean=true" >> "$GITHUB_OUTPUT"`. `shell: bash`.
- Neuer Schritt `summary` (`if: always() && steps.tokencheck.outputs.clean == 'true'`): schreibt `## E2E-Ergebnis` und in einem Codeblock alle Zeilen aus `e2e.log`, die `ABDECKUNG`, `ABWEICHUNG` oder `Credits:` enthalten, nach `$GITHUB_STEP_SUMMARY`.
- Neuer Schritt `log artifact` (`if: always() && steps.tokencheck.outputs.clean == 'true'`): `actions/upload-artifact`, gepinnt per Commit-SHA mit Tag-Kommentar im Format der übrigen Pins (`@<40 hex> # vX.Y.Z`, neueste Release-Version; SHA per `gh api repos/actions/upload-artifact/commits/<tag> --jq .sha`), `name: e2e-log`, `path: e2e.log`, `retention-days: 14`, `if-no-files-found: error`.

**Tests:** `TestMeter…` mit neuen Werten grün; Compile-Guard: lokal einmal `creditBudget = 800` setzen → `go vet -tags e2e ./e2e/...` scheitert (Ausgabe in den Report), zurücksetzen.

**Doku:** Spec §11.1 „Zielwert < 500“ → „Zielwert < 800; Budget 760, davon 62 Aufräumreserve“; `e2e/README.md` (Budget, Workflow-Zusammenfassung, Artefakt 14 Tage, Token-Check); CHANGELOG.

**Akzeptanz:** Gates G1–G7; Header-Summe 683 rechnerisch geprüft (Report zeigt die Addition); Workflow-YAML wird im Review gelesen (kein Probelauf, der Workflow braucht die Sandbox-Secrets); der Token-Check wird lokal mit einer Testdatei und einem Fake-Token `test-token-x` als Shell-Snippet nachgewiesen (Treffer → Exit 1 und Datei gelöscht, kein Treffer → `clean=true`).

**E2E-Kosten:** 0 (nur Budget). Implementer läuft die Live-Suite nicht.

### Task 3: Rechnung live – deliver, pdf, cancel, archive (A3)

**Dateien:** anlegen `e2e/flow_invoice_test.go`; ändern `e2e/e2e_test.go` (Header-Zeile InvoiceFlow), `internal/e2eguard/coverage.go`, `docs/offene-punkte.md` (#1).

**Spec-Abschnitte:** 9.1, 9.2, 8.1, 17; offene Punkte 1, 16.

**Anforderungen:** `TestE2E_InvoiceFlow` (beginnt mit `live(t)`), Schritte mit `-o json`:
1. `s.need(t, 50)`; eigene Rechnung über `s.create` (object `invoice`, Name `<prefix>rechnung-versand`): `invoice create --name <name> --customer <s.customer> --payment-term <erste ID aus s.paymentTerms> --document-date today --item name=pk-e2e-pos-19,qty=1,price=100,vat=19`. MESSUNG „Statuswerte Rechnung“: `state` nach Anlegen.
2. `s.need(t, 2)`; `invoice deliver <id> --via pdf --yes` – **hart** Exit 0; MESSUNG `state`, `invoice_no`, `sent_on`, `sent_via`.
3. `s.need(t, 1)`; `invoice pdf <id> -O <t.TempDir()/rechnung.pdf>` – **hart** Exit 0, Datei beginnt mit `%PDF`; MESSUNG Content-Type und Bytes.
4. `s.need(t, 2)`; `invoice cancel <id> --yes` – Messung (Exit, HTTP, API-Meldung), danach `invoice describe` – MESSUNG `state`.
5. `s.need(t, 3)`; `invoice archive <id>` – **hart** Exit 0; `invoice describe` – MESSUNG `record_state`; `invoice unarchive <id>` – **hart** Exit 0.
6. `s.need(t, 2)`; Angebot aus `s.deliverPDF(t)` (memoisiert, keine neue Anlage im vollen Lauf): `estimate cancel <estimate> --yes` – Messung; `estimate describe` – MESSUNG `state`.
- Summe 60 Credits (+ Fixtures, falls einzeln mit `-run` gestartet).
- CoveragePlan: `Pending` → 0 für DeliverInvoice, GetInvoicePdf, ArchiveInvoice, UnarchiveInvoice (Test `TestE2E_InvoiceFlow`), CancelInvoice, CancelEstimate (ModeAnswered, Note „Statuswechsel; Voraussetzung live unbekannt, nur gemessen“).
- Header: Zeile „InvoiceFlow (geplant, Task 3) 60“ ersetzen durch „InvoiceFlow: invoice 50, deliver 2, pdf 1, cancel + describe 2, archive + describe + unarchive 3, estimate cancel + describe 2 = 60“.

**Tests:** G6 (TestE2ESuiteSource: kein `"email"`, Ausgabe nur über `log_test.go`); `TestCoveragePlan…` grün.

**Doku:** offene-punkte #1: E2E-Spalte ergänzt um `TestE2E_InvoiceFlow` (Statuswerte einer Rechnung im Lebenszyklus, Messung); Ergebnisse trägt der Controller nach Live-Lauf 5 ein. CHANGELOG.

**Akzeptanz:** Gates G1–G6; Report listet jede Anfrage mit Credits.

**E2E-Kosten:** 60 Credits je Lauf; Aufräumen bis zu 3 (delete, cancel, archive). Implementer läuft die Live-Suite nicht.

### Task 4: Beleg live – create --file, document delete, pdf, reverse, cancel, archive (A4)

**Dateien:** anlegen `e2e/flow_voucher_test.go`; ändern `e2e/e2e_test.go` (Header), `internal/e2eguard/coverage.go`.

**Spec-Abschnitte:** 9.3, 9.2, 8.1; offene Punkte 1, 2.

**Anforderungen:** `TestE2E_VoucherFlow`:
1. `s.need(t, 60)`; PDF-Datei aus `minimalPDF` in `t.TempDir()`; über `s.create` (object `voucher`, Name `<prefix>beleg-datei`): `voucher create --name <name> --creditor <s.supplier> --provenance domestic --document-date today --item name=pk-e2e-pos-19,amount=119,vat=19,category=Bürobedarf --file <pfad>` – **hart** Exit 0 (kein Exit 11), POST `/documents` 2xx.
2. `s.need(t, 1)`; `voucher describe <id>` – **hart**: `documents` hat genau einen Eintrag; dessen `id` merken; MESSUNG `keyShape` des Eintrags (ohne Werte).
3. `s.need(t, 1)`; `voucher pdf <id> -O <datei>` – **hart** Exit 0; MESSUNG Content-Type, Bytes, `%PDF`-Präfix.
4. `s.need(t, 1)`; `voucher document delete <id> <dokument-id> --yes` – **hart** Exit 0.
5. `s.need(t, 2)`; `voucher archive <id>`, `voucher unarchive <id>` – **hart** Exit 0.
6. `s.need(t, 2)`; `voucher reverse <id> --yes` – Messung (Exit, HTTP, Meldung); `voucher describe` – MESSUNG `state`.
7. `s.need(t, 2)`; `voucher cancel <id> --yes` – Messung; `voucher describe` – MESSUNG `state`.
- Entscheidung: reverse vor cancel am selben Beleg, kein zweiter Beleg (spart 50 Credits); beide sind Statuswechsel mit unbekannter Voraussetzung und deshalb `ModeAnswered` – die Messung zeigt, welcher Zustand sie erlaubt.
- Summe 69 Credits.
- CoveragePlan: `Pending` → 0 für GetVoucherPdf, DeleteVoucherDocument, ArchiveVoucher, UnarchiveVoucher (ModeSuccess), CancelVoucher, CancelVoucherWithReverseEntry (ModeAnswered, Note „Statuswechsel; Voraussetzung live unbekannt“), Test `TestE2E_VoucherFlow`.
- Header-Zeile VoucherFlow aufschlüsseln (create --file 60, describe 1, pdf 1, document delete 1, archive/unarchive 2, reverse + describe 2, cancel + describe 2 = 69).

**Tests:** G6.

**Doku:** README „Eigenheiten der API“ bleibt bis Task 22; offene-punkte #1 E2E-Spalte um `TestE2E_VoucherFlow` ergänzen. CHANGELOG.

**Akzeptanz:** Gates G1–G6.

**E2E-Kosten:** 69 Credits; Aufräumen bis zu 3. Implementer läuft die Live-Suite nicht.

### Task 5: Zeiterfassung live – time log, Timer, list, archive (A5)

**Dateien:** anlegen `e2e/flow_time_test.go`; ändern `e2e/harness_test.go`, `e2e/e2e_test.go` (Header), `internal/e2eguard/coverage.go`.

**Spec-Abschnitte:** 9.6, 9.8; AGENTS „time stop“.

**Anforderungen:**
- Harness: `execAt(dir string, now func() time.Time, stdin string, args ...string) result` (wie `exec`, aber `App.Now = now`); `exec` ruft `execAt(dir, time.Now, …)`. `runIn(t, dir string, now func() time.Time, stdin string, args ...string) result` wie `run` (Token-Prüfung) mit festem Verzeichnis. Timer-Dateien liegen damit unter `<dir>/cache/papierkram/` des Test-Temp-Verzeichnisses.
- `TestE2E_TimeFlow`, ein `dir := t.TempDir()` für alle Timer-Aufrufe; Uhr `at(h, m)` = heute in `s.loc` um h:m (Datum aus `time.Now().In(s.loc)`), damit nie Mitternacht überschritten und keine Minute gewartet wird:
  1. `s.need(t, 10)`; `time log --task <s.task> --user <s.user> --date heute --from 08:00 --to 08:30 -m <prefix>zeit-log -o json` – **hart** Exit 0; `s.track(object{resource: "time", id, name: <kommentar>})`.
  2. `s.need(t, 1)`; `time list --since heute --until heute -o json` – **hart**: ID aus 1 enthalten.
  3. `s.need(t, 2)`; `time archive <id>`, `time unarchive <id>` – **hart** Exit 0.
  4. Uhr 11:00: `time start --task <task> --user <user> -m <prefix>zeit-timer` – **hart** Exit 0, 0 Credits; `time status -o json` – **hart** `running: true`, `task` = Aufgabe.
  5. `s.need(t, 10)`; Uhr 11:02: `time stop -o json` – **hart** Exit 0; Eintrag registrieren (Name = Kommentar); danach `time status -o json` → `running: false`; erneutes `time stop` → **hart** Exit 2 („Kein Timer läuft“), 0 Credits.
  6. `s.need(t, 1)`; `time describe <stop-id>` – **hart** `started_at`/`ended_at` mit 11:00/11:02 des Tages, `duration` 120.
  7. Verwerfen: Uhr 12:00 `time start …` (0 Credits), `time discard --yes -o json` → **hart** `{"discarded":true}`; `time status` → `running: false`.
- Summe 24 Credits.
- CoveragePlan: ArchiveTimeEntry, UnarchiveTimeEntry `Pending` → 0 (Test `TestE2E_TimeFlow`).
- Header-Zeile TimeFlow aufschlüsseln (log 10, list 1, archive/unarchive 2, stop 10, describe 1 = 24; start/status/discard lokal 0).

**Tests:** G6; `TestE2ESuiteSource` verlangt keine Paketvariable mit Aufruf – Uhr als Funktion im Test bauen.

**Doku:** e2e/README „Was die Suite anlegt“ um zwei Zeiteinträge von heute ergänzen. CHANGELOG.

**Akzeptanz:** Gates G1–G6.

**E2E-Kosten:** 24 Credits; Aufräumen 2 × 1. Implementer läuft die Live-Suite nicht.

### Task 6: Stammdaten archivieren und Lese-Befehle live (A6)

**Dateien:** anlegen `e2e/flow_master_test.go`; ändern `e2e/e2e_test.go` (Header), `internal/e2eguard/coverage.go`, `internal/e2eguard/coverage_test.go`.

**Spec-Abschnitte:** 7.1, 8.1, 10.1 (`auth status`), 10.3.

**Anforderungen:**
- `TestE2E_MasterDataArchive`, Subtests je Ressource (`company` mit `s.supplier`, `project`, `task`, `proposition`, `custom-attribute`, `estimate` mit `s.estimate`): `s.need(t, 2)`; `<res> archive <id> -o json` und `<res> unarchive <id> -o json` – **hart** Exit 0; MESSUNG Antwortform (`record_state` aus der Ausgabe oder „leere Antwort“ bei `{"id":N,"ok":true}`). Summe 12.
- `TestE2E_ReadCommands`, Subtests:
  - `person_list`: `person list --company <s.customer> -o json` – **hart**: Person des Laufs enthalten (1).
  - `user_describe`: `user describe <s.user>` – **hart** `id` gleich (1).
  - `payment_term_describe`: `payment-term describe <erste ID>` – **hart** `id` gleich (1).
  - `bank_connection`: `bank-connection list --limit 1 -o json` – **hart** Exit 0 (1); MESSUNG nur `total_entries` und `keyShape` des ersten Eintrags. Liste leer → `s.coverage.NotApplicable("GetBankConnection", "keine Bankverbindung im Sandbox-Account (gemessen)")`; sonst `bank-connection describe <id>` – **hart** Exit 0 (1).
  - `transaction`: gleiches Muster mit `transaction list --limit 1` und `transaction describe`, Grund „kein Bankumsatz im Sandbox-Account (gemessen)“.
  - `auth_status`: `auth status -o json` – **hart** Exit 0, `quota.remaining` ganze Zahl, `read_only` false, `token_source` `env` (1).
  - Summe 6 bis 8; geplant 8.
- Bankdaten nie mit Werten loggen (Global Constraints).
- CoveragePlan: alle `Pending: 6` → 0 (Tests `TestE2E_MasterDataArchive/<res>` bzw. `TestE2E_ReadCommands/<sub>`).
- Header-Zeilen MasterDataArchive (6 × 2 = 12) und ReadCommands (person 1, user 1, payment-term 1, bank-connection 1–2, transaction 1–2, auth status 1 = höchstens 8) aufschlüsseln.

**Tests:** G6; `TestCoveragePlanModes` unverändert grün.

**Doku:** e2e/README (Bankdaten: nur Anzahl und Schlüsselform). CHANGELOG.

**Akzeptanz:** Gates G1–G6.

**E2E-Kosten:** 20 Credits. Implementer läuft die Live-Suite nicht.

### Task 7: Offene Messungen #28, #10, #13, #12 und vollständiger CoveragePlan (A7)

**Dateien:** anlegen `e2e/measure_open_test.go`; ändern `e2e/e2e_test.go` (Header), `internal/e2eguard/coverage_test.go`, `docs/offene-punkte.md`.

**Spec-Abschnitte:** 17; offene Punkte 10, 12, 13, 28.

**Anforderungen:** `TestE2E_OpenMeasurements`, Subtests (alle Messungen; Abweichungen per `deviatef`):
- `28_InvoiceWithoutDocumentDate`: `s.need(t, 50)`; `invoice create --name <prefix>rechnung-ohne-datum --customer <C> --payment-term <PT> --item name=pk-e2e-pos-19,qty=1,price=100,vat=19 -o json` **ohne** `--document-date` über `s.run`. Exit 0 → Objekt registrieren (`s.track`), MESSUNG `document_date` der Antwort; sonst MESSUNG Exit, HTTP und API-Meldung (422 erwartet wie beim Angebot).
- `10_InvoiceDueDatePut`: an `s.invoice`: `s.need(t, 3)`; `invoice describe` (payment_term.id, due_date); `api PUT /income/invoices/<id> --input -` mit `{"due_date":"<heute+45 Tage>","payment_term":{"id":<aktuelle>}}`; `invoice describe`. MESSUNG HTTP und due_date vorher/nachher. Endet `api` lokal mit Exit 5 (nichts gesendet), MESSUNG „lokal abgelehnt“ und Subtest-Ende.
- `13_InvoiceSupplyDate`: `s.need(t, 5)`; `invoice update <id> --supply-date 2026-11-01` (2: liest die Rechnung); `invoice describe` (1) → MESSUNG Format von `supply_date`; `invoice update <id> --input -` mit genau dieser Ausgabe (1, payment_term im Body → kein Lesen); `invoice describe` (1) → `deviatef`, wenn der Rundlauf scheitert oder `supply_date` sich ändert.
- `12_VoucherEntertainmentPersons`: an `s.voucher`: `s.need(t, 4)`; `voucher update <id> --input -` mit `{"entertainment_persons":["<prefix>gast"]}`; abgelehnt → MESSUNG und Ende; sonst `describe`, dann `{"entertainment_persons":[]}`, `describe`; MESSUNG „geleert“ oder „unverändert“.
- Summe 62 Credits.
- `TestCoveragePlanComplete` (neu): kein Eintrag mit `Pending != 0`.
- Header-Zeile OpenMeasurements aufschlüsseln (50 + 3 + 5 + 4 = 62); Header-Summe unverändert 683.

**Doku:** offene-punkte Übersicht und Einzelpunkte 10, 12, 13, 28: Spalte E2E-Test = jeweiliger Subtest, Art „Messung“; „nicht gemessen“ entfällt; Ergebnisse trägt der Controller nach Live-Lauf 5 ein. CHANGELOG.

**Akzeptanz:** Gates G1–G6; `TestCoveragePlanComplete` grün.

**E2E-Kosten:** 62 Credits; Aufräumen bis zu 3, falls #28 eine Rechnung anlegt. Implementer läuft die Live-Suite nicht.

---

## Phase B – Entscheidungen umsetzen

### Task 8: Punkt 29 – `invoice update --customer` mit gleichem Kunden behält Kontaktperson und Projekt (B1)

**Dateien:** ändern `internal/cli/keep_payment_term.go`, `internal/cli/builder_write.go` (Kommentar `writeSteps`), Tests `internal/cli/keep_payment_term_test.go`; `e2e/measure_carries_test.go`, `e2e/e2e_test.go` (Header); Spec §9.4a, README, AGENTS, `docs/offene-punkte.md` #29.

**Spec-Abschnitte:** 9.4, 9.4a, 6.3, 6.3a.

**Anforderungen:**
- Lesebedarf von `keepPaymentTerm.apply`: `needTerm` = `payment_term` fehlt im fertigen Body (wie heute); `needRefs` = Kunden-Flag (`fieldFlag(ws, "customer")`) gesetzt **und** im Body fehlt `customer.contact_person` oder `customer.project` **und** deren Flags (`--contact-person`, `--customer-project`, per `fieldFlag`) sind nicht gesetzt. Gelesen wird einmal, wenn `needTerm || needRefs`.
- Mit dem gelesenen Dokument bei `needRefs`: `spec.Prune(doc, ws.req, spec.ResourceResponseSchema(e.OperationID))` **ohne** `pruneHints` → `base`; `a.dropForeignRefs(c, ws, base, "der Rechnung", customerRefs)`; danach jedes in `base.customer` verbliebene `contact_person`/`project` in `body.customer` übernehmen, wenn dort nicht gesetzt. Gleicher Kunde → Hinweis `Hinweis: Kontaktperson <id> und Projekt <id> der Rechnung mitgesendet (gleicher Kunde; ohne sie leert die API beide)` (nur die vorhandenen nennen). Anderer Kunde → der Hinweis von `dropForeignRefs` („Kunde X statt Y aus der Rechnung: … nicht übernommen (setzen per …)“), nichts übernommen.
- `needTerm` setzt `payment_term` wie heute; danach erneut `spec.Validate`.
- Credits: Lesen nur bei Bedarf; `--dry-run` zeigt `read` und `credits` 2, auch bei `--payment-term` mit `--customer` ohne Referenz-Flags. `commands --json` bleibt 2.
- Hilfetexte: `keepPaymentTermLong` um einen Satz „Mit --customer ohne --contact-person/--customer-project liest das CLI die Rechnung ebenfalls: beim gleichen Kunden werden Kontaktperson und Projekt mitgesendet, bei einem anderen entfallen sie (Hinweis auf stderr).“; Usage von `--payment-term`: „; spart das Lesen der Rechnung (1 Credit), außer mit --customer ohne --contact-person/--customer-project“.
- Fehler beim Lesen: wie heute „Rechnung %d nicht gelesen, nichts geändert: …“.

**Cross-Pfad-Tabelle (Original `keepPaymentTerm` und `dropForeignRefs`-Aufrufer vs. neu):**

| Parameter | Original | Neu (Refs) | Begründung |
|---|---|---|---|
| Lese-Eintrag | `registry.Lookup(e.Resource, "describe")` | derselbe `k.read` | ein Lesen für beide Zwecke |
| Vorschau/Call | `client.Preview` / `client.Call(ctx, k.read.OperationID, params, nil, nil)` | identisch | – |
| Fehler beim Lesen | Code des Lesens, „nichts geändert“ | identisch | – |
| JSON-Prüfung | `UseNumber`, `atEOF`, Objekt | identisch | – |
| Prune | `--input`: `spec.Prune(in, ws.req, resp)` + `pruneHints` | `spec.Prune(doc, ws.req, resp)` ohne Hints | Dokument ist keine Nutzereingabe |
| `dropForeignRefs` src | „--input“ / „dem Angebot“ | „der Rechnung“ | Herkunft im Hinweis |
| refs | `customerRefs` bzw. `movedCustomerRefs(moved)` | `customerRefs` | beide Referenzen |
| Flags gewinnen | ja (Body nach Flags) | ja (nur fehlende übernehmen) | – |
| Credits/dry-run | `ReadInfo`, `run.Credits += read.Credits` | identisch, einmal | – |
| Validate | nach `payment_term` | nach beiden Ergänzungen | – |

**Tests (rot zuerst):** gleicher Kunde → PUT-Body mit `customer{id,contact_person{id},project{id}}`, Hinweis; anderer Kunde → nur `customer{id}`, Hinweis „Kunde … aus der Rechnung“; `--contact-person` gesetzt → nicht überschrieben; `--payment-term` + `--customer` → genau 1 GET; `--payment-term` ohne `--customer` → 0 GET; `--dry-run` → `read` und `credits` 2; Rechnung ohne Kontaktperson/Projekt → nichts übernommen, kein Hinweis; Lesefehler 404 → Exit 4 ohne PUT; read-only → Exit 9 ohne GET.

**E2E (hart):** `TestE2E_Carries/14_InvoiceUpdateSameCustomer`: `s.need(t, 4)`; `invoice describe` (contact_person.id, project.id, customer.id); `invoice update <id> --customer <gleiche ID> -o json`; `invoice describe` → beide IDs unverändert, sonst `errorf`. Header-Zeile Carries: „(geplant, Task 8)“ entfernen, 41.

**Doku:** Spec §9.4a (Absatz Punkt 29), README „Eigenheiten der API“ (Rechnung ändern), AGENTS „Rundlauf mit Kundenwechsel“ und Credits-Satz zu `invoice update`, offene-punkte #29 → „entschieden und umgesetzt (Task 8), Live-Prüfung `TestE2E_Carries/14_InvoiceUpdateSameCustomer`“; CHANGELOG; `make docs`.

**Akzeptanz:** Gates G1–G6.

**E2E-Kosten:** 4 Credits (im Budget von Task 2). Implementer läuft die Live-Suite nicht.

### Task 9: Punkt 27 – `company` als Voreinstellung für `person … --company` (B2)

**Dateien:** ändern `internal/registry/table.go` (5 person-Einträge `Preset: "company"`), `internal/registry/registry.go` (Doku `Preset`), `internal/cli/builder.go` (`pathValues` → Methode `(a *App) pathValues`, Pfad-Flag-Registrierung), alle Aufrufer von `pathValues`, `internal/cli/preset.go` (`presetArgs` nur bei `e.Preset == e.Resource`), `internal/cli/local.go` (Text), Tests in `internal/cli`; Spec §9.8, README, AGENTS, `docs/offene-punkte.md` #27.

**Spec-Abschnitte:** 9.8, 7.2 (`commands --json`), 10.1.

**Anforderungen:**
- `registry.Entry.Preset` deckt zusätzlich das Pfad-Flag gleichen Namens ab. Für person list|create|describe|update|delete gilt `Preset: "company"`.
- `presetArgs` füllt die Positions-ID nur, wenn `e.Preset == e.Resource` (heute: project describe); für person bleibt die Person-ID Pflicht.
- `(a *App) pathValues`: Pfad-Flag `p.flag == e.Preset` und nicht gesetzt → `a.preset(e.Preset)`; mit Wert → `presetHint`, Wert verwenden; ohne Wert → Exit 2 `Pflichtangabe --company fehlt`, Hinweis `--company angeben oder voreinstellen: PAPIERKRAM_COMPANY oder company in .papierkram.toml (papierkram local init --company <id>)`. Ungültiger Env-Wert → Exit 2 wie bei den anderen Presets (`presetValue`).
- Priorität und Quellen exakt wie bestehende Presets: Flag > `PAPIERKRAM_COMPANY` > `company` der Projektdatei (Kontextregel von `presetFile`, Warnung einmal) > keine (kein Kontext-Default).
- Registrierung: Pfad-Flag mit Preset → `markPreset(fs, "company", "company", false)` (`default_from` = `flag, env, projectfile`, `env` = `PAPIERKRAM_COMPANY`); Usage „ID des übergeordneten Objekts (company_id); ohne Angabe PAPIERKRAM_COMPANY oder company aus der Projektdatei .papierkram.toml (Pflicht)“; `required` bleibt `true` (wie `time log --task`).
- Jeder Runner (list, action, write) gibt die vorgemerkten Preset-Hinweise per `flushPresetHints` vor dem Senden aus; fehlende Aufrufe ergänzen, Reihenfolge der Prüfungen unverändert (Preset-Fehler Exit 2 vor dem Schreibschutz-Gate wie bei `presetArgs`).
- `local show`: Zeile `company` nicht mehr „nur Anzeige“, sondern „genutzt von person … --company“; `local init`-Hilfe entsprechend.

**Cross-Pfad-Tabelle (Original Presets `task create --project` / `project describe` / `task list --project` vs. neu `person --company`):**

| Parameter | Original | Neu | Begründung |
|---|---|---|---|
| Schlüssel | `project`/`task`/`user` | `company` | – |
| Auflösung | `a.preset(key)` → `resolvePreset` | identisch | – |
| Quellen | flag, env, projectfile, context nur bei `user` | flag, env, projectfile | `company` hat keinen Kontext-Default |
| Projektdatei-Kontextregel | `presetFile` (Warnung einmal) | identisch | – |
| Wertprüfung | `presetValue` (ID ≥ 0, Exit 2) | identisch | – |
| Hinweis | `presetHint` + `flushPresetHints` | identisch | – |
| Annotation | `markPreset(…, withContext)` | `withContext=false` | wie Listenfilter |
| Ort der Anwendung | Positions-ID / Listenfilter / Body-Feld | Pfad-Flag | Pfadparameter |
| Fehlt der Wert | `presetArgs`-Meldung bzw. `withPresetHint` | Meldung oben, Exit 2 | gleiche Struktur |

**Tests:** je person-Aktion: Env setzt `--company` (Request-Pfad enthält ID, Hinweis auf stderr); Projektdatei setzt sie; Flag gewinnt; Projektdatei eines anderen Kontexts → Warnung, Exit 2; ungültiger Env-Wert → Exit 2 ohne Request; `person describe` ohne ID bleibt Exit 2; `commands --json` → `default_from`, `env` für `person list --company`; `project describe` ohne ID funktioniert unverändert; read-only + dry-run unverändert.

**Doku:** Spec §9.8 „Genutzt von …“ um `person list|create|describe|update|delete` (`--company`); README Projektdatei-Abschnitt (Zeilen 163/165 neu), AGENTS Hinweis IDs/Voreinstellungen, offene-punkte #27 → „entschieden und umgesetzt (Task 9)“, `skills/papierkram/SKILL.md:124` „(Projekt, Aufgabe, Benutzer)“ → „(Firma, Projekt, Aufgabe, Benutzer)“; CHANGELOG; `make docs`.

**Akzeptanz:** Gates G1–G5.

**E2E-Kosten:** 0 (keine Live-Prüfung; reine CLI-Logik).

### Task 10: Punkt 22 – Ende um Mitternacht (B3)

**Dateien:** ändern `internal/cli/time_log.go` (`parseClock`, `checkPeriod`, `hhmm`, Hilfe), `internal/cli/time_timer.go` (`timerPeriod`, Hilfe), `api/overlay.yaml` (Abschnitt 14), `internal/spec/openapi.gen.json` (per `go generate`), Tests, txtar `time_log.txtar`; `e2e/measure_carries_test.go` (Carry 12), `e2e/e2e_test.go` (Header-Text hart/Messung); Spec §9.6, README, AGENTS, `docs/offene-punkte.md` #22.

**Spec-Abschnitte:** 9.6, 4.4.

**Anforderungen:**
- `time log`: `--to 24:00` und `--to 00:00` bei Beginn > 00:00 bedeuten Ende um Mitternacht des Folgetags (`p.end = 1440`); gesendet wird `ended_at_time` **`"24:00"`** (eindeutig als Tagesende). `--from 24:00` bleibt ungültig (Exit 2, Meldung unverändert). `--from 23:00 --duration 1h` → Ende 1440, erlaubt.
- `checkPeriod`: `p.end == 1440` erlaubt; `p.end > 1440` weiter Exit 2 „endet am Folgetag um …“; `--from 00:00 --to 00:00` bleibt „Dauer 0“.
- `hhmm(1440)` = `"24:00"`.
- `time stop`: Stoppt der Timer am Folgetag zwischen 00:00:00 und 00:00:59, bucht er bis `24:00` des Starttags; sonst bleibt „über Mitternacht“ Exit 2.
- Overlay Abschnitt 14: getrennte Targets für `started_at_time` (Muster unverändert `^([01][0-9]|2[0-3]):[0-5][0-9]$`) und `ended_at_time` (`^(([01][0-9]|2[0-3]):[0-5][0-9]|24:00)$`, `x-cli-pattern-format: 'HH:MM (bis 24:00)'`), je POST und PUT; Kommentar mit Messung Lauf 2.
- `time create|update` (generischer Builder) bekommen keine neue Zeitraumprüfung (heute keine; `checkPeriod` gilt nur für `time log|stop`); sie nehmen `--ended-at-time 24:00` dank Muster an.
- Hilfe `time log`: Satz „Einträge über Mitternacht werden abgelehnt“ → „Ein Ende genau um Mitternacht (--to 24:00 oder 00:00) ist erlaubt; andere Einträge über Mitternacht werden abgelehnt“; `midnightHint` bleibt sinngemäß.

**Tests:** `--from 23:00 --to 00:00` → Body `ended_at_time` `"24:00"`; `--to 24:00` gleich; `--from 23:00 --to 00:30` → Exit 2; `--from 24:00` → Exit 2; `--from 23:00 --duration 1h` → `24:00`; `--from 23:30 --duration 1h` → Exit 2; Timer Start 23:00, Stopp 00:00:30 Folgetag → 23:00–24:00; Stopp 00:01 → Exit 2; mockapi akzeptiert `24:00` (Overlay); `time create --ended-at-time 24:00` → gesendet; `--started-at-time 24:00` → Exit 5.

**E2E (hart):** Carry `12_MidnightEnd` über CLI statt `api`: für `00:00` und `24:00` je `s.need(t, 11)` (unverändert); `time log --task <t> --user <u> --date 2026-06-10 --from 23:00 --to <end> -m <prefix>mitternacht-<0000|2400> -o json` → **hart** Exit 0 und `duration` 3600 in der Ausgabe von `time log` (API-Objekt, keine weitere Anfrage); registrieren; MESSUNG `ended_at` und gesendetes `ended_at_time`. Header: Nr. 12 in die Liste „hard“.

**Doku:** Spec §9.6 (Regel neu, Ruling E2E-Mitternacht ersetzt), README Zeiterfassung (`README.md:186`), offene-punkte #22 → „entschieden und umgesetzt (Task 10)“, Art „hart“; CHANGELOG; `make docs`.

**Akzeptanz:** Gates G1–G6; `go generate ./...` ohne weiteren Diff außer `openapi.gen.json`.

**E2E-Kosten:** unverändert (2 × 11 = 22, schon im Budget). Implementer läuft die Live-Suite nicht.

### Task 11: Timer-Claim gegen paralleles `time stop` (C2) (B4)

**Dateien:** ändern `internal/timer/timer.go`, `internal/timer/timer_test.go`, `internal/cli/time_timer.go`, `internal/cli/time_log.go` (`sendTimeEntry`), Tests in `internal/cli`; Spec §9.6, README, AGENTS (Exit 7, time stop).

**Spec-Abschnitte:** 9.6, 9.3 (Muster „Timer bleibt erhalten“), 10.4.

**Anforderungen:**
- `timer`-Paket: `StoppingPath(path string) string` = `path + ".stopping"`; `Claim(path string) error`: existiert `StoppingPath` → `ErrClaimed`; sonst `os.Rename(path, StoppingPath(path))`; `fs.ErrNotExist` → `ErrClaimed` (ein anderer Stopp war schneller); `Release(path string) error`: Rename zurück; `LoadStopping(path) (*State, error)`: liest `StoppingPath` mit den Regeln von `Load` (nil ohne Datei).
- `time stop`: Claim **nach** Bestätigung und `--dry-run` (also nicht im Probelauf), **vor** dem POST: `sendTimeEntry` bekommt einen optionalen Hook `beforeCall func() error`, aufgerufen zwischen Bestätigung und `client.Call`; `time log` übergibt nil.
- Claim scheitert mit `ErrClaimed` → Exit 7 `Timer wird bereits von einem anderen papierkram time stop gebucht`, Hinweis `Nichts gesendet. Mit papierkram time status prüfen, ob er noch läuft`.
- Fehler nach dem Claim (jeder Code, auch 12 und 5xx) → `Release`, dann `keepTimer` wie heute; scheitert `Release` → Exit 1 mit Meldung „Timer-Datei nicht zurückgestellt: …“ und Hinweis, `<pfad>.stopping` in `<pfad>` umzubenennen oder `papierkram time discard`.
- Erfolg → `timer.Delete(StoppingPath)`; scheitert das → Exit 11 wie heute, Pfad = `.stopping`-Datei.
- Rest-Claim (Absturz): `loadTimer` prüft `LoadStopping` zuerst. `time start` und `time stop` → Exit 7 `Ein time stop wurde nicht abgeschlossen (Timer seit <Start>, <Aufgabe/Benutzer>)`, Hinweis `Erst mit papierkram time list --since <Startdatum> prüfen, ob der Eintrag angelegt wurde; wenn ja: papierkram time discard; sonst papierkram time discard und von Hand buchen mit <timeLogCommand>`. `time status` → Exit 0, Ausgabe des Claims mit Warnung auf stderr (gleicher Text). `time discard` → räumt Hauptdatei und `.stopping` (Bestätigung wie heute; nennt beide).
- Windows: `os.Rename` scheitert, wenn das Ziel existiert – deshalb die Vorab-Prüfung in `Claim`; Test läuft auf allen CI-Plattformen.

**Tests:** zwei parallele Stopps gegen mockapi (Barriere im Mock vor der Antwort) → genau 1 POST, einer Exit 0, einer Exit 7; Fehler 422 nach Claim → Datei wieder da, `.stopping` weg; Exit 12 nach Claim → Datei wieder da, Hinweis unverändert; Dry-run → kein Claim; Bestätigung abgelehnt (TTY) → kein Claim; Rest-Claim → start/stop Exit 7, status Exit 0 + Warnung, discard entfernt beide; Löschfehler `.stopping` → Exit 11; timer-Paket: Claim/Release/LoadStopping-Einheitentests inkl. Symlink-Regel.

**Doku:** Spec §9.6 („time stop beansprucht den Timer atomar; ein zweiter Stopp endet mit Exit 7“), README Zeiterfassung, AGENTS Exit-7-Zeile („409, Timer läuft schon oder wird gerade gestoppt“) und „time stop“-Abschnitt; CHANGELOG; `make docs`.

**Akzeptanz:** Gates G1–G5; `go test -race ./internal/timer/... ./internal/cli/...` mehrfach (`-count=5`) grün.

**E2E-Kosten:** 0 (Task 5 deckt `time stop` live ab).

### Task 12: `time start --unbillable` (B5)

**Dateien:** ändern `internal/timer/timer.go` (`State.Unbillable *bool` mit `json:"unbillable,omitempty"`), `internal/cli/time_timer.go`, `internal/cli/time_log.go` (gemeinsamer Flag-Helper), Tests; `e2e/flow_time_test.go`; Spec §9.6, README.

**Spec-Abschnitte:** 9.6.

**Anforderungen:**
- Gemeinsamer Helper `addUnbillableFlag(fs)` (heute inline in `newTimeLogCmd`): `rawValue{typ: "bool"}`, `NoOptDefVal "true"`, Usage „nicht abrechenbar“; `time log` und `time start` nutzen ihn.
- `time start`: Flag gesetzt → Wert per `spec.CoerceFlag` mit dem Feld `unbillable` aus `timeLogFieldsOf` sofort prüfen (Fehler Exit 5, Timer nicht gestartet) und als `*bool` speichern; nicht gesetzt → nil.
- `time stop`: `st.Unbillable != nil` → `te.unbillable = "true"|"false"`; nil → nicht gesendet (alte Timer-Dateien unverändert gültig).
- `time status`/`time start` JSON: neues Feld `unbillable` (bool, `false` bei nil); Tabelle `wide` Spalte `NICHT ABRECHENBAR` (`ja`/leer).
- Keine `--unbillable` an `time stop` (Start legt fest; Begründung: Abrechenbarkeit steht beim Start fest, nur der Kommentar wird am Ende oft ergänzt).

**Cross-Pfad-Tabelle (Original `time log --unbillable` vs. neu `time start --unbillable` → `time stop`):**

| Parameter | time log | time start/stop | Begründung |
|---|---|---|---|
| Flag-Definition | rawValue bool, NoOptDefVal true, „nicht abrechenbar“ | identisch (Helper) | – |
| Werte | true/false/ja/nein über CoerceFlag | identisch | – |
| Prüfzeitpunkt | beim Body-Bau vor dem Senden (Exit 5) | beim Start (Exit 5) | Fehler nicht erst Stunden später |
| Speicherung | – | `*bool` im State | geprüfter Wert statt Rohtext |
| Gesendet | nur wenn Flag gesetzt | nur wenn im State gesetzt | gleiche Semantik |
| Body-Bau | `timeLogBody` → CoerceFlag → Validate | identisch (Text „true“/„false“) | – |

**Tests:** start `--unbillable` → State `true`, stop sendet `unbillable: true`; `--unbillable=false` → sendet `false`; ohne Flag → kein Feld; ungültiger Wert → Exit 5, keine Datei; alte State-Datei ohne Feld → stop ohne Feld; status JSON/wide.

**E2E (hart):** `TestE2E_TimeFlow` Schritt 4: `time start … --unbillable`; Schritt 6: `time describe` → `unbillable` true, sonst `errorf`. Keine neuen Credits.

**Doku:** Spec §9.6 Syntax `time start [--task] [-m] [--unbillable]`; README; CHANGELOG; `make docs`.

**Akzeptanz:** Gates G1–G6.

**E2E-Kosten:** 0 zusätzlich. Implementer läuft die Live-Suite nicht.

---

## Meilenstein: Live-Lauf 5 (Controller)

Nach Task 12, spätestens 2026-11-14. Der Controller (nicht ein Implementer) führt die Suite lokal mit dem Befehl aus `HANDOFF.md` §2 gegen `<sandbox>` aus und prüft danach: `grep -c` des Tokens im Log = 0; Zeile `ABDECKUNG: abgedeckt …/89` mit nur begründeten Ausschlüssen; `Credits: … von 760`; Restkontingent notiert. Ergebnisse (Statuswerte #1, #10, #12, #13, #28, Bank-Ausschlüsse, Antworten von reverse/cancel) trägt er in `docs/offene-punkte.md`, README „Eigenheiten der API“ und AGENTS ein; nötige Anpassungen (z. B. Overlay `document_date` Pflicht bei Rechnungen, Moduswechsel im CoveragePlan, Kostenkorrekturen) laufen als Fix-Agent mit eigenem Brief. Danach einmal `gh workflow run e2e.yml -f sandbox_confirm=<sandbox>` und Prüfung von Step-Summary und Artefakt. Puffer für Lauf 6 bis 2026-11-18. Erwarteter Verbrauch je Lauf ≈ 683 Credits.

---

## Phase C – Spec-Versprechen und Robustheit

### Task 13: JSON-Schemas für Liste, ok-Ausgabe, `auth status`, `version` (C1)

**Dateien:** anlegen `docs/schemas/list.json`, `docs/schemas/ok.json`, `docs/schemas/auth-status.json`, `docs/schemas/version.json`; ändern `internal/cli/contract_test.go`; AGENTS (Tabelle JSON-Vertrag), Spec §10.3.

**Spec-Abschnitte:** 10.3, 11 (Vertrag).

**Anforderungen:**
- Format wie die bestehenden Schemas (OpenAPI-3-Schema-Objekte, strikt, `additionalProperties: false`).
- `list.json`: `type` (string), `page`, `page_size`, `total_pages`, `total_entries` (integer), `has_more` (boolean), `entries` (array von Objekten mit beliebigen Eigenschaften); alle Pflicht.
- `ok.json`: `oneOf` aus `{"id": integer ≥ 0, "ok": true}` (beide Pflicht) und `{"ok": true}`.
- `auth-status.json`: `context`, `context_source`, `subdomain`, `base_url`, `token_source`, `read_only` (boolean), `read_only_source`, `quota` (`remaining` integer, `request_cost` integer|null, `known` boolean); alle Pflicht.
- `version.json`: `version`, `commit`, `date`, `go_version` (string), Pflicht.

**Tests (contract_test.go, gegen echte Ausgabe über mockapi):** `invoice list -o json`, `invoice list --all -o json` → list.json; `company archive <id> -o json` mit leerer Mock-Antwort → ok.json (Form mit `id`); `company create … -o json` mit leerer Antwort → ok.json (Form ohne `id`); `auth status -o json` → auth-status.json; `version -o json` → version.json; jedes Schema lehnt ein zusätzliches Feld ab (Negativtest).

**Doku:** AGENTS-Tabelle „JSON-Vertrag“: Spalte Schema für `list`, `{"id":N,"ok":true}`, `auth status`, `version`; Spec §10.3 Satz „Schemas in docs/schemas/: …“ ergänzen; CHANGELOG.

**Akzeptanz:** Gates G1–G5.

### Task 14: Längeres Zeitlimit für Uploads (C2)

**Dateien:** ändern `internal/api/retry.go` (bzw. `client.go`: Zeitlimit je Aufruf), `internal/cli/upload.go`, `internal/cli/root.go` (Flag-Hilfe `--timeout`), Tests; Spec §5.5, §9.3, README, AGENTS.

**Spec-Abschnitte:** 5.5, 9.3.

**Anforderungen:**
- Konstante `uploadTimeout = 120 * time.Second`.
- Uploads (`voucher upload`, Upload-Schritt von `voucher create --file`) nutzen ohne `--timeout` je Versuch `max(wirksames Zeitlimit aus Kontext bzw. Standard 30 s, 120 s)`; mit `--timeout` genau diesen Wert (`a.Flags.TimeoutSet`).
- Umsetzung über eine Client-Kopie mit anderem Zeitlimit (`(*api.Client).WithTimeout(d) *api.Client`), nur für den Upload-Aufruf; alle anderen Anfragen unverändert.
- `timeoutHint` lautet „größere Dateien brauchen ein höheres --timeout (Uploads ohne --timeout: 120 s)“.
- `--timeout`-Hilfe: „Zeitlimit je Anfrage, pro Versuch (Uploads ohne Angabe mindestens 120 s); GET wird bis zu zweimal wiederholt“.

**Tests:** Upload ohne `--timeout` gegen langsamen Mock (Deadline des Requests ≥ 119 s geprüft über den Kontext im Test-Transport, ohne zu warten); mit `--timeout 5s` → 5 s; Kontext `timeout = "200s"` → 200 s; `voucher create --file` Upload-Schritt 120 s, Anlage-Schritt 30 s; Zeitüberschreitung → Hinweis neu.

**Doku:** Spec §5.5 und §9.3, README (Upload), AGENTS (Upload-Hinweis), CHANGELOG; `make docs`.

**Akzeptanz:** Gates G1–G5.

### Task 15: Antwortgröße begrenzen (C3)

**Dateien:** ändern `internal/api/retry.go`, `internal/api/errors.go`, Tests `internal/api`; Spec §5.5.

**Spec-Abschnitte:** 5.5, 10.4.

**Anforderungen:**
- `const MaxResponseSize = 64 << 20`; gelesen wird `io.LimitReader(hr.Body, MaxResponseSize+1)`.
- Mehr als 64 MiB:
  - Nicht-2xx-Status: Status entscheidet wie heute (gekürzter Body, Code nach Status).
  - 2xx bei GET/HEAD: Exit 1 (`error`) „Antwort größer als 64 MiB, abgebrochen“, Hinweis „mit --fields/--limit kleinere Antworten anfordern“; keine Wiederholung.
  - 2xx bei POST/PUT/DELETE: Exit 1 „Antwort größer als 64 MiB; die Änderung wurde ausgeführt“, Hinweis „NICHT wiederholen; Ergebnis per describe/list prüfen“.
- `--debug` zeigt die Kürzung („gekürzt nach 64 MiB“).

**Tests:** 2xx-GET mit 64 MiB + 1 Byte (Mock-Stream ohne Speicherexplosion) → Exit 1, keine Wiederholung (1 Request); POST → Exit 1 mit Text „Änderung wurde ausgeführt“; 404 mit großem Body → Exit 4; genau 64 MiB → Erfolg.

**Doku:** Spec §5.5 (Grenze), README Fehlertabelle nicht nötig; CHANGELOG.

**Akzeptanz:** Gates G1–G5.

### Task 16: `commands --json` mit `default`, `minimum`, `maximum` (C4)

**Dateien:** ändern `internal/cli/commands.go`, `internal/cli/builder_list.go` (Annotationen), `docs/schemas/commands.json`, Tests `internal/cli/commands_test.go`, `contract_test.go`; AGENTS, Spec §7.2.

**Spec-Abschnitte:** 7.2.

**Anforderungen:**
- `flagInfo` neu: `Default string json:"default,omitempty"`, `Minimum *float64 json:"minimum,omitempty"`, `Maximum *float64 json:"maximum,omitempty"`.
- `default` = pflag `DefValue`, ausgelassen, wenn es der Nullwert des Typs ist (dieselbe Nullwert-Prüfung, mit der die Hilfe in `templates.go` entscheidet, ob ein Standardwert gezeigt wird).
- `minimum`/`maximum` kommen aus Flag-Annotationen `pk:minimum`/`pk:maximum`, gesetzt aus denselben Konstanten, mit denen die Werte geprüft werden: `--limit` 1/100 (`maxLimit`), `--page` 1, `--max-pages` 1; zusätzlich aus `minimum`/`maximum` des Spec-Schemas eines Feld- oder Filter-Flags, falls die Spec sie nennt (heute keine; spec-getrieben vorbereitet).
- `docs/schemas/commands.json`: Flag-Objekt um die drei optionalen Eigenschaften erweitert; `version` bleibt 1.

**Tests:** `invoice list` → `--limit` `default` „25“, `minimum` 1, `maximum` 100; `--page` `minimum` 1, ohne `default`; `--timeout` (global) `default` „30s“; Bool-Flag ohne `default`; Schema-Vertragstest grün; Fall „unbekannte Klasse“ unverändert.

**Doku:** AGENTS Abschnitt „Selbstbeschreibung“ (Flag-Felder), Spec §7.2; CHANGELOG; `make docs`.

**Akzeptanz:** Gates G1–G5.

---

## Phase D – Minors, Refactor, CI, Doku

### Task 17: Nutzersichtbare Minors I – Fehler und Eingabe (D1)

**Dateien:** `internal/cli/root.go`, `internal/cli/templates.go`, `internal/cli/builder_write.go`, `internal/cli/items.go`, `internal/spec/coerce.go`, `internal/spec/prune.go`, `internal/cli/prompt.go`, `internal/cli/confirm.go`, `internal/cli/builder_list.go`, `internal/cli/auth.go`, jeweilige Tests; Spec §10.4, README Exit-Tabelle, AGENTS.

**Spec-Abschnitte:** 10.4, 6.1–6.4.

**Anforderungen (je Fundstelle → Ergebnis):**
1. L41 `root.go:26-38, 87-103`: Jeder Usage-Fehler hat einen Hinweis: Flag- und Argumentfehler `Verwendung: <useLine(c)>`, `unknownCommand` `Befehle anzeigen: <Pfad> --help`.
2. L41 `root.go:71-75` `englishCauses`: alle pflag-/strconv-/time-Ursachen deutsch. Test über **alle** Flags aller Commands (Typen aus den Annotationen): je Typ ein ungültiger Wert; Meldung enthält keines von `invalid`, `parsing`, `syntax`, `argument`, `flag needs`, `out of range`.
3. L123 `templates.go:50-56, 117-120`: `NoOptDefVal` wie pflag als `[="wert"]` ohne Go-Escaping (Umlaute roh); `englishFlagTypeRe` deckt alle verwendeten Typnamen ab – Test über alle Commands: Hilfe enthält keinen englischen Typnamen.
4. L213 `builder_write.go:415-417`: `line_items: null` in `--input` mit `--replace-items` **und** `--item/--items` → Flags gewinnen, kein Fehler; ohne Positions-Flags Meldung `line_items: null ist nicht erlaubt – für leere Positionsliste [] angeben oder Positionen per --item/--items setzen` (Exit 5).
5. L213 `items.go:385` `itemRefs`: Schlüssel sortiert durchlaufen (deterministische Fehlermeldung bei mehreren ungültigen Referenzen).
6. L179 `coerce.go:434-442` `showValue`: JSON-Darstellung (`{}`, `[]`, `{"a":1}`), keine Go-Syntax.
7. L179 `coerce.go:356-361`: Boolean aus `--input` nimmt JSON-Zahl `0`/`1` als false/true an; andere Zahlen Exit 5 wie heute.
8. L179 `coerce.go:230`: Beispiel im Hinweis nur `--attr 12=Wert`, wenn `12` zum Muster passt; sonst Hinweis `Schlüssel muss zum Muster <muster> passen`.
9. L188 `prune.go:342-356` `jqPath`: Schlüssel mit `.`, `[`, `'`, Leerzeichen oder Nicht-Bezeichner-Zeichen als `.["<JSON-quotiert>"]`.
10. L195 `items.go:166-177` `showItem`: Unicode-Format-Zeichen (Cf: Bidi, Zero-Width) escaped wie `printable`; Spaltennummer in Runen (Test mit `›` in der Eingabe, `items_test.go:132` ergänzen).
11. L202 `prompt.go:15` **Entscheidung:** Abbruch einer Rückfrage per Strg+C → **Exit 8** (`confirmation_required`), Meldung `Abgebrochen: Eingabe durch Unterbrechung beendet`, Hinweis `Nichts gesendet`. Begründung: Alle Rückfragen liegen vor jeder Änderung (Bestätigung, Token-Eingabe, `local init`, Seitenabfrage nach gelesener Seite 1); nichts wurde gesendet, ein Mensch hat abgebrochen – das ist Spec §10.4 Zeile 8 („Bestätigung fehlt/abgelehnt“), nicht ein Netzwerkfehler (Exit 10 hieße für Agents „später erneut versuchen“). Kommentare `confirm.go:91`, `builder_list.go:465`, `auth.go:288` angleichen. Unterbrechung **während** einer Anfrage bleibt Exit 10/12.
12. L202 Echo-Race `prompt.go:37-41`: bewusst nicht behoben; Kommentar dokumentiert das Restrisiko (Fenster zwischen Start der Lese-Goroutine und Echo-Aus von `term.ReadPassword`, Mikrosekunden; eine Behebung bräuchte einen eigenen Raw-Mode-Zeileneditor).

**Tests:** je Punkt ein roter Test; Punkt 11: Interrupt bei Bestätigung, Token-Eingabe, `--all`-Rückfrage → Exit 8 und JSON-Fehler `code` `confirmation_required`.

**Doku:** Spec §10.4 Zeile 8 „Bestätigung fehlt/abgelehnt oder Rückfrage mit Strg+C abgebrochen“, Zeile 10 ohne Strg+C; README Exit-Tabelle (Zeilen 8 und 10); AGENTS Exit-Tabelle Zeile 8; CHANGELOG (Exit-Änderung unter „Changed“); `make docs`.

**Akzeptanz:** Gates G1–G5.

### Task 18: Nutzersichtbare Minors II – Ausgabe, Meldungen, Config (D1)

**Dateien:** `internal/output/table.go`, `internal/output/printable.go`, `internal/output/columns.go`, `internal/output/jq.go`, `internal/cli/auth.go`, `internal/cli/deliver.go`, `internal/cli/from_estimate.go`, `internal/projectfile/projectfile.go`, `internal/api/errors.go`, `internal/cli/context.go`, Tests.

**Spec-Abschnitte:** 10.3, 10.4, 9.1, 9.4, 9.8, 10.1.

**Anforderungen (je Fundstelle → Ergebnis):**
1. L158 `output/table.go:229-233`: Zelle nur aus Leerraum → leere Zelle (keine Leerzeile, Tabelle bleibt einzeilig je Eintrag).
2. L158 `output/printable.go:43-45`: U+2028 und U+2029 werden wie andere Zeilentrenner escaped.
3. L165 `output/columns.go:107-113` `FmtIDNameList`: Einträge ohne ID und Namen werden übersprungen; keine leeren Teile („, ,“).
4. L149 `auth.go:431-438`: scheitert `/info` (Exit 3/4) und zusätzlich die Ausgabe (`--jq`-Laufzeitfehler, Render), gewinnt der Code des Aufrufs; der Ausgabefehler erscheint als Warnung auf stderr.
5. L149 `output/jq.go:71-73`: Ergebnis-Obergrenze wird beim Iterieren geprüft (Abbruch beim ersten Ergebnis über der Grenze, nicht erst nach dem Sammeln).
6. L280 `deliver.go:181-190`: Hinweis „Versand fehlgeschlagen …“ (`deliverFailed`) nur, wenn die deliver-Anfrage gesendet wurde; lokale Fehler davor behalten Meldung und Hinweis ohne diesen Zusatz.
7. L310 `from_estimate.go:200`: Null-Hinweis nennt keine Felder, die per Flag gesetzt wurden (Test `from_estimate_test.go:534` angepasst).
8. L81 `projectfile.go:190`: Lesefehler nach dem Finden als `clierr` (`error`, Exit 1) mit Hinweis `Projektdatei <pfad> prüfen oder entfernen` (Pfad `printable`).
9. L104 `api/errors.go:136`: `DNSError` mit leerem `Name` ohne „()“.
10. L120 `context.go:106, 346`: `context create`/`context rename` melden `ErrUnavailable` des Schlüsselbunds als Warnung auf stderr (gleicher Text wie bei `auth logout`), statt still fortzufahren; Exit-Code unverändert.

**Tests:** je Punkt ein roter Test (Golden nur, wo die Ausgabe sich gewollt ändert; im Report begründen).

**Doku:** CHANGELOG; `make docs`.

**Akzeptanz:** Gates G1–G5.

### Task 19: Interne und Test-Minors (D2)

**Dateien:** `internal/clierr/clierr.go`, `internal/cli/root_test.go`, `internal/cli/context.go`, `internal/cli/context_test.go`, `internal/config/config.go`, `internal/config/resolve_test.go`, `go.mod`, `internal/projectfile/*_test.go`, `internal/api/debug_test.go`, `internal/spec/spec.go`, `internal/spec/spec_test.go`, `internal/spec/overlay_test.go`, `internal/testutil/mockapi/*`, `internal/cli/items_test.go`, `internal/cli/builder_test.go`, `internal/cli/income_test.go`, Golden `invoice-list.wide`, `internal/cli/commands_test.go`.

**Anforderungen (je Fundstelle → Ergebnis):**
1. L41 `clierr.go:80-103`: `From` erkennt cobra-Argumentfehler nicht mehr per String-Präfix; Test belegt, dass alle Argumentfehler aus den deutschen Validatoren stammen (Code-Grep-Test: kein `cobra.ExactArgs|MinimumNArgs|RangeArgs|NoArgs` in `internal/`).
2. L37 `root_test.go:80`: `TestVersionText` prüft das exakte Textformat.
3. L41 `root_test.go:286`: Kurz-Flag-Fall `-x` mit exaktem Meldungsvergleich.
4. L139 `context.go:114, 150, 302, 363`: `a.save` statt `cfg.Save`; Tests: Rename-Rollback nach Save-Fehler, Warnung bei `ErrUnavailable` (Task 18 Punkt 10), `auth logout` mit Save-Fehler, `context active` ohne Kontext (L86).
5. L77 `config.go:81-110`: `Save` macht `f.Sync()` vor dem Umbenennen und auf Nicht-Windows `Sync` des Verzeichnisses danach.
6. L77 `go.mod:16-24`: zweiter direkter `require`-Block in den ersten zusammengeführt; `go mod tidy` ohne Diff.
7. L77 `resolve_test.go:35`: Prioritätstest als Tabelle über alle Kombinationen (Flag, Env, Projektdatei, Config) für Kontext, Ausgabeformat, Zeitlimit, Schreibschutz.
8. L81 projectfile: Tests für relativen cwd, Symlink auf die Datei, leeren und doppelten Alias in `[tasks]`.
9. L118 `debug.go:132`: Tests Kürzung ungültigen UTF-8 an der 4-KB-Grenze und Token neben ungültigem Byte (Schwärzung greift).
10. L59 `overlay_test.go:570-574` Map ohne ungenutzte Werte; `:504-506` Nil-Prüfung `extensionPaths`; `spec.go:25, 55-60`: nach Panik in `load` liefert jeder weitere Aufruf dieselbe Panik (gespeicherter Fehler) statt nil-Maps; `spec_test.go:91` ID-Erzeugung ohne `rune`-Umweg; `:220` TestQueryParams prüft Vorrang.
11. L66 `spec_test.go:238` Test über öffentliche API statt White-Box; `goType` int/int32-Zweige getestet.
12. L128 `mockapi_test.go:541-546` Closures mit eigenem `t`; `mockapi.go:207-231` Violation bei unbekannter Route behält den `url.Parse`-Fehler; Violation-Texte einheitlich Deutsch.
13. L202 `builder_test.go:359-361`: `pagedUsers`-Mock mit `page_size`, gültiges JSON.
14. L235 `income_test.go:487-488`: `--payment-term` per Name statt Index entfernen; Golden `invoice-list.wide` mit echten Datumswerten (`sent_on`, `paid_at_date`); CLI-Test für den Hinweis beim Entfernen von `custom_template: null`.
15. L368 `commands_test.go:164`: Fall „unbekannte Klasse“ prüft den exakten Text.

**Tests:** wie oben; reine Testergänzungen dürfen grün starten (im Report kennzeichnen).

**Doku:** CHANGELOG (nur intern, ein Sammeleintrag).

**Akzeptanz:** Gates G1–G5.

### Task 20: Gemeinsame Helper für leere 2xx-Antworten und Body-Aufbau (D3, Ruling F-M9)

**Dateien:** `internal/cli/builder_action.go`, `internal/cli/builder_write.go`, `internal/cli/pay.go`, `internal/cli/upload.go`, `internal/cli/deliver.go`, neu `internal/cli/result.go` (Helper); keine Teständerungen, neue Tests nur für die Helper selbst.

**Anforderungen:**
- Helper `(a *App) emptyOK(c *cobra.Command, r *config.Resolved, result any, text string) error`: Ausgabe von `result` im wirksamen Format bzw. `text` bei eigenem Textformat (`ownText`), alles über `afterRequest`. Die vier Kopien (`builder_action.go:73-86`, `builder_write.go:291-314`, `pay.go:146-155`, `upload.go:105-114`) rufen ihn auf; nicht-leere Antworten weiter `a.render(c, body, e.Columns)`.
- Helper `(a *App) prunedInput(c *cobra.Command, req, resp *openapi3.Schema) (spec.PruneResult, bool, error)`: `--input` lesen, `spec.Prune`, Fehlerpräfix `--input: `, `pruneHints`; `bool` = `--input` gesetzt. Genutzt von `buildBodyFrom` (`builder_write.go:387-471`), `deliverBody` (`deliver.go:495-532`), `payBody` (`pay.go:186-222`); die Schritte danach bleiben je Pfad eigen.
- Kein Verhalten ändert sich: keine Golden-, txtar- oder Test-Erwartung wird angepasst.

**Cross-Pfad-Tabelle (leere 2xx-Antwort):**

| Parameter | action | write | pay | upload |
|---|---|---|---|---|
| ID | `params[len-1]` | `params[len-1]` (update) | `lastID(params)` | `lastID(params)` |
| Ergebnis | `deletedJSON` (delete) / `okJSON` | `okJSON` / `{"ok":true}` + Warnung (create) | `okJSON` | `okJSON` |
| Text | `%s %d %s.` (`simpleActions`) | `%s %d aktualisiert.` / `%s angelegt.` | `%s %d: Zahlung erfasst.` | `%s %d: Datei hochgeladen.` |
| Format | `outputFormat(r)` | identisch | identisch | identisch |
| Fehler | `afterRequest` | identisch | identisch | identisch |

Alle Unterschiede bleiben als Argumente erhalten; ID-Ermittlung einheitlich `lastID(params)` (gleiches Ergebnis, `params` nie leer an diesen Stellen – im Report belegen).

**Cross-Pfad-Tabelle (Body-Aufbau):**

| Schritt | buildBodyFrom | deliverBody | payBody | Begründung bleibt |
|---|---|---|---|---|
| Request-Schema | `ws.req` | `RequestSchema(op)` | `RequestSchema(op)` | gleiche Quelle |
| Response-Schema für Prune | `ResourceResponseSchema(op)` | **nil** | `ResourceResponseSchema(op)` (nil) | deliver: gewollt, jedes Feld außerhalb des Requests unbekannt |
| Vor Prune | `line_items` ohne `--replace-items` entfernen | – | – | nur Positionen |
| Nach Prune | null-Positionen, `dropForeignRefs` | `email: null` entfernen | – | je Pfad eigen |
| Flags | `flagSets` + Positionen | `applyDeliverFlags` | `payFlags` + `CoerceFlag` | je Pfad eigen |
| Coerce/Validate | `CoerceValue`, `presetBody`, `Validate` | `CoerceValue`, `checkDeliverBody` (Exit 2), `Validate` | `CoerceValue`, `hasPayment` (Exit 5), `Validate` | je Pfad eigen |

**Tests:** bestehende Suite unverändert grün; Report zeigt `git diff --stat` ohne Testdateien bzw. nur neue Helper-Tests.

**Doku:** keine Nutzerdoku; CHANGELOG nicht nötig.

**Akzeptanz:** Gates G1–G5; Coverage nicht gesunken.

### Task 21: CI – `go mod verify`, Go-Version aus einer Quelle, Dependabot für Go-Module (D4)

**Dateien:** anlegen `.go-version`; ändern `.github/workflows/ci.yml`, `.github/workflows/e2e.yml`, `.github/workflows/release.yml`, `.github/dependabot.yml`; Spec §12.1.

**Anforderungen:**
- `.go-version` mit Inhalt `1.27.x` (eine Zeile); alle sechs `actions/setup-go`-Schritte (ci: test, keychain, checks, snapshot; e2e; release) nutzen `go-version-file: .go-version` statt `go-version`. Begründung: `go.mod` nennt die Mindestversion 1.26.0, gebaut wird mit 1.27.x; eine eigene Datei trennt beides, und setup-go liest bei `.go-version` den Inhalt als Versionsangabe.
- `checks`-Job: Schritt `go mod verify` direkt nach setup-go.
- `dependabot.yml`: zweiter Eintrag `package-ecosystem: gomod`, `directory: /`, `schedule.interval: weekly`.
- Action-Pins unverändert.

**Tests:** `sh scripts/check-action-pins.sh`; `go mod verify` lokal grün.

**Doku:** Spec §12.1 (Go-Version aus `.go-version`, `go mod verify`, Dependabot gomod); README bleibt („Go 1.26 oder neuer“ ist die Mindestversion aus `go.mod`); CHANGELOG.

**Akzeptanz:** Gates G1, G2, G5, G7; Hinweis an Pascal im Report: lokales Go auf 1.27.3 heben (govulncheck), nicht Teil der Agent-Arbeit.

### Task 22: Doku-Sync (D5)

**Dateien:** `README.md`, `AGENTS.md`, `skills/papierkram/SKILL.md`, `CHANGELOG.md`, `docs/offene-punkte.md`, `docs/reference/` (generiert), Spec (nur Querverweise).

**Anforderungen:**
- README „Eigenheiten der API“ (`README.md:347-356`): Upload-Satz nennt `voucher create --file` → Exit 11 (Hochladen unbekannt bzw. gescheitert) und `voucher upload` → Exit 1; Punkte 29, 27, 22 und Timer-Claim (C2) je ein Satz; Ergebnisse von Live-Lauf 5, soweit der Controller sie in `offene-punkte.md` eingetragen hat.
- README Ressourcen-Übersicht: alle Ressourcen inkl. `bank-connection`, `custom-attribute`, `proposition`, `transaction` mit je einer Zeile (Name, Alias, Zweck).
- AGENTS: Credits-Liste der lokalen Befehle vollständig (`commands`, `schema`, `context`, `local`, `version`, `completion`, `time start`, `time status`, `time discard`); Exit 7 (Timer-Claim), Exit 8 (Strg+C), `person --company`-Voreinstellung, `invoice update --customer`; `time start --unbillable`; JSON-Vertrag gegen `docs/schemas/` abgleichen (jede Schemadatei in der Tabelle genannt).
- `skills/papierkram/SKILL.md`: Workflows an Punkte 22/27/29 und Timer-Regeln angepasst.
- CHANGELOG `[Unreleased]`: Einträge aller Tasks konsolidiert (Added/Changed/Fixed), keine Doppelungen.
- `docs/offene-punkte.md`: Übersichtstabelle konsistent mit Einzelpunkten (Stand, Art, Testname).
- Alle Beispiele bestehen TestDocCommandsAreValid; `make docs` ohne Diff.

**Tests:** TestDocCommandsAreValid, TestReferenceUpToDate.

**Akzeptanz:** Gates G1–G5; Report listet je Datei die geänderten Abschnitte.

---

## Abschluss (Controller)

Gesamt-Review über alle Commits dieser Runde (opus, `superpowers:requesting-code-review`), Fix-Welle per Fix-Agent, danach Gates G1–G7 und ein letzter Live-Lauf, solange die Sandbox besteht (vor 2026-11-19), mit Token-Check und Abdeckungszeile. `HANDOFF.md` und Memory aktualisieren (Stand, Restkontingent, offene Punkte, Phase F als nächster Brainstorm). Kein Release, kein Tag, kein Deployment ohne ausdrücklichen Auftrag von Pascal.
