# papierkram auth

API-Token anmelden, abmelden und prüfen

```text
Hinterlegt das API-Token eines Kontexts im Schlüsselbund des Betriebssystems, entfernt es wieder
oder zeigt, welcher Kontext, welches Token und welcher Schreibschutz gerade gelten.

Verwendung:
  papierkram auth [Optionen]
  papierkram auth [Befehl]

Verfügbare Befehle:
  login       Token prüfen und speichern
  logout      Token entfernen
  status      Kontext, Token-Quelle, Schreibschutz und Restkontingent anzeigen

Optionen:
  -h, --help   Hilfe zu auth

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

Mit "papierkram auth [Befehl] --help" gibt es weitere Informationen zu einem Befehl.
```

## Siehe auch

- [papierkram](papierkram.md) – Kommandozeile für die Papierkram-API
- [papierkram auth login](papierkram_auth_login.md) – Token prüfen und speichern
- [papierkram auth logout](papierkram_auth_logout.md) – Token entfernen
- [papierkram auth status](papierkram_auth_status.md) – Kontext, Token-Quelle, Schreibschutz und Restkontingent anzeigen
