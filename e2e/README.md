# Live-E2E-Suite (nur Sandbox)

Diese Suite prüft das CLI gegen die echte Papierkram-API (Spezifikation 11.1)
und misst die offenen Punkte aus Abschnitt 17. Sie läuft **nur** mit dem
Build-Tag `e2e` und nur, wenn alle drei Variablen gesetzt sind:

| Variable | Inhalt |
|---|---|
| `PAPIERKRAM_E2E_SUBDOMAIN` | Subdomain des **Sandbox**-Accounts |
| `PAPIERKRAM_E2E_TOKEN` | API-Token dieses Accounts |
| `PAPIERKRAM_E2E_SANDBOX_CONFIRM` | noch einmal die Subdomain, Zeichen für Zeichen gleich |

Fehlt eine davon oder weicht die Bestätigung ab, meldet jeder Test `SKIP`,
und es geht keine einzige Anfrage hinaus. So läuft die Suite auch in der
normalen CI mit (ohne Variablen, nur Kompilieren und Überspringen).

## Verantwortung für die Sandbox

Ob der Account wirklich eine Sandbox ist, kann die Suite nicht prüfen. Das
liegt bei dir: Trage nie Subdomain und Token der echten Buchhaltung ein.

## Was die Suite anlegt

Die Suite legt Firmen, Kontaktpersonen, Projekte, Aufgaben, Zeiteinträge
(darunter zwei von heute aus `TestE2E_TimeFlow`: einen per `time log`, einen
nicht abrechenbaren per `time stop` aus dem lokalen Timer), Artikel, ein
Datenfeld, zwei Angebote, zwei Rechnungen und zwei Belege an, lädt Dateien
hoch, versendet ein Angebot und eine Rechnung als PDF und versucht, beide zu
stornieren und einen Beleg zu stornieren und gegenzubuchen (ob die API das
zulässt, wird gemessen).
`TestE2E_MasterDataArchive` archiviert eigene Stammdaten (Lieferant, Projekt,
Aufgabe, Artikel, Datenfeld) und ein eigenes Angebot und holt sie gleich
danach wieder aus dem Archiv, sofern `archive` sie archiviert haben kann (Exit
0, 1 oder 12; nach einer Ablehnung kein `unarchive`, nach 429 wird der
Untertest übersprungen). Scheitert `unarchive` oder entfällt es nach einer
Ablehnung von `archive`, steht „Fixture <ressource> <id> womöglich noch
archiviert – Folgefehler möglich“ im Log. Bankverbindungen und Bankumsätze liest die Suite
nur.

**Dauerkunde.** Das versendete Angebot und die versendete Rechnung bekommen
eine Nummer; solche Belege kann die API nicht löschen, und eine Firma mit
Belegen auch nicht. Deshalb buchen beide auf eine feste Firma
`pk-e2e-dauerkunde`, das einzige Objekt der Suite ohne Lauf-ID. Jeder Lauf
sucht sie per Name (die Firmenliste von `TestE2E_CleanupLeftovers`, sonst eine
eigene), holt sie aus dem Archiv (1 Credit), nutzt sie und archiviert sie am
Ende wieder (1 Credit); gibt es sie noch nicht, legt der erste Lauf sie an
(10 Credits). Gelöscht wird sie nie, und die Restesuche fasst sie nicht an.
Gibt es mehr als eine Firma dieses Namens, scheitert der erste Test, der sie
braucht („mehrere Dauerkunden“), statt eine zu wählen: dann bis auf eine von
Hand umbenennen. Das Angebot hat keine Kontaktperson und kein Projekt (die
gehören zum Kunden des Laufs). Alle anderen Flows nutzen den Kunden des Laufs
(`pk-e2e-<lauf-id>-kunde`), der so löschbar bleibt.

**Nur archiviert.** Der Dauerkunde, das versendete Angebot, die versendete
Rechnung und deren Storno-Rechnung werden beim Aufräumen nur archiviert
(1 Credit), nie gelöscht oder storniert: Jeder weitere Versuch kostete Credits
ohne Wirkung. Ebenso archiviert die Restesuche Rechnungen und Angebote
früherer Läufe, die eine Nummer haben (`invoice_no` bzw. `estimate_no` nicht
`null`). Bereits archivierte Reste fasst sie gar nicht mehr an; das Log zählt
sie getrennt: `Reste früherer Läufe: <n> entfernt, dauerhaft archiviert: <n>,
übrig: <n>`. Diese Reste wachsen je Lauf um die beiden versendeten Belege und
die Storno-Rechnung, aber um keine Firma mehr.

