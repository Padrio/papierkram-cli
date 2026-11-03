package patch

import (
	"strings"
	"testing"
)

const header = "overlay: 1.0.0\nx-speakeasy-jsonpath: rfc9535\ninfo:\n  title: t\n  version: 1.0.0\n"

func TestApplyRendersSortedIndentedJSON(t *testing.T) {
	spec := []byte(`{"b":{"z":1,"y":[true,null,"s"]},"a":"<&>"}`)
	ov := []byte(header + "actions:\n  - target: $.b\n    update:\n      x: added\n")

	got, err := Apply(spec, ov)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	want := `{
  "a": "<&>",
  "b": {
    "x": "added",
    "y": [
      true,
      null,
      "s"
    ],
    "z": 1
  }
}
`
	if string(got) != want {
		t.Errorf("output mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestApplyPreservesNumberLiterals(t *testing.T) {
	spec := []byte(`{"price":1190.00,"id":9007199254740993,"rate":0.19,"neg":-1e3}`)
	ov := []byte(header + "actions:\n  - target: $\n    update:\n      added: 2.50\n")

	got, err := Apply(spec, ov)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	for _, lit := range []string{`"price": 1190.00`, `"id": 9007199254740993`, `"rate": 0.19`, `"neg": -1e3`, `"added": 2.50`} {
		if !strings.Contains(string(got), lit) {
			t.Errorf("output lacks %s:\n%s", lit, got)
		}
	}
}

func TestApplyKeepsYAMLDatesAsStrings(t *testing.T) {
	spec := []byte(`{"a":{}}`)
	ov := []byte(header + "actions:\n  - target: $.a\n    update:\n      example: 2020-05-15\n")

	got, err := Apply(spec, ov)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !strings.Contains(string(got), `"example": "2020-05-15"`) {
		t.Errorf("date scalar not rendered as string:\n%s", got)
	}
}

func TestApplyRemoveAndFilter(t *testing.T) {
	spec := []byte(`{"s":{"enum":["a",null,"b"],"patternProperties":{}}}`)
	ov := []byte(header + `actions:
  - target: $..enum[?@ == null]
    remove: true
  - target: $..patternProperties
    remove: true
`)
	got, err := Apply(spec, ov)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	want := "{\n  \"s\": {\n    \"enum\": [\n      \"a\",\n      \"b\"\n    ]\n  }\n}\n"
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestApplyFailsOnUnmatchedTarget(t *testing.T) {
	spec := []byte(`{"a":1}`)
	ov := []byte(header + "actions:\n  - target: $.missing\n    update:\n      x: 1\n")

	_, err := Apply(spec, ov)
	if err == nil || !strings.Contains(err.Error(), "$.missing") {
		t.Fatalf("want error naming the unmatched target, got %v", err)
	}
}

func TestApplyFailsOnNoOpUpdate(t *testing.T) {
	spec := []byte(`{"a":{"x":1}}`)
	ov := []byte(header + "actions:\n  - target: $.a\n    update:\n      x: 1\n")

	_, err := Apply(spec, ov)
	if err == nil || !strings.Contains(err.Error(), "does nothing") {
		t.Fatalf("want error for an action without effect, got %v", err)
	}
}

func TestApplyRejectsInvalidInput(t *testing.T) {
	valid := []byte(header + "actions:\n  - target: $\n    update:\n      x: 1\n")
	cases := map[string]struct{ spec, overlay []byte }{
		"spec not parseable":    {[]byte(`{"a":`), valid},
		"overlay not parseable": {[]byte(`{}`), []byte("overlay: [")},
		"overlay invalid":       {[]byte(`{}`), []byte("overlay: 1.0.0\nactions: []\n")},
		"spec is not an object": {[]byte(`[1]`), valid},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Apply(tc.spec, tc.overlay); err == nil {
				t.Fatal("want error, got nil")
			}
		})
	}
}
