# papierkram invoice paid-status

Offene Posten: unbezahlte Rechnungen mit Summe

```text
Zeigt die offenen Posten: Rechnungen mit outstanding_amount > 0, deren Status nicht draft oder canceled ist (archivierte zählen mit), sortiert nach Fälligkeit (ohne Fälligkeit zuletzt), mit den Tagen seit der Fälligkeit (überfällig: Fälligkeit vor heute, Berliner Tag) und der Summe der offenen Beträge (exakt, auf den Cent gerundet). --overdue zeigt nur die überfälligen; Summe und Anzahl gelten dann für sie.

Lädt die Rechnungsliste wie invoice list --all (100 je Seite), aber höchstens --max-pages Seiten. Lässt --max-pages Seiten aus, warnt der Befehl auf stderr, und JSON hat complete false: Summe und Anzahl gelten dann nur für die geladenen Seiten.

Fehlt outstanding_amount in der Liste, bricht der Befehl ab (Exit 1), statt eine falsche Summe zu zeigen; --details liest dann diese Rechnungen einzeln nach.

Kosten: 1 Credit je Seite, mit --details 1 Credit je nachgelesener Rechnung; ein Name bei --customer kostet 1 Credit je Seite der Firmenliste.

Verwendung:
  papierkram invoice paid-status [Optionen]

Beispiele:
  papierkram invoice paid-status
  papierkram invoice paid-status --overdue -o json
  papierkram invoice paid-status --customer 3214 --from 2026-01-01 --to 2026-12-31

Optionen:
      --customer ID      Nur Rechnungen dieses Kunden (ID oder Name)
      --details          Rechnungen ohne outstanding_amount in der Liste einzeln nachlesen (1 Credit je Rechnung)
      --from Text        Nur Rechnungen mit Belegdatum ab diesem Tag (YYYY-MM-DD, heute, gestern)
  -h, --help             Hilfe zu paid-status
      --max-pages Zahl   höchstens so viele Seiten à 100 Rechnungen laden (1 Credit je Seite) (Standard: 5)
      --overdue          nur überfällige Rechnungen (Fälligkeit vor heute, Berliner Tag)
      --to Text          Nur Rechnungen mit Belegdatum bis zu diesem Tag (YYYY-MM-DD, heute, gestern)

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

- [papierkram invoice](papierkram_invoice.md) – Rechnungen verwalten
