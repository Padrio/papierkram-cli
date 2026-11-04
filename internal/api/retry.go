package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"sync/atomic"
	"time"

	"github.com/Padrio/papierkram-cli/internal/clierr"
)

// retryBackoff holds the waits before the retries of a GET; its length is
// the number of retries (spec 5.5: twice, exponential backoff).
var retryBackoff = []time.Duration{500 * time.Millisecond, time.Second}

// do sets the standard headers and sends req. It is the single path of
// every request. Only GET is retried, and only after a network error or a
// 5xx: never mutations (credits, double booking), never 429 (monthly quota).
// A non-2xx status returns the Response together with a *clierr.Error.
func (c *Client) do(ctx context.Context, req *http.Request) (*Response, error) {
	req.Header.Set("Authorization", "Bearer "+c.opts.Token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.opts.UserAgent)

	// A GET body (only possible via CallRaw) must be rebuildable per attempt.
	retry := req.Method == http.MethodGet && (req.Body == nil || req.Body == http.NoBody || req.GetBody != nil)
	c.debugContext()
	for attempt := 0; ; attempt++ {
		resp, err := c.send(ctx, req, attempt)
		if !retry || attempt >= len(retryBackoff) || !retryable(err) || ctx.Err() != nil {
			return resp, err
		}
		c.opts.Sleep(retryBackoff[attempt])
		if ctx.Err() != nil {
			return resp, err
		}
	}
}

// retryable reports whether err is a network error or a 5xx.
func retryable(err error) bool {
	var ce *clierr.Error
	return errors.As(err, &ce) && (ce.Code == clierr.CodeNetwork || ce.Code == clierr.CodeServer)
}

// send performs one attempt with the per-request timeout and reads the whole
// body. A trace records whether the request was completely written, which
// decides between network and outcome_unknown on failure.
func (c *Client) send(ctx context.Context, req *http.Request, attempt int) (*Response, error) {
	ctx, cancel := context.WithTimeout(ctx, c.opts.Timeout)
	defer cancel()
	var wrote atomic.Bool // set from the transport's write goroutine
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		WroteRequest: func(i httptrace.WroteRequestInfo) {
			if i.Err == nil {
				wrote.Store(true)
			}
		},
	})
	r := req.WithContext(ctx)
	if attempt > 0 && req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, fmt.Errorf("api: Anfragekörper für Wiederholung nicht lesbar: %w", err)
		}
		r.Body = body
	}

	c.debugRequest(req, attempt)
	start := time.Now()
	hr, err := c.http.Do(r)
	if err != nil {
		c.debugError(err, time.Since(start))
		return nil, transportError(req.Method, req.URL.Path, wrote.Load(), err)
	}
	defer func() { _ = hr.Body.Close() }()
	quota := parseQuota(hr.Header)
	if quota.Known && c.opts.OnQuota != nil {
		c.opts.OnQuota(quota)
	}
	data, err := io.ReadAll(hr.Body)
	c.debugResponse(hr, data, quota, time.Since(start), err)
	resp := &Response{Status: hr.StatusCode, Header: hr.Header, Body: data, Quota: quota}
	if hr.StatusCode < 200 || hr.StatusCode > 299 {
		// The status decides, even if the body was cut off: a truncated
		// 429 must not turn into a retryable network error.
		return resp, statusError(req.Method, hr.StatusCode, data)
	}
	if err != nil {
		// A 2xx body is the payload; the server has seen the request.
		return nil, transportError(req.Method, req.URL.Path, true, err)
	}
	return resp, nil
}
