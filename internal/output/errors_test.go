package output

import (
	"bytes"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/clierr"
)

func writeErr(o Options, e *clierr.Error) string {
	var b bytes.Buffer
	o.Err = &b
	WriteError(o, e)
	return b.String()
}

func fullErr() *clierr.Error {
	e := clierr.New(clierr.CodeNotFound, `Rechnung "7" nicht gefunden`, "ID prüfen; gehört sie zu diesem Account/Kontext?")
	e.HTTPStatus = 404
	e.CorrelationID = "abc-123"
	return e
}

const fullText = "Fehler: Rechnung \"7\" nicht gefunden\nHinweis: ID prüfen; gehört sie zu diesem Account/Kontext?\nCorrelation-ID: abc-123\n"

func TestWriteErrorTextOnly(t *testing.T) {
	for _, f := range []Format{FormatTable, FormatWide} {
		if got := writeErr(Options{Format: f}, fullErr()); got != fullText {
			t.Errorf("%s: %q", f, got)
		}
	}
}

// JSON contract (spec 10.4): one compact line after the text; code, message
// and exit_code are always present, hint, http_status and correlation_id
// only when set.
func TestWriteErrorJSONShape(t *testing.T) {
	want := fullText + `{"error":{"code":"not_found","message":"Rechnung \"7\" nicht gefunden","hint":"ID prüfen; gehört sie zu diesem Account/Kontext?","http_status":404,"correlation_id":"abc-123","exit_code":4}}` + "\n"
	if got := writeErr(Options{Format: FormatJSON}, fullErr()); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	minimal := clierr.New(clierr.CodeUsage, "a < b & c", "")
	want = "Fehler: a < b & c\n" + `{"error":{"code":"usage","message":"a < b & c","exit_code":2}}` + "\n"
	if got := writeErr(Options{Format: FormatJSON}, minimal); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestWriteErrorJQImpliesJSON(t *testing.T) {
	want := "Fehler: x\n" + `{"error":{"code":"auth","message":"x","exit_code":3}}` + "\n"
	if got := writeErr(Options{Format: FormatTable, JQ: ".id"}, clierr.New(clierr.CodeAuth, "x", "")); got != want {
		t.Errorf("got %q", got)
	}
}

func TestWriteErrorYAML(t *testing.T) {
	want := fullText + `error:
  code: not_found
  message: Rechnung "7" nicht gefunden
  hint: ID prüfen; gehört sie zu diesem Account/Kontext?
  http_status: 404
  correlation_id: abc-123
  exit_code: 4
`
	if got := writeErr(Options{Format: FormatYAML}, fullErr()); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	want = "Fehler: x\nerror:\n  code: error\n  message: x\n  exit_code: 1\n"
	if got := writeErr(Options{Format: FormatYAML}, clierr.New(clierr.CodeError, "x", "")); got != want {
		t.Errorf("got %q", got)
	}
}

func TestWriteErrorNil(t *testing.T) {
	if got := writeErr(Options{Format: FormatJSON}, nil); got != "" {
		t.Errorf("got %q", got)
	}
}
