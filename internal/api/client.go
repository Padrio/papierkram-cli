// Package api is the HTTP client of the Papierkram API: it builds requests
// with the generated builders, sends them with the standard headers and
// returns the raw response bytes plus the quota state. Failures become
// *clierr.Error values; only GET is retried (retry.go, errors.go). With
// Options.Debug set, every attempt is logged with the token redacted (debug.go).
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/Padrio/papierkram-cli/internal/api/gen"
	"github.com/Padrio/papierkram-cli/internal/buildinfo"
	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/redact"
)

// DefaultTimeout applies when Options.Timeout is zero.
const DefaultTimeout = 30 * time.Second

// Options configure a Client.
type Options struct {
	// BaseURL is the API root, e.g. https://firma.papierkram.de/api/v1
	// (a trailing slash is optional).
	BaseURL, Token string
	// UserAgent defaults to buildinfo.UserAgent().
	UserAgent string
	// Timeout is the deadline per attempt; zero means DefaultTimeout.
	Timeout time.Duration
	// HTTP is the client used to send requests; nil means a new http.Client.
	HTTP *http.Client
	// Debug receives the redacted request log (--debug); nil disables it.
	Debug io.Writer
	// DebugInfo is written once per request to Debug as a context line,
	// e.g. the active read-only level ("Schreibschutz: env").
	DebugInfo string
	// Sleep waits between GET retries; nil means time.Sleep.
	Sleep func(time.Duration)
	// OnQuota is called after every response that reports X-Remaining-Quota.
	OnQuota func(Quota)
}

// Response is a raw API response. Body holds the complete body bytes and is
// empty for responses without content; it is never parsed here. For a
// non-2xx status it is returned together with a *clierr.Error.
type Response struct {
	Status int
	Header http.Header
	Body   []byte
	Quota  Quota
}

// Client sends requests to the Papierkram API.
type Client struct {
	opts Options
	base *url.URL // BaseURL, always ending with "/"
	http *http.Client
}

// New creates a Client. BaseURL gets a trailing slash if missing, because
// the generated request builders resolve operation paths relative to it.
func New(o Options) *Client {
	if !strings.HasSuffix(o.BaseURL, "/") {
		o.BaseURL += "/"
	}
	if o.UserAgent == "" {
		o.UserAgent = buildinfo.UserAgent()
	}
	if o.Timeout == 0 {
		o.Timeout = DefaultTimeout
	}
	if o.Sleep == nil {
		o.Sleep = time.Sleep
	}
	hc := o.HTTP
	if hc == nil {
		hc = &http.Client{}
	}
	base, _ := url.Parse(o.BaseURL) // nil on error, reported by CallRaw
	return &Client{opts: o, base: base, http: hc}
}

// Call sends operation opID. A non-nil body is sent as JSON; a nil body
// sends no body and no Content-Type.
func (c *Client) Call(ctx context.Context, opID string, pathParams []int64, query url.Values, body any) (*Response, error) {
	if body == nil {
		return c.callOp(ctx, opID, pathParams, query, "", nil)
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("api: Anfragekörper für %s nicht als JSON darstellbar: %w", opID, err)
	}
	return c.callOp(ctx, opID, pathParams, query, "application/json", bytes.NewReader(data))
}

// Download sends the GET operation opID whose success answer is a file of
// media type accept (application/pdf) and returns its bytes unchanged in
// Body. The Accept header names accept first and application/json, the
// error format of the API, as fallback; errors are mapped like for Call.
func (c *Client) Download(ctx context.Context, opID string, pathParams []int64, accept string) (*Response, error) {
	req, err := c.buildOp(opID, pathParams, nil, "", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept+", application/json;q=0.9")
	return c.do(ctx, req)
}

// CallRaw sends method to path, which is relative to the base URL
// (e.g. "/income/invoices?page=2"); query is appended to any query in path.
// Content-Type is only set when body is non-nil.
func (c *Client) CallRaw(ctx context.Context, method, path string, query url.Values, contentType string, body io.Reader) (*Response, error) {
	if c.base == nil {
		return nil, errBaseURL(c.opts.BaseURL)
	}
	// The "./" prefix keeps the request on the base host even for paths
	// like "//other/x", so the token never leaves the API.
	u, err := c.base.Parse("./" + strings.TrimPrefix(path, "/"))
	if err != nil {
		return nil, fmt.Errorf("api: ungültiger Pfad %q: %w", path, err)
	}
	if len(query) > 0 {
		if u.RawQuery != "" {
			u.RawQuery += "&"
		}
		u.RawQuery += query.Encode()
	}
	req, err := http.NewRequest(method, u.String(), body)
	if err != nil {
		// a parse error would repeat u with its userinfo (redact)
		return nil, fmt.Errorf("api: Anfrage %s %s kann nicht gebaut werden: %w", method, path, redact.Wrap(err))
	}
	if body != nil && contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return c.do(ctx, req)
}

