package output

import (
	"bytes"
	"encoding/json"
	"strings"
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

// Security review: message, hint and correlation ID of an error and the
// text of a warning reach the terminal without control characters (ESC of
// ANSI sequences) or invisible format characters (bidi override U+202E);
// only line breaks of multi-line messages stay. The JSON stays valid.
func TestWriteErrorAndWarnSanitize(t *testing.T) {
	e := &clierr.Error{Code: clierr.CodeValidation,
		Message:       "Feld \x1b[31mrot\x1b[0m\nzweite Zeile\u202eevil\r",
		Hint:          "Hinweis\x07 mit \u200bBreite\nund Zeile",
		CorrelationID: "id\x1b]0;x\x07"}
	for _, format := range []Format{FormatTable, FormatJSON, FormatYAML} {
		var errb bytes.Buffer
		WriteError(Options{Format: format, Err: &errb}, e)
		got := errb.String()
		if strings.ContainsAny(got, "\x1b\x07\r\u202e\u200b") {
			t.Errorf("%s: control characters in %q", format, got)
		}
		if format == FormatTable && (!strings.Contains(got, "rot [0m\nzweite Zeile evil") || !strings.Contains(got, "\nund Zeile")) {
			t.Errorf("%s: line breaks lost: %q", format, got)
		}
		if format == FormatJSON {
			line := got[strings.LastIndex(strings.TrimRight(got, "\n"), "\n")+1:]
			var doc map[string]map[string]any
			if err := json.Unmarshal([]byte(line), &doc); err != nil {
				t.Fatalf("invalid JSON %q: %v", line, err)
			}
			if msg, _ := doc["error"]["message"].(string); !strings.Contains(msg, "\nzweite Zeile") || strings.ContainsAny(msg, "\x1b\u202e") {
				t.Errorf("JSON message = %q", msg)
			}
		}
	}
	// the error itself is not changed
	if !strings.Contains(e.Message, "\x1b") {
		t.Error("WriteError modified the error")
	}

	var w bytes.Buffer
	Warn(&w, "Datei \x1b[2J\u202eweg\nzweite")
	if got := w.String(); got != "Warnung: Datei  [2J weg\nzweite\n" {
		t.Errorf("Warn = %q", got)
	}
}

// Printable (one line) and the message filter share one implementation:
// Printable also replaces line breaks.
func TestPrintable(t *testing.T) {
	if got := Printable("a\nb\x1b\u202ec\td"); got != "a b  c d" {
		t.Errorf("Printable = %q", got)
	}
}

// Review N2: ZWJ (emoji sequences), soft hyphen and BOM disguise nothing;
// they are removed instead of becoming a visible space. Bidi controls and
// zero-width spaces still become spaces.
func TestPrintableDropsHarmlessFormatCharacters(t *testing.T) {
	for in, want := range map[string]string{
		"Team \U0001F468\u200d\U0001F469\u200d\U0001F467": "Team \U0001F468\U0001F469\U0001F467",
		"Rech\u00adnung":   "Rechnung",
		"\ufeffACME Corp.": "ACME Corp.",
		"a\u202eb\u2066c":  "a b c",
		"a\u200bb":         "a b",
	} {
		if got := Printable(in); got != want {
			t.Errorf("Printable(%q) = %q, want %q", in, got, want)
		}
		if got := Message(in); got != want {
			t.Errorf("Message(%q) = %q, want %q", in, got, want)
		}
	}
	if got := oneLine("\ufeffRech\u00adnung"); got != "Rechnung" {
		t.Errorf("oneLine = %q", got)
	}
}
