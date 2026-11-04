# papierkram-cli – Implementierungsplan

> **Für agentische Ausführung:** PFLICHT-SUB-SKILL: `superpowers:subagent-driven-development` (empfohlen) oder `superpowers:executing-plans`. Schritte nutzen Checkboxen (`- [ ]`).

**Ziel:** Go-CLI `papierkram`, das alle 89 Operationen der Papierkram-API spec-getrieben abbildet, mit JSON-Vertrag, Exit-Codes 0–12, Schutzmechanismen und Doku auf Deutsch.

**Architektur:** Die per Overlay gepatchte OpenAPI-Spec wird eingebettet und liefert Flags, Typumwandlung, Bereinigung und Validierung. Eine Go-Registry beschreibt jedes Command (Klasse, Credits, Spalten); ein generischer Ressourcen-Builder erzeugt daraus cobra-Commands, Sonderfälle haben eigene Handler. Der HTTP-Wrapper baut Requests über die von oapi-codegen generierten Request-Builder und gibt rohe JSON-Bytes zurück.

**Tech-Stack:** Go (go.mod `go 1.26.0`), cobra (+ cobra/doc), go-toml/v2, zalando/go-keyring, oapi-codegen v2 (`tool`) + runtime, kin-openapi, gojq, golang.org/x/term, goccy/go-yaml, rogpeppe/go-internal/testscript, speakeasy-api Overlay-Bibliothek (nur `tools/`), GoReleaser v2.

**Spec:** `docs/specs/2026-11-03-papierkram-cli-design.md` (einzige Quelle der Wahrheit). API: `api/openapi.json` (nie ändern), Korrekturen in `api/overlay.yaml`.

## Global Constraints

- Modul `github.com/Padrio/papierkram-cli`; Binary `papierkram`; `go 1.26.0` in go.mod (vorletzte Minor zu lokal 1.27.1).
- Nutzersichtbare Texte **Deutsch mit echten Umlauten** (Hilfe, Flag-Beschreibungen, Fehler, Hinweise, Prompts, Doku, cobra-Templates). Englisch: Command-/Flag-Namen, JSON-Schlüssel, Fehlercodes, Code-Kommentare, Commit-Messages.
- Conventional Commits, klein; **niemals `Co-Authored-By`-Zeilen**; Identität kommt aus globaler Git-Config.
- Hinweise/Warnungen nur auf stderr, Nutzdaten nur auf stdout. Ohne TTY: keine Prompts, keine Farben, keine Spinner. `NO_COLOR` beachten.
- Tokens nie in Logs, `--debug`, Fehlern, Fixtures, Golden-Files. Test-Tokens heißen `test-token-…` und werden in Golden-Tests auf Abwesenheit geprüft.
- Keine Wiederholung bei POST/PUT/DELETE oder 429. GET: max. 2 Wiederholungen bei Netzwerkfehler/5xx.
- Leerer 2xx-Body ist nie ein Fehler.
- Keine handgepflegten Feldlisten pro Ressource in Go: Felder, Typen, Enums kommen aus der gepatchten Spec; semantische Korrekturen (z. B. „String-Boolean“) als `x-cli-*`-Extensions im Overlay, kommentiert.
- Regressionstests heißen `TestRNN_<Kurzname>` (`TestR01_DeliverEmptyBody` … `TestR39_VoucherFileUploadPartial`), jeder genau einmal im Repo.
- Kein Test gegen echte Accounts. E2E nur mit `-tags e2e` und allen drei `PAPIERKRAM_E2E_*`-Variablen, sonst `t.Skip`.
- Browser-Automatisierung (falls je nötig) nur headless.
- papierkram-mcp-server: nur Wissen, kein Code; nicht übernehmen: `X-RateLimit-Remaining`, Antwortkürzung, PDF als base64, `normalizeVatRate(19)`→19, falsche Person-/Task-Typen, `archive` als destruktiv.

### Exit-Codes (`internal/clierr`, verbindlich)
| Exit | Code |
|---|---|
| 0 | – |
| 1 | `error`, `server_error` |
| 2 | `usage` |
| 3 | `auth` |
| 4 | `not_found` |
| 5 | `validation` |
| 6 | `quota` |
| 7 | `conflict` |
| 8 | `confirmation_required` |
| 9 | `read_only` |
| 10 | `network` |
| 11 | `partial` |
| 12 | `outcome_unknown` |

### Reihenfolge der Prüfungen bei jeder Mutation (verbindlich)
1. Argumente/Flags (Exit 2) → 2. Schreibschutz Env/Kontext (Exit 9) → 3. Body bauen, bereinigen, lokal validieren (Exit 5) → 4. `--dry-run` ausgeben, Exit 0 → 5. Bestätigung je Klasse (Exit 8) → 6. Senden.
`--dry-run` ist unter Schreibschutz ebenfalls gesperrt (Spec 8.3: „blockiert alle Klassen außer read_only“) **[Builder]**.

## Review Focus

1. **`--token-stdin` mit abschließendem `\n`/`\r\n`** (z. B. `echo token | papierkram auth login --token-stdin` unter Windows): Token wird getrimmt gespeichert, nie mit Zeilenumbruch gesendet → Test in Task 15.
2. **Dezimalzahlen und große IDs:** `price: 19.99`, `total_gross: 1190.00`, ID `9007199254740993` bleiben in `-o json`, Tabellen und `--input`-Round-Trip exakt (Decoder mit `UseNumber`, Rohbytes bei JSON) → Tests in Task 16 und 20.
3. **UTF-8-BOM und CRLF in `--input`/`--items`/`--body-file`** (Windows-Editoren): BOM wird entfernt, JSON parst; Mailtext behält Zeilenumbrüche → Tests in Task 20 und 31.
4. **Zeitzone/Sommerzeit:** `--since 2026-10-25` in Europe/Berlin → `2026-10-25T00:00:00+02:00`, `--until 2026-10-25` → `2026-10-25T23:59:59+01:00`; `time log --duration` bei Zeitumstellung erzeugt korrekte `HH:MM` → Tests in Task 30 und 36.
5. **Nicht-JSON-Fehlerantworten** (HTML-502 vom Proxy, leerer 401-Body): Exit-Code nach Status, kurze deutsche Meldung ohne HTML-Dump, kein Parse-Fehler → Test in Task 12.

## Dateistruktur

```
cmd/papierkram/main.go            os.Exit(cli.Main())
internal/buildinfo/               Version, Commit, Datum (ldflags)
internal/clierr/                  Error-Typ, Codes, Exit-Mapping
internal/spec/                    generate.go, openapi.gen.json (embed), Lookups, Felder, Coercion, VAT, Prune, Validate, Suggest
internal/api/                     Client, Middleware, Retry, Debug, Fehler-Mapping, Quota
internal/api/gen/                 oapi-codegen (client.gen.go) + dispatch.gen.go
internal/auth/                    Store-Interface, Keyring, Mock
internal/config/                  config.toml, Kontexte, Resolve
internal/projectfile/             .papierkram.toml
internal/timer/                   Timer-State
internal/output/                  Formate, Tabellen, --fields, --jq, Fehlerausgabe
internal/registry/                Ressourcen, Einträge (Tabelle), Lookup
internal/cli/                     Root, Meta-Commands, Builder, Guards, Sonderfälle
internal/testutil/mockapi/        spec-validierender httptest-Server
internal/testutil/golden/         Golden-Helfer mit -update
internal/cli/testdata/script/     testscript-Szenarien (*.txtar)
tools/applyoverlay/               Overlay anwenden → internal/spec/openapi.gen.json
tools/gendispatch/                erzeugt internal/api/gen/dispatch.gen.go
e2e/                              //go:build e2e
docs/schemas/                     JSON-Vertragsschemas (OpenAPI-Schema-Objekte)
docs/reference/                   generierte Command-Referenz
skills/papierkram/SKILL.md
.github/workflows/ci.yml, e2e.yml; .golangci.yml; .goreleaser.yaml; Makefile
```

---

## Phase 1 – Scaffold + CI

