package config

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/Padrio/papierkram-cli/internal/clierr"
)

// tomlKeys returns the TOML key names of a struct type in field order.
func tomlKeys(v any) []string {
	t := reflect.TypeOf(v)
	keys := make([]string, 0, t.NumField())
	for i := range t.NumField() {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("toml"), ",")
		if name != "" && name != "-" {
			keys = append(keys, name)
		}
	}
	return keys
}

// unknownKeyError turns go-toml's strict-mode error into a German usage error
// naming the first unknown key, the file and the closest known key.
func unknownKeyError(path string, sme *toml.StrictMissingError) error {
	var key toml.Key
	if len(sme.Errors) > 0 {
		key = sme.Errors[0].Key()
	}
	name, where, known := "", "", tomlKeys(Config{})
	section := "Erlaubte Schlüssel: "
	switch {
	case len(key) >= 3 && key[0] == "contexts":
		name, where, known = key[2], fmt.Sprintf(" in [contexts.%s]", key[1]), tomlKeys(Context{})
		section = "Erlaubte Schlüssel in [contexts.<name>]: "
	case len(key) > 0:
		name = key[0]
	}
	msg := fmt.Sprintf("unbekannter Schlüssel %q%s der Konfigurationsdatei %s", name, where, path)
	if n := len(sme.Errors) - 1; n > 0 {
		msg += fmt.Sprintf(" (und %d weitere)", n)
	}
	hint := section + strings.Join(known, ", ")
	if s := suggestKey(name, known); s != "" {
		hint = fmt.Sprintf("Meinten Sie %q? ", s) + hint
	}
	e := clierr.New(clierr.CodeUsage, msg, hint)
	e.Cause = sme
	return e
}

// suggestKey returns the known key closest to name: equal after ignoring
// case, "-" and "_", or at most two edits away. "" if none is close.
func suggestKey(name string, known []string) string {
	norm := func(s string) string {
		return strings.NewReplacer("-", "", "_", "").Replace(strings.ToLower(s))
	}
	best, bestDist := "", 3
	for _, k := range known {
		if norm(k) == norm(name) {
			return k
		}
		if d := levenshtein(strings.ToLower(name), k); d < bestDist {
			best, bestDist = k, d
		}
	}
	return best
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}

// decodeStrict decodes data into c and rejects unknown keys.
func decodeStrict(path string, data []byte, c *Config) error {
	err := toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields().Decode(c)
	var sme *toml.StrictMissingError
	if errors.As(err, &sme) {
		return unknownKeyError(path, sme)
	}
	return err
}
