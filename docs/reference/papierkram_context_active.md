# papierkram context active

Aktiven Kontext anzeigen

```text
Zeigt den in der Konfiguration gespeicherten aktiven Kontext (active_context).
--context, PAPIERKRAM_CONTEXT und die Projektdatei werden hier nicht berücksichtigt.

Verwendung:
  papierkram context active [Optionen]

Beispiele:
  papierkram context active
  papierkram context active -o json

Optionen:
  -h, --help   Hilfe zu active

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
