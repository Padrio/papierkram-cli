package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cpuguy83/go-md2man/v2/md2man"
	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/safefile"
)

// gen-docs (spec 12.3, hidden): the command reference as Markdown
// (docs/reference) and man pages of section 1, one page per visible
// command. The pages hold the German help of the command exactly as
// --help prints it (the templates of templates.go); cobra/doc is not used
// because its generators write English headings and pflag's English flag
// list. The man pages are rendered with go-md2man, the renderer of
// cobra/doc. Nothing in the output depends on the time or the build: two
// runs give identical files.

const genDocsLong = `Erzeugt die Befehlsreferenz als Markdown (eine Datei je Befehl, --dir) und Man-Pages in Abschnitt 1 (--man) aus der deutschen Hilfe der Befehle.

Für die Entwicklung (docs/reference, Man-Pages der Release-Archive). Schreibt nur in die angegebenen Verzeichnisse und legt sie bei Bedarf an; ist ein Zielverzeichnis ein symbolischer Link, wird nichts geschrieben. Vorhandene Dateien gleichen Namens werden ersetzt. Sendet keine Anfrage.`

func newGenDocsCmd(app *App) *cobra.Command {
	var mdDir, manDir string
	cmd := &cobra.Command{
		Use:     "gen-docs",
		Hidden:  true,
		Short:   "Befehlsreferenz (Markdown) und Man-Pages erzeugen",
		Long:    genDocsLong,
		Example: "  papierkram gen-docs --dir docs/reference --man man\n  papierkram gen-docs --man man",
		Args: func(c *cobra.Command, args []string) error {
			if err := noArgs(c, args); err != nil {
				return err
			}
			if app.Flags.DryRun {
				return clierr.New(clierr.CodeUsage, "--dry-run wird von diesem Befehl nicht unterstützt",
					"Der Befehl schreibt nur Doku-Dateien; ohne --dry-run ausführen")
			}
			if mdDir == "" && manDir == "" {
				return clierr.New(clierr.CodeUsage, "--dir oder --man fehlt", "Verwendung: "+useLine(c))
			}
			return nil
		},
		RunE: func(c *cobra.Command, _ []string) error {
			return app.genDocs(c.Root(), mdDir, manDir)
		},
	}
	cmd.Flags().StringVar(&mdDir, "dir", "", "Verzeichnis für die Markdown-Dateien (z. B. docs/reference)")
	cmd.Flags().StringVar(&manDir, "man", "", "Verzeichnis für die Man-Pages (z. B. man)")
	return cmd
}

// docPage is one generated file.
type docPage struct{ name, content string }

// genDocs writes the pages of every visible command below root. Both
// targets are checked before anything is written.
func (a *App) genDocs(root *cobra.Command, mdDir, manDir string) error {
	targets := []struct {
		dir, kind, suffix string
		page              func(*cobra.Command, string) docPage
	}{{mdDir, "Markdown-Dateien", ".md", markdownPage}, {manDir, "Man-Pages", ".1", manPage}}
	for i := range targets {
		// "dir/" and "dir/." name the same directory; lstat would follow a
		// link there (review M1)
		if targets[i].dir != "" {
			targets[i].dir = filepath.Clean(targets[i].dir)
		}
	}
	for _, t := range targets {
		if t.dir != "" {
			if err := checkDocDir(t.dir); err != nil {
				return err
			}
		}
	}
	cmds := docCommands(root)
	helps := make([]string, len(cmds))
	for i, c := range cmds {
		h, err := a.helpText(c)
		if err != nil {
			return err
		}
		helps[i] = h
	}
	var done []string
	for _, t := range targets {
		if t.dir == "" {
			continue
		}
		if err := os.MkdirAll(t.dir, 0o755); err != nil {
			return docErr(t.dir, err)
		}
		if err := checkDocDir(t.dir); err != nil { // swapped while creating
			return err
		}
		written := map[string]bool{}
		for i, c := range cmds {
			p := t.page(c, helps[i])
			path := filepath.Join(t.dir, p.name)
			if err := safefile.WriteAtomic(path, []byte(p.content), 0o644, safefile.WriteOptions{NoSync: true}); err != nil {
				return docErr(path, err)
			}
			written[p.name] = true
		}
		removed, err := removeStalePages(t.dir, t.suffix, written)
		if err != nil {
			return err
		}
		line := fmt.Sprintf("%d %s nach %s", len(cmds), t.kind, t.dir)
		if removed > 0 {
			line += fmt.Sprintf(" (%d veraltete entfernt)", removed)
		}
		done = append(done, line)
	}
	_, err := fmt.Fprintln(a.Out, strings.Join(done, ", ")+" geschrieben")
	return err
}

