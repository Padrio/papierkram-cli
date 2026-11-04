package e2eguard

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

// ErrRefused is returned by Transport for a request it does not send.
var ErrRefused = errors.New("e2e: Anfrage abgelehnt")

// Exchange is one request with its answer, as seen by Transport. ReqHeader
// never holds the Authorization header.
type Exchange struct {
	Method, Path string // Path is the URL path, e.g. /api/v1/info
	Status       int
	ReqHeader    http.Header
	ReqBody      []byte
	RespHeader   http.Header
	RespBody     []byte
}

// Transport is the HTTP transport of the CLI in the live suite. Before a
// request is sent it checks, in this order:
//   - scheme and host are exactly Scheme and Host (the sandbox), so
//     nothing else is ever contacted;
//   - a deliver request carries a JSON body with send_via "pdf" (the
//     suite never sends mail);
//   - a pay request carries a JSON body with value null or without value
//     (the suite never pays);
//   - the documented cost still fits the budget of Meter.
//
// A refused request is not sent; RoundTrip returns ErrRefused or
// ErrBudget. Every answered request is counted by Meter and recorded with
// both bodies, so tests can measure what the API answered.
type Transport struct {
	Base         http.RoundTripper
	Scheme, Host string
	Meter        *Meter

	mu        sync.Mutex
	exchanges []Exchange
}

// RoundTrip implements http.RoundTripper.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != t.Scheme || req.URL.Host != t.Host {
		closeBody(req)
		return nil, fmt.Errorf("%w: Ziel %s://%s ist nicht die Sandbox", ErrRefused, req.URL.Scheme, req.URL.Host)
	}
	var body []byte
	if req.Body != nil {
		var err error
		body, err = io.ReadAll(req.Body)
		closeBody(req)
		if err != nil {
			return nil, err
		}
	}
	if isAction(req.URL.Path, "deliver") && !deliversPDF(body) {
		return nil, fmt.Errorf("%w: deliver nur mit send_via \"pdf\"", ErrRefused)
	}
	if isAction(req.URL.Path, "pay") && !paysNothing(body) {
		return nil, fmt.Errorf("%w: pay nur mit value null (die Suite bezahlt nie)", ErrRefused)
	}
	if err := t.Meter.Reserve(req.Method, req.URL.Path); err != nil {
		return nil, err
	}
	out := req.Clone(req.Context())
	if req.Body != nil {
		out.Body = io.NopCloser(bytes.NewReader(body))
		out.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	}
	resp, err := t.Base.RoundTrip(out)
	if err != nil {
		return nil, err
	}
	respBody, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, err
	}
	resp.Body = io.NopCloser(bytes.NewReader(respBody))
	t.Meter.Record(req.Method, req.URL.Path, resp.Header)

	hdr := req.Header.Clone()
	hdr.Del("Authorization")
	t.mu.Lock()
	t.exchanges = append(t.exchanges, Exchange{
		Method: req.Method, Path: req.URL.Path, Status: resp.StatusCode,
		ReqHeader: hdr, ReqBody: body,
		RespHeader: resp.Header.Clone(), RespBody: respBody,
	})
	t.mu.Unlock()
	return resp, nil
}

// Exchanges returns all recorded exchanges.
func (t *Transport) Exchanges() []Exchange { return t.Since(0) }

// Len is the number of recorded exchanges.
func (t *Transport) Len() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.exchanges)
}

// Since returns the exchanges recorded after the first n.
func (t *Transport) Since(n int) []Exchange {
	t.mu.Lock()
	defer t.mu.Unlock()
	if n >= len(t.exchanges) {
		return nil
	}
	return append([]Exchange(nil), t.exchanges[n:]...)
}

func closeBody(req *http.Request) {
	if req.Body != nil {
		_ = req.Body.Close()
	}
}

// isAction reports whether path ends with the action segment /action.
func isAction(path, action string) bool {
	return strings.HasSuffix(strings.TrimRight(path, "/"), "/"+action)
}

// paysNothing reports whether body is a JSON object whose value is null
// or absent: the API gets no amount to book.
func paysNothing(body []byte) bool {
	var b map[string]json.RawMessage
	if err := json.Unmarshal(body, &b); err != nil {
		return false
	}
	v, ok := b["value"]
	return !ok || string(v) == "null"
}

// deliversPDF reports whether body is a JSON object with send_via "pdf".
func deliversPDF(body []byte) bool {
	var b struct {
		SendVia *string `json:"send_via"`
	}
	if err := json.Unmarshal(body, &b); err != nil || b.SendVia == nil {
		return false
	}
	return *b.SendVia == "pdf"
}
