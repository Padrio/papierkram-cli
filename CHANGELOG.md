# Changelog

Alle nennenswerten Änderungen an papierkram-cli stehen in dieser Datei.

Das Format folgt [Keep a Changelog](https://keepachangelog.com/de/1.1.0/), die Versionen folgen [Semantic Versioning](https://semver.org/lang/de/).

## [Unreleased]

### Hinzugefügt

- Befehle für alle 89 Operationen der Papierkram-API: `company`, `person`, `invoice`, `estimate`, `voucher`, `proposition`, `payment-term`, `project`, `task`, `time`, `custom-attribute`, `bank-connection`, `transaction`, `user` und `info`, mit deutschen Aliasen (`firma`, `rechnung`, `beleg`, `zeit`, …; `info` hat keinen).
- Felder, Typen und erlaubte Werte kommen aus der eingebetteten, per Overlay korrigierten API-Spezifikation; jeder Body wird vor dem Senden gegen das Request-Schema geprüft (Exit 5 ohne Anfrage), unbekannte Felder mit Vorschlag gemeldet.
- Positionen per `--item key=wert,…` und `--items datei.json`; Steuersätze in jeder Schreibweise (`19`, `19%`, `0,19`, `0.19`); `update` ersetzt Positionen nur mit `--replace-items`.
- `--input datei.json|-` für jeden Body, mit Bereinigung nur lesbarer Felder, sodass `describe -o json | update --input -` funktioniert.
- Komfortbefehle: `invoice create --from-estimate`, `voucher create --file`, `voucher pay`, `invoice|estimate deliver --via email|pdf`, `… pdf -O datei.pdf`, `voucher upload`, `company list --type customer|supplier`.
- Zeiterfassung: `time log` mit `--from/--to` oder `--duration`, lokaler Timer (`time start|status|stop|discard`), Zusammenfassung mit aufgelösten Namen im Terminal.
- Kontexte für mehrere Accounts (`context create|use|list|active|delete|rename`), Token im Schlüsselbund des Betriebssystems (`auth login|logout|status`), Datei-Fallback nur mit `--insecure-storage`, Direktbetrieb über `PAPIERKRAM_TOKEN` und `PAPIERKRAM_SUBDOMAIN`.
- Projektdatei `.papierkram.toml` mit Standardwerten für Kontext, Firma, Projekt, Aufgabe (mit Aliasen) und Benutzer (`local init|show`), sicher gelesen (höchstens 64 KiB, nur reguläre Dateien) und an ihren Kontext gebunden.
- Schutz: Aktionsklassen mit Bestätigung für `delete`, `cancel`, `reverse`, `pay`, `document delete` und `deliver`; `--dry-run` für jede Änderung; harter Schreibschutz per `PAPIERKRAM_READ_ONLY` und Kontext-Option `read_only`.
- Ausgabe als `table`, `wide`, `json` und `yaml`, mit `--fields` und `--jq`; strukturierte Fehler auf stderr und die Exit-Codes 0–12, darunter `partial` (11) und `outcome_unknown` (12).
- Maschinenlesbare Selbstbeschreibung: `commands --json` (Klasse, Credits, Flags, Beispiele) und `schema <ressource> [aktion]`; JSON-Schemas der Ausgaben in `docs/schemas/`.
- `api <METHOD> <pfad>` als direkter Zugang zur API (wie `gh api`), mit denselben Schutzregeln wie die übrigen Befehle.
- Credit-Kosten in Hilfe, `commands --json` und `--dry-run`; Warnung bei `--all` ab 10 Seiten und bei weniger als 100 verbleibenden Credits.
- Shell-Vervollständigung für bash, zsh, fish und PowerShell, mit IDs aus der API (60 Sekunden Cache, abschaltbar per `completion_lookup = false`).
- Live-E2E-Suite gegen einen Sandbox-Account (`-tags e2e`), die die offenen Punkte der API misst; ohne Freigabe sendet sie keine Anfrage.
- Live-E2E-Suite: Abdeckungsbericht je Lauf (`ABDECKUNG …` im Log) über alle 89 Operationen; ein vollständiger Lauf scheitert, wenn eine erwartete Operation fehlt.
- Live-E2E-Suite: Credit-Budget je Lauf 760 (davon 62 fürs Aufräumen, Zielwert unter 800). Der Workflow „E2E (Sandbox)“ schwärzt die Sandbox-Subdomain im Log, prüft es auf Token und Subdomain, schreibt Abdeckung, Abweichungen und Credits in die Zusammenfassung des Laufs und lädt das Log nur nach bestandener Prüfung als Artefakt `e2e-log` hoch (14 Tage).
- Live-E2E-Suite: `TestE2E_InvoiceFlow` prüft den Lebenszyklus einer eigenen Rechnung (`deliver --via pdf`, `pdf`, `archive`, `unarchive`) und misst ihre Statuswerte sowie `cancel` von Rechnung und versendetem Angebot (60 Credits je Lauf).
- Live-E2E-Suite: `TestE2E_VoucherFlow` prüft den Lebenszyklus eines eigenen Belegs (`create --file`, `pdf`, `document delete`, `archive`, `unarchive`) und misst `reverse` und `cancel` mit den Statuswerten danach (69 Credits je Lauf). Legt ein Storno oder eine Gegenbuchung ein Folgeobjekt mit eigener ID an, meldet die Suite das als Abweichung und räumt es mit auf.
- Live-E2E-Suite: `TestE2E_TimeFlow` prüft die Zeiterfassung (`time log`, `time list --since heute`, `archive`, `unarchive`) und den lokalen Timer (`time start`, `status`, `stop`, `discard`) mit eingespeister Uhr: der gestoppte Eintrag läuft von 11:00 bis 11:02 des Tages, `duration` 120 (24 Credits je Lauf).
- Dokumentation: README, AGENTS.md für KI-Agents, Claude-Code-Skill (`skills/papierkram`), aus der Hilfe generierte Befehlsreferenz (`docs/reference`), offene Punkte der API, Hinweise zu Drittsoftware.

[Unreleased]: https://github.com/Padrio/papierkram-cli/commits/main
