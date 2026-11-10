package mcpserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// ArgError is a tool argument the CLI would not get as the agent meant
// it (unknown parameter, wrong JSON type, missing required parameter, "-"
// as a path). The tool call ends as a usage error (exit 2) without
// running the CLI.
type ArgError struct{ Msg, Hint string }

func (e *ArgError) Error() string { return e.Msg }

// quoted is s in German quotes, with control characters escaped like
// strconv.Quote does (a parameter name comes from the client).
func quoted(s string) string {
	q := strconv.Quote(s)
	return "„" + strings.ReplaceAll(q[1:len(q)-1], `\"`, `"`) + "“"
}

// Args translates the JSON arguments of a call of t into the argument list
// of the CLI: the command words, every given flag as --<flag>=<value> (a
// repeatable flag once per element), sorted by parameter name, the Fixed
// arguments, --no-input -o json, --context=<ctx> and --debug from o, then
// "--" and the positional arguments in Pos order. Every value is glued to
// its flag and the positional arguments follow "--", so no value can set
// another option. A KindJSONFile value is written to <tmpDir>/<flag>.json
// (0600) and the flag gets its path.
func Args(t ToolSpec, raw json.RawMessage, o Options, tmpDir string) ([]string, error) {
	props, err := decodeObject(raw)
	if err != nil {
		return nil, err
	}
	byName := map[string]Param{}
	for _, p := range t.Params {
		byName[p.Name] = p
	}
	for name := range props {
		if _, ok := byName[name]; !ok {
			return nil, &ArgError{
				Msg:  fmt.Sprintf("unbekannter Parameter %s für %s", quoted(name), t.Name),
				Hint: "Erlaubte Parameter: " + paramNames(t),
			}
		}
	}
	params := slices.Clone(t.Params)
	slices.SortFunc(params, func(a, b Param) int { return strings.Compare(a.Name, b.Name) })
	var flags []string
	positional := map[int]string{}
	for _, p := range params {
		v, ok := props[p.Name]
		if !ok {
			if p.Required {
				return nil, &ArgError{
					Msg:  fmt.Sprintf("Parameter %s fehlt für %s", quoted(p.Name), t.Name),
					Hint: "Pflichtparameter: " + requiredNames(t),
				}
			}
			continue
		}
		values, err := p.values(v, tmpDir)
		if err != nil {
			return nil, err
		}
		if p.Pos >= 0 {
			positional[p.Pos] = values[0]
			continue
		}
		for _, s := range values {
			flags = append(flags, "--"+p.Flag+"="+s)
		}
	}
	args := slices.Concat(t.Command, flags, t.Fixed, []string{"--no-input", "-o", "json"})
	if o.Context != "" {
		args = append(args, "--context="+o.Context)
	}
	if o.Debug {
		args = append(args, "--debug")
	}
	args = append(args, "--")
	for _, pos := range slices.Sorted(maps.Keys(positional)) {
		args = append(args, positional[pos])
	}
	return args, nil
}

// decodeObject decodes the arguments of a call (numbers kept as written);
// no arguments or null are an empty object.
func decodeObject(raw json.RawMessage) (map[string]json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return map[string]json.RawMessage{}, nil
	}
	var props map[string]json.RawMessage
	if err := json.Unmarshal(raw, &props); err != nil || props == nil {
		return nil, &ArgError{Msg: "Argumente sind kein JSON-Objekt", Hint: "Argumente als JSON-Objekt übergeben, siehe Eingabe-Schema des Tools"}
	}
	return props, nil
}

// paramNames lists the parameters of t, sorted.
func paramNames(t ToolSpec) string {
	var names []string
	for _, p := range t.Params {
		names = append(names, p.Name)
	}
	slices.Sort(names)
	if len(names) == 0 {
		return "keine"
	}
	return strings.Join(names, ", ")
}

// requiredNames lists the required parameters of t, sorted.
func requiredNames(t ToolSpec) string {
	var names []string
	for _, p := range t.Params {
		if p.Required {
			names = append(names, p.Name)
		}
	}
	slices.Sort(names)
	return strings.Join(names, ", ")
}

