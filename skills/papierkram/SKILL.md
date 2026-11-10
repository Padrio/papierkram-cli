---
name: papierkram
description: Bedient die Buchhaltung Papierkram über das CLI `papierkram`. Verwenden, wenn der Nutzer in Papierkram Rechnungen, Angebote, Belege, Firmen, Projekte oder Zeiteinträge ansehen oder ändern will, etwa einen Beleg hochladen und zuordnen, eine Rechnung aus einem Angebot erstellen und versenden, offene Posten prüfen oder Zeiten erfassen und abrechnen, oder wenn Ausgabe, Exit-Code oder Fehler eines `papierkram`-Befehls auszuwerten sind.
---

# Papierkram mit dem CLI `papierkram`

Das CLI beschreibt sich selbst, prüft jeden Body vor dem Senden und hat feste Exit-Codes. Dieser Skill fasst die Regeln für Agents zusammen und zeigt vier Abläufe. Verbindlich ist [AGENTS.md](https://github.com/Padrio/papierkram-cli/blob/main/AGENTS.md) (JSON-Vertrag, Fehler, Exit 11 und 12 im Detail); bei Widersprüchen gilt AGENTS.md.

Befehle immer als `papierkram` aufrufen (die Kurzform `pk` gibt es nicht bei jeder Installation). Alle IDs unten sind Beispiele. IDs immer per `list` ermitteln, nie raten. Flags mit `resolves` in `commands --json` nehmen auch einen Namen (`--customer "Mustermann GmbH"`); das kostet 1 Credit je geladener Listenseite und gilt nur bei genau einem Treffer (sonst Exit 4 bzw. 2), Ziffern sind immer eine ID. Details: [AGENTS.md, Weitere Hinweise](https://github.com/Padrio/papierkram-cli/blob/main/AGENTS.md#weitere-hinweise).

## Sicherheitsregeln

1. **Schreibschutz, solange nichts geändert werden soll.** Will der Nutzer nur Auskunft, jeden Befehl mit vorangestelltem `PAPIERKRAM_READ_ONLY=1` ausführen. Ohne Schutz nur, wenn der Nutzer ausdrücklich eine Änderung verlangt. Exit 9 heißt: abbrechen und melden, den Schutz nie umgehen (kein anderer Kontext, keine Variable ändern).
2. **Immer `-o json` und `--no-input`.** stdout enthält nur Nutzdaten, stderr Hinweise und Fehler; das CLI fragt nie nach.
3. **Erst `--dry-run`, dann ausführen.** Vor jeder Änderung den Befehl mit `--dry-run` ausführen, dem Nutzer Methode, Body und `credits` zeigen, danach denselben Befehl ohne `--dry-run`. Lokale Befehle (`time start`, `local init`) senden nichts und kennen kein `--dry-run`.
4. **`--yes` nur mit Freigabe.** `destructive` und `send` (delete, cancel, reverse, pay, document delete, deliver) brauchen `--yes`; das setzt der Agent nur, wenn der Mensch genau diese Aktion freigegeben hat. Exit 8 heißt: nichts passiert, nachfragen.
5. **`deliver` nie zweimal.** Kein zweiter Aufruf, weder nach Exit 0 mit Warnung noch nach Exit 12 noch „zur Sicherheit“: Er kann eine zweite E-Mail an den Kunden schicken. Stattdessen `describe` und `sent_on` prüfen.
6. **Exit 11 und 12 nie blind wiederholen.** Erst `hint` lesen, mit `describe` oder `list` nachsehen, dann höchstens den einen Nachholschritt aus dem Hinweis (bei `voucher create --file` das `retry_command`). Ablauf je Befehl: [Exit 11 und 12 Schritt für Schritt](https://github.com/Padrio/papierkram-cli/blob/main/AGENTS.md#exit-11-und-12-schritt-für-schritt).
7. **Exit 6 (429) nie wiederholen**, keine Tokens in Befehlen, `--all` immer mit `--max-pages`. Jede Anfrage kostet Monats-Credits; `credits` steht in `commands --json` und in der `--dry-run`-Ausgabe.

**Über MCP.** Läuft `papierkram mcp serve` als MCP-Server, heißen die Befehle als Tools `<ressource>_<aktion>` (`invoice_list`, `company_create`); ein Tool-Aufruf ist derselbe Befehl mit `-o json --no-input`, alle Regeln oben gelten unverändert. Statt `--dry-run` gibt es den Parameter `dry_run`. Schreibende Tools gibt es nur, wenn der Mensch den Server mit `--allow-writes` gestartet hat, löschende und versendende nur mit `--allow-destructive` (sie laufen dann mit `--yes`, also trotzdem vorher `dry_run` und die Freigabe des Nutzers für genau diese Aktion einholen). `deliver` sendet dort nur als PDF. Fehler kommen als Tool-Fehler mit der JSON-Fehlerzeile (`code`, `exit_code`); den zweiten Inhalt mit Hinweisen von stderr immer lesen. Details: [AGENTS.md, MCP-Server](https://github.com/Padrio/papierkram-cli/blob/main/AGENTS.md#mcp-server).

## Befehle und Schemas finden

```sh
papierkram commands --json --no-input
papierkram commands --json --no-input --jq '.commands[] | select(.command | startswith("voucher")) | {command, class, credits, usage}'
papierkram schema voucher create
papierkram schema invoice list --jq '.response'
```

`commands --json` nennt je Befehl Klasse (`read_only`, `create`, `update`, `archive`, `destructive`, `send`, `local`, `per_request`), Credits, Flags mit erlaubten Werten und Beispiele. `schema <ressource> [<aktion>]` zeigt Request- und Response-Schema einer Aktion. Beide senden keine Anfrage und kosten nichts; `papierkram <befehl> --help` zeigt dieselbe Hilfe mit Beispielen.

## Ablauf 1: Beleg hochladen und zuordnen

Lieferanten und vorhandene Belege suchen (lesend):

```sh
PAPIERKRAM_READ_ONLY=1 papierkram company list --type supplier --max-pages 3 -o json --no-input --jq '.entries[] | {id, name}'
PAPIERKRAM_READ_ONLY=1 papierkram voucher list --creditor 2904 --from 2026-10-01 --to 2026-10-31 -o json --no-input --jq '.entries[] | {id, voucher_no, name, document_date, amount}'
```

Neuen Beleg mit Datei anlegen (60 Credits: Anlegen 50, Hochladen 10). `category` muss ein Wert aus `papierkram schema voucher create` sein:

```sh
papierkram voucher create --name "Büromaterial Oktober" --provenance domestic --creditor 2904 --document-date 2026-10-28 --item "name=Papier,amount=11.90,vat=19,category=Bürobedarf" --file beleg.pdf --dry-run -o json --no-input
papierkram voucher create --name "Büromaterial Oktober" --provenance domestic --creditor 2904 --document-date 2026-10-28 --item "name=Papier,amount=11.90,vat=19,category=Bürobedarf" --file beleg.pdf -o json --no-input
```

Endet das mit Exit 11, ist der Beleg angelegt und nur das Hochladen fehlt oder ist unsicher: **nie** den ganzen Befehl wiederholen (sonst ein zweiter Beleg), sondern dem Hinweis folgen (erst `voucher describe`, dann höchstens `retry_command`; enthält der Hinweis „Kontingent erschöpft – nicht jetzt nachholen“, auch das nicht, sondern melden).

Datei an einen vorhandenen Beleg hängen und das Ergebnis prüfen:

```sh
papierkram voucher upload 295 quittung.jpg --dry-run -o json --no-input
papierkram voucher upload 295 quittung.jpg -o json --no-input
papierkram voucher describe 295 -o json --no-input --jq '.documents'
```

Falsche Datei entfernen (`destructive`, nur mit Freigabe; die Datei-ID steht in `documents[].id`):

```sh
papierkram voucher document delete 295 4711 --dry-run -o json --no-input
papierkram voucher document delete 295 4711 --yes -o json --no-input
```

Einem Bankumsatz zuordnen, also als bezahlt buchen (`destructive`: eine Zahlung lässt sich per API nicht zurücknehmen; nur mit Freigabe und nie wiederholen):

```sh
PAPIERKRAM_READ_ONLY=1 papierkram transaction list --all --max-pages 2 -o json --no-input --jq '.entries[] | {id, bdate, value, usage}'
papierkram voucher pay 295 --transaction 457 --dry-run -o json --no-input
papierkram voucher pay 295 --transaction 457 --yes -o json --no-input
```

## Ablauf 2: Rechnung aus Angebot

Angebot und Zahlungsbedingung suchen (lesend):

```sh
PAPIERKRAM_READ_ONLY=1 papierkram estimate list --company 3214 -o json --no-input --jq '.entries[] | {id, estimate_no, name, state, total_gross}'
PAPIERKRAM_READ_ONLY=1 papierkram estimate list --company "Mustermann GmbH" -o json --no-input  # Platzhalter
PAPIERKRAM_READ_ONLY=1 papierkram payment-term list -o json --no-input --jq '.entries[] | {id, name}'
```

Rechnung anlegen (51 Credits: Angebot lesen 1, Anlegen 50). `--payment-term` und `--document-date` sind Pflicht (ohne Datum Exit 5, bevor das Angebot gelesen wird). Auch `--dry-run` liest das Angebot (1 Credit) und zeigt den vollständigen Body:

```sh
papierkram invoice create --from-estimate 349 --payment-term 1130 --document-date heute --dry-run -o json --no-input
papierkram invoice create --from-estimate 349 --payment-term 1130 --document-date heute -o json --no-input
```

Die Ausgabe enthält die `id` der neuen Rechnung (hier 459). Bei Exit 12 nicht erneut anlegen, sondern per `invoice list` nach der Rechnung suchen.

**Versand nur nach Bestätigung durch den Menschen.** Vorher Kunde, Betrag und Versandart nennen und auf ein ausdrückliches Ja warten. `--via pdf` finalisiert die Rechnung und vergibt die Nummer ohne E-Mail:

```sh
papierkram invoice describe 459 -o json --no-input --jq '{id, name, total_gross, kunde: .billing.company, state}'
papierkram invoice deliver 459 --via pdf --dry-run -o json --no-input
papierkram invoice deliver 459 --via pdf --yes -o json --no-input
```

`--via email` nur mit Empfänger, Betreff und Text, die der Mensch vorgegeben hat:

```sh
papierkram invoice deliver 459 --via email --to kunde@example.org --subject "Rechnung {{rechnung.rechnungsnummer}}" --body-file mail.txt --dry-run -o json --no-input
papierkram invoice deliver 459 --via email --to kunde@example.org --subject "Rechnung {{rechnung.rechnungsnummer}}" --body-file mail.txt --yes -o json --no-input
```

Rechnung vor dem Versand ändern: `invoice update` liest die Rechnung vorher (zusammen mit dem Ändern 2 Credits) und sendet ihre Zahlungsbedingung mit, sonst stellt die API sie auf den Standard um. Mit `--customer` sendet es beim Kunden der Rechnung auch Kontaktperson und Projekt mit (ohne sie leert die API beide); nennt `--customer` einen anderen Kunden, entfallen sie, der Hinweis auf stderr sagt das. Dann Kontaktperson und Projekt beim Menschen erfragen und per `--contact-person`/`--customer-project` mitgeben:

```sh
papierkram invoice update 459 --customer 3214 --description "Leistungszeitraum Oktober" --dry-run -o json --no-input
papierkram invoice update 459 --customer 3214 --description "Leistungszeitraum Oktober" -o json --no-input
```

`deliver` läuft genau einmal. Nach Exit 12 oder einer Warnung „Nachlesen fehlgeschlagen“ nur `invoice describe 459` lesen (`sent_on`, `state`, `invoice_no`) und den Menschen entscheiden lassen.

## Ablauf 3: offene Posten prüfen

Alles lesend, auch unter Schreibschutz. `invoice paid-status` rechnet die offenen Posten selbst aus: Rechnungen mit `outstanding_amount` über 0, deren Status nicht `draft`, `canceled` oder `deleted` ist (archivierte zählen mit), sortiert nach Fälligkeit, mit `days_overdue` (Tage seit der Fälligkeit, Berliner Tag) und der exakten Summe `total_outstanding`. `--overdue` zeigt nur überfällige (Fälligkeit vor heute); `--customer` (ID oder Name), `--from` und `--to` (Rechnungsdatum) filtern wie bei `invoice list`:

```sh
PAPIERKRAM_READ_ONLY=1 papierkram invoice paid-status --overdue -o json --no-input
PAPIERKRAM_READ_ONLY=1 papierkram invoice paid-status --customer 3214 -o json --no-input
PAPIERKRAM_READ_ONLY=1 papierkram invoice paid-status --from 2026-01-01 --to 2026-10-31 -o json --no-input --jq '.entries[] | {invoice_no, customer, due_date, days_overdue, outstanding_amount}'
PAPIERKRAM_READ_ONLY=1 papierkram invoice describe 459 -o json --no-input
```

Der Bericht lädt höchstens `--max-pages` Seiten à 100 Rechnungen (Standard 5, 1 Credit je Seite). Ist `complete` `false` (stderr: „5 von N Seiten geladen“), sind Liste und Summe unvollständig: das dem Menschen sagen und `total_outstanding` nicht als Gesamtbetrag ausgeben; mit Filtern eingrenzen oder `--max-pages` nach Rücksprache erhöhen. Exit 1 „Rechnungsliste ohne outstanding_amount …“ heißt: Die Liste liefert den offenen Betrag nicht; `--details` liest die betroffenen Rechnungen einzeln (1 Credit je Rechnung, die Zahl steht in der Meldung). Die Statuswerte dokumentiert die API nicht vollständig ([offene Punkte](https://github.com/Padrio/papierkram-cli/blob/main/docs/offene-punkte.md)); zeigt der Bericht einen unerwarteten `state` (etwa `imported`), die Rechnung per `invoice describe` prüfen.

## Ablauf 4: Zeiten erfassen und abrechnen

Standardwerte für ein Verzeichnis (Firma, Projekt, Aufgabe, Benutzer) legt die Projektdatei `.papierkram.toml` fest; `local show` zeigt, was gilt. Aufgaben-Aliase aus `[tasks]` gehen statt IDs. `local init` nur auf Wunsch des Nutzers: Es schreibt die Projektdatei neu (Kommentare gehen verloren) und kennt kein `--dry-run`; deshalb vorher `local show` lesen und die geplante Änderung nennen. Fehlt ein Flag, nimmt das CLI den Wert aus `PAPIERKRAM_<NAME>` oder der Projektdatei und nennt die Quelle auf stderr (`Hinweis: Firma 2843 aus PAPIERKRAM_COMPANY`); das gilt auch für `--company` aller `person`-Befehle. Ohne Wert endet ein Befehl, der ihn braucht, mit Exit 2 ohne Anfrage:

```sh
papierkram local show -o json --no-input
papierkram local init --project 4963 --task 545 --no-input
```

Zeit nachtragen (10 Credits je Eintrag, **nicht** idempotent: jeder Aufruf legt einen neuen Eintrag an):

```sh
papierkram time log --task 545 --from 9:00 --to 10:30 -m "Abstimmung mit Kunde" --dry-run -o json --no-input
papierkram time log --task 545 --from 9:00 --to 10:30 -m "Abstimmung mit Kunde" -o json --no-input
papierkram time log --task tickets --date gestern --from 14:00 --duration 1h30m --dry-run -o json --no-input
papierkram time log --task tickets --date gestern --from 14:00 --duration 1h30m -o json --no-input
```

Ein Eintrag hat nur ein Datum: Ein Ende genau um Mitternacht geht mit `--to 24:00` (oder `--to 00:00` bei Beginn nach 00:00, gesendet als `24:00`); `time stop` in der ersten Minute nach Mitternacht bucht bis `24:00` des Starttags. Andere Einträge über Mitternacht enden mit Exit 2 ohne Anfrage und werden als zwei Einträge gebucht (bis `24:00`, ab `00:00` am Folgetag mit `--date`).

Timer (lokal, kostet erst beim Stoppen 10 Credits):

```sh
papierkram time start --task tickets -m "Ticket 4711" -o json --no-input
papierkram time status -o json --no-input
papierkram time stop --dry-run -o json --no-input
papierkram time stop -o json --no-input
```

Nicht abrechenbare Zeit: `--unbillable` gehört an `time start` (wie an `time log`); `time start` prüft den Wert sofort (Exit 5, kein Timer gestartet), `time stop` kennt das Flag nicht und sendet den beim Start gespeicherten Wert. `time start` endet mit Exit 7, wenn schon ein Timer läuft oder ein `time stop` nicht abgeschlossen ist; dann erst `time status` lesen.

`time stop` nach Exit 11 **nie** erneut ausführen: Der Eintrag ist angelegt. Nach Prüfung per `time list --since heute` den Timer mit `papierkram time discard --yes` verwerfen, nur mit Freigabe (die erfasste Zeit geht verloren).

`time stop` mit Exit 7 sendet nichts: Entweder bucht gerade ein anderer `time stop` denselben Timer (nicht wiederholen, `time status` und `time list --since heute` lesen), oder ein `time stop` läuft gerade bzw. wurde nicht abgeschlossen (Hinweis „Ein time stop läuft gerade oder wurde nicht abgeschlossen“): erst per `time list --since <Startdatum>` prüfen, ob der Eintrag existiert, dann, wenn kein anderer `time stop` mehr läuft, wie im Hinweis `time discard --yes` (mit Freigabe) und, falls er fehlt, von Hand mit `time log` buchen.

Abrechnen: offene Zeiten eines Projekts lesen und die abrechenbaren Stunden summieren. Die API speichert als `billable_duration` immer die Dauer aus Beginn und Ende (Sekunden; gemessen, [offene Punkte](https://github.com/Padrio/papierkram-cli/blob/main/docs/offene-punkte.md), Nr. 15), `--billable-duration` ist wirkungslos. Deshalb die Summe mit den Einzeleinträgen vergleichen und dem Menschen vor dem Anlegen der Rechnung nennen:

```sh
PAPIERKRAM_READ_ONLY=1 papierkram time list --project 4963 --state unbilled --all --max-pages 5 -o json --no-input --fields id,started_at,duration,billable_duration,comments
PAPIERKRAM_READ_ONLY=1 papierkram time list --project 4963 --state unbilled --all --max-pages 5 -o json --no-input --jq '([.entries[].billable_duration // 0] | add // 0) / 3600'
```

Aus der bestätigten Summe eine Rechnung anlegen (Kunde, Zahlungsbedingung und Stundensatz kommen vom Menschen) und danach wie in Ablauf 2 nur nach Bestätigung versenden. Nennt der Mensch den Kunden nur beim Namen, geht `--customer "…"` direkt: Schon `--dry-run` löst ihn auf (1 Credit je Seite der Firmenliste) und zeigt die ID unter `resolved`; den echten Aufruf dann mit dieser ID ausführen. Bei Exit 4 (kein Treffer) oder Exit 2 (mehrdeutig) den Menschen fragen, nie einen der ähnlichen Namen aus dem Hinweis selbst wählen:

```sh
papierkram invoice create --name "Beratung Oktober" --customer "Mustermann GmbH" --payment-term 1130 --document-date heute --item "name=Beratung Oktober,qty=12.5,unit=Stunde,price=100,vat=19" --dry-run -o json --no-input  # Platzhalter
papierkram invoice create --name "Beratung Oktober" --customer 3214 --payment-term 1130 --document-date heute --item "name=Beratung Oktober,qty=12.5,unit=Stunde,price=100,vat=19" --dry-run -o json --no-input
papierkram invoice create --name "Beratung Oktober" --customer 3214 --payment-term 1130 --document-date heute --item "name=Beratung Oktober,qty=12.5,unit=Stunde,price=100,vat=19" -o json --no-input
```

Den Abrechnungsstatus der Zeiteinträge kann das CLI nicht setzen (`papierkram schema time update` hat kein Feld dafür); das bleibt in Papierkram selbst zu erledigen.

## Exit-Codes

| Exit | `code` | Was der Agent tut |
|---|---|---|
| 0 | – | Ergebnis auswerten, `warnings[]` und stderr lesen |
| 1 | `error`, `server_error` | Lesen: später einmal erneut. Änderung: erst per `describe`/`list` prüfen. „Befehl ausgeführt, Ausgabe fehlgeschlagen“ oder „Antwort größer als 64 MiB; die Änderung wurde ausgeführt“: angekommen, nicht wiederholen. „Antwort größer als 64 MiB, abgebrochen“ (Lesen): nicht unverändert wiederholen; bei `list` mit kleinerem `--limit`, sonst melden |
| 2 | `usage` | Befehl nach `Hinweis` korrigieren; nichts gesendet (bei einem mehrdeutigen Namen nur die Listenanfragen: ID aus der Meldung nach Rückfrage nehmen) |
| 3 | `auth` | abbrechen, Menschen um `papierkram auth login` bitten |
| 4 | `not_found` | ID und Kontext per `list` prüfen; bei einem Namen ohne Treffer den Menschen fragen |
| 5 | `validation` | Body nach `message` und `schema` korrigieren; nicht angewendet |
| 6 | `quota` | Kontingent erschöpft: abbrechen, **nicht** wiederholen |
| 7 | `conflict` | 409 nicht angewendet, beim Timer nichts gesendet; Zustand lesen (`describe`, `time status`); `time stop` nicht wiederholen (siehe Ablauf 4) |
| 8 | `confirmation_required` | nichts gesendet (auch wenn ein Mensch eine Rückfrage mit Strg+C abbricht; mit `--no-input` vor jeder Anfrage, auch vor der Namensauflösung); Menschen fragen, `--yes` nur mit Freigabe |
| 9 | `read_only` | abbrechen und melden; Schutz nie umgehen |
| 10 | `network` | später erneut; die Änderung ist nicht angekommen |
| 11 | `partial` | `hint` lesen, höchstens den genannten Nachholschritt ([Details](https://github.com/Padrio/papierkram-cli/blob/main/AGENTS.md#exit-11-und-12-schritt-für-schritt)) |
| 12 | `outcome_unknown` | **nicht** wiederholen: per `describe`/`list` prüfen, ob die Änderung angekommen ist ([Details](https://github.com/Padrio/papierkram-cli/blob/main/AGENTS.md#exit-11-und-12-schritt-für-schritt)) |

Mit `-o json` ist die letzte Zeile von stderr der Fehler als JSON (`error.code`, `error.message`, `error.hint`, `error.exit_code`); siehe [Fehler](https://github.com/Padrio/papierkram-cli/blob/main/AGENTS.md#fehler).
