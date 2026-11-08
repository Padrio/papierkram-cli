package clierr

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestExitCodes(t *testing.T) {
	tests := map[Code]int{
		CodeError: 1, CodeServer: 1, CodeUsage: 2, CodeAuth: 3, CodeNotFound: 4,
		CodeValidation: 5, CodeQuota: 6, CodeConflict: 7, CodeConfirmation: 8,
		CodeReadOnly: 9, CodeNetwork: 10, CodePartial: 11, CodeOutcomeUnknown: 12,
	}
	if len(tests) != 13 {
		t.Fatalf("expected 13 codes, got %d", len(tests))
	}
	for code, want := range tests {
		if got := New(code, "m", "").Exit(); got != want {
			t.Errorf("%s: exit = %d, want %d", code, got, want)
		}
	}
}

func TestFrom(t *testing.T) {
	if got := From(errors.New("x")); got.Exit() != 1 || got.Code != CodeError || got.Message != "x" {
		t.Errorf("plain error: %+v", got)
	}
	typed := New(CodeNotFound, "weg", "ID prüfen")
	if got := From(fmt.Errorf("wrap: %w", typed)); got != typed {
		t.Errorf("wrapped typed error not unwrapped: %+v", got)
	}
	if From(nil) != nil {
		t.Error("From(nil) must be nil")
	}
	// L41: From does not guess usage errors from English cobra or pflag
	// texts. Argument errors come from the German validators of
	// internal/cli and flag errors from germanFlagError, both typed
	// (TestNoCobraValidators); an untyped error is an error of the CLI.
	for _, msg := range []string{"accepts 0 arg(s), received 1", "unknown command \"x\" for \"y\"", "required flag(s) \"a\" not set", "unknown flag: --x"} {
		if got := From(errors.New(msg)); got.Exit() != 1 || got.Code != CodeError || got.Message != msg {
			t.Errorf("%q: code %s, exit %d, message %q; want CodeError with exit 1", msg, got.Code, got.Exit(), got.Message)
		}
	}
}

// cobraValidators are cobra's own argument and flag checks; their errors
// are untyped English texts that From would report as exit 1.
var cobraValidators = regexp.MustCompile(`\bcobra\.(ExactArgs|MinimumNArgs|MaximumNArgs|RangeArgs|NoArgs|OnlyValidArgs|ExactValidArgs|MatchAll)\b|\.(MarkFlagRequired|MarkPersistentFlagRequired|MarkFlagsRequiredTogether|MarkFlagsOneRequired|MarkFlagsMutuallyExclusive)\(`)

// L41: every argument error stems from the German validators (noArgs,
// exactArgs, minArgs, …) of internal/cli: no Go file of internal/ or cmd/
// uses cobra's validators, so From needs no fallback for their texts.
func TestNoCobraValidators(t *testing.T) {
	files := 0
	for _, root := range []string{"..", filepath.Join("..", "..", "cmd")} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			files++
			for i, line := range strings.Split(string(data), "\n") {
				if m := cobraValidators.FindString(line); m != "" {
					t.Errorf("%s:%d: %s – deutschen Validator aus internal/cli verwenden", path, i+1, m)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if files < 100 {
		t.Fatalf("nur %d Go-Dateien durchsucht", files)
	}
}

func TestFormat(t *testing.T) {
	e := New(CodeNotFound, "Beleg nicht gefunden", "ID prüfen")
	e.CorrelationID = "abc-123"
	want := "Fehler: Beleg nicht gefunden\nHinweis: ID prüfen\nCorrelation-ID: abc-123\n"
	if got := e.Format(); got != want {
		t.Errorf("Format = %q, want %q", got, want)
	}
	if got := New(CodeError, "x", "").Format(); got != "Fehler: x\n" {
		t.Errorf("Format ohne Hinweis = %q", got)
	}
}
