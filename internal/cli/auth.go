package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/api"
	"github.com/Padrio/papierkram-cli/internal/auth"
	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/config"
	"github.com/Padrio/papierkram-cli/internal/output"
	"github.com/Padrio/papierkram-cli/internal/redact"
)

// maxTokenInput bounds what auth login reads from stdin.
const maxTokenInput = 64 << 10

// opGetInfo is the operation auth login (token check) and auth status
// (quota) send; their pk:operation annotation names it for commands
// --json.
const opGetInfo = "GetInfo"

// withOperation adds the pk:operation annotation op to annotations.
func withOperation(annotations map[string]string, op string) map[string]string {
	annotations[annotOperation] = op
	return annotations
}

func newAuthCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "API-Token anmelden, abmelden und prüfen",
		Long: "Hinterlegt das API-Token eines Kontexts im Schlüsselbund des Betriebssystems, entfernt es wieder\n" +
			"oder zeigt, welcher Kontext, welches Token und welcher Schreibschutz gerade gelten.",
		Args: noArgs,
		RunE: func(c *cobra.Command, _ []string) error { return c.Help() },
	}
	cmd.AddCommand(newAuthLoginCmd(app), newAuthLogoutCmd(app), newAuthStatusCmd(app))
	return cmd
}

// selectContext returns the context auth login/logout act on, in the same
// order as every other command (flag > PAPIERKRAM_CONTEXT > project file >
// active_context).
func (a *App) selectContext(cfg *config.Config) (name, source string, err error) {
	in, err := a.resolveInput()
	if err != nil {
		return "", "", err
	}
	name, source = config.SelectContext(cfg, in)
	return name, source, nil
}

func newAuthLoginCmd(app *App) *cobra.Command {
	var subdomain string
	var tokenStdin, insecure bool
	cmd := &cobra.Command{
		Use:         "login",
		Annotations: withOperation(writesConfig(), opGetInfo),
		Short:       "Token prüfen und speichern",
		Long: "Liest das API-Token (Papierkram: Einstellungen → API) verdeckt vom Terminal oder mit --token-stdin\n" +
			"von der Standardeingabe, prüft es mit GET /info (1 Credit) und speichert es erst danach im\n" +
			"Schlüsselbund des Betriebssystems. Das Token wird nie als Flag-Wert angenommen.\n\n" +
			"Ziel ist der gewählte Kontext (--context, PAPIERKRAM_CONTEXT, Projektdatei, aktiver Kontext).\n" +
			"Existiert er nicht, wird er mit --subdomain angelegt (und aktiv, falls noch keiner aktiv ist).\n" +
			"Ohne Schlüsselbund (z. B. Linux ohne Secret Service) speichert nur --insecure-storage das Token\n" +
			"im Klartext in config.toml (Rechte 0600).",
		Example: "  papierkram auth login --context firma-gmbh --subdomain firma-gmbh\n" +
			"  papierkram auth login --token-stdin < token.txt",
		Args: func(c *cobra.Command, args []string) error {
			if err := noArgs(c, args); err != nil {
				return err
			}
			if c.Flags().Changed("subdomain") {
				s, err := config.NormalizeSubdomain(subdomain, "--subdomain")
				if err != nil {
					return err
				}
				subdomain = s
			}
			if !tokenStdin && (!app.StdinTTY || app.Flags.NoInput) {
				return clierr.New(clierr.CodeUsage,
					"Kein Terminal für die verdeckte Token-Eingabe",
					"Token über die Standardeingabe übergeben: papierkram auth login --token-stdin < token.txt")
			}
			return nil
		},
		RunE: func(c *cobra.Command, _ []string) error {
			return app.login(c, subdomain, tokenStdin, insecure)
		},
	}
	f := cmd.Flags()
	f.StringVar(&subdomain, "subdomain", "", "Subdomain, wenn der Kontext neu angelegt wird")
	f.BoolVar(&tokenStdin, "token-stdin", false, "Token von der Standardeingabe lesen (Leerzeichen und Zeilenende werden entfernt)")
	f.BoolVar(&insecure, "insecure-storage", false, "ohne Schlüsselbund das Token im Klartext in config.toml speichern")
	return cmd
}

