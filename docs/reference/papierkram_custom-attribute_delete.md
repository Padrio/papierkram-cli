# papierkram custom-attribute delete

Benutzerdefiniertes Datenfeld löschen

```text
Benutzerdefiniertes Datenfeld löschen.

Kosten: 1 Credit je Anfrage.

Verwendung:
  papierkram custom-attribute delete <id> [Optionen]

Beispiele:
  papierkram custom-attribute delete 338
  papierkram custom-attribute delete 338 --yes

Optionen:
  -h, --help   Hilfe zu delete

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

- [papierkram custom-attribute](papierkram_custom-attribute.md) – Benutzerdefinierte Datenfelder verwalten
