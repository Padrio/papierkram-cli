package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// useLine is cobra's UseLine with "[Optionen]" instead of "[flags]". Usage
// hints must use it, never c.UseLine().
func useLine(c *cobra.Command) string {
	return strings.ReplaceAll(c.UseLine(), "[flags]", "[Optionen]")
}

// germanTypeNames maps pflag value types to the German placeholder shown
// after the flag name; "" shows none (booleans). Unlisted types show "Wert".
var germanTypeNames = map[string]string{
	"bool": "", "boolfunc": "",
	"string":   "Text",
	"duration": "Dauer",
	"int":      "Zahl", "int8": "Zahl", "int16": "Zahl", "int32": "Zahl", "int64": "Zahl",
	"uint": "Zahl", "uint8": "Zahl", "uint16": "Zahl", "uint32": "Zahl", "uint64": "Zahl",
	"float32": "Dezimalzahl", "float64": "Dezimalzahl",
	"stringSlice": "Texte", "stringArray": "Texte",
	"intSlice": "Zahlen", "int32Slice": "Zahlen", "int64Slice": "Zahlen", "uintSlice": "Zahlen",
	"float32Slice": "Dezimalzahlen", "float64Slice": "Dezimalzahlen",
	"boolSlice":     "Wahrheitswerte",
	"durationSlice": "Dauern",
	"count":         "Anzahl",
	// rawValue types of the create/update field flags (builder_write.go)
	"id": "ID", "integer": "Zahl", "number": "Dezimalzahl", "date": "Datum",
}

// defaultIsZero mirrors pflag's (unexported) defaultIsZeroValue by type name.
// An empty default is always zero (the raw create/update field flags have
// numeric type names but no default).
func defaultIsZero(f *pflag.Flag) bool {
	if f.DefValue == "" {
		return true
	}
	switch t := f.Value.Type(); {
	case t == "bool" || t == "boolfunc":
		return f.DefValue == "false" || f.DefValue == ""
	case t == "duration":
		return f.DefValue == "0" || f.DefValue == "0s"
	case t == "string":
		return f.DefValue == ""
	case strings.HasPrefix(t, "int") || strings.HasPrefix(t, "uint") || strings.HasPrefix(t, "float") || t == "count":
		if strings.HasSuffix(t, "Slice") {
			return f.DefValue == "[]"
		}
		return f.DefValue == "0"
	case strings.HasSuffix(t, "Slice") || t == "stringArray":
		return f.DefValue == "[]"
	}
	switch f.DefValue {
	case "false", "<nil>", "", "0":
		return true
	}
	return false
}

// germanFlagUsages renders a flag list like pflag's FlagUsages, but with
// German type placeholders and "(Standard: …)" instead of "(default …)".
// A back-quoted name in the usage text is kept as placeholder (pflag rule).
func germanFlagUsages(fs *pflag.FlagSet) string {
	type row struct{ left, usage string }
	var rows []row
	maxlen := 0
	fs.VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		left := "      --" + f.Name
		if f.Shorthand != "" && f.ShorthandDeprecated == "" {
			left = "  -" + f.Shorthand + ", --" + f.Name
		}
		name, usage := pflag.UnquoteUsage(f)
		if usage == f.Usage { // no back-quoted name: derive from the type
			var ok bool
			if name, ok = germanTypeNames[f.Value.Type()]; !ok {
				name = "Wert"
			}
		}
		if name != "" {
			left += " " + name
		}
		if f.NoOptDefVal != "" {
			switch f.Value.Type() {
			case "string":
				left += fmt.Sprintf("[=%q]", f.NoOptDefVal)
			case "bool", "boolfunc":
				if f.NoOptDefVal != "true" {
					left += "[=" + f.NoOptDefVal + "]"
				}
			case "count":
				if f.NoOptDefVal != "+1" {
					left += "[=" + f.NoOptDefVal + "]"
				}
			default:
				left += "[=" + f.NoOptDefVal + "]"
			}
		}
		if !defaultIsZero(f) {
			if f.Value.Type() == "string" {
				usage += fmt.Sprintf(" (Standard: %q)", f.DefValue)
			} else {
				usage += fmt.Sprintf(" (Standard: %s)", f.DefValue)
			}
		}
		if f.Deprecated != "" {
			usage += fmt.Sprintf(" (veraltet: %s)", f.Deprecated)
		}
		maxlen = max(maxlen, len(left)+1)
		rows = append(rows, row{left, usage})
	})
	var b strings.Builder
	for _, r := range rows {
		// Same layout as pflag: left, one space, padding, one space, usage;
		// continuation lines of a multi-line usage are indented to match.
		usage := strings.ReplaceAll(r.usage, "\n", "\n"+strings.Repeat(" ", maxlen+2))
		b.WriteString(r.left + " " + strings.Repeat(" ", maxlen-len(r.left)) + " " + usage + "\n")
	}
	return b.String()
}

const usageTemplate = `Verwendung:{{if .Runnable}}
  {{useLine .}}{{end}}{{if .HasAvailableSubCommands}}
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
{{germanFlagUsages .LocalFlags | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Globale Optionen:
{{germanFlagUsages .InheritedFlags | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Weitere Hilfethemen:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Mit "{{.CommandPath}} [Befehl] --help" gibt es weitere Informationen zu einem Befehl.{{end}}
`

const helpTemplate = `{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}

{{end}}{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}`

// applyGerman installs the German templates and help texts on the whole
// command tree. It must be called after all subcommands are added.
func applyGerman(root *cobra.Command) {
	cobra.AddTemplateFunc("useLine", useLine)
	cobra.AddTemplateFunc("germanFlagUsages", germanFlagUsages)
	root.SetUsageTemplate(usageTemplate)
	root.SetHelpTemplate(helpTemplate)
	help := &cobra.Command{
		Use:   "help [Befehl]",
		Short: "Hilfe zu einem Befehl anzeigen",
		Long:  "Zeigt die Hilfe zu einem beliebigen Befehl an.",
		RunE: func(c *cobra.Command, args []string) error {
			target, rest, err := c.Root().Find(args)
			if target == nil || err != nil || (target == c.Root() && len(args) > 0) {
				return unknownCommand(c.Root(), strings.Join(args, " "))
			}
			if len(rest) > 0 {
				return unknownCommand(target, rest[0])
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
