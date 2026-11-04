# Release

Wie ein Release von papierkram-cli entsteht. Die Konfiguration steht in [`.goreleaser.yaml`](../.goreleaser.yaml) (GoReleaser v2, fest auf v2.19.0). Ein Release ist nach außen wirksam (GitHub-Release, Homebrew-Tap, Scoop-Bucket) und geschieht nur auf ausdrücklichen Auftrag von Pascal. Normalweg: Tag pushen, der Workflow [`release.yml`](../.github/workflows/release.yml) veröffentlicht. Für alles andere gibt es den Snapshot, der nichts veröffentlicht.

## Was ein Release erzeugt

- Archive für darwin, linux und windows, je amd64 und arm64: `tar.gz`, unter Windows `zip`. Jedes Archiv enthält das Programm, `LICENSE`, `README.md`, `CHANGELOG.md`, `THIRD_PARTY_NOTICES.md`, die Vervollständigung (`completions/`: bash, zsh, fish, PowerShell) und die Man-Pages (`man/`, Abschnitt 1, deutsch).
- Pakete `deb` und `rpm` für amd64 und arm64: Programm unter `/usr/bin/papierkram`, Kurzform `/usr/bin/pk` als symbolischer Link, Man-Pages unter `/usr/share/man/man1/`, Vervollständigung für bash (`/usr/share/bash-completion/completions/`), zsh (`/usr/share/zsh/site-functions/`) und fish (`/usr/share/fish/vendor_completions.d/`), Lizenzen unter `/usr/share/doc/papierkram/`.
- `checksums.txt` (SHA-256 aller Archive und Pakete).
- Homebrew-Cask `papierkram` in `Padrio/homebrew-tap` (`brew install Padrio/tap/papierkram`).
- Scoop-Manifest `papierkram` in `Padrio/scoop-bucket` (`scoop install papierkram`).

Vervollständigung und Man-Pages entstehen erst beim Release: die Hooks `scripts/completions.sh` und `scripts/manpages.sh` bauen das Programm einmal in ein temporäres Verzeichnis und schreiben nach `completions/` und `man/` im Repository. Beide Verzeichnisse und `dist/` stehen in `.gitignore`. Beide Befehle (`completion`, `gen-docs --man`) senden keine Anfrage.

Die Builds sind reproduzierbar: `-trimpath`, `CGO_ENABLED=0`, Dateizeiten und `buildinfo.Date` aus dem Commit-Datum. Zwei Läufe auf demselben Commit ergeben dieselbe `checksums.txt`.

## Voraussetzungen (einmalig, Pascal)

1. Repository `Padrio/homebrew-tap` anlegen (öffentlich, mit Standard-Branch). Der Cask landet dort unter `Casks/papierkram.rb`.
2. Repository `Padrio/scoop-bucket` anlegen (öffentlich, mit Standard-Branch). Das Manifest landet dort als `papierkram.json`.
3. Fine-grained Personal Access Token mit Zugriff nur auf diese beiden Repositories und der Berechtigung „Contents: Read and write“. Als Actions-Secret `PACKAGES_GITHUB_TOKEN` im Repository `Padrio/papierkram-cli` hinterlegen. Fehlt es, bricht der Workflow vor GoReleaser ab.
4. Das GitHub-Release selbst schreibt der Workflow mit dem automatischen `GITHUB_TOKEN` (`permissions: contents: write` nur im Release-Job); dafür ist nichts anzulegen.

## Release-Schritte

1. `main` ist grün (CI inklusive Job `snapshot`), der Arbeitsbaum ist sauber.
2. In `CHANGELOG.md` den Abschnitt `## [Unreleased]` in `## [X.Y.Z] - JJJJ-MM-TT` umbenennen, einen neuen leeren `## [Unreleased]` darüber anlegen und die Links am Dateiende anpassen. Committen (`docs: release vX.Y.Z`), pushen und die CI abwarten. Prüfen, dass die Notizen gefunden werden:

   ```sh
   sh scripts/release-notes.sh vX.Y.Z
   ```

3. Tag setzen und pushen (SemVer, mit `v`):

   ```sh
   git tag -a vX.Y.Z -m "vX.Y.Z"
   git push origin vX.Y.Z
   ```

