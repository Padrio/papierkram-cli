# papierkram time start

Timer starten (lokal, Buchung mit time stop)

```text
Startet den Timer für den wirksamen Kontext: Startzeit (jetzt), Aufgabe, Benutzer und Kommentar werden gespeichert; time stop bucht die Zeit bis dahin als Zeiteintrag.

Aufgabe und Benutzer werden jetzt bestimmt wie bei time log: Flag, sonst PAPIERKRAM_TASK bzw. PAPIERKRAM_USER, sonst task bzw. user aus der Projektdatei .papierkram.toml (anlegen: papierkram local init), beim Benutzer zuletzt user des Kontexts. --task nimmt auch einen Alias aus [tasks] der Projektdatei.

Läuft schon ein Timer oder wurde ein time stop nicht abgeschlossen, bricht der Befehl ab (Exit 7). Unter Schreibschutz ist er gesperrt (Exit 9), weil time stop nicht buchen könnte. Sendet keine Anfrage. Der Timer ist lokal (eine Datei je Kontext im Cache-Verzeichnis, PAPIERKRAM_CACHE_DIR); Papierkram kennt ihn nicht.

Verwendung:
  papierkram time start [Optionen]

Beispiele:
  papierkram time start --task 545 -m "Abstimmung mit Kunde"
  papierkram time start --task tickets

Optionen:
  -m, --comments Text   Kommentar (time stop -m ersetzt ihn)
  -h, --help            Hilfe zu start
      --task Text       Aufgabe: ID oder Alias aus [tasks] der Projektdatei; ohne Angabe PAPIERKRAM_TASK oder task aus der Projektdatei .papierkram.toml (Pflicht)
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
      --timeout Dauer   Zeitlimit je Anfrage, pro Versuch; GET wird bis zu zweimal wiederholt (Standard: 30s)
  -y, --yes             Bestätigungen automatisch bejahen
```

## Siehe auch

- [papierkram time](papierkram_time.md) – Zeiteinträge erfassen und verwalten
