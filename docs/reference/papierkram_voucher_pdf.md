# papierkram voucher pdf

Beleg als PDF herunterladen

```text
Beleg als PDF herunterladen.

Kosten: 1 Credit je Anfrage.

-O datei.pdf speichert das PDF in der Datei; eine vorhandene Datei wird erst ersetzt, wenn das PDF vollständig geladen ist, und behält ihre Rechte, wenn sie dem Nutzer gehört (eine neue oder fremde Datei erhält die Rechte gemäß umask). Eine schreibgeschützte Datei wird nicht ersetzt; ein symbolischer Verweis bleibt erhalten, ersetzt wird die Datei, auf die er zeigt – nur wenn alle Verweise und die Datei dem Nutzer gehören; Gerätedateien wie /dev/null werden direkt beschrieben. -O - schreibt das PDF nach stdout. Ohne -O geht es nur dann nach stdout, wenn stdout kein Terminal ist (Umleitung oder Pipe); im Terminal bricht der Befehl ohne Anfrage ab.

Danach meldet der Befehl Pfad und Größe ({"path":…,"bytes":…}, path "-" für stdout) auf stderr, bei -O datei mit -o json|yaml, --fields oder --jq auf stdout. Geht das PDF nach stdout, bleibt die Meldung auf stderr; --fields und --jq sind dann nicht erlaubt.

Verwendung:
  papierkram voucher pdf <id> [Optionen]

Beispiele:
  papierkram voucher pdf 295 -O beleg.pdf
  papierkram voucher pdf 295 -O - > beleg.pdf

Optionen:
  -h, --help               Hilfe zu pdf
  -O, --output-file Text   Zieldatei des PDFs oder - für stdout

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

- [papierkram voucher](papierkram_voucher.md) – Belege (Ausgaben) verwalten
