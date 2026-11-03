package config

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/Padrio/papierkram-cli/internal/auth"
	"github.com/Padrio/papierkram-cli/internal/clierr"
)

// Source identifiers; stable strings shown by `auth status` and --debug.
const (
	SourceFlag        = "flag"
	SourceEnv         = "env"
	SourceProjectFile = "projectfile"
	SourceConfig      = "config"
	SourceKeychain    = "keychain"
	SourceContext     = "context"
)

const defaultTimeout = 30 * time.Second

// ResolveInput carries everything Resolve needs from the outside world.
type ResolveInput struct {
	FlagContext    string
	ProjectContext string
	Env            func(string) string
	Store          auth.Store
}

// Resolved is the effective configuration for one invocation.
//
// ContextSource is one of "flag", "env", "projectfile", "config" or "".
// TokenSource is one of "env", "keychain", "config" or "".
// ReadOnlySource is one of "env", "context" or "".
type Resolved struct {
	Context, ContextSource string
	Subdomain, BaseURL     string
	Token, TokenSource     string
	ReadOnly               bool
	ReadOnlySource         string
	Timeout                time.Duration
	Output                 string
	User                   int64
	CompletionLookup       bool
}

// String renders the Resolved without the token (masked as ***).
func (r Resolved) String() string {
	tok := ""
	if r.Token != "" {
		tok = "***"
	}
	return fmt.Sprintf("Resolved{Context:%q(%s) Subdomain:%q BaseURL:%q Token:%q(%s) ReadOnly:%t(%s) Timeout:%s Output:%q User:%d CompletionLookup:%t}",
		r.Context, r.ContextSource, r.Subdomain, r.BaseURL, tok, r.TokenSource,
		r.ReadOnly, r.ReadOnlySource, r.Timeout, r.Output, r.User, r.CompletionLookup)
}

// GoString covers %#v.
func (r Resolved) GoString() string { return r.String() }

// Format makes every fmt verb (%v, %+v, %#v, %s, %d, ...) use the masked form.
func (r Resolved) Format(f fmt.State, _ rune) { _, _ = io.WriteString(f, r.String()) }

// Resolve applies the resolution order (spec 10.1, 8.3):
// context: flag > PAPIERKRAM_CONTEXT > project file > active_context;
// token: PAPIERKRAM_TOKEN > keychain > token in config (no token is not an
// error, callers check Token == ""); subdomain: PAPIERKRAM_SUBDOMAIN >
// context; PAPIERKRAM_BASE_URL overrides the derived base URL; read-only:
// PAPIERKRAM_READ_ONLY (see EnvReadOnly) or the context option.
func Resolve(c *Config, in ResolveInput) (*Resolved, error) {
	if c == nil {
		c = &Config{}
	}
	getenv := in.Env
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	r := &Resolved{Timeout: defaultTimeout, CompletionLookup: true}

	switch {
	case in.FlagContext != "":
		r.Context, r.ContextSource = in.FlagContext, SourceFlag
	case getenv("PAPIERKRAM_CONTEXT") != "":
		r.Context, r.ContextSource = getenv("PAPIERKRAM_CONTEXT"), SourceEnv
	case in.ProjectContext != "":
		r.Context, r.ContextSource = in.ProjectContext, SourceProjectFile
	case c.ActiveContext != "":
		r.Context, r.ContextSource = c.ActiveContext, SourceConfig
	}

	var ctx *Context
	if r.Context != "" {
		var ok bool
		if ctx, ok = c.Contexts[r.Context]; !ok || ctx == nil {
			return nil, c.notFound(r.Context)
		}
		r.Output, r.User = ctx.Output, ctx.User
		if ctx.CompletionLookup != nil {
			r.CompletionLookup = *ctx.CompletionLookup
		}
		if ctx.Timeout != "" {
			d, err := time.ParseDuration(ctx.Timeout)
			if err != nil || d <= 0 {
				return nil, clierr.New(clierr.CodeUsage,
					fmt.Sprintf("Ungültiger timeout %q in Kontext %q", ctx.Timeout, r.Context),
					"Positive Dauer wie 30s oder 1m30s angeben")
			}
			r.Timeout = d
		}
	}

	// Subdomain and base URL. Every subdomain is validated before it is
	// put into the URL, whatever its source.
	if v := getenv("PAPIERKRAM_SUBDOMAIN"); v != "" {
		s, err := NormalizeSubdomain(v, "PAPIERKRAM_SUBDOMAIN")
		if err != nil {
			return nil, err
		}
		r.Subdomain = s
	} else if ctx != nil && ctx.Subdomain != "" {
		s, err := NormalizeSubdomain(ctx.Subdomain, fmt.Sprintf("Kontext %q", r.Context))
		if err != nil {
			return nil, err
		}
		r.Subdomain = s
	}
	switch {
	case getenv("PAPIERKRAM_BASE_URL") != "":
		r.BaseURL = getenv("PAPIERKRAM_BASE_URL")
	case r.Subdomain != "":
		r.BaseURL = "https://" + r.Subdomain + ".papierkram.de/api/v1"
	}

	// Token.
	if t := getenv("PAPIERKRAM_TOKEN"); t != "" {
		r.Token, r.TokenSource = t, SourceEnv
	} else if r.Context != "" {
		if in.Store != nil {
			t, err := in.Store.Get(r.Context)
			switch {
			case err == nil && t != "":
				r.Token, r.TokenSource = t, SourceKeychain
			case err == nil, errors.Is(err, auth.ErrNotFound), errors.Is(err, auth.ErrUnavailable):
				// fall through to the config token
			default:
				return nil, fmt.Errorf("config: Token für Kontext %q lesen: %w", r.Context, err)
			}
		}
		if r.Token == "" && ctx.Token != "" {
			r.Token, r.TokenSource = ctx.Token, SourceConfig
		}
	}

	// Read-only: the environment is the hard stage and always wins.
	switch {
	case EnvReadOnly(getenv):
		r.ReadOnly, r.ReadOnlySource = true, SourceEnv
	case ctx != nil && ctx.ReadOnly:
		r.ReadOnly, r.ReadOnlySource = true, SourceContext
	}
	return r, nil
}
