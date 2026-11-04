# papierkram local

Projektdatei .papierkram.toml anlegen und anzeigen

```text
Die Projektdatei .papierkram.toml legt für ein Verzeichnis Kontext, Firma, Projekt, Aufgabe und Benutzer fest (gesucht im aktuellen Verzeichnis, in einem Git-Repository aufwärts bis zur Git-Wurzel). Vorrang: Flag > Umgebung (PAPIERKRAM_PROJECT, PAPIERKRAM_TASK, …) > Projektdatei > Kontext. Die Werte gelten nur, wenn der wirksame Kontext der Kontext der Datei ist.

Verwendung:
  papierkram local [Optionen]
  papierkram local [Befehl]

Verfügbare Befehle:
  init        Projektdatei anlegen oder ändern
  show        Wirksame Werte mit Herkunft anzeigen

Optionen:
  -h, --help   Hilfe zu local

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

Mit "papierkram local [Befehl] --help" gibt es weitere Informationen zu einem Befehl.
```

## Siehe auch

- [papierkram](papierkram.md) – Kommandozeile für die Papierkram-API
- [papierkram local init](papierkram_local_init.md) – Projektdatei anlegen oder ändern
- [papierkram local show](papierkram_local_show.md) – Wirksame Werte mit Herkunft anzeigen
