# Release

Wie ein Release von papierkram-cli entsteht. Die Konfiguration steht in [`.goreleaser.yaml`](../.goreleaser.yaml) (GoReleaser v2, fest auf v2.19.0). Ein Release ist nach außen wirksam (GitHub-Release, Homebrew-Tap, Scoop-Bucket) und geschieht nur auf ausdrücklichen Auftrag von Pascal. Für alles andere gibt es den Snapshot, der nichts veröffentlicht.

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
3. Fine-grained Personal Access Token mit Zugriff nur auf diese beiden Repositories und der Berechtigung „Contents: Read and write“. Als Secret `PACKAGES_GITHUB_TOKEN` im Repository `Padrio/papierkram-cli` hinterlegen und für einen lokalen Release in der Shell bereithalten.
4. Für das GitHub-Release selbst ein Token mit Schreibrecht auf `Padrio/papierkram-cli` (Contents: Read and write) als `GITHUB_TOKEN`, z. B. `GITHUB_TOKEN="$(gh auth token)"`.

## Release-Schritte

1. `main` ist grün (CI inklusive Job `snapshot`), der Arbeitsbaum ist sauber.
2. In `CHANGELOG.md` den Abschnitt `## [Unreleased]` in `## [X.Y.Z] - JJJJ-MM-TT` umbenennen, einen neuen leeren `## [Unreleased]` darüber anlegen und die Links am Dateiende anpassen. Committen (`docs: release vX.Y.Z`) und pushen.
3. Tag setzen und pushen (SemVer, mit `v`):

   ```sh
   git tag -a vX.Y.Z -m "vX.Y.Z"
   git push origin vX.Y.Z
   ```

4. Release-Notizen aus `CHANGELOG.md` ziehen (nur der Abschnitt des Tags):

   ```sh
   awk -v v="X.Y.Z" 'index($0, "## [" v "]") == 1 {on=1; next} on && /^## \[/ {exit} on && /^\[[^]]*\]: / {next} on {print}' CHANGELOG.md > /tmp/papierkram-notes.md
   ```

5. Release ausführen:

   ```sh
   GITHUB_TOKEN=… PACKAGES_GITHUB_TOKEN=… \
     go run github.com/goreleaser/goreleaser/v2@v2.19.0 release --clean --release-notes /tmp/papierkram-notes.md
   ```

   Die Release-Notizen kommen aus `CHANGELOG.md` (Keep a Changelog). Der Abschnitt `changelog` in `.goreleaser.yaml` (Git-Log ohne `docs`, `test`, `ci`, `chore`) ist nur der Rückfall für einen Lauf ohne `--release-notes`. Er bleibt eingeschaltet, weil GoReleaser `--release-notes` bei abgeschaltetem Changelog ignoriert.

6. Nachbearbeitung Scoop (siehe unten), danach prüfen: Release-Seite mit 6 Archiven, 4 Paketen und `checksums.txt`; `brew install Padrio/tap/papierkram` und `pk version`; `scoop install papierkram` und `pk version`.

Tags mit Vorabversion (z. B. `v1.0.0-rc.1`) erzeugen das GitHub-Release, aktualisieren Tap und Bucket aber nicht (`skip_upload: auto`).

## Snapshot (ohne Veröffentlichung)

```sh
make snapshot
# gleichbedeutend:
go run github.com/goreleaser/goreleaser/v2@v2.19.0 release --snapshot --clean
```

Der Snapshot baut alles nach `dist/`, veröffentlicht nichts und braucht kein Token; Cask und Manifest liegen nur unter `dist/homebrew/` und `dist/scoop/`. Die Version lautet `<nächster Patch>-snapshot+<Commit>`, z. B. `0.0.1-snapshot+351a12c`. Die Konfiguration allein prüft `go run github.com/goreleaser/goreleaser/v2@v2.19.0 check`.

Die CI führt denselben Snapshot im Job `snapshot` aus (`.github/workflows/ci.yml`, ohne Token) und prüft danach: 6 Archive, `deb` und `rpm` für amd64 und arm64, `checksums.txt`, den Link `/usr/bin/pk` im `deb`, Man-Pages und Vervollständigung in Paket und Archiv sowie einen sauberen Arbeitsbaum.

## Kurzform `pk`

| Kanal | Wie `pk` entsteht | Nachbearbeitung |
| --- | --- | --- |
| deb, rpm | symbolischer Link `/usr/bin/pk` → `papierkram` (nfpm) | keine |
| Homebrew | zweite `binary`-Zeile im Cask: `binary "papierkram", target: "pk"` | keine |
| Scoop | Shim über einen Alias in `bin` | ja, nach jedem Release |
| `go install`, Archiv | keiner | – |

**Homebrew:** GoReleaser v2.19 kennt für `homebrew_casks.binaries` kein Linkziel. Die zweite `binary`-Zeile steht deshalb in `custom_block` und landet bei jedem Release automatisch im Cask. `custom_block` steht im Cask vor `version`; das funktioniert, `brew style` kann aber die Reihenfolge der Abschnitte bemängeln. Der Cask ersetzt die früheren Formeln (`brews`), die in v2.19 als veraltet gelten; `brew install Padrio/tap/papierkram` findet den Cask ohne `--cask`. Weil die Programme nicht notarisiert sind, entfernt der Cask nach der Installation unter macOS das Quarantäne-Attribut (`postflight`).

**Scoop:** GoReleaser schreibt `"bin": ["papierkram.exe"]` und hat kein Feld für Aliase oder zusätzliche Shims. Nach jedem Release im Repository `Padrio/scoop-bucket` in `papierkram.json` bei **beiden** Architekturen (`64bit` und `arm64`) den Eintrag ersetzen:

```json
"bin": [
    "papierkram.exe",
    ["papierkram.exe", "pk"]
]
```

Committen und pushen (`papierkram: add pk shim`). Das nächste Release überschreibt das Manifest wieder; der Schritt ist also jedes Mal nötig. Prüfen mit `scoop update papierkram` und `pk version`.

**Vervollständigung:** Die Skripte in `completions/` sind für den Befehlsnamen `papierkram` registriert. `pk` funktioniert als Befehl, hat aber keine eigene Tab-Vervollständigung.

**Namenskonflikt:** [pkram](https://github.com/mickeyl/pkram) legt ebenfalls `pk` an (README, „Kurzform `pk`“). Ist `pk` schon von einem anderen Paket belegt, brechen Homebrew und deb/rpm die Installation mit einem Dateikonflikt ab; `papierkram` bleibt immer eindeutig.
