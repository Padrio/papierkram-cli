# papierkram user

Benutzer des Accounts anzeigen

```text
Benutzer des Accounts anzeigen

Verwendung:
  papierkram user [Optionen]
  papierkram user [Befehl]

Aliase:
  user, benutzer

Verfügbare Befehle:
  describe    Benutzer anzeigen
  list        Benutzer auflisten

Optionen:
  -h, --help   Hilfe zu user

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

Mit "papierkram user [Befehl] --help" gibt es weitere Informationen zu einem Befehl.
```

## Siehe auch

- [papierkram](papierkram.md) – Kommandozeile für die Papierkram-API
- [papierkram user describe](papierkram_user_describe.md) – Benutzer anzeigen
- [papierkram user list](papierkram_user_list.md) – Benutzer auflisten
