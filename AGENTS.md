# papierkram für KI-Agents

Diese Datei richtet sich an KI-Agents (Claude Code, Codex und andere), die das CLI `papierkram` bedienen. Sie beschreibt den maschinenlesbaren Vertrag (JSON, Exit-Codes, Fehler) und die Regeln, mit denen ein Agent nichts doppelt versendet, nichts doppelt bucht und das Monatskontingent nicht verbrennt. Für Installation, Kontexte und Projektdatei siehe [README.md](README.md).

## Die wichtigsten Regeln

1. **Schreibschutz als Standard.** Wer einen Agent nur lesen lässt, startet ihn mit `PAPIERKRAM_READ_ONLY=1`. Dann bricht jede Änderung mit Exit 9 ab, und nichts kann das aufheben.
2. **Immer JSON.** `-o json` (oder `--jq`) an jeden Befehl. stdout enthält nur Nutzdaten, stderr nur Hinweise, Warnungen und Fehler.
3. **Nie hängen bleiben.** `--no-input` an jeden Befehl: Dann fragt das CLI nie nach, auch nicht in einem Terminal.
4. **Erst `--dry-run`, dann ausführen.** Vor jeder Änderung den Befehl mit `--dry-run` prüfen, danach unverändert ohne `--dry-run` (und bei Bedarf mit `--yes`) ausführen.
5. **`deliver` nie wiederholen.** Ein zweiter Aufruf kann eine zweite E-Mail an den Kunden schicken.
6. **Exit 11 und 12 nie blind wiederholen.** Erst mit `describe` oder `list` nachsehen, was passiert ist.
7. **429 (Exit 6) nie automatisch wiederholen.** Das Monatskontingent ist erschöpft; ein Mensch muss entscheiden.
8. **Nie Tokens in Befehle.** Kein Token als Argument, in `-f`/`-F`, in Dateien, Logs oder Ausgaben. Das Token kommt aus dem Schlüsselbund des Kontexts oder aus `PAPIERKRAM_TOKEN`, das ein Mensch setzt.

## Selbstbeschreibung

```sh
papierkram commands --json --no-input
papierkram commands --json --jq '.commands[] | select(.class == "destructive") | .command'
papierkram schema invoice create
papierkram schema voucher --jq '.actions | keys'
```

`papierkram commands --json` liefert alle Befehle (Vertrag: [docs/schemas/commands.json](docs/schemas/commands.json), `version` 1): je Befehl `command`, `usage`, `aliases`, `short`, `class`, `credits`, `blocked_by_read_only`, ggf. `operation_id`, `method`, `path`, die Flags (`name`, `shorthand`, `type`, `required`, `alternatives`, `repeatable`, `enum`, `env`, `default_from`, `credits`, `local_filter`, `description`) und `examples`; die globalen Flags stehen einmal unter `global_flags`. Die Klassen:

| `class` | Bedeutung | Bestätigung |
|---|---|---|
| `read_only` | Lesen (list, describe, pdf, info) | nein |
| `create` | Anlegen, Upload, `time log`, `time stop` | nein |
| `update` | Ändern | nein |
| `archive` | archive, unarchive (umkehrbar) | nein |
| `destructive` | delete, cancel, reverse, pay, document delete | ja (`--yes`) |
| `send` | deliver | ja (`--yes`) |
| `local` | wirkt nur lokal (Konfiguration, Projektdatei, Timer); kostet nichts außer einer genannten Operation | nein |
| `per_request` | `papierkram api`: die Klasse folgt aus Methode und Pfad | je nach Anfrage |

`papierkram schema <ressource> [<aktion>]` zeigt Request- und Response-Schema einer Aktion als JSON (aufgelöst, aus der eingebetteten, korrigierten API-Spezifikation); `null` heißt: kein Schema. Beide Befehle senden keine Anfrage und kosten nichts.

## JSON-Vertrag

Alle Formen sind als strikte JSON-Schemas in [docs/schemas/](docs/schemas/) eingecheckt und per Test gegen die echte Ausgabe abgesichert. Jede Abweichung ist ein Fehler des CLI.

