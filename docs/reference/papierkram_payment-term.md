# papierkram payment-term

Zahlungsbedingungen anzeigen

```text
Zahlungsbedingungen anzeigen

Verwendung:
  papierkram payment-term [Optionen]
  papierkram payment-term [Befehl]

Aliase:
  payment-term, zahlungsbedingung

Verfügbare Befehle:
  describe    Zahlungsbedingung anzeigen
  list        Zahlungsbedingungen auflisten

Optionen:
  -h, --help   Hilfe zu payment-term

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

Mit "papierkram payment-term [Befehl] --help" gibt es weitere Informationen zu einem Befehl.
```

## Siehe auch

- [papierkram](papierkram.md) – Kommandozeile für die Papierkram-API
- [papierkram payment-term describe](papierkram_payment-term_describe.md) – Zahlungsbedingung anzeigen
- [papierkram payment-term list](papierkram_payment-term_list.md) – Zahlungsbedingungen auflisten
