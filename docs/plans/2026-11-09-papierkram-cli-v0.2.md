# papierkram-cli v0.2.0 – Implementierungsplan

> **Für agentische Ausführung:** PFLICHT-SUB-SKILL: `superpowers:subagent-driven-development` (empfohlen) oder `superpowers:executing-plans`. Je Task: Brief → Implementer (model opus, TDD) → Reviewer (model opus) → Fix-Runde → Ledger. Sequenziell: Tasks 2–7 teilen `internal/cli`, Task 7 setzt 1–6 voraus. Schritte mit Checkbox-Syntax (`- [ ]`).

**Ziel:** v0.2.0 mit großzügiger Subdomain-Eingabe, Namensauflösung in ID-Flags, `invoice paid-status`, `papierkram mcp serve` (Tools aus der Registry, ein Ausführungspfad), sparsamerem E2E-Aufräumen, SBOM und winget-Manifest – ohne dass ein Agent über MCP mehr kann als über das CLI.

**Architektur:** Alles neue Verhalten hängt an bestehenden Pfaden. Die Namensauflösung ist ein Schritt zwischen Schreibschutz-Gate und Body (`resolveNames`), der Flag-Werte in IDs umschreibt; danach läuft der unveränderte Builder. `paid-status` lädt Seiten über `fetchPage` und die Filter der Registry wie `invoice list --all`. MCP besteht aus einem SDK-nahen Paket `internal/mcpserver` (Tool-Schema, Argument-Übersetzung, Ergebnis), das das CLI nicht importiert, und `internal/cli/mcp*.go`, das die Tool-Beschreibungen aus `commands --json` (`buildCommandsDoc`) erzeugt und jeden Aufruf in-process über `RunContext` mit eigener `App` ausführt.

**Tech Stack:** Go 1.27.x (`.go-version`, `go.mod` 1.26.0), cobra/pflag, kin-openapi, `github.com/modelcontextprotocol/go-sdk` (neu, ≥ v1.7.0), GoReleaser v2.19.0 mit syft, GitHub Actions.

**Spec:** `docs/specs/2026-11-09-papierkram-cli-v0.2-design.md` (bindend; **[Call]** ist entschieden). Sie ergänzt die Haupt-Spec `docs/specs/2026-11-03-papierkram-cli-design.md`, die im selben Zug angepasst wird. Vertrag für Agents: `AGENTS.md`. Stil und Gates: Vorgängerplan `docs/plans/2026-11-07-papierkram-cli-ausbau.md`.

Zuordnung Spec → Tasks: §1 → 1; §4 → 2 + 3 (MCP-Teil in 5); §5 → 4; §3 → 5 + 6; §2 und E2E-Teil von §7 → 7; §6 → 8; §7 (Doku, Spec-Abgleich) → 9, je Task vorab mitgezogen.

## Global Constraints

Gilt für jede Task zusätzlich zu den Abschnitten „Global Constraints“, „Exit-Codes“, „Reihenfolge der Prüfungen“ des Plans `docs/plans/2026-11-03-papierkram-cli.md`.

**Bindend aus Runde 1 und 2 (verdichtet):**
- Conventional Commits, Englisch, klein; **nie** `Co-Authored-By` oder andere Attribution; Identität aus der globalen Git-Config; nicht pushen (Controller), kein Amend/Rebase/Force.
- TDD: roter Test (Ausgabe im Report), Code, grün.
- Nutzersichtbare Texte Deutsch mit echten Umlauten; Englisch nur Command-/Flag-Namen, JSON-Schlüssel, Fehlercodes, Code-Kommentare, Commits. Hinweise im Stil der bestehenden: `Hinweis: …` auf stderr, Werte in „…“ über `printable`.
- Tokens nie in Logs, `--debug`, Fehlern, Fixtures, Golden-Files, Tool-Ergebnissen; Test-Tokens `test-token-…`.
- Kein Request gegen echte Accounts; Binary bzw. `go run ./cmd/papierkram` nur mit `PAPIERKRAM_BASE_URL` auf 127.0.0.1/mockapi (Ausnahme: rein lokale Befehle). **Live-E2E läuft nur der Controller**, nie Implementer oder Reviewer.
- Keine Wiederholung bei POST/PUT/DELETE oder 429; leerer 2xx-Body ist nie ein Fehler.
- Spec-getrieben: keine handgepflegten Feldlisten; `api/openapi.json` nie ändern (Korrekturen nur in `api/overlay.yaml`, danach `go generate ./...`).
- Keine `cobra.ExactArgs/…`, kein `MarkFlagRequired`; deutsche Validatoren, Usage-Hinweis über `useLine(c)`.
- Mutationen nur über `mutationClient` → Body → `beforeMutation`/`beforeSend`, Annotation `pk:class`; Reihenfolge der Prüfungen **Exit 2 → 9 → Auflösung → 5 → dry-run → 8 → senden** (die Namensauflösung ist neu und liegt nach dem Gate, Task 3).
- Fehler nach gesendetem Request nie Exit 2, sondern Exit 1 mit Präfix „Befehl ausgeführt, Ausgabe fehlgeschlagen“ (`afterRequest`). Ausnahme mit Begründung: Mehrdeutigkeit der Namensauflösung ist Exit 2 (Spec §4), es wurden nur Leseanfragen gesendet.
- Keine neuen `TestRNN_`-Namen (`TestRegressionIndex` bleibt 39/39). Keine neue API-Operation (Registry bleibt 89 Einträge, `CoveragePlan` unverändert).
- Akzeptierte Rulings nicht anfassen: L292 (`pdf_owner_unix.go:14`), L335 (`local.go:429-431`).
- E2E-Suite: nie `pay` mit echter Transaktion, nie `deliver --via email`, kein Literal `"email"` in `e2e/`; Ausgabe nur über `e2e/log_test.go`; jede neue Live-Anfrage im Kosten-Header von `e2e/e2e_test.go` mit `s.need(t, n)` davor; Objekte heißen `pk-e2e-<runid>-…` (Ausnahme Task 7: `pk-e2e-dauerkunde`) und werden über `s.create`/`s.track` registriert.

**Neu in v0.2:**
- Version: nur Minor-Bump auf **v0.2.0**; kein Tag, kein Release in einer Task (Controller, siehe Abschluss).
- MCP-SDK: **offizielles `github.com/modelcontextprotocol/go-sdk`**, neueste stabile Version zum Zeitpunkt der Task (mindestens v1.7.0), stdio; Lizenz aus dem Modulcache lesen und in `tools/gennotices/main.go` (`licenses`) eintragen, `THIRD_PARTY_NOTICES.md` neu erzeugen. Keine weitere neue direkte Abhängigkeit.
- MCP darf nie mehr als das CLI: jeder Tool-Aufruf läuft durch `RunContext` mit `-o json --no-input`; kein zweiter Prüf- oder Sendepfad.
- `commands --json` bleibt `version` 1; neue Felder nur additiv und optional; jedes neue JSON hat ein striktes Schema in `docs/schemas/` mit Vertragstest.
- Namensauflösung: höchstens 5 Seiten à 100 je Flag; nie eine Wahl bei Mehrdeutigkeit oder unvollständiger Liste.
- **Cross-Pfad-Disziplin:** Wer einen bestehenden Pfad spiegelt (MCP ↔ CLI, Namensauflösung ↔ Vervollständigungs-Lookup, paid-status ↔ `list --all`), liefert im Report die Tabelle seiner Task Parameter für Parameter (Original vs. neu) und begründet jede Abweichung; unbegründete Abweichung = Bug.
- **Doku mitziehen:** je Task die betroffenen Abschnitte von Spec, README, AGENTS, SKILL, `docs/offene-punkte.md`; Eintrag unter `## [Unreleased]` in `CHANGELOG.md` (Abschnitte „Hinzugefügt“/„Geändert“); danach `make docs` ohne Diff. Doku-Beispiele mit Namen werden von `TestDocCommandsAreValid` gegen die mockapi ausgeführt: Namen nur verwenden, wenn die Standardantwort der mockapi sie eindeutig enthält, sonst Zeile mit `# Platzhalter` markieren.

**Gates (in jeder Task unter „Akzeptanz“ referenziert):**
- **G1** `go test -race ./...` grün.
- **G2** `golangci-lint run ./...` **und** `golangci-lint run --build-tags=e2e ./...` ohne Befunde.
- **G3** `go vet ./...` und `go vet -tags e2e ./...` sauber; `gofmt -l .` leer.
- **G4** `scripts/coverage.sh 80` grün; Gesamtwert nicht unter dem Stand vor der Task (Ist 94,5 %; Report nennt vorher/nachher).
- **G5** `go generate ./... && make docs && git status --porcelain` leer (TestReferenceUpToDate, TestDocCommandsAreValid grün).
- **G6** (E2E) `go test ./internal/e2eguard/...` grün (u. a. TestE2ESuiteSource) und `go test -count=1 -tags e2e ./e2e/...` ohne `PAPIERKRAM_E2E_*` → jeder Test `SKIP`, Lauf `PASS`.
- **G7** (Workflows) `sh scripts/check-action-pins.sh` grün.
- **G8** `sh scripts/check-go-version.sh` grün.
- **G9** (Release) `go run github.com/goreleaser/goreleaser/v2@v2.19.0 check` und `make snapshot` grün (braucht `syft` im PATH, lokal `brew install syft`).
- **G10** (Abhängigkeiten) `go mod tidy && git diff --exit-code go.mod go.sum`; `go run ./tools/gennotices -check`; `go mod verify`.

## Review Focus

Fünf Eingaben bzw. Fehlerlagen, die die Spec impliziert, die aber leicht durchrutschen (wahrscheinlichste zuerst); jede hat ihren Test in der genannten Task:

1. **stdout von `mcp serve` trägt nur JSON-RPC.** Jede direkte Schreibe auf `os.Stdout`/`os.Stderr` im Code zerstört den Client; erwartet: Tool-Ausgaben landen nur im Tool-Ergebnis, Hinweise und `--debug` nur auf dem Prozess-stderr. → Task 5: `TestNoDirectStdio`, `TestMCPServeStdioOnlyProtocol`.
2. **Tool-Argumente, die wie Flags aussehen** (`"--yes"`, `"-o"`, `"--context=x"`, Dateipfad `-x.pdf`, Name `--dry-run`): erwartet, dass sie Werte bleiben und nie eine Option setzen. → Task 5: `TestMCPArgsNoFlagInjection`.
3. **Namen in vielen Schreibweisen und Grenzfällen**: Groß-/Kleinschreibung, mehrfacher Leerraum, reine Ziffern als Name, archivierte Dubletten, mehr als 500 Einträge; erwartet: genau ein aktiver Treffer oder Abbruch, nie eine stille Wahl. → Task 2: `TestResolveCaseAndSpace`, `TestResolveArchivedIgnored`, `TestResolveIncompleteList`, `TestIsIDValue`.
4. **Beträge der Liste als Text oder Zahl mit beliebig vielen Stellen** (`"226.0"`, `0.1` + `0.2`), `due_date` `null`, `outstanding_amount` `"0.0"`: erwartet exakte Summe auf den Cent, kein Absturz, Rechnung ohne Fälligkeit nie überfällig. → Task 4: `TestPaidStatusDecimalSum`, `TestPaidStatusNullDueDate`.
5. **Parallele oder abgebrochene Tool-Aufrufe** eines MCP-Clients: erwartet keine Datenrennen (Aufrufe serialisiert) und bei Abbruch dieselbe Exit-Semantik wie Strg+C (10 bzw. 12). → Task 5: `TestMCPConcurrentCallsSerialized`, `TestMCPCancelledCall`.

## Reihenfolge

1. Task 1 – Subdomain (unabhängig, klein).
2. Task 2 → 3 – Namensauflösung. **Abweichung vom Vorschlag (8 Tasks):** Die Namensauflösung ist in zwei Tasks geteilt. Task 2 liefert den Resolver mit Cache und den Cross-Pfad zur Vervollständigung (prüfbar ohne Befehlsänderung), Task 3 bindet ihn in die Befehle, `--dry-run` und `commands --json` ein. Ein Reviewer kann die Prüfreihenfolge von Task 3 ablehnen, ohne den Resolver zu verwerfen.
3. Task 4 – `invoice paid-status` (nutzt den Resolver für `--customer`).
4. Task 5 → 6 – MCP: erst nur Lese-Tools (sicherer Zwischenstand), dann Modi, Schreibschutz, Cross-Pfad-Test und MCP-Doku.
5. Task 7 – E2E: Aufräumen und Abdeckung der neuen Features (braucht 1–6).
6. Task 8 – SBOM und winget (unabhängig; spät, damit der Snapshot den fertigen Stand baut).
7. Task 9 – Doku-Sync.
8. Abschluss (Controller, kein Task).

