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
- [Ressourcen](#ressourcen)
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

> Pakete gibt es für Homebrew (macOS, Linux) und deb/rpm (Linux). Unter Windows das Archiv von der [Release-Seite](https://github.com/Padrio/papierkram-cli/releases) oder `go install` verwenden; ein Scoop-Paket ist vorerst ausgesetzt.

**Homebrew** (macOS, Linux):

```sh
brew install Padrio/tap/papierkram
```

**Windows:** das `.zip` für die eigene Architektur von der [Release-Seite](https://github.com/Padrio/papierkram-cli/releases) laden, entpacken und `papierkram.exe` in ein Verzeichnis im `PATH` legen.

**deb/rpm** (Linux): das `.deb`- bzw. `.rpm`-Paket für die eigene Architektur von der [Release-Seite](https://github.com/Padrio/papierkram-cli/releases) laden und mit dem Paketmanager installieren (z. B. `sudo apt install ./<paket>.deb` oder `sudo dnf install ./<paket>.rpm`).

**Aus dem Quellcode** (Go 1.26 oder neuer):

```sh
go install github.com/Padrio/papierkram-cli/cmd/papierkram@latest
```

So gebaut meldet `papierkram version` die Version `dev`, und es gibt keinen Alias `pk`.

### Kurzform `pk`

Homebrew, deb und rpm installieren zusätzlich die Kurzform `pk`. `pk invoice list` ist dasselbe wie `papierkram invoice list`. Hinweis: [pkram](https://github.com/mickeyl/pkram) legt ebenfalls einen Befehl `pk` an. Wer beides installiert, bekommt das Programm, das zuerst im `PATH` steht; `papierkram` ist immer eindeutig.

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

## Ressourcen

Jede Ressource ist ein Befehl mit Unterbefehlen (`list`, `describe`, `create`, `update`, `delete`, …); `papierkram <ressource> --help` zeigt alle mit Beispielen und Credits, `papierkram commands` die vollständige Liste.

| Befehl | Alias | Zweck |
|---|---|---|
| `company` | `firma`, `kontakt` | Firmen (Kunden und Lieferanten): anlegen, ändern, archivieren, löschen; `list --type customer\|supplier` |
| `person` | `kontaktperson` | Kontaktpersonen einer Firma (`--company`, auch aus `PAPIERKRAM_COMPANY` oder der Projektdatei) |
| `invoice` | `rechnung` | Rechnungen: anlegen (auch `--from-estimate`), ändern, `deliver`, `pdf`, `cancel`, archivieren, löschen; `paid-status` ([offene Posten](#offene-posten)) |
| `estimate` | `angebot` | Angebote: anlegen, ändern, `deliver`, `pdf`, `cancel`, archivieren, löschen |
| `voucher` | `beleg`, `expense` | Belege (Ausgaben): anlegen (auch `--file`), ändern, `upload`, `document delete`, `pay`, `reverse`, `cancel`, `pdf`, archivieren, löschen |
| `proposition` | `artikel`, `leistung` | Artikel und Leistungen für Positionen |
| `payment-term` | `zahlungsbedingung` | Zahlungsbedingungen anzeigen (nur lesen) |
| `project` | `projekt` | Projekte |
| `task` | `aufgabe` | Aufgaben eines Projekts |
| `time` | `zeit`, `time-entry` | Zeiteinträge: `log`, lokaler Timer (`start`, `status`, `stop`, `discard`), anlegen, ändern, archivieren, löschen |
| `custom-attribute` | `datenfeld` | benutzerdefinierte Datenfelder |
| `bank-connection` | `bankverbindung` | Bankverbindungen anzeigen (nur lesen) |
| `transaction` | `umsatz` | Bankumsätze anzeigen (nur lesen), z. B. für `voucher pay --transaction` |
| `user` | `benutzer` | Benutzer des Accounts anzeigen (nur lesen) |
| `info` | – | Account-Informationen inkl. Rechnungsvorlagen |
| `api` | – | beliebige Anfrage an die API (wie `gh api`), mit denselben Schutzregeln |

Dazu kommen `context`, `local`, `commands`, `schema`, `completion` und `version` (lokal, ohne Credits) sowie `auth` (`login` und `status` prüfen per `GET /info`, je 1 Credit; `logout` ist lokal).

### Namen statt IDs

ID-Flags benannter Ressourcen nehmen statt der ID auch den Namen: `--customer`, `--creditor`, `--company`, `--project`, `--customer-project`, `--contact-person`, `--task`, `--proposition`, `--default-proposition`, `--payment-term`, `--user` und `--team-member`, als Feld (`create`, `update`), Filter (`list`) und übergeordnetes Objekt (`person … --company`). `papierkram commands --json` nennt sie mit `resolves`.

```sh
papierkram invoice create --name "Beratung Oktober" --customer "Mustermann GmbH" --payment-term 1130 --document-date heute --item "name=Beratung,qty=2,unit=Stunde,price=100,vat=19" --dry-run  # Platzhalter
papierkram invoice list --company "Mustermann GmbH"  # Platzhalter
papierkram person list --company "Mustermann GmbH"  # Platzhalter
papierkram project create --name "Relaunch" --customer 3214 --team-member "Max Mustermann" --team-member 7  # Platzhalter
```

- Ein Wert aus Ziffern (auch mit Vorzeichen oder Leerraum) ist immer eine ID und kostet nichts. Eine Firma, die „1234“ heißt, geht deshalb nur über ihre ID.
- Einen Namen sucht das CLI in der Liste der Ressource: 1 Credit je geladener Seite (100 Einträge), höchstens 5 Seiten. Verglichen wird der ganze Name ohne Groß- und Kleinschreibung und mit zusammengefasstem Leerraum, bei Personen und Benutzern „Vorname Nachname“. Archivierte Einträge zählen nicht.
- Nur genau ein Treffer gilt; das CLI meldet ihn auf stderr (`Hinweis: Kunde „Mustermann GmbH“ → 3214 (1 Credit)`). Kein Treffer ist Exit 4 mit bis zu drei ähnlichen Namen, mehrere Treffer oder eine Liste über 500 Einträge Exit 2 mit den Kandidaten; gesendet wird dann nichts außer den Listenanfragen.
- Die Liste bleibt wie bei der Vervollständigung 60 Sekunden im Cache; ein Treffer daraus kostet nichts (`aus dem Zwischenspeicher, 0 Credits`). Kontaktpersonen und die Projekte eines Kunden werden nicht zwischengespeichert.
- `--contact-person` per Name braucht `--customer` (ID oder Name): Die Personen hängen an der Firma. `--customer-project` per Name sucht mit `--customer` nur unter den Projekten dieses Kunden, ohne `--customer` unter allen Projekten.
- Aufgelöst wird nach dem Schreibschutz (Exit 9 sendet nichts) und vor der Prüfung des Bodys; ein ungültiger Body kostet dann die Credits der Auflösung. `--dry-run` löst ebenfalls auf und zeigt das Ergebnis (`resolved`).
- Keine Namen nehmen Positionsargumente (`invoice describe <id>`), `papierkram api`, `--from-estimate`, `--custom-template`, `time list --invoice`, `transaction list --bank-connection`, `voucher pay --transaction` sowie `time log`/`time start` (dort gelten die Aliase der Projektdatei), `local init` und `context create`.

### Offene Posten

`papierkram invoice paid-status` zeigt die unbezahlten Rechnungen mit Summe: Rechnungen mit `outstanding_amount` über 0, deren Status nicht `draft` oder `canceled` ist (archivierte zählen mit), sortiert nach Fälligkeit, mit den Tagen seit der Fälligkeit und der exakt gerechneten Summe. Der Befehl liest nur und läuft auch unter Schreibschutz.

```sh
papierkram invoice paid-status
papierkram invoice paid-status --overdue
papierkram invoice paid-status --customer 3214 --from 2026-01-01 --to 2026-12-31 -o json
```

```text
NR.      KUNDE            DATUM       FÄLLIG      ÜBERFÄLLIG  BRUTTO    OFFEN
RE-0003  Mustermann GmbH  2026-05-01  2026-06-01  9           119,00 €  119,00 €
RE-0005  Mustermann GmbH  2026-05-01  2026-06-20              119,00 €  50,50 €
Summe offen: 169,50 € (2 Rechnungen, davon 1 überfällig; Stand 2026-06-10)
```

- `--customer` (ID oder Name), `--from` und `--to` (Rechnungsdatum) filtern wie bei `invoice list`; `--overdue` zeigt nur Rechnungen mit Fälligkeit vor heute (Berliner Tag). Rechnungen ohne Fälligkeit sind nie überfällig.
- Geladen werden höchstens `--max-pages` Seiten à 100 Rechnungen (Standard 5, 1 Credit je Seite). Gibt es mehr, warnt das CLI auf stderr, und JSON hat `complete` `false`: Summe und Anzahl gelten dann nur für die geladenen Seiten.
- Fehlt `outstanding_amount` in der Liste, bricht der Befehl ab (Exit 1), statt eine falsche Summe zu zeigen; `--details` liest diese Rechnungen dann einzeln nach (1 Credit je Rechnung).
- `-o json` liefert `entries`, `total_outstanding`, `count`, `overdue_count`, `as_of` und `complete` (Vertrag: [docs/schemas/paid-status.json](docs/schemas/paid-status.json)).

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

`--subdomain` (bei `context create` und `auth login`) nimmt auch den Host oder die URL des Accounts: `papierkram context create firma --subdomain https://firma.papierkram.de` legt den Kontext mit der Subdomain `firma` an und nennt sie auf stderr (`Hinweis: Subdomain „firma“ aus „https://firma.papierkram.de“`). Ports und andere Domains lehnt das CLI ab (Exit 2). Die Subdomain in `config.toml` und `PAPIERKRAM_SUBDOMAIN` bleiben strikt: dort nur die Subdomain selbst.

`context delete` löscht auch das Token des Kontexts im Schlüsselbund; `auth logout` entfernt nur das Token, der Kontext bleibt. Läuft für einen Kontext ein Timer, verweigern `context rename` und `context delete` mit Exit 7, bis er mit `time stop` gebucht oder mit `time discard` verworfen ist.

Welcher Kontext gilt, entscheidet diese Reihenfolge: `--context` > `PAPIERKRAM_CONTEXT` > `context` aus der [Projektdatei](#projektdatei) > `active_context` der Konfiguration. `papierkram auth status` zeigt den wirksamen Kontext mit Herkunft, die Token-Quelle (nie das Token selbst), die Schreibschutzstufe und das Restkontingent.

Die Konfiguration liegt in `config.toml` im Konfigurationsverzeichnis des Betriebssystems (`papierkram/config.toml` unter `~/Library/Application Support` auf macOS, `~/.config` bzw. `$XDG_CONFIG_HOME` auf Linux, `%AppData%` auf Windows) oder unter `PAPIERKRAM_CONFIG_DIR`:

```toml
active_context = "firma-gmbh"

[contexts.firma-gmbh]
subdomain = "firma-gmbh"
read_only = false
output = "table"            # Standardformat: table, wide, json oder yaml
timeout = "30s"             # Zeitlimit je Anfrage (Uploads mindestens 120 s)
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
- **Vorrang:** Flag > Umgebungsvariable (`PAPIERKRAM_COMPANY`, `PAPIERKRAM_PROJECT`, `PAPIERKRAM_TASK`, `PAPIERKRAM_USER`) > Projektdatei > Kontext (nur `user`).
- **Genutzt von:** `person list|create|describe|update|delete` (Firma für `--company`; die Person-ID bleibt Pflicht), `time log` und `time start` (Aufgabe, Benutzer; `--task` nimmt auch einen Alias aus `[tasks]`), `time list`, `task list`, `task create` und `project describe` ohne ID (Projekt).
- **Kontextbindung:** Nennt die Datei einen `context`, gelten ihre IDs nur, wenn genau dieser Kontext wirksam ist. Wählt `--context` oder `PAPIERKRAM_CONTEXT` einen anderen, ignoriert das CLI die Werte der Datei mit einer Warnung, damit keine ID im falschen Account landet.
- `papierkram local show` zeigt die wirksamen Werte mit Herkunft (`flag`, `env`, `projectfile`, `config`, `context`, `default`). Die Zeile `company` (aus `PAPIERKRAM_COMPANY` oder der Projektdatei) ist der Wert, den `person … --company` ohne Flag nutzt. `local init` fragt im Terminal ohne Flags jeden Wert ab.

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
- Ein Zeiteintrag hat nur ein Datum: Ein Ende genau um Mitternacht ist erlaubt (`--to 24:00` oder `--to 00:00` bei Beginn nach 00:00, gesendet als `24:00`, mit dem Hinweis „Ende 00:00 als 24:00 (Mitternacht) gebucht“; ebenso `time stop` in der ersten Minute nach Mitternacht). Andere Einträge über Mitternacht lehnt das CLI ab (Exit 2); sie werden als zwei Einträge erfasst (bis `24:00` und ab `00:00` am Folgetag).
- `heute` und `gestern` folgen einer Regel: Zeiterfassung (`time …`): Tag der Rechneruhr; Belege, Zahlungen, Datumsfilter: Berliner Tag (Europe/Berlin, auch auf einem Rechner in UTC). Zur Zeiterfassung gehören `time create`/`time update --entry-date`, `time log --date`, `time stop` und `time list --since`/`--until`, wie die Uhrzeiten der Einträge; `time list --since heute` findet so den Eintrag, den `time log` oder `time stop` gerade gebucht hat. `--since`/`--until` beginnen bzw. enden um 00:00:00 bzw. 23:59:59 Uhr Ortszeit des Rechners.
- Im Terminal zeigt `time log` vorher eine Zusammenfassung mit Firma, Projekt, Aufgabe und Benutzer und fragt nach (Enter = ja). Jeder Name kostet beim ersten Mal 1 Credit und bleibt 60 Sekunden im Cache. `--yes`, `--no-input` und Aufrufe ohne Terminal senden ohne Rückfrage.

Der **Timer** läuft lokal (eine Datei je Kontext im Cache-Verzeichnis); Papierkram kennt ihn nicht:

```sh
papierkram time start --task tickets -m "Ticket 4711"
papierkram time start --task tickets --unbillable
papierkram time status
papierkram time stop
papierkram time stop -m "Fehler behoben"
papierkram time discard
```

`time stop` bucht die Zeit vom Start bis jetzt wie `time log` und löscht den Timer erst, wenn der Zeiteintrag angelegt ist. `time start --unbillable` (auch `--unbillable=false`) prüft den Wert sofort (Exit 5, Timer nicht gestartet) und speichert ihn; `time stop` sendet ihn wie `time log --unbillable`, ohne Angabe keinen. `time stop` selbst hat kein `--unbillable`; `time status` zeigt den Wert (`-o wide`: Spalte `NICHT ABRECHENBAR`). Läuft schon ein Timer, bricht `time start` mit Exit 7 ab. Direkt vor dem Senden beansprucht `time stop` den Timer atomar (exklusiv angelegte Kopie `….stopping`, danach wird die Timer-Datei entfernt): Ein zweiter `time stop` desselben Timers, etwa in einem anderen Terminal, endet mit Exit 7 und sendet nichts. Scheitert die Anfrage, läuft der Timer weiter. Solange `….stopping` liegt (ein Stopp sendet gerade oder brach ab), brechen `time start` und `time stop` mit Exit 7 ab und `time status` warnt: erst mit `time list --since <Startdatum>` prüfen, ob der Eintrag angelegt wurde, dann, wenn kein anderer `time stop` mehr läuft, `time discard` (und, falls nicht, von Hand mit `time log` buchen).

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
- `--dry-run` gilt für jede Änderung: Das CLI prüft den Body lokal gegen das Schema, zeigt Methode, URL, Body (bei Uploads Dateiname, Größe und MIME-Typ) und die Credits und sendet nichts (Exit 0). Unter Schreibschutz gilt auch für `--dry-run` Exit 9. Namen in Flags werden auch bei `--dry-run` aufgelöst (Zeile `Aufgelöst: …`, Credits verbraucht und in den Credits enthalten), siehe [Namen statt IDs](#namen-statt-ids).

```sh
papierkram invoice create --name "Beratung Oktober" --customer 3214 --payment-term 1130 --document-date heute --item "name=Beratung,qty=2,unit=Stunde,price=100,vat=19" --dry-run
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
| 0 | lokale Befehle (`context`, `local`, `commands`, `schema`, `completion`, `version`, `auth logout`, `time start|status|discard`) |

Zusammengesetzte Befehle kosten die Summe: `deliver` 2 (Versand und Nachlesen), `voucher create --file` 60, `invoice create --from-estimate` 51, `invoice update` 2 (liest die Rechnung, um ihre Zahlungsbedingung und bei `--customer` Kontaktperson und Projekt mitzusenden; 1 mit `--payment-term`, außer mit `--customer` ohne `--contact-person`/`--customer-project`), `auth login` und `auth status` je 1, `invoice paid-status` 1 je Seite (höchstens `--max-pages`, Standard 5) und mit `--details` 1 je nachgelesener Rechnung. Ein Name statt einer ID kostet 1 Credit je geladener Seite der Liste (höchstens 5), aus dem Cache nichts ([Namen statt IDs](#namen-statt-ids)). `--all` kostet 1 Credit je geladener Seite; ab 10 Seiten warnt das CLI und fragt im Terminal nach, `--max-pages` begrenzt. `company list --type` lädt immer alle Seiten (die API filtert nicht). Die Vervollständigung von IDs kostet je Abfrage 1 Credit. Fällt das Restkontingent unter 100, warnt das CLI auf stderr.

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
| `PAPIERKRAM_COMPANY` | Firmen-ID für `--company` von `person list\|create\|describe\|update\|delete` (vor der Projektdatei) |
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
| 7 | `conflict` | 409, Timer läuft schon oder wird gerade gestoppt |
| 8 | `confirmation_required` | Bestätigung fehlt oder abgelehnt, oder Rückfrage mit Strg+C abgebrochen; nichts gesendet; `--yes` |
| 9 | `read_only` | Schreibschutz (`PAPIERKRAM_READ_ONLY` oder `read_only` im Kontext) |
| 10 | `network` | Zeitüberschreitung, DNS, TLS bei einem Lesezugriff oder bevor eine Änderung gesendet wurde; auch Strg+C während einer Leseanfrage |
| 11 | `partial` | mehrstufiger Befehl teilweise ausgeführt; Ausgabe nennt, was fehlt, und den Befehl zum Nachholen |
| 12 | `outcome_unknown` | Verbindung brach ab, **nachdem** eine Änderung gesendet wurde, oder die API beantwortete die Änderung mit einer Weiterleitung (3xx), der das CLI nicht folgt: vor einer Wiederholung mit `describe`/`list` prüfen |

Fehler stehen auf stderr als `Fehler: …` mit `Hinweis: …` und ggf. `Correlation-ID: …`; mit `-o json` oder `--jq` folgt als letzte Zeile eine kompakte JSON-Zeile, mit `-o yaml` ein YAML-Block (`error:` …) am Ende (Schema: [docs/schemas/error.json](docs/schemas/error.json); Auswertung siehe [AGENTS.md](AGENTS.md#fehler)). Lesende Anfragen (GET) wiederholt das CLI bei Netzwerk- und Serverfehlern bis zu zweimal, Änderungen und 429 nie.

## Eigenheiten der API

Gemessen gegen einen Sandbox-Account (Läufe 1 bis 4 am 2026-11-04, Lauf 5 am 2026-11-08) bzw. entschieden, wie das CLI damit umgeht; Einzelheiten und spätere Messungen stehen in [docs/offene-punkte.md](docs/offene-punkte.md):

- **Angebote und Rechnungen brauchen ein Datum:** `estimate create` und `invoice create` ohne `--document-date` lehnt die API mit 422 ab und berechnet trotzdem das Anlegen (gemessen beim Angebot); das CLI prüft das deshalb vorher (Exit 5), bei `invoice create --from-estimate` schon vor dem Lesen des Angebots.
- **Abgelehnte Anfragen kosten:** Auch ein mit 422 abgelehntes Anlegen kostet die vollen Credits.
- **Rechnung ändern:** Ein `PUT` ohne Zahlungsbedingung stellt eine Rechnung auf die Standard-Zahlungsbedingung des Accounts um und ändert das Fälligkeitsdatum. `invoice update` liest die Rechnung deshalb vorher und sendet ihre aktuelle Zahlungsbedingung mit (1 Credit mehr; nicht mit `--payment-term` oder `payment_term` in `--input`). `papierkram api PUT` liest nicht.
- **Kontaktperson und Projekt einer Rechnung** (offener Punkt 29): Ein `PUT`, dessen Kunde keine Kontaktperson und kein Projekt nennt, leert beide, auch beim selben Kunden. `invoice update --customer` ohne `--contact-person`/`--customer-project` nutzt dasselbe Lesen: Beim Kunden der Rechnung sendet es Kontaktperson und Projekt mit, bei einem anderen Kunden entfallen sie (Hinweis auf stderr; neu setzen per `--contact-person`/`--customer-project`); `papierkram api PUT` tut das nicht.
- **Firma für Kontaktpersonen** (Punkt 27): Die API erwartet die Firma im Pfad jedes `person`-Befehls; `--company` kommt ohne Flag aus `PAPIERKRAM_COMPANY` oder `company` der Projektdatei (Hinweis auf stderr), fehlt der Wert überall, endet der Befehl mit Exit 2 ohne Anfrage.
- **Ende um Mitternacht** (Punkt 22): Die API nimmt `ended_at_time` `00:00` und `24:00` nach einem Beginn um 23:00 an, je als eine Stunde. `time log` und `time stop` senden ein Ende genau um Mitternacht als `24:00` (`time create|update` senden `--ended-at-time` unverändert); andere Einträge über Mitternacht lehnen `time log` und `time stop` ab (Exit 2), weil ein Zeiteintrag nur ein Datum hat.
- **Der Timer ist lokal:** Papierkram kennt keinen laufenden Timer und sperrt nichts; zwei `time stop` desselben Timers würden zwei Einträge buchen. `time stop` beansprucht den Timer deshalb direkt vor dem Senden; ein zweiter Stopp endet mit Exit 7 ohne Anfrage (siehe [Zeiterfassung](#zeiterfassung)).
- **`--billable-duration` ist wirkungslos:** Gemessen wurde 1800 Sekunden bei einem Eintrag von 90 Minuten; gespeichert war beim Anlegen und beim Ändern die Dauer (5400).
- **PDF-Versand:** `deliver --via pdf` vergibt die Nummer und setzt den Status (Angebot `sent`, Rechnung `unpaid`), aber weder `sent_on` noch `sent_via` (gemessen an Angebot und Rechnung).
- **Storno:** `voucher reverse` und `voucher cancel` lehnt die API an einem unbezahlten Beleg mit 422 ab (Exit 5, der Beleg bleibt `unpaid`). Eine stornierte Rechnung (`canceled`, der Name bekommt „ (storniert)“) und ein storniertes Angebot lassen sich weder löschen noch erneut stornieren, nur archivieren.
- **`due_date` einer Rechnung** lässt sich per `PUT` nicht setzen (422); `invoice update --input` entfernt es, das Fälligkeitsdatum folgt aus der Zahlungsbedingung.
- **Upload:** PDF, PNG und JPG werden angenommen; TXT lehnt die API mit HTTP 500 ab, andere Typen sind nicht gemessen. `voucher upload` endet dann mit Exit 1 (`server_error`), `voucher create --file` mit Exit 11: Der Beleg ist angelegt, das Hochladen unbekannt (bei HTTP 500 oder einer Zeitüberschreitung) bzw. gescheitert (wenn die API die Datei mit 4xx ablehnt); nur das Hochladen nachholen, nie den ganzen Befehl. Das ist kein vorübergehender Fehler, also nicht mit derselben Datei wiederholen. Die ganze Datei muss innerhalb des Zeitlimits übertragen sein: `voucher upload` und das Hochladen bei `voucher create --file` warten ohne `--timeout` je Versuch mindestens 120 s (ein größeres `timeout` des Kontexts gilt); mit `--timeout` gilt genau dieser Wert, auch wenn er kleiner ist.

## Weitere Dokumentation

- [AGENTS.md](AGENTS.md): JSON-Vertrag, Exit-Codes und Regeln für KI-Agents
- [docs/reference/](docs/reference/): Befehlsreferenz, aus der Hilfe generiert
- [skills/papierkram/SKILL.md](skills/papierkram/SKILL.md): Claude-Code-Skill mit den Abläufen Beleg hochladen und zuordnen, Rechnung aus Angebot, offene Posten prüfen, Zeiten erfassen und abrechnen. Zum Einbinden den Ordner `skills/papierkram` nach `~/.claude/skills/` (für alle Projekte) oder nach `.claude/skills/` eines Projekts kopieren.
- [docs/schemas/](docs/schemas/): JSON-Schemas der Ausgaben
- [docs/offene-punkte.md](docs/offene-punkte.md): offene Fragen zur API und wie die E2E-Suite sie misst
- [CHANGELOG.md](CHANGELOG.md), [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)
- [docs/release.md](docs/release.md): Release mit GoReleaser, Snapshot-Test, Kurzform `pk` je Paketquelle
- Design: [Spezifikation](docs/specs/2026-11-03-papierkram-cli-design.md), [Implementierungsplan](docs/plans/2026-11-03-papierkram-cli.md), [Plan der Runde 2](docs/plans/2026-11-07-papierkram-cli-ausbau.md), [API-Snapshot](api/)

## Dank

Viele Fallen der Papierkram-API hat [papierkram-mcp-server](https://github.com/Wagner-Emden-IT-Services/papierkram-mcp-server) (MIT) live gefunden und dokumentiert. Dieses Projekt übernimmt von dort nur Wissen, keinen Code.

## Lizenz

MIT, siehe [LICENSE](LICENSE). Lizenzen der Abhängigkeiten: [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