| Ausgabe | Form | Schema |
|---|---|---|
| `list` | Envelope der API unverändert: `type`, `page`, `page_size`, `total_pages`, `total_entries`, `has_more`, `entries[]`; mit `--all` alle Einträge zusammengeführt, `page` 1, `has_more` false | – |
| `describe`, `create`, `update`, Aktionen | das Objekt der API; antwortet die API leer: `{"id":N,"ok":true}` (bei `create` `{"ok":true}` mit Warnung) | – |
| `delete` | `{"id":N,"deleted":true}` | [delete.json](docs/schemas/delete.json) |
| `--dry-run` | `{"dry_run":true,"method":…,"url":…,"body":…,"credits":…}`, ggf. mit `upload` und `read` | [dry-run.json](docs/schemas/dry-run.json) |
| `invoice|estimate deliver` | `id`, `invoice_no` bzw. `estimate_no`, `state`, `sent_on`, `sent_via`, `sent_to`; scheitert nur das Nachlesen: `{"id":…,"delivered":true,"warnings":[…]}` | [deliver.json](docs/schemas/deliver.json) |
| `voucher create --file`, Upload gescheitert (Exit 11) | `{"id":…,"completed":["create"],"failed":["upload"],"retry_command":…,"warnings":[…]}` | [partial.json](docs/schemas/partial.json) |
| Fehler | auf **stderr**, letzte Zeile, eine kompakte JSON-Zeile | [error.json](docs/schemas/error.json) |

### Fehler

Bei `-o json`, `-o yaml` oder `--jq` steht der Fehler auf stderr zuerst als Text (`Fehler: …`, `Hinweis: …`, `Correlation-ID: …`) und als letzte Zeile als JSON:

```json
{"error":{"code":"not_found","message":"…","hint":"…","http_status":404,"correlation_id":"…","exit_code":4}}
```

`code` und `exit_code` gehören fest zusammen. `hint`, `http_status` und `correlation_id` fehlen, wenn sie nicht bekannt sind (nie leer oder `null`). Bei Exit 2, 8, 9 und 10 gibt es nie ein `http_status`: Dann wurde nichts gesendet bzw. keine Antwort empfangen. Beginnt `message` mit „Befehl ausgeführt, Ausgabe fehlgeschlagen“ (Exit 1), war die Anfrage erfolgreich und nur die Ausgabe scheiterte (z. B. ein `--jq`-Ausdruck): dann **nicht** wiederholen.

### `--dry-run`

```sh
papierkram invoice create --name "Beratung Oktober" --customer 3214 --payment-term 1130 --item "name=Beratung,qty=2,unit=Stunde,price=100,vat=19" --dry-run -o json
papierkram invoice create --from-estimate 349 --payment-term 1130 --dry-run -o json
papierkram voucher create --name "Büromaterial" --provenance domestic --creditor 2904 --item "name=Papier,amount=11.90,vat=19,category=Bürobedarf" --file beleg.pdf --dry-run -o json
```

- Gilt für jede Änderung, auch `papierkram api` mit anderen Methoden als GET. Das CLI prüft Flags, Argumente und den Body gegen das Request-Schema (Fehler: Exit 2 oder 5), sendet nichts und endet mit Exit 0.
- `credits` sind die Kosten des echten Aufrufs, alle Schritte zusammen (Anlegen und Upload, Versand und Nachlesen).
- `upload` nennt Dateiname, Größe und MIME-Typ; ist das Hochladen ein zweiter Schritt (`voucher create --file`), steht in `upload.url` eine Vorlage mit Platzhalter für die ID.
- `read` nennt eine Leseanfrage, die schon gesendet wurde: `invoice create --from-estimate` liest das Angebot auch bei `--dry-run` (1 Credit, verbraucht).
- Unter Schreibschutz (`PAPIERKRAM_READ_ONLY` oder `read_only` im Kontext) endet auch `--dry-run` einer Änderung mit Exit 9.

### Bestätigungen: `--yes` und `--no-input`

- `destructive` und `send` brauchen eine Bestätigung. Ohne Terminal oder mit `--no-input` bricht der Befehl ohne `--yes` mit Exit 8 ab, bevor etwas gesendet wird.
- `--yes` nur verwenden, wenn der Mensch genau diese Aktion freigegeben hat. Exit 8 heißt: nichts passiert, nachfragen.
- `create`, `update` und `archive` laufen ohne Rückfrage. `time log` und `time stop` fragen nur im Terminal (Enter = ja).
- `--all` und `company list --type` warnen ab 10 Seiten auf stderr; ohne Terminal laden sie trotzdem alle Seiten. Mit `--max-pages` begrenzen.

