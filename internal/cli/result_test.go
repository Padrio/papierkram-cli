package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/spf13/cobra"

	"github.com/Padrio/papierkram-cli/internal/clierr"
	"github.com/Padrio/papierkram-cli/internal/config"
)

// helperCmd is a bare command with the context and the --input flag the
// helpers of result.go read.
func helperCmd() *cobra.Command {
	c := &cobra.Command{Use: "x"}
	c.Flags().String("input", "", "")
	c.SetContext(context.Background())
	return c
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("stdout zu") }

// emptyOK prints the German line in table and wide without --fields, the
// result in every other format, the format of the context when -o is not
// given, and maps a failed write to exit 1 with outputFailed.
func TestEmptyOK(t *testing.T) {
	result := okJSON{ID: 7, OK: true}
	cases := []struct {
		name   string
		output string
		set    bool
		fields string
		r      *config.Resolved
		want   string
	}{
		{name: "table", want: "Beleg 7: Zahlung erfasst.\n"},
		{name: "wide", output: "wide", set: true, want: "Beleg 7: Zahlung erfasst.\n"},
		{name: "json", output: "json", set: true, want: "{\n  \"id\": 7,\n  \"ok\": true\n}\n"},
		{name: "context yaml", r: &config.Resolved{Output: "yaml"}, want: "id: 7\nok: true\n"},
		{name: "-o wins over context", output: "table", set: true, r: &config.Resolved{Output: "json"}, want: "Beleg 7: Zahlung erfasst.\n"},
		{name: "fields", fields: "id", want: "FELD  WERT\nID    7\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app, out, errb := newTestApp(t)
			app.Flags.Output, app.Flags.OutputSet, app.Flags.Fields = tc.output, tc.set, tc.fields
			if err := app.emptyOK(helperCmd(), tc.r, result, "Beleg 7: Zahlung erfasst."); err != nil {
				t.Fatal(err)
			}
			if got := out.String(); got != tc.want {
				t.Errorf("stdout = %q, want %q", got, tc.want)
			}
			if errb.Len() > 0 {
				t.Errorf("stderr = %q", errb)
			}
		})
	}
}

func TestEmptyOKWriteError(t *testing.T) {
	for _, output := range []string{"table", "json"} {
		t.Run(output, func(t *testing.T) {
			app, _, _ := newTestApp(t)
			app.Out = failingWriter{}
			app.Flags.Output, app.Flags.OutputSet = output, true
			err := app.emptyOK(helperCmd(), nil, okJSON{ID: 7, OK: true}, "Beleg 7 gelöscht.")
			ce := clierr.From(err)
			if ce.Code != clierr.CodeError || !strings.HasPrefix(ce.Message, outputFailed) {
				t.Errorf("err = %+v, want exit 1 with %q", ce, outputFailed)
			}
		})
	}
}

// pruneTestSchemas are a request with name and a non-nullable notes and a
// response that also has the read-only id.
func pruneTestSchemas() (req, resp *openapi3.Schema) {
	req = openapi3.NewObjectSchema().
		WithProperty("name", openapi3.NewStringSchema()).
		WithProperty("notes", openapi3.NewStringSchema())
	resp = openapi3.NewObjectSchema().
		WithProperty("id", openapi3.NewIntegerSchema()).
		WithProperty("name", openapi3.NewStringSchema()).
		WithProperty("notes", openapi3.NewStringSchema())
	return req, resp
}

func writeInputFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "in.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// prunedInput: without --input nothing is read or printed; with it the
// object is pruned (read-only fields removed, nulls dropped, both named on
// stderr); errors of Prune carry the prefix --input:, those of reading
// keep their own.
func TestPrunedInput(t *testing.T) {
	req, resp := pruneTestSchemas()

	t.Run("not given", func(t *testing.T) {
		app, _, errb := newTestApp(t)
		app.In = failingReader{}
		pr, given, err := app.prunedInput(helperCmd(), req, resp)
		if err != nil || given || pr.Body != nil {
			t.Errorf("= %+v, %v, %v", pr, given, err)
		}
		if errb.Len() > 0 {
			t.Errorf("stderr = %q", errb)
		}
	})

	t.Run("pruned with hints", func(t *testing.T) {
		app, _, errb := newTestApp(t)
		c := helperCmd()
		_ = c.Flags().Set("input", writeInputFile(t, `{"id":5,"name":"Neu","notes":null}`))
		pr, given, err := app.prunedInput(c, req, resp)
		if err != nil || !given {
			t.Fatalf("given = %v, err = %v", given, err)
		}
		if len(pr.Body) != 1 || pr.Body["name"] != "Neu" {
			t.Errorf("body = %v", pr.Body)
		}
		want := "Hinweis: nur lesbare Felder entfernt: id\nHinweis: null ignoriert (Feld bleibt unverändert): notes\n"
		if errb.String() != want {
			t.Errorf("stderr = %q, want %q", errb, want)
		}
	})

	t.Run("stdin", func(t *testing.T) {
		app, _, _ := newTestApp(t)
		app.In = strings.NewReader(`{"name":"Neu"}`)
		c := helperCmd()
		_ = c.Flags().Set("input", "-")
		pr, given, err := app.prunedInput(c, req, resp)
		if err != nil || !given || pr.Body["name"] != "Neu" {
			t.Errorf("= %+v, %v, %v", pr, given, err)
		}
	})

	t.Run("unknown field without response schema", func(t *testing.T) {
		app, _, errb := newTestApp(t)
		c := helperCmd()
		_ = c.Flags().Set("input", writeInputFile(t, `{"id":5,"name":"Neu"}`))
		_, given, err := app.prunedInput(c, req, nil)
		ce := clierr.From(err)
		if !given || ce.Code != clierr.CodeValidation || !strings.HasPrefix(ce.Message, "--input: ") || !strings.Contains(ce.Message, `"id"`) {
			t.Errorf("given = %v, err = %+v", given, ce)
		}
		if errb.Len() > 0 {
			t.Errorf("stderr = %q (no hints after an error)", errb)
		}
	})

	t.Run("read error keeps its message", func(t *testing.T) {
		app, _, _ := newTestApp(t)
		c := helperCmd()
		_ = c.Flags().Set("input", filepath.Join(t.TempDir(), "fehlt.json"))
		_, given, err := app.prunedInput(c, req, resp)
		ce := clierr.From(err)
		if !given || ce.Code != clierr.CodeUsage || strings.HasPrefix(ce.Message, "--input: --input") {
			t.Errorf("given = %v, err = %+v", given, ce)
		}
	})

	t.Run("no object", func(t *testing.T) {
		app, _, _ := newTestApp(t)
		c := helperCmd()
		_ = c.Flags().Set("input", writeInputFile(t, `[1]`))
		_, _, err := app.prunedInput(c, req, resp)
		if ce := clierr.From(err); ce.Code != clierr.CodeValidation || ce.Message != "--input: Eingabe muss ein JSON-Objekt sein" {
			t.Errorf("err = %+v", ce)
		}
	})
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("stdin gelesen") }