---

### Task 1: Subdomain großzügig annehmen (Spec §1)

**Files:**
- Modify: `internal/config/subdomain.go` (neue Funktion neben `NormalizeSubdomain`)
- Modify: `internal/cli/context.go:77-84` (`context create --subdomain`), `internal/cli/auth.go:81-86` (`auth login --subdomain`)
- Test: `internal/config/subdomain_test.go`, `internal/cli/context_test.go`, `internal/cli/auth_test.go`
- Doku: Haupt-Spec §10.1, README „Kontexte“, `CHANGELOG.md`

**Interfaces:**
- Consumes: `config.NormalizeSubdomain(value, source string) (string, error)` (strikte Prüfung, unverändert).
- Produces: `config.ParseSubdomain(value, source string) (sub string, normalized bool, err error)` – lockere Eingabe für Flags; `normalized` ist true, wenn Schema, Pfad, Abfrage/Fragment, Punkt am Ende oder Suffix `.papierkram.de` entfernt wurden (nicht bei bloßem Trimmen/Kleinschreiben). `(a *App) subdomainFlag(raw string) (string, error)` in `internal/cli/context.go`: ruft `ParseSubdomain(raw, "--subdomain")` und schreibt bei `normalized` den Hinweis.

- [ ] **Step 1: Failing Tests schreiben**

`internal/config/subdomain_test.go`, `TestParseSubdomain` (Tabelle, Ergebnis und `normalized`):

```go
{"pkrason", "pkrason", false}, {" PKRASON ", "pkrason", false},
{"pkrason.papierkram.de", "pkrason", true}, {"PKRASON.Papierkram.DE", "pkrason", true},
{"https://pkrason.papierkram.de", "pkrason", true}, {"https://pkrason.papierkram.de/", "pkrason", true},
{"HTTP://pkrason.papierkram.de/login?x=1#a", "pkrason", true}, {"pkrason.papierkram.de.", "pkrason", true},
```

