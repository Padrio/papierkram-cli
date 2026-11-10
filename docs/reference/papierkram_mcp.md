# papierkram mcp

MCP-Server für KI-Clients

```text
Stellt die Befehle als Tools für KI-Clients bereit (Model Context Protocol).

Verwendung:
  papierkram mcp [Optionen]
  papierkram mcp [Befehl]

Verfügbare Befehle:
  serve       MCP-Server über stdio starten

Optionen:
  -h, --help   Hilfe zu mcp

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

Mit "papierkram mcp [Befehl] --help" gibt es weitere Informationen zu einem Befehl.
```

## Siehe auch

- [papierkram](papierkram.md) – Kommandozeile für die Papierkram-API
- [papierkram mcp serve](papierkram_mcp_serve.md) – MCP-Server über stdio starten
