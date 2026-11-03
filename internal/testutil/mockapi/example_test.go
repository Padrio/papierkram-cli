package mockapi

import (
	"encoding/json"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestJSONExampleFallbacks(t *testing.T) {
	media := func(js string) rawMediaType {
		var mt rawMediaType
		if err := json.Unmarshal([]byte(js), &mt); err != nil {
			t.Fatal(err)
		}
		return mt
	}
	withExample := &openapi3.SchemaRef{Value: &openapi3.Schema{Example: map[string]any{"from": "schema"}}}
	for name, tc := range map[string]struct {
		mt     rawMediaType
		schema *openapi3.SchemaRef
		want   string
	}{
		"first example by key":  {media(`{"examples":{"b":{"value":{"n":2}},"a":{"value":{"n": 1190.00}}}}`), withExample, `{"n":1190.00}`},
		"null example skipped":  {media(`{"examples":{"a":{"value":null},"b":{"value":[1]}}}`), nil, `[1]`},
		"example":               {media(`{"example":{"from":"example"}}`), withExample, `{"from":"example"}`},
		"schema example":        {media(`{}`), withExample, `{"from":"schema"}`},
		"no example":            {media(`{}`), nil, `{}`},
		"no example, no schema": {rawMediaType{}, &openapi3.SchemaRef{}, `{}`},
	} {
		t.Run(name, func(t *testing.T) {
			if got := string(jsonExample(tc.mt, tc.schema)); got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}
