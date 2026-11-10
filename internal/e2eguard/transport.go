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
	Query        string // the raw query, e.g. company_id=7&page=1
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
// ErrBudget. Every request handed to Base is counted by Meter (answered:
// Record, whatever the status; failed: RecordUnanswered); every answered
// one is recorded with both bodies, so tests can measure what the API
// answered, and counted by Coverage (when set) for the operation it
// reached. Refused and unanswered requests never count for Coverage.
type Transport struct {
	Base         http.RoundTripper
	Scheme, Host string
	Meter        *Meter
	Coverage     *Coverage

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
		// the request may have reached the API and been charged
		t.Meter.RecordUnanswered(req.Method, req.URL.Path)
		return nil, err
	}
	// every answer counts, whatever its status (a rejected create is
	// charged in full)
	t.Meter.Record(req.Method, req.URL.Path, resp.Header)
	t.Coverage.Record(req.Method, req.URL.Path, resp.StatusCode)
	respBody, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, err
	}
	resp.Body = io.NopCloser(bytes.NewReader(respBody))

	hdr := req.Header.Clone()
	hdr.Del("Authorization")
	t.mu.Lock()
	t.exchanges = append(t.exchanges, Exchange{
		Method: req.Method, Path: req.URL.Path, Query: req.URL.RawQuery, Status: resp.StatusCode,
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

// maxDecode caps the repeated decoding of a path segment; a segment that
// still changes after it counts as guarded (fail closed).
const maxDecode = 8

// ambiguous is the action of a request that cannot be told apart from
// pay or deliver; checkCanonical refuses it (no canonical path matches).
const ambiguous = "?"

// guardedAction reports "pay" or "deliver" when any segment of the path
// of u names one of them, and ambiguous when a segment cannot be judged;
// "" otherwise. It is deliberately broad (fail closed); such a request
// must then pass checkCanonical. Every form of the path (decoded,
// escaped, raw, opaque) is split at "/" and "\"; each segment is decoded
// until it no longer changes (at most maxDecode times, else ambiguous)
// and split again. A piece with a control character (NUL included) is
// ambiguous; otherwise it is lower-cased, cut at ";", "." or "," and
// reduced to the letters a-z before the comparison (so "payment_terms"
// stays unguarded).
func guardedAction(u *url.URL) string {
	act := ""
	for _, p := range []string{u.Path, u.EscapedPath(), u.RawPath, u.Opaque} {
		for _, seg := range splitPath(p) {
			switch a := segmentAction(seg); a {
			case ambiguous:
				return ambiguous
			case "pay", "deliver":
				act = a
			}
		}
	}
	return act
}

func splitPath(p string) []string {
	return strings.FieldsFunc(p, func(r rune) bool { return r == '/' || r == '\\' })
}

// segmentAction judges one segment (see guardedAction).
func segmentAction(seg string) string {
	for i := 0; ; i++ {
		dec, err := url.PathUnescape(seg)
		if err != nil || dec == seg {
			break
		}
		if i == maxDecode {
			return ambiguous
		}
		seg = dec
	}
	act := ""
	for _, piece := range splitPath(seg) {
		for _, r := range piece {
			if r < 0x20 || r == 0x7f {
				return ambiguous
			}
		}
		piece = strings.ToLower(piece)
		if i := strings.IndexAny(piece, ";.,"); i >= 0 {
			piece = piece[:i]
		}
		letters := strings.Map(func(r rune) rune {
			if r >= 'a' && r <= 'z' {
				return r
			}
			return -1
		}, piece)
		if letters == "pay" || letters == "deliver" {
			act = letters
		}
	}
	return act
}

// checkCanonical lets a guarded request (act from guardedAction) pass
// only in its canonical form: POST to the exact path of the operation
// (no encoded variant, suffix, parameter, other case or trailing slash),
// no query (not even an empty "?"), exactly one Content-Type with media
// type application/json and at most charset utf-8, no Content-Encoding
// and no method override header (header keys compared in any case).
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
	// header keys in any spelling (a key written directly into the map
	// is not canonicalised, Values would miss it)
	var ct []string
	for k, vs := range req.Header {
		switch lk := strings.ToLower(k); {
		case lk == "content-type":
			ct = append(ct, vs...)
		case lk == "content-encoding":
			return refuse("Content-Encoding")
		case strings.Contains(lk, "method"):
			return refuse("Header " + k + " (Methoden-Override)")
		}
	}
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
	return nil
}

// deliversPDF reports whether body is a strict JSON object (see
// strictObject) with the key send_via, exactly so spelled, set to "pdf",
// and no key named email or _method (a method override) in any spelling.
func deliversPDF(body []byte) bool {
	m, ok := strictObject(body)
	if !ok {
		return false
	}
	for k := range m {
		if strings.EqualFold(k, "email") || strings.EqualFold(k, "_method") {
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
