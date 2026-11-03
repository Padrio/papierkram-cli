// Package config handles config.toml (contexts, persistence) and the
// resolution order of context, token, subdomain and read-only mode.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"unicode"

	"github.com/pelletier/go-toml/v2"

	"github.com/Padrio/papierkram-cli/internal/clierr"
)

// Context is one named account setup in config.toml.
type Context struct {
	Subdomain string `toml:"subdomain"`
	ReadOnly  bool   `toml:"read_only"`
	Output    string `toml:"output,omitempty"`
	Timeout   string `toml:"timeout,omitempty"`
	User      int64  `toml:"user,omitempty"`
	// Token is the plain-file fallback (--insecure-storage); normally the
	// token lives in the OS keyring.
	Token string `toml:"token,omitempty"`
	// CompletionLookup is nil when unset (default: enabled).
	CompletionLookup *bool `toml:"completion_lookup,omitempty"`
}

// Config is the content of config.toml.
type Config struct {
	ActiveContext string              `toml:"active_context"`
	Contexts      map[string]*Context `toml:"contexts"`
}

// Path returns <configDir>/papierkram/config.toml. An empty configDir means
// os.UserConfigDir(); the caller passes PAPIERKRAM_CONFIG_DIR when set. An
// error is returned only if no default config directory can be determined.
func Path(configDir string) (string, error) {
	if configDir == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("config: Konfigurationsverzeichnis nicht ermittelbar: %w", err)
		}
		configDir = dir
	}
	return filepath.Join(configDir, "papierkram", "config.toml"), nil
}

// Load reads the config file. A missing file yields an empty Config. Unknown
// keys are a usage error (a misspelled read_only must not be ignored).
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &Config{Contexts: map[string]*Context{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("config: Konfiguration lesen: %w", err)
	}
	c := &Config{}
	if err := decodeStrict(path, data, c); err != nil {
		var ce *clierr.Error
		if errors.As(err, &ce) {
			return nil, err
		}
		return nil, fmt.Errorf("config: Konfiguration %s ungültig: %w", path, err)
	}
	if c.Contexts == nil {
		c.Contexts = map[string]*Context{}
	}
	return c, nil
}

// Save writes the config atomically (temp file in the same directory, then
// rename) with mode 0600; the directory is created with 0700.
func (c *Config) Save(path string) error {
	data, err := toml.Marshal(c)
	if err != nil {
		return fmt.Errorf("config: Konfiguration serialisieren: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("config: Konfigurationsverzeichnis anlegen: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("config: Konfiguration schreiben: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // no-op after successful rename
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("config: Konfiguration schreiben: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil && runtime.GOOS != "windows" {
		_ = tmp.Close()
		return fmt.Errorf("config: Konfiguration schreiben: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("config: Konfiguration schreiben: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("config: Konfiguration schreiben: %w", err)
	}
	return nil
}

// ValidateName checks a context name: non-empty, no whitespace, no dots
// (a dot would split the TOML table key [contexts.<name>]).
func ValidateName(name string) error {
	hint := "Erlaubt sind Namen ohne Leerzeichen und Punkte, z. B. firma-gmbh."
	if name == "" {
		return clierr.New(clierr.CodeUsage, "Kontextname darf nicht leer sein", hint)
	}
	if strings.ContainsFunc(name, unicode.IsSpace) {
		return clierr.New(clierr.CodeUsage, fmt.Sprintf("Kontextname %q darf keine Leerzeichen enthalten", name), hint)
	}
	if strings.Contains(name, ".") {
		return clierr.New(clierr.CodeUsage, fmt.Sprintf("Kontextname %q darf keine Punkte enthalten", name), hint)
	}
	return nil
}

// Names returns the context names, sorted.
func (c *Config) Names() []string {
	names := make([]string, 0, len(c.Contexts))
	for n := range c.Contexts {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func (c *Config) notFound(name string) error {
	hint := "Kontext anlegen: papierkram context create"
	if names := c.Names(); len(names) > 0 {
		hint = "Verfügbare Kontexte: " + strings.Join(names, ", ") + "; neuen anlegen: papierkram context create"
	}
	return clierr.New(clierr.CodeNotFound, fmt.Sprintf("Kontext %q nicht gefunden", name), hint)
}

// Create adds a new context.
func (c *Config) Create(name string, ctx *Context) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	if _, ok := c.Contexts[name]; ok {
		return clierr.New(clierr.CodeUsage, fmt.Sprintf("Kontext %q existiert bereits", name), "Anderen Namen wählen oder zuerst papierkram context delete")
	}
	if ctx == nil {
		ctx = &Context{}
	}
	if c.Contexts == nil {
		c.Contexts = map[string]*Context{}
	}
	c.Contexts[name] = ctx
	return nil
}

// Use makes an existing context the active one.
func (c *Config) Use(name string) error {
	if _, ok := c.Contexts[name]; !ok {
		return c.notFound(name)
	}
	c.ActiveContext = name
	return nil
}

// Delete removes a context and clears active_context if it pointed to it.
// Removing the keychain entry is the caller's job.
func (c *Config) Delete(name string) error {
	if _, ok := c.Contexts[name]; !ok {
		return c.notFound(name)
	}
	delete(c.Contexts, name)
	if c.ActiveContext == name {
		c.ActiveContext = ""
	}
	return nil
}

// Rename renames a context and follows active_context. Moving the keychain
// entry is the caller's job.
func (c *Config) Rename(oldName, newName string) error {
	ctx, ok := c.Contexts[oldName]
	if !ok {
		return c.notFound(oldName)
	}
	if err := ValidateName(newName); err != nil {
		return err
	}
	if _, exists := c.Contexts[newName]; exists {
		return clierr.New(clierr.CodeUsage, fmt.Sprintf("Kontext %q existiert bereits", newName), "Anderen Namen wählen")
	}
	c.Contexts[newName] = ctx
	delete(c.Contexts, oldName)
	if c.ActiveContext == oldName {
		c.ActiveContext = newName
	}
	return nil
}
