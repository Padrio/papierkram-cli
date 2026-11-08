package api

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/Padrio/papierkram-cli/internal/clierr"
)

// Hints shown with mapped errors (spec 10.4).
const (
	hintServer         = "Papierkram-Serverfehler, später erneut versuchen"
	hintAuth           = "Token prüfen: Einstellungen → API; papierkram auth login"
	hintNotFound       = "ID prüfen; gehört sie zu diesem Account/Kontext?"
	hintValidation     = "Eingaben prüfen; Formate: Datum YYYY-MM-DD, vat_rate als Dezimalzahl (z. B. 0.19)"
	hintQuota          = "Monatskontingent erschöpft, Paket prüfen"
	hintNetwork        = "Subdomain und Verbindung prüfen"
	hintOutcomeUnknown = "Ergebnis unbekannt – vor Wiederholung per describe/list prüfen"
	// hintServerMutation replaces hintServer for POST, PUT and DELETE: the
	// server may have booked or sent before failing.
	hintServerMutation = "Papierkram-Serverfehler – die Änderung kann trotzdem ausgeführt worden sein: vor Wiederholung per describe/list prüfen"
	// hintTooLarge gives no size advice: which flag asks for less is known
	// only to the command (ruling T15-A; list sets its own hint).
	hintTooLarge       = "Antwort zu groß für das CLI; Ergebnis nicht verwertbar"
	hintTooLargeChange = "NICHT wiederholen; Ergebnis per describe/list prüfen"
)

// WithoutNetworkHint is hint without the network hint of a transport
// error at its start (and the separator a command put after it: ". " or
// "; "): after an interrupt (Ctrl+C) the request did not fail, so
// checking subdomain and connection is no advice; what a command added
// stays. Any other hint is returned unchanged.
func WithoutNetworkHint(hint string) string {
	rest, ok := strings.CutPrefix(hint, hintNetwork)
	if !ok {
		return hint
	}
	for _, sep := range []string{". ", "; ", "."} {
		if r, ok := strings.CutPrefix(rest, sep); ok {
			return r
		}
	}
	if rest != "" {
		// the constant continues as another word: not the network hint
		return hint
	}
	return ""
}

// ErrResponseTooLarge is the cause of the error of a 2xx answer longer
// than MaxResponseSize (tooLargeError); callers test it with errors.Is.
var ErrResponseTooLarge = errors.New("api: Antwort größer als MaxResponseSize")

// apiErrorBody is the API error body (spec 4.1):
// {type:"error", error_type, message, correlation_id}.
type apiErrorBody struct {
	Message       string `json:"message"`
	CorrelationID string `json:"correlation_id"`
}

// statusError maps a non-2xx response to a typed error; for a 5xx on a
// mutation the hint asks to check before repeating. The message comes
// from the API error body; bodies that are not JSON (proxy HTML, empty 401)
// get a short German default per status and are never echoed.
func statusError(method string, status int, body []byte) *clierr.Error {
	code, hint, msg := classifyStatus(status)
	if code == clierr.CodeServer && !safeMethod(method) {
		hint = hintServerMutation
	}
	e := &clierr.Error{Code: code, Message: msg, Hint: hint, HTTPStatus: status}
	var b apiErrorBody
	if json.Unmarshal(body, &b) == nil {
		if b.Message != "" {
			e.Message = b.Message
		}
		e.CorrelationID = b.CorrelationID
	}
	return e
}

// classifyStatus returns code, hint and default message for an HTTP status.
func classifyStatus(status int) (clierr.Code, string, string) {
	switch {
	case status == http.StatusBadRequest:
		return clierr.CodeValidation, hintValidation, "Ungültige Anfrage (HTTP 400)"
	case status == http.StatusUnprocessableEntity:
		return clierr.CodeValidation, hintValidation, "Eingaben von der API abgelehnt (HTTP 422)"
	case status == http.StatusUnauthorized:
		return clierr.CodeAuth, hintAuth, "Nicht angemeldet oder Token ungültig (HTTP 401)"
	case status == http.StatusForbidden:
		return clierr.CodeAuth, hintAuth, "Zugriff verweigert (HTTP 403)"
	case status == http.StatusNotFound:
		return clierr.CodeNotFound, hintNotFound, "Nicht gefunden (HTTP 404)"
	case status == http.StatusConflict:
		return clierr.CodeConflict, "", "Konflikt mit dem aktuellen Stand (HTTP 409)"
	case status == http.StatusTooManyRequests:
		return clierr.CodeQuota, hintQuota, "Monatskontingent der Papierkram-API erschöpft (HTTP 429)"
	case status >= 500:
		return clierr.CodeServer, hintServer, fmt.Sprintf("Papierkram-Server nicht erreichbar oder fehlerhaft (HTTP %d)", status)
	default:
		return clierr.CodeError, "", fmt.Sprintf("Unerwartete Antwort der Papierkram-API (HTTP %d)", status)
	}
}

