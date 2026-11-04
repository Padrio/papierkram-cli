# papierkram task delete

Aufgabe löschen

```text
Aufgabe löschen.

Kosten: 1 Credit je Anfrage.

Verwendung:
  papierkram task delete <id> [Optionen]

Beispiele:
  papierkram task delete 488
  papierkram task delete 488 --yes

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
      --timeout Dauer   Zeitlimit je Anfrage, pro Versuch; GET wird bis zu zweimal wiederholt (Standard: 30s)
  -y, --yes             Bestätigungen automatisch bejahen
```

## Siehe auch

- [papierkram task](papierkram_task.md) – Aufgaben verwalten
