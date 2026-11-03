package cli

import (
	"fmt"

	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/config"
)

// guardConfigWrite blocks every write to config or keychain while the
// hard read-only mode PAPIERKRAM_READ_ONLY is enabled (spec 8.3, see
// config.EnvReadOnly). It must be called before any file access.
func (a *App) guardConfigWrite() error {
	if config.EnvReadOnly(a.Env) {
		return clierr.New(clierr.CodeReadOnly,
			"Schreibschutz aktiv (PAPIERKRAM_READ_ONLY ist gesetzt): Konfiguration kann nicht geändert werden",
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

// warnReadOnlyEnv prints a warning when PAPIERKRAM_READ_ONLY holds a value
// that is neither clearly on nor off (the mode is enabled then).
func (a *App) warnReadOnlyEnv() {
	if w := config.EnvReadOnlyWarning(a.Env); w != "" {
		_, _ = fmt.Fprintln(a.Err, "Warnung: "+w)
	}
}
