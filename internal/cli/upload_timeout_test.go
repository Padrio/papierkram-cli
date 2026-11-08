package cli

import (
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// Uploads get a longer timeout per attempt without --timeout (spec 5.5,
// 9.3): max(effective timeout, uploadTimeout); --timeout is taken as it
// is, every other request keeps the effective timeout.

// deadlines records the time left until the deadline of each request (by
// path) as the transport sees it, then sends it to the mock: the deadline
// is checked without waiting for it.
type deadlines struct {
	mu   sync.Mutex
	left map[string]time.Duration
}

func recordDeadlines(t *testing.T, app *App) *deadlines {
	t.Helper()
	d := &deadlines{left: map[string]time.Duration{}}
	app.HTTPClient = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		dl, ok := r.Context().Deadline()
		if !ok {
			t.Errorf("%s %s ohne Deadline", r.Method, r.URL.Path)
		}
		d.mu.Lock()
		d.left[r.Method+" "+strings.TrimPrefix(r.URL.Path, "/api/v1")] = time.Until(dl)
		d.mu.Unlock()
		return http.DefaultTransport.RoundTrip(r)
	})}
	return d
}

// want checks that the request had about want left (at most one second
// less, never more).
func (d *deadlines) want(t *testing.T, req string, want time.Duration) {
	t.Helper()
	d.mu.Lock()
	defer d.mu.Unlock()
	got, ok := d.left[req]
	if !ok {
		t.Errorf("%s nicht gesendet (gesendet: %v)", req, d.left)
		return
	}
	if got > want || got < want-time.Second {
		t.Errorf("%s: Restzeit %s, erwartet %s", req, got.Round(time.Millisecond), want)
	}
}

// timeoutCtxApp is a mock app whose active context has the timeout v; the mock answers it
// (PAPIERKRAM_BASE_URL), the token is in the keyring.
func timeoutCtxApp(t *testing.T, v string) *App {
	t.Helper()
	app, _ := mockApp(t, nil)
	writeCfg(t, app, "active_context = \"a\"\n[contexts.a]\nsubdomain = \"firma-a\"\ntimeout = \""+v+"\"\n")
	if err := app.Keyring.Set("a", "test-token-a"); err != nil {
		t.Fatal(err)
	}
	return app
}

const (
	uploadReq = "POST /expense/vouchers/295/documents"
	createReq = "POST /expense/vouchers"
	createUp  = "POST /expense/vouchers/290/documents"
)

func TestUploadTimeout(t *testing.T) {
	if uploadTimeout != 120*time.Second {
		t.Fatalf("uploadTimeout = %s", uploadTimeout)
	}
	path := writeFile(t, "beleg.pdf", pdfContent)
	envApp := func(t *testing.T) *App { app, _ := apiApp(t); return app }
	cases := []struct {
		name         string
		app          func(t *testing.T) *App
		flags        []string
		upload, rest time.Duration
	}{
		{"Standard", envApp, nil, 120 * time.Second, 30 * time.Second},
		{"--timeout 5s", envApp, []string{"--timeout", "5s"}, 5 * time.Second, 5 * time.Second},
		{"--timeout 300s", envApp, []string{"--timeout", "300s"}, 300 * time.Second, 300 * time.Second},
		{"Kontext 200s", func(t *testing.T) *App { return timeoutCtxApp(t, "200s") }, nil, 200 * time.Second, 200 * time.Second},
		{"Kontext 60s", func(t *testing.T) *App { return timeoutCtxApp(t, "60s") }, nil, 120 * time.Second, 60 * time.Second},
		{"Kontext 60s, --timeout 10s", func(t *testing.T) *App { return timeoutCtxApp(t, "60s") }, []string{"--timeout", "10s"}, 10 * time.Second, 10 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name+"/voucher upload", func(t *testing.T) {
			app := tc.app(t)
			d := recordDeadlines(t, app)
			wantExit(t, app, Run(app, append([]string{"voucher", "upload", "295", path, "-o", "json"}, tc.flags...)), 0)
			d.want(t, uploadReq, tc.upload)
			assertNoToken(t, app)
		})
		t.Run(tc.name+"/voucher create --file", func(t *testing.T) {
			app := tc.app(t)
			d := recordDeadlines(t, app)
			args := append(append(append([]string{}, voucherCreateArgs...), "--file", path, "-o", "json"), tc.flags...)
			wantExit(t, app, Run(app, args), 0)
			d.want(t, createReq, tc.rest) // the create step keeps the effective timeout
			d.want(t, createUp, tc.upload)
			assertNoToken(t, app)
		})
	}
}

// Requests other than uploads keep the effective timeout.
func TestUploadTimeoutOnlyForUploads(t *testing.T) {
	app, _ := apiApp(t)
	d := recordDeadlines(t, app)
	wantExit(t, app, Run(app, []string{"voucher", "describe", "295", "-o", "json"}), 0)
	d.want(t, "GET /expense/vouchers/295", 30*time.Second)
}
