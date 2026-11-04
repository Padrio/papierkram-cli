package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/api"
	"github.com/Padrio/papierkram-cli/internal/auth"
	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/config"
	"github.com/Padrio/papierkram-cli/internal/output"
)

// lowQuota is the remaining-credit threshold for the stderr warning (spec 8.5).
const lowQuota = 100

// Client resolves the effective configuration (config.toml, project file,
// --context, environment, keychain) and returns an API client for it.
// A missing subdomain is a usage error, a missing token an auth error.
// The command parameter is part of the shared signature for API commands.
func (a *App) Client(_ *cobra.Command) (*api.Client, *config.Resolved, error) {
	r, err := a.resolveConfig()
	if err != nil {
		return nil, nil, err
	}
	c, err := a.clientFor(r)
	return c, r, err
}

// resolveConfig loads config.toml and, when needed for the context
// selection, the project file, and resolves the settings of this
// invocation (spec 10.1). It warns when environment credentials override an
// explicitly chosen context.
func (a *App) resolveConfig() (*config.Resolved, error) {
	r, err := a.resolveWith(a.Flags.Context, a.Keyring)
	if err != nil {
		return nil, err
	}
	a.resolved = r
	a.warnEnvOverrides(r)
	return r, nil
}

// resolveWith is the resolution of resolveConfig with flagContext as the
// --context value and store as the keychain (nil: none is read), without
// side effects; the ID completion resolves through it.
func (a *App) resolveWith(flagContext string, store auth.Store) (*config.Resolved, error) {
	cfg, _, err := a.loadConfig()
	if err != nil {
		return nil, err
	}
	in, err := a.resolveInputFor(flagContext)
	if err != nil {
		return nil, err
	}
	in.Store = store
	r, err := config.Resolve(cfg, in)
	if err != nil {
		return nil, err
	}
	if err := checkContextOptions(r); err != nil {
		return nil, err
	}
	return r, nil
}

// checkContextOptions validates context options that Resolve passes through
// unchecked; client and login path share it.
func checkContextOptions(r *config.Resolved) error {
	if r.Output != "" && !slices.Contains(outputFormats, r.Output) {
		return clierr.New(clierr.CodeUsage,
			fmt.Sprintf("Ungültiger Wert %q für output in Kontext %q", r.Output, r.Context),
			"Erlaubt sind "+strings.Join(outputFormats, ", ")+"; config.toml korrigieren")
	}
	return nil
}

// resolveInput collects the inputs of the context selection. The project
// file is only consulted when neither --context nor PAPIERKRAM_CONTEXT
// decide.
func (a *App) resolveInput() (config.ResolveInput, error) {
	return a.resolveInputFor(a.Flags.Context)
}

// resolveInputFor is resolveInput with flagContext as the --context value.
func (a *App) resolveInputFor(flagContext string) (config.ResolveInput, error) {
	in := config.ResolveInput{FlagContext: flagContext, Env: a.Env, Store: a.Keyring}
	if in.FlagContext != "" || a.Env("PAPIERKRAM_CONTEXT") != "" {
		return in, nil
	}
	f, err := a.projectFile()
	if err != nil {
		return in, err
	}
	if f != nil {
		in.ProjectContext = f.Context
	}
	return in, nil
}

// warnEnvOverrides reports environment variables that take precedence over
// a context chosen explicitly by --context or PAPIERKRAM_CONTEXT.
func (a *App) warnEnvOverrides(r *config.Resolved) {
	var how string
	switch r.ContextSource {
	case config.SourceFlag:
		how = "--context"
	case config.SourceEnv:
		how = "PAPIERKRAM_CONTEXT"
	default:
		return
	}
	var vars []string
	for _, k := range []string{"PAPIERKRAM_TOKEN", "PAPIERKRAM_SUBDOMAIN", "PAPIERKRAM_BASE_URL"} {
		if a.Env(k) != "" {
			vars = append(vars, k)
		}
	}
	if len(vars) == 0 {
		return
	}
	verb := "hat"
	if len(vars) > 1 {
		verb = "haben"
	}
	output.Warn(a.Err, fmt.Sprintf("Kontext %q ist per %s gewählt, aber %s aus der Umgebung %s Vorrang vor seinen Einstellungen.",
		r.Context, how, strings.Join(vars, ", "), verb))
}