// loginEnv hides the variables that must not influence the verification
// of a new token: it belongs to the target context and its subdomain.
// PAPIERKRAM_BASE_URL still applies (mock servers, tests).
func (a *App) loginEnv(k string) string {
	switch k {
	case "PAPIERKRAM_TOKEN", "PAPIERKRAM_SUBDOMAIN", "PAPIERKRAM_CONTEXT":
		return ""
	}
	return a.Env(k)
}

func (a *App) login(c *cobra.Command, subdomain string, tokenStdin, insecure bool) error {
	cfg, path, err := a.loadConfig()
	if err != nil {
		return err
	}
	name, source, err := a.selectContext(cfg)
	if err != nil {
		return err
	}
	created, activated, err := loginTarget(cfg, name, subdomain)
	if err != nil {
		return err
	}
	// Resolve the target like every other command, so that timeout, base
	// URL, context options and --debug behave the same; only the token is
	// the new one.
	r, err := config.Resolve(cfg, config.ResolveInput{FlagContext: name, Env: a.loginEnv})
	if err != nil {
		return err
	}
	if err := checkContextOptions(r); err != nil {
		return err
	}
	a.resolved = r
	// Probe the keychain before the token is read and verified, so that no
	// credit is spent on a token that cannot be stored.
	// With --insecure-storage there is nothing to probe (and no keychain
	// prompt on macOS); Set alone decides where the token goes.
	newSubdomain := ""
	if created {
		newSubdomain = subdomain
	}
	if !insecure {
		if _, err := a.Keyring.Get(name); errors.Is(err, auth.ErrUnavailable) {
			return noKeyringErr(name, newSubdomain, tokenStdin)
		}
	}
	token, err := a.readLoginToken(c.Context(), tokenStdin, name)
	if err != nil {
		return err
	}
	r.ContextSource, r.Token, r.TokenSource = source, token, ""
	if u := a.Env("PAPIERKRAM_BASE_URL"); u != "" {
		_, _ = fmt.Fprintf(a.Err, "Hinweis: Das Token wird gegen PAPIERKRAM_BASE_URL (%s) geprüft.\n", printable(redact.URL(u)))
	}
	client, err := a.clientFor(r)
	if err != nil {
		return err
	}
	if _, err := client.Call(c.Context(), opGetInfo, nil, nil, nil); err != nil {
		ce := clierr.From(err)
		ce.Message += "; das Token wurde nicht gespeichert"
		return ce
	}

	ctx := cfg.Contexts[name]
	tokenSource := config.SourceKeychain
	inKeychain, dirty := false, created
	switch err := a.Keyring.Set(name, token); {
	case err == nil:
		inKeychain = true
		if ctx.Token != "" { // a plain-text token from an earlier fallback is obsolete
			ctx.Token, dirty = "", true
		}
	case errors.Is(err, auth.ErrUnavailable):
		if !insecure {
			return noKeyringErr(name, newSubdomain, tokenStdin)
		}
		ctx.Token, tokenSource, dirty = token, config.SourceConfig, true
	default:
		return keyringErr(err, fmt.Sprintf("Token für Kontext %q konnte nicht gespeichert werden", name))
	}
	// config.toml is only rewritten when it changed (rewriting drops comments).
	if dirty {
		if err := a.save(cfg, path); err != nil {
			if !inKeychain || created {
				if inKeychain {
					_ = a.Keyring.Delete(name)
				}
				return wrapConfigErr(err, path)
			}
			// The new token is in the keychain and wins over the config
			// token, so the login counts; only the cleanup failed.
			output.Warn(a.Err, fmt.Sprintf("Token im Schlüsselbund gespeichert, altes Klartext-Token in %s konnte nicht entfernt werden (%v). Die Zeile token im Kontext %q von Hand löschen.", path, err, name))
		}
	}

	if created {
		msg := fmt.Sprintf("Kontext %q angelegt", name)
		if activated {
			msg += " und aktiviert"
		}
		_, _ = fmt.Fprintln(a.Err, msg+".")
	}
	where := "im Schlüsselbund"
	if tokenSource == config.SourceConfig {
		where = "in " + path
		output.Warn(a.Err, fmt.Sprintf("Kein Schlüsselbund verfügbar. Das Token steht wegen --insecure-storage im Klartext in %s (Rechte 0600). Wer diese Datei lesen kann, kann die Papierkram-API mit dem Token nutzen.", path))
	}
	_, _ = fmt.Fprintf(a.Err, "Angemeldet: Kontext %q (Subdomain %s), Token %s gespeichert.\n", name, ctx.Subdomain, where)
	if a.Env("PAPIERKRAM_TOKEN") != "" {
		_, _ = fmt.Fprintln(a.Err, "Hinweis: PAPIERKRAM_TOKEN ist gesetzt und hat weiterhin Vorrang vor dem gespeicherten Token.")
	}
	if o := a.outputOptions(c.Context(), a.outputFormat(r)); !ownText(o) {
		return output.RenderValue(o, loginJSON{name, ctx.Subdomain, tokenSource}, output.Columns{})
	}
	return nil
}