## Exit-Codes und was zu tun ist

| Exit | `code` | Bedeutung | Was der Agent tut |
|---|---|---|---|
| 0 | – | Erfolg, auch `--dry-run` | Ergebnis auswerten; `warnings[]` und stderr lesen |
| 1 | `error`, `server_error` | unerwarteter Fehler, 5xx | Lesen: später einmal erneut (GET hat das CLI schon zweimal wiederholt). Änderung: erst per `describe`/`list` prüfen, ob sie angekommen ist. „Befehl ausgeführt, Ausgabe fehlgeschlagen“: angekommen, nicht wiederholen |
| 2 | `usage` | falsche Flags oder Argumente; nichts gesendet | Befehl nach `Hinweis` korrigieren (`--help`, `commands --json`) |
| 3 | `auth` | kein Token, 401, 403 | abbrechen, Menschen bitten (`papierkram auth login`); nie selbst nach Tokens suchen |
| 4 | `not_found` | 404 | ID und Kontext prüfen (`list`); nicht raten |
| 5 | `validation` | lokale Prüfung, 400, 422; nicht angewendet | Body nach `message` korrigieren (`schema <ressource> <aktion>`), Datum `YYYY-MM-DD`, Steuersatz `0.19` |
| 6 | `quota` | 429, Monatskontingent erschöpft | **nicht wiederholen**, abbrechen und melden |
| 7 | `conflict` | 409 oder Timer läuft schon | Zustand lesen (`describe`, `time status`) |
| 8 | `confirmation_required` | Bestätigung fehlt; nichts gesendet | Menschen fragen; `--yes` nur mit Freigabe |
| 9 | `read_only` | Schreibschutz; nichts gesendet | abbrechen und melden; den Schutz nie umgehen (kein anderer Kontext, keine Variable ändern) |
| 10 | `network` | Netzwerkfehler beim Lesen oder **bevor** eine Änderung gesendet wurde | später erneut versuchen; die Änderung ist nicht angekommen |
| 11 | `partial` | mehrstufiger Befehl teilweise ausgeführt | **nur** `retry_command` bzw. den genannten nächsten Schritt ausführen, nie den ganzen Befehl |
| 12 | `outcome_unknown` | Verbindung brach ab, **nachdem** eine Änderung gesendet wurde | **nicht wiederholen**: mit `describe`/`list` prüfen, ob die Änderung angekommen ist |

## Verbotene Muster

- **`deliver` wiederholen.** Auch nach Exit 0 mit Warnung („Versand erfolgreich, Nachlesen fehlgeschlagen … NICHT erneut ausführen“) und nach Exit 12. Stattdessen `invoice describe <id>` bzw. `estimate describe <id>` und `sent_on`, `state` und die Belegnummer prüfen.
- **Exit 11 oder 12 blind wiederholen.** `voucher create --file` legt bei einer Wiederholung einen zweiten Beleg an (50 Credits); nach Exit 11 nur `retry_command` ausführen. Nach Exit 12 erst `describe`/`list`.
- **429 automatisch wiederholen.** Exit 6 heißt: Kontingent für den Monat aufgebraucht. Jede weitere Anfrage scheitert ebenso.
- **Tokens in Befehlen.** Nie `PAPIERKRAM_TOKEN=… papierkram …` mit echtem Wert in Befehlszeilen, Skripten oder Logs, nie Tokens per `api -f`/`-F` oder in `--input`-Dateien.
- **Den Schreibschutz umgehen.** Kein Wechsel auf einen anderen Kontext, kein Ändern von `PAPIERKRAM_READ_ONLY`, keine Konfiguration bearbeiten, um Exit 9 loszuwerden.
- **`deliver --via email` ohne ausdrückliche Vorgabe.** Empfänger, Betreff und Text kommen vom Menschen, nicht vom Agent.
- **`--yes` aus Gewohnheit.** `--yes` ist die Freigabe eines Menschen für genau diese Aktion.

## Wiederholbarkeit (Idempotenz)

