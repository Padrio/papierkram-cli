package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Padrio/papierkram-cli/internal/clierr"
)

// redirectServer answers every request outside /api/v1/target/ with status
// and a Location below /api/v1/target/, and the target itself with 200.
// It records "METHOD path" of every request.
type redirectServer struct {
	mu     sync.Mutex
	seen   []string
	status int
}

func (s *redirectServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.seen = append(s.seen, r.Method+" "+r.URL.Path)
	s.mu.Unlock()
	if strings.HasPrefix(r.URL.Path, "/api/v1/target/") {
		_, _ = fmt.Fprint(w, `{"id":7,"name":"unverändert"}`)
		return
	}
	w.Header().Set("Location", "/api/v1/target/"+strings.TrimPrefix(r.URL.Path, "/api/v1/"))
	w.WriteHeader(s.status)
}

func (s *redirectServer) requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.seen...)
}

func redirectClient(t *testing.T, status int, hc *http.Client) (*Client, *redirectServer) {
	t.Helper()
	rs := &redirectServer{status: status}
	srv := httptest.NewServer(rs)
	t.Cleanup(srv.Close)
	return New(Options{BaseURL: srv.URL + "/api/v1", Token: testToken, HTTP: hc}), rs
}

// A 3xx answer to a mutation is not followed: Go would turn POST, PUT and
// DELETE into a GET and report the GET's answer as the mutation's (I1).
// The server may have acted (Post/Redirect/Get), so it is exit 12.
func TestRedirectOnMutationIsOutcomeUnknown(t *testing.T) {
	calls := map[string]func(*Client) (*Response, error){
		"Call PUT": func(c *Client) (*Response, error) {
			return c.Call(context.Background(), "UpdateProject", []int64{7}, nil, map[string]any{"name": "Neu"})
		},
		"Call DELETE": func(c *Client) (*Response, error) {
			return c.Call(context.Background(), "DeleteProject", []int64{7}, nil, nil)
		},
		"Call POST": func(c *Client) (*Response, error) {
			return c.Call(context.Background(), "CreateProject", nil, nil, map[string]any{"name": "Neu"})
		},
		"CallRaw POST": func(c *Client) (*Response, error) {
			return c.CallRaw(context.Background(), http.MethodPost, "/projects", nil, "application/json", strings.NewReader(`{"name":"Neu"}`))
		},
		"CallRaw PATCH": func(c *Client) (*Response, error) {
			return c.CallRaw(context.Background(), http.MethodPatch, "/projects/7", nil, "application/json", strings.NewReader(`{}`))
		},
		"CallRaw DELETE": func(c *Client) (*Response, error) {
			return c.CallRaw(context.Background(), http.MethodDelete, "/projects/7", nil, "", nil)
		},
		"Upload": func(c *Client) (*Response, error) {
			return c.Upload(context.Background(), "UploadVoucherDocument", []int64{3}, "file", "beleg.pdf", strings.NewReader("%PDF-1.4"), 8)
		},
	}
	for _, status := range []int{http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		for name, call := range calls {
			t.Run(fmt.Sprintf("%d %s", status, name), func(t *testing.T) {
				for _, hc := range []*http.Client{nil, http.DefaultClient} {
					c, rs := redirectClient(t, status, hc)
					resp, err := call(c)
					if err == nil {
						t.Fatalf("no error, resp = %d %s; want outcome_unknown (requests %v)", resp.Status, resp.Body, rs.requests())
					}
					ce := clierr.From(err)
					if ce.Code != clierr.CodeOutcomeUnknown || ce.Exit() != 12 {
						t.Fatalf("err = %v (code %q), resp = %+v; want outcome_unknown", err, ce.Code, resp)
					}
					if ce.HTTPStatus != 0 || ce.Hint != hintOutcomeUnknown {
						t.Errorf("http_status %d hint %q", ce.HTTPStatus, ce.Hint)
					}
					if !strings.Contains(ce.Message, fmt.Sprintf("(HTTP %d)", status)) || !strings.Contains(ce.Message, "Weiterleitung") {
						t.Errorf("message = %q", ce.Message)
					}
					if got := rs.requests(); len(got) != 1 || strings.Contains(got[0], "/target/") {
						t.Errorf("requests = %v, want exactly the one mutation", got)
					}
				}
			})
		}
	}
}

// GET keeps following redirects (a download may be served elsewhere).
func TestRedirectOnGetIsFollowed(t *testing.T) {
	for _, status := range []int{http.StatusFound, http.StatusTemporaryRedirect} {
		c, rs := redirectClient(t, status, nil)
		resp, err := c.Call(context.Background(), "GetProject", []int64{7}, nil, nil)
		if err != nil || resp.Status != http.StatusOK {
			t.Fatalf("%d: resp %+v err %v", status, resp, err)
		}
		if got := rs.requests(); len(got) != 2 || got[1] != "GET /api/v1/target/projects/7" {
			t.Errorf("%d: requests = %v", status, got)
		}
	}
}

// New never changes the caller's client (it may be http.DefaultClient),
// and a CheckRedirect of the caller still decides for GET.
func TestRedirectPolicyLeavesCallerClient(t *testing.T) {
	if http.DefaultClient.CheckRedirect != nil {
		t.Fatal("http.DefaultClient.CheckRedirect set before the test")
	}
	_, _ = redirectClient(t, http.StatusFound, http.DefaultClient)
	if http.DefaultClient.CheckRedirect != nil {
		t.Error("New changed http.DefaultClient.CheckRedirect")
	}

	stop := errors.New("stop")
	called := 0
	hc := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { called++; return stop }}
	rs := &redirectServer{status: http.StatusFound}
	srv := httptest.NewServer(rs)
	t.Cleanup(srv.Close)
	c := New(Options{BaseURL: srv.URL + "/api/v1", Token: testToken, HTTP: hc, Sleep: func(time.Duration) {}})
	if _, err := c.Call(context.Background(), "GetProject", []int64{7}, nil, nil); err == nil || !errors.Is(err, stop) {
		t.Errorf("GET err = %v, want the caller's policy error", err)
	}
	// a failed GET is retried; every attempt asks the caller's policy once
	if called == 0 || called != len(rs.requests()) {
		t.Errorf("caller policy called %d times, requests %v", called, rs.requests())
	}
}

// The default policy for GET stops after 10 redirects, like net/http.
func TestRedirectLoopOnGetStops(t *testing.T) {
	var n int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		http.Redirect(w, r, r.URL.Path, http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	c := New(Options{BaseURL: srv.URL + "/api/v1", Token: testToken, Sleep: func(time.Duration) {}})
	if _, err := c.Call(context.Background(), "GetProject", []int64{7}, nil, nil); err == nil {
		t.Fatal("redirect loop on GET did not fail")
	}
	if n > 3*11 {
		t.Errorf("requests = %d", n)
	}
}
