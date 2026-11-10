# papierkram-cli v0.1.1 – Design

Stand 2026-11-09. Ergänzt die Haupt-Spec [`2026-11-03-papierkram-cli-design.md`](2026-11-03-papierkram-cli-design.md); wo dieses Dokument abweicht, gilt es und die Haupt-Spec wird im selben Zug angepasst. Nachtrag nach der Umsetzung: Maßgeblich für das Umgesetzte ist die Haupt-Spec (§6.1, §7.2, §8.4, §9.9, §11.1, §12.2) samt Rulings; dieses Dokument ist der Entwurf vom 2026-11-09. Auftrag Pascal (2026-11-09): alles in einem Zug umsetzen, ohne Rückfragen (Entscheidungen nach Empfehlung, hier mit **[Call]** markiert), subagent-getrieben, am Ende Release **v0.1.1** (Patch-Bump; Pascal hat am 2026-11-09 von Minor auf Patch korrigiert).

## Ziele

1. **Subdomain großzügig annehmen** – `--subdomain pkrason.papierkram.de`, `https://pkrason.papierkram.de/` und `pkrason` ergeben denselben Kontext.
2. **E2E-Aufräumen verbessern** – die Reste im Sandbox-Account wachsen nicht mehr unnötig je Lauf; Folgeobjekte (Storno) werden erkannt.
3. **`papierkram mcp serve`** – MCP-Server über stdio, Tools aus der Command-Registry.
4. **Namensauflösung in Flags** – `--customer "Mustermann GmbH"` statt ID.
5. **`invoice paid-status`** – Bericht über offene Posten.
6. **winget-Manifest** und **SBOM** im Release.
7. Release v0.1.1 nach grüner CI und Live-Lauf gegen die Sandbox.

## Nicht-Ziele

- Scoop bleibt ausgesetzt. Kein `$EDITOR`-Bearbeiten (eigener Punkt, nicht beauftragt).
- Keine neue API-Operation, kein Schreibzugriff, den das CLI nicht schon hat.
- MCP: kein HTTP/SSE-Transport, keine Prompts/Resources, nur Tools über stdio.

## Anti-Ziele (wäre ein Fehlschlag, auch wenn es läuft)

- Ein Agent kann über MCP mehr als über das CLI: Schreibschutz, Bestätigungen, Credits-Regeln und Exit-Semantik (AGENTS.md) dürfen nicht umgangen werden.
- Namensauflösung, die still den falschen Kunden wählt oder ungefragt viele Credits verbraucht.
- MCP-Pfad und CLI-Pfad, die für dieselbe Operation unterschiedlich prüfen oder senden (Cross-Pfad-Bug).
- Ein Release, das am winget-Schritt scheitert und halb veröffentlicht.

## 1. Subdomain

- Gilt für jede Stelle, die eine Subdomain annimmt (`auth login --subdomain`, `context create --subdomain`, Konfigurationswert beim Laden nicht – dort bleibt die strikte Prüfung, damit `config.toml` eindeutig ist). **[Call]**
- Normalisierung: Schema `http://`/`https://` entfernen, Pfad/Slash am Ende entfernen, Port ablehnen, Suffix `.papierkram.de` (Groß-/Kleinschreibung egal) entfernen; Ergebnis klein schreiben und mit der bestehenden Prüfung validieren. Andere Domains (z. B. `firma.example.com`) bleiben Exit 2 mit dem bisherigen Hinweis.
- Wurde normalisiert, Hinweis auf stderr: `Hinweis: Subdomain „pkrason“ aus „https://pkrason.papierkram.de“`.

## 2. E2E-Aufräumen

Befund: Je Lauf bleiben eine versendete und stornierte Rechnung samt Storno-Beleg, ein versendetes und storniertes Angebot und die Kundenfirma (hat Belege, nicht löschbar) archiviert zurück. Rechnungen/Angebote mit Nummer kann die API nicht löschen (GoBD); das ist unvermeidbar, das Wachstum der übrigen Reste nicht.

