---
name: papierkram
description: Bedient die Buchhaltung Papierkram über das CLI `papierkram`. Verwenden, wenn der Nutzer in Papierkram Rechnungen, Angebote, Belege, Firmen, Projekte oder Zeiteinträge ansehen oder ändern will, etwa einen Beleg hochladen und zuordnen, eine Rechnung aus einem Angebot erstellen und versenden, offene Posten prüfen oder Zeiten erfassen und abrechnen, oder wenn Ausgabe, Exit-Code oder Fehler eines `papierkram`-Befehls auszuwerten sind.
---

# Papierkram mit dem CLI `papierkram`

Das CLI beschreibt sich selbst, prüft jeden Body vor dem Senden und hat feste Exit-Codes. Dieser Skill fasst die Regeln für Agents zusammen und zeigt vier Abläufe. Verbindlich ist [AGENTS.md](https://github.com/Padrio/papierkram-cli/blob/main/AGENTS.md) (JSON-Vertrag, Fehler, Exit 11 und 12 im Detail); bei Widersprüchen gilt AGENTS.md.

Befehle immer als `papierkram` aufrufen (die Kurzform `pk` gibt es nicht bei jeder Installation). Alle IDs unten sind Beispiele. IDs immer per `list` ermitteln, nie raten; Flags nehmen IDs, keine Namen.

## Sicherheitsregeln

1. **Schreibschutz, solange nichts geändert werden soll.** Will der Nutzer nur Auskunft, jeden Befehl mit vorangestelltem `PAPIERKRAM_READ_ONLY=1` ausführen. Ohne Schutz nur, wenn der Nutzer ausdrücklich eine Änderung verlangt. Exit 9 heißt: abbrechen und melden, den Schutz nie umgehen (kein anderer Kontext, keine Variable ändern).
2. **Immer `-o json` und `--no-input`.** stdout enthält nur Nutzdaten, stderr Hinweise und Fehler; das CLI fragt nie nach.
3. **Erst `--dry-run`, dann ausführen.** Vor jeder Änderung den Befehl mit `--dry-run` ausführen, dem Nutzer Methode, Body und `credits` zeigen, danach denselben Befehl ohne `--dry-run`. Lokale Befehle (`time start`, `local init`) senden nichts und kennen kein `--dry-run`.
4. **`--yes` nur mit Freigabe.** `destructive` und `send` (delete, cancel, reverse, pay, document delete, deliver) brauchen `--yes`; das setzt der Agent nur, wenn der Mensch genau diese Aktion freigegeben hat. Exit 8 heißt: nichts passiert, nachfragen.
5. **`deliver` nie zweimal.** Kein zweiter Aufruf, weder nach Exit 0 mit Warnung noch nach Exit 12 noch „zur Sicherheit“: Er kann eine zweite E-Mail an den Kunden schicken. Stattdessen `describe` und `sent_on` prüfen.
6. **Exit 11 und 12 nie blind wiederholen.** Erst `hint` lesen, mit `describe` oder `list` nachsehen, dann höchstens den einen Nachholschritt aus dem Hinweis (bei `voucher create --file` das `retry_command`). Ablauf je Befehl: [Exit 11 und 12 Schritt für Schritt](https://github.com/Padrio/papierkram-cli/blob/main/AGENTS.md#exit-11-und-12-schritt-für-schritt).
7. **Exit 6 (429) nie wiederholen**, keine Tokens in Befehlen, `--all` immer mit `--max-pages`. Jede Anfrage kostet Monats-Credits; `credits` steht in `commands --json` und in der `--dry-run`-Ausgabe.

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
PAPIERKRAM_READ_ONLY=1 papierkram payment-term list -o json --no-input --jq '.entries[] | {id, name}'
```

Rechnung anlegen (51 Credits: Angebot lesen 1, Anlegen 50). `--payment-term` ist Pflicht. Auch `--dry-run` liest das Angebot (1 Credit) und zeigt den vollständigen Body:

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

`deliver` läuft genau einmal. Nach Exit 12 oder einer Warnung „Nachlesen fehlgeschlagen“ nur `invoice describe 459` lesen (`sent_on`, `state`, `invoice_no`) und den Menschen entscheiden lassen.

## Ablauf 3: offene Posten prüfen

Alles lesend. Die Statuswerte (`state`) dokumentiert die API nicht vollständig ([offene Punkte](https://github.com/Padrio/papierkram-cli/blob/main/docs/offene-punkte.md)); maßgeblich ist `outstanding_amount`, das die API als Zeichenkette liefert (`"119.0"`): in `--jq` vor jedem Vergleich mit `tonumber` umwandeln, sonst gilt auch `"0.0" > 0`. Entwürfe (`draft`), stornierte (`canceled`) und gelöschte (`deleted`) Rechnungen sind keine offenen Posten, auch wenn `outstanding_amount` stehen bleibt (eine stornierte Rechnung behält ihren Betrag). Nach dem Status filtert die API nicht, nur per `--jq`. Filter der API: `--company`, `--project`, `--from` und `--to` (Belegdatum); alles Weitere per `--jq`:

```sh
PAPIERKRAM_READ_ONLY=1 papierkram invoice list --from 2026-01-01 --to 2026-10-31 --all --max-pages 5 -o json --no-input --jq '.entries[] | select((.outstanding_amount // "0" | tonumber) > 0 and (.state | IN("draft", "canceled", "deleted") | not)) | {id, invoice_no, kunde: .billing.company, due_date, outstanding_amount}'
PAPIERKRAM_READ_ONLY=1 papierkram invoice list --all --max-pages 5 -o json --no-input --jq '(now | strftime("%Y-%m-%d")) as $heute | [.entries[] | select((.outstanding_amount // "0" | tonumber) > 0 and (.state | IN("draft", "canceled", "deleted") | not) and .due_date != null and .due_date < $heute)] | sort_by(.due_date) | .[] | {invoice_no, due_date, outstanding_amount}'
PAPIERKRAM_READ_ONLY=1 papierkram invoice list --company 3214 --all --max-pages 5 -o json --no-input --jq '[.entries[] | select((.state | IN("draft", "canceled", "deleted") | not)) | .outstanding_amount // "0" | tonumber] | add // 0'
PAPIERKRAM_READ_ONLY=1 papierkram invoice describe 459 -o json --no-input
```

Die zweite Zeile listet überfällige Rechnungen (Fälligkeit vor heute, UTC), die dritte summiert die offenen Beträge eines Kunden. `--all` kostet 1 Credit je Seite; bei `has_more: true` im Ergebnis ohne `--all` gibt es weitere Seiten. Meldet stderr, dass `--max-pages` erreicht ist („5 von N Seiten geladen“), sind Liste und Summe unvollständig: das dem Menschen sagen und nicht als Gesamtbetrag ausgeben.

## Ablauf 4: Zeiten erfassen und abrechnen

Standardwerte für ein Verzeichnis (Firma, Projekt, Aufgabe, Benutzer) legt die Projektdatei `.papierkram.toml` fest; `local show` zeigt, was gilt. Aufgaben-Aliase aus `[tasks]` gehen statt IDs. `local init` nur auf Wunsch des Nutzers: Es schreibt die Projektdatei neu (Kommentare gehen verloren) und kennt kein `--dry-run`; deshalb vorher `local show` lesen und die geplante Änderung nennen:

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

Ein Eintrag hat nur ein Datum: Ein Ende genau um Mitternacht geht mit `--to 24:00` (oder `--to 00:00`); andere Einträge über Mitternacht enden mit Exit 2 und werden als zwei Einträge gebucht (bis `24:00`, ab `00:00` am Folgetag mit `--date`).

Timer (lokal, kostet erst beim Stoppen 10 Credits):

```sh
papierkram time start --task tickets -m "Ticket 4711" -o json --no-input
papierkram time status -o json --no-input
papierkram time stop --dry-run -o json --no-input
papierkram time stop -o json --no-input
```

`time stop` nach Exit 11 **nie** erneut ausführen: Der Eintrag ist angelegt. Nach Prüfung per `time list --since heute` den Timer mit `papierkram time discard --yes` verwerfen, nur mit Freigabe (die erfasste Zeit geht verloren).

Abrechnen: offene Zeiten eines Projekts lesen und die abrechenbaren Stunden summieren. Das CLI nimmt `billable_duration` in Sekunden an; bestätigt ist das erst mit der E2E-Messung ([offene Punkte](https://github.com/Padrio/papierkram-cli/blob/main/docs/offene-punkte.md), Nr. 15). Deshalb die Summe mit den Einzeleinträgen vergleichen und dem Menschen vor dem Anlegen der Rechnung nennen:

```sh
PAPIERKRAM_READ_ONLY=1 papierkram time list --project 4963 --state unbilled --all --max-pages 5 -o json --no-input --fields id,started_at,duration,billable_duration,comments
PAPIERKRAM_READ_ONLY=1 papierkram time list --project 4963 --state unbilled --all --max-pages 5 -o json --no-input --jq '([.entries[].billable_duration // 0] | add // 0) / 3600'
```

Aus der bestätigten Summe eine Rechnung anlegen (Kunde, Zahlungsbedingung und Stundensatz kommen vom Menschen) und danach wie in Ablauf 2 nur nach Bestätigung versenden:

```sh
papierkram invoice create --name "Beratung Oktober" --customer 3214 --payment-term 1130 --item "name=Beratung Oktober,qty=12.5,unit=Stunde,price=100,vat=19" --dry-run -o json --no-input
papierkram invoice create --name "Beratung Oktober" --customer 3214 --payment-term 1130 --item "name=Beratung Oktober,qty=12.5,unit=Stunde,price=100,vat=19" -o json --no-input
```

Den Abrechnungsstatus der Zeiteinträge kann das CLI nicht setzen (`papierkram schema time update` hat kein Feld dafür); das bleibt in Papierkram selbst zu erledigen.

## Exit-Codes

| Exit | `code` | Was der Agent tut |
|---|---|---|
| 0 | – | Ergebnis auswerten, `warnings[]` und stderr lesen |
| 1 | `error`, `server_error` | Lesen: später einmal erneut. Änderung: erst per `describe`/`list` prüfen. „Befehl ausgeführt, Ausgabe fehlgeschlagen“: angekommen, nicht wiederholen |
| 2 | `usage` | Befehl nach `Hinweis` korrigieren; nichts gesendet |
| 3 | `auth` | abbrechen, Menschen um `papierkram auth login` bitten |
| 4 | `not_found` | ID und Kontext per `list` prüfen |
| 5 | `validation` | Body nach `message` und `schema` korrigieren; nicht angewendet |
| 6 | `quota` | Kontingent erschöpft: abbrechen, **nicht** wiederholen |
| 7 | `conflict` | Zustand lesen (`describe`, `time status`) |
| 8 | `confirmation_required` | Menschen fragen; `--yes` nur mit Freigabe |
| 9 | `read_only` | abbrechen und melden; Schutz nie umgehen |
| 10 | `network` | später erneut; die Änderung ist nicht angekommen |
| 11 | `partial` | `hint` lesen, höchstens den genannten Nachholschritt ([Details](https://github.com/Padrio/papierkram-cli/blob/main/AGENTS.md#exit-11-und-12-schritt-für-schritt)) |
| 12 | `outcome_unknown` | **nicht** wiederholen: per `describe`/`list` prüfen, ob die Änderung angekommen ist ([Details](https://github.com/Padrio/papierkram-cli/blob/main/AGENTS.md#exit-11-und-12-schritt-für-schritt)) |

Mit `-o json` ist die letzte Zeile von stderr der Fehler als JSON (`error.code`, `error.message`, `error.hint`, `error.exit_code`); siehe [Fehler](https://github.com/Padrio/papierkram-cli/blob/main/AGENTS.md#fehler).