### Task 1: Modul, Root-Command, `version`, deutsche cobra-Templates
**Files:** `go.mod`, `cmd/papierkram/main.go`, `internal/buildinfo/buildinfo.go`, `internal/cli/root.go`, `internal/cli/app.go`, `internal/cli/templates.go`, `internal/cli/version.go`, Tests `internal/cli/root_test.go`, `Makefile`, `.golangci.yml`
**Interfaces – Produces:**
- `type App struct { In io.Reader; Out, Err io.Writer; Env func(string) string; Now func() time.Time; Location *time.Location; StdinTTY, StdoutTTY bool; ConfigDir, CacheDir string; HTTPClient *http.Client }` – alle Abhängigkeiten injizierbar; `func NewApp() *App` füllt aus OS.
- `func NewRootCmd(app *App) *cobra.Command`; `func Main() int`; `func Run(app *App, args []string) int`.
- `buildinfo.Version, Commit, Date string` (ldflags), `buildinfo.UserAgent() string` = `papierkram-cli/<version> (<os>/<arch>)`.
- Globale Flags (persistent, Werte in `app.Flags`): `--context`, `-o/--output`, `--fields`, `--jq`, `--debug`, `--timeout`, `--no-input`, `-y/--yes`, `--dry-run`, `--no-color`.
- [ ] Tests (rot): `TestRootHelpGerman` – `--help` enthält „Verwendung:“, „Verfügbare Befehle:“, „Flags:“-Äquivalent „Optionen:“, enthält **nicht** „Usage:“/„help for“; `TestVersionJSON` – `version -o json` → Objekt mit `version`, `commit`, `date`, `go_version`; `TestUnknownFlagGerman` – `--nope` → stderr enthält „unbekanntes Flag“.
- [ ] Implementieren: eigene Usage/Help-Templates, `SetFlagErrorFunc` mit deutscher Meldung, Default-Completion-Command abschalten (`CompletionOptions.DisableDefaultCmd`), eigener `help`-Flag-Text „Hilfe zu <cmd>“. Makefile-Ziele `build test lint generate cover snapshot`. `.golangci.yml` im v2-Format (errcheck, govet, staticcheck, revive, misspell aus).
- [ ] Grün: `go test ./... && go build ./cmd/papierkram`
- [ ] Commit: `feat(cli): scaffold root command with german help and version`

### Task 2: `clierr` und Exit-Code-Abbildung in `Main`
**Files:** `internal/clierr/clierr.go`, Test `clierr_test.go`; Modify `internal/cli/root.go`
**Produces:**
```go
type Code string // Konstanten: CodeError="error", CodeServer="server_error", CodeUsage="usage", CodeAuth="auth", CodeNotFound="not_found", CodeValidation="validation", CodeQuota="quota", CodeConflict="conflict", CodeConfirmation="confirmation_required", CodeReadOnly="read_only", CodeNetwork="network", CodePartial="partial", CodeOutcomeUnknown="outcome_unknown"
type Error struct { Code Code; Message, Hint string; HTTPStatus int; CorrelationID string; Cause error }
func New(code Code, msg, hint string) *Error
func (e *Error) Exit() int            // Tabelle oben
func From(err error) *Error           // nicht-clierr → CodeError; cobra-Argumentfehler → CodeUsage
```
- [ ] Tests (rot): Tabelle Code→Exit für alle 13 Codes; `From(errors.New("x")).Exit()==1`; `Run` mit unbekanntem Flag → 2; Textausgabe `Fehler: <msg>\nHinweis: <hint>\nCorrelation-ID: <id>` auf stderr.
- [ ] Implementieren; `Run` gibt `From(err).Exit()` zurück; Fehlerausgabe zunächst nur Text (JSON-Variante in Task 16).
- [ ] Grün + Commit: `feat(clierr): add typed errors and exit codes 0-12`

### Task 3: CI-Skelett
**Files:** `.github/workflows/ci.yml`, `scripts/coverage.sh`
- Job `test` Matrix ubuntu/macos/windows-latest: `go build ./...`, `go test -race ./...`.
- Job `checks` (ubuntu): golangci-lint (Action, Version gepinnt), `go run golang.org/x/vuln/cmd/govulncheck@<pin> ./...`, `go generate ./... && git diff --exit-code`, `scripts/coverage.sh 80` (Profil über alle Pakete außer `internal/api/gen`, `tools/`, `e2e`; Schwelle als Argument; Exit ≠ 0 darunter).
- Keychain-Job folgt in Task 7, GoReleaser-Job in Task 47.
- [ ] `scripts/coverage.sh` lokal mit Schwelle 0 ausführen → OK. Schwelle bleibt bis Task 43 bei 0 in CI (Kommentar „wird in Task 43 auf 80 gesetzt“).
- [ ] Commit: `ci: add build, test, lint and vulncheck workflow`
- [ ] **Phasenende:** push, `gh run watch`.

## Phase 2 – Overlay, Codegen, Embed

### Task 4: Overlay und Overlay-Werkzeug
**Spec:** 4.7, 5.4, 17. **Files:** `api/overlay.yaml`, `tools/applyoverlay/main.go`, `internal/spec/generate.go`, `internal/spec/openapi.gen.json` (generiert), Test `internal/spec/overlay_test.go`; Modify `api/README.md` (ins Deutsche übersetzen, Overlay-Abschnitt ergänzen)
- `generate.go` enthält **alle** go:generate-Direktiven in fester Reihenfolge (Overlay → oapi-codegen → dispatch), damit `go generate ./...` deterministisch ist.
- Overlay (OpenAPI Overlay 1.0, jede Aktion mit YAML-Kommentar „warum“):
  1. `operationId` für alle 89 Operationen nach `<Verb><Resource>[<Action>]`: `List…`, `Get…`, `Create…`, `Update…`, `Delete…`, `Archive…`, `Unarchive…`, `Cancel…`, `CancelVoucherWithReverseEntry`, `PayVoucher`, `DeliverInvoice`, `DeliverEstimate`, `GetInvoicePdf`, `GetEstimatePdf`, `GetVoucherPdf`, `UploadVoucherDocument`, `DeleteVoucherDocument`, `GetInfo`, Ressourcennamen Singular bei Einzelobjekt (`GetCompany`), Plural bei Liste (`ListCompanies`, `ListTimeEntries`, `ListBankConnections`).
  2. PDF-Responses: Content-Type-Key `application/pdf`, Schema `{type: string, format: binary}`.
  3. `deliver`-Response: `200`/`201` ohne `content`.
  4. `order_direction`: `{type: string, enum: [asc, desc]}`.
  5. `null` aus allen Enums entfernen (Nullability bleibt über `nullable`), doppelte Enum-Werte entfernen (`difference_reason`, `category` → 94).
  6. Pfadparameter `id`/`voucher_id`/`company_id` einheitlich `integer`, fehlende Deklarationen ergänzen.
  7. Datumsfelder in Request-Schemas ohne `format` (`*_date`, `deadline`, `entry_date`) → `format: date`, falls nicht vorhanden.
  8. `x-cli-bool-string: true` an Task `complete`, `flagged`, Person `default`, `flagged` (String in der API, semantisch Boolean, Spec 4.4).
  9. `x-cli-enum-hints: {non_eu: foreign}` an Voucher `provenance` (R12).
  10. Falls kin-openapi oder oapi-codegen an `patternProperties` u. Ä. scheitern: entfernen, kommentiert **[Builder]**.
- Werkzeug: `go run ./tools/applyoverlay <spec> <overlay> <out>`; Ausgabe JSON, sortierte Schlüssel, Newline am Ende.
- [ ] Tests (rot): `TestSnapshotUnchanged` (SHA-256 `api/openapi.json` == `da826bc0bd364c1749ae088d42287b818b99e8c536ca5ced61193f2581e9ccea`); `TestPatchedSpecLoadsAndValidates` (kin-openapi `Loader` + `doc.Validate`); `TestOperationIDsComplete` (89 Operationen, 89 eindeutige IDs); `TestPDFResponsesBinary`; `TestDeliverResponseHasNoContent`; `TestOrderDirectionScalar`; `TestNoNullInEnums`; `TestCategoryEnumHas94UniqueValues`; `TestGeneratedSpecUpToDate` (Overlay erneut anwenden == eingebettete Datei).
- [ ] Implementieren, `go generate ./internal/spec/`, grün.
- [ ] Commit: `feat(spec): add openapi overlay and patched spec generation` + `docs(api): translate api readme to german`

