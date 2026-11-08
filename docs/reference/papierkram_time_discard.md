# papierkram time discard

Timer verwerfen, ohne zu buchen

```text
Löscht den Timer des wirksamen Kontexts, ohne einen Zeiteintrag anzulegen: die erfasste Zeit geht verloren. Im Terminal fragt der Befehl nach (Enter = nein); ohne Terminal oder mit --no-input ist --yes nötig (sonst Exit 8). Verwirft auch eine beschädigte Timer-Datei und die Datei eines nicht abgeschlossenen time stop (<datei>.stopping). Auch unter Schreibschutz erlaubt, da nur lokal. Der Timer ist lokal (eine Datei je Kontext im Cache-Verzeichnis, PAPIERKRAM_CACHE_DIR); Papierkram kennt ihn nicht.

Verwendung:
  papierkram time discard [Optionen]

Beispiele:
  papierkram time discard
  papierkram time discard --yes

Optionen:
  -h, --help   Hilfe zu discard

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
