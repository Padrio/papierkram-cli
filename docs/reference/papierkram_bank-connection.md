# papierkram bank-connection

Bankverbindungen anzeigen

```text
Bankverbindungen anzeigen

Verwendung:
  papierkram bank-connection [Optionen]
  papierkram bank-connection [Befehl]

Aliase:
  bank-connection, bankverbindung

Verfügbare Befehle:
  describe    Bankverbindung anzeigen
  list        Bankverbindungen auflisten

Optionen:
  -h, --help   Hilfe zu bank-connection

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

Mit "papierkram bank-connection [Befehl] --help" gibt es weitere Informationen zu einem Befehl.
```

## Siehe auch

- [papierkram](papierkram.md) – Kommandozeile für die Papierkram-API
- [papierkram bank-connection describe](papierkram_bank-connection_describe.md) – Bankverbindung anzeigen
- [papierkram bank-connection list](papierkram_bank-connection_list.md) – Bankverbindungen auflisten
