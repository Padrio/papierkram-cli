package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Padrio/papierkram-cli/internal/clierr"
)

// asCLIErr asserts that err is a *clierr.Error and returns it.
func asCLIErr(t *testing.T, err error) *clierr.Error {
	t.Helper()
	var ce *clierr.Error
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v (%T), want *clierr.Error", err, err)
	}
	return ce
}

func TestStatusMapping(t *testing.T) {
	tests := []struct {
		status int
		code   clierr.Code
		exit   int
		hint   string // substring; "" means the hint must be empty
	}{
		{400, clierr.CodeValidation, 5, "YYYY-MM-DD"},
		{401, clierr.CodeAuth, 3, "papierkram auth login"},
		{403, clierr.CodeAuth, 3, "Token prüfen"},
		{404, clierr.CodeNotFound, 4, "ID prüfen"},
		{409, clierr.CodeConflict, 7, ""},
		{418, clierr.CodeError, 1, ""},
		{422, clierr.CodeValidation, 5, "0.19"},
		{429, clierr.CodeQuota, 6, "Monatskontingent erschöpft, Paket prüfen"},
		{500, clierr.CodeServer, 1, "später erneut versuchen"},
		{502, clierr.CodeServer, 1, "später erneut versuchen"},
		{503, clierr.CodeServer, 1, "später erneut versuchen"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.status), func(t *testing.T) {
			rec := &recorder{status: tt.status, body: `{"type":"error","error_type":"x","message":"Meldung der API","correlation_id":"c0ffee-1"}`}
			c := newTestClient(t, rec, func(o *Options) { o.Sleep = func(time.Duration) {} })

			resp, err := c.Call(context.Background(), "GetInvoice", []int64{9}, nil, nil)
			ce := asCLIErr(t, err)
			if ce.Code != tt.code || ce.Exit() != tt.exit || ce.HTTPStatus != tt.status {
				t.Errorf("code %s exit %d status %d, want %s %d %d", ce.Code, ce.Exit(), ce.HTTPStatus, tt.code, tt.exit, tt.status)
			}
			if ce.Message != "Meldung der API" || ce.CorrelationID != "c0ffee-1" {
				t.Errorf("message %q correlation %q", ce.Message, ce.CorrelationID)
			}
			if tt.hint == "" && ce.Hint != "" || !strings.Contains(ce.Hint, tt.hint) {
				t.Errorf("hint = %q, want containing %q", ce.Hint, tt.hint)
			}
			// The raw response stays available for callers (headers, body, --debug).
			if resp == nil || resp.Status != tt.status || !strings.Contains(string(resp.Body), "c0ffee-1") {
				t.Errorf("resp = %+v", resp)
			}
		})
	}
}

// A 5xx answer to a mutation does not prove that nothing happened (the
// server may fail after booking or sending): the hint never invites a plain
// retry, unlike for GET.
func TestServerErrorOnMutationHintChecksFirst(t *testing.T) {
	for _, status := range []int{500, 502, 504} {
		for _, call := range []struct {
			op   string
			body any
		}{
			{"DeliverInvoice", map[string]any{"send_via": "pdf"}},
			{"UpdateInvoice", map[string]any{"name": "x"}},
			{"DeleteInvoice", nil},
		} {
			rec := &recorder{status: status, body: `{"message":"kaputt"}`}
			c := newTestClient(t, rec, func(o *Options) { o.Sleep = func(time.Duration) {} })
			_, err := c.Call(context.Background(), call.op, []int64{9}, nil, call.body)
			ce := asCLIErr(t, err)
			if ce.Code != clierr.CodeServer || ce.Exit() != 1 || strings.Contains(ce.Hint, "später erneut versuchen") ||
				!strings.Contains(ce.Hint, "vor Wiederholung per describe/list prüfen") {
				t.Errorf("%s %d: code %s exit %d hint %q", call.op, status, ce.Code, ce.Exit(), ce.Hint)
			}
		}
	}
}

// Review Focus 5: non-JSON error bodies give a short German message by status,
// never a parse error or an HTML dump.
func TestNonJSONErrorBodies(t *testing.T) {
	tests := []struct {
		name   string
		status int
		header map[string]string
		body   string
		code   clierr.Code
		exit   int
		msg    string
	}{
		{"html 502", 502, map[string]string{"Content-Type": "text/html"},
			"<html><head><title>502 Bad Gateway</title></head><body><h1>Bad Gateway</h1></body></html>",
			clierr.CodeServer, 1, "Papierkram-Server nicht erreichbar oder fehlerhaft (HTTP 502)"},
		{"empty 401", 401, nil, "", clierr.CodeAuth, 3, "Nicht angemeldet oder Token ungültig (HTTP 401)"},
		{"json without message", 404, nil, `{"type":"error","error_type":"not_found"}`, clierr.CodeNotFound, 4, "Nicht gefunden (HTTP 404)"},
		{"plain text 422", 422, map[string]string{"Content-Type": "text/plain"}, "nope", clierr.CodeValidation, 5, "Eingaben von der API abgelehnt (HTTP 422)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recorder{status: tt.status, header: tt.header, body: tt.body}
			c := newTestClient(t, rec, func(o *Options) { o.Sleep = func(time.Duration) {} })

			resp, err := c.Call(context.Background(), "GetInvoice", []int64{9}, nil, nil)
			ce := asCLIErr(t, err)
			if ce.Code != tt.code || ce.Exit() != tt.exit || ce.HTTPStatus != tt.status {
				t.Errorf("code %s exit %d status %d", ce.Code, ce.Exit(), ce.HTTPStatus)
			}
			if ce.Message != tt.msg {
				t.Errorf("message = %q, want %q", ce.Message, tt.msg)
			}
			if out := ce.Format(); strings.Contains(out, "<html") || strings.Contains(out, "nope") {
				t.Errorf("raw body leaked: %q", out)
			}
			if resp == nil || string(resp.Body) != tt.body {
				t.Errorf("raw body not kept on Response: %+v", resp)
			}
		})
	}
}