### Task 5: `internal/spec` – Laden und Lookups
**Files:** `internal/spec/spec.go`, Test `spec_test.go`
**Produces:**
```go
func Raw() []byte                        // eingebettete gepatchte Spec
func Doc() *openapi3.T                   // sync.Once
type Operation struct { ID, Method, Path string; Op *openapi3.Operation }
func Operations() []Operation            // 89, sortiert nach Path, Method
func OperationByID(id string) (Operation, bool)
func MatchPath(method, concretePath string) (Operation, []int64, bool) // "/income/invoices/12/deliver" → DeliverInvoice, [12]
func RequestSchema(opID string) *openapi3.Schema   // application/json oder multipart, nil wenn keiner
func ResponseSchema(opID string) *openapi3.Schema  // erste 2xx application/json, allOf aufgelöst lassen
func QueryParams(opID string) map[string]*openapi3.Parameter
```
- [ ] Tests (rot): `len(Operations())==89`; `MatchPath("POST","/income/invoices/0/deliver")` → `DeliverInvoice`, `[0]`; Query-Prefix/Trailing-Slash tolerant; `RequestSchema("CreateTask").Properties["complete"].Type` ist string mit `x-cli-bool-string`.
- [ ] Commit: `feat(spec): embed patched spec with operation lookups`

### Task 6: oapi-codegen und Dispatch-Tabelle
**Files:** `internal/api/gen/oapi-codegen.yaml`, `internal/api/gen/client.gen.go`, `tools/gendispatch/main.go`, `internal/api/gen/dispatch.gen.go`, Test `internal/api/gen/dispatch_test.go`; Modify `go.mod` (`tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen`), `internal/spec/generate.go`
**Produces:**
```go
// in package gen (generiert)
func BuildRequest(opID, server string, pathParams []int64, query url.Values, contentType string, body io.Reader) (*http.Request, error)
```
Ruft je Operation den generierten `New<OpID>Request[WithBody]` auf (Pfad aus Generator), setzt danach `URL.RawQuery = query.Encode()`. Generator liest die gepatchte Spec; unbekannte opID → Fehler.
- [ ] Test (rot): für alle 89 Operationen `BuildRequest(id, "http://x/api/v1", ids…)` → Methode stimmt, Pfad == Template mit eingesetzten IDs; ID `0` landet als `/0` (R24-Vorstufe).
- [ ] `go generate ./... && git diff --exit-code` → kein Diff beim zweiten Lauf.
- [ ] Commit: `build(api): generate client and request dispatch with oapi-codegen`
- [ ] **Phasenende:** push, CI beobachten.

## Phase 3 – Config, Kontexte, Keychain

### Task 7: `internal/auth`
**Spec:** 10.2. **Files:** `internal/auth/store.go`, `keyring.go`, Tests `store_test.go`, `keychain_integration_test.go` (`//go:build keychain`); Modify `.github/workflows/ci.yml`
**Produces:** `type Store interface { Get(context string) (string, error); Set(context, token string) error; Delete(context string) error }`, `var ErrNotFound`, `var ErrUnavailable`, `func Keyring() Store` (Service `papierkram-cli`, Account = Kontextname), `func NewMemory() Store`.
- [ ] Tests (rot): Memory-Store CRUD; Keyring-Fehler „kein Secret Service“ → `ErrUnavailable`; Integrationstest Set/Get/Delete mit Kontext `pk-ci-<rand>`.
- [ ] CI: Job `keychain` Matrix 3 OS: Linux `sudo apt-get install -y gnome-keyring dbus-x11` und `dbus-run-session -- sh -c 'echo -n "" | gnome-keyring-daemon --unlock --components=secrets && go test -tags keychain ./internal/auth/...'`; macOS/Windows direkt.
- [ ] Commit: `feat(auth): add keyring token store with memory fallback for tests`

### Task 8: `internal/config` – Datei, Kontexte, Auflösung
**Spec:** 10.1, 10.2, 8.3. **Files:** `internal/config/config.go`, `resolve.go`, Tests
**Produces:**
```go
type Context struct { Subdomain string; ReadOnly bool; Output, Timeout string; User int64; Token string `toml:"token,omitempty"`; CompletionLookup *bool `toml:"completion_lookup,omitempty"` }
type Config struct { ActiveContext string `toml:"active_context"`; Contexts map[string]*Context `toml:"contexts"` }
func Path(configDir string) string                     // <configDir>/papierkram/config.toml; configDir = PAPIERKRAM_CONFIG_DIR oder os.UserConfigDir()
func Load(path string) (*Config, error)                // fehlende Datei = leere Config
func (c *Config) Save(path string) error               // atomar, 0600
func (c *Config) Create/Use/Delete/Rename(...) error   // Fehler clierr usage/not_found
type ResolveInput struct { FlagContext, ProjectContext string; Env func(string) string; Store auth.Store }
type Resolved struct { Context, ContextSource, Subdomain, BaseURL, Token, TokenSource string; ReadOnly bool; ReadOnlySource string; Timeout time.Duration; Output string; User int64; CompletionLookup bool }
func Resolve(c *Config, in ResolveInput) (*Resolved, error)
```
Regeln: Kontext `--context` > `PAPIERKRAM_CONTEXT` > Projektdatei > `active_context`. Token `PAPIERKRAM_TOKEN` > Keychain > `token` in Config. Subdomain `PAPIERKRAM_SUBDOMAIN` > Kontext. `PAPIERKRAM_BASE_URL` überschreibt alles, sonst `https://<subdomain>.papierkram.de/api/v1`. `ReadOnly = env PAPIERKRAM_READ_ONLY=="1"` (Source `env`) ODER Kontext (`context`). Timeout Default 30 s.
- [ ] Tests (rot): Tabellentest Priorität (alle Kombinationen); `PAPIERKRAM_READ_ONLY=1` + Kontext `read_only=false` → `ReadOnly=true, Source=env`; Save-Rechte 0600 (nicht unter Windows prüfen); Rename verschiebt auch `active_context`; Token erscheint in keinem `fmt.Sprint(resolved)` (eigene `String()`-Methode maskiert).
- [ ] Commit: `feat(config): add contexts and resolution order`

### Task 9: `internal/projectfile`
**Spec:** 9.8. **Files:** `internal/projectfile/projectfile.go`, Test
**Produces:** `type File struct { Path string; Context string; Company, Project, Task, User int64; Tasks map[string]int64; Unknown []string }`, `func Find(cwd string) (path string, ok bool)`, `func Load(path string) (*File, error)`, `func (f *File) ResolveTask(v string) (int64, error)` (ID oder Alias).
- [ ] Tests (rot): Suche im cwd; in Git-Repo aufwärts bis einschließlich Wurzel (`.git` Ordner **oder** Datei); außerhalb Git nur cwd; nie über Git-Wurzel; unbekannte Schlüssel in `Unknown`; Alias `tickets`→57, Zahl „57“→57, unbekannter Alias → usage-Fehler mit Liste.
- [ ] Commit: `feat(projectfile): find and parse .papierkram.toml`

### Task 10: `context`-Commands und Config-Schreibsperre
**Spec:** 7.2, 8.3. **R:** R36. **Files:** `internal/cli/context.go`, `internal/cli/guard.go` (`func (a *App) guardConfigWrite() error` → Exit 9 bei `PAPIERKRAM_READ_ONLY=1`), Tests
- `context create <name> --subdomain [--read-only] [--user]`, `use`, `list` (Tabelle/JSON, aktiver markiert), `active`, `delete` (löscht Keychain-Eintrag), `rename`.
- [ ] Tests (rot): `TestR36_ReadOnlyEnvBlocksContextUse` (Exit 9, Config unverändert) für use/create/delete/rename; `context list -o json`.
- [ ] Commit: `feat(cli): add context commands with read-only guard`
- [ ] **Phasenende:** push, CI.

## Phase 4 – HTTP-Wrapper, Fehler, Exit-Codes

### Task 11: API-Client-Kern
**Spec:** 5.4, 5.5, 4.3. **R:** R21. **Files:** `internal/api/client.go`, `quota.go`, Tests
**Produces:**
```go
type Options struct { BaseURL, Token, UserAgent string; Timeout time.Duration; HTTP *http.Client; Debug io.Writer; Sleep func(time.Duration); OnQuota func(Quota) }
type Quota struct { Remaining, Consumed int; Known bool }
type Response struct { Status int; Header http.Header; Body []byte; Quota Quota }
func New(o Options) *Client
func (c *Client) Call(ctx context.Context, opID string, pathParams []int64, query url.Values, body any) (*Response, error)   // body nil → kein Content-Type
func (c *Client) CallRaw(ctx context.Context, method, path string, query url.Values, contentType string, body io.Reader) (*Response, error) // für `api`
func (c *Client) Upload(ctx context.Context, opID string, pathParams []int64, field, filename string, r io.Reader) (*Response, error)
```
- [ ] Tests (rot): Header `Authorization: Bearer test-token-1`, `Accept: application/json`, `User-Agent` wie buildinfo; `Content-Type` nur mit Body; leerer 200/201/204-Body → `Body` leer, kein Fehler; `TestR21_QuotaFromRemainingHeader` (`X-Remaining-Quota: 42`, `X-Consumed-Quota: 7` → Quota; `X-RateLimit-Remaining` wird ignoriert).
- [ ] Commit: `feat(api): add client core with quota headers`

