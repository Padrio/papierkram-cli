# papierkram completion

Skript für die Shell-Vervollständigung ausgeben

```text
Gibt das Skript für die Shell-Vervollständigung aus (bash, zsh, fish oder powershell).

Vervollständigt werden Befehle, Flags, erlaubte Werte (z. B. -o, --state, die 94 Ausgabekategorien bei --item category=), Kontextnamen aus der config.toml und IDs. Kategorien mit Komma (z. B. "Gas, Strom, Wasser") stehen in Anführungszeichen: dafür mit category=" beginnen.

IDs (z. B. papierkram invoice describe <Tab>, --company <Tab>) lädt die Vervollständigung per Listenabfrage aus der API: erste Seite, bis zu 100 Einträge, angezeigt als ID mit Name. Jede Abfrage kostet 1 Credit. Das Ergebnis wird je Kontext und Ressource 60 Sekunden im Cache-Verzeichnis gehalten; so lange zeigt die Vervollständigung die IDs auch ohne neue Anfrage. auth logout, context delete und context rename löschen den Cache des Kontexts. Ohne Token wird nichts geladen; bei Fehlern oder nach 3 Sekunden gibt es keine IDs, aber nie eine Meldung. Das Token wird erst gelesen, wenn eine Anfrage nötig ist; bei gesperrtem Schlüsselbund (Linux) kann dabei ein Entsperrdialog erscheinen, completion_lookup = false vermeidet jeden Zugriff. Abschalten je Kontext in der config.toml:

  [contexts.<name>]
  completion_lookup = false

Dann sendet die Vervollständigung keine Anfrage und liest keinen Cache.

Jedes Skript vervollständigt auch die Kurzform pk, die Homebrew, Scoop, deb und rpm installieren; deren Pakete richten die Vervollständigung selbst ein (Scoop ausgenommen).

Einrichten (danach eine neue Shell starten):

bash (braucht das Paket bash-completion):
  aktuelle Shell:  source <(papierkram completion bash)
  dauerhaft Linux: papierkram completion bash > /etc/bash_completion.d/papierkram
  dauerhaft macOS: papierkram completion bash > $(brew --prefix)/etc/bash_completion.d/papierkram

zsh (falls noch nicht aktiv, vorher einmal: echo "autoload -U compinit; compinit" >> ~/.zshrc):
  papierkram completion zsh > "${fpath[1]}/_papierkram"

fish (lädt die Datei nach dem Befehlsnamen, daher für pk ein zweiter Name):
  papierkram completion fish > ~/.config/fish/completions/papierkram.fish
  ln -s papierkram.fish ~/.config/fish/completions/pk.fish

PowerShell:
  aktuelle Sitzung: papierkram completion powershell | Out-String | Invoke-Expression
  dauerhaft: dieselbe Zeile in das Profil ($PROFILE) eintragen

Gibt nur das Skript aus und sendet keine Anfrage.

Verwendung:
  papierkram completion <shell> [Optionen]

Beispiele:
  papierkram completion zsh > "${fpath[1]}/_papierkram"
  papierkram completion fish > ~/.config/fish/completions/papierkram.fish
  papierkram completion bash > /etc/bash_completion.d/papierkram

Optionen:
  -h, --help   Hilfe zu completion

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
