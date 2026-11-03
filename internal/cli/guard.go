package cli

import (
	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/config"
)

// guardConfigWrite blocks every write to config or keychain while the
// hard read-only mode PAPIERKRAM_READ_ONLY=1 is set (spec 8.3). It must be
// called before any file access.
func (a *App) guardConfigWrite() error {
	if a.Env("PAPIERKRAM_READ_ONLY") == "1" {
		return clierr.New(clierr.CodeReadOnly,
			"Schreibschutz aktiv (PAPIERKRAM_READ_ONLY=1): Konfiguration kann nicht geändert werden",
			"Die Umgebungsvariable wurde von dem gesetzt, der diesen Prozess gestartet hat, und lässt sich weder per Flag noch per Config oder Projektdatei aufheben.")
	}
	return nil
}

// configPath returns the config.toml path; PAPIERKRAM_CONFIG_DIR overrides
// the app's config directory.
func (a *App) configPath() (string, error) {
	dir := a.ConfigDir
	if v := a.Env("PAPIERKRAM_CONFIG_DIR"); v != "" {
		dir = v
	}
	return config.Path(dir)
}
