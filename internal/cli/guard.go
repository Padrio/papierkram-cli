package cli

import (
	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/config"
	"github.com/Padrio/papierkram-cli/internal/output"
)

// annotWritesConfig marks commands that write config.toml, the keychain or
// the project file (context create|use|delete|rename, auth login|logout,
// local init). The root PersistentPreRunE runs guardConfigWrite for every
// command carrying it; commands never call the guard themselves.
const annotWritesConfig = "pk:writes-config"

// writesConfig is the Annotations value for config-writing commands.
func writesConfig() map[string]string { return map[string]string{annotWritesConfig: "true"} }

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
		output.Warn(a.Err, w)
	}
}