// noKeyringErr is the exit-3 error for a missing keychain without
// --insecure-storage; the hint repeats context, the subdomain of a context
// that is still to be created (newSubdomain) and the input mode.
func noKeyringErr(name, newSubdomain string, tokenStdin bool) error {
	cmd := "papierkram auth login --context " + name
	if newSubdomain != "" {
		cmd += " --subdomain " + newSubdomain
	}
	if tokenStdin {
		cmd += " --token-stdin --insecure-storage < token.txt"
	} else {
		cmd += " --insecure-storage"
	}
	return clierr.New(clierr.CodeAuth,
		"Kein Schlüsselbund verfügbar; das Token wurde nicht gespeichert",
		"Mit --insecure-storage wird es stattdessen im Klartext in config.toml (Rechte 0600) gespeichert: "+cmd)
}

// loginTarget checks the target context of auth login and creates it (in
// memory) when it is missing and a subdomain was given.
func loginTarget(cfg *config.Config, name, subdomain string) (created, activated bool, err error) {
	if name == "" {
		return false, false, clierr.New(clierr.CodeUsage, "Kein Kontext gewählt",
			"Kontext angeben und bei Bedarf anlegen: papierkram auth login --context <name> --subdomain <subdomain>")
	}
	if ctx, ok := cfg.Contexts[name]; ok && ctx != nil {
		if subdomain != "" && subdomain != ctx.Subdomain {
			return false, false, clierr.New(clierr.CodeUsage,
				fmt.Sprintf("Kontext %q hat bereits die Subdomain %q; --subdomain %s passt nicht dazu", name, ctx.Subdomain, subdomain),
				"--subdomain weglassen oder einen anderen Kontext wählen")
		}
		return false, false, nil
	}
	if subdomain == "" {
		return false, false, clierr.New(clierr.CodeUsage, fmt.Sprintf("Kontext %q existiert nicht", name),
			fmt.Sprintf("Mit Subdomain anlegen: papierkram auth login --context %s --subdomain <subdomain>", name))
	}
	if err := cfg.Create(name, &config.Context{Subdomain: subdomain}); err != nil {
		return false, false, err
	}
	if cfg.ActiveContext == "" {
		cfg.ActiveContext = name
		activated = true
	}
	return true, activated, nil
}

// readLoginToken reads the token from stdin or, with a terminal, hidden
// from the keyboard. Surrounding spaces and line endings (CR/LF) are
// removed; the token never appears in an error.
func (a *App) readLoginToken(ctx context.Context, fromStdin bool, name string) (string, error) {
	var raw []byte
	var err error
	if fromStdin {
		in := a.stdin()
		raw, err = a.readCtx(ctx, func() ([]byte, error) { return io.ReadAll(io.LimitReader(in, maxTokenInput)) })
	} else {
		_, _ = fmt.Fprintf(a.Err, "Token für Kontext %q (Eingabe verdeckt): ", name)
		raw, err = a.readCtx(ctx, a.ReadPassword)
	}
	var ce *clierr.Error
	if errors.As(err, &ce) {
		return "", interruptedHint(ce, nothingChanged) // exit 8, nothing stored; readCtx ended the line
	}
	if !fromStdin {
		_, _ = fmt.Fprintln(a.Err)
	}
	if err != nil {
		return "", clierr.New(clierr.CodeError, "Token konnte nicht gelesen werden: "+err.Error(), "")
	}
	// Trim spaces and CR/LF, and a UTF-8 BOM written by Windows editors.
	token := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(raw)), "\uFEFF"))
	if token == "" {
		return "", clierr.New(clierr.CodeAuth, "Kein Token angegeben",
			"Token in Papierkram unter Einstellungen → API anlegen und erneut eingeben")
	}
	if strings.IndexFunc(token, func(r rune) bool { return unicode.IsSpace(r) || !unicode.IsPrint(r) }) >= 0 {
		return "", clierr.New(clierr.CodeAuth, "Token ungültig: enthält Leer-, Steuer- oder unsichtbare Zeichen",
			"Nur das Token übergeben, eine Zeile, z. B. papierkram auth login --token-stdin < token.txt")
	}
	return token, nil
}

func newAuthLogoutCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:         "logout",
		Annotations: writesConfig(),
		Short:       "Token entfernen",
		Long: "Entfernt das Token des gewählten Kontexts aus dem Schlüsselbund und aus config.toml.\n" +
			"Der Kontext selbst bleibt erhalten. War kein Token gespeichert, ist das kein Fehler.",
		Example: "  papierkram auth logout\n  papierkram auth logout --context firma-gmbh",
		Args:    noArgs,
		RunE:    func(_ *cobra.Command, _ []string) error { return app.logout() },
	}
}

func (a *App) logout() error {
	cfg, path, err := a.loadConfig()
	if err != nil {
		return err
	}
	name, _, err := a.selectContext(cfg)
	if err != nil {
		return err
	}
	if name == "" {
		return clierr.New(clierr.CodeUsage, "Kein Kontext gewählt", "Kontext angeben: papierkram auth logout --context <name>")
	}
	ctx, ok := cfg.Contexts[name]
	if !ok || ctx == nil {
		return clierr.New(clierr.CodeNotFound, fmt.Sprintf("Kontext %q nicht gefunden", name),
			"Vorhandene Kontexte: papierkram context list")
	}
	var removed []string
	switch err := a.Keyring.Delete(name); {
	case err == nil:
		removed = append(removed, "Schlüsselbund")
	case errors.Is(err, auth.ErrNotFound):
	case errors.Is(err, auth.ErrUnavailable):
		a.warnKeyring("Schlüsselbund nicht verfügbar, ein dort gespeichertes Token von %q wurde nicht entfernt.", name)
	default:
		return keyringErr(err, fmt.Sprintf("Token von Kontext %q konnte nicht entfernt werden", name))
	}
	if ctx.Token != "" {
		ctx.Token = ""
		if err := wrapConfigErr(a.save(cfg, path), path); err != nil {
			return err
		}
		removed = append(removed, "config.toml")
	}
	a.removeCompletionCaches(name)
	if len(removed) == 0 {
		_, _ = fmt.Fprintf(a.Err, "Hinweis: Kontext %q war bereits abgemeldet (kein Token gespeichert).\n", name)
	} else {
		_, _ = fmt.Fprintf(a.Err, "Abgemeldet: Kontext %q, Token entfernt aus: %s.\n", name, strings.Join(removed, ", "))
	}
	if a.Env("PAPIERKRAM_TOKEN") != "" {
		_, _ = fmt.Fprintln(a.Err, "Hinweis: PAPIERKRAM_TOKEN ist weiterhin gesetzt und wird verwendet.")
	}
	return nil
}

type loginJSON struct {
	Context     string `json:"context"`
	Subdomain   string `json:"subdomain"`
	TokenSource string `json:"token_source"`
}

// quotaJSON is the quota of auth status: the credits left and what the
// status call itself cost (X-Consumed-Quota is the cost of one request,
// not a monthly consumption; null when the header is missing). The
// consumption of the month is not reported by the API.
type quotaJSON struct {
	Remaining   int  `json:"remaining"`
	RequestCost *int `json:"request_cost"`
	Known       bool `json:"known"`
}

type statusJSON struct {
	Context        string    `json:"context"`
	ContextSource  string    `json:"context_source"`
	Subdomain      string    `json:"subdomain"`
	BaseURL        string    `json:"base_url"`
	TokenSource    string    `json:"token_source"`
	ReadOnly       bool      `json:"read_only"`
	ReadOnlySource string    `json:"read_only_source"`
	Quota          quotaJSON `json:"quota"`
}

// quotaOf is the quota of auth status from the last quota of a response.
func quotaOf(q api.Quota) quotaJSON {
	out := quotaJSON{Remaining: q.Remaining, Known: q.Known}
	if q.CostKnown {
		cost := q.RequestCost
		out.RequestCost = &cost
	}
	return out
}

func newAuthStatusCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:         "status",
		Annotations: withOperation(readsOnly(), opGetInfo),
		Short:       "Kontext, Token-Quelle, Schreibschutz und Restkontingent anzeigen",
		Long: "Zeigt den wirksamen Kontext (mit Herkunft), Subdomain, Basis-URL, die Quelle des Tokens (env, keychain\n" +
			"oder config; das Token selbst nie), die Schreibschutzstufe und das Restkontingent.\n" +
			"Das Kontingent stammt aus einem Aufruf von GET /info und kostet 1 Credit.\n" +
			"Fehlt das Token oder lehnt die API es ab, werden die bekannten Angaben trotzdem ausgegeben (Exit 3).",
		Example: "  papierkram auth status\n  papierkram auth status --context firma-gmbh -o json",
		Args:    noArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			r, err := app.resolveConfig()
			if err != nil {
				return err
			}
			client, callErr := app.clientFor(r)
			if callErr == nil {
				_, callErr = client.Call(c.Context(), opGetInfo, nil, nil, nil)
			}
			st := statusJSON{
				Context: r.Context, ContextSource: r.ContextSource,
				Subdomain: r.Subdomain, BaseURL: printable(redact.URL(r.BaseURL)), TokenSource: r.TokenSource,
				ReadOnly: r.ReadOnly, ReadOnlySource: r.ReadOnlySource,
				Quota: quotaOf(app.quota),
			}
			var outErr error
			if o := app.outputOptions(c.Context(), app.outputFormat(r)); !ownText(o) {
				outErr = output.RenderValue(o, st, output.Columns{})
			} else {
				outErr = app.writeStatusText(st)
			}
			return statusResult(app, callErr, outErr)
		},
	}
}

// statusResult is the result of auth status: a failed call (no token, exit
// 3; 401/403; 404) keeps its code, and an output error then is a warning
// (L149); otherwise the output error is that of an output after a request
// (afterRequest, exit 1).
func statusResult(app *App, callErr, outErr error) error {
	if callErr == nil {
		return afterRequest(outErr)
	}
	if outErr != nil {
		msg := strings.TrimPrefix(clierr.From(outErr).Message, outputFailed)
		output.Warn(app.Err, "Ausgabe fehlgeschlagen: "+msg)
	}
	return callErr
}

var contextSourceText = map[string]string{
	config.SourceFlag:        "--context",
	config.SourceEnv:         "PAPIERKRAM_CONTEXT",
	config.SourceProjectFile: "Projektdatei",
	config.SourceConfig:      "active_context",
}

var tokenSourceText = map[string]string{
	config.SourceEnv:      "env (PAPIERKRAM_TOKEN)",
	config.SourceKeychain: "keychain (Schlüsselbund)",
	config.SourceConfig:   "config (Klartext in config.toml)",
	"":                    "keins",
}

func (a *App) writeStatusText(st statusJSON) error {
	orDash := func(s string) string {
		if s == "" {
			return "–"
		}
		return s
	}
	ctx := "– (keiner gewählt)"
	if st.Context != "" {
		ctx = fmt.Sprintf("%s (%s)", st.Context, contextSourceText[st.ContextSource])
	}
	ro := "aus"
	switch st.ReadOnlySource {
	case config.SourceEnv:
		ro = "an (env: PAPIERKRAM_READ_ONLY)"
	case config.SourceContext:
		ro = "an (context: read_only = true im Kontext)"
	}
	quota := "unbekannt"
	if st.Quota.Known {
		quota = fmt.Sprintf("%d Credits übrig", st.Quota.Remaining)
		if st.Quota.RequestCost != nil {
			quota += fmt.Sprintf(" (Kosten dieser Abfrage: %s)", creditsText(*st.Quota.RequestCost))
		}
	}
	tw := tabwriter.NewWriter(a.Out, 0, 4, 2, ' ', 0)
	for _, row := range [][2]string{
		{"Kontext:", ctx},
		{"Subdomain:", orDash(st.Subdomain)},
		{"Basis-URL:", orDash(st.BaseURL)},
		{"Token-Quelle:", tokenSourceText[st.TokenSource]},
		{"Schreibschutz:", ro},
		{"Kontingent:", quota},
	} {
		_, _ = fmt.Fprintf(tw, "%s\t%s\n", row[0], row[1])
	}
	return tw.Flush()
}
