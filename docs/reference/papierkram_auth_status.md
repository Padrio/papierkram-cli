# papierkram auth status

Kontext, Token-Quelle, Schreibschutz und Restkontingent anzeigen

```text
Zeigt den wirksamen Kontext (mit Herkunft), Subdomain, Basis-URL, die Quelle des Tokens (env, keychain
oder config; das Token selbst nie), die Schreibschutzstufe und das Restkontingent.
Das Kontingent stammt aus einem Aufruf von GET /info und kostet 1 Credit.
Fehlt das Token oder lehnt die API es ab, werden die bekannten Angaben trotzdem ausgegeben (Exit 3).

Verwendung:
  papierkram auth status [Optionen]

Beispiele:
  papierkram auth status
  papierkram auth status --context firma-gmbh -o json

Optionen:
  -h, --help   Hilfe zu status

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