### Task 12: Fehler-Mapping, Netzwerkklassifizierung, Retry
**Spec:** 4.1, 5.5, 10.4. **R:** R22, R23, R38. **Files:** `internal/api/errors.go`, `retry.go`, Tests
- Status→Code: 400/422 validation, 401/403 auth, 404 not_found, 409 conflict, 429 quota (Hinweis „Monatskontingent erschöpft, Paket prüfen“), 5xx server_error. `message`, `correlation_id` aus Fehlerkörper; Nicht-JSON-Körper → deutsche Standardmeldung pro Status, Rohkörper nur in `--debug`.
- Netzwerk: mit `httptrace.ClientTrace.WroteRequest` festhalten, ob die Anfrage vollständig gesendet wurde. GET oder nicht gesendet → `network` (10); gesendet + POST/PUT/DELETE → `outcome_unknown` (12, Hinweis „Ergebnis unbekannt – vor Wiederholung per describe/list prüfen“).
- Retry nur GET bei Netzwerkfehler/5xx, 2×, Backoff 500 ms/1 s über `Options.Sleep`.
- [ ] Tests (rot): Tabelle Status→Exit; `TestR22_NoRetryOn429` (1 Request, Exit 6); `TestR23_NoRetryOnPostNetworkError` (Server schließt Verbindung, 1 Request); GET 503,503,200 → Erfolg mit 3 Requests; `TestR38_TimeoutAfterCreateIsOutcomeUnknown` (Server liest Body, schläft > Timeout → Exit 12); HTML-502 und leerer 401 → Exit 1 bzw. 3, Meldung ohne `<html>`.
- [ ] Commit: `feat(api): map errors to exit codes and retry only safe requests`

### Task 13: `--debug` mit Redaktion
**Spec:** 5.5. **R:** R26. **Files:** `internal/api/debug.go`, Test
- stderr: Request-Zeile, Header (`Authorization: Bearer ***`), Body ≤ 4 KB (sonst „… (gekürzt, N Bytes)“), Status, Dauer, Quota, aktive Schreibschutzstufe; multipart und `application/pdf` nur als Größe.
- [ ] Tests (rot): `TestR26_DebugRedactsToken` (Ausgabe enthält `Bearer ***`, nicht `test-token-`); Body 5000 Bytes gekürzt; PDF als „<binär, N Bytes>“.
- [ ] Commit: `feat(api): add redacted debug logging`

### Task 14: Spec-validierender Mock-Server
**Spec:** 11 (HTTP-Mock). **Files:** `internal/testutil/mockapi/mockapi.go`, Test
**Produces:**
```go
type Recorded struct { Method, Path string; Query url.Values; Header http.Header; Body []byte }
func New(t testing.TB) *Server   // validiert jeden Request mit openapi3filter gegen spec.Doc(); Verstoß → t.Errorf beim Cleanup + Antwort 500
func (s *Server) URL() string    // .../api/v1
func (s *Server) On(method, path string, status int, body []byte, hdr ...string)  // Override, path konkret oder Template
func (s *Server) OnFunc(method, path string, h http.HandlerFunc)
func (s *Server) Requests() []Recorded
func (s *Server) Violations() []string
```
Default-Antwort: erstes 2xx-Beispiel der Operation aus der Spec; unbekannter Pfad → 404 im API-Fehlerformat; Header `X-Remaining-Quota: 9000`, `X-Consumed-Quota: 1000`.
- [ ] Tests (rot): GET `/income/invoices` liefert Spec-Beispiel; POST mit unbekanntem Feld → Violation; Override greift.
- [ ] Commit: `test(mockapi): add spec-validating mock server`

### Task 15: Client-Wiring und `auth login|logout|status`
**Spec:** 7.2, 8.5, 10.1, 10.2. **Files:** `internal/cli/client.go` (`func (a *App) Client(cmd) (*api.Client, *config.Resolved, error)`), `internal/cli/auth.go`, Tests
- `auth login [--context] [--subdomain] [--token-stdin] [--insecure-storage]`: Token verdeckt per `term.ReadPassword` (nur TTY) oder stdin, **getrimmt**; Prüfung per `GET /info`; Speicherung Keychain, bei `ErrUnavailable` nur mit `--insecure-storage` (Warnung) in Config, sonst Exit 3 mit Hinweis auf den Schalter. Unter `PAPIERKRAM_READ_ONLY=1` gesperrt (Exit 9).
- `auth status`: Kontext (+Quelle), Subdomain, Token-Quelle (nie Token), Schreibschutzstufe, Restkontingent aus `GET /info`-Antwortheadern; `-o json`.
- Quota-Warnung < 100 einmal pro Aufruf auf stderr.
- [ ] Tests (rot): login mit `"test-token-9\r\n"` auf stdin → gespeichert `test-token-9`; kein Token → Exit 3; Keychain nicht verfügbar ohne Flag → Exit 3, Meldung nennt `--insecure-storage`; status zeigt `token_source: env|keychain|config`; Warnung bei `X-Remaining-Quota: 50` genau einmal.
- [ ] Commit: `feat(cli): add auth login, logout and status`
- [ ] **Phasenende:** push, CI.

## Phase 5 – Ausgabe

### Task 16: Formate json/yaml, `--fields`, `--jq`, Fehlerausgabe
**Spec:** 10.3, 10.4. **Files:** `internal/output/output.go`, `jq.go`, `errors.go`, Tests
**Produces:**
```go
type Format string // "table","wide","json","yaml"
type Options struct { Format Format; Fields []string; JQ string; Color bool; Out, Err io.Writer }
func Render(o Options, raw []byte, cols Columns) error   // raw = API-JSON
func RenderValue(o Options, v any, cols Columns) error   // für CLI-eigene Objekte (delete, dry-run, …)
func WriteError(o Options, e *clierr.Error)               // Text immer; bei json/yaml zusätzlich {"error":{code,message,hint,http_status,correlation_id,exit_code}}
func Warn(w io.Writer, msg string)                         // "Warnung: …"
```
- JSON ohne `--fields`/`--jq`: Rohbytes eingerückt, Zahlen unverändert. `--fields a,b.c`: Projektion je Objekt bzw. je `entries[]`. `--jq` (gojq) impliziert JSON, jedes Ergebnis eine Zeile.
- [ ] Tests (rot): `19.99` und `9007199254740993` bleiben exakt; `--fields id,billing.company` auf Liste; `--jq '.entries[].id'`; yaml-Ausgabe; Fehler-JSON-Form exakt; `NO_COLOR` schaltet Farbe ab.
- [ ] Commit: `feat(output): add json, yaml, fields and jq rendering`

### Task 17: Tabellen, Spalten, Golden-Helfer
**Files:** `internal/output/table.go`, `columns.go`, `internal/testutil/golden/golden.go`, Tests, `internal/output/testdata/golden/*`
**Produces:** `type Column struct { Header, Path string; Fmt func(any) string }`, `type Columns struct { Table, Wide []Column }`, Formatter `FmtDurationHM` (Minuten/Sekunden laut Schema → `h:mm`), `FmtMoney`, `FmtDate`; `golden.Assert(t, name string, got []byte)` mit Flag `-update`.
- Liste → Zeilen aus `entries`; Objekt → Schlüssel/Wert-Tabelle; leere Liste → nur Kopfzeile, „Keine Einträge.“ auf stderr. `text/tabwriter`, Breite nach Runen.
- [ ] Tests (rot): Golden für Liste, Objekt, leere Liste, Umlaute ausgerichtet.
- [ ] Commit: `feat(output): add table rendering and golden helper`
- [ ] **Phasenende:** push, CI.

## Phase 6 – Registry, Spec-Logik, generischer Builder, Schutz

