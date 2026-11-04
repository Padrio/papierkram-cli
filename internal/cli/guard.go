package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/api"
	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/config"
	"github.com/Padrio/papierkram-cli/internal/output"
	"github.com/Padrio/papierkram-cli/internal/registry"
)

// annotWritesConfig marks commands that write config.toml, the keychain or
// the project file (context create|use|delete|rename, auth login|logout,
// local init). The root PersistentPreRunE runs guardConfigWrite for every
// command carrying it; commands never call the guard themselves.
const annotWritesConfig = "pk:writes-config"

// writesConfig is the Annotations value for config-writing commands.
func writesConfig() map[string]string { return map[string]string{annotWritesConfig: "true"} }

// guardConfigWrite blocks every write to config, keychain or project file
// while the hard read-only mode PAPIERKRAM_READ_ONLY is enabled (spec 8.3, see
// config.EnvReadOnly). It must be called before any file access.
func (a *App) guardConfigWrite() error {
	if config.EnvReadOnly(a.Env) {
		return clierr.New(clierr.CodeReadOnly,
			"Schreibschutz aktiv (PAPIERKRAM_READ_ONLY ist gesetzt): Konfiguration und Projektdatei können nicht geändert werden",
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

// annotClass carries the action class (registry.Class) of a registry
// command; checkDryRun reads it.
const annotClass = "pk:class"

// readsOnly is the Annotations value of meta commands that only read
// (version, context list|active, auth status): class read_only, so
// --dry-run is rejected there like on list and describe.
func readsOnly() map[string]string {
	return map[string]string{annotClass: string(registry.ClassReadOnly)}
}

// annotLocal marks commands that change only local state and send no
// request (time start, time discard, gen-docs): checkDryRun rejects
// --dry-run there, so it never runs them for real.
const annotLocal = "pk:local"

// checkDryRun runs in the root PersistentPreRunE, before configuration and
// requests (exit 2): --dry-run is only meaningful for API mutations. A
// registry command of class read_only rejects it, and so does a command
// that writes the configuration (it has no dry run and would write), and
// so does one that changes only local state (annotLocal).
// Commands without a class (command groups that only print help) ignore
// it.
func (a *App) checkDryRun(c *cobra.Command) error {
	if !a.Flags.DryRun {
		return nil
	}
	switch {
	case c.Annotations[annotWritesConfig] == "true":
		return clierr.New(clierr.CodeUsage, "--dry-run wird von diesem Befehl nicht unterstützt",
			"Der Befehl ändert die lokale Konfiguration; ohne --dry-run ausführen")
	case c.Annotations[annotLocal] == "true":
		return clierr.New(clierr.CodeUsage, "--dry-run wird von diesem Befehl nicht unterstützt",
			"Der Befehl sendet nichts und ändert nur lokale Dateien; ohne --dry-run ausführen")
	case c.Annotations[annotClass] == string(registry.ClassReadOnly):
		return clierr.New(clierr.CodeUsage, "--dry-run gilt nur für ändernde Befehle",
			"Dieser Befehl liest nur; ohne --dry-run ausführen. Verwendung: "+useLine(c))
	}
	return nil
}

// guardMutation is the read-only gate of every mutating command (spec 8.3,
// exit 9): every class but read_only is blocked when PAPIERKRAM_READ_ONLY
// is on, whatever context is chosen (R35), or when the chosen context has
// read_only = true. The environment is checked through the shared
// predicate as well, so the gate stays closed even without a resolution.
func (a *App) guardMutation(e registry.Entry, r *config.Resolved) error {
	if e.Class == registry.ClassReadOnly {
		return nil
	}
	switch {
	case config.EnvReadOnly(a.Env) || (r != nil && r.ReadOnly && r.ReadOnlySource == config.SourceEnv):
		return clierr.New(clierr.CodeReadOnly,
			"Schreibschutz aktiv (PAPIERKRAM_READ_ONLY) – Schreibzugriffe sind gesperrt",
			"Die Variable setzt, wer diesen Prozess startet; sie lässt sich nicht aufheben.")
	case r != nil && r.ReadOnly:
		return clierr.New(clierr.CodeReadOnly,
			fmt.Sprintf("Kontext %q ist schreibgeschützt (read_only = true)", r.Context),
			"anderen Kontext mit --context wählen")
	}
	return nil
}

// mutationClient is Client for mutating commands: it resolves the
// configuration, applies guardMutation and only then builds the client
// (plan order: read-only lock before the body is read and independent of
// the token). The resolution and client construction are those of Client.
func (a *App) mutationClient(e registry.Entry) (*api.Client, *config.Resolved, error) {
	r, err := a.resolveConfig()
	if err != nil {
		return nil, nil, err
	}
	if err := a.guardMutation(e, r); err != nil {
		return nil, nil, err
	}
	c, err := a.clientFor(r)
	return c, r, err
}