// clientFor checks that r has a base URL and a token and builds the client.
func (a *App) clientFor(r *config.Resolved) (*api.Client, error) {
	if err := requireCredentials(r); err != nil {
		return nil, err
	}
	return api.New(a.clientOptions(r)), nil
}

// requireCredentials checks that r has a base URL (usage error) and a
// token (auth error); every client is built after it (clientFor, the ID
// completion).
func requireCredentials(r *config.Resolved) error {
	if r.BaseURL == "" {
		msg := "Keine Subdomain konfiguriert: kein Kontext gewählt und PAPIERKRAM_SUBDOMAIN nicht gesetzt"
		if r.Context != "" {
			msg = fmt.Sprintf("Kontext %q hat keine Subdomain", r.Context)
		}
		return clierr.New(clierr.CodeUsage, msg,
			"Kontext anlegen: papierkram context create <name> --subdomain <subdomain> (oder PAPIERKRAM_SUBDOMAIN setzen)")
	}
	if r.Token == "" {
		if r.Context == "" {
			return clierr.New(clierr.CodeAuth, "Kein Token gesetzt",
				"Anmelden: papierkram auth login --context <name> (oder PAPIERKRAM_TOKEN setzen)")
		}
		e := clierr.New(clierr.CodeAuth, fmt.Sprintf("Kein Token für Kontext %q gespeichert", r.Context),
			fmt.Sprintf("Anmelden: papierkram auth login --context %s (oder PAPIERKRAM_TOKEN setzen)", r.Context))
		e.Cause = auth.ErrNotFound
		return e
	}
	return nil
}

// clientOptions maps r and the global flags to client options. An explicit
// --timeout beats the context timeout, which beats the default.
func (a *App) clientOptions(r *config.Resolved) api.Options {
	timeout := r.Timeout
	if a.Flags.TimeoutSet {
		timeout = a.Flags.Timeout
	}
	o := api.Options{
		BaseURL: r.BaseURL,
		Token:   r.Token,
		Timeout: timeout,
		HTTP:    a.HTTPClient,
		Sleep:   a.Sleep,
		OnQuota: a.onQuota,
	}
	if a.Flags.Debug {
		o.Debug, o.DebugInfo = a.Err, a.debugInfo(r)
	}
	return o
}

// debugInfo is the --debug context line: context with source, token source
// and read-only level (spec 8.3: --debug shows the active level).
func (a *App) debugInfo(r *config.Resolved) string {
	ctx := "–"
	if r.Context != "" {
		ctx = fmt.Sprintf("%q (%s)", r.Context, r.ContextSource)
	}
	tok := r.TokenSource
	if tok == "" {
		tok = "keins"
	}
	ro := r.ReadOnlySource
	if ro == "" {
		ro = "aus"
	}
	return fmt.Sprintf("Kontext: %s, Token-Quelle: %s, Schreibschutz: %s", ctx, tok, ro)
}

// onQuota records the quota of every response and warns once per
// invocation when the remaining credits drop below lowQuota.
func (a *App) onQuota(q api.Quota) {
	a.quota = q
	if q.Known && q.Remaining < lowQuota && !a.quotaWarned {
		a.quotaWarned = true
		output.Warn(a.Err, fmt.Sprintf("Restkontingent niedrig: %d Credits", q.Remaining))
	}
}

// outputFormat returns the effective output format: an explicit -o beats
// the context option, which beats the default "table".
func (a *App) outputFormat(r *config.Resolved) string {
	out := a.Flags.Output
	if !a.Flags.OutputSet && r != nil && r.Output != "" {
		out = r.Output
	}
	if out == "" {
		out = "table"
	}
	return out
}
