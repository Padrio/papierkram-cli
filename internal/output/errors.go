package output

import (
	"io"

	"github.com/Padrio/papierkram-cli/internal/clierr"
)

// errorDoc is the structured error of spec 10.4 (JSON contract). code,
// message and exit_code are always present; hint, http_status and
// correlation_id only when known.
type errorDoc struct {
	Error errorBody `json:"error" yaml:"error"`
}

type errorBody struct {
	Code          clierr.Code `json:"code" yaml:"code"`
	Message       string      `json:"message" yaml:"message"`
	Hint          string      `json:"hint,omitempty" yaml:"hint,omitempty"`
	HTTPStatus    int         `json:"http_status,omitempty" yaml:"http_status,omitempty"`
	CorrelationID string      `json:"correlation_id,omitempty" yaml:"correlation_id,omitempty"`
	ExitCode      int         `json:"exit_code" yaml:"exit_code"`
}

// WriteError writes e to o.Err: always as text (Fehler/Hinweis/
// Correlation-ID); with -o json or --jq additionally as one compact JSON
// line, with -o yaml as a YAML document.
func WriteError(o Options, e *clierr.Error) {
	if e == nil {
		return
	}
	// message, hint and correlation ID can carry outside text (API, project
	// file): no terminal escapes, line breaks stay; e itself is unchanged
	clean := *e
	clean.Message, clean.Hint, clean.CorrelationID = Message(e.Message), Message(e.Hint), Printable(e.CorrelationID)
	e = &clean
	_, _ = io.WriteString(o.Err, e.Format())
	doc := errorDoc{errorBody{
		Code: e.Code, Message: e.Message, Hint: e.Hint,
		HTTPStatus: e.HTTPStatus, CorrelationID: e.CorrelationID, ExitCode: e.Exit(),
	}}
	var b []byte
	var err error
	switch o.errorFormat() {
	case FormatJSON:
		if b, err = marshalJSON(doc); err == nil {
			b = append(b, '\n')
		}
	case FormatYAML:
		b, err = marshalYAML(doc)
	default:
		return
	}
	if err == nil {
		_, _ = o.Err.Write(b)
	}
}
