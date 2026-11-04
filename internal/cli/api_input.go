package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/api"
	"github.com/Padrio/papierkram-cli/internal/clierr"
)

// The input of papierkram api: the request path, the fields of -f/-F and
// the body (--input merged with the fields).

// apiPath is a checked request path: the canonical path that is sent
// (escaped, without /api/v1 and query), its decoded form for MatchPath and
// the query of the path.
type apiPath struct {
	send, match string
	query       url.Values
}

// parseAPIPath checks the path of papierkram api (exit 2): relative to
// the API base, starting with "/", without scheme or host, backslash,
// fragment, control characters, empty, "." or ".." segments (also
// percent-encoded, see api.CheckPath) or encoded slashes. The path that is
// sent is rebuilt from its decoded segments, so it is exactly the one the
// class was looked up for; a leading /api/v1 is the base and dropped.
func parseAPIPath(c *cobra.Command, raw string) (apiPath, error) {
	bad := func(why string) (apiPath, error) {
		return apiPath{}, clierr.New(clierr.CodeUsage, fmt.Sprintf("ungültiger Pfad %q: %s", raw, why),
			"Pfad relativ zu /api/v1 angeben, z. B. /income/invoices?page=2. Verwendung: "+useLine(c))
	}
	switch {
	case raw == "":
		return bad("Pfad fehlt")
	case !strings.HasPrefix(raw, "/"):
		return bad("muss mit / beginnen (keine vollständige URL)")
	case strings.HasPrefix(raw, "//"):
		return bad("keine Host-Angabe erlaubt")
	case strings.Contains(raw, "#"):
		return bad("# nicht erlaubt")
	}
	if err := api.CheckPath(raw); err != nil {
		var pe *api.PathError
		if errors.As(err, &pe) {
			return bad(pe.Reason)
		}
		return bad(err.Error())
	}
	p, q, _ := strings.Cut(raw, "?")
	query, err := url.ParseQuery(q)
	if err != nil {
		return bad("Query nicht lesbar")
	}
	trailing := len(p) > 1 && strings.HasSuffix(p, "/")
	var segs []string
	if p = strings.TrimSuffix(p[1:], "/"); p != "" || trailing {
		for _, s := range strings.Split(p, "/") {
			d, _ := url.PathUnescape(s) // checked by api.CheckPath
			switch {
			case d == "":
				return bad("leeres Segment (//)")
			case d == "." || d == "..":
				return bad(`Segment "` + d + `" nicht erlaubt`)
			case strings.Contains(d, "/"):
				return bad("kodierter Schrägstrich (%2F) nicht erlaubt")
			}
			segs = append(segs, d)
		}
	}
	if len(segs) >= 2 && segs[0] == "api" && segs[1] == "v1" {
		segs = segs[2:]
	}
	esc := make([]string, len(segs))
	for i, s := range segs {
		esc[i] = url.PathEscape(s)
	}
	ap := apiPath{send: "/" + strings.Join(esc, "/"), match: "/" + strings.Join(segs, "/"), query: query}
	if trailing && len(segs) > 0 {
		ap.send += "/"
	}
	return ap, nil
}

// apiField is one -f or -F value: the key split at dots, the raw value
// and whether it is typed (-F).
type apiField struct {
	key   []string
	value string
	typed bool
}

func (f apiField) name() string { return strings.Join(f.key, ".") }

// flag is how the field was given, for messages ("-F email.body").
func (f apiField) flag() string {
	if f.typed {
		return "-F " + f.name()
	}
	return "-f " + f.name()
}

// stdin reports a -F field that reads the standard input (@-).
func (f apiField) stdin() bool { return f.typed && f.value == "@-" }

// parseAPIFields parses -f and -F (exit 2): key=value, keys of non-empty
// dot-separated parts, each key once and no key inside another (a=1 with
// a.b=2), @ with a file name.
func parseAPIFields(c *cobra.Command) ([]apiField, error) {
	var fields []apiField
	for _, fl := range []struct {
		name  string
		typed bool
	}{{"raw-field", false}, {"field", true}} {
		vals, _ := c.Flags().GetStringArray(fl.name)
		short := "-f"
		if fl.typed {
			short = "-F"
		}
		for _, v := range vals {
			k, val, ok := strings.Cut(v, "=")
			key := strings.Split(k, ".")
			if !ok || slices.Contains(key, "") {
				return nil, clierr.New(clierr.CodeUsage, fmt.Sprintf("%s %q: key=value erwartet", short, v),
					"Schlüssel aus Teilen ohne Leerstellen, verschachtelt mit Punkten, z. B. "+short+" email.recipient=kunde@example.org")
			}
			f := apiField{key: key, value: val, typed: fl.typed}
			if f.typed && val == "@" {
				return nil, errMissingPath(f.flag() + "=@")
			}
			for _, g := range fields {
				if n := min(len(g.key), len(f.key)); slices.Equal(g.key[:n], f.key[:n]) {
					return nil, clierr.New(clierr.CodeUsage, fmt.Sprintf("Feld %s und %s widersprechen sich", g.flag(), f.flag()),
						"jeden Schlüssel nur einmal angeben; ein Schlüssel ist entweder Wert oder Objekt")
				}
			}
			fields = append(fields, f)
		}
	}
	return fields, nil
}

// apiBody builds the body: the JSON object of --input as given (not
// pruned), then the fields, which win.
func (a *App) apiBody(call *apiCall) (map[string]any, error) {
	body := map[string]any{}
	if call.input != "" {
		in, err := a.readInput(call.input)
		if err != nil {
			return nil, err
		}
		body = in
	}
	for _, f := range call.fields {
		v, err := a.fieldValue(f)
		if err != nil {
			return nil, err
		}
		setPath(body, f.key, v)
	}
	return body, nil
}

// fieldValue is the value of a field: -f the text; -F true, false, null, a
// JSON number (json.Number, exact), the text of @file or @- (read like
// --body-file: BOM removed, UTF-8 required, line breaks kept), else the
// text.
func (a *App) fieldValue(f apiField) (any, error) {
	if !f.typed {
		return f.value, nil
	}
	switch v := f.value; {
	case v == "true":
		return true, nil
	case v == "false":
		return false, nil
	case v == "null":
		return nil, nil
	case isJSONNumber(v):
		return json.Number(v), nil
	case strings.HasPrefix(v, "@"):
		return a.readText(f.flag(), v[1:])
	default:
		return v, nil
	}
}

// jsonNumberRe is the grammar of a JSON number (RFC 8259).
var jsonNumberRe = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

// isJSONNumber reports whether s is a number in JSON syntax.
func isJSONNumber(s string) bool { return jsonNumberRe.MatchString(s) }

// queryText is the text of a field value in the query: null is empty
// (like gh api), every other value its text.
func queryText(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

// encodeBody encodes the body as compact JSON without HTML escaping;
// json.Number values keep their literal.
func encodeBody(body map[string]any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(body); err != nil {
		return nil, clierr.New(clierr.CodeValidation, "Body nicht als JSON darstellbar: "+err.Error(), "")
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}
