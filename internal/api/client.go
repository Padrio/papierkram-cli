// Package api is the HTTP client of the Papierkram API: it builds requests
// with the generated builders, sends them with the standard headers and
// returns the raw response bytes plus the quota state.
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
	// Timeout is the deadline per request; zero means DefaultTimeout.
	Timeout time.Duration
	// HTTP is the client used to send requests; nil means a new http.Client.
	HTTP *http.Client
	// Debug receives the request log (--debug); used from Task 13 on.
	Debug io.Writer
	// Sleep waits between retries; used from Task 12 on.
	Sleep func(time.Duration)
	// OnQuota is called after every response that reports X-Remaining-Quota.
	OnQuota func(Quota)
}

// Response is a raw API response. Body holds the complete body bytes and is
// empty for responses without content; it is never parsed here.
type Response struct {
	Status int
	Header http.Header
	Body   []byte
	Quota  Quota
}

// HTTPError is returned together with the Response for every non-2xx status.
// It is a minimal placeholder; Task 12 maps statuses to clierr codes.
type HTTPError struct {
	Status int
	Body   []byte
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("api: Server antwortete mit HTTP-Status %d", e.Status)
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

// CallRaw sends method to path, which is relative to the base URL
// (e.g. "/income/invoices?page=2"); query is appended to any query in path.
// Content-Type is only set when body is non-nil.
func (c *Client) CallRaw(ctx context.Context, method, path string, query url.Values, contentType string, body io.Reader) (*Response, error) {
	if c.base == nil {
		return nil, fmt.Errorf("api: ungültige Basis-URL %q", c.opts.BaseURL)
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
		return nil, fmt.Errorf("api: Anfrage %s %s kann nicht gebaut werden: %w", method, path, err)
	}
	if body != nil && contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return c.do(ctx, req)
}

// Upload sends r as multipart/form-data with one file part named field.
// The part's Content-Type is derived from the extension of filename.
func (c *Client) Upload(ctx context.Context, opID string, pathParams []int64, field, filename string, r io.Reader) (*Response, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	ct := mime.TypeByExtension(filepath.Ext(filename))
	if ct == "" {
		ct = "application/octet-stream"
	}
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`,
		quoteEscaper.Replace(field), quoteEscaper.Replace(filename)))
	h.Set("Content-Type", ct)
	part, err := mw.CreatePart(h)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, r); err != nil {
		return nil, fmt.Errorf("api: Datei %s nicht lesbar: %w", filename, err)
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	return c.callOp(ctx, opID, pathParams, nil, mw.FormDataContentType(), &buf)
}

// quoteEscaper escapes multipart header parameters like mime/multipart does.
var quoteEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`)

// callOp builds opID with the generated builder and sends it. body must be
// an untyped nil when there is no body.
func (c *Client) callOp(ctx context.Context, opID string, pathParams []int64, query url.Values, contentType string, body io.Reader) (*Response, error) {
	req, err := gen.BuildRequest(opID, c.opts.BaseURL, pathParams, query, contentType, body)
	if err != nil {
		return nil, fmt.Errorf("api: Anfrage für %s kann nicht gebaut werden: %w", opID, err)
	}
	if body == nil {
		// Builders of body operations always add Content-Type, even empty.
		req.Header.Del("Content-Type")
	}
	return c.do(ctx, req)
}

// do sets the standard headers, sends req with the per-request timeout and
// reads the whole body.
func (c *Client) do(ctx context.Context, req *http.Request) (*Response, error) {
	ctx, cancel := context.WithTimeout(ctx, c.opts.Timeout)
	defer cancel()
	req = req.WithContext(ctx)
	req.Header.Set("Authorization", "Bearer "+c.opts.Token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.opts.UserAgent)

	hr, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = hr.Body.Close() }()
	data, err := io.ReadAll(hr.Body)
	if err != nil {
		return nil, err
	}
	resp := &Response{Status: hr.StatusCode, Header: hr.Header, Body: data, Quota: parseQuota(hr.Header)}
	if resp.Quota.Known && c.opts.OnQuota != nil {
		c.opts.OnQuota(resp.Quota)
	}
	if hr.StatusCode < 200 || hr.StatusCode > 299 {
		return resp, &HTTPError{Status: hr.StatusCode, Body: data}
	}
	return resp, nil
}
