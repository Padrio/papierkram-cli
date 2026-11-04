# papierkram context rename

Kontext umbenennen

```text
Benennt einen Kontext um und verschiebt das Token im Schlüsselbund. Ein aktiver Kontext bleibt aktiv.

Verwendung:
  papierkram context rename <alt> <neu> [Optionen]

Beispiele:
  papierkram context rename firma firma-gmbh
  papierkram context rename agent agent-lesend

Optionen:
  -h, --help   Hilfe zu rename

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

- [papierkram context](papierkram_context.md) – Kontexte (Account-Setups) verwalten
