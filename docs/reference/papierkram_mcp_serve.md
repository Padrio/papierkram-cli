# papierkram mcp serve

MCP-Server über stdio starten

```text
Startet einen MCP-Server (Model Context Protocol) über stdin/stdout, damit ein KI-Client wie Claude Desktop oder Claude Code die Befehle als Tools aufrufen kann.

Je Befehl einer freigeschalteten Klasse gibt es ein Tool, z. B. invoice_list oder company_create; Name, Beschreibung (mit Klasse und Credits) und Eingabe-Schema stammen aus papierkram commands --json. Lokale Befehle (auth, context, local, completion, time start|stop|status|discard) und papierkram api haben keine Tools.

Modi: Ohne Option gibt es nur lesende Tools (read_only). --allow-writes schaltet create, update und archive frei. --allow-destructive schaltet zusätzlich destructive und send frei (delete, cancel, reverse, pay, document delete, deliver) und schließt --allow-writes ein; diese Tools laufen mit --yes, die Option ist die Freigabe dafür. Ein nicht freigeschaltetes Tool gibt es nicht (Protokollfehler beim Aufruf). Ist der Schreibschutz aktiv (PAPIERKRAM_READ_ONLY oder read_only des Kontexts), bleibt der Server bei lesenden Tools, mit einem Hinweis auf stderr; jeder Aufruf geht außerdem wie auf der Kommandozeile durch den Schreibschutz (Exit 9).

Jedes schreibende Tool hat den Parameter dry_run (--dry-run): Es prüft und zeigt die Anfrage, ohne sie zu senden. Die deliver-Tools (invoice_deliver, estimate_deliver) versenden nur als PDF (--via pdf) und haben nur id und dry_run; eine E-Mail ist über MCP nicht möglich.

Ein Tool-Aufruf führt genau den Befehl aus, den er beschreibt, mit -o json und --no-input: dieselben Prüfungen, derselbe Schreibschutz, dieselben Credits und Exit-Codes wie auf der Kommandozeile. Das Ergebnis ist die JSON-Ausgabe des Befehls; Hinweise und Warnungen von stderr folgen als zweiter Inhalt. Bei einem Fehler ist das Ergebnis ein Tool-Fehler mit der JSON-Fehlerzeile (code, message, hint, exit_code). Aufrufe laufen nacheinander.

Dateipfade gibt es nur mit --files-dir: ein vorhandener Ordner, nicht die Wurzel des Dateisystems und nicht das Home-Verzeichnis selbst (sonst Exit 2), am besten ein eigener Unterordner. Ohne --files-dir fehlen die Tools, die eine Datei brauchen (pdf, voucher upload), und voucher_create hat keinen Parameter file, mit einem Hinweis auf stderr; ein Arbeitsverzeichnis gilt nie als Dateiordner (Claude Desktop startet Server z. B. in /). Dateipfade der Tools sind relativ zu diesem Ordner: kein absoluter Pfad, kein „..“, kein „-“, kein symbolischer Verweis ins Leere oder aus dem Ordner hinaus, kein fehlendes Verzeichnis (sonst Exit 2 ohne Anfrage). Das CLI erhält den absoluten Pfad im Ordner. Die pdf-Tools laufen mit --no-clobber: Sie ersetzen keine vorhandene Datei. Ein Upload liest nur Dateien aus diesem Ordner.

In der Konfiguration eines Clients (JSON, z. B. {"command":"papierkram","args":["mcp","serve","--files-dir","/Users/name/Belege"]}) wird ~ nicht erweitert: den Ordner absolut angeben.

Der Kontext wird beim Start festgelegt (--context oder wie sonst: PAPIERKRAM_CONTEXT, Projektdatei, aktiver Kontext) und gilt für alle Aufrufe, auch wenn sich der aktive Kontext später ändert; Tools haben keinen Kontext-Parameter. --timeout gilt für jeden Aufruf. --yes, --jq, --fields, -o und --dry-run gelten nicht für den Server (Exit 2). --debug schreibt die Anfragen auf stderr, nie in ein Tool-Ergebnis. stdout trägt nur das Protokoll (JSON-RPC).

Der Server endet mit Exit 0, wenn stdin geschlossen wird oder ein Signal (Strg+C) kommt. Ein Signal wartet auf den laufenden Aufruf und bricht ihn nicht ab: Er läuft zu Ende, seine Antwort erreicht den Client aber nicht mehr (Verbindung geschlossen; eine Änderung dann wie nach Exit 12 per describe/list prüfen). Aufrufe, die noch warten, laufen nach einem Signal nicht mehr. Ein interner Fehler (Panic) eines Aufrufs beendet den Server nicht: Der Aufruf endet als Tool-Fehler mit code error und Exit 1 (Ergebnis unbekannt, per describe/list prüfen), der Stack steht auf stderr. Ein zweites Strg+C beendet den Prozess sofort. Bricht der Client einen Aufruf ab, endet der Befehl wie nach Strg+C (Exit 10, nach gesendeter Änderung 12), der Client erhält dafür keine Antwort.

Sendet selbst keine Anfrage; jeder Tool-Aufruf kostet die Credits seines Befehls.

Verwendung:
  papierkram mcp serve [Optionen]

Beispiele:
  papierkram mcp serve
  papierkram mcp serve --context firma
  papierkram mcp serve --context firma --allow-writes
  papierkram mcp serve --files-dir /Users/name/Belege

Optionen:
      --allow-destructive   zusätzlich Tools, die löschen, stornieren, zahlen oder versenden (destructive, send); sie laufen mit --yes, schließt --allow-writes ein
      --allow-writes        auch Tools, die anlegen, ändern oder archivieren (create, update, archive)
      --files-dir Text      Ordner, in dem alle Dateipfade der Tools liegen (absolut; ohne ihn keine Dateipfade)
  -h, --help                Hilfe zu serve

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

- [papierkram mcp](papierkram_mcp.md) – MCP-Server für KI-Clients
