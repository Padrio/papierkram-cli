# papierkram time status

Laufenden Timer anzeigen

```text
Zeigt den Timer des wirksamen Kontexts: Start, bisherige Dauer (h:mm nach Uhrzeit, wie time stop sie bucht, auch an Tagen der Zeitumstellung), Aufgabe, Benutzer, Kommentar, ob der Eintrag nicht abrechenbar ist (JSON unbillable, -o wide Spalte NICHT ABRECHENBAR; false bzw. leer auch ohne --unbillable beim Start) und das Konto (Host), für das er gestartet wurde (-o wide, JSON account; gilt jetzt ein anderes, warnt der Befehl, denn time stop bucht dann nicht). Läuft gerade ein time stop oder wurde einer nicht abgeschlossen, zeigt der Befehl dessen Timer mit einer Warnung. Ohne Timer: „Kein Timer läuft“, mit -o json {"running":false} (Exit 0). Sendet keine Anfrage. Der Timer ist lokal (eine Datei je Kontext im Cache-Verzeichnis, PAPIERKRAM_CACHE_DIR); Papierkram kennt ihn nicht.

Verwendung:
  papierkram time status [Optionen]

Beispiele:
  papierkram time status
  papierkram time status -o json

Optionen:
  -h, --help   Hilfe zu status

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
