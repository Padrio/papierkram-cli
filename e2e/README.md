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
im Eingabefeld die Subdomain der Sandbox zur Bestätigung eingeben. Subdomain
und Token kommen aus den Repository-Secrets `PAPIERKRAM_E2E_SUBDOMAIN` und
`PAPIERKRAM_E2E_TOKEN`.

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
  lehnt der Transport der Suite ab.
- `deliver` geht nur mit `--via pdf` hinaus. Eine E-Mail verschickt die
  Suite nie; auch das erzwingt der Transport.
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
- `voucher pay` wird nur mit einer Bankumsatz-ID gemessen, die es nicht
  gibt; echte Bankumsätze bleiben unberührt.
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
Puffer). Die Tests dürfen also höchstens 444 Credits verbrauchen: Eine
Anfrage, die in die Reserve greifen würde, wird nicht gesendet, und reicht das
Budget für einen Test nicht mehr, werden dieser und alle folgenden Tests mit
Begründung übersprungen. Erst das Aufräumen am Ende darf die Reserve nutzen.
Am Ende steht die Summe im Log.

## Messungen

Ergebnisse stehen im Log mit dem Präfix `MESSUNG`. Hart fehlschlagen nur
Prüfungen, auf deren Ergebnis sich das CLI verlässt; reine Messungen
(z. B. erlaubte Upload-Typen, Statuswerte, Kosten und Struktur von
`GET /info`, Einheiten der Zahlungsbedingungen) schlagen nicht fehl.
Weicht eine Messung von dem ab, was das CLI annimmt (z. B. andere Kosten
für `GET /info` als in der Registry), steht sie mit `ABWEICHUNG:` im Log und
am Ende des Laufs noch einmal in einer Zusammenfassung. Welche Prüfung zu welcher Gruppe gehört, steht im Kopf
von `e2e_test.go`.