### Task 18: Registry-Typen und Lese-Ressourcen
**Spec:** 5.3, 7.1, 8.1, 15. **Files:** `internal/registry/registry.go`, `resources.go`, `table.go`, Test `registry_test.go`
**Produces:**
```go
type Class string // ClassReadOnly="read_only", ClassCreate="create", ClassUpdate="update", ClassArchive="archive", ClassDestructive="destructive", ClassSend="send"
type Filter struct { Flag, Param, Kind, Help string } // Kind: "id","date","datetime_from","datetime_to","enum"
type Entry struct { Resource, Action, OperationID, Method, Path string; Class Class; Credits int; Filters []Filter; Columns output.Columns; Short string; Examples []string; Custom bool }
type Resource struct { Name string; Aliases []string; Short string }
func Resources() []Resource
func Entries() []Entry
func Lookup(resource, action string) (Entry, bool)
func ByOperationID(id string) (Entry, bool)
func ExpectedCredits(method, path string) int // Regel aus Spec 4.3
```
Action-Namen: `list describe create update delete archive unarchive cancel reverse pay pdf deliver upload "document delete"`; `info` hat Resource `info`, Action `""`.
Dieser Task trägt alle 14 Ressourcen (Namen, deutsche Aliase, Kurzhilfe) ein und die Einträge Nr. 1–4, 50–51, 59, 88–89 (bank-connection, transaction, payment-term, info, user).
- [ ] Tests (rot): `TestRegistryEntriesMatchSpec` – jede opID existiert, Method/Path identisch, Credits == `ExpectedCredits`, Klasse gültig, keine Duplikate (Resource+Action, opID); Aliase eindeutig über alle Ressourcen.
- [ ] Commit: `feat(registry): add command registry with read-only resources`

### Task 19: Feldmodell, Coercion, VAT, Vorschläge
**Spec:** 4.4, 5.1, 6.1, 6.2. **R:** R7, R8, R9, R10, R11. **Files:** `internal/spec/fields.go`, `coerce.go`, `vat.go`, `suggest.go`, Tests
**Produces:**
```go
type FieldKind string // "scalar","ref","nested_ref","group","ref_list","attrs","items","scalar_list","object_list","binary"
type Field struct { Flag string; Path []string; Kind FieldKind; Type, Format string; Enum []string; Required, BoolString, Repeatable bool; Help string }
func Fields(req *openapi3.Schema, resourceNames []string) []Field
func CoerceFlag(f Field, raw string) (any, error)
func CoerceValue(s *openapi3.Schema, v any, path string) (any, error)
func NormalizeVAT(v any) (float64, error)
func FormatVAT(f float64) string                 // 0.19 → "0.19"
func Suggest(in string, candidates []string, n int) []string // Levenshtein, case-insensitiv
func EnumHint(s *openapi3.Schema, in string) (string, bool)   // x-cli-enum-hints
```
Flag-Regeln (Spec 6.1, „Flagname = Feldname in kebab-case“): Skalar `x_y` → `--x-y`; Objekt nur mit `id` → `--<feld>` (`--payment-term`, `--creditor`, `--customer`, `--project`, `--task`, `--user`, `--proposition`, `--custom-template`, `--default-proposition`); verschachtelte Referenz `a.b{id}` → `--b`, außer `b` ist ein Registry-Ressourcenname → `--a-b` (ergibt `--contact-person`, `--customer-project`); Objekt mit Skalaren → `--<feld>-<sub>` (`--billing-company`); Array von `{id}` → wiederholbar, Singular (`--team-member`); `custom_attributes` → `--attr id=wert`; `line_items` → `--item`/`--items`; Array von Skalaren (`entertainment_persons`) → wiederholbar, Singular (`--entertainment-person`, Kind `scalar_list`); andere Objekt-Arrays (`people`) nur per `--input` (Kind `object_list`, kein Flag); Dateifeld mit `format: binary` (Upload) → kein Flag, setzt der Befehl (Kind `binary`). Typ `boolean` oder `x-cli-bool-string` → Bool-Flag (`"true"`/`"false"` bei String).
Datumsfelder (`format: date`): `YYYY-MM-DD`, `today`/`heute`, `yesterday`/`gestern` (Spec 6.1, Bezug `App.Now` in `App.Location`).
VAT: `19`, `19%`, `"19"`, `0,19`, `0.19`, Zahl 19 → 0.19; `7` → 0.07; `0` → 0; > 1 → /100 (auf 4 Stellen gerundet); < 0, > 100, nicht parsebar → validation. String-Feld `vat_rate` (Artikel) → `FormatVAT`.
- [ ] Tests (rot): `TestR07_BudgetAsString` (`--budget-money 5000` und Wert 5000 aus JSON → `"5000"`); `TestR08_TaskFieldsAsString` (`complete`, `relative_costs`, `flagged`); `TestR09_PersonDefaultFlaggedAsString`; `TestR10_PropositionPriceVatAsString` (`price` 12.5 → `"12.5"`, `vat_rate` 19 → `"0.19"`); `TestR11_VatNormalization` (alle fünf Formen + `7` + `0`); Flag-Set für `CreateInvoice` enthält genau `--customer --contact-person --customer-project --payment-term --custom-template --billing-*` usw.; `Suggest("note", […"notes"…],1)==["notes"]`.
- [ ] Commit: `feat(spec): derive flags from schema and coerce values`

### Task 20: Bereinigung und Validierung
**Spec:** 6.3, 6.4. **R:** R12, R13, R14, R17, R28, R31. **Files:** `internal/spec/prune.go`, `validate.go`, Tests mit Fixtures aus Spec-Beispielen
**Produces:**
```go
type PruneResult struct { Body map[string]any; Removed []string }
func Prune(body map[string]any, req, resp *openapi3.Schema) (PruneResult, error) // Request-Schema hat Vorrang; nur-Response-Felder entfernt; unbekannt → validation mit Vorschlag
func Validate(body any, req *openapi3.Schema) error                              // kin-openapi VisitJSON, alle Fehler gesammelt, deutsch, Feldpfad
func DecodeJSON(r io.Reader) (any, error)                                        // UseNumber, BOM entfernen
```
Response-Schema: `allOf` vereinigen, `*WithDetailedEmbeds` berücksichtigen. Fehlertexte: `unbekanntes Feld "note" – meintest du "notes"?`; Enum: `ungültiger Wert "non_eu" für provenance – meintest du "foreign"? Erlaubt: domestic, eu, foreign`; Kategorie: drei ähnlichste Werte; Datum: Hinweis `YYYY-MM-DD`; vat_rate: Hinweis `0.19`.
- [ ] Tests (rot): `TestR12_ProvenanceNonEuSuggestsForeign`; `TestR13_NoteSuggestsNotes`; `TestR14_UpdateInputPrunesReadOnly` (Spec-Beispiel `GetInvoice` → Body ohne `id`, `invoice_no`, `created_at`, `line_items[].total_net`; Removed-Liste); `TestR17_LineItemsRequired` (Invoice/Estimate/Voucher ohne Positionen → validation); `TestR28_CategoryTypoThreeSuggestions`; `TestR31_DescribeRoundTripKeepsItems` (Positionsfelder bleiben, `vat_rate "19%"` → 0.19); BOM-Datei parst; `19.99` bleibt `json.Number`.
- [ ] Commit: `feat(spec): prune input to request schema and validate locally`

### Task 21: Positions-Parser
**Spec:** 6.2. **Files:** `internal/cli/items.go`, Test
**Produces:** `func ParseItem(s string) (map[string]any, error)`; `func LoadItems(path string, stdin io.Reader) ([]any, error)`.
Grammatik: `key=value` getrennt durch `,`; Werte in `"…"` dürfen `,` und `=` enthalten, `\"` und `\\` als Escape; Kurzformen `qty`→`quantity`, `vat`→`vat_rate`; doppelte Schlüssel → usage-Fehler; unbekannte Schlüssel → Vorschlag aus Positionsschema.
- [ ] Tests (rot): `name="Beratung, vor Ort",qty=2,price=100,vat=19`; Escape-Fälle; leerer Wert; offenes Anführungszeichen → Fehler mit Spalte; `--items -` von stdin.
- [ ] Commit: `feat(cli): parse line items from flags and files`