4. Der Push des Tags startet `.github/workflows/release.yml`. Der Workflow läuft nur bei Tags `v*` im Repository `Padrio/papierkram-cli`, nie bei Branches, Pull Requests oder in Forks. Er zieht mit `scripts/release-notes.sh` den Abschnitt des Tags aus `CHANGELOG.md` (fehlt der Abschnitt oder ist er leer, bricht er ab), prüft, dass `PACKAGES_GITHUB_TOKEN` gesetzt ist, und führt GoReleaser v2.19.0 aus: `release --clean --release-notes <Datei>`.
5. Nachbearbeitung Scoop (siehe unten), danach prüfen: Release-Seite mit 6 Archiven, 4 Paketen und `checksums.txt`; `brew install Padrio/tap/papierkram` und `pk version`; `scoop install papierkram` und `pk version`.

Die Release-Notizen kommen aus `CHANGELOG.md` (Keep a Changelog). Der Abschnitt `changelog` in `.goreleaser.yaml` (Git-Log ohne `docs`, `test`, `ci`, `chore`) ist nur der Rückfall für einen Lauf ohne `--release-notes`. Er bleibt eingeschaltet, weil GoReleaser `--release-notes` bei abgeschaltetem Changelog ignoriert.

Tags mit Vorabversion (z. B. `v1.0.0-rc.1`) erzeugen das GitHub-Release, aktualisieren Tap und Bucket aber nicht (`skip_upload: auto`). Auch dafür braucht `CHANGELOG.md` einen Abschnitt `## [1.0.0-rc.1]`.

### Rückfall: lokaler Release

Nur wenn der Workflow nicht laufen kann (z. B. GitHub Actions gestört) und der Tag schon gepusht ist. Beide Tokens kommen aus der Shell, nicht aus Dateien: `GITHUB_TOKEN` mit Schreibrecht auf `Padrio/papierkram-cli` (z. B. `"$(gh auth token)"`) und derselbe PAT wie im Secret `PACKAGES_GITHUB_TOKEN`.

```sh
git checkout vX.Y.Z
sh scripts/release-notes.sh vX.Y.Z > /tmp/papierkram-notes.md
GITHUB_TOKEN=… PACKAGES_GITHUB_TOKEN=… \
  go run github.com/goreleaser/goreleaser/v2@v2.19.0 release --clean --release-notes /tmp/papierkram-notes.md
```

## Snapshot (ohne Veröffentlichung)

```sh
make snapshot
# gleichbedeutend:
go run github.com/goreleaser/goreleaser/v2@v2.19.0 release --snapshot --clean
```

Der Snapshot baut alles nach `dist/`, veröffentlicht nichts und braucht kein Token; Cask und Manifest liegen nur unter `dist/homebrew/` und `dist/scoop/`. Die Version lautet `<nächster Patch>-snapshot+<Commit>`, z. B. `0.0.1-snapshot+351a12c`. Die Konfiguration allein prüft `go run github.com/goreleaser/goreleaser/v2@v2.19.0 check`.

Die CI führt denselben Snapshot im Job `snapshot` aus (`.github/workflows/ci.yml`, ohne Token) und prüft danach: 6 Archive, `deb` und `rpm` für amd64 und arm64, `checksums.txt`, den Link `/usr/bin/pk` im `deb`, Man-Pages und Vervollständigung in Paket und Archiv, die Vervollständigung von `papierkram` und `pk` in bash, zsh, fish und PowerShell (`scripts/check-completions.sh`), `scripts/release-notes.sh` sowie einen sauberen Arbeitsbaum. Lokal prüft `sh scripts/check-completions.sh` die installierten Shells und überspringt die übrigen.

## Kurzform `pk`

| Kanal | Wie `pk` entsteht | Vervollständigung für `pk` | Nachbearbeitung |
| --- | --- | --- | --- |
| deb, rpm | symbolischer Link `/usr/bin/pk` → `papierkram` (nfpm) | bash, zsh, fish | keine |
| Homebrew | zweite `binary`-Zeile im Cask: `binary "papierkram", target: "pk"` | bash, zsh, fish | keine |
| Scoop | Shim über einen Alias in `bin` | PowerShell, nach Einrichten im Profil | ja, nach jedem Release |
| Archiv | keiner | Skripte in `completions/` registrieren `pk` mit | – |
| `go install` | keiner | Skripte registrieren `pk` mit, aber es gibt kein `pk` | – |