`TestParseSubdomainRejects` (Exit 2, `code` `usage`, Meldung `ungültige Subdomain "<eingabe>" (--subdomain)` wie bisher): `pkrason.papierkram.de:443` und `https://pkrason.papierkram.de:8443/` (Hinweis enthält „ohne Port“), `firma.example.com` (Hinweis unverändert „Nur die Subdomain angeben, z. B. firma (für https://firma.papierkram.de)“), `https://`, `.papierkram.de`, `a.b.papierkram.de`, `https://user@pkrason.papierkram.de`, `ftp://pkrason.papierkram.de`, `""`.

`internal/cli/context_test.go`:
- `TestContextCreateSubdomainURL`: `context create firma --subdomain https://pkrason.papierkram.de` → Exit 0, Kontext hat `subdomain = "pkrason"`, stderr genau `Hinweis: Subdomain „pkrason“ aus „https://pkrason.papierkram.de“\n`.
- `TestContextCreateSubdomainPlainNoHint`: `--subdomain Pkrason` → `pkrason`, stderr leer.
- `TestResolveSubdomainStaysStrict`: `config.toml` mit `subdomain = "pkrason.papierkram.de"` → `invoice list` Exit 2 ohne Anfrage; `PAPIERKRAM_SUBDOMAIN=https://pkrason.papierkram.de` → Exit 2 ohne Anfrage (Ruling unten).

`internal/cli/auth_test.go`, `TestAuthLoginSubdomainURL`: neuer Kontext per `auth login --context neu --subdomain pkrason.papierkram.de --token-stdin` (Token per stdin, `/info` über mockapi wie in den bestehenden Login-Tests) → Kontext `pkrason`, derselbe Hinweis.

- [ ] **Step 2: Tests laufen lassen, rot**

Run: `go test ./internal/config/ ./internal/cli/ -run 'Subdomain' -v`
Expected: FAIL (`ParseSubdomain` undefiniert).

- [ ] **Step 3: `ParseSubdomain` implementieren**

Reihenfolge: trimmen; Schema `http://`/`https://` (ohne Groß-/Kleinschreibung) entfernen, ein anderes `://` ist ein Fehler; ab dem ersten `/`, `?` oder `#` abschneiden; enthält der Rest `@` → Fehler mit dem Hinweis von `NormalizeSubdomain`; enthält er `:` → Fehler mit Hinweis `Ohne Port angeben, z. B. firma (für https://firma.papierkram.de)`; einen Punkt am Ende entfernen; Suffix `.papierkram.de` ohne Groß-/Kleinschreibung entfernen; Rest an `NormalizeSubdomain(rest, source)` – aber die Fehlermeldung nennt immer die **ursprüngliche** Eingabe.

- [ ] **Step 4: Aufrufer umstellen**

`context create` und `auth login` rufen `a.subdomainFlag(subdomain)`; Hinweistext exakt `Hinweis: Subdomain „%s“ aus „%s“` mit `printable(raw)` (getrimmt). `config.Resolve` (Konfiguration und `PAPIERKRAM_SUBDOMAIN`) bleibt bei `NormalizeSubdomain`.

- [ ] **Step 5: Tests grün**

Run: `go test ./internal/config/ ./internal/cli/ -run 'Subdomain|AuthLogin|ContextCreate' -v`
Expected: PASS.

- [ ] **Step 6: Doku**

Haupt-Spec §10.1: Satz „`--subdomain` nimmt auch `firma.papierkram.de` oder die URL; Konfigurationswert und `PAPIERKRAM_SUBDOMAIN` bleiben strikt“. README „Kontexte“: ein Satz mit Beispiel `papierkram context create firma --subdomain https://firma.papierkram.de`. CHANGELOG „Hinzugefügt“.

- [ ] **Step 7: Gates und Commit**

Run: G1–G5. Dann:

```bash
git add internal/config/subdomain.go internal/config/subdomain_test.go internal/cli/context.go internal/cli/auth.go internal/cli/context_test.go internal/cli/auth_test.go docs/specs/2026-11-03-papierkram-cli-design.md README.md CHANGELOG.md docs/reference
git commit -m "feat(config): accept subdomain as host or url"
```

**Ruling (Spec offen):** `PAPIERKRAM_SUBDOMAIN` bleibt strikt wie der Konfigurationswert: Beide werden bei jedem Aufruf gelesen, ein Hinweis je Aufruf wäre Rauschen, und die Spec nimmt „Konfigurationswert beim Laden“ ausdrücklich aus.

**Akzeptanz:** Tests oben grün; Gates G1–G5.

---

### Task 2: Namensauflösung – Resolver, Cache, Zuordnung (Spec §4)

**Files:**
- Create: `internal/registry/names.go`, `internal/registry/names_test.go`
- Create: `internal/cli/resolve.go`, `internal/cli/resolve_test.go`
- Modify: `internal/cli/completion_ids.go` (Cache-Format, `lookupIDs`, Kürzung erst bei der Anzeige), `internal/cli/completion_test.go`
- Doku: keine (nutzersichtbar erst mit Task 3)

**Interfaces:**
- Consumes: `registry.Lookup(resource, "list")`, `fetchPage(fetch pageFunc, query url.Values, page int64) (*listPage, error)` (`builder_list.go`), `objectName(map[string]any) string` (`confirm.go`), `jsonInt`, `cacheScope`, `completionCachePath`, `readCompletionCache`, `writeCompletionCache`, `namecache.TTL`, `spec.Suggest`.
- Produces (Task 3, 4, 5 nutzen):
  - `registry.NameRef{Flag, Resource, Label, Parent string}`; `registry.NameRefs() []NameRef`; `registry.NameRefOf(flag string) (NameRef, bool)`.
  - `cli.Resolution{Flag string "flag"; Name string "name"; Resource string "resource"; ID int64 "id"; Credits int "credits"; Cached bool "cached"}` (JSON-Tags in Anführungszeichen).
  - `isIDValue(raw string) bool` – getrimmt `^-?[0-9]+$` (negative Werte bleiben IDs und scheitern wie heute mit Exit 2).
  - `nameKey(s string) string` – `strings.ToLower` von `strings.Join(strings.Fields(output.Printable(s)), " ")`.
  - `(a *App) newNameResolver(c *cobra.Command, client *api.Client, r *config.Resolved) *nameResolver`; `(n *nameResolver) resolve(ref registry.NameRef, name string, parent int64) (Resolution, error)` – schreibt den Hinweis auf stderr.
  - Konstanten `resolvePageSize = allPageSize` (100), `resolveMaxPages = 5`.
  - `completionID` neu `Archived bool "archived,omitempty"`; `completionCache` neu `Complete bool "complete,omitempty"`; `completionCacheMax = 1 << 20`; `cacheName(s string) string` (Printable, Leerraum zusammengefasst, **ungekürzt**); `idEntries(entries []map[string]any) []completionID`; `(a *App) lookupIDs(ctx, r, e) (ids []completionID, complete, ok bool)`.

`internal/registry/names.go`, Tabelle (Flag → Ressource, Label; Parent nur bei Kontaktperson):

| Flag | Ressource | Label | Parent |
|---|---|---|---|
| customer | company | Kunde | |
| creditor | company | Lieferant | |
| company | company | Firma | |
| project | project | Projekt | |
| customer-project | project | Projekt | |
| contact-person | person | Kontaktperson | customer |
| task | task | Aufgabe | |
| proposition | proposition | Artikel | |
| default-proposition | proposition | Artikel | |
| payment-term | payment-term | Zahlungsbedingung | |
| user | user | Benutzer | |
| team-member | user | Benutzer | |

- [ ] **Step 1: Failing Tests – Registry**

`TestNameRefsMatchRegistry`: jede `Resource` hat `registry.Lookup(res, "list")`; `Parent` ist selbst ein `Flag` der Tabelle; Flags eindeutig; `NameRefOf("customer")` → `{company, Kunde}`; `NameRefOf("invoice")` → `false`.

- [ ] **Step 2: Failing Tests – Resolver** (`internal/cli/resolve_test.go`, mockapi per `OnFunc` mit eigener Liste; Uhr `testClock`)

- `TestIsIDValue`: `"3214"`, `" 12 "`, `"0"`, `"-1"` → true; `"1und1"`, `"Mustermann"`, `""`, `"12a"`, `"1 2"` → false.
- `TestNameKey`: `nameKey("  Mustermann \t GmbH ") == "mustermann gmbh"`.
- `TestResolveUniqueMatch`: Liste `[{1,"ACME"},{2,"Mustermann GmbH"}]`, `total_pages` 1 → `Resolution{Flag:"customer", Name:"Mustermann GmbH", Resource:"company", ID:2, Credits:1}`; genau 1 GET `/contact/companies?page=1&page_size=100`; stderr genau `Hinweis: Kunde „Mustermann GmbH“ → 2 (1 Credit)\n`.
- `TestResolveCaseAndSpace`: Eingabe `"  mustermann   gmbh "` → ID 2.
- `TestResolveArchivedIgnored`: zwei Einträge gleichen Namens, einer `record_state` `archived` → der aktive; nur ein archivierter → Exit 4 (`not_found`), Hinweis enthält `archiviert` und dessen ID sowie `papierkram company unarchive`.
- `TestResolveAmbiguous`: `[{2,"Mustermann GmbH"},{3,"mustermann gmbh"}]` → Exit 2 (`usage`), Meldung `Kunde „Mustermann GmbH“ ist mehrdeutig: 2 „Mustermann GmbH“, 3 „mustermann gmbh“`, Hinweis `ID statt Name angeben`.
- `TestResolveNotFoundSuggests`: `[{1,"Mustermann Automobile GmbH"},{2,"Musterfrau AG"},{3,"ACME"}]`, Eingabe `Mustermann` → Exit 4, Meldung `Kunde „Mustermann“ nicht gefunden`, Hinweis beginnt mit `Ähnlich: „Mustermann Automobile GmbH“ (1)` (Teilstring-Treffer zuerst, dann `spec.Suggest`, höchstens 3) und endet mit `sonst ID angeben (papierkram company list)`.
- `TestResolveMultiplePages`: `total_pages` 3 → 3 GETs (`page` 1–3), `Credits` 3, Hinweis `(3 Credits)`.
- `TestResolveIncompleteList`: `total_pages` 6 → genau 5 GETs, Exit 2, Meldung enthält `mehr als 500 Einträge`, Hinweis `ID statt Name angeben (papierkram company list)` – auch wenn auf den 5 Seiten genau ein Treffer steht.
- `TestResolveCacheHit`: zweiter Aufruf innerhalb von `namecache.TTL` → 0 GETs, `Cached` true, `Credits` 0, Hinweis `Hinweis: Kunde „Mustermann GmbH“ → 2 (aus dem Zwischenspeicher, 0 Credits)`; nach `Now + 61 s` wieder 1 GET.
- `TestResolveCacheIncompleteIgnored`: Cache-Datei mit `complete` false (wie die Vervollständigung sie bei 2 Seiten schreibt) → Resolver lädt.
- `TestResolveWritesCacheForCompletion`: nach `resolve` liefert `completeIDs(c, "company", "")` die IDs ohne GET.
- `TestCompletionCacheOldFormat`: Datei ohne `complete`/`archived` (v0.1-Form) → Vervollständigung trifft weiter, Resolver lädt neu.
- `TestCompletionNamesTruncatedAtDisplay`: Name mit 80 Runen liegt ungekürzt im Cache, die Vervollständigung zeigt 60.
- `TestResolvePersonByParent`: `NameRef` `contact-person`, parent 2843 → GET `/contact/companies/2843/persons`, `"Anna Groß"` trifft `first_name` Anna + `last_name` Groß; **keine** Cache-Datei geschrieben; parent 0 → Fehler (Programmierfehler, `CodeError`).
- `TestResolveUserName`: Benutzer `Max Mustermann` (Vor- und Nachname) → ID.
- `TestResolveErrorsPassThrough`: Liste 429 → Exit 6, 401 → Exit 3, Netzwerkfehler → Exit 10 (Code des Lesens unverändert).

- [ ] **Step 3: Tests laufen lassen, rot**

Run: `go test ./internal/registry/ ./internal/cli/ -run 'NameRef|Resolve|IsIDValue|NameKey|CompletionCache|CompletionNames' -v`
Expected: FAIL (undefinierte Symbole).

- [ ] **Step 4: Registry-Tabelle und Cache-Format implementieren**

`names.go` wie oben (Tabelle als `var nameRefs`, `NameRefs()` liefert eine Kopie). In `completion_ids.go`: `idEntries` (ID > 0, `Name: cacheName(objectName(obj))`, `Archived: record_state == "archived"`); `lookupIDs` setzt `complete = total_pages <= 1` (fehlt `total_pages`: false); `completeIDs` speichert `Complete` mit und kürzt erst beim Ausgeben mit `completionName`.

- [ ] **Step 5: Resolver implementieren**

`resolve`: Cache nur bei `Parent == ""` und Cache-Verzeichnis: Treffer bei frischer Datei mit `Complete` (Schlüssel und Pfad wie die Vervollständigung: `completionCachePath(a.cacheDir(), r.Context, ref.Resource)`, Scope `cacheScope(r)`). Sonst Seiten über `fetchPage` mit `page_size=100`, Seite 1 bis `min(total_pages, 5)`; `total_pages > 5` → Exit 2 wie oben. Danach Cache schreiben (`Complete: true`), außer bei Personen. Vergleich per `nameKey`; aktive Treffer zählen, archivierte nur für den Hinweis. Kein Einsatz von `fetchPages` (dessen `--max-pages`-Warnung nennt ein Flag, das der Nutzer nicht gesetzt hat).

- [ ] **Step 6: Tests grün**

Run: wie Step 3. Expected: PASS. Zusätzlich `go test ./internal/cli/ -run 'Completion' -v` unverändert grün.

- [ ] **Step 7: Gates und Commit**

Run: G1–G4.

```bash
git add internal/registry/names.go internal/registry/names_test.go internal/cli/resolve.go internal/cli/resolve_test.go internal/cli/completion_ids.go internal/cli/completion_test.go
git commit -m "feat(cli): resolve names to ids from list requests"
```

**Cross-Pfad-Tabelle (Original `completeIDs`/`lookupIDs` vs. neu `nameResolver.resolve`) – Akzeptanzkriterium:**

| Parameter | Vervollständigung | Namensauflösung | Begründung |
|---|---|---|---|
| Operation | `registry.Lookup(resource, "list")` | identisch | – |
| Ressourcen mit `{` im Pfad | übersprungen | nur `person` über `Parent`, ohne Cache | Liste hängt an der Firma; Cache-Schlüssel ist Kontext + Ressource |
| `page_size` | 100 (`completionPageSize`) | 100 (`resolvePageSize`) | – |
| Seiten | 1 | bis 5 (`resolveMaxPages`), mehr → Exit 2 | Spec §4 |
| Zeitlimit | `completionTimeout` 3 s gesamt | Client des Befehls (`--timeout`/Kontext) | der Befehl wartet ohnehin, die Shell nicht |
| `--debug` | aus | wie der Befehl | Befehl, keine Shell-Ausgabe |
| Warnung ohne https | keine (`lookupIDs`) | wie `clientFor` | Befehl |
| Zugangsdaten | `resolveWith` + `deadlineStore` | Client des Befehls (nach dem Gate) | eine Auflösung je Aufruf |
| Cache-Pfad, Scope, TTL | `completionCachePath`, `cacheScope`, `namecache.TTL` | identisch | Spec: gleicher Schlüssel |
| Cache gilt | frisch | frisch **und** `complete` | ein Fehltreffer auf Seite 1 beweist nichts |
| Einträge | ID > 0 | ID > 0, `archived` zählt nicht als Treffer | Spec §4 |
| Name | `objectName` → gekürzt | `objectName` → `cacheName` (ungekürzt), Vergleich `nameKey` | Kürzung nur zur Anzeige |
| `completion_lookup = false` | keine Anfrage, kein Cache | wirkt nicht | Option gilt nur für Tab-Vervollständigung |
| Schreibt Cache | ja | ja (`Complete: true`), Personen nie | Spec [Call] |

**Rulings (Spec offen):** Vergleich fasst inneren Leerraum zusammen (strenger als „getrimmt“ wäre nutzlos; Kollisionen enden als Mehrdeutigkeit, nie als stille Wahl). Eine Liste mit mehr als 5 Seiten bricht mit Exit 2 ab, auch bei genau einem Treffer (sonst könnte ein Doppel auf Seite 6 still verloren gehen). Personen werden nicht gecacht. Nur archivierter Treffer → Exit 4 mit Hinweis auf die archivierte ID.

**Akzeptanz:** Tests oben grün; Cross-Pfad-Tabelle im Report; Gates G1–G4.

---

### Task 3: Namensauflösung in den Befehlen, `--dry-run`, `commands --json` (Spec §4)

**Files:**
- Create: `internal/cli/resolve_flags.go`, `internal/cli/resolve_flags_test.go`
- Modify: `internal/cli/builder.go` (Pfad-Flags `rawValue{typ:"id"}`, `pathValues`), `internal/cli/builder_list.go` (ID-Filter `rawValue{typ:"id"}`, `filterValue`, `runList`), `internal/cli/builder_write.go` (`addWriteFlags`: `markResolves`; `runWriteThen`), `internal/cli/builder_action.go` (`runDescribe`, `runSimpleAction`), `internal/cli/dryrun.go` (`resolved`), `internal/cli/commands.go` (`flagInfo.Resolves`, Annotation), `docs/schemas/commands.json`, `docs/schemas/dry-run.json`
- Test: `internal/cli/commands_test.go`, `internal/cli/contract_test.go`, `internal/cli/keep_payment_term_test.go`
- Doku: Haupt-Spec §2 (Nicht-Ziel „Namensauflösung“ streichen, Verweis auf v0.2-Spec), §6.1, §7.2, §8.4; README (neuer Abschnitt „Namen statt IDs“ unter „Ressourcen“), AGENTS (Weitere Hinweise „IDs statt Namen“ → „IDs oder Namen“, Credits, `--dry-run` `resolved`, Selbstbeschreibung `resolves`), CHANGELOG

**Interfaces:**
- Consumes: Task 2 (`registry.NameRefOf`, `nameResolver.resolve`, `Resolution`, `isIDValue`).
- Produces:
  - `const flagResolves = "pk:resolves"`; `markResolves(fs *pflag.FlagSet, name string)` – setzt die Annotation mit `NameRef.Resource`, wenn `registry.NameRefOf(name)` existiert.
  - `flagInfo.Resolves string "resolves,omitempty"` in `commands --json`.
  - `flagID(c *cobra.Command, name string) (int64, error)` – liest ein `rawValue{typ:"id"}`; Nicht-Ziffern bei Flags **ohne** `pk:resolves` → Exit 2 mit dem bisherigen Text `ungültiger Wert "<v>" für --<flag>: ganze Zahl erwartet`.
  - `checkNameFlags(c *cobra.Command) error` – Exit 2: leerer Wert eines Auflösungs-Flags (`--<flag> braucht eine ID oder einen Namen`); Name bei `--contact-person` ohne gesetztes `--customer` (`Name für --contact-person braucht --customer (ID oder Name)`, Hinweis `sonst die ID der Kontaktperson angeben`).
  - `(a *App) resolveNames(c *cobra.Command, client *api.Client, r *config.Resolved) ([]Resolution, error)` – löst alle gesetzten Flags mit `pk:resolves` und Namenswert auf (erst ohne `Parent`, dann mit), schreibt die IDs zurück (`fs.Set`; `stringArray` über `pflag.SliceValue.Replace`), gibt die Auflösungen zurück.
  - `patchPathParams(c *cobra.Command, e registry.Entry, params []int64) error` – setzt nach `resolveNames` die Werte gesetzter Pfad-Flags neu.
  - `sendExtras.resolved []Resolution`; `dryRunJSON.Resolved []Resolution "resolved,omitempty"`; `credits` += Summe `Resolution.Credits`.
  - Für Task 4 und 5: Flags mit `pk:resolves` sind `rawValue{typ:"id"}` bzw. `StringArray` mit `pk:type` `id`.

Prüfreihenfolge je Runner (Akzeptanzkriterium): Flags inkl. `checkNameFlags` (Exit 2) → `pathValues` (Name ergibt vorläufig 0) → Konfiguration und Gate (`mutationClient`, Exit 9; bei Lesebefehlen `Client`) → `resolveNames` → `patchPathParams` → Query/Body (Exit 5) → `flushPresetHints` → `beforeSend` (dry-run, Exit 8) → senden.

- [ ] **Step 1: Failing Tests** (`resolve_flags_test.go`, mockapi)

- `TestNameFlagsAnnotated`: `commands --json`: `resolves` genau an den Flags der Tabelle aus Task 2 in Builder-Befehlen, z. B. `invoice create --customer` = `company`, `--contact-person` = `person`, `--payment-term` = `payment-term`, `project create --team-member` = `user`, `invoice list --company` = `company`, `person list --company` = `company`; **ohne** `resolves`: `time log --task`, `time start --user`, `local init --company`, `context create --user`, `time list --invoice`, `transaction list --bank-connection`, `invoice create --from-estimate`, `--custom-template`.
- `TestInvoiceCreateCustomerByName`: `invoice create --name X --customer "Mustermann GmbH" --payment-term 1130 --document-date heute --item …` → Anfragen in Reihenfolge GET `/contact/companies`, POST `/income/invoices` mit `customer.id` 2.
- `TestNameResolutionAfterReadOnlyGate`: `PAPIERKRAM_READ_ONLY=1` → Exit 9, 0 Anfragen.
- `TestNameResolutionAfterFlagErrors`: `invoice list --company Mustermann --limit 500` → Exit 2, 0 Anfragen.
- `TestNameResolutionBeforeBody`: Name + `--document-date 2026-13-45` → Exit 5 nach genau 1 GET (dokumentierte Reihenfolge).
- `TestNoResolutionOutsideFlags`: `invoice describe Mustermann` → Exit 2 wie bisher (Positionsargument), 0 Anfragen; `papierkram api` hat kein Flag mit `resolves` (Prüfung über `commands --json`).
- `TestDryRunShowsResolved`: `--dry-run -o json` → `resolved` = `[{"flag":"customer","name":"Mustermann GmbH","resource":"company","id":2,"credits":1,"cached":false}]`, `credits` 51, Ausgabe erfüllt `dry-run.json`; Textform enthält `Aufgelöst: --customer „Mustermann GmbH“ → 2 (1 Credit, bereits verbraucht)`.
- `TestListFilterByName`: `invoice list --company Mustermann` → GET Firmen, dann GET `/income/invoices?company_id=2…`.
- `TestPersonCompanyByName`: `person list --company Mustermann` → Pfad `/contact/companies/2/persons`; ohne Flag greift weiter `PAPIERKRAM_COMPANY` (keine Auflösung).
- `TestContactPersonByName`: `invoice create … --customer 2 --contact-person "Anna Groß"` → GET `/contact/companies/2/persons`, Body `customer.contact_person.id`; mit `--customer Mustermann` erst Firmen, dann Personen.
- `TestContactPersonNameWithoutCustomer`: Exit 2, 0 Anfragen.
- `TestTeamMemberNames`: `project create --name P --customer 2 --team-member "Max Mustermann" --team-member 7` → `team_members` IDs `[<id>, 7]`, Benutzerliste genau einmal geladen.
- `TestIDFlagStaysID`: `--customer 3214` → keine Leseanfrage; `--customer -1` → Exit 2 wie bisher.
- `TestNonResolvingIDFlagMessage`: `time list --invoice abc` → Exit 2, Meldung unverändert.
- `TestEmptyNameExit2`: `--customer "  "` → Exit 2, 0 Anfragen.
- `TestInvoiceUpdateCustomerNameKeepsRefs` (`keep_payment_term_test.go`): `invoice update 459 --customer Mustermann` (gleicher Kunde wie die Rechnung) → GET Firmen, GET Rechnung, PUT mit `contact_person` und `project` (Punkt 29 bleibt erfüllt).
- `TestCommandsJSONMatchesContract` und `TestDryRunContract` erweitert; `TestContractSchemasStrict`: `resolves` muss String mit `minLength` 1 sein, `resolved[].credits` ≥ 0, unbekanntes Feld in `resolved[]` abgelehnt.

- [ ] **Step 2: Tests laufen lassen, rot**

Run: `go test ./internal/cli/ -run 'Name|DryRunShowsResolved|ListFilterByName|PersonCompanyByName|ContactPerson|TeamMember|IDFlag|EmptyName|InvoiceUpdateCustomerName|Contract' -v`
Expected: FAIL.

- [ ] **Step 3: Flags umstellen und annotieren**

Alle `Kind: "id"`-Filter und Pfad-Flags als `rawValue{typ:"id"}` (bisher `Int64`; `commands --json` bleibt Typ `id`); `markResolves` in `addListFlags`, `addWriteFlags`, `newEntryCmd` (Pfad-Flags); `filterValue` und `pathValues` lesen über `flagID`. Nur diese drei Builder setzen die Annotation (Custom-Befehle bleiben ohne).

- [ ] **Step 4: Runner einhängen**

`runList`, `runDescribe`, `runSimpleAction`, `runWriteThen` nach der Reihenfolge oben; in `runWriteThen` vor `s.keep.apply` (vergleicht Kunden-IDs). `dryRun` zeigt `resolved` und addiert die Credits; Text: eine Zeile `Aufgelöst: …` je Auflösung.

- [ ] **Step 5: Schemas**

`commands.json`: Flag-Objekt (Befehle **und** `global_flags`, `TestCommandsContractFlagSchemasEqual`) um optional `resolves` (`type: string`, `minLength: 1`, Beschreibung „Flag nimmt statt der ID auch einen Namen dieser Ressource; die Auflösung kostet 1 Credit je geladener Seite (höchstens 5), aus dem Zwischenspeicher 0“). `dry-run.json`: optional `resolved` (Array strikter Objekte `flag`, `name`, `resource`, `id` ≥ 0, `credits` ≥ 0, `cached`; alle Pflicht).

- [ ] **Step 6: Tests grün, Doku, Gates**

Run: Step-2-Befehl → PASS; dann G1–G5. Hilfetexte der Flags: Usage um `; ID oder Name` ergänzen (z. B. `ID oder Name für customer`), sichtbar in `docs/reference/` nach `make docs`.

- [ ] **Step 7: Commit**

```bash
git add internal/cli/resolve_flags.go internal/cli/resolve_flags_test.go internal/cli/builder.go internal/cli/builder_list.go internal/cli/builder_write.go internal/cli/builder_action.go internal/cli/dryrun.go internal/cli/commands.go internal/cli/*_test.go docs/schemas/commands.json docs/schemas/dry-run.json docs/specs/2026-11-03-papierkram-cli-design.md README.md AGENTS.md CHANGELOG.md docs/reference
git commit -m "feat(cli): accept names in id flags"
```

**Rulings (Spec offen):**
- **Geltungsbereich:** nur Flags der Builder-Befehle (Liste, Pfad, Body), dazu `invoice paid-status --customer` (Task 4). Nicht: `time log|start --task/--user` (Alias-Mechanismus der Projektdatei; `time start` ist lokal und muss offline gehen; `time stop` bucht, was `start` speicherte), `local init`, `context create` (lokal), `--from-estimate`, `--custom-template`, `--invoice`, `--bank-connection`, `--transaction` (nicht in der Spec-Liste bzw. ohne Namensliste). Zusätzlich zur Spec-Liste: `--default-proposition` und `--team-member` (ID-Flags benannter Ressourcen laut Registry).
- **`--dry-run`:** `read` bleibt das bisherige Einzelobjekt (Vertrag `dry-run.json`, mehrere Auflösungen bzw. Seiten passen nicht hinein); die Auflösung steht additiv unter `resolved`. `credits` enthält die in diesem Lauf verbrauchten Auflösungs-Credits (Obergrenze; der echte Aufruf trifft binnen 60 s den Cache).
- **`--contact-person` per Name** braucht `--customer` als Flag (Personenliste hängt an der Firma).
- **Reihenfolge:** Auflösung nach dem Gate und vor dem Body; ein Body-Fehler nach der Auflösung kostet deren Credits (höchstens 5 je Flag). Exit 9 sendet so weiterhin nichts.

**Akzeptanz:** Tests oben grün; Reihenfolge der Prüfungen im Report mit Testnamen belegt; Gates G1–G5.

---

### Task 4: `invoice paid-status` (Spec §5)

**Files:**
- Create: `internal/cli/paid_status.go`, `internal/cli/paid_status_test.go`, `docs/schemas/paid-status.json`, Golden `internal/cli/testdata/golden/invoice-paid-status.table`, `…/invoice-paid-status.json`
- Modify: `internal/cli/builder.go` (`groupCmds["invoice"]`), `internal/cli/contract_test.go` (`outputRuns`, Strict-Fälle), `internal/cli/commands_test.go` (Erwartungen Befehlsliste)
- Doku: Haupt-Spec §7.1 und neuer Abschnitt §9.9, §14 (Eintrag streichen); README „Ressourcen“ bzw. eigener Absatz „Offene Posten“; AGENTS (JSON-Vertrag-Zeile `paid-status.json`, Credits); `skills/papierkram/SKILL.md` „Ablauf 3: offene Posten prüfen“; CHANGELOG

**Interfaces:**
- Consumes: `registry.Lookup("invoice","list")` und dessen `Filters` (`company`, `from`, `to`), `(a *App) filterValue`, `fetchPage`, `pageTooLarge`/`hintListTooLarge`, `(a *App) dayZone()`, `newNameResolver`/`resolveNames` (Task 2/3), `germanAmount`, `annotOperation`, `markBounds`, `markCredits`, `markResolves`.
- Produces:
  - `(a *App) newPaidStatusCmd() *cobra.Command` (Use `paid-status`, Annotationen `pk:class` `read_only`, `pk:operation` `ListInvoices`).
  - `paidStatusEntry{ID int64 "id"; InvoiceNo *string "invoice_no"; Name string "name"; Customer string "customer"; State string "state"; DocumentDate *string "document_date"; DueDate *string "due_date"; DaysOverdue int "days_overdue"; TotalGross json.Number "total_gross"; OutstandingAmount json.Number "outstanding_amount"}`.
  - `paidStatusReport{Entries []paidStatusEntry "entries"; TotalOutstanding json.Number "total_outstanding"; Count int "count"; OverdueCount int "overdue_count"; AsOf string "as_of"; Complete bool "complete"}`.
  - `var paidStatusSkipStates = []string{"draft", "canceled"}`.
  - `buildPaidStatus(entries []json.RawMessage, today time.Time, overdueOnly bool) (paidStatusReport, []int64, error)` – zweiter Wert: IDs ohne `outstanding_amount`.
  - `sumDecimals(vs []json.Number) (json.Number, error)` – `math/big.Rat`, Ergebnis mit 2 Nachkommastellen.

Flags: `--customer` (ID oder Name, `pk:resolves` `company`, → Query `company_id`), `--from`, `--to` (Datum, wie `invoice list`), `--overdue` (bool), `--max-pages` (Standard 5, Minimum 1), `--details` (bool, `pk:credits` 1: je nachgelesener Rechnung).

- [ ] **Step 1: Failing Tests** (`paid_status_test.go`, mockapi, `testClock` = 2026-06-10 Berlin)

- `TestPaidStatusSelectsOpen`: Einträge `draft` 50, `canceled` 30, `unpaid` `"119.0"`, `paid` `"0.0"`, `partially_paid` `"50.5"` → `entries` 2, `total_outstanding` `169.50`, `count` 2.
- `TestPaidStatusOverdue`: `due_date` 2026-06-09 → `days_overdue` 1, `overdue_count` 1; 2026-06-10 → 0, nicht überfällig; `--overdue` → nur der überfällige, `count` 1.
- `TestPaidStatusDecimalSum`: `"0.1"` + `0.2` (Zahl) → `0.30`; `"1234567.89"` + `"0.01"` → `1234567.90`; ungültiger Betrag `"x"` → Exit 1 (`error`), Meldung nennt die Rechnungs-ID.
- `TestPaidStatusNullDueDate`: `due_date` `null` → `days_overdue` 0, nicht in `--overdue`.
- `TestPaidStatusSorted`: Reihenfolge nach `due_date` aufsteigend (ohne Fälligkeit zuletzt), dann `id`.
- `TestPaidStatusQuery`: `--customer 3214 --from 2026-01-01 --to 2026-03-31` → Query `company_id=3214`, `document_date_range_start=2026-01-01`, `document_date_range_end=2026-03-31`, `page_size=100`, `page=1`.
- `TestPaidStatusCustomerByName`: Firmenliste, dann Rechnungen mit `company_id`.
- `TestPaidStatusMaxPages`: `total_pages` 7 → 5 GETs, `complete` false, Warnung `--max-pages 5: 5 von 7 Seiten geladen; weitere Einträge vorhanden`; `--max-pages 0` → Exit 2.
- `TestPaidStatusMissingOutstanding`: Eintrag ohne `outstanding_amount` → Exit 1, Meldung `Rechnungsliste ohne outstanding_amount bei 1 Rechnung(en): offene Posten nicht bestimmbar`, Hinweis `mit --details je Rechnung nachlesen (1 Credit je Rechnung)`; mit `--details` → GET `/income/invoices/<id>` nur für diese, Betrag aus der Antwort, stderr `Hinweis: --details liest 1 Rechnung einzeln (1 Credit)`.
- `TestPaidStatusContract` (über `outputRuns`): `-o json` erfüllt `paid-status.json`; Strict: zusätzliches Feld, fehlendes `as_of`, `total_outstanding` als String → abgelehnt.
- `TestPaidStatusGolden`: Tabelle (`-o table`) gegen `invoice-paid-status.table`: Spalten `NR.`, `KUNDE`, `DATUM`, `FÄLLIG`, `ÜBERFÄLLIG` (Tage, leer bei 0), `BRUTTO`, `OFFEN`; letzte Zeile `Summe offen: 169,50 € (2 Rechnungen, davon 1 überfällig; Stand 2026-06-10)`.
- `TestPaidStatusCommandsJSON`: Klasse `read_only`, `credits` 1, `operation_id` `ListInvoices`; `--max-pages` `default` „5“, `minimum` 1; `--details` `credits` 1; `--customer` `resolves` `company`.
- `TestPaidStatusReadOnlyAllowed`: `PAPIERKRAM_READ_ONLY=1` → Exit 0; `--dry-run` → Exit 2 (lesender Befehl).

- [ ] **Step 2: Tests laufen lassen, rot**

Run: `go test ./internal/cli/ -run 'PaidStatus|OutputContract|ContractSchemasStrict' -v`
Expected: FAIL.

- [ ] **Step 3: Implementieren**

Ablauf: Flags (Exit 2) → `Client` → `resolveNames` → Query aus den Registry-Filtern von `ListInvoices` (`company` mit dem Wert von `--customer`, `from`, `to` über `filterValue`) → eigene Seitenschleife über `fetchPage` (Seite 1 bestimmt `total_pages`, Kappung `--max-pages`, Warnung wie `fetchPages`) → `buildPaidStatus` → ggf. `--details` → Ausgabe. Heute = `a.Now().In(a.dayZone())`. Offen: `outstanding_amount` > 0 und `state` ∉ `paidStatusSkipStates`. Beträge als `json.Number` im Text der API (Zahl oder Dezimal-String), Summe über `sumDecimals`. Strukturierte Formate rendern `paidStatusReport`; `table`/`wide` die Spalten plus Summenzeile (`germanAmount`).

- [ ] **Step 4: Schema `docs/schemas/paid-status.json`**

Strikt, alle Felder Pflicht: `entries[]` (Felder wie `paidStatusEntry`; `invoice_no`, `document_date`, `due_date` nullable; Beträge `type: number`; `days_overdue` integer ≥ 0), `total_outstanding` number, `count`/`overdue_count` integer ≥ 0, `as_of` string `format: date`, `complete` boolean.

- [ ] **Step 5: Tests grün, Doku, Gates**

Run: Step-2-Befehl → PASS; dann G1–G5. Haupt-Spec §9.9 beschreibt Auswahl, Credits, `--details`, `complete`. SKILL „Ablauf 3“ nutzt `papierkram invoice paid-status --overdue -o json --no-input`.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/paid_status.go internal/cli/paid_status_test.go internal/cli/builder.go internal/cli/contract_test.go internal/cli/commands_test.go internal/cli/testdata/golden/invoice-paid-status.* docs/schemas/paid-status.json docs/specs/2026-11-03-papierkram-cli-design.md README.md AGENTS.md skills/papierkram/SKILL.md CHANGELOG.md docs/reference
git commit -m "feat(invoice): add paid-status report"
```

**Cross-Pfad-Tabelle (Original `invoice list --all` vs. neu `invoice paid-status`) – Akzeptanzkriterium:**

| Parameter | `invoice list --all` | `invoice paid-status` | Begründung |
|---|---|---|---|
| Operation | `ListInvoices` | identisch (`pk:operation`) | keine neue Operation |
| `page_size` | 100 (`allPageSize`, ohne `--limit`) | 100 | – |
| Seitenzahl | alle; `--max-pages` optional | Standard 5 (`--max-pages`) | Spec §5 |
| Seite laden | `fetchPage` über `fetchPages` | `fetchPage`, eigene Schleife | `fetchPages` führt Einträge roh zusammen und fragt ab 10 Seiten; bei 5 Seiten nie erreicht |
| Warnung bei Kappung | `--max-pages N: … weitere Einträge vorhanden` | identischer Text, zusätzlich `complete` false | Vertrag für Agents |
| Firmenfilter | `--company` (ID) → `company_id` | `--customer` (ID oder Name) → `company_id` | Spec §5 nennt `--customer` |
| Datumsfilter | `--from`/`--to` über `filterValue` (Berliner Tag) | identisch, gleiche Registry-Filter | – |
| Sortierung | `--sort` optional (API) | lokal nach `due_date`, `id` | Bericht |
| Antwort zu groß | `pageTooLarge(err, hintListTooLarge)` | identisch | – |
| Credits | 1 je Seite | 1 je Seite, `--details` +1 je Rechnung | Spec §5 |
| Klasse, Schreibschutz | `read_only` | identisch | – |

**Rulings (Spec offen):** Additives Feld `complete` (false, wenn `--max-pages` Seiten auslässt) – sonst wäre eine unvollständige Summe für Agents nicht erkennbar. Fehlt `outstanding_amount`, bricht der Befehl ohne `--details` mit Exit 1 ab statt eine falsche Summe zu liefern. `--details` liest nur die Rechnungen ohne Betrag. Archivierte Rechnungen zählen mit (offene Forderung bleibt offen). Die Statusliste `paidStatusSkipStates` passt der Controller nach dem Live-Lauf an (Task 7 misst).

**Akzeptanz:** Tests oben grün; Cross-Pfad-Tabelle im Report; Gates G1–G5.

---

### Task 5: `papierkram mcp serve` – Kern, nur Lese-Tools (Spec §3)

**Files:**
- Create: `internal/mcpserver/mcpserver.go` (Typen, `Tools`, `InputSchema`, `New`, Handler), `internal/mcpserver/args.go` (`Args`, `ArgError`), `internal/mcpserver/result.go` (`ToolResult`), Tests `internal/mcpserver/mcpserver_test.go`, `internal/mcpserver/args_test.go`
- Create: `internal/cli/mcp.go` (Befehl `mcp serve`, `NewMCPServer`, `MCPOptions`, Runner, `mcpChild`), `internal/cli/mcp_tools.go` (`mcpToolSpecs`), Tests `internal/cli/mcp_test.go`, `internal/cli/mcp_tools_test.go`, `internal/cli/stdio_test.go`
- Modify: `internal/cli/root.go` (`AddCommand`, `RunContext`), `internal/cli/app.go` (`DebugOut`), `internal/cli/client.go:198` (`o.Debug = a.debugOut()`), `internal/registry/registry.go` (`reservedNames` + `"mcp"`), `internal/cli/commands_test.go` (Meta-Klassen), `go.mod`, `go.sum`, `tools/gennotices/main.go` (`licenses`), `THIRD_PARTY_NOTICES.md` (generiert)
- Doku: CHANGELOG (README/AGENTS/SKILL folgen in Task 6, wenn die Modi stehen)

**Interfaces:**
- Consumes: `buildCommandsDoc(root) (commandsDoc, error)` mit `commandInfo`/`flagInfo` (inkl. `Resolves`, Task 3), `executeContext`, `NewRootCmd`, `annotLocal`, `creditsText`, `buildinfo.Version`, `output.WriteError`-Form der Fehlerzeile.
- Produces (Task 6 und 7 nutzen):
  - Paket `mcpserver` (importiert **nicht** `internal/cli`):
    - `type Mode int`; `const (ModeRead Mode = iota; ModeWrite; ModeDestructive)`; `(m Mode) Allows(class string) bool` – Read: `read_only`; Write: + `create`, `update`, `archive`; Destructive: + `destructive`, `send`.
    - `type Kind string`; `KindString`, `KindInteger`, `KindNumber`, `KindBool`, `KindStrings` (wiederholbar), `KindRef` (ganze Zahl oder Name), `KindJSONFile` (JSON-Wert → Temp-Datei, Flag bekommt den Pfad), `KindPath` (Dateipfad, `-` verboten).
    - `type Param struct{ Name, Flag string; Pos int; Kind Kind; Description string; Enum []string; Default any; Minimum, Maximum *float64; Required bool }` (`Pos` −1 für Flags, sonst Index des Positionsarguments).
    - `type ToolSpec struct{ Name, Description, Class string; Command []string; Params []Param; Fixed []string }` (Credits stehen in `Description`).
    - `type Result struct{ Exit int; Stdout, Stderr []byte }`; `type Runner func(ctx context.Context, args []string) Result`.
    - `type Options struct{ Mode Mode; Context string; Debug bool; Version string }`.
    - `Tools(specs []ToolSpec, m Mode) []ToolSpec`; `InputSchema(t ToolSpec) map[string]any`; `Args(t ToolSpec, raw json.RawMessage, o Options, tmpDir string) ([]string, error)`; `ToolResult(r Result) *mcp.CallToolResult`; `New(specs []ToolSpec, run Runner, o Options) *mcp.Server`.
    - `type ArgError struct{ Msg, Hint string }` → Tool-Fehler `{"error":{"code":"usage","message":…,"hint":…,"exit_code":2}}`, kein Aufruf.
  - Paket `cli`:
    - `type MCPOptions struct{ Context string }` (Task 6 ergänzt `AllowWrites`, `AllowDestructive`).
    - `NewMCPServer(app *App, o MCPOptions) (*mcp.Server, error)`.
    - `RunContext(ctx context.Context, app *App, args []string) int` = `executeContext(ctx, app, NewRootCmd(app), args)`.
    - `(a *App) mcpChild(out, errw io.Writer) *App` – kopiert `Env`, `Now`, `Location`, `ConfigDir`, `CacheDir`, `HTTPClient`, `Keyring`, `Getwd`, `Sleep`, `saveConfig`; `In` leer, `StdinTTY`/`StdoutTTY` false, `ReadPassword` liefert Fehler, `DebugOut` = Prozess-stderr der Eltern-App.
    - `App.DebugOut io.Writer` (nil → `a.Err`); `(a *App) debugOut() io.Writer`.
    - `mcpToolSpecs(root *cobra.Command) ([]mcpserver.ToolSpec, error)`.

**Tool-Erzeugung (`mcpToolSpecs`):**
- Name: Befehlswörter mit `_`, `-` → `_` (`invoice_list`, `voucher_document_delete`, `payment_term_list`, `info`).
- Ausgeschlossen: Klassen `local` und `per_request`; erstes Wort `auth`, `context`, `local`, `completion`, `api`, `commands`, `schema`, `version`, `help`, `gen-docs`, `mcp`; Befehle `time start`, `time stop`, `time status`, `time discard`.
- Beschreibung: `<short>. Klasse <class>, <creditsText> je Aufruf` (Listen: `je Seite`), bei Flags mit `resolves`: `; ID-Felder nehmen auch Namen (1 Credit je geladener Seite)`.
- Positionsargumente aus `Usage`: `<id>` → `id`, `[<id>]` → `id` (optional), `<beleg-id>` → `voucher_id`, `<datei-id>` → `document_id` (alle `KindInteger`, Minimum 0), `<datei>` → `file` (`KindPath`); unbekanntes Token → Fehler (fail closed).
- Flags: Name `-` → `_`; Typ `id` → `KindInteger` (Minimum 0) bzw. `KindRef` mit `resolves`; `integer`/`number`/`boolean`/`string`/`date`/`duration` → passender Kind (Datum/Dauer als String mit Beschreibung der Form); `repeatable` → `KindStrings`; `input` → `KindJSONFile` (Objekt), `items` → `KindJSONFile` (Array), `output-file`/`file` → `KindPath`; `enum`, `default` (typisiert), `minimum`/`maximum` übernommen; `Required` nur bei `required` **ohne** `alternatives` und **ohne** `default_from`.
- `InputSchema`: `{"type":"object","properties":…,"required":[…],"additionalProperties":false}`; `KindRef` → `"type":["integer","string"]`.

**Argument-Übersetzung (`Args`):** JSON mit `UseNumber`; unbekannte Eigenschaft, falscher JSON-Typ, Ganzzahl mit Nachkommastellen, `KindPath` gleich `-` → `ArgError`. Ergebnis: `Command…`, je Flag `--<flag>=<wert>` (wiederholbar: je Element), `Fixed…`, `--no-input`, `-o`, `json`, ggf. `--context=<ctx>`, ggf. `--debug`, dann `--`, dann Positionsargumente in `Pos`-Reihenfolge. `KindJSONFile` → Datei `0600` in `tmpDir` (je Aufruf `os.MkdirTemp`, danach gelöscht).

**Ergebnis (`ToolResult`):** Exit 0 → `Content[0]` = stdout als Text; stderr nicht leer → `Content[1]` = `Hinweise (stderr):\n` + stderr. Exit ≠ 0 → `IsError` true, `Content[0]` = letzte nicht leere stderr-Zeile (die kompakte JSON-Fehlerzeile von `-o json`), übriger stderr als `Content[1]`. Tool-Annotationen: `ReadOnlyHint` bei `read_only`, `DestructiveHint` bei `destructive`/`send`, `IdempotentHint` bei `read_only`, `OpenWorldHint` true.

**Server (`New`):** `mcp.NewServer(&mcp.Implementation{Name: "papierkram", Version: o.Version}, …)`, je Tool `AddTool` mit Roh-Handler; **ein Mutex serialisiert alle Aufrufe**. `mcp serve`: Klasse `local`, Annotation `annotLocal` (kein `--dry-run`), Transport über `app.In`/`app.Out` (`mcp.IOTransport`, entspricht stdio), Ende bei EOF oder Signal mit Exit 0. Konfiguration beim Start per `resolveWith(o.Context, nil)` (ohne Schlüsselbund); Fehler → Exit des Fehlers vor dem Serve. In Task 5 ist der Modus fest `ModeRead`.

> SDK-Namen (`mcp.NewServer`, `Server.AddTool`, `CallToolRequest.Params.Arguments`, `CallToolResult`, `TextContent`, `ToolAnnotations`, `IOTransport`, `NewInMemoryTransports`, `Client.Connect`, `ClientSession.ListTools/CallTool`) vor dem Schreiben mit `go doc github.com/modelcontextprotocol/go-sdk/mcp <Name>` der gepinnten Version prüfen; Abweichungen im Report nennen. Das SDK prüft beim Roh-Handler die Argumente nicht – `Args` prüft selbst.

- [ ] **Step 1: Abhängigkeit**

Run: `go get github.com/modelcontextprotocol/go-sdk@latest && go mod tidy`. Lizenzdateien aller neu gelinkten Module im Modulcache lesen, SPDX in `licenses` eintragen, `go run ./tools/gennotices`. Version und Lizenzen in den Report.

- [ ] **Step 2: Failing Tests – `internal/mcpserver`** (Spec-Tabellen ohne CLI, Fake-`Runner`)

- `TestModeAllows`: Matrix Modus × Klasse wie oben.
- `TestInputSchema`: Param-Set mit allen Kinds → erwartetes Schema-JSON (Golden im Test), `additionalProperties` false, `required` sortiert.
- `TestMCPArgsOrder`: `invoice_list` `{"company":3214,"all":true,"max_pages":2}` → `["invoice","list","--all=true","--company=3214","--max-pages=2","--no-input","-o","json","--"]` (Flags nach `Name` sortiert).
- `TestMCPArgsNoFlagInjection`: String-Werte `"--yes"`, `"-o"`, `"--context=x"`, `"--dry-run"` landen als `--name=--yes` usw.; Positionsargument `file` `"-x.pdf"` steht hinter `--`; keine Eigenschaft erzeugt `--yes` oder `--context` außer über `Options`.
- `TestMCPArgsRejects`: unbekannte Eigenschaft `via`, `id` als String, `id` 1.5, `limit` `"25"`, `output_file` `"-"`, `input` als String → `ArgError`, Runner nicht aufgerufen.
- `TestMCPArgsJSONFile`: `input` `{"name":"x"}` → `--input=<tmp>/input.json`, Datei `0600` mit genau diesem JSON; nach dem Aufruf gelöscht.
- `TestMCPArgsContextAndDebug`: `Options{Context:"firma", Debug:true}` → `--context=firma`, `--debug` vor `--`.
- `TestToolResult`: Exit 0 mit stderr → 2 Inhalte; Exit 4 mit stderr `Fehler: …\n{"error":{…,"exit_code":4}}\n` → `IsError`, `Content[0]` exakt die JSON-Zeile.
- `TestMCPConcurrentCallsSerialized`: 10 parallele `CallTool` über In-Memory-Transport, Runner zählt gleichzeitige Läufe → Maximum 1; `go test -race`.
- `TestMCPCancelledCall`: Runner blockiert bis `ctx.Done()`, Client bricht ab → Runner sieht abgebrochenen Kontext (Exit-Semantik prüft `TestMCPCancelMapsExit` in `cli`).

- [ ] **Step 3: Failing Tests – `internal/cli`** (mockapi, In-Memory-Transport)

- `TestMCPToolSpecsReadMode`: Tool-Namen = alle `read_only`-Befehle aus `commands --json` minus Ausschlüsse; enthält `invoice_list`, `invoice_describe`, `invoice_pdf`, `invoice_paid_status`, `info`, `user_list`; enthält nicht `auth_status`, `invoice_create`, `time_status`, `commands`; Namen eindeutig; keine Eigenschaft kollidiert mit `id`, `voucher_id`, `document_id`, `file`, `dry_run`.
- `TestMCPToolSchemasFromCommands`: `invoice_list.limit` = `{"type":"integer","default":25,"minimum":1,"maximum":100}`; `invoice_list.company` = `["integer","string"]`; `invoice_pdf.output_file` Pflicht.
- `TestMCPPositionalTokensKnown`: jedes `<…>`-Token aller Befehle mit Tool steht in der Zuordnung.
- `TestMCPReadToolCall`: `invoice_list` `{"limit":1}` → genau 1 GET `/income/invoices?page_size=1`, `Content[0]` = stdout von `papierkram invoice list --limit 1 -o json --no-input` (byte-gleich, gleicher Mock).
- `TestMCPErrorIsStructured`: `invoice_describe` `{"id":999}` mit 404 → `IsError`, JSON `code` `not_found`, `exit_code` 4.
- `TestMCPCancelMapsExit`: abgebrochener Aufruf während GET → Fehler-JSON `exit_code` 10.
- `TestMCPDebugGoesToStderr`: `--debug` beim Start → Debug-Zeilen im Prozess-stderr (`app.Err` der Eltern-App), nicht im Tool-Ergebnis; Token `test-token-mcp` erscheint weder im Ergebnis noch im stderr.
- `TestMCPServeStdioOnlyProtocol`: `Run(app, ["mcp","serve"])` mit Pipes als `In`/`Out`, Client über `IOTransport`: `tools/list`, ein Aufruf, Pipe schließen → Exit 0; alles auf `Out` sind gültige JSON-RPC-Nachrichten.
- `TestNoDirectStdio` (`stdio_test.go`): Quellscan aller Nicht-Test-Dateien unter `internal/` und `cmd/`: `os.Stdout`, `os.Stderr`, `fmt.Print*`, Paket `log` nur in `internal/cli/app.go` (`NewApp`) bzw. `cmd/papierkram/main.go`.
- `TestCommandsJSONMCPServe`: `mcp serve` Klasse `local`, `credits` 0, `blocked_by_read_only` false; `--dry-run` → Exit 2.

- [ ] **Step 4: Tests laufen lassen, rot**

Run: `go test ./internal/mcpserver/ ./internal/cli/ -run 'MCP|Mode|InputSchema|ToolResult|NoDirectStdio' -v`
Expected: FAIL.

- [ ] **Step 5: `internal/mcpserver` implementieren**, dann **Step 6: `internal/cli/mcp*.go` und `RunContext`**

Runner: `mcpChild(&out, &errb)`, `RunContext(ctx, child, args)`, `Result{Exit, out.Bytes(), errb.Bytes()}`. `reservedNames` um `mcp` ergänzen.

- [ ] **Step 7: Tests grün, Gates**

Run: Step-4-Befehl → PASS; `go test -race -count=3 ./internal/mcpserver/ ./internal/cli/ -run MCP`; G1–G5, G10.

- [ ] **Step 8: Commit**

```bash
git add internal/mcpserver internal/cli/mcp.go internal/cli/mcp_tools.go internal/cli/mcp_test.go internal/cli/mcp_tools_test.go internal/cli/stdio_test.go internal/cli/root.go internal/cli/app.go internal/cli/client.go internal/cli/commands_test.go internal/registry/registry.go go.mod go.sum tools/gennotices/main.go THIRD_PARTY_NOTICES.md CHANGELOG.md docs/reference
git commit -m "feat(mcp): serve read-only tools over stdio"
```

**Rulings (Spec offen):** `input`/`items` als JSON-Wert über eine Temp-Datei (stdin ist der Transport; der Dateipfad ist derselbe Prüfpfad wie `--input datei.json`). `-` als Dateipfad ist verboten (PDF auf stdout wäre kein JSON). stderr erscheint als zweiter Inhalt (Warnungen wie „NICHT erneut ausführen“ müssen den Agent erreichen). Aufrufe laufen serialisiert. Keine `--fields`/`--jq`/`--timeout` als Tool-Parameter. `--debug` geht auf den Prozess-stderr (`DebugOut`). `mcp serve` endet bei EOF und Signal mit Exit 0.

**Akzeptanz:** Tests oben grün; SDK-Version und Lizenzen im Report; Gates G1–G5, G10.

---

### Task 6: MCP – Modi, Schreibschutz, `deliver`, Cross-Pfad, MCP-Doku (Spec §3)

**Files:**
- Modify: `internal/cli/mcp.go` (Flags `--allow-writes`, `--allow-destructive`, `mcpMode`), `internal/cli/mcp_tools.go` (deliver-Sonderfall), `internal/mcpserver/mcpserver.go` (`dry_run`, `--yes`)
- Create: `internal/cli/mcp_crosspath_test.go`
- Test: `internal/cli/mcp_test.go`, `internal/mcpserver/mcpserver_test.go`, `internal/cli/docs_examples_test.go` (`docShellCommands` + `claude`)
- Doku: README (Abschnitt „MCP-Server“ nach „Shell-Vervollständigung“, Inhaltsverzeichnis), AGENTS (Abschnitt „MCP-Server“ nach „Selbstbeschreibung“; Regeln), `skills/papierkram/SKILL.md` (Hinweis MCP), Haupt-Spec §2 (Nicht-Ziel MCP streichen), §5.3 („später die MCP-Tools“ → `internal/cli/mcp_tools.go`), §7.2 (`mcp serve`), CHANGELOG

**Interfaces:**
- Consumes: Task 5 (alles), `guardMutation`-Semantik, `config.EnvReadOnly`, `Resolved.ReadOnly`/`ReadOnlySource`.
- Produces:
  - `MCPOptions{Context string; AllowWrites, AllowDestructive bool}`.
  - `(a *App) mcpMode(o MCPOptions) (mcpserver.Mode, error)` – schreibt bei erzwungenem Lesemodus den Hinweis.
  - In `mcpserver`: Tools der Klassen ≠ `read_only` bekommen `dry_run` (`KindBool`) → `--dry-run`; `destructive`/`send` bekommen immer `--yes` in `Args`.
  - deliver-Tools (`invoice_deliver`, `estimate_deliver`): Parameter nur `id` und `dry_run`, `Fixed` = `["--via=pdf"]`.

- [ ] **Step 1: Failing Tests**

- `TestMCPModeFlags`: ohne Optionen → `ModeRead`; `--allow-writes` → `ModeWrite`; `--allow-destructive` → `ModeDestructive` (schließt Schreiben ein).
- `TestMCPToolListPerMode`: je Modus genau die Klassen aus `Mode.Allows`; `voucher_pay` und `invoice_deliver` nur in `ModeDestructive`; `time_log` ab `ModeWrite`.
- `TestMCPReadOnlyEnvForcesRead`: `PAPIERKRAM_READ_ONLY=1` + `--allow-destructive` → nur Lese-Tools; stderr beim Start genau `Hinweis: Schreibschutz aktiv (PAPIERKRAM_READ_ONLY): --allow-writes/--allow-destructive wirkungslos, nur lesende Tools\n`.
- `TestMCPReadOnlyContextForcesRead`: Kontext `read_only = true` → dasselbe mit `Kontext „agent“ ist schreibgeschützt`.
- `TestMCPGateStillHolds`: Server mit `ModeWrite`, danach `PAPIERKRAM_READ_ONLY=1` in der Env des Kindes → `company_create` → `IsError`, `exit_code` 9, 0 Anfragen.
- `TestMCPWriteCall`: `company_create` `{"name":"X","contact_type":"customer"}` → 1 POST, Ergebnis = API-Objekt.
- `TestMCPWriteDryRun`: `{"name":"X","contact_type":"customer","dry_run":true}` → 0 Anfragen, `Content[0]` erfüllt `dry-run.json`.
- `TestMCPDestructiveCall`: `invoice_delete` `{"id":459}` in `ModeDestructive` → 1 DELETE, `{"id":459,"deleted":true}`; im `ModeWrite` existiert das Tool nicht (`tools/call` → Protokollfehler „unbekanntes Tool“).
- `TestMCPDeliverPdfOnly`: `invoice_deliver` Schema hat genau `id`, `dry_run`; Aufruf → Body `{"delivery_method":"pdf"}` wie `invoice deliver 459 --via pdf --yes`; `{"id":459,"via":"email"}` → `ArgError`, 0 Anfragen.
- `TestMCPNameResolution`: `invoice_create` mit `customer` `"Mustermann GmbH"` → GET Firmen, POST mit ID (gleicher Pfad wie Task 3).
- `TestMCPCrossPath` (`mcp_crosspath_test.go`): Tabelle CLI-Argumente ↔ Tool-Aufruf; je Fall zwei frische mockapi-Server mit gleicher Konfiguration, Vergleich von Methode, Pfad, Query und Body (JSON-gleich) aller Anfragen sowie stdout (byte-gleich). Fälle: `invoice list --company Mustermann --from 2026-01-01 --all --max-pages 2`; `invoice create --name X --customer 2 --payment-term 1130 --document-date heute --item name=A,qty=1,price=100,vat=19`; dasselbe mit `--dry-run`; `invoice update 459 --name Y` (liest die Rechnung); `company archive 2843`; `invoice delete 459 --yes`; `invoice deliver 459 --via pdf --yes`; `invoice paid-status --customer 2 --overdue`; `voucher create … --input <datei>` ↔ `input`-Objekt.

- [ ] **Step 2: Tests laufen lassen, rot**

Run: `go test ./internal/mcpserver/ ./internal/cli/ -run 'MCP' -v`
Expected: FAIL.

- [ ] **Step 3: Implementieren** (`mcpMode`: `config.EnvReadOnly(a.Env)` oder `resolveWith(o.Context, nil)` mit `ReadOnly` → `ModeRead` + Hinweis; deliver-Sonderfall; `dry_run`/`--yes`).

- [ ] **Step 4: Tests grün**

Run: Step-2-Befehl → PASS; `go test -race ./internal/cli/ -run MCPCrossPath -count=3`.

- [ ] **Step 5: Doku**

README „MCP-Server“: Start, Modi (Tabelle Option → Klassen), Schreibschutz, `deliver` nur PDF, `dry_run`, Namen in ID-Feldern, Kontext nur beim Start; Client-Konfiguration als `json`-Block `{"command":"papierkram","args":["mcp","serve","--context","firma"]}` und als `sh` `claude mcp add papierkram -- papierkram mcp serve --context firma`. AGENTS „MCP-Server“: dieselben Regeln als Vertrag – Tool-Ergebnis = stdout-JSON, Fehler = `isError` mit der Fehler-JSON-Zeile (`code`, `exit_code` wie im CLI), stderr als zweiter Inhalt lesen, Exit-11/12-Regeln und „`deliver` nie wiederholen“ gelten unverändert, `--allow-destructive` ist die Freigabe für `--yes` und nur auf ausdrücklichen Wunsch des Menschen. SKILL: ein Absatz „Über MCP“. Spec §2, §5.3, §7.2.

- [ ] **Step 6: Gates und Commit**

Run: G1–G5.

```bash
git add internal/cli/mcp.go internal/cli/mcp_tools.go internal/cli/mcp_test.go internal/cli/mcp_crosspath_test.go internal/cli/docs_examples_test.go internal/mcpserver README.md AGENTS.md skills/papierkram/SKILL.md docs/specs/2026-11-03-papierkram-cli-design.md CHANGELOG.md docs/reference
git commit -m "feat(mcp): write and destructive modes behind explicit flags"
```

**Cross-Pfad-Tabelle (CLI-Aufruf vs. MCP-Tool-Aufruf) – Akzeptanzkriterium:**

| Parameter | CLI | MCP | Begründung |
|---|---|---|---|
| Einstieg | `Run` → `execute` (Signal-Kontext) | `RunContext` → `executeContext` (Kontext der Anfrage) | Abbruch kommt vom Client statt von Strg+C; Exit-Abbildung identisch |
| Befehlsbaum | `NewRootCmd(app)` | `NewRootCmd(child)` | identisch |
| Argumente | Shell | `Args`: Wörter, `--flag=wert`, `Fixed`, `--`, Positionsargumente | Schutz vor Flag-Injektion |
| Ausgabeformat | `-o`/Kontext | immer `-o json` | Spec §3 |
| Rückfragen | Terminal | immer `--no-input`, `StdinTTY` false | nie hängen |
| Kontext | Flag > Env > Projektdatei > aktiv | nur `--context` des Starts | Spec §3: keine Kontext-Parameter |
| `--yes` | Mensch je Aufruf | nur `destructive`/`send` und nur mit `--allow-destructive` | Start mit Option ist die Freigabe (Spec [Call]) |
| `--dry-run` | Flag | `dry_run` | identisch |
| `--input`/`--items` | Datei oder `-` | JSON → Temp-Datei `0600` | stdin ist der Transport |
| stdin | Terminal/Pipe | leer | stdin ist der Transport |
| Env, Schlüsselbund, HTTP-Client, Config-/Cache-Verzeichnis, Uhr, Zone | `NewApp` | von der Server-App kopiert | Schreibschutz und Token wie im CLI |
| `--debug` | `a.Err` | `DebugOut` = Prozess-stderr | nie ins Tool-Ergebnis |
| `deliver --via` | Flag | fest `pdf` | AGENTS-Regel |
| Namensauflösung | Flags mit `resolves` | dieselben Felder (`integer` oder `string`) | ein Pfad |
| Ergebnis | stdout, stderr, Exit | Text-Inhalt stdout, zweiter Inhalt stderr, `isError` + Fehler-JSON | Spec §3 |
| Prüfung, Credits, Schreibschutz, Fehlertexte | CLI | dieselben (kein eigener Pfad) | Anti-Ziel Cross-Pfad-Bug |

**Rulings (Spec offen):** `--allow-destructive` schließt `--allow-writes` ein (zerstörend ohne Anlegen ergibt keinen sinnvollen Modus). Ein Kontext mit `read_only` erzwingt den Lesemodus wie `PAPIERKRAM_READ_ONLY`, mit eigenem Hinweistext. Ein nicht freigeschaltetes Tool existiert nicht (Protokollfehler), statt mit Exit 8/9 zu antworten.

**Akzeptanz:** Tests oben grün; Cross-Pfad-Tabelle im Report; Gates G1–G5.

---

### Task 7: E2E – Dauerkunde, Folgeobjekte, Restesuche und Abdeckung der neuen Features (Spec §2, §7)

**Files:**
- Modify: `internal/e2eguard/names.go` (`PermanentCustomer`, `Leftover`), `internal/e2eguard/names_test.go`
- Modify: `e2e/e2e_test.go` (Kosten-Header, `cleanupReserve`, `TestE2E_CleanupLeftovers`, `deliverPDF`), `e2e/fixtures_test.go` (`permanentCustomer`), `e2e/harness_test.go` (`object.archiveOnly`, `remove`, `mcpSession`, `checkContract`), `e2e/flow_invoice_test.go` (Dauerkunde, paid-status-Messung, Storno-Suche)
- Create: `e2e/flow_paid_status_test.go` (`TestE2E_PaidStatus`), `e2e/flow_mcp_test.go` (`TestE2E_MCPReadTool`)
- Doku: `e2e/README.md` („Was die Suite anlegt“, „Credit-Budget“, Dauerkunde), `docs/offene-punkte.md` (neue Punkte 30, 31), CHANGELOG

**Interfaces:**
- Consumes: `cli.NewMCPServer(app, cli.MCPOptions{})`, SDK-Client und `mcp.NewInMemoryTransports()` (Task 5), `invoice paid-status` (Task 4), Namensauflösung (Task 3), `e2eguard.JudgeFollowUp`/`IsFollowUpType`.
- Produces:
  - `e2eguard.PermanentCustomer = "pk-e2e-dauerkunde"`; `Leftover(name, runID)` ist für diesen Namen false (`Owned` bleibt true).
  - `object.archiveOnly bool` – Aufräumen nur per `archive` (1 Credit), nie `delete`/`cancel`.
  - `(s *liveSuite) permanentCustomer(t *testing.T) int64` – Fixture `dauerkunde`.
  - `(s *liveSuite) mcpSession(t *testing.T, dir string) *mcp.ClientSession` – App wie `execAt`, Server über In-Memory-Transport; zählt Anfragen über `s.tr` wie `exec`.
  - `(s *liveSuite) checkContract(t *testing.T, schema, stdout string)` – Verallgemeinerung von `checkListContract`.

- [ ] **Step 1: Failing Tests (offline)**

`internal/e2eguard/names_test.go`: `TestLeftoverPermanentCustomer` – `Leftover("pk-e2e-dauerkunde", "abc")` false, `Owned("pk-e2e-dauerkunde")` true, `Leftover("pk-e2e-xyz-kunde","abc")` true.

Run: `go test ./internal/e2eguard/ -run Leftover -v` → FAIL; implementieren; PASS.

- [ ] **Step 2: Dauerkunde**

`TestE2E_CleanupLeftovers` merkt sich beim Durchsuchen der Firmen Einträge mit Name `PermanentCustomer` (ID, `record_state`) und fasst sie nicht an. `permanentCustomer`: gemerkte Einträge, sonst `listAll(t, "company")`; 0 → `company create --name pk-e2e-dauerkunde --contact-type customer` (10, `need`); 1 archiviert → `company unarchive` (1); 1 aktiv → 0; mehr als 1 → `fatalf` „mehrere Dauerkunden“. Registrierung `object{resource:"company", name:PermanentCustomer, archiveOnly:true}`.

Nutzer des Dauerkunden: `TestE2E_InvoiceFlow` (Rechnung) und `deliverPDF` (Angebot **ohne** Kontaktperson und Projekt; diese gehören zum Laufkunden). Alle anderen Flows nutzen weiter `s.customer` (der dadurch löschbar wird).

- [ ] **Step 3: `archiveOnly` und Restesuche**

`remove`: `archiveOnly` → nur `archive`, Ergebnis „archiviert (dauerhaft)“. `archiveOnly` setzen für: Dauerkunde, versendetes Angebot, versendete Rechnung, Storno-Folgeobjekt; in `TestE2E_CleanupLeftovers` für Rechnungen/Angebote mit gesetzter Nummer (`invoice_no`/`estimate_no` nicht `null`). Archivierte Reste: nicht angefasst, Log getrennt: `Reste früherer Läufe: %d entfernt, dauerhaft archiviert: %d, übrig: %d`.

- [ ] **Step 4: Storno-Folgeobjekt und paid-status in `TestE2E_InvoiceFlow`**

Nach Schritt 2 (deliver): `s.need(t, 1)`; `invoice paid-status --customer <dauerkunde-ID> -o json` → **hart** Exit 0 und `checkContract("paid-status")`; MESSUNG „paid-status nach Versand“: eigene Rechnung enthalten ja/nein, `state`, `outstanding_amount`, ob die Listeneinträge `outstanding_amount` tragen (Anzahl mit/ohne); nicht enthalten → `deviatef`.
Nach Schritt 4 (cancel): `s.need(t, 1)`; `invoice list --company <dauerkunde-ID> --from today --to today --sort id --desc --limit 100 -o json`; Kandidaten: `id` > Rechnungs-ID und nicht von diesem Lauf registriert; MESSUNG je Kandidat `name`, `state`, `invoice_no`, `keyShape`; genau ein Kandidat → `s.track(object{resource:"invoice", id, name, followUpOf:<rechnung>, followUpType:"invoice", archiveOnly:true})` und MESSUNG „Storno-Rechnung gefunden“; keiner → MESSUNG „kein Folgeobjekt in der Liste“; mehrere → `deviatef`, keinen registrieren.

- [ ] **Step 5: `TestE2E_PaidStatus` (`flow_paid_status_test.go`)**

Ein `dir := t.TempDir()` für beide Aufrufe (gemeinsamer Cache):
1. `s.need(t, 6)`; `invoice paid-status --customer pk-e2e-dauerkunde -o json` über `s.runIn` → **hart**: Exit 0; `checkContract("paid-status")`; stderr enthält `→ <dauerkunde-ID>`; Austausch enthält GET `/contact/companies` und GET `/income/invoices` mit `company_id=<ID>`. MESSUNG `count`, `overdue_count`, `complete`.
2. `s.need(t, 1)`; dasselbe mit `--overdue` → **hart**: Exit 0; kein GET `/contact/companies` (Cache-Treffer); stderr enthält `aus dem Zwischenspeicher`.

- [ ] **Step 6: `TestE2E_MCPReadTool` (`flow_mcp_test.go`)**

`s.need(t, 1)`; `mcpSession`; `ListTools` → **hart**: `user_list` vorhanden, `invoice_create` und `auth_status` nicht, Namen eindeutig (0 Credits). `CallTool("user_list", {"limit":1})` → **hart**: kein `IsError`, `Content[0]` ist JSON mit Array `entries`; kein Inhalt enthält das Token (`s.red.Contains` → `errorf`); Tool-Fehler mit `exit_code` 6 → `markQuota`, `skipf`.

- [ ] **Step 7: Kosten-Header und Reserve neu rechnen**

Header-Zeilen (Beträge im Report rechnerisch belegen, Abweichungen begründen):
- `DeliverPdfEmptyBody`: Dauerkunde (unarchive) 1, Angebot 50, deliver 1, Nachlesen 1 = **53** (bisher 82).
- `VatRate019Is19Percent`: Kunde, Person, Projekt 30, Angebot 50, Artikel 10, 2 describes 2 = **92** (bisher 62; Fixtures wandern hierher).
- `InvoiceFlow`: bisher 60 + paid-status 1 + Storno-Liste 1 = **62**.
- `MCPReadTool`: user_list 1 = **1**; `PaidStatus`: Firmenliste 1 (1 Seite je 100 Firmen) + Liste 1 + Liste mit Cache 1 = **3**.
- Aufräumen: 4 Entwurfsbelege und 13 andere Objekte je 1 delete = 17; Lieferant aus `VoucherFlow` ggf. archive +1; Laufkunde ggf. archive +1; `archiveOnly` (Dauerkunde, versendetes Angebot, versendete Rechnung, Storno) je 1 = 4 → **23** (bisher 30).
- Summe erwartet 615 + 1 + 2 + 3 + 1 = 622 Tests + 23 Aufräumen = **645**; schlimmster Fall erster Lauf +9 (Dauerkunde anlegen statt entarchivieren).
- `creditBudget` bleibt **760**; `cleanupReserve = e2eguard.CleanupReserve(4, 17, 10)` = 60 (4 Entwurfsbelege, 13 andere + 4 `archiveOnly`, Rand 10); Text „Tests dürfen 760 − 60 = 700 nutzen“ aktualisieren. Die Hard/Measured-Liste im Header um die neuen Prüfungen ergänzen.

- [ ] **Step 8: Doku**

`docs/offene-punkte.md`: Übersicht und Einzelpunkte **30** „`outstanding_amount` in der Rechnungsliste“ (Annahme: vorhanden, Dezimal-String; E2E `TestE2E_InvoiceFlow`, Messung) und **31** „Storno-Rechnung nach `invoice cancel`“ (Annahme: eigenes Objekt in der Liste, erkennbar an höherer ID; E2E `TestE2E_InvoiceFlow`, Messung); Ergebnisse trägt der Controller nach dem Live-Lauf ein. `e2e/README.md`: Dauerkunde (einziges Objekt ohne Lauf-ID, nie gelöscht, am Ende archiviert), Budget-Tabelle, neue Tests. CHANGELOG „Geändert“.

- [ ] **Step 9: Gates und Commit**

Run: G1–G3, G6 (offline; **kein Live-Lauf**). `TestOpenPointsNameExistingE2ETests` grün.

```bash
git add internal/e2eguard/names.go internal/e2eguard/names_test.go e2e docs/offene-punkte.md CHANGELOG.md
git commit -m "test(e2e): permanent customer, follow-up search and v0.2 features"
```

**Rulings (Spec offen):** `deliverPDF` bucht auf den Dauerkunden ohne Kontaktperson/Projekt. Nummerierte Belege und der Dauerkunde werden nur archiviert (die API kann sie nicht löschen; jeder weitere Versuch kostet Credits ohne Wirkung). Die Storno-Erkennung registriert nur bei genau einem Kandidaten. MCP-Lesetool ist `user_list` (kleinste, immer vorhandene Liste).

**Akzeptanz:** Gates G1–G3, G6; Header-Summe im Report addiert; `CoveragePlan` unverändert (89, ABDECKUNG bleibt 88/89 im Live-Lauf).

---

### Task 8: SBOM und winget im Release (Spec §6)

**Files:**
- Modify: `.goreleaser.yaml` (`sboms`, `winget`), `.github/workflows/ci.yml` (Job `snapshot`), `.github/workflows/release.yml`
- Create: `scripts/check-fork-push.sh`
- Doku: `docs/release.md` („Was ein Release erzeugt“, „Voraussetzungen“, „Release-Schritte“ Prüfen, „Snapshot“), README „Installation“ (Windows), Haupt-Spec §12.2, CHANGELOG; `internal/cli/docs_examples_test.go` (`docShellCommands` + `winget`)

**Interfaces:**
- Consumes: Archive-ID `papierkram`, Windows-`zip` (`format_overrides`), Secret `PACKAGES_GITHUB_TOKEN`.
- Produces: `scripts/check-fork-push.sh <owner/repo>` – liest die JSON-Antwort von `GET /repos/<owner/repo>` von stdin; Exit 0 nur bei `.permissions.push == true`, sonst Exit 1 mit `PACKAGES_GITHUB_TOKEN kann <owner/repo> nicht beschreiben (docs/release.md, Voraussetzungen)`; leere oder ungültige Eingabe → Exit 1 mit `Antwort von GitHub nicht lesbar`.

- [ ] **Step 1: Failing Selbsttest des Skripts**

Lokal (Ausgabe in den Report):

```sh
printf '{"permissions":{"push":true}}' | sh scripts/check-fork-push.sh Padrio/winget-pkgs; echo $?   # 0
printf '{"permissions":{"push":false}}' | sh scripts/check-fork-push.sh Padrio/winget-pkgs; echo $?  # 1
printf '{}' | sh scripts/check-fork-push.sh Padrio/winget-pkgs; echo $?                             # 1
printf '' | sh scripts/check-fork-push.sh Padrio/winget-pkgs; echo $?                               # 1
```

Vor dem Skript: alle vier scheitern (Datei fehlt). Danach wie kommentiert. Dieselben vier Fälle als CI-Schritt `fork check script` im Job `snapshot`.

- [ ] **Step 2: GoReleaser**

```yaml
sboms:
  - id: archives
    artifacts: archive
    documents: ["{{ .ArtifactName }}.sbom.json"]
    args: ["$artifact", "--output", "spdx-json=$document"]
```

`winget`: `name: papierkram`, `package_identifier: Padrio.Papierkram`, `publisher: Pascal Krason`, `publisher_url`/`homepage` `https://github.com/Padrio/papierkram-cli`, `license: MIT`, `license_url` auf `LICENSE` in `main`, `short_description: Kommandozeile für die Papierkram-API`, `ids: [papierkram]`, `use: archive` (portable aus dem `zip`), `commit_author` Pascal Krason <p.krason@icloud.com>, `skip_upload: auto`, `repository: {owner: Padrio, name: winget-pkgs, branch: "papierkram-{{ .Version }}", token: "{{ .Env.PACKAGES_GITHUB_TOKEN }}", pull_request: {enabled: true, base: {owner: microsoft, name: winget-pkgs, branch: master}}}`. Kommentarblock wie bei `homebrew_casks`.

Run: G9 `check`; `make snapshot` → 6 `dist/*.sbom.json` (SPDX JSON) und 3 Manifest-Dateien `Padrio.Papierkram*.yaml` unter `dist/winget/` (Pfade und Zahl der Prüfsummenzeilen messen und in Step 3 eintragen).

- [ ] **Step 3: CI-Snapshot**

Vor `goreleaser-action`: `anchore/sbom-action/download-syft@<40 hex> # vX.Y.Z` (neueste Release-Version; SHA per `gh api repos/anchore/sbom-action/commits/<tag> --jq .sha`). Prüfschritt ergänzen: genau 6 `dist/*.sbom.json`, jede mit `"spdxVersion"`; die 3 winget-Manifeste; `checksums.txt` mit der in Step 2 gemessenen Zeilenzahl (bisher 10; mit SBOMs voraussichtlich 16 – messen, nicht raten).

- [ ] **Step 4: Release-Workflow**

Nach „token for tap and bucket is set“ und vor GoReleaser: derselbe `download-syft`-Schritt; Schritt `winget fork writable` (`shell: bash`, `env: GH_TOKEN: ${{ secrets.PACKAGES_GITHUB_TOKEN }}`): `set -o pipefail; gh api repos/Padrio/winget-pkgs | sh scripts/check-fork-push.sh Padrio/winget-pkgs`. Scheitert er, veröffentlicht nichts.

- [ ] **Step 5: Doku**

`docs/release.md`: SBOM je Archiv, winget-Manifest per PR gegen `microsoft/winget-pkgs`; Voraussetzungen Pascal: `gh repo fork microsoft/winget-pkgs --clone=false --default-branch-only` (ergibt `Padrio/winget-pkgs`), PAT bearbeiten: Repository-Zugriff um den Fork erweitern (Contents: Read and write; kein neuer Token); der Workflow prüft das vorab. README Installation Windows: `winget install Padrio.Papierkram` mit dem Satz „sobald Microsoft den Pull Request angenommen hat; bis dahin das zip-Archiv“. Spec §12.2: „SBOM optional“ → „SBOM (SPDX, syft) je Archiv; winget-Manifest“.

- [ ] **Step 6: Gates und Commit**

Run: G5, G7, G8, G9; YAML wird im Review gelesen (kein Probelauf des Release-Workflows).

```bash
git add .goreleaser.yaml .github/workflows/ci.yml .github/workflows/release.yml scripts/check-fork-push.sh docs/release.md README.md docs/specs/2026-11-03-papierkram-cli-design.md internal/cli/docs_examples_test.go CHANGELOG.md
git commit -m "build(release): add sboms and winget manifest"
```

**Rulings (Spec offen):** Prüfung in ein Skript ausgelagert (in der CI selbsttestbar, im Release nur mit dem PAT). Branch je Version im Fork. Kein winget-Manifest bei Vorabversionen (`skip_upload: auto`).

**Akzeptanz:** Skript-Selbsttest im Report; Snapshot erzeugt 6 SBOMs und 3 Manifeste; Gates G5, G7–G9.

---

### Task 9: Doku-Sync (Spec §7)

**Files:**
- Modify: `docs/specs/2026-11-03-papierkram-cli-design.md`, `README.md`, `AGENTS.md`, `skills/papierkram/SKILL.md`, `docs/offene-punkte.md`, `CHANGELOG.md`, `docs/reference/` (generiert), `e2e/README.md` (nur Querverweise)

**Interfaces:**
- Consumes: Ergebnisse der Tasks 1–8.
- Produces: konsistente Doku; keine Code-Änderung außer generierter Referenz.

- [ ] **Step 1: Haupt-Spec abgleichen**

§2 Nicht-Ziele (MCP „v1.x“ und Namensauflösung: jetzt Ziel, Verweis auf die v0.2-Spec), §5.3 (Registry → MCP-Tools über `commands --json`), §7.1/§7.2 (`invoice paid-status`, `mcp serve`), §8.4 (`resolved`), §10.1 (Subdomain), §12.2 (SBOM, winget), §14 Ausblick (nur noch `$EDITOR`-Bearbeitung und Scoop), Kopf: Verweis auf die v0.2-Spec.

- [ ] **Step 2: README, AGENTS, SKILL**

README: Inhaltsverzeichnis, „Namen statt IDs“, „Offene Posten“, „MCP-Server“, Installation (winget), Kontexte (Subdomain), Credits (Namensauflösung, paid-status). AGENTS: Regelliste um „Namen kosten Credits; im Zweifel IDs“ und MCP-Regeln; JSON-Vertrag-Tabelle nennt jede Datei aus `docs/schemas/` (inkl. `paid-status.json`, `resolved` in `dry-run.json`); Credits-Absatz um Namensauflösung (1 je Seite, höchstens 5, Cache 0) und paid-status (1 je Seite, `--details` +1 je Rechnung); Selbstbeschreibung um `resolves`. SKILL: Abläufe mit Namen (`--customer "…"` als `# Platzhalter`), paid-status, MCP-Absatz.

- [ ] **Step 3: CHANGELOG und offene Punkte**

`## [Unreleased]`: Einträge aller Tasks konsolidiert unter „Hinzugefügt“/„Geändert“, keine Doppelungen; Umbenennen in `[0.2.0]` macht der Controller beim Release. `docs/offene-punkte.md`: Übersicht und Einzelpunkte 30/31 konsistent.

- [ ] **Step 4: Prüfen**

Run: `make docs`; G1, G5; `go test ./internal/cli/ -run 'DocCommands|Reference|AgentsRecipe|Skill' -v` → PASS; `go test . -run OpenPoints -v` → PASS.

- [ ] **Step 5: Commit**

```bash
git add docs/specs/2026-11-03-papierkram-cli-design.md README.md AGENTS.md skills/papierkram/SKILL.md docs/offene-punkte.md CHANGELOG.md docs/reference e2e/README.md
git commit -m "docs: sync docs for v0.2.0"
```

**Akzeptanz:** Gates G1–G5; Report listet je Datei die geänderten Abschnitte; AGENTS-Regeln 1–8 unverletzt (MCP und Namensauflösung ergänzen sie, ersetzen keine).

---

## Abschluss (Controller, kein Task)

1. Gesamt-Review über alle Commits dieser Runde (opus, `superpowers:requesting-code-review`), Fix-Welle per Fix-Agent, danach Gates G1–G10.
2. Live-Lauf gegen die Sandbox (Frist ca. 2026-11-19) mit Token-Check und Abdeckungszeile (88/89): Ergebnisse der Messungen in `docs/offene-punkte.md` 30/31 eintragen, `paidStatusSkipStates` und die Storno-Erkennung bei Bedarf per Fix-Agent anpassen.
3. Schritte für Pascal: `gh repo fork microsoft/winget-pkgs --clone=false --default-branch-only`; PAT `PACKAGES_GITHUB_TOKEN` um den Fork erweitern.
4. Release **v0.2.0** nach `docs/release.md` (CHANGELOG `[Unreleased]` → `[0.2.0] - <Datum>`, Commit `docs: release v0.2.0`, Tag `v0.2.0`), erst nach grüner CI und Live-Lauf. Danach `HANDOFF.md` und Memory aktualisieren.