### Task 22: Generischer Builder – Lesen und einfache Aktionen
**Spec:** 7.1 (gemeinsame list-Flags), 10.3 (JSON-Vertrag), 8.5. **R:** R3, R20, R24, R30. **Files:** `internal/cli/builder.go`, `builder_list.go`, `builder_action.go`, Tests mit mockapi
**Produces:** `func BuildResourceCommands(app *App, root *cobra.Command)` (eine Gruppe je `registry.Resource`, Aliase, Einträge mit `Custom=true` überspringen); `func (a *App) runList(cmd, e registry.Entry) error`; `runDescribe`; `runSimpleAction` (archive, unarchive, cancel, reverse, delete).
- list: `--page`, `--limit` (1–100, Default 25, sonst Exit 2), `--all`, `--max-pages`, `--sort` + `--desc` (immer `order_by` und `order_direction`), Filter aus `Entry.Filters` (Enum aus Spec-Parameter geprüft, Datum `YYYY-MM-DD`/today/heute/yesterday/gestern). `--all`: Envelope zusammenführen (`page=1`, `has_more=false`), ab 10 Seiten Warnung, mit TTY Nachfrage.
- describe: genau ein Argument, Integer ≥ 0, sonst Exit 2.
- delete → `{"id":N,"deleted":true}`; Aktion mit leerem Body → `{"id":N,"ok":true}`, sonst API-Objekt.
- [ ] Tests (rot): `TestR03_EmptyBodyActionIsSuccess`; `TestR20_SortAlwaysSendsDirection` (`--sort name` → `order_by=name&order_direction=asc`); `TestR24_IDZeroIsSent` (`describe 0` → `/…/0`); `TestR30_UnknownBillingStateRejectedLocally` (kein Request, Exit 5); `--all` mit 3 Seiten; `--limit 101` → Exit 2. Pilot: `user list|describe`, `payment-term list|describe`, `bank-connection`, `transaction` (`--bank-connection`), `info`.
- [ ] Commit: `feat(cli): add generic list, describe and action commands`

### Task 23: Generischer Builder – create/update
**Spec:** 6.1–6.4, 6.3a. **R:** R18, R32. **Files:** `internal/cli/builder_write.go`, Tests
**Produces:** `func (a *App) buildBody(cmd *cobra.Command, e registry.Entry, isUpdate bool) (map[string]any, []string, error)` (Body, entfernte Felder); `runCreate`, `runUpdate`.
Ablauf: `--input file|-` laden → Prune → nur geänderte Flags darüberlegen → `--attr` (Wert nach `data_type`, falls Definition bekannt, sonst String; keine Definition laden ohne Bedarf) → Positionen (`--item`/`--items`) → CoerceValue → Validate. update: `--item`/`--items`/leeres `line_items` ohne `--replace-items` → Exit 2 mit Erklärung; mit `--replace-items` wird auch `[]` gesendet. Hinweis auf entfernte Felder auf stderr.
- [ ] Tests (rot): `TestR18_EmptyLineItemsSentWithReplace`; `TestR32_UpdateItemRequiresReplaceItems` (Exit 2, 0 Requests); Flags überschreiben `--input`; nur geänderte Flags im PUT; `--attr 3=42`.
- [ ] Commit: `feat(cli): add generic create and update with spec-driven flags`

### Task 24: Schutzmechanismen
**Spec:** 8.1–8.4. **R:** R25, R35. **Files:** `internal/cli/guard.go` (erweitern), `confirm.go`, `dryrun.go`, Tests
**Produces:**
```go
type Summary struct { Resource string; ID int64; Name, Amount, Recipient, Subject string }
func (a *App) guardMutation(e registry.Entry, r *config.Resolved) error             // Exit 9
func (a *App) confirm(e registry.Entry, s Summary) error                            // destructive/send; TTY-Prompt „ja“/„yes“, delete: ID eintippen; sonst Exit 8 mit Hinweis „--yes verwenden“
func (a *App) dryRun(e registry.Entry, method, url string, body any, upload *UploadInfo) error // {"dry_run":true,"method","url","body","credits"}
```
Zusammenfassung für Bestätigung per GET nur mit TTY und ohne `--yes`. Reihenfolge gemäß Global Constraints. Alle Builder-Pfade (Task 22/23) rufen die Guards auf.
- [ ] Tests (rot): `TestR25_ArchiveInvoiceNoConfirmation` (ohne TTY, ohne `--yes` → Exit 0); `TestR35_ReadOnlyEnvOverridesWritableContext` (Exit 9, 0 Requests); delete ohne TTY/ohne `--yes` → Exit 8; `--no-input` mit TTY → Exit 8; TTY + Eingabe „ja“ → gesendet; dry-run JSON exakt, 0 Requests; Kontext `read_only=true` + `--context` anderer → erlaubt.
- [ ] Commit: `feat(cli): enforce action classes, confirmation and dry-run`
- [ ] **Phasenende:** push, CI.

## Phase 7 – Ressourcen (je Task: Registry-Einträge mit Filtern, Spalten laut Spec 10.3, deutscher Kurzhilfe, ≥ 2 Beispielen; Golden table/wide/json/yaml für list und describe; mockapi-Test je Aktion)

### Task 25: company + person
**Spec:** 7.1, 9.7, 15 Nr. 5–16. **R:** R9 (CLI-Ebene), R19.
- `company list --type customer|supplier`: alle Seiten laden (Warnung ab 10), lokal filtern, Pagination-Flags auf gefiltertes Ergebnis; Hilfe sagt das ausdrücklich.
- `person …` verlangt `--company` (Pfadparameter `company_id`), sonst Exit 2.
- [ ] Tests: `TestR19_CompanyListTypeFiltersAllPages`; person ohne `--company` → Exit 2; Golden.
- [ ] Commit: `feat(cli): add company and person commands`

### Task 26: invoice + estimate (Basis)
**Spec:** 15 Nr. 30–37, 40–47. **R:** R14 (CLI), R17 (CLI), R31 (CLI). deliver/pdf/from-estimate folgen in Phase 8 (Einträge mit `Custom=true` schon jetzt anlegen).
- Filter `--company`→`company_id`, `--project`→`project_id`, `--from`/`--to`→`document_date_range_start/_end`.
- [ ] Tests: create ohne Position → Exit 5, 0 Requests; `describe -o json | update --input -` (über Szenario-Helfer) → spec-konformer PUT; archive ohne Bestätigung; cancel verlangt Bestätigung; Golden.
- [ ] Commit: `feat(cli): add invoice and estimate commands`

### Task 27: voucher (Basis)
**Spec:** 15 Nr. 19–27. **R:** R12 (CLI), R28 (CLI). pay/pdf/upload/document delete → Phase 8 (`Custom=true`).
- Filter `--creditor`, `--project`, `--from`, `--to`; `reverse` → `CancelVoucherWithReverseEntry`; Completion-Liste für `category` vorbereitet.
- [ ] Tests: `--item category=Bürobedarf…` Tippfehler → Exit 5 mit drei Vorschlägen; `provenance non_eu` → Vorschlag `foreign`; Golden.
- [ ] Commit: `feat(cli): add voucher commands`

### Task 28: proposition, payment-term, info, user, bank-connection, transaction (Spalten/Golden)
**Spec:** 15 Nr. 1–4, 50–59, 88–89. **R:** R10 (CLI).
- payment-term Spalten: `id, name` + Fälligkeit in Tagen (Feld aus Schema wählen) **[Builder]**.
- [ ] Tests: `proposition create --vat-rate 19 --price 12.5` → Body `"0.19"`, `"12.5"`; Golden für alle sechs.
- [ ] Commit: `feat(cli): add proposition and reference data commands`

### Task 29: project, task, custom-attribute
**Spec:** 15 Nr. 60–80, 9.8 (project describe ohne ID, task list/create aus Projektdatei). **R:** R7 (CLI), R8 (CLI).
- [ ] Tests: `project create --budget-money 5000` → `"5000"`; `task create --complete --flagged` → `"true"`; `project describe` ohne ID mit Projektdatei; ohne Projektdatei → Exit 2; Golden.
- [ ] Commit: `feat(cli): add project, task and custom attribute commands`

### Task 30: time (CRUD + list)
**Spec:** 15 Nr. 81–87, 7.1. **R:** R16 (create/update-Ebene), R30 (CLI).
- Filter `--project --task --invoice --user --state` (→ `billing_state`, Enum) `--since --until` (RFC 3339, lokale Zone; Datum: since 00:00:00, until 23:59:59). Spalte `duration` als `h:mm`.
- [ ] Tests: `--since 2026-10-25 --until 2026-10-25` (Europe/Berlin) → `2026-10-25T00:00:00+02:00` / `2026-10-25T23:59:59+01:00`; `--started-at-time 9:00` → validation (Hinweis `HH:MM`); Golden.
- [ ] Commit: `feat(cli): add time entry commands`
- [ ] **Phasenende:** push, CI.

