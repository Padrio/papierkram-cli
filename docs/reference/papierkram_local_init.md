# papierkram local init

Projektdatei anlegen oder ändern

```text
Schreibt Werte in die Projektdatei .papierkram.toml: in die gefundene Datei, sonst neu ins aktuelle Verzeichnis. Nur die angegebenen Schlüssel ändern sich; andere Schlüssel und [tasks] bleiben, Kommentare und Formatierung gehen beim Schreiben verloren.

Im Terminal ohne Flags fragt der Befehl jeden Wert ab (Enter behält den bisherigen). Ohne Terminal oder mit --no-input sind Flags nötig. --context nennt hier den Kontext, für den die Datei gilt. Unter PAPIERKRAM_READ_ONLY ist der Befehl gesperrt.

Die Werte belegen Flags vor: company (genutzt von person … --company), project (task list|create, time list, project describe ohne ID), task und user (time log, time start).

Verwendung:
  papierkram local init [Optionen]

Beispiele:
  papierkram local init --project 4963 --task 545
  papierkram local init --context firma-gmbh --company 3352 --user 1
  papierkram local init --task tickets

Optionen:
      --company ID     Firma-ID
      --context Text   Kontext, für den die Projektdatei gilt
  -h, --help           Hilfe zu init
      --project ID     Projekt-ID
      --task Text      Aufgaben-ID oder Alias aus [tasks] der Projektdatei
      --user ID        Benutzer-ID

Globale Optionen:
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

- [papierkram local](papierkram_local.md) – Projektdatei .papierkram.toml anlegen und anzeigen