- **Dauerhafte Kundenfirma** **[Call]**: Die Flows, die nummerierte Belege erzeugen (Versand, Storno), nutzen eine feste Firma `pk-e2e-dauerkunde` (per Name gesucht, bei Bedarf entarchiviert und am Ende wieder archiviert, sonst einmal angelegt). Andere Flows legen ihre Firmen weiter je Lauf an und löschen sie.
- **Folgeobjekte erkennen**: `invoice cancel` erzeugt eine Storno-Rechnung. Die Suite findet sie nach dem Storno (Liste nach Name/Nummer, z. B. „(storniert)“ bzw. Referenz auf die Originalrechnung – der Implementer misst, was die API liefert) und registriert sie fürs Aufräumen (archivieren).
- **Restesuche**: bereits archivierte Reste früherer Läufe werden nicht jedes Mal erneut angefasst (keine Credits für Zustände, die sich nicht ändern lassen); das Log zählt sie getrennt („dauerhaft archiviert: N“).
- Budget im Header von `e2e/e2e_test.go` neu rechnen; `creditBudget` bleibt 760, solange die Rechnung darunter liegt.

## 3. `papierkram mcp serve`

- **SDK** **[Call]**: offizielles `github.com/modelcontextprotocol/go-sdk` (stdio-Transport). Neue Abhängigkeit → `THIRD_PARTY_NOTICES.md` (Lizenz lesen und eintragen).
- **Tools aus der Registry**: je Befehl der Klassen, die der Modus erlaubt, ein Tool. Name `<ressource>_<aktion>` (z. B. `invoice_list`, `voucher_document_delete`), Beschreibung aus `short` plus Klasse und Credits, Eingabe-Schema aus den Flags (Typ, `required`, `enum`, `minimum`/`maximum`, `default` wie `commands --json`) und Positionsargumenten (`id`). Lokale Befehle (`context`, `local`, `time start|stop|status|discard`, `completion`, `auth`) bekommen **keine** Tools **[Call]**; `papierkram api` auch nicht.
- **Ein Ausführungspfad** **[Call]**: Ein Tool-Aufruf wird in die Argumentliste des CLI übersetzt und in-process über denselben Einstieg wie die Kommandozeile ausgeführt (`cli.Run` mit eigener `App`, `-o json --no-input`). Damit gelten Prüfung, Schreibschutz, Credits, Exit-Codes, `--dry-run` und Fehlertexte unverändert. Ergebnis: stdout-JSON als Tool-Ergebnis; Exit ≠ 0 → Tool-Fehler (`isError`) mit dem strukturierten Fehler-JSON (`code`, `message`, `hint`, `exit_code`).
- **Sicherheit** **[Call]**:
  - Standard: nur `read_only`-Tools. `--allow-writes` schaltet `create`, `update`, `archive` frei. `--allow-destructive` zusätzlich `destructive` und `send` (sie laufen dann mit `--yes`; ohne die Option existieren diese Tools nicht). `PAPIERKRAM_READ_ONLY` bzw. `read_only` des Kontexts erzwingt den Lese-Modus, auch mit den Optionen (Hinweis auf stderr beim Start).
  - `deliver` nur mit `--via pdf` als Tool-Vorgabe; `--via email` ist im MCP-Modus nicht erlaubt (AGENTS-Regel). `voucher pay` nur mit `--allow-destructive`.
  - Jedes schreibende Tool hat einen Parameter `dry_run` (bool).
  - Tokens nie in Tool-Ergebnissen oder Logs; `--debug` im MCP-Modus nur auf stderr.
- **Kontext**: `--context` beim Start wählt den Kontext für alle Aufrufe; Tools haben keinen Kontext-Parameter.
- **Tests**: In-Memory-Transport des SDK gegen mockapi; Tool-Liste je Modus; ein Lese-, ein Schreib- und ein destruktiver Aufruf; Schreibschutz; Cross-Pfad-Test: gleiche Anfrage (Methode, Pfad, Body) über CLI und MCP.
- Doku: README-Abschnitt mit Client-Konfiguration (Claude Desktop/Claude Code: `{"command":"papierkram","args":["mcp","serve","--context","firma"]}`), AGENTS.md, SKILL.md.

