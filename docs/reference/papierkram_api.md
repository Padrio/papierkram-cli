# papierkram api

Beliebige Anfrage an die Papierkram-API senden (wie gh api)

```text
Beliebige Anfrage an die Papierkram-API senden (wie gh api).

<pfad> ist relativ zur API-Basis /api/v1, beginnt mit / und darf eine Query enthalten (/income/invoices?page=2). Vollständige URLs, //, \, die Segmente . und .. (auch %-kodiert), kodierte Schrägstriche, # und Steuerzeichen sind nicht erlaubt; Segmente bestehen nur aus Buchstaben, Ziffern, _ und -. Ein Pfad, der nur ohne Groß- und Kleinschreibung oder mit einer ID, die nicht nur aus Ziffern besteht, zu einer Operation der Spec passt (12-x, Deliver), ist nicht eindeutig und wird abgelehnt. METHOD ist GET, POST, PUT, PATCH oder DELETE (Groß- und Kleinschreibung egal).

Felder: -f key=value setzt immer Text. -F key=value erkennt true, false, null und JSON-Zahlen (exakt, 19.99 bleibt 19.99); @datei setzt den Inhalt der Datei als Text, @- die Standardeingabe; alles andere ist Text. Punkte im Schlüssel bilden verschachtelte Objekte (-F email.recipient=…); jeder Schlüssel nur einmal. Bei GET und bei DELETE ohne --input werden die Felder Query-Parameter, sonst bilden sie den JSON-Body. --input datei.json (oder - für die Standardeingabe) liefert den Body; -f/-F überschreiben darin einzelne, auch verschachtelte Felder.

Der Body wird ohne Bereinigung gesendet, wie angegeben. --validate prüft ihn vorher gegen das Request-Schema der Spec (Exit 5); für Pfade außerhalb der Spec geht das nicht.

Schutzregeln: Methode und Pfad werden in der Registry nachgeschlagen; deren Klasse entscheidet über Schreibschutz (Exit 9), Bestätigung bzw. --yes (Exit 8 ohne Terminal) und --dry-run. Pfade außerhalb der Spec gelten bei GET als lesend, sonst als destruktiv. Für …/deliver gelten die Regeln von deliver: keine Query (Exit 2), send_via (email oder pdf) ist Pflicht, bei email auch email.recipient, email.subject und email.body, und der Body wird immer gegen das Request-Schema geprüft (Exit 5). --dry-run zeigt Methode, URL, Body und Credits und gilt nur für ändernde Anfragen.

Kosten: wie der passende Befehl (z. B. 50 Credits für das Anlegen einer Rechnung); außerhalb der Spec nach derselben Regel (Anlegen 10 bzw. 50, sonst 1 Credit).

Antwort: JSON wie bei den anderen Befehlen (-o, --fields, --jq), ein leerer Body als {"ok": true}. Andere Antworten (PDF, Bilder) gehen unverändert nach stdout, aber nur, wenn stdout kein Terminal ist. --paginate lädt bei Listen der Spec alle Seiten (1 Anfrage je Seite, ohne page_size 100 je Seite) und führt sie zusammen wie --all; ab 10 Seiten Warnung, --max-pages begrenzt.

Verwendung:
  papierkram api <METHOD> <pfad> [Optionen]

Beispiele:
  papierkram api GET /info
  papierkram api GET '/income/invoices?page=2' -f page_size=50 -o json
  papierkram api GET /contact/companies --paginate --jq '.entries[].name'
  papierkram api POST /income/invoices/12/deliver -F send_via=email -f email.recipient=kunde@example.org -f email.subject='Ihre Rechnung' -F email.body=@mail.txt --yes
  papierkram api PUT /projects/5 --input projekt.json --validate --dry-run
  papierkram api DELETE /tracker/time_entries/7 --yes

Optionen:
  -F, --field Texte       Feld mit Typ: true, false, null, Zahl, @datei, @- (Standardeingabe), sonst Text (bei GET und DELETE ohne --input Query-Parameter)
  -h, --help              Hilfe zu api
      --input Text        Body als JSON-Datei oder - für die Standardeingabe; -f/-F überschreiben einzelne Felder
      --max-pages Zahl    mit --paginate höchstens so viele Seiten laden
      --paginate          bei GET alle Seiten einer Liste laden und zusammenführen (1 Anfrage je Seite)
  -f, --raw-field Texte   Feld als Text: key=value (Punkte verschachteln; bei GET und DELETE ohne --input Query-Parameter)
      --validate          Body vor dem Senden gegen das Request-Schema der Spec prüfen

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

- [papierkram](papierkram.md) – Kommandozeile für die Papierkram-API
