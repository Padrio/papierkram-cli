# papierkram commands

Alle Befehle mit Klasse, Credits, Flags und Beispielen auflisten

```text
Listet alle Befehle mit Klasse (Spec 8.1), Credits je Aufruf (bei deliver mit Nachlesen, bei invoice update mit dem Lesen der Rechnung; Flags wie --file und --from-estimate sowie --all kosten zusätzlich) und Kurzhilfe.

Mit --json (oder -o json|yaml, --jq) kommt die maschinenlesbare Liste für Agents: je Befehl Verwendungszeile mit Positionsargumenten, Aliase, Kurzhilfe, Klasse, Credits, Operation (operation_id, method, path), Flags mit Kurzform, Typ, Pflicht (mit gleichwertigen Flags unter alternatives), Wiederholbarkeit, erlaubten Werten, Standardwert wie „Standard:“ in der Hilfe (Feld default), geprüften Grenzen (Felder minimum, maximum), Namensauflösung (Feld resolves: das ID-Flag nimmt auch einen Namen dieser Ressource) und Hilfetext, dazu Beispiele; die globalen Flags stehen einmal unter global_flags. Vertrag: docs/schemas/commands.json (Version 1).

Klassen außer denen der Spec: local wirkt nur lokal (Konfiguration, Schlüsselbund, Projektdatei, Timer) und kostet nichts, außer der Befehl nennt eine Operation (auth login prüft das Token per GET /info); per_request entscheidet sich je Anfrage (papierkram api), credits ist dann null. blocked_by_read_only sagt, ob der Befehl unter PAPIERKRAM_READ_ONLY mit Exit 9 abbricht.

Sendet keine Anfrage.

Verwendung:
  papierkram commands [Optionen]

Beispiele:
  papierkram commands
  papierkram commands --json
  papierkram commands --json --jq '.commands[] | select(.class == "destructive") | .command'

Optionen:
  -h, --help   Hilfe zu commands
      --json   maschinenlesbare Liste als JSON (wie -o json)

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

- [papierkram](papierkram.md) – Kommandozeile für die Papierkram-API