// tooLargeError is the error of a 2xx answer longer than MaxResponseSize
// (spec 5.5): exit 1 (code error, never retried). For a mutation the
// server has made the change, so the hint says not to repeat it. It has
// no HTTP status (ruling FR-D): a 2xx status is no error status in the
// contract (docs/schemas/error.json), the message says what happened.
func tooLargeError(method string) *clierr.Error {
	e := &clierr.Error{
		Code:    clierr.CodeError,
		Message: "Antwort größer als 64 MiB, abgebrochen",
		Hint:    hintTooLarge,
		Cause:   ErrResponseTooLarge,
	}
	if !safeMethod(method) {
		e.Message = "Antwort größer als 64 MiB; die Änderung wurde ausgeführt"
		e.Hint = hintTooLargeChange
	}
	return e
}

// isRedirect reports a 3xx status.
func isRedirect(status int) bool { return status >= 300 && status <= 399 }

// redirectError is the error of a mutation answered with a redirect, which
// redirectPolicy does not follow. The server may have acted before
// redirecting (Post/Redirect/Get), so the outcome is unknown. The status
// is only in the message: outcome_unknown carries no http_status (error
// contract, docs/schemas/error.json), there was no answer to the mutation
// itself.
func redirectError(method, path string, status int) *clierr.Error {
	return &clierr.Error{
		Code:    clierr.CodeOutcomeUnknown,
		Message: fmt.Sprintf("%s %s mit einer Weiterleitung beantwortet (HTTP %d), ihr wurde nicht gefolgt", method, path, status),
		Hint:    hintOutcomeUnknown,
	}
}

// transportError classifies a failed exchange. sent reports whether the
// request may have reached the server (fully written or a response
// started). Then a mutation's outcome is unknown; otherwise, and always
// for safe methods, it is a plain network error.
func transportError(method, path string, sent bool, err error) *clierr.Error {
	reason := transportReason(err)
	if sent && !safeMethod(method) {
		return &clierr.Error{
			Code:    clierr.CodeOutcomeUnknown,
			Message: fmt.Sprintf("%s %s gesendet, aber keine vollständige Antwort erhalten (%s)", method, path, reason),
			Hint:    hintOutcomeUnknown,
			Cause:   err,
		}
	}
	return &clierr.Error{
		Code:    clierr.CodeNetwork,
		Message: fmt.Sprintf("Papierkram-API nicht erreichbar (%s)", reason),
		Hint:    hintNetwork,
		Cause:   err,
	}
}

// transportReason is a short German reason for err, without the URL.
func transportReason(err error) string {
	var (
		ue  *url.Error
		dns *net.DNSError
	)
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "Zeitüberschreitung"
	case errors.Is(err, context.Canceled):
		return "abgebrochen"
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return "Verbindung vom Server geschlossen"
	case errors.As(err, &dns) && dns.Name == "":
		return "Adresse nicht auflösbar"
	case errors.As(err, &dns):
		return fmt.Sprintf("Adresse nicht auflösbar (%s)", dns.Name)
	case isCertificateError(err):
		return "TLS-Zertifikat ungültig"
	case errors.As(err, &ue):
		return ue.Err.Error()
	default:
		return err.Error()
	}
}

// isCertificateError reports whether err is a failed TLS certificate check.
func isCertificateError(err error) bool {
	var (
		verify    *tls.CertificateVerificationError
		authority x509.UnknownAuthorityError
		hostname  x509.HostnameError
		invalid   x509.CertificateInvalidError
	)
	return errors.As(err, &verify) || errors.As(err, &authority) ||
		errors.As(err, &hostname) || errors.As(err, &invalid)
}

// safeMethod reports whether repeating method cannot change server state.
func safeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	return false
}
