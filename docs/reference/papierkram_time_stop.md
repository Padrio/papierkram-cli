# papierkram time stop

Timer stoppen und als Zeiteintrag buchen

```text
Bucht die Zeit des Timers wie time log: Datum und Uhrzeit des Starts bis jetzt (in ganzen Minuten), Aufgabe, Benutzer und Kommentar des Timers; -m ersetzt den Kommentar. Stoppt der Timer in der ersten Minute des Folgetags (00:00:00 bis 00:00:59), endet der Eintrag um 24:00 des Starttags; andere Einträge über Mitternacht und Einträge unter einer Minute werden abgelehnt (Exit 2). Start und Ende gelten in der Zeitzone des Rechners beim Stoppen: Wechselt die Zeitzone zwischen time start und time stop (z. B. auf Reisen), werden die Uhrzeiten des Stopp-Orts gebucht (Start 09:00 in Berlin, Stopp 11:00 in London ergibt 08:00–11:00); die Dauer bleibt richtig.

Im Terminal zeigt der Befehl vorher eine Zusammenfassung und fragt nach (Enter = ja), wie time log; --dry-run zeigt die Anfrage. Der Timer wird erst gelöscht, wenn der Zeiteintrag angelegt ist; bei jedem Fehler, Abbruch oder Probelauf läuft er weiter (dann von Hand mit papierkram time log buchen oder mit papierkram time discard verwerfen).

Direkt vor dem Senden beansprucht der Befehl den Timer atomar (er legt <datei>.stopping exklusiv an und entfernt danach die Timer-Datei): Ein zweiter time stop desselben Timers endet mit Exit 7, ohne etwas zu senden. Scheitert die Anfrage, wird die Timer-Datei wiederhergestellt. Bleibt sie nach einem Absturz liegen, brechen time start und time stop mit Exit 7 ab, bis sie geprüft und mit papierkram time discard verworfen ist.

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
