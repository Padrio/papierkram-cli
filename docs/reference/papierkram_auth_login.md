# papierkram auth login

Token prüfen und speichern

```text
Liest das API-Token (Papierkram: Einstellungen → API) verdeckt vom Terminal oder mit --token-stdin
von der Standardeingabe, prüft es mit GET /info (1 Credit) und speichert es erst danach im
Schlüsselbund des Betriebssystems. Das Token wird nie als Flag-Wert angenommen.

Ziel ist der gewählte Kontext (--context, PAPIERKRAM_CONTEXT, Projektdatei, aktiver Kontext).
Existiert er nicht, wird er mit --subdomain angelegt (und aktiv, falls noch keiner aktiv ist).
Ohne Schlüsselbund (z. B. Linux ohne Secret Service) speichert nur --insecure-storage das Token
im Klartext in config.toml (Rechte 0600).

Verwendung:
  papierkram auth login [Optionen]

Beispiele:
  papierkram auth login --context firma-gmbh --subdomain firma-gmbh
  papierkram auth login --token-stdin < token.txt

Optionen:
  -h, --help               Hilfe zu login
      --insecure-storage   ohne Schlüsselbund das Token im Klartext in config.toml speichern
      --subdomain Text     Subdomain, wenn der Kontext neu angelegt wird
      --token-stdin        Token von der Standardeingabe lesen (Leerzeichen und Zeilenende werden entfernt)

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

- [papierkram auth](papierkram_auth.md) – API-Token anmelden, abmelden und prüfen