**Storno-Rechnung.** `invoice cancel` legt eine Storno-Rechnung an (gemessen
Live-Lauf 6, siehe unten). `TestE2E_InvoiceFlow` sucht sie danach in der Rechnungsliste des
Dauerkunden von heute, Berliner Tag (`invoice list --company <id> --from
<heute> --to <heute> --sort id --desc --limit 100`; lehnt die API die
Sortierung ab, noch einmal ohne, 1 Credit mehr): Kandidaten sind Rechnungen
mit höherer ID als die stornierte, die dieser Lauf nicht selbst registriert
hat. Je Kandidat stehen Name, Status, Nummer, `type` und Schlüsselform als
`MESSUNG` im Log. Genau ein Kandidat mit `type` `invoice` wird als
Folgeobjekt der stornierten Rechnung registriert und am Ende archiviert;
mehrere (etwa durch einen parallelen Lauf) sind eine `ABWEICHUNG`, und
keiner wird angefasst. Die Annahmen dieser Suche (eigenes Objekt, gleiche
Firma, Belegdatum heute, steigende IDs, keine fremde Rechnung, Sortierung
nach `id`, archivierbar, Name ohne `pk-e2e-`) und was Live-Lauf 6 davon
bestätigt oder widerlegt hat, stehen in
[offener Punkt 31](../docs/offene-punkte.md#31-storno-rechnung-nach-invoice-cancel).

Endet `archive` bei einem dieser nur zu archivierenden Objekte mit einem
Exit-Code ungleich 0, liest das Aufräumen es einmal (`describe`, 1 Credit);
ist es danach archiviert (`record_state` `archived`), gilt es als
aufgeräumt.

## Ausführen

Lokal:

```sh
PAPIERKRAM_E2E_SUBDOMAIN=meine-sandbox \
PAPIERKRAM_E2E_TOKEN=… \
PAPIERKRAM_E2E_SANDBOX_CONFIRM=meine-sandbox \
go test -count=1 -tags e2e -v ./e2e/...
```

In GitHub: Workflow „E2E (Sandbox)“ manuell starten (`workflow_dispatch`) und
im Eingabefeld die Subdomain der Sandbox zur Bestätigung eingeben. Der Job
läuft nur auf `main` (`if: github.ref == 'refs/heads/main'`); von einem
anderen Branch gestartet, wird er übersprungen. Er nutzt die GitHub-Umgebung
`sandbox` (`environment: sandbox`): Subdomain und Token gehören dort als
Secrets `PAPIERKRAM_E2E_SUBDOMAIN` und `PAPIERKRAM_E2E_TOKEN` hinein. Umgebung,
Secrets und Schutzregeln richtet der Repository-Inhaber in den
Repository-Einstellungen ein; das Repository selbst legt sie nicht an.

Ablauf des Workflows:

1. `confirmation matches the sandbox`: Die Eingabe muss genau dem Secret
   `PAPIERKRAM_E2E_SUBDOMAIN` entsprechen, sonst bricht der Job ab. Enthält
   das Secret einen Zeilenumbruch oder Leerraum, nennt die Meldung das.
2. `live suite`: `go test -count=1 -timeout 25m -tags e2e -v ./e2e/...`,
   die Ausgabe geht per `tee` auch nach `e2e.log` (`set -o pipefail`: der
   Exit-Code der Suite zählt).
3. `token check` (läuft immer): Das Repository und seine Artefakte sind
   öffentlich. Der Schritt sucht deshalb zuerst das Token in `e2e.log`,
   ersetzt dann die Sandbox-Subdomain durch `<sandbox>` (fester Text, jede
   Groß-/Kleinschreibung; der Wert kommt aus der Umgebung, nie als
   Argument) und sucht danach Token und Subdomain erneut. Die erste Suche
   findet auch ein Token, das die Subdomain enthält (das Ersetzen würde es
   zerschneiden). Beide gehen `grep` als Musterdatei zu, nie als Argument;
   Leerraum und Zeilenumbrüche am Anfang und Ende des Token-Secrets zählen
   nicht zum Token.
   Ein Treffer löscht das Log und lässt den Schritt scheitern („Token im
   Log gefunden: Log gelöscht, kein Upload“ bzw. „Subdomain im Log
   gefunden: …“); ebenso ein leeres Secret („Token-Secret leer: Log
   gelöscht, kein Upload“ bzw. „Subdomain-Secret leer: …“), ein Token-Secret
   mit Zeilenumbruch im Inneren und ein Fehler von `grep` oder `sed`, weil
   sich dann nichts sicher prüfen lässt. Ohne sauberes Log gibt es weder
   Zusammenfassung noch Artefakt.
4. `summary`: nur bei sauberem Log. Schreibt unter „E2E-Ergebnis“ alle
   Zeilen mit `ABDECKUNG`, `ABWEICHUNG` oder `Credits:` in die
   Zusammenfassung des Laufs, in einem Codeblock, dessen Begrenzung länger
   ist als jede Folge von Backticks in diesen Zeilen (keine Logzeile
   beendet ihn).
5. `log artifact`: nur bei sauberem Log. Lädt `e2e.log` als Artefakt
   `e2e-log` hoch; es bleibt **14 Tage** erhalten.

**Wichtig:** Secrets `PAPIERKRAM_E2E_*` dürfen **nur** in der Umgebung
`sandbox` liegen, **nie** als Repository-Secrets. Die Umgebung `sandbox`
braucht eine Branch-Regel (Deployment-Branch nur `main`), sinnvollerweise
auch eine Pflicht-Freigabe. Grund: `workflow_dispatch` führt die
Workflow-Datei des gewählten Branches aus. Ein Branch kann `if:` und
`environment:` entfernen; an Umgebungs-Secrets mit Branch-Regel kommt er
trotzdem nicht heran, an Repository-Secrets schon.

Ohne die Variablen (Nachweis, dass alles überspringt):

```sh
go vet -tags e2e ./e2e/...
go test -tags e2e -v ./e2e/...
```

Nachweis, dass ohne Freigabe kein Request hinausgeht: das Testbinary ohne
jeden Netzzugang laufen lassen. Jeder Test muss `SKIP` melden, der Lauf mit
`PASS` enden.

```sh
go test -c -tags e2e -o /tmp/e2e.test ./e2e

# macOS: Sandbox ohne Netz (Gegenprobe: curl scheitert darin)
sandbox-exec -p '(version 1)(allow default)(deny network*)' /tmp/e2e.test -test.v

# Linux: eigener Netz-Namespace ohne Schnittstellen
unshare -rn /tmp/e2e.test -test.v
```

Ein gesetzter Dead-Proxy (`HTTPS_PROXY=http://127.0.0.1:9`) taugt nicht als
Nachweis: Die Suite ignoriert Proxy-Variablen absichtlich.

## Sicherheitsregeln

- Das CLI läuft im Test mit eigener Umgebung: nur Subdomain, Token und
  temporäre Verzeichnisse für Config und Cache. Deine Config, dein
  Schlüsselbund und andere `PAPIERKRAM_*`-Variablen werden nie gelesen.
- Anfragen gehen nur an `https://<subdomain>.papierkram.de`; alles andere
  lehnt der Transport der Suite ab. Er geht direkt, ohne Proxy:
  `HTTPS_PROXY`, `HTTP_PROXY` und `NO_PROXY` der Shell werden ignoriert.
- `deliver` geht nur mit `--via pdf` hinaus. Eine E-Mail verschickt die
  Suite nie; auch das erzwingt der Transport (`send_via` genau `"pdf"`,
  kein `email`-Schlüssel).
- Alles, was die Suite anlegt, heißt `pk-e2e-<lauf-id>-…` (Lauf-ID:
  `GITHUB_RUN_ID`, bei einer Wiederholung mit Versuchsnummer, sonst eine
  Zufallszahl), einzige Ausnahme ist der Dauerkunde `pk-e2e-dauerkunde`
  (siehe oben). Aufgeräumt werden nur Objekte,
  deren Name mit `pk-e2e-` beginnt: am Ende des Laufs die eigenen, im Test
  `TestE2E_CleanupLeftovers` die Reste früherer Läufe. Andere Objekte
  fasst die Suite nie an (Lesen ausgenommen); einzige Ausnahme sind
  Folgeobjekte eines Stornos oder einer Gegenbuchung (siehe unten).
- Was sich nicht löschen lässt, wird storniert bzw. archiviert; was eine
  Nummer hat (ein versendetes Angebot, eine versendete Rechnung, die
  Storno-Rechnung) und der Dauerkunde werden nur archiviert. Rechnungen,
  Angebote und Belege werden als Entwurf angelegt; versendet werden nur ein
  eigenes Angebot und eine eigene Rechnung (`TestE2E_InvoiceFlow`), beide
  nur als PDF und an den Dauerkunden; danach versucht die Suite, beide zu
  stornieren. Die Rechnung erhält dabei eine Nummer aus dem
  Rechnungsnummernkreis der Sandbox. Einen eigenen Beleg bucht
  `TestE2E_VoucherFlow` gegen (`reverse`) und storniert ihn; er gehört zu
  einem eigenen Lieferanten (`pk-e2e-<runid>-lieferant-storno`), der danach
  womöglich nur archiviert werden kann, damit der gemeinsame Lieferant
  löschbar bleibt (`DeleteCompany` in der Abdeckung).
- Nennt die Antwort auf `cancel` oder `reverse` ein anderes Objekt als das
  stornierte (ein Folgeobjekt mit eigener ID, z. B. eine Gegenbuchung), steht
  das als `ABWEICHUNG` im Log. Hat es denselben Typ (`type` der Antwort)
  wie das stornierte Objekt, entfernt das Aufräumen genau dieses Objekt mit,
  auch wenn die API ihm einen Namen ohne `pk-e2e-` gegeben hat: Seine ID
  stammt unverändert aus dieser Antwort, und das stornierte Objekt hat
  dieser Lauf selbst angelegt. Ein Folgeobjekt wird nur gelöscht oder
  archiviert, nie storniert. Ein Folgeobjekt anderen Typs fasst die Suite
  nicht an. Ebenso Folgeobjekt ist die Storno-Rechnung, die
  `TestE2E_InvoiceFlow` als einzigen neuen Kandidaten in der Rechnungsliste
  des Dauerkunden findet (siehe oben); sie wird nur archiviert.
- Die Suite bezahlt nie etwas: `voucher pay` wird nur mit `value: null` und
  der Bankumsatz-ID 999999999 gemessen, die es nicht gibt (ohne Betrag). Der
  Transport lässt für `pay` genau diesen Body durch,
  `{"value": null, "banking_transaction": {"id": 999999999}}`, und lehnt
  alles andere vor dem Senden ab: einen Betrag, eine andere Bankumsatz-ID,
  einen Body ohne `value`, `{}`, zusätzliche oder doppelte Schlüssel. Echte
  Bankumsätze bleiben unberührt.
- Für `deliver` und `pay` lehnt der Transport außerdem jeden Body ab, in dem
  ein Schlüssel doppelt vorkommt oder sich nur in Groß-/Kleinschreibung von
  einem anderen unterscheidet, auf jeder Ebene.
- Nennt ein Pfadsegment in irgendeiner Form `pay` oder `deliver` (groß/klein,
  auch mehrfach kodiert, mit `.json` oder `;…`, getrennt durch `/` oder `\`),
  geht die Anfrage nur in genau einer Form hinaus: `POST` auf
  `/api/v1/expense/vouchers/<id>/pay` bzw.
  `/api/v1/income/{invoices,estimates}/<id>/deliver`, ohne Query (auch kein
  leeres `?`), mit genau einem `Content-Type: application/json` (höchstens
  `; charset=utf-8`), ohne `Content-Encoding` und ohne Methoden-Override
  (Header wie `X-HTTP-Method-Override`, `_method` in Query oder Body).
  Header-Namen zählen in jeder Schreibweise. Ein Pfad, der sich nicht
  eindeutig beurteilen lässt (Steuerzeichen wie NUL, Kodierung über mehr als
  acht Stufen), gilt ebenfalls als geschützt. Alles andere wird vor dem
  Senden abgelehnt, z. B.
  `papierkram api POST '/expense/vouchers/<id>/pay?value=5' --input -`.
- Bankdaten (Bankverbindungen, Bankumsätze) stehen nie mit Werten im Log,
  auch nicht ihre IDs: `TestE2E_ReadCommands` protokolliert nur die Anzahl
  (`total_entries`) und die Schlüsselform des ersten Eintrags (nur die
  Schlüssel, ohne Werte). Scheitert ein Lesen, nennt die Meldung nur den
  Exit-Code und den Fehlercode des CLI, nie die Ausgabe.
- Das Token erscheint nie in der Testausgabe: Jede Logzeile wird vorher
  geschwärzt und danach auf Reste geprüft; die CLI-Ausgaben werden auf das
  Token geprüft.

## Credit-Budget

Ein voller Lauf kostet nach den dokumentierten Preisen (Abschnitt 4.3) etwa
**645 Credits** (Tests 622, Aufräumen 23; jede Zeile im Kopf von
`e2e_test.go` gehört zu einem vorhandenen Test, keine ist mehr geplant); das
Budget je Lauf ist **760** (Zielwert unter 800). Die Aufschlüsselung steht
im Kopf von `e2e_test.go`; die Tests im Überblick (Credits eines vollen
Laufs ohne Reste; eine Vorbedingung zählt beim ersten Test, der sie nutzt):

| Test | Credits |
|---|---:|
| `TestE2E_CleanupLeftovers` (je Ressource 1 Seite je 100 Einträge, mehr mit Resten) | mindestens 9 |
| `TestE2E_QuotaHeadersPresent`, `TestE2E_InfoCreditCost` | 4 |
| `TestE2E_DeliverPdfEmptyBody` (Dauerkunde entarchivieren 1, Angebot 50, Versand 1, Nachlesen 1) | 53 |
| `TestE2E_VatRate019Is19Percent` (Kunde, Person, Projekt 30, Angebot 50, Artikel 10, 2 × describe) | 92 |
| `TestE2E_PersonDefaultIsString` | 1 |
| `TestE2E_UploadMimeTypes` | 100 |
| `TestE2E_VoucherStateValues` | 3 |
| `TestE2E_PartialUpdateSemantics` | 114 |
| `TestE2E_ReplaceLineItems` | 4 |
| `TestE2E_InvoiceFlow` (bisher 60, dazu `paid-status` 1 und Storno-Suche 1) | 62 |
| `TestE2E_MasterDataArchive` | 12 |
| `TestE2E_ReadCommands` | höchstens 8 |
| `TestE2E_MCPReadTool` (`user_list`) | 1 |
| `TestE2E_PaidStatus` (Firmenliste 1, Rechnungsliste 1, Rechnungsliste mit Name aus dem Zwischenspeicher 1) | 3 |
| `TestE2E_TimeFlow` | 24 |
| `TestE2E_VoucherFlow` | 79 |
| `TestE2E_Carries` | 41 |
| `TestE2E_OpenMeasurements` | 12 |
| **Tests** | **622** |
| Aufräumen: 4 Entwurfsbelege und 13 andere Objekte je 1 `delete` (17), Lieferant aus `TestE2E_VoucherFlow` und Kunde des Laufs ggf. je 1 `archive` (2), Dauerkunde, versendetes Angebot, versendete Rechnung und Storno-Rechnung je 1 `archive` (4) | 23 |
| **Summe** | **645** |

Im ersten Lauf einer Sandbox legt die Suite den Dauerkunden an (10 statt 1
Credit): 654. Jede gesendete Anfrage
zählt, auch eine abgelehnte (4xx/5xx): Die API berechnet z. B. ein mit 422
abgelehntes Anlegen voll (gemessen 2026-11-04). Gezählt wird der größere
Wert aus den dokumentierten Kosten und `X-Consumed-Quota`, das die Kosten
genau dieser einen Anfrage nennt (kein laufender Zähler); fehlt der Header
oder kommt keine Antwort (Netzwerkfehler), zählen die dokumentierten
Kosten.

Vom Budget sind **60 Credits fürs Aufräumen reserviert** (dokumentierter
Höchstaufwand: je Entwurfsbeleg 4, je anderem Objekt 2, plus 10 Puffer; bei
4 Entwurfsbelegen und 17 anderen Objekten, darunter die 4, die nur archiviert
werden). Die Tests dürfen also höchstens 700 Credits verbrauchen (geplant:
622): Eine
Anfrage, die in die Reserve greifen würde, wird nicht gesendet, und reicht das
Budget für einen Test nicht mehr, werden dieser und alle folgenden Tests mit
Begründung übersprungen. Erst das Aufräumen am Ende darf die Reserve nutzen.
Entstehen beim Stornieren Folgeobjekte, die die Antwort selbst nennt (nicht
erwartet), kostet jedes beim Aufräumen bis zu 2 Credits mehr, bei den 4
Stornos und Gegenbuchungen der Suite zusammen höchstens 8 aus dem Puffer der
Reserve.
Am Ende steht die Summe im Log.

Antwortet die API mit 429 (Exit 6, Monatskontingent erschöpft), sendet die
Suite in den Tests keine weitere Anfrage: Alle folgenden Tests werden
übersprungen (einmal im Log: `e2e: Kontingent erschöpft (HTTP 429): weitere
Tests übersprungen`), das Abdeckungs-Gate entfällt. Das Aufräumen am Ende
versucht es trotzdem; antwortet auch dort die API mit 429, bricht es ohne
weitere Anfrage ab und nennt die übrigen Objekte (sie räumt
`TestE2E_CleanupLeftovers` des nächsten Laufs auf). Ein solcher Lauf endet
immer mit Exit 1 und wird nicht automatisch wiederholt.

## Abdeckung

Der Transport der Suite ordnet jede beantwortete Anfrage über ihren Pfad
einer der 89 Operationen der API zu (`internal/e2eguard/coverage.go`).
Abgelehnte Anfragen (fremder Host, Budget, unzulässiger `pay`- oder
`deliver`-Body) und Anfragen ohne Antwort zählen nicht. Was ein Lauf
abdecken muss, steht in `e2eguard.CoveragePlan`: je Operation der Test, der
sie zuerst trifft, ob eine 2xx-Antwort nötig ist oder jede Antwort genügt
(z. B. `voucher pay` ohne echte Zahlung, 4xx erwartet) oder nur bei
vorhandenen Daten (Bankverbindung, Umsatz), ob die Aufräumphase zählt (nur
`delete`) und welche spätere Task sie erst abdeckt (derzeit keine: jede
Operation ist erwartet, geprüft von `TestCoveragePlanComplete`).

Nach dem Aufräumen stehen im Log diese Zeilen (jeweils mit `e2e: ` davor):

```text
ABDECKUNG: abgedeckt X/89 (davon A nur mit Fehlerantwort, C nur im Aufräumen), nicht anwendbar N, ausstehend P, fehlend M
ABDECKUNG fehlt: <OpID> (erwartet von <Test>)
ABDECKUNG nicht anwendbar: <OpID>: <Grund>
ABDECKUNG ausstehend: <OpID> (Task <N>)
ABDECKUNG unbekannter Pfad: <METHOD> <path>
```

Hat der Account keine Bankverbindung bzw. keinen Bankumsatz, meldet
`TestE2E_ReadCommands` das Lesen per ID als nicht anwendbar, z. B.
`ABDECKUNG nicht anwendbar: GetBankConnection: keine Bankverbindung im
Sandbox-Account (gemessen)`; das entschuldigt nur diese beiden Operationen.

Gate: Fehlt eine erwartete Operation, endet der Lauf mit Exit 1 und
`e2e: Abdeckung unvollständig: <n> Operation(en) fehlen`, aber nur bei einem
vollständigen Lauf: kein `-run`, kein `-skip` und kein `-list`, Budget und
Kontingent nicht erschöpft und bis dahin kein Fehler. Sonst stehen nur die
Zeilen im Log. Ausstehende Operationen lassen den Lauf nie scheitern.

## Bewusst nicht gemessen

- Die maximale Dateigröße beim Upload (Spezifikation 17) bleibt bewusst
  offen: Sie zu messen hieße, große Dateien in einen echten Account
  hochzuladen. Der Punkt steht in den offenen Punkten der Doku.
- `voucher pay` mit Betrag und Bankumsatz zusammen (siehe oben: die Suite
  bezahlt nie).

## Messungen

Ergebnisse stehen im Log mit dem Präfix `MESSUNG`. Hart fehlschlagen nur
Prüfungen, auf deren Ergebnis sich das CLI verlässt; reine Messungen
(z. B. erlaubte Upload-Typen, Statuswerte, Kosten und Struktur von
`GET /info`, Einheiten der Zahlungsbedingungen) schlagen nicht fehl.
Weicht eine Messung von dem ab, was das CLI annimmt (z. B. andere Kosten
für `GET /info` als in der Registry), steht sie mit `ABWEICHUNG:` im Log und
am Ende des Laufs noch einmal in einer Zusammenfassung. Welche Prüfung zu welcher Gruppe gehört, steht im Kopf
von `e2e_test.go`.

`TestE2E_CleanupLeftovers` prüft nebenbei die erste Seite jeder Liste, die es
ohnehin lädt, gegen den Vertrag
[docs/schemas/list.json](../docs/schemas/list.json) (kein Credit mehr); eine
Verletzung steht als `ABWEICHUNG` mit JSON-Pfad und Regel im Log, nie mit
Werten der Einträge.

`TestE2E_InvoiceFlow` ruft nach einem erfolgreichen Versand (Exit 0)
`invoice paid-status --customer <Dauerkunde> --from <heute>` auf (Berliner
Tag: eine Seite, egal wie viele Läufe auf den Dauerkunden gebucht haben;
hart: Exit 0 und Vertrag
[docs/schemas/paid-status.json](../docs/schemas/paid-status.json)) und misst
offenen Punkt 30: ob die versendete Rechnung ein offener Posten ist (Status,
`outstanding_amount`) und wie viele Einträge der Rechnungsliste
`outstanding_amount` tragen (Ergebnisse von Live-Lauf 6:
[offener Punkt 30](../docs/offene-punkte.md#30-outstanding_amount-in-der-rechnungsliste-und-offene-posten)). Fehlt die Rechnung in den offenen Posten, steht
das als `ABWEICHUNG` im Log. Nach dem Storno sucht er die Storno-Rechnung
(offener Punkt 31, siehe [Was die Suite anlegt](#was-die-suite-anlegt)).

`TestE2E_PaidStatus` prüft die Namensauflösung an einem Flag (3 Credits):
`invoice paid-status --customer pk-e2e-dauerkunde --from <heute>` muss mit Exit 0 enden,
den Vertrag erfüllen, auf stderr die ID nennen (`→ <id> (`) und die
Firmenliste sowie die Rechnungsliste mit `company_id=<id>` laden; derselbe
Aufruf mit `--overdue` im selben Verzeichnis muss den Namen aus dem
Zwischenspeicher nehmen (keine Firmenliste, „aus dem Zwischenspeicher“ auf
stderr). `count`, `overdue_count` und `complete` sind Messungen.

`TestE2E_MCPReadTool` startet den MCP-Server des CLI im Prozess (ohne
Optionen, also nur lesende Tools; In-Memory-Transport des SDK) und ruft das
Tool `user_list` mit `limit` 1 auf (1 Credit). Hart: `user_list` ist
gelistet, `invoice_create` und `auth_status` nicht, jeder Name einmal; das
Ergebnis ist kein Tool-Fehler, sein erster Inhalt JSON mit dem Array
`entries`, und kein Inhalt enthält das Token. Seine Anfragen laufen über
denselben Transport wie alle anderen (Budget, Abdeckung, Schutzregeln); ein
Tool-Fehler mit `exit_code` 6 stoppt die Suite wie ein 429 eines CLI-Laufs.

`TestE2E_OpenMeasurements` prüft vier offene Punkte (12 Credits je Lauf,
siehe [docs/offene-punkte.md](../docs/offene-punkte.md)): Nr. 28 hart, die
übrigen als Messung. Nr. 28 ruft `invoice create` ohne `--document-date` auf
und verlangt Exit 5 ohne Anfrage (0 Credits): Die API lehnt eine solche
Rechnung mit 422 ab (gemessen Live-Lauf 5, 2026-11-08), und ein abgelehntes
Anlegen kostet die vollen Credits (gemessen beim Angebot, 2026-11-04). Nr. 10
sendet per `papierkram api PUT` ein `due_date` an die Entwurfsrechnung, Nr. 13 setzt
`supply_date` per `--supply-date` und schickt die `describe`-Ausgabe im
Rundlauf zurück, Nr. 12 setzt eine Bewirtungsperson am Beleg und sendet
danach `[]` („geleert“ oder „unverändert“). Antwortet ein Schritt mit 429,
endet der Subtest ohne weitere Anfrage (und alle folgenden Tests werden
übersprungen, siehe Credit-Budget). Von den Messungen lässt nur Nr. 13 den
Lauf hart fehlschlagen, wenn `invoice update` im Rundlauf (`payment_term` im Body) die Rechnung vor dem
`PUT` liest: Das ist Verhalten des CLI, kein Messwert der API.
