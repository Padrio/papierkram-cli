# papierkram auth logout

Token entfernen

```text
Entfernt das Token des gewählten Kontexts aus dem Schlüsselbund und aus config.toml.
Der Kontext selbst bleibt erhalten. War kein Token gespeichert, ist das kein Fehler.

Verwendung:
  papierkram auth logout [Optionen]

Beispiele:
  papierkram auth logout
  papierkram auth logout --context firma-gmbh

Optionen:
  -h, --help   Hilfe zu logout

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

- [papierkram auth](papierkram_auth.md) – API-Token anmelden, abmelden und prüfen
