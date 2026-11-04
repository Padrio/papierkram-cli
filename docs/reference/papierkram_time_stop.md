# papierkram time stop

Timer stoppen und als Zeiteintrag buchen

```text
Bucht die Zeit des Timers wie time log: Datum und Uhrzeit des Starts bis jetzt (in ganzen Minuten), Aufgabe, Benutzer und Kommentar des Timers; -m ersetzt den Kommentar. Einträge über Mitternacht und unter einer Minute werden abgelehnt (Exit 2). Start und Ende gelten in der Zeitzone des Rechners beim Stoppen: Wechselt die Zeitzone zwischen time start und time stop (z. B. auf Reisen), werden die Uhrzeiten des Stopp-Orts gebucht (Start 09:00 in Berlin, Stopp 11:00 in London ergibt 08:00–11:00); die Dauer bleibt richtig.

Im Terminal zeigt der Befehl vorher eine Zusammenfassung und fragt nach (Enter = ja), wie time log; --dry-run zeigt die Anfrage. Der Timer wird erst gelöscht, wenn der Zeiteintrag angelegt ist; bei jedem Fehler, Abbruch oder Probelauf läuft er weiter (dann von Hand mit papierkram time log buchen oder mit papierkram time discard verwerfen).

Kosten: 10 Credits je Zeiteintrag.

Verwendung:
  papierkram time stop [Optionen]

Beispiele:
  papierkram time stop
  papierkram time stop -m "Fehler behoben" --yes

Optionen:
  -m, --comments Text   Kommentar statt des Kommentars aus time start
  -h, --help            Hilfe zu stop

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
