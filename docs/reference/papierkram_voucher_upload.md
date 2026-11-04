# papierkram voucher upload

Datei (Belegbild) an einen Beleg anhängen

```text
Datei (Belegbild) an einen Beleg anhängen.

Kosten: 10 Credits je Anfrage.

Die Datei wird als multipart/form-data hochgeladen; der MIME-Typ folgt aus der Dateiendung (unbekannte Endung: application/octet-stream). Gesendet wird nur der Dateiname ohne Verzeichnis und ohne Steuer- oder unsichtbare Zeichen. Die Datei muss vorhanden, lesbar und nicht leer sein, sonst bricht der Befehl ohne Anfrage ab. --dry-run zeigt Dateiname, Größe und MIME-Typ. Die ganze Datei muss innerhalb von --timeout (Standard 30 s) übertragen sein; größere Dateien brauchen ggf. --timeout 120s.

Verwendung:
  papierkram voucher upload <beleg-id> <datei> [Optionen]

Beispiele:
  papierkram voucher upload 295 beleg.pdf
  papierkram voucher upload 295 quittung.jpg --dry-run

Optionen:
  -h, --help   Hilfe zu upload

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

- [papierkram voucher](papierkram_voucher.md) – Belege (Ausgaben) verwalten
