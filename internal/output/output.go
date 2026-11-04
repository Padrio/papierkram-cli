// Package output renders command results (table, wide, json, yaml, --fields,
// --jq) on stdout and errors and warnings on stderr (spec 10.3, 10.4).
//
// Numbers are never converted to float64 on the way: plain JSON output
// re-indents the raw API bytes, every other path decodes with UseNumber and
// writes the original number literal (19.99, 1190.00, 9007199254740993).
package output

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/Padrio/papierkram-cli/internal/clierr"
)

// Format is the value of -o/--output.
type Format string

// The output formats of -o/--output.
const (
	FormatTable Format = "table"
	FormatWide  Format = "wide"
	FormatJSON  Format = "json"
	FormatYAML  Format = "yaml"
)

// Options controls one rendering. Fields are dotted paths (--fields, see
// ParseFields), JQ a gojq expression (--jq, implies JSON). Color is decided
// by the caller (see UseColor). Out receives the data, Err errors. Context
// cancels a running --jq expression (nil means context.Background()).
type Options struct {
	Format   Format
	Fields   []string
	JQ       string
	Color    bool
	Out, Err io.Writer
	Context  context.Context
}

// Structured reports whether the output is machine-readable: json, yaml or
// any format combined with --jq.
func (o Options) Structured() bool {
	return o.JQ != "" || o.Format == FormatJSON || o.Format == FormatYAML
}

// errorFormat is the format of the structured error on stderr: --jq
// implies JSON.
func (o Options) errorFormat() Format {
	if o.JQ != "" {
		return FormatJSON
	}
	return o.Format
}

// ParseFields splits the value of --fields at commas; blanks and empty
// entries are dropped. Unknown fields are not an error: the API omits
// some fields, so a missing path is simply left out of the output.
func ParseFields(s string) []string {
	var fields []string
	for f := range strings.SplitSeq(s, ",") {
		if f = strings.TrimSpace(f); f != "" {
			fields = append(fields, f)
		}
	}
	return fields
}

// UseColor decides whether stdout gets colors: only on a terminal, not with
// --no-color and not when NO_COLOR is set to a non-empty value
// (https://no-color.org).
func UseColor(stdoutTTY bool, env func(string) string, noColorFlag bool) bool {
	if !stdoutTTY || noColorFlag {
		return false
	}
	return env == nil || env("NO_COLOR") == ""
}

// Warn writes a warning line to w (stderr); msg goes through Message.
func Warn(w io.Writer, msg string) {
	_, _ = fmt.Fprintf(w, "Warnung: %s\n", Message(msg))
}

// Render writes the API response raw (JSON) to o.Out in the format of o.
// An empty body prints nothing. With --fields each object, or each element
// of entries[] in a list envelope, is reduced to the given paths; --jq is
// applied afterwards and prints one result per line. table and wide show
// cols (see renderTable); with --fields the fields are the columns.
func Render(o Options, raw []byte, cols Columns) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if o.JQ != "" {
		return renderJQ(o, raw)
	}
	switch o.Format {
	case FormatJSON:
		return renderJSON(o, raw)
	case FormatYAML:
		return renderYAML(o, raw)
	case FormatTable, FormatWide:
		return renderTable(o, raw, cols)
	default:
		return clierr.New(clierr.CodeUsage, fmt.Sprintf("unbekanntes Ausgabeformat %q", o.Format),
			"Erlaubt sind table, wide, json, yaml")
	}
}

// RenderValue renders a value built by the CLI itself (delete, dry-run,
// auth status, …) like an API response.
func RenderValue(o Options, v any, cols Columns) error {
	raw, err := marshalJSON(v)
	if err != nil {
		return err
	}
	return Render(o, raw, cols)
}

// renderJSON re-indents the raw bytes, so every number literal stays as the
// API sent it; only --fields needs a decode (with UseNumber).
func renderJSON(o Options, raw []byte) error {
	if len(o.Fields) > 0 {
		v, err := decode(raw)
		if err != nil {
			return err
		}
		if raw, err = marshalJSON(project(v, o.Fields)); err != nil {
			return err
		}
	}
	var b bytes.Buffer
	// json.Indent keeps trailing whitespace: a body ending in a newline
	// would print a blank line
	if err := json.Indent(&b, bytes.TrimSpace(raw), "", "  "); err != nil {
		return invalidJSON(err)
	}
	b.WriteByte('\n')
	_, err := o.Out.Write(b.Bytes())
	return err
}

func renderYAML(o Options, raw []byte) error {
	v, err := decode(raw)
	if err != nil {
		return err
	}
	if len(o.Fields) > 0 {
		v = project(v, o.Fields)
	}
	b, err := marshalYAML(toYAML(v))
	if err != nil {
		return err
	}
	_, err = o.Out.Write(b)
	return err
}

// marshalYAML encodes v as block YAML; multi-line strings become literal
// blocks.
func marshalYAML(v any) ([]byte, error) {
	b, err := yaml.MarshalWithOptions(v, yaml.UseLiteralStyleIfMultiline(true))
	if err != nil {
		return nil, clierr.New(clierr.CodeError, "YAML-Ausgabe fehlgeschlagen: "+err.Error(), "")
	}
	return b, nil
}

// marshalJSON encodes v compactly without HTML escaping (API texts such as
// "Müller & Söhne" stay readable, as in the raw output).
func marshalJSON(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, clierr.New(clierr.CodeError, "JSON-Ausgabe fehlgeschlagen: "+err.Error(), "")
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}

func invalidJSON(err error) error {
	e := clierr.New(clierr.CodeError, "Antwort ist kein gültiges JSON: "+err.Error(), "")
	e.Cause = err
	return e
}
