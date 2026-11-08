# papierkram local show

Wirksame Werte mit Herkunft anzeigen

```text
Zeigt Kontext, Firma (genutzt von person … --company), Projekt, Aufgabe und Benutzer, wie sie in diesem Verzeichnis gelten, mit Herkunft: flag, env, projectfile, config (active_context), context (Standard des Kontexts, nur user) oder default (nicht gesetzt). Gilt die Projektdatei für einen anderen Kontext als den wirksamen, werden ihre Werte ignoriert; das steht in der Ausgabe. Sendet keine Anfrage.

Verwendung:
  papierkram local show [Optionen]

Beispiele:
  papierkram local show
  papierkram local show --context firma-gmbh -o json

Optionen:
  -h, --help   Hilfe zu show

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

- [papierkram local](papierkram_local.md) – Projektdatei .papierkram.toml anlegen und anzeigen
