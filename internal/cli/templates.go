package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

const usageTemplate = `Verwendung:{{if .Runnable}}
  {{germanUseLine .UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [Befehl]{{end}}{{if gt (len .Aliases) 0}}

Aliase:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Beispiele:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

Verfügbare Befehle:{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

Weitere Befehle:{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Optionen:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Globale Optionen:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Weitere Hilfethemen:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Mit "{{.CommandPath}} [Befehl] --help" gibt es weitere Informationen zu einem Befehl.{{end}}
`

const helpTemplate = `{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}

{{end}}{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}`

// applyGerman installs the German templates and help texts on the whole
// command tree. It must be called after all subcommands are added.
func applyGerman(root *cobra.Command) {
	cobra.AddTemplateFunc("germanUseLine", func(s string) string { return strings.ReplaceAll(s, "[flags]", "[Optionen]") })
	root.SetUsageTemplate(usageTemplate)
	root.SetHelpTemplate(helpTemplate)
	help := &cobra.Command{
		Use:   "help [Befehl]",
		Short: "Hilfe zu einem Befehl anzeigen",
		Long:  "Zeigt die Hilfe zu einem beliebigen Befehl an.",
		RunE: func(c *cobra.Command, args []string) error {
			target, _, err := c.Root().Find(args)
			if target == nil || err != nil || (target == c.Root() && len(args) > 0) {
				return unknownCommand(c.Root(), strings.Join(args, " "))
			}
			return target.Help()
		},
	}
	help.InitDefaultHelpFlag()
	help.Flags().Lookup("help").Usage = "Hilfe zu help"
	root.SetHelpCommand(help)
	walk(root, func(c *cobra.Command) {
		c.InitDefaultHelpFlag()
		if f := c.Flags().Lookup("help"); f != nil {
			f.Usage = "Hilfe zu " + c.Name()
		}
	})
}

func walk(c *cobra.Command, fn func(*cobra.Command)) {
	fn(c)
	for _, sub := range c.Commands() {
		walk(sub, fn)
	}
}
