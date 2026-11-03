package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/auth"
	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/config"
)

// wrapConfigErr turns plain config errors into German clierr errors that
// name the config file.
func wrapConfigErr(err error, path string) error {
	if err == nil {
		return nil
	}
	var ce *clierr.Error
	if errors.As(err, &ce) {
		return err
	}
	e := clierr.New(clierr.CodeError, err.Error(), "Konfigurationsdatei prüfen: "+path)
	e.Cause = err
	return e
}

func (a *App) loadConfig() (*config.Config, string, error) {
	path, err := a.configPath()
	if err != nil {
		return nil, "", clierr.New(clierr.CodeError, err.Error(), "Konfigurationsverzeichnis mit PAPIERKRAM_CONFIG_DIR setzen")
	}
	c, err := config.Load(path)
	return c, path, wrapConfigErr(err, path)
}

func (a *App) writeJSON(v any) error {
	enc := json.NewEncoder(a.Out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func newContextCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Kontexte (Account-Setups) verwalten",
		Long: "Kontexte verwalten, ähnlich wie bei hcloud: anlegen, wechseln, auflisten, umbenennen und löschen.\n" +
			"Befehle, die die Konfiguration ändern, schreiben config.toml neu; Kommentare und eigene Formatierung gehen dabei verloren.",
		Args: noArgs,
		RunE: func(c *cobra.Command, _ []string) error { return c.Help() },
	}
	cmd.AddCommand(newContextCreateCmd(app), newContextUseCmd(app), newContextListCmd(app),
		newContextActiveCmd(app), newContextDeleteCmd(app), newContextRenameCmd(app))
	return cmd
}

func newContextCreateCmd(app *App) *cobra.Command {
	var subdomain, user string
	var readOnly bool
	var userID int64
	cmd := &cobra.Command{
		Use:         "create <name>",
		Annotations: writesConfig(),
		Short:       "Kontext anlegen",
		Long: "Legt einen neuen Kontext an. Ist noch kein Kontext aktiv, wird der neue Kontext automatisch aktiv.\n" +
			"Das Token wird separat mit \"papierkram auth login\" hinterlegt.",
		Example: "  papierkram context create firma-gmbh --subdomain firma-gmbh\n" +
			"  papierkram context create agent --subdomain firma-gmbh --read-only",
		// Flags are checked with the arguments, i.e. before the read-only
		// guard (usage errors come first).
		Args: func(c *cobra.Command, args []string) error {
			if err := exactArgs(1)(c, args); err != nil {
				return err
			}
			if err := config.ValidateName(args[0]); err != nil {
				return err
			}
			if subdomain == "" {
				return clierr.New(clierr.CodeUsage, "Pflicht-Flag --subdomain fehlt", "Verwendung: "+useLine(c))
			}
			s, err := config.NormalizeSubdomain(subdomain, "--subdomain")
			if err != nil {
				return err
			}
			subdomain = s
			if c.Flags().Changed("user") {
				id, err := strconv.ParseInt(user, 10, 64)
				if userID = id; err != nil || id <= 0 {
					return usageErr(fmt.Sprintf("ungültiger Wert %q für --user: positive Zahl erwartet", user))
				}
			}
			return nil
		},
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, path, err := app.loadConfig()
			if err != nil {
				return err
			}
			if err := cfg.Create(args[0], &config.Context{Subdomain: subdomain, ReadOnly: readOnly, User: userID}); err != nil {
				return err
			}
			activated := cfg.ActiveContext == ""
			if activated {
				cfg.ActiveContext = args[0]
			}
			if err := wrapConfigErr(cfg.Save(path), path); err != nil {
				return err
			}
			msg := fmt.Sprintf("Kontext %q angelegt.\n", args[0])
			if activated {
				msg += "Der Kontext ist jetzt aktiv.\n"
			}
			_, err = fmt.Fprint(app.Out, msg)
			return err
		},
	}
	f := cmd.Flags()
	f.StringVar(&subdomain, "subdomain", "", "Papierkram-Subdomain (Pflicht)")
	f.BoolVar(&readOnly, "read-only", false, "Kontext gegen Schreibzugriffe sperren")
	f.StringVar(&user, "user", "", "Standard-User-ID für Zeiteinträge")
	return cmd
}

func newContextUseCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:         "use <name>",
		Annotations: writesConfig(),
		Short:       "Kontext aktivieren",
		Args:        exactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, path, err := app.loadConfig()
			if err != nil {
				return err
			}
			if err := cfg.Use(args[0]); err != nil {
				return err
			}
			if err := wrapConfigErr(cfg.Save(path), path); err != nil {
				return err
			}
			_, err = fmt.Fprintf(app.Out, "Aktiver Kontext: %s\n", args[0])
			return err
		},
	}
}

type contextRow struct {
	Name      string `json:"name"`
	Subdomain string `json:"subdomain"`
	ReadOnly  bool   `json:"read_only"`
	User      int64  `json:"user"`
	Active    bool   `json:"active"`
}

func newContextListCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Kontexte auflisten",
		Long:  "Listet alle Kontexte auf; der aktive Kontext ist markiert. Tokens werden nie ausgegeben.",
		Args:  noArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, _, err := app.loadConfig()
			if err != nil {
				return err
			}
			rows := make([]contextRow, 0, len(cfg.Contexts))
			for _, n := range cfg.Names() {
				ctx := cfg.Contexts[n]
				rows = append(rows, contextRow{n, ctx.Subdomain, ctx.ReadOnly, ctx.User, n == cfg.ActiveContext})
			}
			if app.Flags.Output == "json" {
				return app.writeJSON(map[string]any{"active_context": cfg.ActiveContext, "contexts": rows})
			}
			tw := tabwriter.NewWriter(app.Out, 0, 4, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "AKTIV\tNAME\tSUBDOMAIN\tSCHREIBSCHUTZ")
			for _, r := range rows {
				mark, ro := "", "nein"
				if r.Active {
					mark = "*"
				}
				if r.ReadOnly {
					ro = "ja"
				}
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", mark, r.Name, r.Subdomain, ro)
			}
			return tw.Flush()
		},
	}
}

func newContextActiveCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "active",
		Short: "Aktiven Kontext anzeigen",
		Long: "Zeigt den in der Konfiguration gespeicherten aktiven Kontext (active_context).\n" +
			"--context, PAPIERKRAM_CONTEXT und die Projektdatei werden hier nicht berücksichtigt.",
		Args: noArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, _, err := app.loadConfig()
			if err != nil {
				return err
			}
			if app.Flags.Output == "json" {
				return app.writeJSON(map[string]string{"active_context": cfg.ActiveContext})
			}
			if cfg.ActiveContext == "" {
				_, _ = fmt.Fprintln(app.Err, "Kein aktiver Kontext. Anlegen: papierkram context create <name> --subdomain <s>")
				return nil
			}
			_, err = fmt.Fprintln(app.Out, cfg.ActiveContext)
			return err
		},
	}
}

// warnKeyring reports a non-fatal keychain problem on stderr.
func (a *App) warnKeyring(format string, args ...any) {
	_, _ = fmt.Fprintf(a.Err, "Warnung: "+format+"\n", args...)
}

func keyringErr(err error, what string) error {
	return clierr.New(clierr.CodeError, fmt.Sprintf("%s: %v", what, err), "Schlüsselbund prüfen")
}

func newContextDeleteCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:         "delete <name>",
		Annotations: writesConfig(),
		Short:       "Kontext löschen",
		Long:        "Löscht den Kontext und das zugehörige Token im Schlüsselbund. Eine Bestätigung ist nicht nötig.",
		Args:        exactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]
			cfg, path, err := app.loadConfig()
			if err != nil {
				return err
			}
			if err := cfg.Delete(name); err != nil {
				return err
			}
			if err := wrapConfigErr(cfg.Save(path), path); err != nil {
				return err
			}
			switch err := app.Keyring.Delete(name); {
			case err == nil, errors.Is(err, auth.ErrNotFound):
			case errors.Is(err, auth.ErrUnavailable):
				app.warnKeyring("Schlüsselbund nicht verfügbar, Token von %q wurde nicht entfernt.", name)
			default:
				return keyringErr(err, fmt.Sprintf("Kontext %q gelöscht, aber Token konnte nicht entfernt werden", name))
			}
			_, err = fmt.Fprintf(app.Out, "Kontext %q gelöscht.\n", name)
			return err
		},
	}
}

func newContextRenameCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:         "rename <alt> <neu>",
		Annotations: writesConfig(),
		Short:       "Kontext umbenennen",
		Long:        "Benennt einen Kontext um und verschiebt das Token im Schlüsselbund. Ein aktiver Kontext bleibt aktiv.",
		Args:        exactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			oldName, newName := args[0], args[1]
			cfg, path, err := app.loadConfig()
			if err != nil {
				return err
			}
			if err := cfg.Rename(oldName, newName); err != nil {
				return err
			}
			moved := false
			switch tok, err := app.Keyring.Get(oldName); {
			case err == nil:
				if err := app.Keyring.Set(newName, tok); err != nil {
					return keyringErr(err, "Token konnte nicht verschoben werden, Konfiguration unverändert")
				}
				moved = true
			case errors.Is(err, auth.ErrNotFound):
			case errors.Is(err, auth.ErrUnavailable):
				app.warnKeyring("Schlüsselbund nicht verfügbar, Token von %q wurde nicht verschoben.", oldName)
			default:
				return keyringErr(err, "Token konnte nicht gelesen werden, Konfiguration unverändert")
			}
			if err := wrapConfigErr(cfg.Save(path), path); err != nil {
				if moved {
					_ = app.Keyring.Delete(newName)
				}
				return err
			}
			if moved {
				if err := app.Keyring.Delete(oldName); err != nil {
					app.warnKeyring("altes Token von %q konnte nicht entfernt werden.", oldName)
				}
			}
			_, err = fmt.Fprintf(app.Out, "Kontext %q in %q umbenannt.\n", oldName, newName)
			return err
		},
	}
}