// Upload sends size bytes of the file r as multipart/form-data with one
// part named field, carrying filename (quoted like mime/multipart does) and
// its media type (MediaType). The body is streamed: only the part header
// and the closing boundary are held in memory, Content-Length is set, and
// exactly size bytes of r are sent. When r fails or ends before size
// bytes, the request was never completely sent: that is an error of the
// file (exit 1), not of the network. Otherwise the request goes the way of
// Call (headers, --debug with the body only as its size, no retry for POST,
// error mapping).
func (c *Client) Upload(ctx context.Context, opID string, pathParams []int64, field, filename string, r io.Reader, size int64) (*Response, error) {
	if size < 0 {
		return nil, fmt.Errorf("api: Größe der Datei %s unbekannt", filename)
	}
	var head, tail bytes.Buffer
	to := &switchWriter{w: &head}
	mw := multipart.NewWriter(to)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", multipart.FileContentDisposition(field, filename))
	h.Set("Content-Type", MediaType(filename))
	if _, err := mw.CreatePart(h); err != nil {
		return nil, err
	}
	to.w = &tail
	if err := mw.Close(); err != nil { // writes the closing boundary only
		return nil, err
	}
	length := int64(head.Len()) + size + int64(tail.Len())
	file := &fileReader{r: io.LimitReader(r, size)}
	req, err := c.buildOp(opID, pathParams, nil, mw.FormDataContentType(), io.MultiReader(&head, file, &tail))
	if err != nil {
		return nil, err
	}
	req.ContentLength = length
	resp, err := c.do(ctx, req)
	if err != nil && clierr.From(err).HTTPStatus == 0 {
		if ferr := file.failure(filename, size); ferr != nil {
			return nil, ferr
		}
	}
	return resp, err
}

// MediaType is the media type of a file by the extension of name
// (mime.TypeByExtension), application/octet-stream when it is unknown.
func MediaType(name string) string {
	if ct := mime.TypeByExtension(filepath.Ext(name)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

// switchWriter writes to w, which can be swapped between writes.
type switchWriter struct{ w io.Writer }

func (s *switchWriter) Write(p []byte) (int, error) { return s.w.Write(p) }

// fileReader counts the bytes read from the file of an upload and keeps
// a read error or an early end.
type fileReader struct {
	r     io.Reader
	n     int64
	err   error // read error other than io.EOF
	ended bool  // io.EOF seen
}

func (f *fileReader) Read(p []byte) (int, error) {
	n, err := f.r.Read(p)
	f.n += int64(n)
	switch {
	case err == io.EOF:
		f.ended = true
	case err != nil:
		f.err = err
	}
	return n, err
}

// failure is the error of a file that could not be sent completely, nil
// when the file is not the cause.
func (f *fileReader) failure(filename string, size int64) error {
	switch {
	case f.err != nil:
		return &clierr.Error{Code: clierr.CodeError, Message: fmt.Sprintf("Datei %s nicht lesbar: %v", filename, f.err),
			Hint: "Nichts hochgeladen; Datei prüfen und erneut hochladen", Cause: f.err}
	case f.ended && f.n < size:
		return clierr.New(clierr.CodeError, fmt.Sprintf("Datei %s hat sich beim Hochladen verändert (%d statt %d Bytes)", filename, f.n, size),
			"Nichts hochgeladen; erneut hochladen, wenn die Datei fertig geschrieben ist")
	}
	return nil
}

// Preview returns the method and the full URL (query included) that Call
// would send for opID, built with the same generated builder and base URL
// as callOp, without sending anything (--dry-run).
func (c *Client) Preview(opID string, pathParams []int64, query url.Values) (method, rawURL string, err error) {
	req, err := c.buildOp(opID, pathParams, query, "", nil)
	if err != nil {
		return "", "", err
	}
	return req.Method, redact.URLOf(req.URL), nil
}

// TemplateURL is the URL of path, a path template of the spec, resolved
// against the base URL like the generated builders behind callOp and
// Preview do (url.Parse, then "./"+path relative to it), with its
// parameters left as they are ({voucher_id}). It is for display only: a dry
// run shows a request whose ID is not known yet.
func (c *Client) TemplateURL(path string) string {
	rel := "./" + strings.TrimPrefix(path, "/")
	base, err := url.Parse(c.opts.BaseURL)
	if err != nil {
		return redact.Invalid + rel // Preview reports the broken base URL
	}
	u, err := base.Parse(rel)
	if err != nil {
		return redact.URLOf(base) + rel
	}
	// the braces of the parameters are escaped in the path; shown as such
	return strings.NewReplacer("%7B", "{", "%7D", "}").Replace(redact.URLOf(u))
}

// errBaseURL reports a base URL that cannot be parsed, without showing it:
// it may carry credentials (redact).
func errBaseURL(raw string) error {
	return fmt.Errorf("api: ungültige Basis-URL %s", redact.URL(raw))
}

// buildOp builds the request of opID with the generated builder against the
// base URL; callOp and Preview share it, so a dry run shows exactly what
// would be sent. body must be an untyped nil when there is no body; then no
// Content-Type is set.
func (c *Client) buildOp(opID string, pathParams []int64, query url.Values, contentType string, body io.Reader) (*http.Request, error) {
	if c.base == nil {
		return nil, errBaseURL(c.opts.BaseURL)
	}
	req, err := gen.BuildRequest(opID, c.opts.BaseURL, pathParams, query, contentType, body)
	if err != nil {
		// the text of a parse error repeats the URL with its userinfo
		return nil, fmt.Errorf("api: Anfrage für %s kann nicht gebaut werden: %w", opID, redact.Wrap(err))
	}
	if body == nil {
		// Builders of body operations always add Content-Type, even empty.
		req.Header.Del("Content-Type")
	}
	return req, nil
}

// callOp builds opID with the generated builder and sends it. body must be
// an untyped nil when there is no body.
func (c *Client) callOp(ctx context.Context, opID string, pathParams []int64, query url.Values, contentType string, body io.Reader) (*Response, error) {
	req, err := c.buildOp(opID, pathParams, query, contentType, body)
	if err != nil {
		return nil, err
	}
	return c.do(ctx, req)
}
