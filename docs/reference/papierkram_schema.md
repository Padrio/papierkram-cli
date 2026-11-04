# papierkram schema

Request- und Response-Schema einer Aktion als JSON anzeigen

```text
Zeigt das Request- und das Response-Schema der API-Operation eines Befehls als {"request": …, "response": …} – aufgelöst ($ref ersetzt, allOf zusammengeführt) aus der eingebetteten, korrigierten Spec, mit den x-cli-*-Erweiterungen des CLI. null = die Spec beschreibt kein Schema (kein Body; die Antwort kann trotzdem JSON sein, z. B. bei info und upload); PDF-Downloads antworten mit {"type": "string", "format": "binary"}.

Ressource und Aktion dürfen Aliase sein (rechnung send). Ohne Aktion kommen alle Aktionen der Ressource, die eine Anfrage senden: {"resource": …, "actions": {"<aktion>": {"request": …, "response": …}}}. time log und time stop senden die Operation von time create.

Ausgabe als JSON (Standard) oder mit -o yaml; --fields und --jq wirken wie bei anderen Befehlen. Sendet keine Anfrage.

Verwendung:
  papierkram schema <ressource> [<aktion>] [Optionen]

Beispiele:
  papierkram schema invoice create
  papierkram schema rechnung send -o yaml
  papierkram schema voucher --jq '.actions | keys'

Optionen:
  -h, --help   Hilfe zu schema

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

- [papierkram](papierkram.md) – Kommandozeile für die Papierkram-API