## Phase 8 – Sonderfälle

### Task 31: deliver (invoice, estimate)
**Spec:** 9.1, 10.4 (Ergebnis unbekannt bei deliver). **R:** R1, R2, R4, R5, R6, R37.
- `--via email|pdf` Pflicht; email: `--to`, `--subject`, `--body` oder `--body-file` (BOM entfernen, Zeilenumbrüche behalten); Alias `send`; Klasse send mit Zusammenfassung (Empfänger, Betreff); `--input` erlaubt.
- Erfolg: GET nachlesen, Ausgabe `id`, `invoice_no`/`estimate_no`, `state`, `sent_on`, `sent_via`, `sent_to`.
- [ ] Tests: `TestR01_DeliverEmptyBody`; `TestR02_DeliverReadbackFails` (Exit 0, Warnung enthält „NICHT erneut ausführen“); `TestR04_DeliverWithoutVia`; `TestR05_DeliverEmailMissingFields`; `TestR06_DeliverInputAddressInsteadOfRecipient` (Exit 5, 0 Requests); `TestR37_DeliverTimeoutReadbackShowsSent` (Exit 0 + Warnung; ohne `sent_on` → Exit 12).
- [ ] Commit: `feat(cli): add deliver with readback and outcome handling`

### Task 32: pdf
**Spec:** 9.2. **R:** R29.
- `pdf <id> [-O datei|-]`; ohne `-O` auf TTY → Exit 2 mit Hinweis; ohne TTY → stdout; Meldung `{"path","bytes"}` auf stderr bzw. bei `-o json` auf stdout.
- [ ] Tests: `TestR29_PdfRefusesTTY`; `-O out.pdf` schreibt Bytes exakt; `-O -` Binärbytes unverändert.
- [ ] Commit: `feat(cli): add pdf download for invoices, estimates and vouchers`

### Task 33: voucher upload, document delete, create --file
**Spec:** 9.3. **R:** R39.
- multipart Feld `file`, MIME aus Endung (`mime.TypeByExtension`, Fallback `application/octet-stream`), keine lokale Einschränkung; dry-run zeigt Dateiname, Größe, MIME.
- `create --file`: Beleg anlegen, dann hochladen; Upload-Fehler → Exit 11, stdout `{"id":N,"completed":["create"],"failed":["upload"],"retry_command":"papierkram voucher upload N <datei>","warnings":[…]}`.
- [ ] Tests: `TestR39_VoucherFileUploadPartial`; Upload-Request spec-konform (mockapi); document delete verlangt Bestätigung.
- [ ] Commit: `feat(cli): add voucher document upload and create --file`

### Task 34: invoice create --from-estimate
**Spec:** 9.4.
- Angebot lesen (typisiertes Modell aus `gen`), Body bauen (Feldliste gegen `EstimateWithDetailedEmbeds` und Rechnungs-Request-Schema abgleichen, nicht übertragbare Felder in der Hilfe nennen), Flags überschreiben, `--payment-term` Pflicht (Exit 2), Lesefehler → normaler Fehler, nichts angelegt.
- [ ] Tests: Body aus Spec-Beispiel exakt (Golden-JSON), `vat_rate "19%"` → 0.19, `--dry-run` zeigt Body ohne POST.
- [ ] Commit: `feat(cli): create invoices from estimates`

### Task 35: voucher pay
**Spec:** 9.5.
- `--amount`→`value`, `--transaction`→`banking_transaction:{id}`, `--date`→`payment_date`, `--reason`→`difference_reason` (Enum); mindestens eins von amount/transaction (Exit 2); Klasse destructive.
- [ ] Tests: Body-Mapping; ohne beide → Exit 2; Bestätigung erforderlich.
- [ ] Commit: `feat(cli): add voucher pay`

### Task 36: `local init|show` und `time log`
**Spec:** 9.6, 9.8. **R:** R15, R16.
- `local init`: interaktiv nur mit TTY, sonst Flags (`--company --project --task --user --context`); unter `PAPIERKRAM_READ_ONLY=1` Exit 9. `local show`: wirksame Werte + Herkunft (`flag|env|projectfile|context|default`).
- `time log`: `--task` (ID|Alias) `--date` (`today`/`heute`…) `--from/--to HH:MM` oder `--duration` (`1h30m`, `90m`, `1:30`) `[--from]`, `-m`, `--unbillable`; task/user: Flag > Env (`PAPIERKRAM_TASK`, `PAPIERKRAM_USER`, …) > Projektdatei > Kontext; Mitternacht überschritten → Exit 2; TTY-Zusammenfassung mit Namen (gecacht 60 s), Enter = ja, `--yes` überspringt.
- [ ] Tests: `TestR15_TimeLogWithoutTaskUser` (Hinweis `papierkram local init`); `TestR16_TimeEntryBodyHHMM` (`entry_date`, `started_at_time`, `ended_at_time` `HH:MM`); `--duration 1h30m` ohne `--from` endet `Now`; Mitternacht → Exit 2; Zeitumstellung 2026-10-25.
- [ ] Commit: `feat(cli): add project file commands and time log`

### Task 37: Timer
**Spec:** 9.6. **Files:** `internal/timer/timer.go`, `internal/cli/time_timer.go`
**Produces:** `type State struct { Context string; StartedAt time.Time; Task, User int64; Comment string }`, `func Path(cacheDir, ctx string) string` (`<cacheDir>/papierkram/timer-<ctx>.json`, cacheDir = `PAPIERKRAM_CACHE_DIR` oder `os.UserCacheDir()`), `Load/Save/Delete`.
- `time start` (läuft schon → Exit 7), `status`, `stop [-m]` (ruft time-log-Logik; State erst nach Erfolg löschen), `discard`.
- [ ] Tests: start/stop-Fluss mit Mock; stop mit Fehler → State bleibt; doppelter start → Exit 7.
- [ ] Commit: `feat(cli): add local timer for time tracking`
- [ ] **Phasenende:** push, CI.

## Phase 9 – Meta-Commands

### Task 38: `api`
**Spec:** 7.2. **R:** R27, R33, R34.
- `api <METHOD> <path> [-f key=value] [-F key=@file] [--input file.json] [--paginate] [--validate]`; Klasse per `registry` über `spec.MatchPath`; unbekannt: GET → read_only, sonst destructive; deliver-Regel (send_via email ohne vollständiges `email` → Exit 5); Body ohne Bereinigung; `--validate` prüft gegen Request-Schema.
- [ ] Tests: `TestR27_ReadOnlyContextBlocksApiPost` (Exit 9, 0 Requests); `TestR33_ApiDeliverEmailWithoutEmailObject`; `TestR34_ApiDeleteWithoutYesNonTTY` (Exit 8); `--paginate` führt Seiten zusammen.
- [ ] Commit: `feat(cli): add api escape hatch with registry protections`

### Task 39: `commands --json` und `schema`
**Spec:** 7.2, 11 (Vertrag). **Files:** `internal/cli/commands.go`, `schema.go`, `docs/schemas/commands.json`, Tests
- Format: `{"version":1,"commands":[{"command":"invoice create","aliases":[…],"short":"…","class":"create","credits":50,"operation_id":"CreateInvoice","method":"POST","path":"/income/invoices","flags":[{"name","type","required","repeatable","enum","description"}],"examples":[…]}]}` inkl. Meta-Commands (ohne operation_id).
- `schema <resource> [action]` → `{"request":…,"response":…}` aufgelöst.
- [ ] Tests: Ausgabe validiert gegen `docs/schemas/commands.json` (OpenAPI-Schema-Objekt, Validierung mit kin-openapi); alle 89 opIDs enthalten; `schema invoice create` enthält `line_items`.
- [ ] Commit: `feat(cli): add machine-readable commands and schema output`

### Task 40: Completion und Doku-Generator
**Spec:** 7.2, 7.3, 12.3. **Files:** `internal/cli/completion.go`, `gendocs.go`
- `completion bash|zsh|fish|powershell` (deutsche Hilfe); Enum-Completion für Flags (inkl. 94 Kategorien); dynamische ID-Completion (`page_size=100`, `id\tname`, Cache `<cacheDir>/papierkram/completion-<ctx>-<resource>.json` 60 s, aus bei `completion_lookup=false`).
- versteckt `gen-docs --dir docs/reference --man man/` (cobra/doc).
- [ ] Tests: `__complete invoice describe ""` liefert IDs aus Mock, zweiter Aufruf ohne Request; `completion_lookup=false` → kein Request; gen-docs schreibt Markdown für jedes Command.
- [ ] Commit: `feat(cli): add shell completion and docs generator`
- [ ] **Phasenende:** push, CI.

