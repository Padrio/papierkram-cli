# papierkram invoice deliver

Rechnung versenden (per E-Mail oder als PDF finalisieren)

```text
Rechnung versenden (per E-Mail oder als PDF finalisieren).

Kosten: 1 Credit je Anfrage.

--via email versendet eine E-Mail mit Empfänger (--to), Betreff (--subject) und Text (--body oder --body-file, - für die Standardeingabe); Betreff und Text dürfen Platzhalter wie {{rechnung.rechnungsnummer}} oder {{angebot.angebotsnummer}} enthalten. --via pdf finalisiert den Beleg und vergibt die Nummer, ohne E-Mail. Ohne Terminal ist --yes nötig.

Danach liest das CLI den Beleg nach (1 weiterer Credit; --dry-run zeigt die Summe) und zeigt id, Nummer, state, sent_on, sent_via und sent_to. Scheitert nur das Nachlesen, endet der Befehl mit Exit 0 und einer Warnung – dann NICHT erneut ausführen. Bricht die Verbindung nach dem Senden ab, entscheidet das Nachlesen: sent_on von heute → Exit 0 mit Warnung, sonst Exit 12 (vor Wiederholung per describe prüfen). Bei --via pdf setzt die API weder sent_on noch sent_via (beide bleiben null, state wird z. B. sent); ein abgebrochener PDF-Versand endet deshalb immer mit Exit 12.

Verwendung:
  papierkram invoice deliver <id> [Optionen]

Aliase:
  deliver, send

Beispiele:
  papierkram invoice deliver 459 --via pdf
  papierkram invoice deliver 459 --via email --to kunde@example.org --subject "Rechnung {{rechnung.rechnungsnummer}}" --body-file mail.txt

Optionen:
      --body Text        Text der E-Mail, Platzhalter erlaubt (Pflicht bei --via email, sonst --body-file)
      --body-file Text   Text der E-Mail aus Datei oder - für die Standardeingabe
  -h, --help             Hilfe zu deliver
      --input Text       Body als JSON-Datei oder - für die Standardeingabe; Flags überschreiben einzelne Felder
      --subject Text     Betreff der E-Mail, Platzhalter erlaubt (Pflicht bei --via email)
      --to Text          Empfängeradresse der E-Mail (Pflicht bei --via email)
      --via Text         Versandart: email oder pdf (Pflicht; pdf finalisiert ohne E-Mail)

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