## 4. Namensauflösung in Flags

Die Haupt-Spec (Abschnitt Nicht-Ziele) hatte Namensauflösung abgelehnt (**[Call]**); Pascal hat sie am 2026-11-09 beauftragt. Die Bedenken (Credits, Mehrdeutigkeit) werden so behandelt:

- Gilt für Flags, die eine ID einer Ressource mit Namen nehmen: `--customer`, `--creditor`, `--company`, `--project`, `--contact-person`, `--customer-project`, `--task`, `--proposition`, `--payment-term`, `--user` (Liste der tatsächlich vorhandenen ID-Flags ermittelt der Implementer aus der Registry; Zuordnung Flag → Ressource dort).
- Ist der Wert eine ganze Zahl, bleibt er eine ID (keine Anfrage). Sonst: Name.
- Auflösung **[Call]**: Liste der Ressource seitenweise laden (100 je Seite, höchstens 5 Seiten, wie `--max-pages`), Name exakt vergleichen (ohne Groß-/Kleinschreibung, Leerraum getrimmt; bei Personen „Vorname Nachname“). Archivierte Einträge zählen nicht. Genau ein Treffer → ID; Hinweis auf stderr `Hinweis: Kunde „Mustermann GmbH“ → 3214 (1 Credit)`. Kein Treffer → Exit 4 mit bis zu drei ähnlichen Namen. Mehrere → Exit 2 mit den Kandidaten (ID, Name), nichts gesendet.
- Cache: `internal/namecache` bzw. der Cache der Vervollständigung (gleicher Schlüssel: Kontext und Ressource) wird genutzt, wenn er frisch ist; die Auflösung schreibt ihn **[Call]**. Ein Treffer aus dem Cache kostet nichts und wird im Hinweis so genannt.
- `--dry-run`: die Auflösung findet statt (lesend, kostet Credits) und steht unter `resolved[]` in der Ausgabe (`flag`, `name`, `resource`, `id`, `credits`, `cached`; Ruling P-A: `read` bleibt das Einzelobjekt einer Leseanfrage wie bei `--from-estimate`); `credits` enthält die in diesem Lauf verbrauchten Credits der Auflösung, ein Treffer aus dem Cache zählt 0.
- `commands --json`: Flags mit Namensauflösung bekommen `resolves` (Ressourcenname) – additiv, `version` bleibt 1; Schema `docs/schemas/commands.json` erweitern.
- Gilt nicht für `papierkram api` und nicht für Positionsargumente `<id>`.
- MCP: Tools nehmen bei diesen Feldern ebenfalls Namen an (gleicher Pfad).

## 5. `invoice paid-status`

- Lesender Befehl, Klasse `read_only`. Lädt Rechnungen per Liste (`--all`, Standard `--max-pages 5`), Filter `--customer` (ID oder Name), `--from`/`--to` (Rechnungsdatum), `--overdue` (nur fällige mit `due_date` < heute, Berliner Tag).
- Offene Posten: Einträge mit `outstanding_amount` > 0 und `state` nicht `draft`/`canceled` (Implementer misst die Werte live; Ergebnis in `docs/offene-punkte.md`). Liefert die Liste `outstanding_amount` nicht, je Rechnung `describe` nur mit `--details` **[Call]** und Hinweis auf Credits.
- Ausgabe: Tabelle `invoice_no`, Kunde, `document_date`, `due_date`, Tage überfällig, `total_gross`, `outstanding_amount`; Summenzeile. JSON: `{"entries":[…],"total_outstanding":…,"count":…,"overdue_count":…,"as_of":"YYYY-MM-DD"}` mit Schema `docs/schemas/paid-status.json`. Beträge als Zahlen, Summe mit Dezimal-Arithmetik (keine Float-Rundungsfehler bei der Summe).
- Credits: 1 je Seite; `commands --json` nennt das.