## Phase 10 – Abdeckung, Szenarien, Golden, Regression

### Task 41: Abdeckungs- und Regressions-Gate
**Files:** `internal/registry/coverage_test.go`, `regression_index_test.go` (Repo-Wurzel-Paket oder `internal/cli`)
- [ ] `TestCoverageAllOperations`: Spec→Registry und Registry→Spec, genau 89/89, Klasse und Credits gemäß Tabelle Spec 15 (Tabelle als Testdaten aus der Spec-Markdown geparst).
- [ ] `TestRegressionIndex`: alle `_test.go` nach `func TestR(\d\d)_` scannen → 01–39 je genau einmal.
- [ ] Rot-Nachweis: Eintrag temporär entfernen → Test rot; zurück.
- [ ] Commit: `test: enforce operation coverage and regression index`

### Task 42: testscript-Szenarien und JSON-Vertrag
**Files:** `internal/cli/script_test.go`, `internal/cli/testdata/script/*.txtar`, `docs/schemas/{error,dry-run,delete,partial,deliver}.json`
- Setup: mockapi je Skript, Env `PAPIERKRAM_BASE_URL`, `PAPIERKRAM_TOKEN=test-token-script`, `PAPIERKRAM_CONFIG_DIR`, `PAPIERKRAM_CACHE_DIR`; TTY-Simulation nur über Test-Hook im Testbinary (`PK_TEST_TTY=1` wird ausschließlich vom Test-Main gelesen, nie vom Produktions-Binary). Eigene Befehle `mock` (Override) und `mockreq` (Anzahl/Body prüfen).
- Szenarien: `help.txtar` (deutsch), `commands.txtar`, `invoice_flow.txtar` (create → describe → deliver pdf), `readonly.txtar`, `dryrun.txtar`, `confirm_tty.txtar`/`confirm_notty.txtar`, `time_log.txtar`, `voucher_partial.txtar`, `errors_json.txtar`; jede Ausgabe zusätzlich ohne `test-token-`.
- [ ] JSON-Ausgaben gegen `docs/schemas/*` validiert.
- [ ] Commit: `test: add end-to-end cli scenarios and json contract schemas`

### Task 43: Coverage ≥ 80 %
- [ ] `scripts/coverage.sh 80` lokal; Lücken mit gezielten Tests schließen (keine Tests ohne Assertion); CI-Schwelle auf 80 setzen.
- [ ] Commit: `test: raise coverage above 80 percent` / `ci: enforce 80 percent coverage`
- [ ] **Phasenende:** push, CI.

## Phase 11 – E2E

### Task 44: E2E-Suite
**Spec:** 11.1, 17. **Files:** `e2e/e2e_test.go` (`//go:build e2e`), `.github/workflows/e2e.yml` (`workflow_dispatch`, Secrets)
- `TestMain`: Skip, wenn `PAPIERKRAM_E2E_SUBDOMAIN`/`_TOKEN`/`_SANDBOX_CONFIRM` fehlen oder `SANDBOX_CONFIRM != SUBDOMAIN`. Präfix `pk-e2e-<runid>-`, Aufräumen am Ende; `TestE2E_CleanupLeftovers`.
- Prüfungen (offene Punkte Spec 17 als Messung, nicht geraten): `DeliverPdfEmptyBody`, `QuotaHeadersPresent`, `VatRate019Is19Percent` (Position + Artikel), `PersonDefaultIsString`, `UploadMimeTypes` (pdf, png, jpg, txt; Ergebnis loggen), `VoucherStateValues`, `InfoCreditCost`, `PartialUpdateSemantics`, `ReplaceLineItems`. Credit-Budget im Dateikopf (< 500).
- [ ] `go vet -tags e2e ./e2e/... && go test -tags e2e ./e2e/...` → kompiliert, „SKIP“. CI-Job `test` führt das mit aus.
- [ ] Commit: `test(e2e): add opt-in live suite for sandbox accounts`

## Phase 12 – Doku

### Task 45: README, AGENTS.md, THIRD_PARTY_NOTICES, CHANGELOG, offene Punkte
- `README.md`: Installation (Homebrew/Scoop/deb/rpm „ab erstem Release“), Schnellstart, Kontexte, Projektdatei, Sicherheit (Schreibschutz, Keychain), `pk`-Hinweis, Env-Variablen inkl. `PAPIERKRAM_CONFIG_DIR`/`PAPIERKRAM_CACHE_DIR`.
- `AGENTS.md`: JSON-Vertrag, Exit-Codes, `PAPIERKRAM_READ_ONLY=1`, `--dry-run`, Credits, verbotene Muster (`deliver` nie wiederholen, Exit 11/12 nicht blind wiederholen).
- `THIRD_PARTY_NOTICES.md` (aus `go-licenses`-Lauf oder `go list -m all` manuell), Dank an papierkram-mcp-server. `CHANGELOG.md` (Keep a Changelog, `Unreleased`). `docs/offene-punkte.md` (Spec 17 + Verweis auf E2E-Tests).
- [ ] Prüfung: alle Beispielbefehle aus README/AGENTS.md per Test gegen `commands --json` (Command existiert, Flags existieren).
- [ ] Commit: `docs: add readme, agent guide and notices`

### Task 46: Skill und generierte Referenz
- `skills/papierkram/SKILL.md` (Frontmatter name/description; Workflows: Beleg hochladen und zuordnen, Rechnung aus Angebot, offene Posten prüfen, Zeiten erfassen und abrechnen; immer `-o json`, `--dry-run` vor Mutationen, Exit-Codes).
- `docs/reference/` per `go run ./cmd/papierkram gen-docs`; Test `TestReferenceUpToDate` (neu generieren == eingecheckt).
- [ ] Commit: `docs: add claude code skill and generated command reference`
- [ ] **Phasenende:** push, CI.

## Phase 13 – GoReleaser-Snapshot

### Task 47: GoReleaser
**Spec:** 12.2. **Files:** `.goreleaser.yaml`, `scripts/completions.sh`, `scripts/manpages.sh`; Modify `ci.yml`
- builds darwin/linux/windows × amd64/arm64, `CGO_ENABLED=0`, ldflags buildinfo; Archive mit Completions + Man-Pages; Checksums; nfpm deb/rpm mit `/usr/bin/pk`-Symlink; Homebrew (`Padrio/homebrew-tap`, `pk`-Link) und Scoop (`Padrio/scoop-bucket`, `pk`-Shim) **vorbereitet**, Token `{{ .Env.PACKAGES_GITHUB_TOKEN }}`; falls GoReleaser den `pk`-Alias für Scoop/Brew nicht abbildet: dokumentierter Nachbearbeitungsschritt **[Builder]**.
- CI-Job `snapshot`: `goreleaser release --snapshot --clean` (Action, Version gepinnt).
- [ ] Lokal: `go run github.com/goreleaser/goreleaser/v2@<pin> check` und `… release --snapshot --clean` → 6 Archive + deb/rpm in `dist/`.
- [ ] Commit: `build: add goreleaser configuration with snapshot ci job`
- [ ] **Phasenende:** push, CI vollständig grün.

## Definition of Done (Verifikation)

```bash
go test -race ./...                         # inkl. TestCoverageAllOperations 89/89, TestRegressionIndex 39/39
golangci-lint run ./...
go run golang.org/x/vuln/cmd/govulncheck@<pin> ./...
go generate ./... && git diff --exit-code
scripts/coverage.sh 80
go test -tags e2e ./e2e/...                 # SKIP ohne Env
go run github.com/goreleaser/goreleaser/v2@<pin> release --snapshot --clean
go build -o /tmp/pk ./cmd/papierkram && /tmp/pk --help && /tmp/pk commands --json | jq '.commands|length'
go test ./internal/cli -run TestScript       # Mock-Szenario
gh run list --limit 1                         # CI-Matrix grün, inkl. keychain
```
Danach `superpowers:requesting-code-review` über den Gesamtstand, Findings beheben, push. Abschlussbericht: erledigt mit Belegen, offen, [Builder]-Entscheidungen, nächste Schritte mit Pascals Auftrag (Tap/Bucket-Repos, `PACKAGES_GITHUB_TOKEN`, erster Tag, E2E-Account).
