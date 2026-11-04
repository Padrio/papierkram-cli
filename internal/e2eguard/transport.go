package e2eguard

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// NoTransactionID is a banking transaction ID that does not exist. The
// only pay request the transport lets through names it (with value null):
// the live suite measures the shape of a pay body, it never pays.
const NoTransactionID = 999999999

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
//   - a request whose path names pay or deliver in any form is only sent
//     in the canonical form of the operation (checkCanonical: POST, exact
//     path, no query, application/json, no Content-Encoding);
//   - a deliver request carries a strict JSON body with send_via "pdf"
//     and no email (the suite never sends mail);
//   - a pay request carries exactly {"value": null, "banking_transaction":
//     {"id": NoTransactionID}} (the suite never pays);
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

// DirectTransport is the base transport for the live suite: a copy of
// http.DefaultTransport without proxy. HTTPS_PROXY and the like of the
// shell are ignored, so no proxy ever sees a request with the token.
func DirectTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil
	return t
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
	if act := guardedAction(req.URL); act != "" {
		if err := checkCanonical(req, act); err != nil {
			return nil, err
		}
		if act == "deliver" && !deliversPDF(body) {
			return nil, fmt.Errorf("%w: deliver nur mit send_via \"pdf\"", ErrRefused)
		}
		if act == "pay" && !paysNothing(body) {
			return nil, fmt.Errorf("%w: pay nur mit value null und der Bankumsatz-ID %d, die es nicht gibt (die Suite bezahlt nie)", ErrRefused, NoTransactionID)
		}
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

// guardedPath is the one canonical path of each guarded operation:
// voucher pay, invoice and estimate deliver.
var guardedPath = regexp.MustCompile(`^/api/v1/(?:expense/vouchers/[0-9]+/(pay)|income/(?:invoices|estimates)/[0-9]+/(deliver))$`)

// guardedAction reports "pay" or "deliver" when any segment of the path
// of u, decoded or not, in any case, with a ";…" parameter or a ".…"
// suffix cut off and blanks trimmed, names one of them; "" otherwise.
// It is deliberately broad (fail closed): such a request must then pass
// checkCanonical.
func guardedAction(u *url.URL) string {
	for _, p := range []string{u.Path, u.EscapedPath(), u.RawPath, u.Opaque} {
		for _, seg := range strings.Split(p, "/") {
			if dec, err := url.PathUnescape(seg); err == nil {
				seg = dec
			}
			seg = strings.ToLower(seg)
			if i := strings.IndexAny(seg, ";.,"); i >= 0 {
				seg = seg[:i]
			}
			switch seg = strings.TrimSpace(seg); seg {
			case "pay", "deliver":
				return seg
			}
		}
	}
	return ""
}

// checkCanonical lets a guarded request (act from guardedAction) pass
// only in its canonical form: POST to the exact path of the operation
// (no encoded variant, suffix, parameter, other case or trailing slash),
// no query (not even an empty "?"), exactly one Content-Type with media
// type application/json and at most charset utf-8, no Content-Encoding.
// Rails merges query parameters into the body parameters, honours format
// suffixes and does not parse JSON sent as another type (review round 2).
func checkCanonical(req *http.Request, act string) error {
	refuse := func(why string) error {
		return fmt.Errorf("%w: %s nur als POST auf den genauen Pfad mit JSON-Body: %s", ErrRefused, act, why)
	}
	u := req.URL
	if req.Method != http.MethodPost {
		return refuse("Methode " + req.Method)
	}
	m := guardedPath.FindStringSubmatch(u.Path)
	if m == nil || u.RawPath != "" || u.Opaque != "" || m[1]+m[2] != act {
		return refuse("Pfad weicht ab")
	}
	if u.RawQuery != "" || u.ForceQuery {
		return refuse("Query-Parameter")
	}
	ct := req.Header.Values("Content-Type")
	if len(ct) != 1 {
		return refuse("Content-Type fehlt oder mehrfach")
	}
	mt, params, err := mime.ParseMediaType(ct[0])
	if err != nil || mt != "application/json" {
		return refuse("Content-Type ist nicht application/json")
	}
	for k, v := range params {
		if k != "charset" || !strings.EqualFold(v, "utf-8") {
			return refuse("Content-Type-Parameter " + k)
		}
	}
	if len(req.Header.Values("Content-Encoding")) > 0 {
		return refuse("Content-Encoding")
	}
	return nil
}

// deliversPDF reports whether body is a strict JSON object (see
// strictObject) with the key send_via, exactly so spelled, set to "pdf",
// and no key named email in any spelling.
func deliversPDF(body []byte) bool {
	m, ok := strictObject(body)
	if !ok {
		return false
	}
	for k := range m {
		if strings.EqualFold(k, "email") {
			return false
		}
	}
	var via string
	raw, ok := m["send_via"]
	return ok && json.Unmarshal(raw, &via) == nil && via == "pdf"
}

// paysNothing reports whether body is exactly
// {"value": null, "banking_transaction": {"id": NoTransactionID}}: value
// present and null, the sentinel ID as an integer, no other key at any
// level, keys unique (see strictObject).
func paysNothing(body []byte) bool {
	m, ok := strictObject(body)
	if !ok || len(m) != 2 || string(bytes.TrimSpace(m["value"])) != "null" {
		return false
	}
	bt, ok := strictObject(m["banking_transaction"])
	return ok && len(bt) == 1 &&
		string(bytes.TrimSpace(bt["id"])) == strconv.Itoa(NoTransactionID)
}

// strictObject parses body as exactly one JSON object and returns its
// members. It refuses (false) anything else: invalid JSON, another type,
// trailing data, and keys of one object that are equal or differ only by
// case, at any depth. The check walks the tokens (json.Decoder.Token):
// a map decode would silently keep one of the duplicates, and
// encoding/json matches struct fields ignoring case.
func strictObject(body []byte) (map[string]json.RawMessage, bool) {
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	if tok, err := d.Token(); err != nil || tok != json.Delim('{') {
		return nil, false
	}
	if checkObject(d) != nil {
		return nil, false
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, false
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil || m == nil {
		return nil, false
	}
	return m, true
}

var errDuplicateKey = errors.New("e2e: doppelter JSON-Schlüssel")

// checkObject reads the members of an object whose "{" was read, up to
// and including its "}".
func checkObject(d *json.Decoder) error {
	var keys []string
	for d.More() {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		key, ok := tok.(string)
		if !ok {
			return errDuplicateKey
		}
		for _, k := range keys {
			if strings.EqualFold(k, key) {
				return errDuplicateKey
			}
		}
		keys = append(keys, key)
		if err := checkValue(d); err != nil {
			return err
		}
	}
	_, err := d.Token() // "}"
	return err
}

// checkValue reads one value; objects and arrays are checked to the end.
func checkValue(d *json.Decoder) error {
	tok, err := d.Token()
	if err != nil {
		return err
	}
	switch tok {
	case json.Delim('{'):
		return checkObject(d)
	case json.Delim('['):
		for d.More() {
			if err := checkValue(d); err != nil {
				return err
			}
		}
		_, err := d.Token() // "]"
		return err
	}
	return nil
}
