package gen

import (
	"errors"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/spec"
)

const testServer = "http://x/api/v1/"

var placeholder = regexp.MustCompile(`\{[^}]+\}`)

// idsFor returns n distinct, non-zero path parameter values.
func idsFor(n int) []int64 {
	ids := make([]int64, n)
	for i := range ids {
		ids[i] = int64(100 + i*11)
	}
	return ids
}

func substitute(template string, ids []int64) string {
	i := 0
	return placeholder.ReplaceAllStringFunc(template, func(string) string {
		s := strconv.FormatInt(ids[i], 10)
		i++
		return s
	})
}

func hasBody(op spec.Operation) bool {
	return op.Op.RequestBody != nil
}

func TestBuildRequestAllOperations(t *testing.T) {
	ops := spec.Operations()
	if len(ops) == 0 {
		t.Fatal("no operations in spec")
	}
	for _, op := range ops {
		t.Run(op.ID, func(t *testing.T) {
			n := len(placeholder.FindAllString(op.Path, -1))
			ids := idsFor(n)

			req, err := BuildRequest(op.ID, testServer, ids, nil, "", nil)
			if err != nil {
				t.Fatalf("BuildRequest: %v", err)
			}
			if req.Method != op.Method {
				t.Errorf("method = %s, want %s", req.Method, op.Method)
			}
			if want := "/api/v1" + substitute(op.Path, ids); req.URL.Path != want {
				t.Errorf("path = %s, want %s", req.URL.Path, want)
			}
			if req.URL.RawQuery != "" {
				t.Errorf("query = %q, want empty", req.URL.RawQuery)
			}

			// ID 0 must stay in the path.
			zeros := make([]int64, n)
			zreq, err := BuildRequest(op.ID, testServer, zeros, nil, "", nil)
			if err != nil {
				t.Fatalf("BuildRequest zero IDs: %v", err)
			}
			if want := "/api/v1" + substitute(op.Path, zeros); zreq.URL.Path != want {
				t.Errorf("zero-ID path = %s, want %s", zreq.URL.Path, want)
			}
			if n > 0 && !strings.Contains(zreq.URL.Path, "/0") {
				t.Errorf("zero-ID path %s lacks /0", zreq.URL.Path)
			}

			q := url.Values{"page": {"2"}, "filter[name]": {"a b&c"}}
			qreq, err := BuildRequest(op.ID, testServer, ids, q, "", nil)
			if err != nil {
				t.Fatalf("BuildRequest with query: %v", err)
			}
			if qreq.URL.RawQuery != q.Encode() {
				t.Errorf("query = %q, want %q", qreq.URL.RawQuery, q.Encode())
			}

			if hasBody(op) {
				breq, err := BuildRequest(op.ID, testServer, ids, nil, "application/json", strings.NewReader(`{"a":1}`))
				if err != nil {
					t.Fatalf("BuildRequest with body: %v", err)
				}
				if got := breq.Header.Get("Content-Type"); got != "application/json" {
					t.Errorf("Content-Type = %q", got)
				}
				data, err := io.ReadAll(breq.Body)
				if err != nil || string(data) != `{"a":1}` {
					t.Errorf("body = %q, %v", data, err)
				}
			} else if _, err := BuildRequest(op.ID, testServer, ids, nil, "application/json", strings.NewReader("x")); !errors.Is(err, ErrUnexpectedBody) {
				t.Errorf("body on bodyless operation: err = %v, want ErrUnexpectedBody", err)
			}

			if n > 0 {
				if _, err := BuildRequest(op.ID, testServer, ids[:n-1], nil, "", nil); !errors.Is(err, ErrPathParams) {
					t.Errorf("too few IDs: err = %v, want ErrPathParams", err)
				}
			}
			if _, err := BuildRequest(op.ID, testServer, append(ids[:n:n], 1), nil, "", nil); !errors.Is(err, ErrPathParams) {
				t.Errorf("too many IDs: err = %v, want ErrPathParams", err)
			}
		})
	}
}

func TestBuildRequestUnknownOperation(t *testing.T) {
	_, err := BuildRequest("NoSuchOperation", testServer, nil, nil, "", nil)
	if !errors.Is(err, ErrUnknownOperation) {
		t.Fatalf("err = %v, want ErrUnknownOperation", err)
	}
}

func TestBuildRequestKnownShapes(t *testing.T) {
	req, err := BuildRequest("DeliverInvoice", testServer, []int64{12}, nil, "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	if req.Method != "POST" || req.URL.String() != "http://x/api/v1/income/invoices/12/deliver" {
		t.Errorf("got %s %s", req.Method, req.URL)
	}
}