// removeStalePages removes the pages of commands that no longer exist:
// files in dir named like gen-docs' own (papierkram*<suffix>) that this
// run did not write. Directories and every other file stay (review M8).
func removeStalePages(dir, suffix string, written map[string]bool) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, docErr(dir, err)
	}
	n := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || written[name] || !strings.HasPrefix(name, "papierkram") || !strings.HasSuffix(name, suffix) {
			continue
		}
		path := filepath.Join(dir, name)
		if err := os.Remove(path); err != nil { // a link itself, never its target
			return n, docErr(path, err)
		}
		n++
	}
	return n, nil
}

// checkDocDir accepts a directory or a path that does not exist yet; a
// symbolic link (to anything) or another file is refused, so gen-docs
// never writes outside the given directory.
func checkDocDir(dir string) error {
	st, err := os.Lstat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return docErr(dir, err)
	case st.Mode()&fs.ModeSymlink != 0:
		return clierr.New(clierr.CodeError, fmt.Sprintf("Zielverzeichnis %q ist ein symbolischer Link; nichts geschrieben", dir),
			"Ein echtes Verzeichnis angeben")
	case !st.IsDir():
		return clierr.New(clierr.CodeError, fmt.Sprintf("Ziel %q ist kein Verzeichnis; nichts geschrieben", dir),
			"Ein Verzeichnis angeben")
	}
	return nil
}

// docErr is a failure to create or write path (exit 1).
func docErr(path string, err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		err = pe.Err // the path is named already
	}
	e := clierr.New(clierr.CodeError, fmt.Sprintf("Doku nicht geschrieben: %q: %v", path, err), "")
	e.Cause = err
	return e
}

// docCommands are the root and every command cobra/doc documents
// (available: not hidden, not deprecated, not help; below such a command
// nothing), sorted by command path.
func docCommands(root *cobra.Command) []*cobra.Command {
	var out []*cobra.Command
	var visit func(*cobra.Command)
	visit = func(c *cobra.Command) {
		out = append(out, c)
		for _, sub := range docChildren(c) {
			visit(sub)
		}
	}
	visit(root)
	slices.SortFunc(out, func(a, b *cobra.Command) int { return strings.Compare(a.CommandPath(), b.CommandPath()) })
	return out
}

// docChildren are the documented subcommands of c, sorted by name.
func docChildren(c *cobra.Command) []*cobra.Command {
	var out []*cobra.Command
	for _, sub := range c.Commands() {
		if sub.IsAvailableCommand() && !sub.IsAdditionalHelpTopicCommand() {
			out = append(out, sub)
		}
	}
	slices.SortFunc(out, func(a, b *cobra.Command) int { return strings.Compare(a.Name(), b.Name()) })
	return out
}

// helpText is the help of c as --help prints it.
func (a *App) helpText(c *cobra.Command) (string, error) {
	var b bytes.Buffer
	c.SetOut(&b)
	err := c.Help()
	if c.HasParent() {
		c.SetOut(nil) // inherit again
	} else {
		c.SetOut(a.Out)
	}
	return b.String(), err
}

// fence is a Markdown code fence longer than every backtick run of s.
func fence(s string) string {
	n, run := 3, 0
	for _, r := range s {
		if r == '`' {
			run++
			n = max(n, run+1)
		} else {
			run = 0
		}
	}
	return strings.Repeat("`", n)
}

// related are the parent and the documented subcommands of c.
func related(c *cobra.Command) []*cobra.Command {
	var out []*cobra.Command
	if c.HasParent() {
		out = append(out, c.Parent())
	}
	return append(out, docChildren(c)...)
}

func markdownName(c *cobra.Command) string {
	return strings.ReplaceAll(c.CommandPath(), " ", "_") + ".md"
}

func manName(c *cobra.Command) string { return strings.ReplaceAll(c.CommandPath(), " ", "-") }

