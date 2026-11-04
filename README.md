# papierkram-cli

Kommandozeilen-Client für die API von [Papierkram](https://www.papierkram.de) (Buchhaltung, Rechnungen, Angebote, Belege, Projekte, Zeiterfassung), gebaut für Menschen und KI-Agents gleichermaßen. Vorbilder: `hcloud`, `gh`, `sentry-cli`, `jira-cli`.

- Jede der 89 Operationen der offiziellen API ist ein eigener Befehl, mit deutschen Aliasen (`rechnung`, `beleg`, `zeit`, …).
- Mehrere Papierkram-Accounts parallel über Kontexte, das Token liegt im Schlüsselbund des Betriebssystems.
- Bestätigung vor folgenreichen Aktionen, `--dry-run` für jede Änderung, harter Schreibschutz für Agents.
- Stabile JSON-Ausgabe, dokumentierte Exit-Codes und strukturierte Fehler; für Agents siehe [AGENTS.md](AGENTS.md).

Das Binary heißt `papierkram`. Alle Texte (Hilfe, Fehler, Hinweise) sind deutsch; Befehle, Flags, JSON-Schlüssel und Fehlercodes bleiben englisch.

## Inhalt

- [Installation](#installation)
- [Schnellstart](#schnellstart)
- [Kontexte](#kontexte)
- [Projektdatei](#projektdatei)
- [Zeiterfassung](#zeiterfassung)
- [Sicherheit](#sicherheit)
- [Ausgabeformate](#ausgabeformate)
- [Credits](#credits)
- [Umgebungsvariablen](#umgebungsvariablen)
- [Shell-Vervollständigung](#shell-vervollständigung)
- [Exit-Codes](#exit-codes)
- [Eigenheiten der API](#eigenheiten-der-api)
- [Weitere Dokumentation](#weitere-dokumentation)
- [Dank](#dank)

## Installation

> Die Paketquellen (Homebrew, Scoop, deb/rpm) stehen **ab dem ersten Release** bereit. Bis dahin geht nur die Installation aus dem Quellcode.

**Homebrew** (macOS, Linux), ab erstem Release:

```sh
brew install Padrio/tap/papierkram
```

**Scoop** (Windows), ab erstem Release:

```powershell
scoop bucket add padrio https://github.com/Padrio/scoop-bucket
scoop install papierkram
```

**deb/rpm** (Linux), ab erstem Release: das `.deb`- bzw. `.rpm`-Paket für die eigene Architektur von der [Release-Seite](https://github.com/Padrio/papierkram-cli/releases) laden und mit dem Paketmanager installieren (z. B. `sudo apt install ./<paket>.deb` oder `sudo dnf install ./<paket>.rpm`).

**Aus dem Quellcode** (Go 1.26 oder neuer):

```sh
go install github.com/Padrio/papierkram-cli/cmd/papierkram@latest
```

So gebaut meldet `papierkram version` die Version `dev`, und es gibt keinen Alias `pk`.

### Kurzform `pk`

Homebrew, Scoop, deb und rpm installieren zusätzlich die Kurzform `pk` (ab erstem Release). `pk invoice list` ist dasselbe wie `papierkram invoice list`. Hinweis: [pkram](https://github.com/mickeyl/pkram) legt ebenfalls einen Befehl `pk` an. Wer beides installiert, bekommt das Programm, das zuerst im `PATH` steht; `papierkram` ist immer eindeutig.

Die Shell-Vervollständigung gilt auch für `pk`: Homebrew, deb und rpm richten sie für beide Namen ein, und das Skript von `papierkram completion <shell>` registriert `pk` mit (Details in [docs/release.md](docs/release.md#kurzform-pk)).

## Schnellstart

1. In Papierkram unter **Einstellungen → API** ein API-Token anlegen.
2. Anmelden. Der Befehl legt den Kontext an, fragt das Token verdeckt ab, prüft es mit `GET /info` (1 Credit) und speichert es im Schlüsselbund:

```sh
papierkram auth login --context firma-gmbh --subdomain firma-gmbh
papierkram auth status
```

Die Subdomain ist der Teil vor `.papierkram.de`. Ohne Terminal (Skripte) kommt das Token über die Standardeingabe, nie als Flag-Wert:

```sh
papierkram auth login --context firma-gmbh --subdomain firma-gmbh --token-stdin < token.txt
```

3. Erste Abfragen:

```sh
papierkram invoice list --limit 10
papierkram invoice list --from 2026-10-01 --to 2026-10-31 -o wide
papierkram invoice describe 459
papierkram company list --type customer
papierkram rechnung describe 459 -o json
```

4. Hilfe zu jedem Befehl, mit Beispielen und Credit-Kosten:

```sh
papierkram --help
papierkram invoice create --help
papierkram commands
```

## Kontexte

Ein Kontext ist ein Account-Setup: Subdomain, Optionen und ein Token im Schlüsselbund. Kontexte funktionieren wie bei `hcloud context`:

```sh
papierkram context create firma-gmbh --subdomain firma-gmbh
papierkram context create agent --subdomain firma-gmbh --read-only
papierkram context list
papierkram context use firma-gmbh
papierkram context active
papierkram context rename agent analyse
papierkram context delete analyse
papierkram invoice list --context analyse
```

`context delete` löscht auch das Token des Kontexts im Schlüsselbund; `auth logout` entfernt nur das Token, der Kontext bleibt. Läuft für einen Kontext ein Timer, verweigern `context rename` und `context delete` mit Exit 7, bis er mit `time stop` gebucht oder mit `time discard` verworfen ist.

Welcher Kontext gilt, entscheidet diese Reihenfolge: `--context` > `PAPIERKRAM_CONTEXT` > `context` aus der [Projektdatei](#projektdatei) > `active_context` der Konfiguration. `papierkram auth status` zeigt den wirksamen Kontext mit Herkunft, die Token-Quelle (nie das Token selbst), die Schreibschutzstufe und das Restkontingent.

Die Konfiguration liegt in `config.toml` im Konfigurationsverzeichnis des Betriebssystems (`papierkram/config.toml` unter `~/Library/Application Support` auf macOS, `~/.config` bzw. `$XDG_CONFIG_HOME` auf Linux, `%AppData%` auf Windows) oder unter `PAPIERKRAM_CONFIG_DIR`:

```toml
active_context = "firma-gmbh"

[contexts.firma-gmbh]
subdomain = "firma-gmbh"
read_only = false
output = "table"            # Standardformat: table, wide, json oder yaml
timeout = "30s"             # Zeitlimit je Anfrage
user = 1                    # Standard-Benutzer für Zeiteinträge
completion_lookup = true    # IDs in der Vervollständigung per API laden

[contexts.agent]
subdomain = "firma-gmbh"
read_only = true
```

Unbekannte Schlüssel in `config.toml` sind ein Fehler (Exit 2), damit sich z. B. ein falsch geschriebenes `read_only` nicht still verliert.

> **Kommentare gehen verloren:** `context create|use|delete|rename`, `auth login` und `auth logout` schreiben `config.toml` neu, sobald sie etwas daran ändern, `local init` die Projektdatei `.papierkram.toml`. Kommentare und eigene Formatierung gehen dabei verloren. `local init` warnt, wenn die Projektdatei Kommentare hatte.

Ohne Kontext geht es im **Direktbetrieb** mit `PAPIERKRAM_TOKEN` und `PAPIERKRAM_SUBDOMAIN` (beide haben Vorrang vor den Werten eines Kontexts; `auth status` nennt die Quelle). Das ist für CI und kurzlebige Umgebungen gedacht; am Terminal ist der Schlüsselbund sicherer.

## Projektdatei

Eine Datei `.papierkram.toml` legt für ein Verzeichnis Standardwerte fest, vor allem für die Zeiterfassung:

```toml
context = "firma-gmbh"   # optional: Kontext für dieses Verzeichnis
company = 3214
project = 4963
task    = 545
user    = 1

[tasks]
tickets = 57
wartung = 58
```

```sh
papierkram local init --project 4963 --task 545
papierkram local init --context firma-gmbh --company 3214 --user 1
papierkram local show
```

- **Suche:** im aktuellen Verzeichnis; liegt es in einem Git-Repository, aufwärts bis einschließlich zur Git-Wurzel, nie darüber hinaus. Außerhalb von Git nur im aktuellen Verzeichnis.
- **Vorrang:** Flag > Umgebungsvariable (`PAPIERKRAM_PROJECT`, `PAPIERKRAM_TASK`, `PAPIERKRAM_USER`) > Projektdatei > Kontext (nur `user`).
- **Genutzt von:** `time log` und `time start` (Aufgabe, Benutzer; `--task` nimmt auch einen Alias aus `[tasks]`), `time list`, `task list`, `task create` und `project describe` ohne ID (Projekt). `company` ist **keine** Voreinstellung: kein Befehl nutzt sie, `local show` zeigt sie nur an (ob sie künftig `person … --company` vorbelegt, ist offen, siehe [offene Punkte](docs/offene-punkte.md)).
- **Kontextbindung:** Nennt die Datei einen `context`, gelten ihre IDs nur, wenn genau dieser Kontext wirksam ist. Wählt `--context` oder `PAPIERKRAM_CONTEXT` einen anderen, ignoriert das CLI die Werte der Datei mit einer Warnung, damit keine ID im falschen Account landet.
- `papierkram local show` zeigt die wirksamen Werte mit Herkunft (`flag`, `env`, `projectfile`, `config`, `context`, `default`). Die Zeile `company` (aus `PAPIERKRAM_COMPANY` oder der Projektdatei) ist nur Anzeige und wird von keinem Befehl als Standardwert verwendet. `local init` fragt im Terminal ohne Flags jeden Wert ab.

**Sicherheitshinweise:** Eine Projektdatei kann aus einem fremden, geklonten Repository stammen. Das CLI behandelt sie deshalb als nicht vertrauenswürdig:

- Sie enthält nur IDs, Aliase und einen Kontextnamen, nie ein Token. Ein `context` darin kann nur einen Kontext wählen, den es in der eigenen `config.toml` schon gibt; anlegen oder ein Token setzen kann sie nicht. `PAPIERKRAM_READ_ONLY` und `read_only` des gewählten Kontexts gelten weiter.
- Gelesen werden höchstens **64 KiB**, und nur eine reguläre Datei: Geräte, FIFOs und Verzeichnisse werden ohne Öffnen abgelehnt.
- Ein **symbolischer Link** auf eine reguläre Datei wird gelesen (z. B. eine gemeinsame Datei im Monorepo). `local init` schreibt dagegen nie durch einen Link, sondern bricht ab.
- Werte aus der Datei erscheinen in Meldungen ohne Steuerzeichen; unbekannte Schlüssel erzeugen nur eine Warnung.
- Nach dem Klonen eines fremden Repositorys zeigt `papierkram local show`, was die Datei bewirkt.

## Zeiterfassung

`time log` legt einen Zeiteintrag an. Aufgabe und Benutzer kommen aus Flag, Umgebung, Projektdatei oder (Benutzer) Kontext:

```sh
papierkram time log --task 545 --from 9:00 --to 10:30 -m "Abstimmung mit Kunde"
papierkram time log --task tickets --duration 1h30m
papierkram time log --date gestern --from 13:00 --duration 45m --unbillable
```

- Zeitraum: `--from` und `--to` (`H:MM` oder `HH:MM`) oder `--duration` (`1h30m`, `90m`, `1:30`); ohne `--from` endet der Eintrag jetzt.
- Ein Zeiteintrag hat nur ein Datum: Einträge über Mitternacht lehnt das CLI ab (Exit 2); sie werden als zwei Einträge erfasst.
- `heute` und `gestern` folgen einer Regel: Zeiterfassung (`time …`): Tag der Rechneruhr; Belege, Zahlungen, Datumsfilter: Berliner Tag (Europe/Berlin, auch auf einem Rechner in UTC). Zur Zeiterfassung gehören `time create`/`time update --entry-date`, `time log --date`, `time stop` und `time list --since`/`--until`, wie die Uhrzeiten der Einträge; `time list --since heute` findet so den Eintrag, den `time log` oder `time stop` gerade gebucht hat. `--since`/`--until` beginnen bzw. enden um 00:00:00 bzw. 23:59:59 Uhr Ortszeit des Rechners.
- Im Terminal zeigt `time log` vorher eine Zusammenfassung mit Firma, Projekt, Aufgabe und Benutzer und fragt nach (Enter = ja). Jeder Name kostet beim ersten Mal 1 Credit und bleibt 60 Sekunden im Cache. `--yes`, `--no-input` und Aufrufe ohne Terminal senden ohne Rückfrage.

Der **Timer** läuft lokal (eine Datei je Kontext im Cache-Verzeichnis); Papierkram kennt ihn nicht:

```sh
papierkram time start --task tickets -m "Ticket 4711"
papierkram time status
papierkram time stop
papierkram time stop -m "Fehler behoben"
papierkram time discard
```

`time stop` bucht die Zeit vom Start bis jetzt wie `time log` und löscht den Timer erst, wenn der Zeiteintrag angelegt ist. Läuft schon ein Timer, bricht `time start` mit Exit 7 ab.

```sh
papierkram time list --since 2026-10-01 --until 2026-10-31
papierkram time list --project 4963 --state unbilled -o wide
```

## Sicherheit

### Schreibschutz

Zwei Stufen:

- **Harter Schreibschutz per Umgebung:** Mit `PAPIERKRAM_READ_ONLY=1` blockiert das CLI jede Änderung (Exit 9), gleich welcher Kontext gewählt ist, auch `papierkram api` mit anderen Methoden als GET sowie das Ändern von Konfiguration und Projektdatei (`context create|use|delete|rename`, `auth login|logout`, `local init`) und den Timer (`time start|stop`). Weder Flag noch Konfiguration noch Projektdatei können das aufheben. Gedacht für den Start eines Agents durch einen Menschen. Aus ist der Schutz nur bei leerem Wert, `0`, `false`, `no` oder `off`; jeder andere Wert schaltet ihn ein (unbekannte Werte mit Warnung).
- **Kontext-Option `read_only = true`** (oder `context create … --read-only`): schützt gegen Versehen im gewählten Kontext. Ein anderer Kontext per `--context` bleibt möglich.

```sh
PAPIERKRAM_READ_ONLY=1 papierkram invoice list
papierkram auth status
```

`auth status` und `--debug` zeigen, welche Stufe aktiv ist.

### Token und Schlüsselbund

- Das Token liegt im Schlüsselbund des Betriebssystems (macOS-Schlüsselbund, Windows-Anmeldeinformationsverwaltung, Secret Service unter Linux), Dienst `papierkram-cli`, Konto = Kontextname.
- `auth login` liest das Token verdeckt vom Terminal oder mit `--token-stdin` von der Standardeingabe, nie als Flag-Wert.
- Ohne Schlüsselbund (z. B. Linux ohne Secret Service) speichert nur `--insecure-storage` das Token, dann im Klartext in `config.toml` (Rechte 0600), mit Warnung:

```sh
papierkram auth login --context firma-gmbh --subdomain firma-gmbh --insecure-storage
```

- `PAPIERKRAM_TOKEN` hat Vorrang vor Schlüsselbund und `config.toml`.

### Bestätigung und `--dry-run`

- Folgenreiche Aktionen fragen im Terminal nach: `delete`, `cancel`, `reverse`, `pay`, `document delete` und `deliver` (mit Zusammenfassung: Beleg, Name, Betrag, bei `deliver` Versandart, Empfänger und Betreff). Bei `delete` wird die ID eingetippt, sonst `ja`. `time discard` fragt ebenfalls (Enter = nein), weil die erfasste Zeit verloren geht.
- Ohne Terminal oder mit `--no-input` bricht so eine Aktion ohne `--yes` ab (Exit 8). `--yes` bestätigt vorab.
- `archive` und `unarchive` sind umkehrbar und fragen nie nach.
- `--dry-run` gilt für jede Änderung: Das CLI prüft den Body lokal gegen das Schema, zeigt Methode, URL, Body (bei Uploads Dateiname, Größe und MIME-Typ) und die Credits und sendet nichts (Exit 0). Unter Schreibschutz gilt auch für `--dry-run` Exit 9.

```sh
papierkram invoice create --name "Beratung Oktober" --customer 3214 --payment-term 1130 --item "name=Beratung,qty=2,unit=Stunde,price=100,vat=19" --dry-run
papierkram voucher pay 295 --amount 150 --reason skonto --date heute --dry-run
papierkram invoice deliver 459 --via pdf --yes
```

`deliver --via email` verschickt nur mit ausdrücklich angegebenem Empfänger, Betreff und Text (`--to`, `--subject`, `--body` oder `--body-file`); fehlt eines, sendet das CLI nichts (Exit 2).

### Schwärzung

Tokens erscheinen nie in Ausgaben, Fehlermeldungen oder der `--debug`-Ausgabe; `--debug` zeigt den `Authorization`-Header geschwärzt. Zugangsdaten in `PAPIERKRAM_BASE_URL` (`https://nutzer:passwort@…`) werden für Anfragen genutzt, aber nie angezeigt oder gespeichert. Texte aus der API, aus `--input` und aus Projektdateien erscheinen in Bestätigungen und Meldungen ohne Steuerzeichen, damit nichts die Zusammenfassung überschreibt. `--jq` sieht keine Umgebungsvariablen (`$ENV` ist leer).

## Ausgabeformate

| Format | Inhalt |
|---|---|
| `-o table` | Standard: lesbare Tabelle mit den wichtigsten Spalten |
| `-o wide` | Tabelle mit zusätzlichen Spalten |
| `-o json` | die Antwort der API; Vertrag für Skripte und Agents, siehe [AGENTS.md](AGENTS.md) |
| `-o yaml` | wie JSON, als YAML |

- `--fields id,name,billing.company` wählt Felder (bei Listen je Eintrag); in Tabellen werden die Felder zu Spalten.
- `--jq '<ausdruck>'` wendet einen jq-Ausdruck (gojq) auf die JSON-Ausgabe an; Zeichenketten kommen ohne Anführungszeichen, ein Ergebnis je Zeile.
- Das Standardformat eines Kontexts setzt `output` in `config.toml`.
- Nutzdaten gehen nach stdout, Hinweise, Warnungen und Fehler nach stderr. Ohne Terminal gibt es keine Farben und keine Rückfragen; `--no-color` und `NO_COLOR` schalten Farben ab.
- PDFs: `-O datei.pdf` speichert, `-O -` schreibt nach stdout. Ohne `-O` geht das PDF nur in eine Umleitung oder Pipe, nie ins Terminal.

```sh
papierkram invoice list -o json --jq '.entries[] | select(.state == "unpaid") | .invoice_no'
papierkram company list --fields id,name,email
papierkram invoice pdf 459 -O rechnung.pdf
papierkram schema invoice create
```

## Credits

Papierkram rechnet in Monats-Credits (Paket M 10 000, L 20 000); danach antwortet die API mit 429 (Exit 6). Die Kosten je Befehl stehen in der Hilfe (`Kosten: …`) und in `papierkram commands --json`:

| Credits | Anfrage |
|---|---|
| 50 | Rechnung, Angebot oder Beleg anlegen |
| 10 | anderes Anlegen (Firma, Kontaktperson, Artikel, Projekt, Datenfeld, Aufgabe, Zeiteintrag) und Beleg-Upload |
| 1 | alles andere: Liste je Seite, Abruf, Änderung, Archivieren, Löschen, Aktion, PDF, `GET /info` |
| 0 | lokale Befehle (`context`, `local`, `commands`, `schema`, `completion`, `version`, `time start|status|discard`) |

Zusammengesetzte Befehle kosten die Summe: `deliver` 2 (Versand und Nachlesen), `voucher create --file` 60, `invoice create --from-estimate` 51, `invoice update` 2 (liest die Rechnung, um ihre Zahlungsbedingung mitzusenden; 1 mit `--payment-term`), `auth login` und `auth status` je 1. `--all` kostet 1 Credit je geladener Seite; ab 10 Seiten warnt das CLI und fragt im Terminal nach, `--max-pages` begrenzt. `company list --type` lädt immer alle Seiten (die API filtert nicht). Die Vervollständigung von IDs kostet je Abfrage 1 Credit. Fällt das Restkontingent unter 100, warnt das CLI auf stderr.

## Umgebungsvariablen

| Variable | Wirkung |
|---|---|
| `PAPIERKRAM_CONTEXT` | Kontext für diesen Aufruf (nach `--context`, vor Projektdatei und `active_context`) |
| `PAPIERKRAM_TOKEN` | Token; Vorrang vor Schlüsselbund und `config.toml`. Mit `PAPIERKRAM_SUBDOMAIN` oder `PAPIERKRAM_BASE_URL` Direktbetrieb ohne Kontext |
| `PAPIERKRAM_SUBDOMAIN` | Subdomain; Vorrang vor der Subdomain des Kontexts |
| `PAPIERKRAM_BASE_URL` | ersetzt die komplette Basis-URL (Tests, Mock-Server); Zugangsdaten darin (`nutzer:passwort@`) werden nie angezeigt. Ohne `https` warnt das CLI auf stderr (das Token ginge unverschlüsselt), außer für `localhost`, `127.0.0.0/8` und `::1` |
| `PAPIERKRAM_READ_ONLY` | harter Schreibschutz (Exit 9); aus nur bei leer, `0`, `false`, `no`, `off` |
| `PAPIERKRAM_CONFIG_DIR` | Basisverzeichnis der Konfiguration statt des Betriebssystem-Standards; die Datei liegt dann in `<verzeichnis>/papierkram/config.toml` |
| `PAPIERKRAM_CACHE_DIR` | Basisverzeichnis des Caches (Namen, Vervollständigung, Timer) statt des Betriebssystem-Standards; die Dateien liegen in `<verzeichnis>/papierkram/` |
| `PAPIERKRAM_PROJECT` | Projekt-ID für `time list`, `task list`, `task create` und `project describe` ohne ID (vor der Projektdatei) |
| `PAPIERKRAM_TASK` | Aufgabe (ID oder Alias aus `[tasks]`) für `time log` und `time start` |
| `PAPIERKRAM_USER` | Benutzer-ID für `time log` und `time start` |
| `PAPIERKRAM_COMPANY` | Firmen-ID, **keine** Voreinstellung: wird nur von `local show` angezeigt, kein Befehl verwendet sie als Standardwert |
| `NO_COLOR` | nicht leer: keine Farben |

Die Variablen `PAPIERKRAM_E2E_*` nutzt nur die Live-E2E-Suite, siehe [e2e/README.md](e2e/README.md).

## Shell-Vervollständigung

`papierkram completion bash|zsh|fish|powershell` gibt das Skript aus. Vervollständigt werden Befehle, Flags, erlaubte Werte, Kontextnamen und IDs.

```sh
source <(papierkram completion bash)
papierkram completion zsh > "${fpath[1]}/_papierkram"
papierkram completion fish > ~/.config/fish/completions/papierkram.fish
```

PowerShell: `papierkram completion powershell | Out-String | Invoke-Expression` (für dauerhaft in `$PROFILE` eintragen). Die Skripte vervollständigen auch die Kurzform `pk`; fish lädt sie nach dem Befehlsnamen und braucht dafür zusätzlich `ln -s papierkram.fish ~/.config/fish/completions/pk.fish`. `papierkram completion --help` beschreibt die Einrichtung je Shell.

**IDs kosten Credits:** Für IDs (`papierkram invoice describe <Tab>`, `--company <Tab>`) lädt die Vervollständigung die erste Seite der Liste aus der API (1 Credit je Abfrage) und hält das Ergebnis je Kontext und Ressource **60 Sekunden** im Cache. Abschalten je Kontext in `config.toml`; dann sendet die Vervollständigung keine Anfrage und liest keinen Cache:

```toml
[contexts.firma-gmbh]
completion_lookup = false
```

## Exit-Codes

| Exit | Code | Bedeutung |
|---|---|---|
| 0 | – | Erfolg, auch `--dry-run` |
| 1 | `error`, `server_error` | unerwarteter Fehler oder Serverfehler (5xx) |
| 2 | `usage` | falsche Flags oder Argumente, fehlende Pflichtangabe; nichts gesendet |
| 3 | `auth` | kein Token, 401 oder 403 |
| 4 | `not_found` | 404: ID prüfen, gehört sie zu diesem Kontext? |
| 5 | `validation` | lokale Prüfung, 400 oder 422 |
| 6 | `quota` | 429: Monatskontingent erschöpft |
| 7 | `conflict` | 409 oder Timer läuft schon |
| 8 | `confirmation_required` | Bestätigung fehlt oder abgelehnt; `--yes` |
| 9 | `read_only` | Schreibschutz (`PAPIERKRAM_READ_ONLY` oder `read_only` im Kontext) |
| 10 | `network` | Zeitüberschreitung, DNS, TLS bei einem Lesezugriff oder bevor eine Änderung gesendet wurde; Abbruch einer Rückfrage mit Strg+C |
| 11 | `partial` | mehrstufiger Befehl teilweise ausgeführt; Ausgabe nennt, was fehlt, und den Befehl zum Nachholen |
| 12 | `outcome_unknown` | Verbindung brach ab, **nachdem** eine Änderung gesendet wurde, oder die API beantwortete die Änderung mit einer Weiterleitung (3xx), der das CLI nicht folgt: vor einer Wiederholung mit `describe`/`list` prüfen |

Fehler stehen auf stderr als `Fehler: …` mit `Hinweis: …` und ggf. `Correlation-ID: …`; mit `-o json` oder `--jq` folgt als letzte Zeile eine kompakte JSON-Zeile, mit `-o yaml` ein YAML-Block (`error:` …) am Ende (Schema: [docs/schemas/error.json](docs/schemas/error.json); Auswertung siehe [AGENTS.md](AGENTS.md#fehler)). Lesende Anfragen (GET) wiederholt das CLI bei Netzwerk- und Serverfehlern bis zu zweimal, Änderungen und 429 nie.

## Eigenheiten der API

Live gemessen gegen einen Sandbox-Account (Stand 2026-11-04, Einzelheiten in [docs/offene-punkte.md](docs/offene-punkte.md)):

- **Angebote brauchen ein Datum:** `estimate create` ohne `--document-date` lehnt die API ab und berechnet trotzdem 50 Credits; das CLI prüft das deshalb vorher (Exit 5).
- **Abgelehnte Anfragen kosten:** Auch ein mit 422 abgelehntes Anlegen kostet die vollen Credits.
- **Rechnung ändern:** Ein `PUT` ohne Zahlungsbedingung stellt eine Rechnung auf die Standard-Zahlungsbedingung des Accounts um und ändert das Fälligkeitsdatum. `invoice update` liest die Rechnung deshalb vorher und sendet ihre aktuelle Zahlungsbedingung mit (1 Credit mehr; nicht mit `--payment-term` oder `payment_term` in `--input`). `papierkram api PUT` tut das nicht.
- **`--billable-duration` ist wirkungslos:** Gemessen wurde 1800 Sekunden bei einem Eintrag von 90 Minuten; gespeichert war beim Anlegen und beim Ändern die Dauer (5400).
- **PDF-Versand:** `deliver --via pdf` vergibt die Nummer und setzt den Status (z. B. `sent`), aber weder `sent_on` noch `sent_via`.
- **Upload:** PDF, PNG und JPG werden angenommen; TXT lehnt die API mit HTTP 500 ab (Exit 1), andere Typen sind nicht gemessen. Das ist kein vorübergehender Fehler, also nicht wiederholen.

## Weitere Dokumentation

- [AGENTS.md](AGENTS.md): JSON-Vertrag, Exit-Codes und Regeln für KI-Agents
- [docs/reference/](docs/reference/): Befehlsreferenz, aus der Hilfe generiert
- [skills/papierkram/SKILL.md](skills/papierkram/SKILL.md): Claude-Code-Skill mit den Abläufen Beleg hochladen und zuordnen, Rechnung aus Angebot, offene Posten prüfen, Zeiten erfassen und abrechnen. Zum Einbinden den Ordner `skills/papierkram` nach `~/.claude/skills/` (für alle Projekte) oder nach `.claude/skills/` eines Projekts kopieren.
- [docs/schemas/](docs/schemas/): JSON-Schemas der Ausgaben
- [docs/offene-punkte.md](docs/offene-punkte.md): offene Fragen zur API und wie die E2E-Suite sie misst
- [CHANGELOG.md](CHANGELOG.md), [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)
- [docs/release.md](docs/release.md): Release mit GoReleaser, Snapshot-Test, Kurzform `pk` je Paketquelle
- Design: [Spezifikation](docs/specs/2026-11-03-papierkram-cli-design.md), [Implementierungsplan](docs/plans/2026-11-03-papierkram-cli.md), [API-Snapshot](api/)

## Dank

Viele Fallen der Papierkram-API hat [papierkram-mcp-server](https://github.com/Wagner-Emden-IT-Services/papierkram-mcp-server) (MIT) live gefunden und dokumentiert. Dieses Projekt übernimmt von dort nur Wissen, keinen Code.

## Lizenz

MIT, siehe [LICENSE](LICENSE). Lizenzen der Abhängigkeiten: [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
