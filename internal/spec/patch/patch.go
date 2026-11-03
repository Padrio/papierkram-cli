// Package patch applies an OpenAPI Overlay 1.0 document to a JSON OpenAPI
// spec and renders the result as deterministic JSON (sorted keys, two-space
// indent, trailing newline), so that regenerating is diff-free.
//
// It lives outside tools/ because the spec tests apply the overlay in-process
// to prove that the embedded openapi.gen.json is up to date.
package patch

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/speakeasy-api/openapi/overlay"
	"go.yaml.in/yaml/v3"
)

// Apply applies the overlay to the spec and returns the patched spec as JSON.
//
// Application is strict: every action target must match at least one node
// and every update must change something. A stale action (for example after
// a snapshot refresh fixed the defect upstream) therefore fails loudly
// instead of silently doing nothing.
func Apply(spec, overlayDoc []byte) ([]byte, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(spec, &root); err != nil {
		return nil, fmt.Errorf("parse spec: %w", err)
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) != 1 || root.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("parse spec: top level is not an object")
	}

	ov, err := overlay.ParseReader(bytes.NewReader(overlayDoc))
	if err != nil {
		return nil, fmt.Errorf("parse overlay: %w", err)
	}
	if err := ov.Validate(); err != nil {
		return nil, fmt.Errorf("invalid overlay: %w", err)
	}

	warnings, err := ov.ApplyToStrict(&root)
	if err != nil {
		return nil, fmt.Errorf("apply overlay: %w", err)
	}
	if len(warnings) > 0 {
		return nil, fmt.Errorf("apply overlay: %s", strings.Join(warnings, "; "))
	}

	v, err := toJSON(&root)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("render JSON: %w", err)
	}
	return buf.Bytes(), nil
}

// jsonNumber matches the JSON number grammar (RFC 8259, section 6).
var jsonNumber = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?$`)

// toJSON converts a YAML node tree into values encoding/json can render.
// Maps become map[string]any, which encoding/json writes with sorted keys.
// Numbers keep their literal text (json.RawMessage), so 1190.00 or
// 9007199254740993 survive unchanged instead of passing through float64.
func toJSON(n *yaml.Node) (any, error) {
	switch n.Kind {
	case yaml.DocumentNode:
		return toJSON(n.Content[0])
	case yaml.AliasNode:
		return toJSON(n.Alias)
	case yaml.MappingNode:
		m := make(map[string]any, len(n.Content)/2)
		for i := 0; i < len(n.Content); i += 2 {
			key := n.Content[i].Value
			if _, dup := m[key]; dup {
				return nil, fmt.Errorf("line %d: duplicate key %q", n.Content[i].Line, key)
			}
			v, err := toJSON(n.Content[i+1])
			if err != nil {
				return nil, err
			}
			m[key] = v
		}
		return m, nil
	case yaml.SequenceNode:
		s := make([]any, 0, len(n.Content))
		for _, c := range n.Content {
			v, err := toJSON(c)
			if err != nil {
				return nil, err
			}
			s = append(s, v)
		}
		return s, nil
	case yaml.ScalarNode:
		return scalarToJSON(n)
	default:
		return nil, fmt.Errorf("line %d: unsupported YAML node kind %d", n.Line, n.Kind)
	}
}

func scalarToJSON(n *yaml.Node) (any, error) {
	switch n.ShortTag() {
	case "!!null":
		return nil, nil
	case "!!bool":
		var b bool
		if err := n.Decode(&b); err != nil {
			return nil, fmt.Errorf("line %d: %w", n.Line, err)
		}
		return b, nil
	case "!!int", "!!float":
		if !jsonNumber.MatchString(n.Value) {
			return nil, fmt.Errorf("line %d: number %q is not valid JSON; write it in JSON notation", n.Line, n.Value)
		}
		return json.RawMessage(n.Value), nil
	case "!!str", "!!timestamp":
		// An unquoted 2020-05-15 in the overlay resolves to !!timestamp;
		// OpenAPI has no date type, so keep the text as written.
		return n.Value, nil
	default:
		return nil, fmt.Errorf("line %d: unsupported YAML tag %s", n.Line, n.ShortTag())
	}
}