// markdownPage is the Markdown page of c: title, short help, the full
// help as preformatted text and links to the parent and subcommands.
func markdownPage(c *cobra.Command, help string) docPage {
	var b strings.Builder
	f := fence(help)
	fmt.Fprintf(&b, "# %s\n\n%s\n\n%stext\n%s%s\n", c.CommandPath(), c.Short, f, help, f)
	if rel := related(c); len(rel) > 0 {
		b.WriteString("\n## Siehe auch\n\n")
		for _, r := range rel {
			fmt.Fprintf(&b, "- [%s](%s) – %s\n", r.CommandPath(), markdownName(r), r.Short)
		}
	}
	return docPage{markdownName(c), b.String()}
}

// manPage is the man page (section 1) of c, rendered from Markdown with
// go-md2man like cobra/doc does: the standard NAME section (whatis,
// apropos, makewhatis) with the German short help, then German sections
// with the parts of the help (templates.go): use line and aliases, the
// description as filled paragraphs (indented blocks preformatted),
// subcommands and flags as tagged paragraphs (germanFlagRows, the rows
// of --help), examples preformatted, related pages. No date.
func manPage(c *cobra.Command, _ string) docPage {
	var b strings.Builder
	name := manName(c)
	fmt.Fprintf(&b, "%% %q %q %q %q %q\n", strings.ToUpper(name), "1", "", "papierkram", "Papierkram-CLI")
	fmt.Fprintf(&b, "# NAME\n%s \\- %s\n\n", mdEscape(name), mdEscape(c.Short))
	fmt.Fprintf(&b, "# ÜBERSICHT\n**%s**\n", mdEscape(useLine(c)))
	if len(c.Aliases) > 0 {
		fmt.Fprintf(&b, "\nAliase: %s\n", mdEscape(c.NameAndAliases()))
	}
	desc := c.Long
	if desc == "" {
		desc = c.Short
	}
	fmt.Fprintf(&b, "\n# BESCHREIBUNG\n%s", manParagraphs(desc))
	if subs := docChildren(c); len(subs) > 0 {
		b.WriteString("\n# BEFEHLE\n")
		for _, sub := range subs {
			fmt.Fprintf(&b, "**%s**\n: %s\n\n", mdEscape(sub.Name()), mdEscape(sub.Short))
		}
	}
	for _, sec := range []struct {
		title string
		rows  []flagRow
	}{{"OPTIONEN", germanFlagRows(c.LocalFlags())}, {"GLOBALE OPTIONEN", germanFlagRows(c.InheritedFlags())}} {
		if len(sec.rows) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n# %s\n", sec.title)
		for _, r := range sec.rows {
			fmt.Fprintf(&b, "**%s**\n: %s\n\n", mdEscape(r.flag), mdEscape(strings.ReplaceAll(r.usage, "\n", " ")))
		}
	}
	if c.Example != "" {
		f := fence(c.Example)
		fmt.Fprintf(&b, "\n# BEISPIELE\n%s\n%s\n%s\n", f, c.Example, f)
	}
	if rel := related(c); len(rel) > 0 {
		var refs []string
		for _, r := range rel {
			refs = append(refs, fmt.Sprintf("**%s(1)**", mdEscape(manName(r))))
		}
		fmt.Fprintf(&b, "\n# SIEHE AUCH\n%s\n", strings.Join(refs, ", "))
	}
	return docPage{name + ".1", string(md2man.Render([]byte(b.String())))}
}

// manParagraphs renders a help text for the man page: paragraphs (split
// at blank lines) are filled text whose line breaks are kept; a paragraph
// with an indented line (a config example, a list of commands) is
// preformatted.
func manParagraphs(text string) string {
	var b strings.Builder
	for _, para := range strings.Split(strings.TrimSpace(text), "\n\n") {
		lines := strings.Split(para, "\n")
		indented := slices.ContainsFunc(lines, func(l string) bool {
			return strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t")
		})
		if indented {
			f := fence(para)
			fmt.Fprintf(&b, "%s\n%s\n%s\n\n", f, para, f)
			continue
		}
		for i := range lines {
			lines[i] = mdEscape(lines[i])
		}
		// two trailing spaces: a line break, the lines are filled
		b.WriteString(strings.Join(lines, "  \n") + "\n\n")
	}
	return b.String()
}

// mdEscape escapes the characters Markdown (blackfriday) would read as
// markup, so help texts with <pfad>, * or _ arrive as written.
func mdEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune("\\`*_{}[]()#+-.!:|&<>~", r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
