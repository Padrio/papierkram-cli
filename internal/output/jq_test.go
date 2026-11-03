package output

import (
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/clierr"
)

func jq(t *testing.T, expr, raw string) string {
	t.Helper()
	return mustRender(t, Options{Format: FormatTable, JQ: expr}, raw)
}

func TestJQEntriesIDs(t *testing.T) {
	if got := jq(t, ".entries[].id", list); got != "1\n9007199254740993\n3\n" {
		t.Errorf("got %q", got)
	}
}

func TestJQStringsRaw(t *testing.T) {
	if got := jq(t, ".entries[].name", list); got != "A\nB\nC\n" {
		t.Errorf("got %q", got)
	}
	// Non-string values are compact JSON, one result per line.
	if got := jq(t, ".entries[0].billing, [.page], null, true", list); got != "{\"city\":\"Köln\",\"company\":\"Müller GmbH\"}\n[1]\nnull\ntrue\n" {
		t.Errorf("got %q", got)
	}
}

// Review Focus 2 through --jq: number literals pass through unchanged as
// long as the expression does not compute with them. Arithmetic works on
// float64 (decimals) or big integers, so 1190.00+0 prints 1190 (pinned,
// see jq.go).
func TestJQKeepsNumbersExact(t *testing.T) {
	if got := jq(t, ".", precise); got != `{"id":9007199254740993,"name":"Müller & Söhne <GmbH>","price":19.99,"total_gross":1190.00}`+"\n" {
		t.Errorf("identity: %q", got)
	}
	if got := jq(t, ".total_gross, .price, .id", precise); got != "1190.00\n19.99\n9007199254740993\n" {
		t.Errorf("paths: %q", got)
	}
	if got := jq(t, ".total_gross + 0, .price * 1, .id + 1", precise); got != "1190\n19.99\n9007199254740994\n" {
		t.Errorf("arithmetic: %q", got)
	}
}

func TestJQAfterFields(t *testing.T) {
	got := mustRender(t, Options{Format: FormatJSON, Fields: []string{"id"}, JQ: ".entries[0]"}, list)
	if got != "{\"id\":1}\n" {
		t.Errorf("got %q", got)
	}
}

func TestJQHasNoEnvironment(t *testing.T) {
	t.Setenv("PAPIERKRAM_TOKEN", "test-token-env")
	if got := jq(t, "$ENV | length, (env | length)", `{}`); got != "0\n0\n" {
		t.Errorf("got %q", got)
	}
}

func TestJQErrorsAreUsageErrors(t *testing.T) {
	for _, expr := range []string{".[", "foo(1)", ".id | keys"} {
		out, _, err := render(t, Options{Format: FormatJSON, JQ: expr}, `{"id":1}`)
		ce := clierr.From(err)
		if ce == nil || ce.Exit() != 2 || !strings.Contains(ce.Message, "jq-Ausdruck") || out != "" {
			t.Errorf("%q: err = %+v, out = %q", expr, ce, out)
		}
	}
}

func TestJQPartialResultsNotPrintedOnError(t *testing.T) {
	out, _, err := render(t, Options{Format: FormatJSON, JQ: ".[] | keys"}, `[{"a":1},2]`)
	if err == nil || out != "" {
		t.Errorf("err = %v, out = %q", err, out)
	}
}

func TestJQHaltStopsWithoutError(t *testing.T) {
	if got := jq(t, "1, halt, 2", `{}`); got != "1\n" {
		t.Errorf("got %q", got)
	}
}
