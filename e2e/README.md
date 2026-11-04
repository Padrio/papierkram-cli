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
liegt bei dir: Trage nie Subdomain und Token der echten Buchhaltung ein. Die
Suite legt Firmen, Kontaktpersonen, Projekte, Aufgaben, Zeiteinträge,
Artikel, ein Datenfeld, zwei Angebote, eine Rechnung und einen Beleg an,
lädt Dateien hoch und versendet ein Angebot als PDF.

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
Secrets und Schutzregeln (z. B. Pflicht-Freigabe, nur Branch `main`) richtet
der Repository-Inhaber in den Repository-Einstellungen ein; das Repository
selbst legt sie nicht an.

Ohne die Variablen (Nachweis, dass alles überspringt):

```sh
go vet -tags e2e ./e2e/...
go test -tags e2e -v ./e2e/...
```

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
  Zufallszahl). Aufgeräumt werden nur Objekte,
  deren Name mit `pk-e2e-` beginnt: am Ende des Laufs die eigenen, im Test
  `TestE2E_CleanupLeftovers` die Reste früherer Läufe. Andere Objekte
  fasst die Suite nie an (Lesen ausgenommen).
- Was sich nicht löschen lässt (z. B. ein versendetes Angebot), wird
  storniert bzw. archiviert. Rechnungen, Angebote und Belege werden als
  Entwurf angelegt; versendet wird nur ein eigenes Angebot, keine Rechnung
  (keine Lücke im Rechnungsnummernkreis).
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
  kodiert, mit `.json` oder `;…`), geht die Anfrage nur in genau einer Form
  hinaus: `POST` auf `/api/v1/expense/vouchers/<id>/pay` bzw.
  `/api/v1/income/{invoices,estimates}/<id>/deliver`, ohne Query (auch kein
  leeres `?`), mit genau `Content-Type: application/json` (höchstens
  `; charset=utf-8`) und ohne `Content-Encoding`. Sonst wird sie vor dem
  Senden abgelehnt, z. B.
  `papierkram api POST '/expense/vouchers/<id>/pay?value=5' --input -`.
- Das Token erscheint nie in der Testausgabe: Jede Logzeile wird vorher
  geschwärzt und danach auf Reste geprüft; die CLI-Ausgaben werden auf das
  Token geprüft.

## Credit-Budget

Ein voller Lauf kostet nach den dokumentierten Preisen (Abschnitt 4.3) etwa
**426 Credits**; das Budget je Lauf ist **490** (Zielwert unter 500, die
Aufschlüsselung steht im Kopf von `e2e_test.go`). Gezählt wird mit den
Antwort-Headern `X-Consumed-Quota`/`X-Remaining-Quota`.

Vom Budget sind **46 Credits fürs Aufräumen reserviert** (dokumentierter
Höchstaufwand: je Rechnung/Angebot/Beleg 4, je anderem Objekt 2, plus 10
Puffer). Die Tests dürfen also höchstens 444 Credits verbrauchen (geplant: 412): Eine
Anfrage, die in die Reserve greifen würde, wird nicht gesendet, und reicht das
Budget für einen Test nicht mehr, werden dieser und alle folgenden Tests mit
Begründung übersprungen. Erst das Aufräumen am Ende darf die Reserve nutzen.
Am Ende steht die Summe im Log.

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