func TestNetworkErrorBeforeSendIsNetwork(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	base := srv.URL
	srv.Close() // connection refused: nothing is ever written

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			var sleeps []time.Duration
			c := New(Options{BaseURL: base + "/api/v1", Token: testToken, Sleep: func(d time.Duration) { sleeps = append(sleeps, d) }})
			_, err := c.CallRaw(context.Background(), method, "/income/invoices", nil, "application/json", strings.NewReader("{}"))
			ce := asCLIErr(t, err)
			if ce.Code != clierr.CodeNetwork || ce.Exit() != 10 {
				t.Errorf("code %s exit %d, want network 10", ce.Code, ce.Exit())
			}
			if !strings.Contains(ce.Hint, "Subdomain und Verbindung prüfen") {
				t.Errorf("hint = %q", ce.Hint)
			}
			if ce.Cause == nil {
				t.Error("cause not kept")
			}
			if strings.Contains(ce.Format(), testToken) {
				t.Error("token in error output")
			}
			wantSleeps := 0
			if method == http.MethodGet {
				wantSleeps = 2
			}
			if len(sleeps) != wantSleeps {
				t.Errorf("sleeps = %v, want %d", sleeps, wantSleeps)
			}
		})
	}
}

func TestTransportReasonDNS(t *testing.T) {
	err := &url.Error{Op: "Get", URL: "https://firma.papierkram.de/api/v1/x", Err: &net.OpError{
		Op: "dial", Net: "tcp", Err: &net.DNSError{Err: "no such host", Name: "firma.papierkram.de", IsNotFound: true},
	}}
	ce := transportError(http.MethodGet, "/api/v1/x", false, err)
	if ce.Code != clierr.CodeNetwork || ce.Message != "Papierkram-API nicht erreichbar (Adresse nicht auflösbar (firma.papierkram.de))" {
		t.Errorf("code %s message %q", ce.Code, ce.Message)
	}
	// L104: without a name no empty "()"
	err = &url.Error{Op: "Get", URL: "https://firma.papierkram.de/api/v1/x", Err: &net.DNSError{Err: "no such host", IsNotFound: true}}
	if ce := transportError(http.MethodGet, "/api/v1/x", false, err); ce.Message != "Papierkram-API nicht erreichbar (Adresse nicht auflösbar)" {
		t.Errorf("ohne Name: message %q", ce.Message)
	}
}

func TestTLSCertificateErrorIsNetwork(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.NotFoundHandler())
	srv.Config.ErrorLog = log.New(io.Discard, "", 0) // silence handshake errors
	srv.StartTLS()
	t.Cleanup(srv.Close)

	// The default client does not trust the test certificate.
	c := New(Options{BaseURL: srv.URL + "/api/v1", Token: testToken, Sleep: func(time.Duration) {}})
	_, err := c.Call(context.Background(), "ListInvoices", nil, nil, nil)
	ce := asCLIErr(t, err)
	if ce.Code != clierr.CodeNetwork || ce.Message != "Papierkram-API nicht erreichbar (TLS-Zertifikat ungültig)" {
		t.Errorf("code %s message %q", ce.Code, ce.Message)
	}
	if strings.Contains(ce.Message, "x509") {
		t.Errorf("raw Go error in message: %q", ce.Message)
	}
}

// After an interrupt only the network hint at the start goes; what a
// command added after it stays, any other hint is unchanged.
func TestWithoutNetworkHint(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{hintNetwork, ""},
		{hintNetwork + ".", ""},
		{hintNetwork + ". Der Timer läuft weiter", "Der Timer läuft weiter"},
		{hintNetwork + "; Wert prüfen", "Wert prüfen"},
		{hintNetwork + "en", hintNetwork + "en"},
		{"Nichts gesendet", "Nichts gesendet"},
		{"Erst " + hintNetwork, "Erst " + hintNetwork},
		{"", ""},
	} {
		if got := WithoutNetworkHint(tc.in); got != tc.want {
			t.Errorf("WithoutNetworkHint(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
