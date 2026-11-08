package cli

import (
	"bytes"
	"context"
	"os"
	"slices"
	"testing"
	"time"
)

// Ctrl+C at the prompts of local init, time log, time stop and time
// discard ends them with exit 8 and the hint of the prompt; nothing is
// sent, the project file is not written and the timer stays (task 17).
func TestPromptInterruptedLocalAndTime(t *testing.T) {
	for _, tc := range []struct {
		name  string
		timer bool // a running timer before the command
		args  []string
		hint  string
	}{
		{"local init", false, []string{"local", "init"}, "Nichts gesendet oder geändert"},
		{"time log", false, []string{"time", "log", "--task", "545", "--user", "1", "--from", "09:00", "--to", "10:00"}, "Nichts gesendet"},
		{"time stop", true, []string{"time", "stop"}, "Nichts gesendet. Der Timer läuft weiter"},
		{"time discard", true, []string{"time", "discard"}, "Nichts gesendet oder geändert. Der Timer läuft weiter"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, srv := logApp(t, nine(t))
			if tc.timer {
				wantExit(t, app, at(app, nine(t), "time", "start", "--task", "545", "--user", "1"), 0)
			}
			before := timerFile(t, app, "")
			app.Now = func() time.Time { return nine(t).Add(time.Hour) }
			app.Out.(*bytes.Buffer).Reset()
			app.Err.(*bytes.Buffer).Reset()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			app.StdinTTY = true
			app.In = newBlockingReader(t, cancel)
			args := append(slices.Clone(tc.args), "-o", "json")
			code := waitExit(t, func() int { return executeContext(ctx, app, NewRootCmd(app), args) })
			wantExit(t, app, code, 8)
			wantInterruptedPrompt(t, app, tc.hint)
			for _, r := range srv.Requests() {
				if r.Method != "GET" {
					t.Errorf("request %s %s", r.Method, r.Path)
				}
			}
			if after := timerFile(t, app, ""); !bytes.Equal(before, after) {
				t.Errorf("timer changed: %s → %s", before, after)
			}
			if entries, _ := os.ReadDir(cwdOf(t, app)); len(entries) != 0 {
				t.Errorf("working directory written: %v", entries)
			}
		})
	}
}

// cwdOf is the working directory of the test app.
func cwdOf(t *testing.T, app *App) string {
	t.Helper()
	dir, err := app.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return dir
}
