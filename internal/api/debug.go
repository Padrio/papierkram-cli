package api

import (
	"fmt"
	"io"
	"maps"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Padrio/papierkram-cli/internal/redact"
)

// debugBodyLimit is the number of body bytes --debug shows (spec 5.5).
const debugBodyLimit = 4096

// The debug log (Options.Debug) has one line per item, prefixed with
// "* " (context), "> " (request) or "< " (response), so it can be grepped.
// Every attempt of a request is logged. The token never appears: the
// Authorization header is always "Bearer ***" and every occurrence of the
// token value is replaced as well (spec 10.2, R26).

// debugContext writes Options.DebugInfo once per request.
func (c *Client) debugContext() {
	if c.opts.Debug == nil || c.opts.DebugInfo == "" {
		return
	}
	c.debugWrite("* " + c.opts.DebugInfo + "\n")
}

// debugRequest writes request line, headers and body of one attempt. The
// body is read from a GetBody copy only, never from the body being sent.
func (c *Client) debugRequest(req *http.Request, attempt int) {
	if c.opts.Debug == nil {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "> %s %s", req.Method, redact.URLOf(req.URL)) // no userinfo (redact)
	if attempt > 0 {
		fmt.Fprintf(&b, " (Versuch %d)", attempt+1)
	}
	b.WriteByte('\n')
	for _, k := range slices.Sorted(maps.Keys(req.Header)) {
		for _, v := range req.Header[k] {
			if strings.EqualFold(k, "Authorization") {
				v = "Bearer ***"
			}
			fmt.Fprintf(&b, "> %s: %s\n", k, v)
		}
	}
	if req.Body != nil && req.Body != http.NoBody {
		switch {
		case !isTextType(req.Header.Get("Content-Type")):
			fmt.Fprintf(&b, "> <binär, %s>\n", sizeText(req.ContentLength))
		case req.GetBody == nil:
			fmt.Fprintf(&b, "> <nicht protokolliert, %s>\n", sizeText(req.ContentLength))
		default:
			data, err := readCopy(req)
			if err != nil {
				fmt.Fprintf(&b, "> <nicht lesbar: %v>\n", err)
				break
			}
			c.writeBody(&b, "> ", req.Header.Get("Content-Type"), data)
		}
	}
	c.debugWrite(b.String())
}

// debugResponse writes status, duration, quota and body of one attempt;
// readErr is a failure while reading the body, cut reports a body cut at
// MaxResponseSize.
func (c *Client) debugResponse(hr *http.Response, data []byte, q Quota, d time.Duration, readErr error, cut bool) {
	if c.opts.Debug == nil {
		return
	}
	var b strings.Builder
	status := fmt.Sprint(hr.StatusCode)
	if text := http.StatusText(hr.StatusCode); text != "" {
		status += " " + text
	}
	fmt.Fprintf(&b, "< %s (%dms)\n", status, d.Milliseconds())
	for _, k := range debugResponseHeaders {
		if v := hr.Header.Get(k); v != "" {
			fmt.Fprintf(&b, "< %s: %s\n", k, v)
		}
	}
	if q.Known {
		fmt.Fprintf(&b, "< X-Remaining-Quota: %d\n", q.Remaining)
		if q.CostKnown {
			fmt.Fprintf(&b, "< X-Consumed-Quota: %d (Kosten dieser Anfrage)\n", q.RequestCost)
		}
	}
	c.writeBody(&b, "< ", hr.Header.Get("Content-Type"), data)
	if cut {
		fmt.Fprintf(&b, "< gekürzt nach %d MiB\n", MaxResponseSize>>20)
	}
	if readErr != nil {
		fmt.Fprintf(&b, "< Fehler beim Lesen: %v\n", readErr)
	}
	c.debugWrite(b.String())
}

// debugError writes a failed attempt without response.
func (c *Client) debugError(err error, d time.Duration) {
	if c.opts.Debug == nil {
		return
	}
	c.debugWrite(fmt.Sprintf("< Fehler: %s (%dms)\n", redact.Error(err), d.Milliseconds()))
}

// debugResponseHeaders are the response headers --debug shows besides
// status and quota; the rest is left out to keep the log compact.
var debugResponseHeaders = []string{"Content-Type", "Retry-After"}

// writeBody appends data line by line: bodies that are not text (see
// isTextType) only as their size, text cut to debugBodyLimit bytes.
// Invalid UTF-8 (e.g. a Latin-1 proxy error page) is shown with
// replacement characters.
func (c *Client) writeBody(b *strings.Builder, prefix, contentType string, data []byte) {
	if len(data) == 0 {
		return
	}
	if !isTextType(contentType) {
		fmt.Fprintf(b, "%s<binär, %d Bytes>\n", prefix, len(data))
		return
	}
	// Redact before cutting, so a cut cannot leave half a token behind.
	s := c.redact(strings.ToValidUTF8(string(data), "\uFFFD"))
	marker := ""
	if len(s) > debugBodyLimit {
		cut := debugBodyLimit
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		s = s[:cut]
		marker = fmt.Sprintf("… (gekürzt, %d Bytes)", len(data))
	}
	lines := strings.Split(strings.TrimSuffix(s, "\n"), "\n")
	lines[len(lines)-1] += marker
	for _, line := range lines {
		b.WriteString(prefix + line + "\n")
	}
}

// debugWrite writes s with the token removed.
func (c *Client) debugWrite(s string) {
	_, _ = io.WriteString(c.opts.Debug, c.redact(s))
}

// redact replaces every occurrence of the token, also URL-encoded.
func (c *Client) redact(s string) string {
	t := c.opts.Token
	if t == "" {
		return s
	}
	s = strings.ReplaceAll(s, t, "***")
	if e := url.QueryEscape(t); e != t {
		s = strings.ReplaceAll(s, e, "***")
	}
	return s
}

// readCopy reads the request body from a fresh GetBody reader; the body
// that is sent stays untouched.
func readCopy(req *http.Request) ([]byte, error) {
	rc, err := req.GetBody()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	return io.ReadAll(rc)
}

// isTextType reports whether a body of this Content-Type is logged as text:
// JSON (application/json, *+json) and text/* (e.g. an HTML proxy page).
// Every other body, also one without or with an unreadable Content-Type
// (PDF, uploads, images, octet streams), is logged only by its size, so
// binary bytes never reach the terminal (spec 5.5).
func isTextType(contentType string) bool {
	mt, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mt == "application/json" || strings.HasSuffix(mt, "+json") || strings.HasPrefix(mt, "text/")
}

func sizeText(n int64) string {
	if n <= 0 {
		return "Größe unbekannt"
	}
	return fmt.Sprintf("%d Bytes", n)
}
