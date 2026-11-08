# papierkram time log

Zeit erfassen (Zeiteintrag mit Zeitraum, Aufgabe und Benutzer)

```text
Erfasst Zeit als Zeiteintrag: Datum, Zeitraum, Aufgabe und Benutzer.

Zeitraum: --from und --to (H:MM oder HH:MM) oder --duration (1h30m, 90m, 1:30), mit --from ab dieser Uhrzeit, ohne --from bis jetzt (nur für heute). Ein Zeiteintrag hat nur ein Datum: Ein Ende genau um Mitternacht (--to 24:00 oder 00:00) ist erlaubt; andere Einträge über Mitternacht werden abgelehnt, sie werden als zwei Einträge erfasst. Eine Dauer wird in Uhrzeit gerechnet, wie Papierkram sie bucht (Uhrzeiten ohne Zeitzone): --from 01:30 --duration 2h ergibt 01:30–03:30, auch an den Tagen der Zeitumstellung; --from und --to gelten wie angegeben.

Aufgabe und Benutzer: Flag, sonst PAPIERKRAM_TASK bzw. PAPIERKRAM_USER, sonst task bzw. user aus der Projektdatei .papierkram.toml (anlegen: papierkram local init), beim Benutzer zuletzt user des Kontexts. --task nimmt auch einen Alias aus [tasks] der Projektdatei.

Im Terminal zeigt der Befehl vorher eine Zusammenfassung mit Firma, Projekt, Aufgabe und Benutzer und fragt nach (Enter = ja); jeder Name kostet beim ersten Mal 1 Credit und wird 60 Sekunden zwischengespeichert. --yes, --no-input und Aufrufe ohne Terminal senden ohne Rückfrage.

Kosten: 10 Credits je Zeiteintrag.

Verwendung:
  papierkram time log [Optionen]

Beispiele:
  papierkram time log --task 545 --from 9:00 --to 10:30 -m "Abstimmung mit Kunde"
  papierkram time log --task tickets --duration 1h30m
  papierkram time log --date gestern --from 13:00 --duration 45m --unbillable --yes

Optionen:
  -m, --comments Text   Kommentar
      --date Datum      Datum: YYYY-MM-DD, today/heute, yesterday/gestern (Standard: heute)
      --duration Text   Dauer: 1h30m, 90m oder 1:30; ohne --from endet der Eintrag jetzt
      --from Text       Beginn als H:MM oder HH:MM
  -h, --help            Hilfe zu log
      --task Text       Aufgabe: ID oder Alias aus [tasks] der Projektdatei; ohne Angabe PAPIERKRAM_TASK oder task aus der Projektdatei .papierkram.toml (Pflicht)
      --to Text         Ende als H:MM oder HH:MM, um Mitternacht 24:00 oder 00:00 (nicht mit --duration)
      --unbillable      nicht abrechenbar
      --user ID         Benutzer-ID; ohne Angabe PAPIERKRAM_USER oder user aus der Projektdatei .papierkram.toml oder user des Kontexts (Pflicht)

Globale Optionen:
      --context Text    Kontext statt des aktiven verwenden
      --debug           HTTP-Debug-Ausgabe auf stderr (ohne Token)
      --dry-run         Mutation nur anzeigen, nicht senden
      --fields Text     Felder der Ausgabe, kommagetrennt
      --jq Text         jq-Ausdruck auf die Ausgabe anwenden
      --no-color        Farben abschalten
      --no-input        keine interaktiven Rückfragen
  -o, --output Text     Ausgabeformat: table|wide|json|yaml (Standard: "table")
      --timeout Dauer   Zeitlimit je Anfrage, pro Versuch (Uploads ohne Angabe mindestens 120 s); GET wird bis zu zweimal wiederholt (Standard: 30s)
  -y, --yes             Bestätigungen automatisch bejahen
```

## Siehe auch

- [papierkram time](papierkram_time.md) – Zeiteinträge erfassen und verwalten