// typeErr is the error of a value of the wrong JSON type for p.
func (p Param) typeErr(want string) error {
	return &ArgError{
		Msg:  fmt.Sprintf("Parameter %s: %s erwartet", quoted(p.Name), want),
		Hint: "Typen stehen im Eingabe-Schema des Tools (tools/list)",
	}
}

// values are the CLI values of the JSON value v of p: one for a scalar,
// one per element for KindStrings.
func (p Param) values(v json.RawMessage, tmpDir string) ([]string, error) {
	dec := json.NewDecoder(bytes.NewReader(v))
	dec.UseNumber()
	var val any
	// v is a value of the decoded arguments object: it decodes
	_ = dec.Decode(&val)
	switch p.Kind {
	case KindString:
		if s, ok := val.(string); ok {
			return []string{s}, nil
		}
		return nil, p.typeErr("Text")
	case KindPath:
		s, ok := val.(string)
		if !ok {
			return nil, p.typeErr("Dateipfad als Text")
		}
		if s == "-" {
			return nil, &ArgError{
				Msg:  fmt.Sprintf("Parameter %s: „-“ (stdin/stdout) ist im MCP-Server nicht erlaubt", quoted(p.Name)),
				Hint: "Einen Dateipfad angeben; stdin und stdout sind der Transport des MCP-Servers",
			}
		}
		return []string{s}, nil
	case KindInteger:
		n, ok := val.(json.Number)
		if !ok {
			return nil, p.typeErr("ganze Zahl")
		}
		return p.integer(n)
	case KindRef:
		switch x := val.(type) {
		case json.Number:
			return p.integer(x)
		case string:
			return []string{x}, nil
		}
		return nil, p.typeErr("ganze Zahl (ID) oder Name als Text")
	case KindNumber:
		if n, ok := val.(json.Number); ok {
			return []string{n.String()}, nil
		}
		return nil, p.typeErr("Zahl")
	case KindBool:
		if b, ok := val.(bool); ok {
			return []string{strconv.FormatBool(b)}, nil
		}
		return nil, p.typeErr("Wahrheitswert (true oder false)")
	case KindStrings:
		list, ok := val.([]any)
		if !ok {
			return nil, p.typeErr("Liste von Texten")
		}
		out := make([]string, 0, len(list))
		for _, e := range list {
			s, ok := e.(string)
			if !ok {
				return nil, p.typeErr("Liste von Texten")
			}
			out = append(out, s)
		}
		return out, nil
	case KindJSONFile:
		return p.jsonFile(val, v, tmpDir)
	}
	return nil, fmt.Errorf("parameter %s: unknown kind %q", p.Name, p.Kind)
}

// integer is n as a decimal integer; a fraction or exponent is no integer.
func (p Param) integer(n json.Number) ([]string, error) {
	i, err := strconv.ParseInt(n.String(), 10, 64)
	if err != nil {
		return nil, p.typeErr("ganze Zahl")
	}
	return []string{strconv.FormatInt(i, 10)}, nil
}

// jsonFile writes the JSON value raw (decoded as val) of p to
// <tmpDir>/<flag>.json with mode 0600 and returns its path. The value
// must have the JSON type of p (object or array).
func (p Param) jsonFile(val any, raw json.RawMessage, tmpDir string) ([]string, error) {
	switch _, isArray := val.([]any); {
	case p.JSONType == "array" && !isArray:
		return nil, p.typeErr("JSON-Liste")
	case p.JSONType != "array":
		if _, ok := val.(map[string]any); !ok {
			return nil, p.typeErr("JSON-Objekt")
		}
	}
	// tmpDir is new for every call (New: os.MkdirTemp, mode 0700)
	path := filepath.Join(tmpDir, p.Flag+".json")
	if err := os.WriteFile(path, bytes.TrimSpace(raw), 0o600); err != nil {
		return nil, err
	}
	return []string{path}, nil
}
