# papierkram context create

Kontext anlegen

```text
Legt einen neuen Kontext an. Ist noch kein Kontext aktiv, wird der neue Kontext automatisch aktiv.
Das Token wird separat mit "papierkram auth login" hinterlegt.

Verwendung:
  papierkram context create <name> [Optionen]

Beispiele:
  papierkram context create firma-gmbh --subdomain firma-gmbh
  papierkram context create agent --subdomain firma-gmbh --read-only

Optionen:
  -h, --help             Hilfe zu create
      --read-only        Kontext gegen Schreibzugriffe sperren
      --subdomain Text   Papierkram-Subdomain (Pflicht; auch firma.papierkram.de oder die URL)
      --user Text        Standard-User-ID für Zeiteinträge

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

- [papierkram context](papierkram_context.md) – Kontexte (Account-Setups) verwalten
