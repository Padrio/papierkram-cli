package clierr

import (
	"errors"
	"fmt"
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
	for _, msg := range []string{"accepts 0 arg(s), received 1", "unknown command \"x\" for \"y\"", "required flag(s) \"a\" not set", "unknown flag: --x"} {
		if got := From(errors.New(msg)); got.Exit() != 2 {
			t.Errorf("%q: exit = %d, want 2", msg, got.Exit())
		}
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