| Aktion | Wiederholung | Vor einer Wiederholung prüfen |
|---|---|---|
| `list`, `describe`, `pdf`, `info` | unbedenklich, kostet aber Credits | – |
| `update` | gleicher Body ergibt den gleichen Zustand (Teil-Update: nur die gesendeten Felder) | nach Exit 12 `describe` |
| `archive`, `unarchive` | Zielzustand; eine Wiederholung ändert nichts mehr oder wird abgelehnt | `describe` |
| `delete` | nach Erfolg liefert die Wiederholung Exit 4 | nach Exit 12: `describe` mit Exit 4 heißt gelöscht |
| `create`, `time log`, `upload` | **nicht** idempotent: jeder Aufruf legt ein neues Objekt an | nach Exit 12 per `list` suchen (Name, Datum) |
| `voucher create --file` | nicht wiederholen | nach Exit 11 nur `retry_command` |
| `invoice create --from-estimate` | wie `create`; scheitert das Lesen des Angebots, wurde nichts angelegt | `invoice list` |
| `time stop` | der Timer wird erst nach dem Anlegen gelöscht | nach Exit 12 `time list --since heute`; war der Eintrag angelegt, `time discard` statt erneut `time stop` |
| `deliver` | **nie** | `describe`: `sent_on`, `state`, Nummer |
| `pay` | **nie**; eine Zahlung lässt sich per API nicht zurücknehmen | `voucher describe` |
| `cancel`, `reverse` | Statuswechsel; eine Wiederholung lehnt die API in der Regel ab (422, Exit 5) | `describe` |

## Empfohlene Abläufe

Lesen (unter Schreibschutz):

```sh
PAPIERKRAM_READ_ONLY=1 papierkram invoice list --from 2026-10-01 --to 2026-10-31 --all --max-pages 5 -o json --no-input
PAPIERKRAM_READ_ONLY=1 papierkram invoice describe 459 -o json --no-input
PAPIERKRAM_READ_ONLY=1 papierkram time list --since 2026-10-01 --until 2026-10-31 --fields id,started_at,duration,comments -o json --no-input
```

Ändern (nur mit Freigabe, ohne Schreibschutz):

```sh
papierkram invoice update 459 --description "Leistungszeitraum Oktober" --dry-run -o json --no-input
papierkram invoice update 459 --description "Leistungszeitraum Oktober" -o json --no-input
papierkram invoice describe 459 -o json | papierkram invoice update 459 --input - --dry-run -o json --no-input
papierkram invoice deliver 459 --via pdf --dry-run -o json --no-input
papierkram invoice deliver 459 --via pdf --yes -o json --no-input
```

Fehler auswerten: Exit-Code lesen, bei Bedarf die letzte stderr-Zeile als JSON parsen (`error.code`, `error.hint`).

## Credits

Papierkram berechnet Monats-Credits; danach 429 (Exit 6). `credits` in `commands --json` und in der `--dry-run`-Ausgabe nennt die Kosten. Faustregeln: Rechnung, Angebot, Beleg anlegen 50; anderes Anlegen und Beleg-Upload 10; alles andere 1 je Anfrage (Listen je Seite). `deliver` kostet 2 (Versand und Nachlesen), `voucher create --file` 60, `invoice create --from-estimate` 51, `auth status` 1. `--all` kostet 1 je Seite; immer mit `--max-pages` begrenzen. Lokale Befehle (`commands`, `schema`, `context`, `local`, `version`) kosten nichts. Fällt das Restkontingent unter 100, steht eine Warnung auf stderr. `papierkram auth status -o json` zeigt das Restkontingent unter `quota` (1 Credit).

## Weitere Hinweise

- Kontexte: `--context <name>` wählt einen Kontext; `papierkram auth status -o json` zeigt den wirksamen Kontext, Token-Quelle und Schreibschutz (`read_only`, `read_only_source`).
- IDs statt Namen: Flags nehmen IDs (`--customer 3214`), keine Namen. IDs liefert `list`.
- Steuersätze: `vat=19`, `19%`, `0.19` und `0,19` ergeben 0.19.
- Positionen ändern: `update` mit `--item`/`--items` verlangt `--replace-items` und ersetzt **alle** Positionen.
- Befehlsreferenz: [docs/reference/](docs/reference/); offene Fragen zur API: [docs/offene-punkte.md](docs/offene-punkte.md).