## 6. winget und SBOM

- **SBOM** **[Call]**: GoReleaser `sboms` mit syft für jedes Archiv (`*.sbom.json`, SPDX JSON). syft im Release- und im Snapshot-Job per gepinnter Action (`anchore/sbom-action/download-syft`, SHA-Pin, `check-action-pins.sh`). CI-Snapshot prüft, dass je Archiv eine SBOM entsteht.
- **winget** **[Call]**: GoReleaser `winget` mit Paket-ID `Padrio.Papierkram`, Typ portable (zip), Publisher „Pascal Krason“, Lizenz MIT, Repository = Fork `Padrio/winget-pkgs` (wird per `gh repo fork microsoft/winget-pkgs` angelegt), Pull Request gegen `microsoft/winget-pkgs` aktiviert, `skip_upload: auto`. Der PAT `PACKAGES_GITHUB_TOKEN` braucht zusätzlich Zugriff auf den Fork – das ist ein Schritt für Pascal (Token bearbeiten, kein neuer Token). Damit ein fehlender Zugriff kein halbes Release erzeugt: Der Release-Workflow prüft vor GoReleaser mit dem PAT per API, ob der Fork beschreibbar ist (`permissions.push`); sonst Abbruch vor jeder Veröffentlichung mit klarer Meldung.
- `docs/release.md`, README (Installation Windows: `winget install Padrio.Papierkram` – sobald Microsoft den PR angenommen hat).
- **Ruling W-A (Controller, ersetzt oben PR und `permissions.push`):** GoReleaser öffnet keinen Pull Request (`pull_request.enabled: false`; ein fine-grained PAT kann in `microsoft/winget-pkgs` vermutlich keinen öffnen). Die Manifeste landen auf dem Branch `papierkram-<version>` im Fork, den Pull Request stellt ein Mensch nach dem Release per `gh pr create` (`docs/release.md`, Release-Schritte). Die Vorab-Prüfung ist ein echter Schreibtest: `scripts/check-fork-push.sh <owner/repo>` legt mit dem PAT einen Branch `pk-write-check-<Lauf>-<Versuch>` auf dem Standard-Branch an und löscht ihn wieder, für `Padrio/winget-pkgs` und `Padrio/homebrew-tap`; scheitert das Anlegen, bricht der Workflow vor GoReleaser ab, scheitert nur das Löschen, warnt er. Das Token geht nur über die Umgebung an `gh`; die CI testet das Skript ohne Netz gegen ein vorgetäuschtes `gh` (`CHECK_FORK_PUSH_GH`).

## 7. Querschnitt

- Spec-Abgleich: Haupt-Spec Abschnitte Nicht-Ziele (Namensauflösung, MCP „v1.x“), 5.3 (Registry → MCP), Release (SBOM, winget), Ausblick anpassen.
- Doku: README, AGENTS.md (MCP-Regeln, Namensauflösung kostet Credits, paid-status), `skills/papierkram/SKILL.md`, `docs/reference/` (`make docs`), CHANGELOG `[Unreleased]` → `[0.1.1]`.
- Gates wie bisher (Plan-Verifikation): `go test -race`, golangci-lint mit/ohne e2e, vet, gofmt, Coverage ≥ 80 (Ist 94,5 – nicht senken), `go generate`/`make docs` ohne Diff, E2E offline, Action-Pins, Go-Version, goreleaser check + snapshot.
- E2E live: paid-status, Namensauflösung (ein Flag), MCP-Lesetool gegen Sandbox (in-process), Aufräumen; ABDECKUNG bleibt 88/89.

## Dem Builder überlassen

Paketaufteilung (`internal/mcpserver`, `internal/resolve`), genaue Tool-Schema-Erzeugung, Formulierung der Hinweise (Deutsch, echte Umlaute, Stil wie bestehende Hinweise), Messung der Storno-Folgeobjekte und von `outstanding_amount` in der Liste.
