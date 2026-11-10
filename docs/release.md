# Release

Wie ein Release von papierkram-cli entsteht. Die Konfiguration steht in [`.goreleaser.yaml`](../.goreleaser.yaml) (GoReleaser v2, fest auf v2.19.0). Ein Release ist nach außen wirksam (GitHub-Release, Homebrew-Tap, Branch im winget-Fork) und geschieht nur auf ausdrücklichen Auftrag von Pascal. Normalweg: Tag pushen, der Workflow [`release.yml`](../.github/workflows/release.yml) veröffentlicht. Für alles andere gibt es den Snapshot, der nichts veröffentlicht.

## Was ein Release erzeugt

- Archive für darwin, linux und windows, je amd64 und arm64: `tar.gz`, unter Windows `zip`. Jedes Archiv enthält das Programm, `LICENSE`, `README.md`, `CHANGELOG.md`, `THIRD_PARTY_NOTICES.md`, die Vervollständigung (`completions/`: bash, zsh, fish, PowerShell) und die Man-Pages (`man/`, Abschnitt 1, deutsch).
- Pakete `deb` und `rpm` für amd64 und arm64: Programm unter `/usr/bin/papierkram`, Kurzform `/usr/bin/pk` als symbolischer Link, Man-Pages gzip-komprimiert unter `/usr/share/man/man1/`, Vervollständigung für bash (`/usr/share/bash-completion/completions/`), zsh (`deb`: `/usr/share/zsh/vendor-completions/`, weil das zsh von Debian und Ubuntu `/usr/share/zsh/site-functions/` nicht durchsucht; `rpm`: `/usr/share/zsh/site-functions/`) und fish (`/usr/share/fish/vendor_completions.d/`), Lizenzen unter `/usr/share/doc/papierkram/`.
- Je Archiv eine SBOM `<archiv>.sbom.json` (SPDX JSON, von syft aus dem Archiv erzeugt; enthält die Go-Module des Programms).
- `checksums.txt` (SHA-256 aller Archive, Pakete und SBOMs, 16 Zeilen).
- Homebrew-Cask `papierkram` in `Padrio/homebrew-tap` (`brew install Padrio/tap/papierkram`).
- winget-Manifest `Padrio.Papierkram` (drei Dateien: `Padrio.Papierkram.yaml`, `….installer.yaml`, `….locale.de-DE.yaml`; Typ `zip` mit `portable` darin, je ein Eintrag für x64 und arm64, Befehl `papierkram`) auf dem Branch `papierkram-<version>` im Fork `Padrio/winget-pkgs`, unter `manifests/p/Padrio/Papierkram/<version>/`. GoReleaser legt den Branch vom Standard-Branch des Forks an. Den Pull Request gegen `microsoft/winget-pkgs` stellt nicht GoReleaser (ein fine-grained Token kann dort keinen öffnen), sondern ein Mensch nach dem Release (siehe [Release-Schritte](#release-schritte), Schritt 6). `winget install Padrio.Papierkram` funktioniert erst, wenn Microsoft ihn angenommen hat.
- Kein Scoop-Manifest: Scoop ist ausgesetzt (siehe [Scoop (ausgesetzt)](#scoop-ausgesetzt)); unter Windows gilt bis zur Aufnahme in winget das `zip`-Archiv.

Vervollständigung und Man-Pages entstehen erst beim Release: die Hooks `scripts/completions.sh` und `scripts/manpages.sh` bauen das Programm einmal in ein temporäres Verzeichnis und schreiben nach `completions/`, `man/` und `man-gz/` (die Man-Pages mit `gzip -9 -n` komprimiert, für die Pakete) im Repository. Diese Verzeichnisse und `dist/` stehen in `.gitignore`. Beide Befehle (`completion`, `gen-docs --man`) senden keine Anfrage.

Die Builds sind reproduzierbar: `-trimpath`, `CGO_ENABLED=0`, Dateizeiten und `buildinfo.Date` aus dem Commit-Datum, Archiv-Einträge mit Besitzer und Gruppe `root` und festen Modi (unabhängig von Benutzer und umask). Zwei Läufe auf demselben Commit ergeben dieselbe `checksums.txt` bis auf die 6 Zeilen der SBOMs: syft schreibt in jede SBOM die Erzeugungszeit und eine zufällige ID (`documentNamespace`), Archive und Pakete bleiben gleich. Auf einer anderen Maschine gilt das nur mit derselben Go-Version und, für `deb` und `rpm`, derselben gzip-Implementierung (GNU gzip auf dem Runner, Apple gzip unter macOS).

## Voraussetzungen (einmalig, Pascal)

1. Repository `Padrio/homebrew-tap` anlegen (öffentlich, mit Standard-Branch; angelegt am 2026-11-09). Der Cask landet dort unter `Casks/papierkram.rb`.
2. Fork von `microsoft/winget-pkgs` anlegen; er heißt dann `Padrio/winget-pkgs` (angelegt):

   ```sh
   gh repo fork microsoft/winget-pkgs --clone=false --default-branch-only
   ```

3. Fine-grained Personal Access Token mit Zugriff nur auf `Padrio/homebrew-tap` und `Padrio/winget-pkgs` und der Berechtigung „Contents: Read and write“ (gesetzt am 2026-11-09 für den Tap; für den Fork den vorhandenen Token bearbeiten: unter „Repository access“ `Padrio/winget-pkgs` hinzufügen, kein neuer Token). Als Actions-Secret `PACKAGES_GITHUB_TOKEN` im Repository `Padrio/papierkram-cli` hinterlegen. Fehlt es, bricht der Workflow vor GoReleaser ab. Kann es eines der beiden Repositories nicht beschreiben, ebenso: Der Workflow legt vorab in beiden mit `scripts/check-fork-push.sh` einen Branch `pk-write-check-<Lauf>-<Versuch>` an und löscht ihn wieder; scheitert das Anlegen, endet er mit „PACKAGES_GITHUB_TOKEN kann <owner/repo> nicht beschreiben (docs/release.md, Voraussetzungen)“, bevor etwas veröffentlicht ist. Scheitert nur das Löschen, warnt er und läuft weiter; der Branch ist harmlos und lässt sich mit dem genannten `gh api -X DELETE …` entfernen. Von Hand prüfen (das Token aus der Shell, nicht aus einer Datei):

   ```sh
   PACKAGES_GITHUB_TOKEN=… sh scripts/check-fork-push.sh Padrio/winget-pkgs
   ```

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

4. Der Push des Tags startet `.github/workflows/release.yml`. Der Workflow läuft nur bei Tags `v*` im Repository `Padrio/papierkram-cli`, nie bei Branches, Pull Requests oder in Forks. Er zieht mit `scripts/release-notes.sh` den Abschnitt des Tags aus `CHANGELOG.md` (nur SemVer-Tags wie `v1.2.3` oder `v1.2.3-rc.1`; fehlt der Abschnitt oder ist er leer, bricht er ab; `## [` und Link-Definitionen in Code-Blöcken gehören zu den Notizen; ein nicht geschlossener Code-Block bricht mit Exit 4 ab), prüft, dass `PACKAGES_GITHUB_TOKEN` gesetzt ist und `Padrio/homebrew-tap` und `Padrio/winget-pkgs` beschreiben kann (siehe [Voraussetzungen](#voraussetzungen-einmalig-pascal)), lädt syft (`anchore/sbom-action/download-syft`) und führt GoReleaser v2.19.0 aus: `release --clean --release-notes <Datei>`. Der Job bricht nach 30 Minuten ab; der Checkout legt kein Token in `.git/config` ab (`persist-credentials: false`), GoReleaser veröffentlicht über die API.
5. Prüfen: Release-Seite mit 6 Archiven, 6 SBOMs (`*.sbom.json`), 4 Paketen und `checksums.txt` (16 Zeilen); `brew install Padrio/tap/papierkram` und `pk version`; die drei winget-Manifeste auf dem Branch im Fork:

   ```sh
   gh api "repos/Padrio/winget-pkgs/contents/manifests/p/Padrio/Papierkram/X.Y.Z?ref=papierkram-X.Y.Z" --jq '.[].name'
   ```

6. Pull Request gegen `microsoft/winget-pkgs` stellen (nicht bei Vorabversionen; mit dem eigenen `gh`-Login, nicht mit dem PAT). Titel nach der Vorlage des Repositorys: beim ersten Release `New package: Padrio.Papierkram version X.Y.Z`, danach `Update: Padrio.Papierkram to X.Y.Z`. Den Text aus der Vorlage [`.github/PULL_REQUEST_TEMPLATE.md`](https://github.com/microsoft/winget-pkgs/blob/master/.github/PULL_REQUEST_TEMPLATE.md) übernehmen und nur abhaken, was wirklich geprüft ist (`winget validate` und `winget install --manifest` gehen nur unter Windows):

   ```sh
   gh pr create --repo microsoft/winget-pkgs --base master --head Padrio:papierkram-X.Y.Z \
     --title "New package: Padrio.Papierkram version X.Y.Z" --body-file <datei>
   ```

   Die Prüfungen von Microsoft laufen im Pull Request (Validierung, Testinstallation, beim ersten Beitrag die CLA); Rückfragen dort beantworten. Nach der Annahme gilt `winget install Padrio.Papierkram`. Der Branch `papierkram-X.Y.Z` im Fork kann danach weg.

Die Release-Notizen kommen aus `CHANGELOG.md` (Keep a Changelog). Der Abschnitt `changelog` in `.goreleaser.yaml` (Git-Log ohne `docs`, `test`, `ci`, `chore`) ist nur der Rückfall für einen Lauf ohne `--release-notes`. Er bleibt eingeschaltet, weil GoReleaser `--release-notes` bei abgeschaltetem Changelog ignoriert.

Tags mit Vorabversion (z. B. `v1.0.0-rc.1`) erzeugen ein GitHub-Release, das als Vorabversion markiert ist und nie „latest“ wird (`release.prerelease: auto`), und aktualisieren weder den Tap noch den winget-Fork (`skip_upload: auto`). Auch dafür braucht `CHANGELOG.md` einen Abschnitt `## [1.0.0-rc.1]`.

### Rückfall: lokaler Release

Nur wenn der Workflow nicht laufen kann (z. B. GitHub Actions gestört) und der Tag schon gepusht ist. Beide Tokens kommen aus der Shell, nicht aus Dateien: `GITHUB_TOKEN` mit Schreibrecht auf `Padrio/papierkram-cli` (z. B. `"$(gh auth token)"`) und derselbe PAT wie im Secret `PACKAGES_GITHUB_TOKEN`. `syft` muss im `PATH` liegen (z. B. `brew install syft`).

```sh
git checkout vX.Y.Z
sh scripts/release-notes.sh vX.Y.Z > /tmp/papierkram-notes.md
export PACKAGES_GITHUB_TOKEN=…
sh scripts/check-fork-push.sh Padrio/homebrew-tap
sh scripts/check-fork-push.sh Padrio/winget-pkgs
GITHUB_TOKEN=… \
  go run github.com/goreleaser/goreleaser/v2@v2.19.0 release --clean --release-notes /tmp/papierkram-notes.md
```

## Snapshot (ohne Veröffentlichung)

```sh
make snapshot
# gleichbedeutend:
go run github.com/goreleaser/goreleaser/v2@v2.19.0 release --snapshot --clean
```

Der Snapshot baut alles nach `dist/`, veröffentlicht nichts und braucht kein Token; der Cask liegt nur unter `dist/homebrew/`, die winget-Manifeste unter `dist/winget/manifests/p/Padrio/Papierkram/<version>/` (mit der Snapshot-Version in den URLs, die es nicht gibt). Für die SBOMs muss `syft` im `PATH` liegen (z. B. `brew install syft`); fehlt es, bricht der Snapshot ab. Die Version lautet `<nächster Patch>-snapshot+<Commit>`, z. B. `0.0.1-snapshot+351a12c`. Die Konfiguration allein prüft `go run github.com/goreleaser/goreleaser/v2@v2.19.0 check`.

Die CI führt denselben Snapshot im Job `snapshot` aus (`.github/workflows/ci.yml`, ohne Token) (syft aus `anchore/sbom-action/download-syft`) und prüft danach: 6 Archive, `deb` und `rpm` für amd64 und arm64, `checksums.txt` mit 16 Zeilen, je Archiv eine SBOM mit `spdxVersion` (auch in `checksums.txt`), die drei winget-Manifeste (`zip` mit `portable` für beide Architekturen), den Link `/usr/bin/pk` im `deb`, Man-Pages (`.1.gz`) und Vervollständigung in Paket und Archiv (zsh im `deb` unter `vendor-completions`), die Vervollständigung von `papierkram` und `pk` in bash, zsh, fish und PowerShell (`scripts/check-completions.sh`), `scripts/release-notes.sh`, `scripts/check-fork-push.sh` gegen ein vorgetäuschtes `gh` ohne Netz (`CHECK_FORK_PUSH_GH`; Erfolg, Anlegen scheitert, Löschen scheitert, Token fehlt, Repository nicht lesbar; das Token darf nur in `GH_TOKEN` stehen, nie in den Argumenten oder der Ausgabe) sowie einen sauberen Arbeitsbaum. Lokal prüft `sh scripts/check-completions.sh` die installierten Shells und überspringt die übrigen. Alle Actions der Workflows sind per Commit-SHA gepinnt, der Release-Tag steht als Kommentar dahinter; `scripts/check-action-pins.sh` (Job `checks`) lehnt jede andere Form ab, Dependabot aktualisiert die Pins wöchentlich.

## Kurzform `pk`

| Kanal | Wie `pk` entsteht | Vervollständigung für `pk` | Nachbearbeitung |
| --- | --- | --- | --- |
| deb, rpm | symbolischer Link `/usr/bin/pk` → `papierkram` (nfpm) | bash, zsh, fish | keine |
| Homebrew | zweite `binary`-Zeile im Cask: `binary "papierkram", target: "pk"` | bash, zsh, fish | keine |
| Archiv | keiner | Skripte in `completions/` registrieren `pk` mit | – |
| winget | keiner (GoReleaser kennt je Programm nur einen `PortableCommandAlias`) | – | – |
| `go install` | keiner | Skripte registrieren `pk` mit, aber es gibt kein `pk` | – |

**Homebrew:** GoReleaser v2.19 kennt für `homebrew_casks.binaries` kein Linkziel. Die zweite `binary`-Zeile steht deshalb in `custom_block` und landet bei jedem Release automatisch im Cask. `custom_block` steht im Cask vor `version`; das funktioniert, `brew style` kann aber die Reihenfolge der Abschnitte bemängeln. Der Cask ersetzt die früheren Formeln (`brews`), die in v2.19 als veraltet gelten; `brew install Padrio/tap/papierkram` findet den Cask ohne `--cask`. Weil die Programme nicht notarisiert sind, entfernt der Cask nach der Installation unter macOS das Quarantäne-Attribut (`postflight`).

**Vervollständigung:** `papierkram completion <shell>` gibt ein Skript aus, das auch `pk` registriert (eine Quelle: `internal/cli/completion.go`); `scripts/completions.sh` ruft nur diesen Befehl auf. bash, zsh und PowerShell rufen das Programm auf, das in der Befehlszeile steht (`pk __complete …`); fish vervollständigt `pk` über `complete -w papierkram` und ruft dabei `papierkram __complete …` auf. Beide Namen sind dasselbe Programm, daher genügt die Registrierung:

- bash: `complete -o default -F __start_papierkram pk` (wie bei `papierkram` mit `-o nospace` für eine bash ohne `compopt`).
- zsh: Kopfzeilen `#compdef papierkram pk` und `compdef _papierkram papierkram pk`. Eine angehängte `compdef`-Zeile würde bei Installation über `fpath` nur innerhalb der Funktion laufen; deshalb ändert der Befehl die ersten beiden Zeilen von cobra.
- fish: `complete -c pk -w papierkram`.
- PowerShell: `Register-ArgumentCompleter -CommandName 'pk'` mit demselben Skriptblock.

bash-completion und fish laden ein Skript erst beim ersten Tab für den jeweiligen Befehlsnamen. Die Pakete legen daher zusätzlich `/usr/share/bash-completion/completions/pk` → `papierkram` und `/usr/share/fish/vendor_completions.d/pk.fish` → `papierkram.fish` an; der Cask verlinkt das fish-Skript ein zweites Mal als `pk.fish`. zsh bindet `pk` über die `#compdef`-Zeile (aus `vendor-completions` bzw. `site-functions`), bash unter Homebrew lädt `etc/bash_completion.d` beim Start. Wer selbst einrichtet (`completion --help`), braucht für fish zusätzlich `pk.fish` als zweiten Namen. Fehlt eine der Zeilen von cobra, auf die sich der Befehl stützt (z. B. nach einem cobra-Update mit anderen Funktionsnamen), meldet `completion` einen Fehler statt ein Skript ohne `pk` auszugeben; `TestCompletionShortForm` prüft die Zeilen je Shell, und der Release bricht ab.

**Namenskonflikt:** [pkram](https://github.com/mickeyl/pkram) legt ebenfalls `pk` an (README, „Kurzform `pk`“). Ist `pk` schon von einem anderen Paket belegt, brechen Homebrew und deb/rpm die Installation mit einem Dateikonflikt ab; `papierkram` bleibt immer eindeutig.

## Scoop (ausgesetzt)

Seit 2026-11-09 (Entscheidung Pascal) veröffentlicht der Release kein Scoop-Manifest; unter Windows gilt das `zip`-Archiv der Release-Seite. Wieder einschalten:

1. Repository `Padrio/scoop-bucket` anlegen (öffentlich, mit Standard-Branch) und es dem PAT hinter `PACKAGES_GITHUB_TOKEN` hinzufügen (Token-Einstellungen, „Repository access“; kein neuer Token nötig).
2. Den Abschnitt `scoops:` in `.goreleaser.yaml` wiederherstellen (`git log -S scoops -- .goreleaser.yaml` findet den Commit, der ihn entfernt hat) und README, Spec und diese Datei zurück auf Scoop stellen.
3. Nach jedem Release den Shim für `pk` von Hand nachziehen: GoReleaser schreibt `"bin": ["papierkram.exe"]` und hat kein Feld für Aliase. Im Repository `Padrio/scoop-bucket` in `papierkram.json` bei **beiden** Architekturen (`64bit` und `arm64`) den Eintrag ersetzen:

   ```json
   "bin": [
       "papierkram.exe",
       ["papierkram.exe", "pk"]
   ]
   ```

   Committen und pushen (`papierkram: add pk shim`); prüfen mit `scoop update papierkram` und `pk version`.