**Homebrew:** GoReleaser v2.19 kennt für `homebrew_casks.binaries` kein Linkziel. Die zweite `binary`-Zeile steht deshalb in `custom_block` und landet bei jedem Release automatisch im Cask. `custom_block` steht im Cask vor `version`; das funktioniert, `brew style` kann aber die Reihenfolge der Abschnitte bemängeln. Der Cask ersetzt die früheren Formeln (`brews`), die in v2.19 als veraltet gelten; `brew install Padrio/tap/papierkram` findet den Cask ohne `--cask`. Weil die Programme nicht notarisiert sind, entfernt der Cask nach der Installation unter macOS das Quarantäne-Attribut (`postflight`).

**Scoop:** GoReleaser schreibt `"bin": ["papierkram.exe"]` und hat kein Feld für Aliase oder zusätzliche Shims. Nach jedem Release im Repository `Padrio/scoop-bucket` in `papierkram.json` bei **beiden** Architekturen (`64bit` und `arm64`) den Eintrag ersetzen:

```json
"bin": [
    "papierkram.exe",
    ["papierkram.exe", "pk"]
]
```

Committen und pushen (`papierkram: add pk shim`). Das nächste Release überschreibt das Manifest wieder; der Schritt ist also jedes Mal nötig. Prüfen mit `scoop update papierkram` und `pk version`.

**Vervollständigung:** `papierkram completion <shell>` gibt ein Skript aus, das auch `pk` registriert (eine Quelle: `internal/cli/completion.go`); `scripts/completions.sh` ruft nur diesen Befehl auf. bash, zsh und PowerShell rufen das Programm auf, das in der Befehlszeile steht (`pk __complete …`); fish vervollständigt `pk` über `complete -w papierkram` und ruft dabei `papierkram __complete …` auf. Beide Namen sind dasselbe Programm, daher genügt die Registrierung:

- bash: `complete -o default -F __start_papierkram pk` (wie bei `papierkram` mit `-o nospace` für eine bash ohne `compopt`).
- zsh: Kopfzeilen `#compdef papierkram pk` und `compdef _papierkram papierkram pk`. Eine angehängte `compdef`-Zeile würde bei Installation über `fpath` nur innerhalb der Funktion laufen; deshalb ändert der Befehl die ersten beiden Zeilen von cobra.
- fish: `complete -c pk -w papierkram`.
- PowerShell: `Register-ArgumentCompleter -CommandName 'pk'` mit demselben Skriptblock.

bash-completion und fish laden ein Skript erst beim ersten Tab für den jeweiligen Befehlsnamen. Die Pakete legen daher zusätzlich `/usr/share/bash-completion/completions/pk` → `papierkram` und `/usr/share/fish/vendor_completions.d/pk.fish` → `papierkram.fish` an; der Cask verlinkt das fish-Skript ein zweites Mal als `pk.fish`. zsh bindet `pk` über die `#compdef`-Zeile, bash unter Homebrew lädt `etc/bash_completion.d` beim Start. Wer selbst einrichtet (`completion --help`), braucht für fish zusätzlich `pk.fish` als zweiten Namen. Fehlt eine der Zeilen von cobra, auf die sich der Befehl stützt (z. B. nach einem cobra-Update mit anderen Funktionsnamen), meldet `completion` einen Fehler statt ein Skript ohne `pk` auszugeben; `TestCompletionShortForm` prüft die Zeilen je Shell, und der Release bricht ab.

**Namenskonflikt:** [pkram](https://github.com/mickeyl/pkram) legt ebenfalls `pk` an (README, „Kurzform `pk`“). Ist `pk` schon von einem anderen Paket belegt, brechen Homebrew und deb/rpm die Installation mit einem Dateikonflikt ab; `papierkram` bleibt immer eindeutig.
